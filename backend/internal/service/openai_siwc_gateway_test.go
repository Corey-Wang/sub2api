package service

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type siwcUpstreamStub struct {
	responses []*http.Response
	requests  []*http.Request
}

func (s *siwcUpstreamStub) Do(req *http.Request, _ string, _ int64, _ int) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	if req.Body != nil && req.GetBody != nil {
		body, err := req.GetBody()
		if err == nil {
			cloned.Body = body
		}
	}
	s.requests = append(s.requests, cloned)
	if len(s.responses) == 0 {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}, nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return resp, nil
}

func (s *siwcUpstreamStub) DoWithTLS(req *http.Request, proxyURL string, accountID int64, accountConcurrency int, _ *tlsfingerprint.Profile) (*http.Response, error) {
	return s.Do(req, proxyURL, accountID, accountConcurrency)
}

func siwcTestAccount() *Account {
	return &Account{
		ID:          9,
		Platform:    PlatformOpenAI,
		Type:        AccountTypeSIWC,
		Concurrency: 1,
		Credentials: map[string]any{"owner_user_id": int64(7), "access_token": "stale-token"},
	}
}

func siwcTestContext(userID int64, exclusive bool) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	c.Set("api_key", &APIKey{UserID: userID, Group: &Group{IsExclusive: exclusive}})
	return c, w
}

func TestBuildSIWCUpstreamRequestPersonalUseAndSanitize(t *testing.T) {
	svc := &OpenAIGatewayService{}
	account := siwcTestAccount()

	denied, deniedRec := siwcTestContext(8, true)
	_, err := svc.buildSIWCUpstreamRequest(context.Background(), denied, account, []byte(`{"model":"gpt-5","temperature":0.2,"input":"hi"}`), "token")
	require.Error(t, err)
	require.Contains(t, err.Error(), "upstream response failed:")
	require.Equal(t, http.StatusForbidden, deniedRec.Code)
	require.Contains(t, deniedRec.Body.String(), "siwc_personal_use")

	shared, _ := siwcTestContext(7, false)
	_, err = svc.buildSIWCUpstreamRequest(context.Background(), shared, account, []byte(`{"model":"gpt-5","input":"hi"}`), "token")
	require.Error(t, err)

	allowed, _ := siwcTestContext(7, true)
	req, err := svc.buildSIWCUpstreamRequest(context.Background(), allowed, account, []byte(`{"model":"gpt-5","temperature":0.2,"input":"hi"}`), "token")
	require.NoError(t, err)
	require.Equal(t, "https://api.openai.com/v1/responses", req.URL.String())
	require.Equal(t, siwc.UserAgent, req.Header.Get("User-Agent"))
	require.Equal(t, "Bearer token", req.Header.Get("Authorization"))
	require.Empty(t, req.Header.Get("originator"))
	body, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.NotContains(t, string(body), "temperature")
	require.Contains(t, string(body), `"store":false`)
	require.Contains(t, string(body), `"stream":true`)
}

func TestDoSIWCUpstreamRetries401Once(t *testing.T) {
	siwcAllowTestUpstream = true
	t.Cleanup(func() { siwcAllowTestUpstream = false })

	stub := &siwcUpstreamStub{responses: []*http.Response{
		{StatusCode: http.StatusUnauthorized, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"chatpass_v2_invalid_authorization_context"}}`)), Header: make(http.Header)},
		{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("event: response.completed\ndata: {}\n\n")), Header: make(http.Header)},
	}}
	account := siwcTestAccount()
	svc := &OpenAIGatewayService{httpUpstream: stub}
	svc.siwcForceRefresh = func(context.Context, *Account, string) (string, error) {
		return "fresh-token", nil
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1/v1/responses", strings.NewReader(`{"model":"gpt-5"}`))
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer stale-token")
	req.Header.Set("originator", "codex")
	resp, err := svc.doSIWCUpstream(req, "", account)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Len(t, stub.requests, 2)
	require.Equal(t, "Bearer fresh-token", stub.requests[1].Header.Get("Authorization"))
	require.Empty(t, stub.requests[1].Header.Get("originator"))
	require.Equal(t, siwc.UserAgent, stub.requests[1].Header.Get("User-Agent"))
}

func TestSIWCKnownErrorCode(t *testing.T) {
	code, ok := SIWCKnownErrorCode([]byte(`{"error":{"code":"subscription_sharing_usage_limit_exceeded","message":"limit"}}`))
	require.True(t, ok)
	require.Equal(t, "subscription_sharing_usage_limit_exceeded", code)
	_, ok = SIWCKnownErrorCode([]byte(`{"error":{"code":"something_else"}}`))
	require.False(t, ok)
}
