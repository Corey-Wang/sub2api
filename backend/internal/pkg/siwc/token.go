package siwc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// OAuthError is a protocol error with a stable code.
type OAuthError struct {
	Code        string
	Description string
	HTTPStatus  int
}

func (e *OAuthError) Error() string {
	if e == nil {
		return ""
	}
	message := e.Code
	if e.Description != "" {
		message = e.Code + ": " + e.Description
	}
	// Sub2API's refresh lock treats the substring invalid_grant as a possible
	// concurrent rotation. Surface every unusable refresh code that way so
	// refresh_token_reused can still recover when another worker already saved
	// the rotated token.
	if e.Code != "invalid_grant" {
		if _, ok := UnusableRefreshCodes[e.Code]; ok {
			return "invalid_grant: " + message
		}
	}
	return message
}

// TokenSet is a parsed token response.
type TokenSet struct {
	AccessToken       string
	RefreshToken      string
	IDToken           string
	TokenType         string
	ExpiresIn         int64
	ExpiresAt         time.Time
	Scopes            []string
	EarliestRefreshAt *time.Time
}

// ExchangeCode trades an authorization code for tokens.
func ExchangeCode(ctx context.Context, client *http.Client, tokenEndpoint, clientID, code, verifier, redirectURI string) (*TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("client_id", clientID)
	form.Set("code", code)
	form.Set("code_verifier", verifier)
	form.Set("redirect_uri", redirectURI)
	form.Set("resource", Resource)
	return tokenRequest(ctx, client, tokenEndpoint, form, nil, true)
}

// RefreshToken rotates the refresh token. Scope is omitted so the existing grant is kept.
func RefreshToken(ctx context.Context, client *http.Client, tokenEndpoint, clientID, refreshToken string, previousScopes []string) (*TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("client_id", clientID)
	form.Set("refresh_token", refreshToken)
	form.Set("resource", Resource)
	return tokenRequest(ctx, client, tokenEndpoint, form, previousScopes, true)
}

// RevokeRefreshToken posts the documented revocation request.
// An empty HTTP 200 is success, including for an already-invalid token.
// Network errors and HTTP 5xx are retried.
func RevokeRefreshToken(ctx context.Context, client *http.Client, revocationEndpoint, refreshToken, clientID string) error {
	if client == nil {
		client = http.DefaultClient
	}
	form := url.Values{}
	form.Set("token", refreshToken)
	form.Set("token_type_hint", "refresh_token")
	form.Set("client_id", clientID)
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(revokeBackoff(attempt)):
			}
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, revocationEndpoint, strings.NewReader(form.Encode()))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", UserAgent)
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return nil
		}
		lastErr = fmt.Errorf("siwc revoke: HTTP %d", resp.StatusCode)
		if resp.StatusCode < 500 {
			return lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("siwc revoke: failed")
	}
	return lastErr
}

func revokeBackoff(attempt int) time.Duration {
	if RevokeBackoff != nil {
		return RevokeBackoff(attempt)
	}
	return time.Duration(attempt) * 50 * time.Millisecond
}

// RevokeBackoff overrides the retry delay. Tests set this to zero.
var RevokeBackoff func(attempt int) time.Duration

func tokenRequest(ctx context.Context, client *http.Client, endpoint string, form url.Values, previousScopes []string, requireRefresh bool) (*TokenSet, error) {
	if client == nil {
		client = http.DefaultClient
	}
	if strings.TrimSpace(form.Get("client_id")) == "" || form.Get("client_id") == DynamicClient {
		return nil, &OAuthError{Code: "invalid_client", Description: "token request requires the issued client_id"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("siwc token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, parseOAuthError(resp.StatusCode, body)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "token endpoint returned non-JSON"}
	}
	return parseTokenResponse(data, previousScopes, requireRefresh, time.Now())
}

func parseOAuthError(status int, body []byte) error {
	code := fmt.Sprintf("http_%d", status)
	desc := strings.TrimSpace(string(body))
	var data map[string]any
	if json.Unmarshal(body, &data) == nil {
		switch errField := data["error"].(type) {
		case string:
			if errField != "" {
				code = errField
			}
			if d, ok := data["error_description"].(string); ok {
				desc = d
			}
		case map[string]any:
			if c, ok := errField["code"].(string); ok && c != "" {
				code = c
			} else if t, ok := errField["type"].(string); ok && t != "" {
				code = t
			}
			if m, ok := errField["message"].(string); ok {
				desc = m
			}
		}
	}
	return &OAuthError{Code: code, Description: desc, HTTPStatus: status}
}

func parseTokenResponse(data map[string]any, previousScopes []string, requireRefresh bool, now time.Time) (*TokenSet, error) {
	scopes, err := parseScopes(data["scope"], previousScopes)
	if err != nil {
		return nil, err
	}
	access, _ := data["access_token"].(string)
	if access == "" {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "token response had no access_token"}
	}
	tokenType, _ := data["token_type"].(string)
	if !strings.EqualFold(tokenType, "bearer") {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "unexpected token_type"}
	}
	expiresIn, ok := asPositiveInt(data["expires_in"])
	if !ok {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "token response had no valid expires_in"}
	}
	refresh, _ := data["refresh_token"].(string)
	if requireRefresh && refresh == "" {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "token response had no refresh_token"}
	}
	if containsScope(scopes, "offline_access") && refresh == "" {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "offline_access grant had no refresh_token"}
	}
	idToken, _ := data["id_token"].(string)
	set := &TokenSet{
		AccessToken:  access,
		RefreshToken: refresh,
		IDToken:      idToken,
		TokenType:    "Bearer",
		ExpiresIn:    expiresIn,
		ExpiresAt:    now.Add(time.Duration(expiresIn) * time.Second),
		Scopes:       scopes,
	}
	if earliest, ok := ParseEarliestRefreshAt(data["earliest_refresh_at"]); ok {
		set.EarliestRefreshAt = &earliest
	}
	return set, nil
}

func parseScopes(raw any, previous []string) ([]string, error) {
	if raw == nil && previous != nil {
		return append([]string(nil), previous...), nil
	}
	text, ok := raw.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil, &OAuthError{Code: "invalid_token_response", Description: "token response did not confirm granted scopes"}
	}
	return strings.Fields(text), nil
}

func containsScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if scope == want {
			return true
		}
	}
	return false
}

func asPositiveInt(v any) (int64, bool) {
	switch n := v.(type) {
	case float64:
		if n > 0 {
			return int64(n), true
		}
	case int:
		if n > 0 {
			return int64(n), true
		}
	case int64:
		if n > 0 {
			return n, true
		}
	case json.Number:
		i, err := n.Int64()
		if err == nil && i > 0 {
			return i, true
		}
	case string:
		i, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		if err == nil && i > 0 {
			return i, true
		}
	}
	return 0, false
}

// ParseEarliestRefreshAt accepts epoch seconds, epoch milliseconds, or RFC3339.
// The docs list the field but do not define its format. Unrecognized values are ignored.
func ParseEarliestRefreshAt(v any) (time.Time, bool) {
	switch n := v.(type) {
	case nil, bool:
		return time.Time{}, false
	case float64:
		return earliestFromNumber(n), true
	case int64:
		return earliestFromNumber(float64(n)), true
	case int:
		return earliestFromNumber(float64(n)), true
	case json.Number:
		f, err := n.Float64()
		if err != nil {
			return time.Time{}, false
		}
		return earliestFromNumber(f), true
	case string:
		text := strings.TrimSpace(n)
		if text == "" {
			return time.Time{}, false
		}
		if f, err := strconv.ParseFloat(text, 64); err == nil {
			return earliestFromNumber(f), true
		}
		if t, err := time.Parse(time.RFC3339, text); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func earliestFromNumber(v float64) time.Time {
	if v > 1e12 {
		v = v / 1000
	}
	sec := int64(v)
	nsec := int64((v - float64(sec)) * float64(time.Second))
	return time.Unix(sec, nsec).UTC()
}

// IsUnusableRefresh reports refresh errors that require a new sign-in.
func IsUnusableRefresh(err error) bool {
	oauthErr, ok := err.(*OAuthError)
	if !ok || oauthErr == nil {
		return false
	}
	_, found := UnusableRefreshCodes[oauthErr.Code]
	return found
}

// NeedsRefresh is true when the access token is expired or inside the margin.
// earliest_refresh_at delays proactive refresh only; an already-expired token refreshes anyway.
func NeedsRefresh(expiresAt time.Time, earliest *time.Time, now time.Time, margin time.Duration) bool {
	if expiresAt.IsZero() || !expiresAt.After(now) {
		return true
	}
	if expiresAt.Sub(now) >= margin {
		return false
	}
	if earliest != nil && now.Before(*earliest) {
		return false
	}
	return true
}

// ScopeGranted reports whether the sharing scope was included.
func ScopeGranted(scopes []string) bool {
	return containsScope(scopes, SharingScope)
}
