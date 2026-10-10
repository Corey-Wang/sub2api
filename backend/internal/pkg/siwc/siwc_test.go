package siwc

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestPKCEStateAndNonce(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(challenge, "+/=") {
		t.Fatalf("challenge must be unpadded base64url, got %s", challenge)
	}
	if challenge != CodeChallengeS256(verifier) {
		t.Fatal("challenge mismatch")
	}
	// RFC 7636 appendix B.
	const rfcVerifier = "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"
	const rfcChallenge = "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"
	if got := CodeChallengeS256(rfcVerifier); got != rfcChallenge {
		t.Fatalf("S256 vector: got %s", got)
	}
	state, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := RandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if state == nonce {
		t.Fatal("state and nonce must be independent")
	}
	if !StateMatches(state, state) {
		t.Fatal("state should match itself")
	}
	if StateMatches(state, nonce) || StateMatches(state, state+"x") || StateMatches("", state) {
		t.Fatal("state mismatch was accepted")
	}
}

func TestParseCallbackURL(t *testing.T) {
	const state = "state-value-0123456789abcdef"
	okURL := "http://127.0.0.1:1455/auth/callback?code=abc&state=" + state + "&client_id=oaiapp_issued"
	cb, err := ParseCallbackURL(okURL, state, "")
	if err != nil {
		t.Fatal(err)
	}
	if cb.Code != "abc" || cb.ClientID != "oaiapp_issued" {
		t.Fatalf("parsed %+v", cb)
	}

	if _, err := ParseCallbackURL(okURL, "other-state-0123456789abcdef", ""); err == nil || !strings.Contains(err.Error(), "state_mismatch") {
		t.Fatalf("state mismatch: %v", err)
	}
	missing := "http://127.0.0.1:1455/auth/callback?code=abc&state=" + state
	if _, err := ParseCallbackURL(missing, state, ""); err == nil || !strings.Contains(err.Error(), "missing_client_id") {
		t.Fatalf("missing client_id: %v", err)
	}
	dynamic := "http://127.0.0.1:1455/auth/callback?code=abc&state=" + state + "&client_id=dynamic_agent_client"
	if _, err := ParseCallbackURL(dynamic, state, ""); err == nil || !strings.Contains(err.Error(), "dynamic_agent_client") {
		t.Fatalf("dynamic client: %v", err)
	}
	mismatch := "http://127.0.0.1:1455/auth/callback?code=abc&state=" + state + "&client_id=oaiapp_other"
	if _, err := ParseCallbackURL(mismatch, state, "oaiapp_saved"); err == nil || !strings.Contains(err.Error(), "client_id_mismatch") {
		t.Fatalf("mismatch: %v", err)
	}
	reauth, err := ParseCallbackURL(missing, state, "oaiapp_saved")
	if err != nil || reauth.ClientID != "oaiapp_saved" {
		t.Fatalf("reauth without client_id: %v %+v", err, reauth)
	}
	if _, err := ParseCallbackURL("http://localhost:1455/auth/callback?code=abc&state="+state+"&client_id=oaiapp_issued", state, ""); err == nil {
		t.Fatal("localhost callback must be rejected")
	}
	denied := "http://127.0.0.1:1455/auth/callback?error=access_denied&state=" + state
	if _, err := ParseCallbackURL(denied, state, ""); err == nil || !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("access_denied: %v", err)
	}
}

func TestAuthorizeURL(t *testing.T) {
	raw, err := AuthorizeParams("https://auth.openai.com/api/accounts/authorize", "", DefaultRedirectURI(), "st", "no", "ch", "urn:uuid:11111111-1111-4111-8111-111111111111", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(raw, "client_id=dynamic_agent_client") || !strings.Contains(raw, "agent_name_hint=sub2api") || strings.Contains(raw, "id_token_hint") {
		t.Fatal(raw)
	}
	reauth, err := AuthorizeParams("https://auth.openai.com/api/accounts/authorize", "oaiapp_saved", DefaultRedirectURI(), "st", "no", "ch", "urn:uuid:11111111-1111-4111-8111-111111111111", "user@example.com", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reauth, "dynamic_agent_client") || strings.Contains(reauth, "agent_name_hint") || !strings.Contains(reauth, "login_hint=") || !strings.Contains(reauth, "client_id=oaiapp_saved") {
		t.Fatal(reauth)
	}
	if _, err := AuthorizeParams("https://auth.openai.com/api/accounts/authorize", "", "http://localhost:1455/auth/callback", "st", "no", "ch", "urn:uuid:x", "", false); err == nil {
		t.Fatal("localhost redirect accepted")
	}
}

func TestTokenRefreshRotationAndInvalidGrant(t *testing.T) {
	var got urlValues
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got = parseForm(string(body))
		if got["refresh_token"] == "old-refresh" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"new-access","refresh_token":"rotated-refresh","token_type":"Bearer","expires_in":3600,"scope":"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct","earliest_refresh_at":"2099-01-01T00:00:00Z"}`))
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_grant","error_description":"refresh_token_reused"}`))
	}))
	defer srv.Close()

	set, err := RefreshToken(context.Background(), srv.Client(), srv.URL, "oaiapp_saved", "old-refresh", strings.Fields(Scopes))
	if err != nil {
		t.Fatal(err)
	}
	if got["grant_type"] != "refresh_token" || got["scope"] != "" || got["client_id"] != "oaiapp_saved" || got["resource"] != Resource {
		t.Fatalf("form: %#v", got)
	}
	if set.AccessToken != "new-access" || set.RefreshToken != "rotated-refresh" || set.EarliestRefreshAt == nil {
		t.Fatalf("token set %+v", set)
	}
	if !ScopeGranted(set.Scopes) {
		t.Fatal("sharing scope missing")
	}

	_, err = RefreshToken(context.Background(), srv.Client(), srv.URL, "oaiapp_saved", "consumed", nil)
	if !IsUnusableRefresh(err) {
		t.Fatalf("expected unusable refresh, got %v", err)
	}
	oauthErr := err.(*OAuthError)
	if oauthErr.Code != "invalid_grant" {
		t.Fatal(oauthErr.Code)
	}
}

func TestRevokeRetries5xx(t *testing.T) {
	RevokeBackoff = func(int) time.Duration { return 0 }
	t.Cleanup(func() { RevokeBackoff = nil })
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	if err := RevokeRefreshToken(context.Background(), srv.Client(), srv.URL, "rt", "oaiapp_saved"); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestSanitizeRequest(t *testing.T) {
	raw := []byte(`{"model":"gpt-5","temperature":0.2,"top_p":1,"metadata":{"a":1},"previous_response_id":"resp_old","store":true,"input":"hello","instructions":"be brief"}`)
	got, err := SanitizeRequest(raw)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(got.Body, &doc); err != nil {
		t.Fatal(err)
	}
	if doc["store"] != false || doc["stream"] != true {
		t.Fatalf("flags: %#v", doc)
	}
	for _, field := range []string{"temperature", "top_p", "metadata", "previous_response_id"} {
		if _, ok := doc[field]; ok {
			t.Fatalf("%s was not stripped", field)
		}
	}
	if !contains(got.Stripped, "temperature") || !contains(got.Stripped, "previous_response_id") {
		t.Fatalf("stripped %#v", got.Stripped)
	}
	input := doc["input"].([]any)
	item := input[0].(map[string]any)
	if item["role"] != "user" || item["content"] != "hello" {
		t.Fatalf("input %#v", item)
	}
	if doc["instructions"] != "be brief" {
		t.Fatal("instructions should be kept")
	}

	systemBody := []byte(`{"model":"gpt-5","input":[{"type":"message","role":"system","content":"x"}]}`)
	system, err := SanitizeRequest(systemBody)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(system.Notes, "system role -> developer") {
		t.Fatal(system.Notes)
	}
}

func TestSSEPassthroughAndAggregate(t *testing.T) {
	raw := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hi\"}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"output\":[{\"type\":\"message\"}]}}\n\n")
	if string(PassthroughSSE(raw)) != string(raw) {
		t.Fatal("passthrough changed bytes")
	}
	agg, err := AggregateResponsesSSE(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agg.Body), `"id":"resp_1"`) || strings.Contains(string(agg.Body), "response.output_text.delta") {
		t.Fatalf("aggregated %s", agg.Body)
	}

	failed := []byte("event: response.failed\ndata: {\"type\":\"response.failed\",\"response\":{\"error\":{\"code\":\"subscription_sharing_usage_limit_exceeded\",\"message\":\"limit\"}}}\n\n")
	_, err = AggregateResponsesSSE(failed)
	if err == nil || !strings.Contains(err.Error(), "subscription_sharing_usage_limit_exceeded") {
		t.Fatalf("failed sse: %v", err)
	}
}

func TestClassify429(t *testing.T) {
	got := ClassifyUpstreamError(http.StatusTooManyRequests, "subscription_sharing_usage_limit_exceeded")
	if got.Action != ActionRateLimit || got.HTTPStatus != http.StatusTooManyRequests {
		t.Fatalf("%+v", got)
	}
	if got := ClassifyUpstreamError(http.StatusUnauthorized, "subscription_sharing_invalid_user"); got.Action != ActionReauth {
		t.Fatalf("%+v", got)
	}
	if got := ClassifyUpstreamError(http.StatusBadRequest, "subscription_sharing_unsupported_capability"); got.Action != ActionNone || got.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("%+v", got)
	}
	if got := ClassifyUpstreamError(http.StatusServiceUnavailable, "subscription_sharing_usage_unavailable"); got.Action != ActionTemporary {
		t.Fatalf("%+v", got)
	}
	if !KnownCode("chatpass_v2_scope_not_authorized") || KnownCode("rate_limit_exceeded") {
		t.Fatal("known code set")
	}
}

func TestNeedsRefreshHonorsEarliest(t *testing.T) {
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	expires := now.Add(2 * time.Minute)
	earliest := now.Add(10 * time.Minute)
	if NeedsRefresh(expires, &earliest, now, RefreshMargin) {
		t.Fatal("proactive refresh should wait for earliest_refresh_at")
	}
	if !NeedsRefresh(now.Add(-time.Second), &earliest, now, RefreshMargin) {
		t.Fatal("expired token must refresh even when earliest_refresh_at is in the future")
	}
	if NeedsRefresh(now.Add(time.Hour), nil, now, RefreshMargin) {
		t.Fatal("fresh token")
	}
	parsed, ok := ParseEarliestRefreshAt("1700000000000")
	if !ok || parsed.UnixMilli() != 1700000000000 {
		t.Fatalf("ms parse: %v %v", parsed, ok)
	}
}

func TestVerifyIDToken(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	jwks := map[string]any{"keys": []any{rsaJWK(key, "kid-1")}}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "openid-configuration") {
			_ = json.NewEncoder(w).Encode(Discovery{
				Issuer:                srv.URL,
				AuthorizationEndpoint: srv.URL + "/authorize",
				TokenEndpoint:         srv.URL + "/token",
				JWKSURI:               srv.URL + "/jwks",
				RevocationEndpoint:    srv.URL + "/revoke",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(jwks)
	}))
	defer srv.Close()
	disc, err := FetchDiscovery(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	nonce := "nonce-1"
	token := signToken(t, key, jwt.MapClaims{
		"iss": srv.URL, "aud": "oaiapp_issued", "sub": "user-1", "email": "a@example.com",
		"nonce": nonce, "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
	})
	claims, err := VerifyIDToken(context.Background(), srv.Client(), disc.JWKSURI, disc.Issuer, token, "oaiapp_issued", nonce)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Email != "a@example.com" || claims.Subject != "user-1" {
		t.Fatalf("%+v", claims)
	}
	if _, err := VerifyIDToken(context.Background(), srv.Client(), disc.JWKSURI, disc.Issuer, token, "oaiapp_issued", "other"); err == nil {
		t.Fatal("nonce mismatch accepted")
	}
	if _, err := VerifyIDToken(context.Background(), srv.Client(), disc.JWKSURI, disc.Issuer, token, "oaiapp_other", nonce); err == nil {
		t.Fatal("audience mismatch accepted")
	}
}

func TestAllowedUpstream(t *testing.T) {
	if err := AllowedUpstream("https://api.openai.com/v1/responses", false); err != nil {
		t.Fatal(err)
	}
	if err := AllowedUpstream("https://chatgpt.com/backend-api/codex/responses", false); err == nil {
		t.Fatal("chatgpt backend accepted")
	}
	if err := AllowedUpstream("https://api.openai.com/v1/chat/completions", false); err == nil {
		t.Fatal("chat completions accepted")
	}
}

func rsaJWK(key *rsa.PrivateKey, kid string) map[string]string {
	return map[string]string{
		"kty": "RSA",
		"kid": kid,
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
	}
}

func signToken(t *testing.T, key *rsa.PrivateKey, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid-1"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

type urlValues map[string]string

func parseForm(body string) urlValues {
	parsed, _ := url.ParseQuery(body)
	out := urlValues{}
	for key, values := range parsed {
		if len(values) > 0 {
			out[key] = values[0]
		}
	}
	return out
}

func contains(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}
