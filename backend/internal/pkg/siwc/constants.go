// Package siwc implements OpenAI's official Sign in with ChatGPT token-sharing
// flow for open-source / locally hosted apps.
//
// Docs: https://developers.openai.com/siwc/token-sharing-open-source
// and the sign-in, token-reference, profiles-and-sessions, models-and-inference,
// errors-and-recovery, and preview-limitations subpages.
//
// This program lets a user call the API with their own ChatGPT plan. Sharing
// that account with other people violates OpenAI's terms. Callers must keep
// these accounts out of multi-user groups.
package siwc

import "time"

const (
	Issuer           = "https://auth.openai.com"
	Resource         = "https://api.openai.com/v1"
	APIBase          = "https://api.openai.com/v1"
	Scopes           = "openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"
	SharingScope     = "chatgpt.tokens.use.direct"
	DynamicClient    = "dynamic_agent_client"
	AgentNameHint    = "sub2api"
	CallbackPath     = "/auth/callback"
	DefaultPort      = 1455
	UserAgent        = "sub2api/0.2.15 (siwc)"
	RefreshMargin    = 5 * time.Minute
	SessionTTL       = 30 * time.Minute
	UsagePageURL     = "https://chatgpt.com/settings/usage"
	HostIDSettingKey = "siwc_ext_agent_host_id"
)

// DefaultRedirectURI is the loopback callback used when the admin does not
// pick another port. Docs require http://127.0.0.1:<port>/auth/callback
// (never localhost).
func DefaultRedirectURI() string {
	return "http://127.0.0.1:1455/auth/callback"
}

// UnsupportedFields are the preview-limitation fields that must be omitted,
// plus previous_response_id which HTTP sharing does not support.
var UnsupportedFields = []string{
	"background",
	"conversation",
	"max_output_tokens",
	"max_tool_calls",
	"metadata",
	"moderation",
	"multi_agent",
	"prompt",
	"prompt_cache_retention",
	"safety_identifier",
	"temperature",
	"top_logprobs",
	"top_p",
	"truncation",
	"user",
	"previous_response_id",
}

// UnusableRefreshCodes mark a refresh token that must not be retried.
var UnusableRefreshCodes = map[string]struct{}{
	"invalid_grant":             {},
	"invalid_refresh_token":     {},
	"token_expired":             {},
	"refresh_token_expired":     {},
	"refresh_token_invalidated": {},
	"refresh_token_reused":      {},
	"invalid_client":            {},
}

// AllowedIDTokenAlgs are the asymmetric JOSE algorithms accepted for ID tokens.
// The docs do not list signing algorithms; HMAC and "none" are rejected.
var AllowedIDTokenAlgs = map[string]struct{}{
	"RS256": {},
	"RS384": {},
	"RS512": {},
	"PS256": {},
	"PS384": {},
	"PS512": {},
	"ES256": {},
	"ES384": {},
}
