package service

import (
	"context"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
)

func (p *OpenAITokenProvider) SetSIWC(svc *OpenAISIWCService) {
	if p == nil {
		return
	}
	p.siwc = svc
	if svc != nil {
		p.siwcExecutor = NewSIWCTokenRefresher(svc)
	}
}

func (p *OpenAITokenProvider) GetSIWCAccessToken(ctx context.Context, account *Account) (string, error) {
	if account == nil || !account.IsOpenAISIWC() {
		return "", errString("not an official SIWC account")
	}
	if p == nil || p.refreshAPI == nil || p.siwcExecutor == nil {
		token := strings.TrimSpace(account.GetCredential("access_token"))
		if token == "" {
			return "", errString("access_token not found in credentials")
		}
		return token, nil
	}
	result, err := p.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(ctx), account, p.siwcExecutor, siwc.RefreshMargin)
	if err != nil {
		p.noteSIWCRefreshError(ctx, account, err)
		return "", err
	}
	if result != nil && result.Account != nil {
		account = result.Account
	}
	token := strings.TrimSpace(account.GetCredential("access_token"))
	if token == "" {
		return "", errString("access_token not found in credentials")
	}
	return token, nil
}

// ForceRefreshSIWC refreshes even when the access token is still inside the
// normal window. It no-ops when the stored access token already differs from stale.
func (p *OpenAITokenProvider) ForceRefreshSIWC(ctx context.Context, account *Account, stale string) (string, error) {
	if p == nil || p.refreshAPI == nil || p.siwc == nil {
		return "", errString("siwc refresh is not configured")
	}
	executor := &siwcStaleTokenRefresher{inner: NewSIWCTokenRefresher(p.siwc), stale: stale}
	result, err := p.refreshAPI.RefreshIfNeeded(withOAuthRefreshRequestPath(ctx), account, executor, siwc.RefreshMargin)
	if err != nil {
		p.noteSIWCRefreshError(ctx, account, err)
		return "", err
	}
	if result != nil && result.Account != nil {
		account = result.Account
	}
	return strings.TrimSpace(account.GetCredential("access_token")), nil
}

func (p *OpenAITokenProvider) noteSIWCRefreshError(ctx context.Context, account *Account, err error) {
	if p == nil || p.accountRepo == nil || account == nil || !siwc.IsUnusableRefresh(err) {
		return
	}
	code := "invalid_grant"
	if oauthErr, ok := err.(*siwc.OAuthError); ok && oauthErr.Code != "" {
		code = oauthErr.Code
	}
	if setErr := p.accountRepo.SetError(ctx, account.ID, "需要重新授权 ("+code+")"); setErr != nil {
		return
	}
}
