package siwc

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
)

// GeneratePKCE returns a fresh verifier and its S256 challenge (base64url, no padding).
func GeneratePKCE() (verifier, challenge string, err error) {
	raw := make([]byte, 32)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("siwc pkce: %w", err)
	}
	verifier = base64.RawURLEncoding.EncodeToString(raw)
	challenge = CodeChallengeS256(verifier)
	return verifier, challenge, nil
}

// CodeChallengeS256 is the RFC 7636 S256 transform.
func CodeChallengeS256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// RandomToken returns a fresh base64url token used for OAuth state and OIDC nonce.
func RandomToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("siwc random token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// StateMatches compares OAuth state values in constant time.
func StateMatches(expected, got string) bool {
	if expected == "" || got == "" {
		return false
	}
	if len(expected) != len(got) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(got)) == 1
}
