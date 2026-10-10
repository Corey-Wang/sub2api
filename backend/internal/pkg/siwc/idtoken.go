package siwc

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// IDClaims are the verified identity claims this flow stores.
type IDClaims struct {
	Subject string
	Email   string
	Nonce   string
}

// VerifyIDToken checks signature (JWKS), iss, aud, exp, and nonce.
func VerifyIDToken(ctx context.Context, client *http.Client, jwksURI, issuer, rawToken, audience, nonce string) (*IDClaims, error) {
	if client == nil {
		client = http.DefaultClient
	}
	parser := jwt.NewParser(
		jwt.WithValidMethods(allowedAlgList()),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
	)
	claims := jwt.MapClaims{}
	token, err := parser.ParseWithClaims(rawToken, claims, func(token *jwt.Token) (any, error) {
		alg, _ := token.Header["alg"].(string)
		if _, ok := AllowedIDTokenAlgs[alg]; !ok {
			return nil, fmt.Errorf("unsupported alg %s", alg)
		}
		kid, _ := token.Header["kid"].(string)
		return lookupJWK(ctx, client, jwksURI, kid, alg)
	})
	if err != nil || token == nil || !token.Valid {
		return nil, &OAuthError{Code: "invalid_id_token", Description: "ID token validation failed"}
	}
	gotNonce, _ := claims["nonce"].(string)
	if nonce != "" && !StateMatches(nonce, gotNonce) {
		return nil, &OAuthError{Code: "invalid_id_token", Description: "ID token nonce mismatch"}
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, &OAuthError{Code: "invalid_id_token", Description: "ID token has no subject"}
	}
	email, _ := claims["email"].(string)
	return &IDClaims{Subject: sub, Email: email, Nonce: gotNonce}, nil
}

func allowedAlgList() []string {
	out := make([]string, 0, len(AllowedIDTokenAlgs))
	for alg := range AllowedIDTokenAlgs {
		out = append(out, alg)
	}
	return out
}

func lookupJWK(ctx context.Context, client *http.Client, jwksURI, kid, alg string) (any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, &OAuthError{Code: "identity_verification_unavailable", Description: "could not fetch OpenAI JWKS"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &OAuthError{Code: "identity_verification_unavailable", Description: "could not fetch OpenAI JWKS"}
	}
	var doc struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	for _, key := range doc.Keys {
		if kid != "" {
			if got, _ := key["kid"].(string); got != kid {
				continue
			}
		}
		pub, err := publicKeyFromJWK(key, alg)
		if err != nil {
			continue
		}
		return pub, nil
	}
	return nil, &OAuthError{Code: "invalid_id_token", Description: "ID token key not found in JWKS"}
}

func publicKeyFromJWK(key map[string]any, alg string) (any, error) {
	kty, _ := key["kty"].(string)
	switch kty {
	case "RSA":
		if !strings.HasPrefix(alg, "RS") && !strings.HasPrefix(alg, "PS") {
			return nil, fmt.Errorf("alg/kty mismatch")
		}
		nBytes, err := decodeB64(stringField(key, "n"))
		if err != nil {
			return nil, err
		}
		eBytes, err := decodeB64(stringField(key, "e"))
		if err != nil {
			return nil, err
		}
		e := 0
		for _, b := range eBytes {
			e = e<<8 | int(b)
		}
		if e == 0 {
			return nil, fmt.Errorf("bad rsa exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
	case "EC":
		if !strings.HasPrefix(alg, "ES") {
			return nil, fmt.Errorf("alg/kty mismatch")
		}
		curve := elliptic.P256()
		if crv, _ := key["crv"].(string); crv == "P-384" {
			curve = elliptic.P384()
		}
		xBytes, err := decodeB64(stringField(key, "x"))
		if err != nil {
			return nil, err
		}
		yBytes, err := decodeB64(stringField(key, "y"))
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(xBytes), Y: new(big.Int).SetBytes(yBytes)}, nil
	default:
		return nil, fmt.Errorf("unsupported kty")
	}
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func decodeB64(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}
