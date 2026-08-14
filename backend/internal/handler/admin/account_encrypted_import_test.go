package admin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/pkg/accountimport"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func TestImportEncryptedAccountsDecryptSuccessAndHidesSecretsInAPIJSON(t *testing.T) {
	priv, pub, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	refresh := "rt-secret-must-not-leak"
	email := "hidden-user@seller.example"
	record := accountimport.PlaintextAccount{
		Version:  accountimport.Version,
		ID:       "desk-1",
		Platform: "openai",
		Type:     "oauth",
		Name:     "门店A-1",
		Credentials: map[string]any{
			"access_token":       buildCodexAccessToken(t, "acct-1", "user-1", time.Now().Add(time.Hour)),
			"refresh_token":      refresh,
			"email":              email,
			"chatgpt_account_id": "acct-1",
			"chatgpt_user_id":    "user-1",
		},
	}
	bundle, err := accountimport.EncryptBundle(pub, []accountimport.PlaintextAccount{record})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}

	svc := newCodexImportMemoryAdminService(nil)
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetAccountImportKey(priv)
	skip := true
	result, err := handler.importEncryptedAccounts(context.Background(), EncryptedAccountImportRequest{
		SkipDefaultGroupBind: &skip,
	}, bundle.Items)
	if err != nil {
		t.Fatalf("importEncryptedAccounts: %v", err)
	}
	if result.Created != 1 || result.Failed != 0 {
		t.Fatalf("result = %+v, want 1 created", result)
	}
	if len(svc.createdAccounts) != 1 {
		t.Fatalf("created = %d", len(svc.createdAccounts))
	}
	created := svc.createdAccounts[0]
	if created.Credentials["refresh_token"] != refresh {
		t.Fatalf("gateway credentials missing refresh_token")
	}
	if created.Extra[accountimport.ExtraImportSourceKey] != accountimport.ImportSource {
		t.Fatalf("extra = %#v", created.Extra)
	}
	if created.Extra[accountimport.ExtraImportIDKey] != "desk-1" {
		t.Fatalf("encrypted_import_id = %v", created.Extra[accountimport.ExtraImportIDKey])
	}
	if created.Name != "门店A-1" {
		t.Fatalf("name = %q", created.Name)
	}

	stored, err := svc.GetAccount(context.Background(), result.Items[len(result.Items)-1].AccountID)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	apiAccount := dto.AccountFromServiceShallow(stored)
	raw, err := json.Marshal(apiAccount)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	for _, secret := range []string{refresh, email, "at-"} {
		if strings.Contains(body, secret) && secret != "at-" {
			t.Fatalf("API JSON leaked %q: %s", secret, body)
		}
	}
	if strings.Contains(body, refresh) || strings.Contains(body, email) {
		t.Fatalf("API JSON leaked secrets: %s", body)
	}
	if apiAccount.Credentials != nil {
		if _, ok := apiAccount.Credentials["refresh_token"]; ok {
			t.Fatal("refresh_token present in API credentials")
		}
		if _, ok := apiAccount.Credentials["email"]; ok {
			t.Fatal("email present in API credentials")
		}
	}
}

func TestImportEncryptedAccountsDecryptFailureWrongKeyAndTamper(t *testing.T) {
	_, pub, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	other, _, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair other: %v", err)
	}
	record := accountimport.PlaintextAccount{
		Version:  accountimport.Version,
		ID:       "desk-bad",
		Platform: "openai",
		Type:     "oauth",
		Credentials: map[string]any{
			"access_token":  buildCodexAccessToken(t, "acct-x", "user-x", time.Now().Add(time.Hour)),
			"refresh_token": "rt-secret",
		},
	}
	bundle, err := accountimport.EncryptBundle(pub, []accountimport.PlaintextAccount{record})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}

	svc := newCodexImportMemoryAdminService(nil)
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetAccountImportKey(other)
	skip := true
	result, err := handler.importEncryptedAccounts(context.Background(), EncryptedAccountImportRequest{
		SkipDefaultGroupBind: &skip,
	}, bundle.Items)
	if err != nil {
		t.Fatalf("importEncryptedAccounts: %v", err)
	}
	if result.Failed != 1 || result.Created != 0 {
		t.Fatalf("wrong-key result = %+v", result)
	}
	if !strings.Contains(result.Items[0].Message, "解密失败") {
		t.Fatalf("message = %q", result.Items[0].Message)
	}
	if len(svc.createdAccounts) != 0 {
		t.Fatal("wrong key must not create accounts")
	}

	priv, pub2, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	bundle, err = accountimport.EncryptBundle(pub2, []accountimport.PlaintextAccount{record})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	bundle.Items[0].Ciphertext = bundle.Items[0].Ciphertext + "AA"
	handler.SetAccountImportKey(priv)
	result, err = handler.importEncryptedAccounts(context.Background(), EncryptedAccountImportRequest{
		SkipDefaultGroupBind: &skip,
	}, bundle.Items)
	if err != nil {
		t.Fatalf("tamper import: %v", err)
	}
	if result.Failed != 1 {
		t.Fatalf("tamper result = %+v", result)
	}
}

func TestImportEncryptedAccountsBatchPartialFailure(t *testing.T) {
	priv, pub, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	good := accountimport.PlaintextAccount{
		Version:  accountimport.Version,
		ID:       "good-1",
		Platform: "openai",
		Type:     "oauth",
		Name:     "good-desk",
		Credentials: map[string]any{
			"access_token":  buildCodexAccessToken(t, "acct-g", "user-g", time.Now().Add(time.Hour)),
			"refresh_token": "rt-good",
		},
	}
	bundle, err := accountimport.EncryptBundle(pub, []accountimport.PlaintextAccount{good})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}
	bad := bundle.Items[0]
	bad.ID = "tampered-id"
	items := []accountimport.Item{bundle.Items[0], bad}

	svc := newCodexImportMemoryAdminService(nil)
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetAccountImportKey(priv)
	skip := true
	result, err := handler.importEncryptedAccounts(context.Background(), EncryptedAccountImportRequest{
		SkipDefaultGroupBind: &skip,
	}, items)
	if err != nil {
		t.Fatalf("importEncryptedAccounts: %v", err)
	}
	if result.Created != 1 || result.Failed != 1 {
		t.Fatalf("partial result = %+v", result)
	}
	if len(svc.createdAccounts) != 1 {
		t.Fatalf("created = %d, want 1", len(svc.createdAccounts))
	}
}

func TestImportEncryptedAccountsIdempotentByImportID(t *testing.T) {
	priv, pub, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	token := buildCodexAccessToken(t, "acct-1", "user-1", time.Now().Add(time.Hour))
	record := accountimport.PlaintextAccount{
		Version:  accountimport.Version,
		ID:       "stable-desk",
		Platform: "openai",
		Type:     "oauth",
		Name:     "stable",
		Credentials: map[string]any{
			"access_token":       token,
			"refresh_token":      "rt-1",
			"chatgpt_account_id": "acct-1",
			"chatgpt_user_id":    "user-1",
		},
	}
	bundle, err := accountimport.EncryptBundle(pub, []accountimport.PlaintextAccount{record})
	if err != nil {
		t.Fatalf("EncryptBundle: %v", err)
	}

	existing := service.Account{
		ID:       77,
		Name:     "stable",
		Platform: service.PlatformOpenAI,
		Type:     service.AccountTypeOAuth,
		Credentials: map[string]any{
			"access_token":       token,
			"refresh_token":      "rt-old",
			"chatgpt_account_id": "acct-1",
			"chatgpt_user_id":    "user-1",
		},
		Extra: map[string]any{
			accountimport.ExtraImportSourceKey: accountimport.ImportSource,
			accountimport.ExtraImportIDKey:     "stable-desk",
		},
	}
	svc := newCodexImportMemoryAdminService([]service.Account{existing})
	handler := NewAccountHandler(svc, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	handler.SetAccountImportKey(priv)
	skip := true
	result, err := handler.importEncryptedAccounts(context.Background(), EncryptedAccountImportRequest{
		SkipDefaultGroupBind: &skip,
	}, bundle.Items)
	if err != nil {
		t.Fatalf("importEncryptedAccounts: %v", err)
	}
	if result.Created != 0 || result.Updated != 1 {
		t.Fatalf("idempotent result = %+v, want updated", result)
	}
	if len(svc.createdAccounts) != 0 {
		t.Fatalf("created duplicates: %d", len(svc.createdAccounts))
	}
}

func TestGetEncryptedImportPublicKeyNeverIncludesPrivateKey(t *testing.T) {
	priv, _, err := accountimport.GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair: %v", err)
	}
	pub, err := priv.Public()
	if err != nil {
		t.Fatalf("Public: %v", err)
	}
	info := pub.Info()
	raw, _ := json.Marshal(info)
	if strings.Contains(string(raw), priv.Encode()) {
		t.Fatal("public key info contained private key")
	}
}
