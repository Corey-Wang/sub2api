// Package accountimport implements the versioned encrypted account bundle
// used to hand ChatGPT/OpenAI OAuth accounts from a seller to an on-prem
// sub2api instance without exposing plaintext secrets to site staff.
package accountimport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// Kind identifies an encrypted account bundle.
	Kind = "sub2api.encrypted_account_bundle"
	// Version is the current envelope version.
	Version = 1
	// Algorithm is X25519 ECDH + HKDF-SHA256 + XChaCha20-Poly1305.
	Algorithm = "x25519-hkdf-sha256-xchacha20poly1305"
	// HKDFInfo binds derived keys to this protocol.
	HKDFInfo = "sub2api-account-import-v1"
	// ImportSource is stored on imported accounts (safe metadata).
	ImportSource = "encrypted_bundle"
	// ExtraImportIDKey is the idempotency key stored in account extra.
	ExtraImportIDKey = "encrypted_import_id"
	// ExtraImportSourceKey marks how the account was created.
	ExtraImportSourceKey = "import_source"
	// ExtraImportedAtKey is an RFC3339 timestamp.
	ExtraImportedAtKey = "imported_at"
	// ExtraImportKIDKey records which recipient key accepted the item.
	ExtraImportKIDKey = "encrypted_import_kid"
)

// Bundle is a versioned file the seller hands to the customer.
type Bundle struct {
	Kind         string `json:"kind"`
	Version      int    `json:"version"`
	Alg          string `json:"alg"`
	RecipientKID string `json:"recipient_kid"`
	CreatedAt    string `json:"created_at,omitempty"`
	Items        []Item `json:"items"`
}

// Item is one encrypted account record. Items are independently encrypted so
// a single tampered row can fail without discarding the rest of the batch.
type Item struct {
	ID                 string `json:"id,omitempty"`
	EphemeralPublicKey string `json:"ephemeral_public_key"`
	Nonce              string `json:"nonce"`
	Ciphertext         string `json:"ciphertext"`
}

// PlaintextAccount is the versioned payload sealed inside each Item.
// Credentials follow the same OpenAI/Codex OAuth shape the gateway already uses.
type PlaintextAccount struct {
	Version     int            `json:"version"`
	ID          string         `json:"id,omitempty"`
	Platform    string         `json:"platform,omitempty"`
	Type        string         `json:"type,omitempty"`
	Name        string         `json:"name,omitempty"`
	Notes       *string        `json:"notes,omitempty"`
	Credentials map[string]any `json:"credentials,omitempty"`
	Session     map[string]any `json:"session,omitempty"`
}

// PublicKeyInfo is the safe metadata an admin instance may publish.
type PublicKeyInfo struct {
	Configured bool   `json:"configured"`
	Kind       string `json:"kind"`
	Version    int    `json:"version"`
	Alg        string `json:"alg"`
	KID        string `json:"kid,omitempty"`
	PublicKey  string `json:"public_key,omitempty"`
}

var (
	ErrNotConfigured     = errors.New("account import key is not configured")
	ErrInvalidKey        = errors.New("account import key is invalid")
	ErrUnsupportedBundle = errors.New("unsupported encrypted account bundle")
	ErrDecryptFailed     = errors.New("encrypted account item could not be decrypted")
	ErrUnsupportedRecord = errors.New("decrypted account record is not a supported OpenAI/ChatGPT OAuth account")
)

// NewBundle builds an empty v1 bundle addressed to kid.
func NewBundle(kid string) Bundle {
	return Bundle{
		Kind:         Kind,
		Version:      Version,
		Alg:          Algorithm,
		RecipientKID: strings.TrimSpace(kid),
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
		Items:        []Item{},
	}
}

func (b Bundle) ValidateHeader() error {
	if strings.TrimSpace(b.Kind) != Kind {
		return fmt.Errorf("%w: kind %q", ErrUnsupportedBundle, b.Kind)
	}
	if b.Version != Version {
		return fmt.Errorf("%w: version %d", ErrUnsupportedBundle, b.Version)
	}
	if strings.TrimSpace(b.Alg) != Algorithm {
		return fmt.Errorf("%w: alg %q", ErrUnsupportedBundle, b.Alg)
	}
	if strings.TrimSpace(b.RecipientKID) == "" {
		return fmt.Errorf("%w: missing recipient_kid", ErrUnsupportedBundle)
	}
	if len(b.Items) == 0 {
		return fmt.Errorf("%w: bundle has no items", ErrUnsupportedBundle)
	}
	return nil
}

func (a PlaintextAccount) Validate() error {
	platform := strings.ToLower(strings.TrimSpace(a.Platform))
	switch platform {
	case "", "openai", "chatgpt", "codex":
	default:
		return fmt.Errorf("%w: platform %q", ErrUnsupportedRecord, a.Platform)
	}
	accountType := strings.ToLower(strings.TrimSpace(a.Type))
	switch accountType {
	case "", "oauth":
	default:
		return fmt.Errorf("%w: type %q", ErrUnsupportedRecord, a.Type)
	}
	if a.Version != 0 && a.Version != Version {
		return fmt.Errorf("%w: record version %d", ErrUnsupportedRecord, a.Version)
	}
	if len(a.Credentials) == 0 && len(a.Session) == 0 {
		return fmt.Errorf("%w: missing credentials", ErrUnsupportedRecord)
	}
	return nil
}

// CodexValue returns a map the existing Codex session importer can consume.
func (a PlaintextAccount) CodexValue() map[string]any {
	var out map[string]any
	if len(a.Session) > 0 {
		out = cloneMap(a.Session)
	} else {
		out = cloneMap(a.Credentials)
	}
	if a.Name != "" {
		if _, ok := out["name"]; !ok {
			out["name"] = a.Name
		}
	}
	out[EncryptedImportMetaKey] = map[string]any{
		"id":   strings.TrimSpace(a.ID),
		"name": strings.TrimSpace(a.Name),
	}
	return out
}

// EncryptedImportMetaKey is stripped before persistence; it only travels
// from decrypt → Codex normalize on the server.
const EncryptedImportMetaKey = "_sub2api_encrypted_import"

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func marshalCanonical(v any) ([]byte, error) {
	return json.Marshal(v)
}
