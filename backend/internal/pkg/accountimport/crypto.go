package accountimport

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

const (
	x25519KeySize = 32
	nonceSize     = chacha20poly1305.NonceSizeX
)

// PrivateKey is the instance import private key. Only the server process
// should hold this value (env / config).
type PrivateKey struct {
	raw [x25519KeySize]byte
}

// PublicKey is the X25519 public key sellers encrypt to.
type PublicKey struct {
	raw [x25519KeySize]byte
}

func GenerateKeyPair() (*PrivateKey, *PublicKey, error) {
	var priv PrivateKey
	if _, err := io.ReadFull(rand.Reader, priv.raw[:]); err != nil {
		return nil, nil, fmt.Errorf("generate import private key: %w", err)
	}
	clampX25519(&priv.raw)
	pub, err := priv.Public()
	if err != nil {
		return nil, nil, err
	}
	return &priv, pub, nil
}

func ParsePrivateKey(encoded string) (*PrivateKey, error) {
	raw, err := decodeKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	var key PrivateKey
	copy(key.raw[:], raw)
	clampX25519(&key.raw)
	return &key, nil
}

func ParseOptionalPrivateKey(encoded string) (*PrivateKey, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	return ParsePrivateKey(encoded)
}

func ParsePublicKey(encoded string) (*PublicKey, error) {
	raw, err := decodeKey(encoded)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}
	var key PublicKey
	copy(key.raw[:], raw)
	return &key, nil
}

func (k *PrivateKey) Public() (*PublicKey, error) {
	if k == nil {
		return nil, ErrNotConfigured
	}
	var pub PublicKey
	curve25519.ScalarBaseMult(&pub.raw, &k.raw)
	return &pub, nil
}

func (k *PrivateKey) Encode() string {
	if k == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(k.raw[:])
}

func (k *PublicKey) Encode() string {
	if k == nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(k.raw[:])
}

func (k *PublicKey) KID() string {
	if k == nil {
		return ""
	}
	sum := sha256.Sum256(k.raw[:])
	return "sha256:" + hex.EncodeToString(sum[:])
}

func (k *PublicKey) Info() PublicKeyInfo {
	if k == nil {
		return PublicKeyInfo{
			Configured: false,
			Kind:       Kind,
			Version:    Version,
			Alg:        Algorithm,
		}
	}
	return PublicKeyInfo{
		Configured: true,
		Kind:       Kind,
		Version:    Version,
		Alg:        Algorithm,
		KID:        k.KID(),
		PublicKey:  k.Encode(),
	}
}

func EncryptItem(recipient *PublicKey, record PlaintextAccount) (Item, error) {
	if recipient == nil {
		return Item{}, ErrInvalidKey
	}
	if err := record.Validate(); err != nil {
		return Item{}, err
	}
	plaintext, err := marshalCanonical(record)
	if err != nil {
		return Item{}, fmt.Errorf("marshal account record: %w", err)
	}

	var ephPriv [x25519KeySize]byte
	if _, err := io.ReadFull(rand.Reader, ephPriv[:]); err != nil {
		return Item{}, fmt.Errorf("generate ephemeral key: %w", err)
	}
	clampX25519(&ephPriv)
	var ephPub [x25519KeySize]byte
	curve25519.ScalarBaseMult(&ephPub, &ephPriv)

	aeadKey, err := deriveItemKey(ephPriv[:], recipient.raw[:], ephPub[:], recipient.raw[:])
	if err != nil {
		return Item{}, err
	}
	aead, err := chacha20poly1305.NewX(aeadKey)
	if err != nil {
		return Item{}, err
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Item{}, fmt.Errorf("generate nonce: %w", err)
	}
	aad := itemAAD(recipient.KID(), record.ID)
	ciphertext := aead.Seal(nil, nonce, plaintext, aad)
	return Item{
		ID:                 strings.TrimSpace(record.ID),
		EphemeralPublicKey: base64.RawURLEncoding.EncodeToString(ephPub[:]),
		Nonce:              base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext:         base64.RawURLEncoding.EncodeToString(ciphertext),
	}, nil
}

func (k *PrivateKey) Decrypt(item Item) (PlaintextAccount, error) {
	var empty PlaintextAccount
	if k == nil {
		return empty, ErrNotConfigured
	}
	pub, err := k.Public()
	if err != nil {
		return empty, err
	}
	ephPub, err := decodeExact(item.EphemeralPublicKey, x25519KeySize)
	if err != nil {
		return empty, ErrDecryptFailed
	}
	nonce, err := decodeExact(item.Nonce, nonceSize)
	if err != nil {
		return empty, ErrDecryptFailed
	}
	ciphertext, err := decodeBytes(item.Ciphertext)
	if err != nil || len(ciphertext) < chacha20poly1305.Overhead {
		return empty, ErrDecryptFailed
	}
	aeadKey, err := deriveItemKey(k.raw[:], ephPub, ephPub, pub.raw[:])
	if err != nil {
		return empty, ErrDecryptFailed
	}
	aead, err := chacha20poly1305.NewX(aeadKey)
	if err != nil {
		return empty, ErrDecryptFailed
	}
	aad := itemAAD(pub.KID(), item.ID)
	plaintext, err := aead.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return empty, ErrDecryptFailed
	}
	var record PlaintextAccount
	if err := unmarshalStrict(plaintext, &record); err != nil {
		return empty, ErrDecryptFailed
	}
	if err := record.Validate(); err != nil {
		return empty, err
	}
	if record.ID == "" {
		record.ID = strings.TrimSpace(item.ID)
	}
	return record, nil
}

func EncryptBundle(recipient *PublicKey, records []PlaintextAccount) (Bundle, error) {
	if recipient == nil {
		return Bundle{}, ErrInvalidKey
	}
	bundle := NewBundle(recipient.KID())
	for i, record := range records {
		if strings.TrimSpace(record.ID) == "" {
			record.ID = fmt.Sprintf("item-%d", i+1)
		}
		if record.Version == 0 {
			record.Version = Version
		}
		item, err := EncryptItem(recipient, record)
		if err != nil {
			return Bundle{}, fmt.Errorf("encrypt item %d: %w", i+1, err)
		}
		bundle.Items = append(bundle.Items, item)
	}
	return bundle, nil
}

func deriveItemKey(localPriv, remotePub, ephPub, recipientPub []byte) ([]byte, error) {
	shared, err := curve25519.X25519(localPriv, remotePub)
	if err != nil {
		return nil, err
	}
	salt := append(append([]byte{}, recipientPub...), ephPub...)
	r := hkdf.New(sha256.New, shared, salt, []byte(HKDFInfo))
	key := make([]byte, chacha20poly1305.KeySize)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, err
	}
	return key, nil
}

func itemAAD(kid, id string) []byte {
	return []byte("v1|" + strings.TrimSpace(kid) + "|" + strings.TrimSpace(id))
}

func clampX25519(key *[x25519KeySize]byte) {
	key[0] &= 248
	key[31] &= 127
	key[31] |= 64
}

func decodeKey(encoded string) ([]byte, error) {
	raw, err := decodeBytes(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) != x25519KeySize {
		return nil, fmt.Errorf("key must be %d bytes, got %d", x25519KeySize, len(raw))
	}
	return raw, nil
}

func decodeExact(encoded string, size int) ([]byte, error) {
	raw, err := decodeBytes(encoded)
	if err != nil {
		return nil, err
	}
	if len(raw) != size {
		return nil, fmt.Errorf("expected %d bytes, got %d", size, len(raw))
	}
	return raw, nil
}

func decodeBytes(encoded string) ([]byte, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, fmt.Errorf("empty encoding")
	}
	if raw, err := base64.RawURLEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	if raw, err := base64.URLEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	if raw, err := base64.StdEncoding.DecodeString(encoded); err == nil {
		return raw, nil
	}
	if raw, err := hex.DecodeString(encoded); err == nil {
		return raw, nil
	}
	return nil, fmt.Errorf("invalid base64/hex")
}
