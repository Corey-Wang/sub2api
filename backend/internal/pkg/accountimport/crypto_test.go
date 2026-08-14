package accountimport

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func sampleRecord(id, refresh string) PlaintextAccount {
	return PlaintextAccount{
		Version:  Version,
		ID:       id,
		Platform: "openai",
		Type:     "oauth",
		Name:     "desk-" + id,
		Credentials: map[string]any{
			"access_token":       "at-" + id,
			"refresh_token":      refresh,
			"email":              id + "@seller.example",
			"chatgpt_account_id": "acct-" + id,
			"chatgpt_user_id":    "user-" + id,
		},
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	record := sampleRecord("a1", "rt-secret-a1")
	bundle, err := EncryptBundle(pub, []PlaintextAccount{record})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	if err := bundle.ValidateHeader(); err != nil {
		t.Fatalf("ValidateHeader: %v", err)
	}
	got, err := priv.Decrypt(bundle.Items[0])
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got.Credentials["refresh_token"] != "rt-secret-a1" {
		t.Fatalf("refresh_token = %v", got.Credentials["refresh_token"])
	}
	if got.ID != "a1" {
		t.Fatalf("id = %q", got.ID)
	}
}

func TestDecryptRejectsWrongKey(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	other, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair other: %v", err)
	}
	bundle, err := EncryptBundle(pub, []PlaintextAccount{sampleRecord("a1", "rt-secret")})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	_, err = other.Decrypt(bundle.Items[0])
	if err == nil {
		t.Fatal("Decrypt with wrong key succeeded")
	}
	if err != ErrDecryptFailed {
		t.Fatalf("err = %v, want ErrDecryptFailed", err)
	}
}

func TestDecryptRejectsTamperedCiphertext(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	bundle, err := EncryptBundle(pub, []PlaintextAccount{sampleRecord("a1", "rt-secret")})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	item := bundle.Items[0]
	raw, err := decodeBytes(item.Ciphertext)
	if err != nil {
		t.Fatalf("decode ciphertext: %v", err)
	}
	raw[len(raw)-1] ^= 0x01
	item.Ciphertext = base64.RawURLEncoding.EncodeToString(raw)
	_, err = priv.Decrypt(item)
	if err != ErrDecryptFailed {
		t.Fatalf("err = %v, want ErrDecryptFailed", err)
	}
}

func TestDecryptRejectsTamperedItemID(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	bundle, err := EncryptBundle(pub, []PlaintextAccount{sampleRecord("a1", "rt-secret")})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	item := bundle.Items[0]
	item.ID = "tampered"
	_, err = priv.Decrypt(item)
	if err != ErrDecryptFailed {
		t.Fatalf("err = %v, want ErrDecryptFailed", err)
	}
}

func TestParseImportDocumentsBundleAndArray(t *testing.T) {
	_, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	bundle, err := EncryptBundle(pub, []PlaintextAccount{
		sampleRecord("a1", "rt-1"),
		sampleRecord("a2", "rt-2"),
	})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	items, err := ParseImportDocuments(string(raw))
	if err != nil {
		t.Fatalf("ParseImportDocuments: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}

	arrayRaw, err := json.Marshal(bundle.Items)
	if err != nil {
		t.Fatalf("marshal items: %v", err)
	}
	items, err = ParseImportDocuments(string(arrayRaw))
	if err != nil {
		t.Fatalf("ParseImportDocuments array: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("array len(items) = %d, want 2", len(items))
	}
}

func TestNormalizeSellerRecordsCodexSession(t *testing.T) {
	content := `{
		"id": "desk-9",
		"name": "desk-9",
		"access_token": "at-9",
		"refresh_token": "rt-9",
		"chatgpt_account_id": "acct-9"
	}`
	records, err := NormalizeSellerRecords(content)
	if err != nil {
		t.Fatalf("NormalizeSellerRecords: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("len = %d", len(records))
	}
	if records[0].Credentials["refresh_token"] != "rt-9" {
		t.Fatalf("credentials = %#v", records[0].Credentials)
	}
}

func TestPublicKeyInfoOmitsPrivateMaterial(t *testing.T) {
	priv, pub, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	info := pub.Info()
	raw, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), priv.Encode()) {
		t.Fatal("public key info leaked private key")
	}
	if !info.Configured || info.PublicKey == "" || info.KID == "" {
		t.Fatalf("info = %+v", info)
	}
}
