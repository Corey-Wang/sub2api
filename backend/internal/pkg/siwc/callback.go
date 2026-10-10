package siwc

import (
	"net/url"
	"regexp"
	"strings"
)

// Issued client ids are oaiapp_… values. dynamic_agent_client is the registration
// entrypoint and must never be stored or used at the token endpoint.
var clientIDPattern = regexp.MustCompile(`^oaiapp_[A-Za-z0-9_-]{1,180}$`)

// Callback is a parsed loopback redirect.
type Callback struct {
	Code        string
	State       string
	ClientID    string
	Error       string
	ErrorDesc   string
	HadClientID bool
}

// ParseCallbackURL reads the full callback URL the admin pastes back.
// savedClientID is empty on first login and the previously issued id on reauth.
func ParseCallbackURL(rawURL, expectedState, savedClientID string) (*Callback, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, &OAuthError{Code: "invalid_callback", Description: "callback URL is not a valid URL"}
	}
	if err := ValidateRedirectURI(parsed.Scheme + "://" + parsed.Host + parsed.Path); err != nil {
		return nil, err
	}
	q := parsed.Query()
	gotState := firstQuery(q, "state")
	if !StateMatches(expectedState, gotState) {
		return nil, &OAuthError{Code: "state_mismatch", Description: "OAuth state does not match this login attempt"}
	}
	cb := &Callback{State: gotState}
	if oauthErr := firstQuery(q, "error"); oauthErr != "" {
		cb.Error = oauthErr
		cb.ErrorDesc = firstQuery(q, "error_description")
		return cb, &OAuthError{Code: oauthErr, Description: cb.ErrorDesc}
	}
	cb.Code = firstQuery(q, "code")
	if cb.Code == "" {
		return nil, &OAuthError{Code: "invalid_callback", Description: "callback is missing code"}
	}
	returned := q["client_id"]
	cb.HadClientID = len(returned) > 0 && strings.TrimSpace(returned[0]) != ""
	if len(returned) > 1 {
		return nil, &OAuthError{Code: "invalid_client", Description: "callback returned multiple client_id values"}
	}
	saved := strings.TrimSpace(savedClientID)
	switch {
	case cb.HadClientID:
		cb.ClientID = strings.TrimSpace(returned[0])
	case saved != "":
		cb.ClientID = saved
	default:
		return nil, &OAuthError{Code: "missing_client_id", Description: "first login callback did not return an issued client_id"}
	}
	if cb.ClientID == DynamicClient {
		return nil, &OAuthError{Code: "invalid_client", Description: "dynamic_agent_client cannot be used as the issued client_id"}
	}
	if !clientIDPattern.MatchString(cb.ClientID) {
		return nil, &OAuthError{Code: "invalid_client", Description: "callback client_id is not an issued oaiapp_ client"}
	}
	if saved != "" && cb.HadClientID && cb.ClientID != saved {
		return nil, &OAuthError{Code: "client_id_mismatch", Description: "callback client_id does not match the saved client"}
	}
	return cb, nil
}

func firstQuery(q url.Values, key string) string {
	values := q[key]
	if len(values) == 0 {
		return ""
	}
	return strings.TrimSpace(values[0])
}
