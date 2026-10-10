package siwc

import "net/http"

// AccountAction is how a documented sharing error should move account state.
type AccountAction string

const (
	// ActionNone leaves the account schedulable. The error is still returned to the client.
	ActionNone AccountAction = ""
	// ActionRateLimit marks the account rate-limited. The reset time is not known.
	ActionRateLimit AccountAction = "rate_limited"
	// ActionReauth marks the account as needing a new sign-in.
	ActionReauth AccountAction = "needs_reauth"
	// ActionTemporary marks a short unschedulable window.
	ActionTemporary AccountAction = "temporary"
)

// ClassifiedError is the account-state decision for one upstream error.
type ClassifiedError struct {
	Code       string
	Action     AccountAction
	HTTPStatus int
	Message    string
}

// ClassifyUpstreamError maps documented Sign in with ChatGPT error codes.
// status is the HTTP status when known; 0 lets the code imply a status.
func ClassifyUpstreamError(status int, code string) ClassifiedError {
	out := ClassifiedError{Code: code, HTTPStatus: status}
	switch code {
	case "subscription_sharing_usage_limit_exceeded":
		out.Action = ActionRateLimit
		out.HTTPStatus = http.StatusTooManyRequests
		out.Message = "ChatGPT plan usage limit exceeded. See " + UsagePageURL + ". Do not infer a reset time."
	case "subscription_sharing_usage_unavailable", "subscription_sharing_user_unavailable":
		out.Action = ActionTemporary
		if status == 0 {
			out.HTTPStatus = http.StatusServiceUnavailable
		}
		out.Message = "ChatGPT plan usage is temporarily unavailable."
	case "subscription_sharing_invalid_user", "chatpass_v2_scope_not_authorized", "chatpass_v2_invalid_authorization_context":
		out.Action = ActionReauth
		if status == 0 {
			out.HTTPStatus = http.StatusUnauthorized
		}
		if code == "subscription_sharing_invalid_user" {
			out.HTTPStatus = http.StatusUnauthorized
		} else if status == 0 || status == http.StatusUnauthorized {
			out.HTTPStatus = http.StatusForbidden
		}
		out.Message = "Sign in with ChatGPT authorization must be renewed."
	case "subscription_sharing_user_not_eligible":
		out.Action = ActionReauth
		out.HTTPStatus = http.StatusForbidden
		out.Message = "This ChatGPT account is not eligible for token sharing."
	case "subscription_sharing_unsupported_capability":
		out.Action = ActionNone
		out.HTTPStatus = http.StatusBadRequest
		out.Message = "The request used a capability this preview does not support."
	case "subscription_sharing_route_not_supported":
		out.Action = ActionNone
		out.HTTPStatus = http.StatusForbidden
		out.Message = "This route is not supported for Sign in with ChatGPT token sharing."
	default:
		out.Action = ActionNone
		if out.HTTPStatus == 0 {
			out.HTTPStatus = http.StatusBadGateway
		}
	}
	return out
}

// KnownCode reports whether code is one of the documented sharing errors.
func KnownCode(code string) bool {
	switch code {
	case "subscription_sharing_usage_limit_exceeded",
		"subscription_sharing_usage_unavailable",
		"subscription_sharing_user_unavailable",
		"subscription_sharing_invalid_user",
		"subscription_sharing_user_not_eligible",
		"subscription_sharing_unsupported_capability",
		"subscription_sharing_route_not_supported",
		"chatpass_v2_scope_not_authorized",
		"chatpass_v2_invalid_authorization_context":
		return true
	default:
		return false
	}
}
