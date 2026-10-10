package siwc

import (
	"net/http"
	"net/url"
	"strings"
)

// AllowedUpstream reports whether a URL is one of the two documented inference routes.
// Tests may pass an http://127.0.0.1 base; production callers must use https://api.openai.com.
func AllowedUpstream(raw string, allowLoopback bool) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return &OAuthError{Code: "invalid_upstream", Description: "upstream URL is invalid"}
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "chatgpt.com" || strings.HasSuffix(host, ".chatgpt.com") || strings.Contains(parsed.Path, "backend-api") {
		return &OAuthError{Code: "invalid_upstream", Description: "chatgpt.com/backend-api is not used for SIWC accounts"}
	}
	path := strings.TrimRight(parsed.Path, "/")
	switch path {
	case "/v1/responses", "/v1/models":
	default:
		return &OAuthError{Code: "invalid_upstream", Description: "SIWC accounts may only call /v1/responses and /v1/models"}
	}
	if host == "api.openai.com" && parsed.Scheme == "https" {
		return nil
	}
	if allowLoopback && (host == "127.0.0.1" || host == "localhost") && (parsed.Scheme == "http" || parsed.Scheme == "https") {
		return nil
	}
	return &OAuthError{Code: "invalid_upstream", Description: "SIWC upstream must be https://api.openai.com"}
}

// UpstreamHeaderAllowlist is the only header set sent upstream.
var upstreamHeaderAllow = map[string]struct{}{
	"authorization": {},
	"content-type":  {},
	"accept":        {},
	"user-agent":    {},
}

// SanitizeHeaders drops Codex / browser fingerprint headers and sets an honest User-Agent.
func SanitizeHeaders(header http.Header, accessToken string, models bool) http.Header {
	out := make(http.Header)
	for key, values := range header {
		if _, ok := upstreamHeaderAllow[strings.ToLower(key)]; !ok || len(values) == 0 {
			continue
		}
		if strings.EqualFold(key, "User-Agent") || strings.EqualFold(key, "Authorization") || strings.EqualFold(key, "Accept") {
			continue
		}
		out[http.CanonicalHeaderKey(key)] = append([]string(nil), values...)
	}
	out.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	out.Set("User-Agent", UserAgent)
	if models {
		out.Set("Accept", "application/json")
	} else {
		out.Set("Accept", "text/event-stream")
		if out.Get("Content-Type") == "" {
			out.Set("Content-Type", "application/json")
		}
	}
	return out
}
