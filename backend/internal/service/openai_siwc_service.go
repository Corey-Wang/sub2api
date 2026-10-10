package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/proxyutil"
	"github.com/Wei-Shaw/sub2api/internal/pkg/siwc"
)

// OpenAISIWCService runs the official Sign in with ChatGPT token-sharing login.
type OpenAISIWCService struct {
	settings    *SettingService
	proxyRepo   ProxyRepository
	accountRepo AccountRepository
	sessions    *siwcSessionStore
	now         func() time.Time
	newClient   func(proxyURL string) (*http.Client, error)

	mu          sync.Mutex
	discovery   *siwc.Discovery
	discoveryAt time.Time
}

type siwcLoginSession struct {
	State         string
	Nonce         string
	Verifier      string
	RedirectURI   string
	ProxyURL      string
	HostID        string
	SavedClientID string
	AccountID     int64
	CreatedAt     time.Time
}

type siwcSessionStore struct {
	mu    sync.Mutex
	items map[string]*siwcLoginSession
}

func (s *siwcSessionStore) put(id string, session *siwcLoginSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.items == nil {
		s.items = map[string]*siwcLoginSession{}
	}
	now := time.Now()
	for key, item := range s.items {
		if item == nil || now.Sub(item.CreatedAt) > siwc.SessionTTL {
			delete(s.items, key)
		}
	}
	s.items[id] = session
}

func (s *siwcSessionStore) take(id string) (*siwcLoginSession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.items[id]
	if !ok || item == nil || time.Since(item.CreatedAt) > siwc.SessionTTL {
		delete(s.items, id)
		return nil, false
	}
	delete(s.items, id)
	return item, true
}

// SIWCAuthURLResult is returned to the admin UI.
type SIWCAuthURLResult struct {
	AuthURL   string `json:"auth_url"`
	SessionID string `json:"session_id"`
}

// SIWCExchangeResult is the credential payload the admin UI stores on the account.
type SIWCExchangeResult struct {
	Credentials map[string]any `json:"credentials"`
	Email       string         `json:"email,omitempty"`
	Scopes      string         `json:"scopes,omitempty"`
	ExpiresAt   string         `json:"expires_at,omitempty"`
	ClientID    string         `json:"client_id,omitempty"`
	AccountID   int64          `json:"account_id,omitempty"`
}

// NewOpenAISIWCService creates the SIWC login service. All dependencies may be nil in unit tests
// that inject an HTTP client and skip settings.
func NewOpenAISIWCService(settings *SettingService, proxyRepo ProxyRepository, accountRepo AccountRepository) *OpenAISIWCService {
	return &OpenAISIWCService{
		settings:    settings,
		proxyRepo:   proxyRepo,
		accountRepo: accountRepo,
		sessions:    &siwcSessionStore{},
		now:         time.Now,
		newClient:   newSIWCHTTPClient,
	}
}

func newSIWCHTTPClient(proxyURL string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if strings.TrimSpace(proxyURL) != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("siwc proxy: %w", err)
		}
		if err := proxyutil.ConfigureTransportProxy(transport, parsed); err != nil {
			return nil, err
		}
	}
	return &http.Client{Timeout: 30 * time.Second, Transport: transport}, nil
}

// GenerateAuthURL starts a login or re-authorization.
// accountID > 0 reuses that account's issued client id and email as login_hint.
func (s *OpenAISIWCService) GenerateAuthURL(ctx context.Context, proxyID *int64, redirectURI string, accountID int64, reconsent bool) (*SIWCAuthURLResult, error) {
	if s == nil {
		return nil, infraerrors.New(http.StatusInternalServerError, "SIWC_UNAVAILABLE", "SIWC service is not configured")
	}
	if strings.TrimSpace(redirectURI) == "" {
		redirectURI = siwc.DefaultRedirectURI()
	}
	if err := siwc.ValidateRedirectURI(redirectURI); err != nil {
		return nil, infraerrors.BadRequest("SIWC_REDIRECT_URI", err.Error())
	}
	hostID, err := s.hostID(ctx)
	if err != nil {
		return nil, err
	}
	proxyURL, err := s.proxyURL(ctx, proxyID)
	if err != nil {
		return nil, err
	}
	savedClientID := ""
	loginHint := ""
	if accountID > 0 {
		account, err := s.accountRepo.GetByID(ctx, accountID)
		if err != nil || account == nil || !account.IsOpenAISIWC() {
			return nil, infraerrors.BadRequest("SIWC_ACCOUNT", "account is not an official SIWC account")
		}
		savedClientID = strings.TrimSpace(account.GetCredential("client_id"))
		loginHint = strings.TrimSpace(account.GetCredential("email"))
		if account.ProxyID != nil && proxyID == nil {
			proxyURL = s.proxyURLForAccount(ctx, account)
		}
	}
	verifier, challenge, err := siwc.GeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := siwc.RandomToken()
	if err != nil {
		return nil, err
	}
	nonce, err := siwc.RandomToken()
	if err != nil {
		return nil, err
	}
	disc, client, err := s.discoveryClient(ctx, proxyURL)
	if err != nil {
		return nil, err
	}
	_ = client
	authURL, err := siwc.AuthorizeParams(disc.AuthorizationEndpoint, savedClientID, redirectURI, state, nonce, challenge, hostID, loginHint, reconsent)
	if err != nil {
		return nil, infraerrors.BadRequest("SIWC_AUTHORIZE", err.Error())
	}
	sessionID, err := randomSessionID()
	if err != nil {
		return nil, err
	}
	s.sessions.put(sessionID, &siwcLoginSession{
		State: state, Nonce: nonce, Verifier: verifier, RedirectURI: redirectURI,
		ProxyURL: proxyURL, HostID: hostID, SavedClientID: savedClientID,
		AccountID: accountID, CreatedAt: s.now(),
	})
	return &SIWCAuthURLResult{AuthURL: authURL, SessionID: sessionID}, nil
}

// ExchangeCallback completes the login from the pasted callback URL.
// When the session belongs to an existing account, credentials are persisted and the error flag is cleared.
func (s *OpenAISIWCService) ExchangeCallback(ctx context.Context, sessionID, callbackURL string) (*SIWCExchangeResult, error) {
	session, ok := s.sessions.take(strings.TrimSpace(sessionID))
	if !ok {
		return nil, infraerrors.BadRequest("SIWC_SESSION", "login session not found or expired")
	}
	savedClientID := session.SavedClientID
	cb, err := siwc.ParseCallbackURL(callbackURL, session.State, savedClientID)
	if err != nil {
		return nil, infraerrors.BadRequest("SIWC_CALLBACK", err.Error())
	}
	disc, client, err := s.discoveryClient(ctx, session.ProxyURL)
	if err != nil {
		return nil, err
	}
	tokenSet, err := siwc.ExchangeCode(ctx, client, disc.TokenEndpoint, cb.ClientID, cb.Code, session.Verifier, session.RedirectURI)
	if err != nil {
		return nil, infraerrors.BadRequest("SIWC_TOKEN", err.Error())
	}
	if !siwc.ScopeGranted(tokenSet.Scopes) {
		return nil, infraerrors.BadRequest("SIWC_SCOPE", "chatgpt.tokens.use.direct was not granted; re-authorize and approve plan usage")
	}
	claims, err := siwc.VerifyIDToken(ctx, client, disc.JWKSURI, disc.Issuer, tokenSet.IDToken, cb.ClientID, session.Nonce)
	if err != nil {
		return nil, infraerrors.BadRequest("SIWC_ID_TOKEN", err.Error())
	}
	creds := siwcCredentialsFromToken(nil, tokenSet, cb.ClientID, claims.Subject, claims.Email)
	models, modelErr := s.fetchModels(ctx, client, tokenSet.AccessToken)
	if modelErr != nil {
		slog.Warn("siwc_models_fetch_failed", "error", modelErr)
	} else if models != nil {
		creds["models"] = models
	}
	result := &SIWCExchangeResult{
		Credentials: creds,
		Email:       claims.Email,
		Scopes:      strings.Join(tokenSet.Scopes, " "),
		ExpiresAt:   tokenSet.ExpiresAt.UTC().Format(time.RFC3339),
		ClientID:    cb.ClientID,
		AccountID:   session.AccountID,
	}
	if session.AccountID > 0 {
		if err := s.persistReauth(ctx, session.AccountID, creds); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// RevokeAccount revokes the refresh token and clears local tokens.
func (s *OpenAISIWCService) RevokeAccount(ctx context.Context, accountID int64) error {
	if s == nil || s.accountRepo == nil {
		return infraerrors.New(http.StatusInternalServerError, "SIWC_UNAVAILABLE", "SIWC service is not configured")
	}
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil || !account.IsOpenAISIWC() {
		return infraerrors.BadRequest("SIWC_ACCOUNT", "account is not an official SIWC account")
	}
	refreshToken := account.GetCredential("refresh_token")
	clientID := account.GetCredential("client_id")
	if refreshToken != "" && clientID != "" {
		disc, client, err := s.discoveryClient(ctx, s.proxyURLForAccount(ctx, account))
		if err != nil {
			return err
		}
		if err := siwc.RevokeRefreshToken(ctx, client, disc.RevocationEndpoint, refreshToken, clientID); err != nil {
			return infraerrors.BadRequest("SIWC_REVOKE", err.Error())
		}
	}
	creds := cloneSIWCMap(account.Credentials)
	for _, key := range []string{"access_token", "refresh_token", "id_token", "expires_at", "expires_in", "earliest_refresh_at"} {
		delete(creds, key)
	}
	if err := persistAccountCredentials(ctx, s.accountRepo, account, creds); err != nil {
		return err
	}
	return s.accountRepo.SetError(ctx, account.ID, "已撤销授权，需要重新授权")
}

func (s *OpenAISIWCService) persistReauth(ctx context.Context, accountID int64, creds map[string]any) error {
	account, err := s.accountRepo.GetByID(ctx, accountID)
	if err != nil || account == nil {
		return infraerrors.BadRequest("SIWC_ACCOUNT", "account not found")
	}
	merged := cloneSIWCMap(account.Credentials)
	for key, value := range creds {
		merged[key] = value
	}
	if owner := account.GetCredential("owner_user_id"); owner != "" {
		merged["owner_user_id"] = ownerUserIDValue(account)
	}
	if err := persistAccountCredentials(ctx, s.accountRepo, account, merged); err != nil {
		return err
	}
	if err := s.accountRepo.ClearError(ctx, accountID); err != nil {
		slog.Warn("siwc_clear_error_failed", "account_id", accountID, "error", err)
	}
	return nil
}

func (s *OpenAISIWCService) RefreshAccount(ctx context.Context, account *Account) (map[string]any, error) {
	if account == nil || !account.IsOpenAISIWC() {
		return nil, fmt.Errorf("siwc refresh: unsupported account")
	}
	refreshToken := strings.TrimSpace(account.GetCredential("refresh_token"))
	clientID := strings.TrimSpace(account.GetCredential("client_id"))
	if refreshToken == "" || clientID == "" || clientID == siwc.DynamicClient {
		return nil, &siwc.OAuthError{Code: "invalid_grant", Description: "missing refresh token or issued client_id"}
	}
	disc, client, err := s.discoveryClient(ctx, s.proxyURLForAccount(ctx, account))
	if err != nil {
		return nil, err
	}
	previous := strings.Fields(account.GetCredential("scopes"))
	tokenSet, err := siwc.RefreshToken(ctx, client, disc.TokenEndpoint, clientID, refreshToken, previous)
	if err != nil {
		return nil, err
	}
	if tokenSet.IDToken != "" {
		claims, verifyErr := siwc.VerifyIDToken(ctx, client, disc.JWKSURI, disc.Issuer, tokenSet.IDToken, clientID, "")
		if verifyErr != nil {
			if oauthErr, ok := verifyErr.(*siwc.OAuthError); ok && oauthErr.Code == "identity_verification_unavailable" {
				slog.Warn("siwc_refresh_id_token_unverified", "account_id", account.ID, "error", verifyErr)
			} else {
				return nil, &siwc.OAuthError{Code: "invalid_grant", Description: "refreshed identity could not be verified"}
			}
		} else if subject := account.GetCredential("subject"); subject != "" && claims.Subject != subject {
			return nil, &siwc.OAuthError{Code: "invalid_grant", Description: "refreshed identity does not match the saved account"}
		}
	}
	return siwcCredentialsFromToken(account.Credentials, tokenSet, clientID, account.GetCredential("subject"), account.GetCredential("email")), nil
}

func (s *OpenAISIWCService) fetchModels(ctx context.Context, client *http.Client, accessToken string) ([]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, siwc.APIBase+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header = siwc.SanitizeHeaders(nil, accessToken, true)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("models HTTP %d", resp.StatusCode)
	}
	filtered, err := siwc.FilterListedModels(body)
	if err != nil {
		return nil, err
	}
	var doc struct {
		Models []any `json:"models"`
	}
	if err := json.Unmarshal(filtered, &doc); err != nil {
		return nil, err
	}
	return doc.Models, nil
}

func (s *OpenAISIWCService) hostID(ctx context.Context) (string, error) {
	if s.settings == nil {
		return "", infraerrors.New(http.StatusInternalServerError, "SIWC_HOST_ID", "settings service is not configured")
	}
	id, err := s.settings.GetOrCreateSIWCHostID(ctx)
	if err != nil {
		return "", infraerrors.Newf(http.StatusInternalServerError, "SIWC_HOST_ID", "failed to persist ext_agent_host_id: %v", err)
	}
	return id, nil
}

func (s *OpenAISIWCService) proxyURL(ctx context.Context, proxyID *int64) (string, error) {
	if proxyID == nil || s.proxyRepo == nil {
		return "", nil
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *proxyID)
	if err != nil || proxy == nil {
		return "", infraerrors.BadRequest("SIWC_PROXY", "proxy not found")
	}
	return proxy.URL(), nil
}

func (s *OpenAISIWCService) proxyURLForAccount(ctx context.Context, account *Account) string {
	if account == nil {
		return ""
	}
	if account.Proxy != nil {
		return account.Proxy.URL()
	}
	if account.ProxyID == nil || s.proxyRepo == nil {
		return ""
	}
	proxy, err := s.proxyRepo.GetByID(ctx, *account.ProxyID)
	if err != nil || proxy == nil {
		return ""
	}
	return proxy.URL()
}

func (s *OpenAISIWCService) discoveryClient(ctx context.Context, proxyURL string) (*siwc.Discovery, *http.Client, error) {
	client, err := s.newClient(proxyURL)
	if err != nil {
		return nil, nil, infraerrors.BadRequest("SIWC_PROXY", err.Error())
	}
	s.mu.Lock()
	if s.discovery != nil && time.Since(s.discoveryAt) < time.Hour {
		disc := s.discovery
		s.mu.Unlock()
		return disc, client, nil
	}
	s.mu.Unlock()
	disc, err := siwc.FetchDiscovery(ctx, client, siwc.Issuer)
	if err != nil {
		return nil, nil, infraerrors.Newf(http.StatusBadGateway, "SIWC_DISCOVERY", "openid discovery failed: %v", err)
	}
	s.mu.Lock()
	s.discovery = disc
	s.discoveryAt = time.Now()
	s.mu.Unlock()
	return disc, client, nil
}

func siwcCredentialsFromToken(base map[string]any, tokenSet *siwc.TokenSet, clientID, subject, email string) map[string]any {
	creds := cloneSIWCMap(base)
	creds["access_token"] = tokenSet.AccessToken
	creds["refresh_token"] = tokenSet.RefreshToken
	if tokenSet.IDToken != "" {
		creds["id_token"] = tokenSet.IDToken
	}
	creds["client_id"] = clientID
	creds["token_type"] = "Bearer"
	creds["expires_in"] = tokenSet.ExpiresIn
	creds["expires_at"] = tokenSet.ExpiresAt.UTC().Format(time.RFC3339)
	creds["scopes"] = strings.Join(tokenSet.Scopes, " ")
	if tokenSet.EarliestRefreshAt != nil {
		creds["earliest_refresh_at"] = tokenSet.EarliestRefreshAt.UTC().Format(time.RFC3339)
	}
	if subject != "" {
		creds["subject"] = subject
	}
	if email != "" {
		creds["email"] = email
	}
	return creds
}

func cloneSIWCMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in)+8)
	for key, value := range in {
		out[key] = value
	}
	return out
}

func ownerUserIDValue(account *Account) any {
	if account == nil || account.Credentials == nil {
		return nil
	}
	return account.Credentials["owner_user_id"]
}

func randomSessionID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// SIWCTokenRefresher refreshes SIWC accounts through OAuthRefreshAPI.
type SIWCTokenRefresher struct {
	svc *OpenAISIWCService
}

// NewSIWCTokenRefresher creates the request-path refresher. It is not registered
// with the Codex background refresher, which only selects type=oauth.
func NewSIWCTokenRefresher(svc *OpenAISIWCService) *SIWCTokenRefresher {
	return &SIWCTokenRefresher{svc: svc}
}

func (r *SIWCTokenRefresher) CacheKey(account *Account) string {
	if account == nil {
		return "siwc:0"
	}
	return fmt.Sprintf("siwc:%d", account.ID)
}

func (r *SIWCTokenRefresher) CanRefresh(account *Account) bool {
	return account != nil && account.IsOpenAISIWC() &&
		strings.TrimSpace(account.GetCredential("refresh_token")) != "" &&
		strings.TrimSpace(account.GetCredential("client_id")) != ""
}

func (r *SIWCTokenRefresher) NeedsRefresh(account *Account, refreshWindow time.Duration) bool {
	if !r.CanRefresh(account) {
		return false
	}
	if refreshWindow <= 0 {
		refreshWindow = siwc.RefreshMargin
	}
	expiresAt := account.GetCredentialAsTime("expires_at")
	var expiry time.Time
	if expiresAt != nil {
		expiry = *expiresAt
	}
	return siwc.NeedsRefresh(expiry, account.GetCredentialAsTime("earliest_refresh_at"), time.Now(), refreshWindow)
}

func (r *SIWCTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	if r == nil || r.svc == nil {
		return nil, fmt.Errorf("siwc refresher is not configured")
	}
	return r.svc.RefreshAccount(ctx, account)
}

type siwcStaleTokenRefresher struct {
	inner *SIWCTokenRefresher
	stale string
}

func (r *siwcStaleTokenRefresher) CacheKey(account *Account) string { return r.inner.CacheKey(account) }
func (r *siwcStaleTokenRefresher) CanRefresh(account *Account) bool {
	return r.inner.CanRefresh(account)
}
func (r *siwcStaleTokenRefresher) Refresh(ctx context.Context, account *Account) (map[string]any, error) {
	return r.inner.Refresh(ctx, account)
}
func (r *siwcStaleTokenRefresher) NeedsRefresh(account *Account, _ time.Duration) bool {
	if account == nil {
		return false
	}
	// Another worker already rotated the access token. Do not consume the refresh token again.
	return account.GetCredential("access_token") == r.stale
}
