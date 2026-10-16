package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type accountReportFixtureReader struct {
	rows map[string][]Account
	err  error
}

func (r accountReportFixtureReader) ListAllWithFilters(_ context.Context, p, _, _, _ string, _ int64, _ string) ([]Account, error) {
	return r.rows[p], r.err
}

func TestAccountReportKiroCollectionDeduplicationAndFailure(t *testing.T) {
	now := time.Now().UTC()
	var fail atomic.Bool
	var mutations atomic.Int32
	var balanceCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			mutations.Add(1)
		}
		if r.Header.Get("x-api-key") != "fixture-admin-secret" {
			w.WriteHeader(401)
			return
		}
		if fail.Load() {
			w.WriteHeader(503)
			fmt.Fprint(w, "sensitive upstream error")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/admin/credentials":
			fmt.Fprint(w, `{"credentials":[{"id":1,"email":"same@example.test","authMethod":"social","refreshTokenHash":"fixture-hash"}]}`)
		case "/api/admin/credentials/1/balance":
			balanceCalls.Add(1)
			fmt.Fprintf(w, `{"subscriptionTitle":"Pro","currentUsage":30,"usageLimit":100,"nextResetAt":%d}`, now.Add(24*time.Hour).Unix())
		default:
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s := &AccountReportService{repo: accountReportFixtureReader{}, client: upstream.Client(), instances: []KiroReportInstance{{ID: "kiro-a", Name: "A", URL: upstream.URL, AdminKey: "fixture-admin-secret"}, {ID: "kiro-b", Name: "B", URL: upstream.URL, AdminKey: "fixture-admin-secret"}}}
	snap := s.collect(context.Background(), accountReportSnapshot{}, now)
	r := buildAccountReport(snap, s.instances, now)
	require.True(t, r.Complete)
	require.Len(t, r.Accounts, 1)
	require.Len(t, r.Accounts[0].Locations, 2)
	require.Equal(t, 1, r.Counts.Total)
	require.Equal(t, 1, r.Counts.Available)
	q := r.Platforms[2].Quotas[0]
	require.Equal(t, 30.0, *q.Used)
	require.Equal(t, 100.0, *q.Limit)
	require.Equal(t, 30.0, q.Percent)
	serialized, _ := json.Marshal(r)
	require.NotContains(t, string(serialized), "fixture-admin-secret")
	require.NotContains(t, string(serialized), "fixture-hash")
	require.NotContains(t, string(serialized), upstream.URL)
	s.collect(context.Background(), snap, now.Add(time.Minute))
	require.EqualValues(t, 2, balanceCalls.Load(), "page/native refresh must not repeatedly fetch Kiro quotas")
	fail.Store(true)
	failed := s.collect(context.Background(), snap, now.Add(6*time.Minute))
	r = buildAccountReport(failed, s.instances, now.Add(6*time.Minute))
	require.False(t, r.Complete)
	require.Len(t, r.Accounts, 1, "failed sources retain last known accounts")
	require.Equal(t, "unknown", r.Accounts[0].Status)
	require.True(t, r.Accounts[0].Quotas[0].Stale)
	require.Empty(t, r.Platforms[2].Quotas, "expired observations must not inflate current totals")
	require.Zero(t, mutations.Load())
}

func TestAccountReportQuotaFailureAndAmbiguousIdentities(t *testing.T) {
	now := time.Now().UTC()
	var fail atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/admin/credentials" {
			fmt.Fprint(w, `{"credentials":[{"id":1,"email":"same@example.test","refreshTokenHash":"one"},{"id":2,"email":"same@example.test","refreshTokenHash":"two"}]}`)
			return
		}
		if fail.Load() {
			w.WriteHeader(429)
			return
		}
		fmt.Fprint(w, `{"subscriptionTitle":"Free","currentUsage":80,"usageLimit":100}`)
	}))
	defer upstream.Close()
	v := KiroReportInstance{ID: "a", Name: "A", URL: upstream.URL}
	s := &AccountReportService{repo: accountReportFixtureReader{}, client: upstream.Client(), instances: []KiroReportInstance{v}}
	snap := s.collect(context.Background(), accountReportSnapshot{}, now)
	r := buildAccountReport(snap, s.instances, now)
	require.Equal(t, 2, r.Counts.IdentityUncertain)
	require.Empty(t, r.Platforms[2].Quotas, "same email is not sufficient proof of a shared account")
	fail.Store(true)
	next := s.collect(context.Background(), snap, now.Add(5*time.Minute))
	r = buildAccountReport(next, s.instances, now.Add(5*time.Minute))
	require.True(t, r.Accounts[0].QuotaError)
	require.Equal(t, 80.0, *r.Accounts[0].Quotas[0].Used)
	require.True(t, r.Accounts[0].Quotas[0].Stale)
	// Operator aliases join different credentials without adding their quota twice.
	v.AccountAliases = map[string]string{"1": "same-account", "2": "same-account"}
	s.instances = []KiroReportInstance{v}
	fail.Store(false)
	r = buildAccountReport(s.collect(context.Background(), accountReportSnapshot{}, now), s.instances, now)
	require.Len(t, r.Accounts, 1)
	require.Equal(t, 1, r.Platforms[2].Quotas[0].Count)
}

func TestAccountReportNativeUnknownExpiryAndShadows(t *testing.T) {
	now := time.Now().UTC()
	parent := int64(1)
	s := &AccountReportService{repo: accountReportFixtureReader{rows: map[string][]Account{PlatformOpenAI: {
		{ID: 1, Name: "expired-observation", Platform: PlatformOpenAI, Type: AccountTypeOAuth, Status: "active", Schedulable: true, Extra: map[string]any{"codex_usage_updated_at": now.Add(-time.Hour).Format(time.RFC3339), "codex_5h_used_percent": 90.0, "codex_5h_reset_at": now.Add(-time.Minute).Format(time.RFC3339)}},
		{ID: 2, Name: "no-observation", Platform: PlatformOpenAI, Status: "active", Schedulable: true},
		{ID: 3, ParentAccountID: &parent, Platform: PlatformOpenAI, Status: "active", Schedulable: true},
	}}}}
	r := buildAccountReport(s.collect(context.Background(), accountReportSnapshot{}, now), nil, now)
	require.Len(t, r.Accounts, 2, "shadow accounts share a parent quota")
	require.Equal(t, 2, r.Counts.UnknownQuota)
	require.Equal(t, 90.0, r.Accounts[0].Quotas[0].Percent, "a reset is not evidence of zero upstream usage")
	require.Empty(t, r.Platforms[0].Quotas)
	s.repo = accountReportFixtureReader{err: errors.New("database credentials must not leak")}
	r = buildAccountReport(s.collect(context.Background(), accountReportSnapshot{}, now), nil, now)
	require.False(t, r.Complete)
	require.Equal(t, "source_unavailable", r.Sources[0].Error)
}

func TestAccountReportEmptyAndInvalidResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"credentials":null}`, `{"error":"not credentials"}`} {
		t.Run(body, func(t *testing.T) {
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
			defer u.Close()
			s := &AccountReportService{client: u.Client()}
			src, _ := s.collectKiro(context.Background(), KiroReportInstance{ID: "a", URL: u.URL}, AccountReportSource{}, nil, time.Now())
			require.Equal(t, "source_unavailable", src.Error)
		})
	}
	r := buildAccountReport(accountReportSnapshot{}, nil, time.Now())
	require.True(t, r.Pending)
	require.False(t, r.Complete)
}

func TestAccountReportAntigravityGeminiChannel(t *testing.T) {
	now := time.Now().UTC()
	usage := &AccountUsageService{antigravityQuotaFetcher: &AntigravityQuotaFetcher{}, cache: NewUsageCache()}
	usage.cache.antigravityCache.Store(int64(7), &antigravityUsageCache{timestamp: now, usageInfo: &UsageInfo{UpdatedAt: &now, SubscriptionTier: "PRO", AntigravityQuota: map[string]*AntigravityModelQuota{
		"gemini-pro":    {Utilization: 100, ResetTime: now.Add(time.Hour).Format(time.RFC3339)},
		"gemini-flash":  {Utilization: 25, ResetTime: now.Add(time.Hour).Format(time.RFC3339)},
		"claude-sonnet": {Utilization: 90},
	}}})
	s := &AccountReportService{usage: usage, repo: accountReportFixtureReader{rows: map[string][]Account{PlatformAntigravity: {{ID: 7, Name: "Antigravity account", Platform: PlatformAntigravity, Type: AccountTypeOAuth, Status: "active", Schedulable: true, Credentials: map[string]any{"access_token": "fixture-never-transmitted"}}}}}}
	r := buildAccountReport(s.collect(context.Background(), accountReportSnapshot{}, now), nil, now)
	require.Equal(t, 1, r.Platforms[1].Counts.Total)
	require.Len(t, r.Accounts[0].Quotas, 2)
	require.Equal(t, "limited", r.Accounts[0].Status)
	require.Equal(t, "Antigravity", r.Accounts[0].Locations[0].Name)
	require.Len(t, r.Platforms[1].Quotas, 2)
	require.Equal(t, "gemini-flash", r.Platforms[1].Quotas[0].Model)
	require.Equal(t, 25.0, r.Platforms[1].Quotas[0].Percent)
	require.Equal(t, "PRO", r.Accounts[0].Plan)
	data, _ := json.Marshal(r)
	require.NotContains(t, string(data), "claude-sonnet")
	require.NotContains(t, string(data), "fixture-never-transmitted")
}

func TestAccountReportModelAndMinuteQuotaExhaustion(t *testing.T) {
	pro := AccountReportQuota{Key: "gemini_pro_daily", Percent: 100}
	flash := AccountReportQuota{Key: "gemini_flash_daily", Percent: 100}
	require.Equal(t, "limited", reportQuotaStatus("gemini", "available", []AccountReportQuota{pro}))
	require.Equal(t, "exhausted", reportQuotaStatus("gemini", "available", []AccountReportQuota{pro, flash}))
	require.Equal(t, "limited", reportQuotaStatus("gemini", "available", []AccountReportQuota{{Key: "gemini_shared_minute", Percent: 100}}))
	require.Equal(t, "available", reportQuotaStatus("gpt", "available", []AccountReportQuota{{Key: "gpt_5h", Percent: 100, Stale: true}}))
	require.Equal(t, "exhausted", reportQuotaStatus("gpt", "available", []AccountReportQuota{{Key: "gpt_5h", Percent: 100}}))
}

func TestAccountReportConfigAndRedirects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instances.json")
	keyPath := filepath.Join(t.TempDir(), "key")
	require.NoError(t, os.WriteFile(keyPath, []byte("fixture-key\n"), 0600))
	require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`[{"id":"a","url":"http://127.0.0.1:8990","admin_key_file":%q}]`, keyPath)), 0600))
	s, err := NewAccountReportService(nil, nil, nil, &config.Config{AccountReport: config.AccountReportConfig{KiroInstancesFile: path}})
	require.NoError(t, err)
	require.Equal(t, "fixture-key", s.instances[0].AdminKey)
	for _, origin := range []string{"file:///tmp/x", "http://user:pass@host", "http://host/admin", "http://host?token=x"} {
		require.NoError(t, os.WriteFile(path, []byte(fmt.Sprintf(`[{"id":"a","url":%q,"admin_key":"key"}]`, origin)), 0600))
		_, err := NewAccountReportService(nil, nil, nil, &config.Config{AccountReport: config.AccountReportConfig{KiroInstancesFile: path}})
		require.Error(t, err)
	}
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	var out any
	require.Error(t, s.kiroGET(context.Background(), KiroReportInstance{URL: redirect.URL, AdminKey: "key"}, "/api/admin/credentials", &out))
	require.False(t, leaked.Load(), "admin keys must never follow redirects")
}
