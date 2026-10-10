package siwc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Discovery is the subset of the OpenID configuration this flow uses.
type Discovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
	RevocationEndpoint    string `json:"revocation_endpoint"`
}

// FetchDiscovery loads /.well-known/openid-configuration and checks that every
// endpoint lives on the issuer origin.
func FetchDiscovery(ctx context.Context, client *http.Client, issuer string) (*Discovery, error) {
	if client == nil {
		client = http.DefaultClient
	}
	issuer = strings.TrimRight(strings.TrimSpace(issuer), "/")
	issuerURL, err := url.Parse(issuer)
	if err != nil || issuerURL.Scheme == "" || issuerURL.Host == "" {
		return nil, fmt.Errorf("siwc discovery: invalid issuer")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer+"/.well-known/openid-configuration", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("siwc discovery: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("siwc discovery: HTTP %d", resp.StatusCode)
	}
	var doc Discovery
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("siwc discovery: %w", err)
	}
	if strings.TrimRight(doc.Issuer, "/") != issuer {
		return nil, fmt.Errorf("siwc discovery: issuer mismatch")
	}
	for _, endpoint := range []string{doc.AuthorizationEndpoint, doc.TokenEndpoint, doc.JWKSURI, doc.RevocationEndpoint} {
		if err := endpointOnOrigin(issuerURL, endpoint); err != nil {
			return nil, err
		}
	}
	return &doc, nil
}

func endpointOnOrigin(issuer *url.URL, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("siwc discovery: missing endpoint")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("siwc discovery: bad endpoint: %w", err)
	}
	if !strings.EqualFold(parsed.Scheme, issuer.Scheme) || !strings.EqualFold(parsed.Host, issuer.Host) {
		return fmt.Errorf("siwc discovery: endpoint %s is not on issuer origin", raw)
	}
	return nil
}
