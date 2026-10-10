package siwc

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var redirectURIPattern = regexp.MustCompile(`^http://127\.0\.0\.1:(\d+)/auth/callback$`)

// ValidateRedirectURI accepts only the documented loopback callback.
func ValidateRedirectURI(raw string) error {
	raw = strings.TrimSpace(raw)
	m := redirectURIPattern.FindStringSubmatch(raw)
	if m == nil {
		return errRedirectURI
	}
	port, err := strconv.Atoi(m[1])
	if err != nil || port < 1 || port > 65535 {
		return errRedirectURI
	}
	return nil
}

var errRedirectURI = &OAuthError{Code: "invalid_redirect_uri", Description: "redirect_uri must be http://127.0.0.1:<port>/auth/callback"}

// AuthorizeParams builds the authorization URL.
// savedClientID empty means first-time registration (dynamic_agent_client + agent_name_hint).
// A saved oaiapp_ client id is reused and agent_name_hint is omitted.
// id_token_hint is intentionally not set: the URL is copied by the admin.
func AuthorizeParams(authorizationEndpoint string, savedClientID, redirectURI, state, nonce, challenge, hostID, loginHint string, reconsent bool) (string, error) {
	if err := ValidateRedirectURI(redirectURI); err != nil {
		return "", err
	}
	if strings.TrimSpace(authorizationEndpoint) == "" || state == "" || nonce == "" || challenge == "" || strings.TrimSpace(hostID) == "" {
		return "", &OAuthError{Code: "invalid_request", Description: "missing authorize parameter"}
	}
	if strings.Contains(hostID, "@") {
		return "", &OAuthError{Code: "invalid_request", Description: "ext_agent_host_id must not contain an email"}
	}
	values := url.Values{}
	clientID := strings.TrimSpace(savedClientID)
	if clientID == "" {
		values.Set("client_id", DynamicClient)
		values.Set("agent_name_hint", AgentNameHint)
	} else {
		if clientID == DynamicClient || !clientIDPattern.MatchString(clientID) {
			return "", &OAuthError{Code: "invalid_client", Description: "saved client_id is not an issued client"}
		}
		values.Set("client_id", clientID)
	}
	values.Set("response_type", "code")
	values.Set("redirect_uri", redirectURI)
	values.Set("scope", Scopes)
	values.Set("resource", Resource)
	values.Set("state", state)
	values.Set("nonce", nonce)
	values.Set("code_challenge_method", "S256")
	values.Set("code_challenge", challenge)
	values.Set("ext_agent_host_id", hostID)
	if hint := strings.TrimSpace(loginHint); hint != "" {
		values.Set("login_hint", hint)
	}
	if reconsent {
		// prompt=consent remains supported until OpenAI confirms force_reconsent.
		values.Set("prompt", "consent")
	}
	return strings.TrimRight(authorizationEndpoint, "?") + "?" + values.Encode(), nil
}
