package service

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

var errSIWCPersonalUse = errString("官方 SIWC 账号仅限本人使用，不能通过共享分组调用")

type errString string

func (e errString) Error() string { return string(e) }

// buildSIWCUpstreamRequest builds a /v1/responses request that does not use
// ChatGPT backend-api or Codex client impersonation.
func (s *OpenAIGatewayService) buildSIWCUpstreamRequest(ctx context.Context, c *gin.Context, account *Account, body []byte, token string) (*http.Request, error) {
	if err := s.rejectSIWCPersonalUse(c, account); err != nil {
		return nil, err
	}
	sanitized, err := siwc.SanitizeRequest(body)
	if err != nil {
		return nil, err
	}
	if len(sanitized.Stripped) > 0 || len(sanitized.Notes) > 0 {
		accountID := int64(0)
		if account != nil {
			accountID = account.ID
		}
		slog.Info("siwc_request_sanitized",
			"account_id", accountID,
			"stripped", strings.Join(sanitized.Stripped, ","),
			"notes", strings.Join(sanitized.Notes, ","),
		)
	}
	targetURL := siwc.APIBase + "/responses"
	if err := siwc.AllowedUpstream(targetURL, false); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewReader(sanitized.Body))
	if err != nil {
		return nil, err
	}
	req.Header = siwc.SanitizeHeaders(nil, token, false)
	req.Host = "api.openai.com"
	return req, nil
}

func (s *OpenAIGatewayService) rejectSIWCPersonalUse(c *gin.Context, account *Account) error {
	if account == nil || !account.IsOpenAISIWC() {
		return nil
	}
	apiKey := getAPIKeyFromContext(c)
	owner := siwcOwnerUserID(account)
	allowed := apiKey != nil && apiKey.Group != nil && apiKey.Group.IsExclusive && (owner <= 0 || apiKey.UserID == owner)
	if allowed {
		return nil
	}
	if c != nil {
		c.JSON(http.StatusForbidden, gin.H{
			"error": gin.H{
				"type":    "permission_error",
				"code":    "siwc_personal_use",
				"message": errSIWCPersonalUse.Error(),
			},
		})
	}
	// Prefix matches openAIForwardErrorAlreadyCommunicated so the handler does not append a second error body.
	return errString("upstream response failed: " + errSIWCPersonalUse.Error())
}

func siwcOwnerUserID(account *Account) int64 {
	if account == nil {
		return 0
	}
	return account.GetCredentialAsInt64("owner_user_id")
}

// doSIWCUpstream sends the request with the normal HTTP client (no utls / Codex plugin).
// A 401 is retried once after a serialized refresh, unless another worker already rotated the token.
func (s *OpenAIGatewayService) doSIWCUpstream(request *http.Request, proxyURL string, account *Account) (*http.Response, error) {
	if request == nil || request.URL == nil {
		return nil, errString("siwc upstream request is missing")
	}
	if err := siwc.AllowedUpstream(request.URL.String(), siwcAllowTestUpstream); err != nil {
		return nil, err
	}
	request.Header = siwc.SanitizeHeaders(request.Header, bearerToken(request), strings.HasSuffix(strings.TrimRight(request.URL.Path, "/"), "/models"))
	request.Host = request.URL.Host
	resp, err := s.httpUpstream.Do(request, proxyURL, account.ID, account.Concurrency)
	if err != nil || resp == nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	stale := bearerToken(request)
	fresh, refreshErr := s.forceRefreshSIWC(request.Context(), account, stale)
	if refreshErr != nil || fresh == "" || fresh == stale {
		return resp, err
	}
	retry, retryErr := cloneSIWCRequest(request, fresh)
	if retryErr != nil {
		return resp, err
	}
	if resp.Body != nil {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		_ = resp.Body.Close()
	}
	return s.httpUpstream.Do(retry, proxyURL, account.ID, account.Concurrency)
}

// siwcAllowTestUpstream lets unit tests point the upstream at 127.0.0.1.
var siwcAllowTestUpstream bool

func (s *OpenAIGatewayService) forceRefreshSIWC(ctx context.Context, account *Account, stale string) (string, error) {
	if s != nil && s.siwcForceRefresh != nil {
		return s.siwcForceRefresh(ctx, account, stale)
	}
	if s == nil || s.openAITokenProvider == nil {
		return "", errString("siwc token provider is not configured")
	}
	return s.openAITokenProvider.ForceRefreshSIWC(ctx, account, stale)
}

func bearerToken(req *http.Request) string {
	if req == nil {
		return ""
	}
	value := strings.TrimSpace(req.Header.Get("Authorization"))
	value = strings.TrimPrefix(value, "Bearer ")
	return strings.TrimPrefix(value, "bearer ")
}

func cloneSIWCRequest(req *http.Request, token string) (*http.Request, error) {
	var body io.Reader
	if req.GetBody != nil {
		cloned, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		body = cloned
	}
	clone := req.Clone(req.Context())
	if body != nil {
		clone.Body = io.NopCloser(body)
	}
	clone.Header = siwc.SanitizeHeaders(req.Header, token, strings.HasSuffix(strings.TrimRight(req.URL.Path, "/"), "/models"))
	clone.Host = req.URL.Host
	return clone, nil
}

func (s *OpenAIGatewayService) fetchSIWCModelsList(ctx context.Context, account *Account) (*OpenAIModelsResponse, error) {
	token, _, err := s.GetAccessToken(ctx, account)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siwc.APIBase+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header = siwc.SanitizeHeaders(nil, token, true)
	proxyURL := ""
	if account.Proxy != nil {
		proxyURL = account.Proxy.URL()
	}
	resp, err := s.doSIWCUpstream(req, proxyURL, account)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errString("siwc models upstream failed")
	}
	filtered, err := siwc.FilterListedModels(body)
	if err != nil {
		return nil, err
	}
	standard, err := standardOpenAIModelsBody(filtered, true)
	if err != nil {
		return nil, err
	}
	return &OpenAIModelsResponse{Body: standard}, nil
}

func (s *OpenAIGatewayService) handleSIWCUpstreamError(ctx context.Context, account *Account, statusCode int, responseBody []byte) (handled bool, shouldDisable bool) {
	code := siwcErrorCode(responseBody)
	if !siwc.KnownCode(code) {
		return false, false
	}
	classified := siwc.ClassifyUpstreamError(statusCode, code)
	switch classified.Action {
	case siwc.ActionRateLimit:
		if s != nil && s.rateLimitService != nil {
			s.rateLimitService.apply429FallbackRateLimit(ctx, account, "siwc:"+code)
		}
		return true, false
	case siwc.ActionTemporary:
		if s != nil && s.accountRepo != nil {
			_ = s.accountRepo.SetTempUnschedulable(ctx, account.ID, time.Now().Add(5*time.Minute), "siwc:"+code)
		}
		return true, false
	case siwc.ActionReauth:
		message := "需要重新授权 (" + code + ")"
		if code == "subscription_sharing_user_not_eligible" {
			message = "账号不符合官方 SIWC 使用条件 (" + code + ")"
		}
		if s != nil && s.rateLimitService != nil {
			s.rateLimitService.handleAuthError(ctx, account, message)
		} else if s != nil && s.accountRepo != nil {
			_ = s.accountRepo.SetError(ctx, account.ID, message)
		}
		return true, true
	default:
		// Unsupported capability / route errors must not disable the account.
		return true, false
	}
}

func siwcErrorCode(body []byte) string {
	code := strings.TrimSpace(extractUpstreamErrorCode(body))
	if code != "" {
		return code
	}
	for _, path := range []string{"response.error.code", "error.code", "code"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			return value
		}
	}
	return ""
}

// SIWCKnownErrorCode returns a documented sharing error code found in an upstream body.
func SIWCKnownErrorCode(body []byte) (string, bool) {
	code := siwcErrorCode(body)
	if !siwc.KnownCode(code) {
		return "", false
	}
	return code, true
}
