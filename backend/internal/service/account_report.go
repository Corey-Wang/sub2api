package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/google/uuid"
)

const reportInterval = time.Minute
const reportKiroInterval = 5 * time.Minute

// AccountReportCache shares sanitized observations and a collection lease between replicas.
type AccountReportCache interface {
	Load(context.Context, string) ([]byte, error)
	Save(context.Context, string, []byte) error
	Lock(context.Context, string, string, time.Duration) (bool, error)
	Unlock(context.Context, string, string) error
}

type accountReportReader interface {
	ListAllWithFilters(context.Context, string, string, string, string, int64, string) ([]Account, error)
}

// KiroReportInstance is operator configuration, never returned by the report API.
type KiroReportInstance struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	URL          string `json:"url"`
	AdminKey     string `json:"admin_key"`
	AdminKeyFile string `json:"admin_key_file,omitempty"`
	// Explicit aliases join separately authenticated credentials for the same account.
	AccountAliases map[string]string `json:"account_aliases,omitempty"`
}

type AccountReportQuota struct {
	Model     string     `json:"model,omitempty"`
	Key       string     `json:"key"`
	Unit      string     `json:"unit"`
	Used      *float64   `json:"used"`
	Limit     *float64   `json:"limit"`
	Percent   float64    `json:"percent"`
	ResetAt   *time.Time `json:"reset_at"`
	UpdatedAt *time.Time `json:"updated_at"`
	Estimated bool       `json:"estimated"`
	Stale     bool       `json:"stale"`
}

type AccountReportLocation struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AccountID string    `json:"account_id"`
	Status    string    `json:"status"`
	UpdatedAt time.Time `json:"updated_at"`
	Stale     bool      `json:"stale"`
}

type AccountReportRow struct {
	ID                string                  `json:"id"`
	Platform          string                  `json:"platform"`
	Name              string                  `json:"name"`
	Plan              string                  `json:"plan"`
	Type              string                  `json:"type"`
	Status            string                  `json:"status"`
	Locations         []AccountReportLocation `json:"locations"`
	Quotas            []AccountReportQuota    `json:"quotas"`
	QuotaError        bool                    `json:"quota_error"`
	IdentityUncertain bool                    `json:"identity_uncertain"`
}

type AccountReportSource struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Platform    string     `json:"platform"`
	UpdatedAt   *time.Time `json:"updated_at"`
	AttemptedAt time.Time  `json:"attempted_at"`
	Error       string     `json:"error"`
	Stale       bool       `json:"stale"`
}

type AccountReportCounts struct {
	Total             int `json:"total"`
	Available         int `json:"available"`
	Limited           int `json:"limited"`
	Error             int `json:"error"`
	Disabled          int `json:"disabled"`
	Exhausted         int `json:"exhausted"`
	Unknown           int `json:"unknown"`
	UnknownQuota      int `json:"unknown_quota"`
	NearLimit         int `json:"near_limit"`
	IdentityUncertain int `json:"identity_uncertain"`
}

type AccountReportPlan struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type AccountReportQuotaSummary struct {
	Model     string   `json:"model,omitempty"`
	Key       string   `json:"key"`
	Plan      string   `json:"plan"`
	Unit      string   `json:"unit"`
	Count     int      `json:"count"`
	Percent   float64  `json:"percent"`
	Used      *float64 `json:"used"`
	Limit     *float64 `json:"limit"`
	Estimated bool     `json:"estimated"`
}
type AccountReportPlatform struct {
	Platform string                      `json:"platform"`
	Counts   AccountReportCounts         `json:"counts"`
	Plans    []AccountReportPlan         `json:"plans"`
	Quotas   []AccountReportQuotaSummary `json:"quotas"`
}
type AccountReport struct {
	UpdatedAt *time.Time              `json:"updated_at"`
	Pending   bool                    `json:"pending"`
	Complete  bool                    `json:"complete"`
	Sources   []AccountReportSource   `json:"sources"`
	Accounts  []AccountReportRow      `json:"accounts"`
	Counts    AccountReportCounts     `json:"counts"`
	Platforms []AccountReportPlatform `json:"platforms"`
}
type accountReportSnapshot struct {
	UpdatedAt time.Time             `json:"updated_at"`
	Sources   []AccountReportSource `json:"sources"`
	Rows      []AccountReportRow    `json:"rows"`
}

type AccountReportService struct {
	repo      accountReportReader
	usage     *AccountUsageService
	cache     AccountReportCache
	instances []KiroReportInstance
	client    *http.Client
	cacheID   string
	mu        sync.RWMutex
	latest    []byte
	cancel    context.CancelFunc
	done      chan struct{}
}

func NewAccountReportService(repo AccountRepository, usage *AccountUsageService, cache AccountReportCache, cfg *config.Config) (*AccountReportService, error) {
	s := &AccountReportService{repo: repo, usage: usage, cache: cache,
		client: &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	if cfg.AccountReport.KiroInstancesFile != "" {
		data, err := os.ReadFile(cfg.AccountReport.KiroInstancesFile)
		if err != nil {
			return nil, errors.New("account report: cannot read Kiro instances file")
		}
		if err = json.Unmarshal(data, &s.instances); err != nil {
			return nil, errors.New("account report: invalid Kiro instances JSON")
		}
	}
	seen := map[string]bool{}
	for i := range s.instances {
		v := &s.instances[i]
		if v.ID == "" || seen[v.ID] || v.ID == "sub2api-gpt" || v.ID == "sub2api-gemini" {
			return nil, errors.New("account report: Kiro instance IDs must be unique and nonempty")
		}
		seen[v.ID] = true
		u, err := url.Parse(v.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return nil, errors.New("account report: Kiro URL must be an HTTP(S) origin without credentials, query or path")
		}
		v.URL = strings.TrimRight(v.URL, "/")
		if v.AdminKeyFile != "" {
			key, err := os.ReadFile(v.AdminKeyFile)
			if err != nil {
				return nil, errors.New("account report: cannot read Kiro admin key file")
			}
			v.AdminKey = strings.TrimSpace(string(key))
		}
		if strings.TrimSpace(v.AdminKey) == "" {
			return nil, errors.New("account report: missing Kiro admin key")
		}
		if v.Name == "" {
			v.Name = v.ID
		}
	}
	data, _ := json.Marshal(s.instances)
	s.cacheID = reportHash("v2:" + string(data))
	return s, nil
}

func (s *AccountReportService) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	go func() {
		defer close(s.done)
		ticker := time.NewTicker(reportInterval)
		defer ticker.Stop()
		for {
			s.refresh(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *AccountReportService) Stop() {
	if s.cancel != nil {
		s.cancel()
		<-s.done
	}
}

func (s *AccountReportService) load(ctx context.Context) accountReportSnapshot {
	if s.cache != nil {
		if data, err := s.cache.Load(ctx, s.cacheID); err == nil && len(data) > 0 {
			s.mu.Lock()
			s.latest = data
			s.mu.Unlock()
		}
	}
	s.mu.RLock()
	data := append([]byte(nil), s.latest...)
	s.mu.RUnlock()
	var snap accountReportSnapshot
	_ = json.Unmarshal(data, &snap)
	return snap
}

func (s *AccountReportService) refresh(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 90*time.Second)
	defer cancel()
	prev := s.load(ctx)
	if time.Since(prev.UpdatedAt) < 55*time.Second {
		return
	}
	if s.cache != nil {
		token := uuid.NewString()
		locked, err := s.cache.Lock(ctx, s.cacheID, token, 2*time.Minute)
		if err != nil || !locked {
			return
		}
		defer func() {
			unlockCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
			defer c()
			_ = s.cache.Unlock(unlockCtx, s.cacheID, token)
		}()
		prev = s.load(ctx)
		if time.Since(prev.UpdatedAt) < 55*time.Second {
			return
		}
	}
	snap := s.collect(ctx, prev, time.Now().UTC())
	data, err := json.Marshal(snap)
	if err != nil {
		return
	}
	s.mu.Lock()
	s.latest = data
	s.mu.Unlock()
	if s.cache != nil {
		_ = s.cache.Save(ctx, s.cacheID, data)
	}
}

func (s *AccountReportService) Get(ctx context.Context) *AccountReport {
	snap := s.load(ctx)
	return buildAccountReport(snap, s.instances, time.Now().UTC())
}

func reportHash(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:16])
}
func reportSourceRows(rows []AccountReportRow, id string) []AccountReportRow {
	out := []AccountReportRow{}
	for _, row := range rows {
		if len(row.Locations) == 1 && row.Locations[0].ID == id {
			out = append(out, row)
		}
	}
	return out
}
func reportPreviousSource(prev accountReportSnapshot, id string) AccountReportSource {
	for _, v := range prev.Sources {
		if v.ID == id {
			return v
		}
	}
	return AccountReportSource{}
}

func (s *AccountReportService) collect(ctx context.Context, prev accountReportSnapshot, now time.Time) accountReportSnapshot {
	snap := accountReportSnapshot{UpdatedAt: now, Sources: []AccountReportSource{}, Rows: []AccountReportRow{}}
	for _, p := range []struct{ native, report string }{{PlatformOpenAI, "gpt"}, {PlatformGemini, "gemini"}, {PlatformAntigravity, "gemini"}} {
		id := "sub2api-" + p.report
		name := "Sub2API"
		if p.native == PlatformAntigravity {
			id = "sub2api-antigravity"
			name = "Antigravity"
		}
		source := AccountReportSource{ID: id, Name: name, Platform: p.report, AttemptedAt: now}
		accounts, err := s.repo.ListAllWithFilters(ctx, p.native, "", "", "", 0, "")
		if err != nil {
			source.UpdatedAt = reportPreviousSource(prev, id).UpdatedAt
			source.Error = "source_unavailable"
			snap.Rows = append(snap.Rows, reportSourceRows(prev.Rows, id)...)
		} else {
			source.UpdatedAt = &now
			for i := range accounts {
				if accounts[i].ParentAccountID != nil {
					continue
				} // A shadow is a quota dimension, not another account.
				snap.Rows = append(snap.Rows, s.nativeRow(ctx, &accounts[i], p.report, now))
			}
		}
		snap.Sources = append(snap.Sources, source)
	}
	for _, instance := range s.instances {
		old := reportPreviousSource(prev, instance.ID)
		if !old.AttemptedAt.IsZero() && now.Sub(old.AttemptedAt) < reportKiroInterval {
			snap.Sources = append(snap.Sources, old)
			snap.Rows = append(snap.Rows, reportSourceRows(prev.Rows, instance.ID)...)
			continue
		}
		source, rows := s.collectKiro(ctx, instance, old, reportSourceRows(prev.Rows, instance.ID), now)
		snap.Sources = append(snap.Sources, source)
		snap.Rows = append(snap.Rows, rows...)
	}
	return snap
}

func (s *AccountReportService) nativeRow(ctx context.Context, a *Account, platform string, now time.Time) AccountReportRow {
	status := "available"
	switch {
	case a.Status == "error":
		status = "error"
	case !a.IsActive() || !a.Schedulable:
		status = "disabled"
	case a.IsRateLimited() || a.IsOverloaded() || (a.TempUnschedulableUntil != nil && now.Before(*a.TempUnschedulableUntil)):
		status = "limited"
	case a.AutoPauseOnExpired && a.ExpiresAt != nil && !now.Before(*a.ExpiresAt):
		status = "error"
	case a.IsAPIKeyOrBedrock() && a.IsQuotaExceeded():
		status = "exhausted"
	}
	row := AccountReportRow{ID: platform + ":" + strconv.FormatInt(a.ID, 10), Platform: platform, Name: a.Name, Type: a.Type, Status: status,
		Locations: []AccountReportLocation{{ID: "sub2api-" + platform, Name: "Sub2API", AccountID: strconv.FormatInt(a.ID, 10), Status: status, UpdatedAt: now}}, Quotas: []AccountReportQuota{}}
	if platform == "gpt" {
		row.Plan = a.GetCredential("plan_type")
		if row.Plan == "" {
			row.Plan, _ = a.Extra["plan_type"].(string)
		}
		var updated *time.Time
		if raw, ok := a.Extra["codex_usage_updated_at"]; ok {
			if t, err := parseTime(fmt.Sprint(raw)); err == nil {
				updated = &t
			}
		}
		for _, w := range []struct{ window, key string }{{"5h", "gpt_5h"}, {"7d", "gpt_7d"}} {
			p := buildCodexUsageProgressFromExtra(a.Extra, w.window, now)
			if p == nil {
				continue
			}
			// Expired observations are unknown; a reset is not evidence of zero usage.
			percent := parseExtraFloat64(a.Extra["codex_"+w.window+"_used_percent"])
			q := AccountReportQuota{Key: w.key, Unit: "percent", Percent: percent, ResetAt: p.ResetsAt, UpdatedAt: updated}
			q.Stale = updated == nil || now.Sub(*updated) > 10*time.Minute || (q.ResetAt != nil && !now.Before(*q.ResetAt))
			if finiteReportNumber(percent) {
				row.Quotas = append(row.Quotas, q)
			}
		}
	} else if a.Platform == PlatformAntigravity {
		row.Locations[0].ID = "sub2api-antigravity"
		row.Locations[0].Name = "Antigravity"
		row.Plan = a.GetCredential("plan_type")
		if s.usage != nil {
			usage, err := s.usage.getAntigravityUsage(ctx, a)
			if err != nil || usage == nil || usage.ErrorCode != "" || usage.IsForbidden {
				row.QuotaError = true
			} else {
				if usage.SubscriptionTier != "" {
					row.Plan = usage.SubscriptionTier
				}
				row.Quotas = reportAntigravityQuotas(usage)
			}
		}
	} else {
		row.Plan = a.GeminiTierID()
		if s.usage != nil {
			usage, err := s.usage.getGeminiUsage(ctx, a)
			if err != nil {
				row.QuotaError = true
			} else if usage != nil {
				for _, w := range []struct {
					key string
					p   *UsageProgress
				}{{"gemini_shared_daily", usage.GeminiSharedDaily}, {"gemini_pro_daily", usage.GeminiProDaily}, {"gemini_flash_daily", usage.GeminiFlashDaily}, {"gemini_shared_minute", usage.GeminiSharedMinute}, {"gemini_pro_minute", usage.GeminiProMinute}, {"gemini_flash_minute", usage.GeminiFlashMinute}} {
					if w.p == nil || w.p.LimitRequests <= 0 {
						continue
					}
					used, limit := float64(w.p.UsedRequests), float64(w.p.LimitRequests)
					row.Quotas = append(row.Quotas, AccountReportQuota{Key: w.key, Unit: "requests", Used: &used, Limit: &limit, Percent: w.p.Utilization, ResetAt: w.p.ResetsAt, UpdatedAt: &now, Estimated: true})
				}
			}
		}
	}
	return row
}

func finiteReportNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 }

func reportAntigravityQuotas(usage *UsageInfo) []AccountReportQuota {
	quotas := []AccountReportQuota{}
	for model, value := range usage.AntigravityQuota {
		if value == nil || !strings.Contains(strings.ToLower(model), "gemini") || value.Utilization < 0 {
			continue
		}
		var reset *time.Time
		if t, err := time.Parse(time.RFC3339, value.ResetTime); err == nil {
			reset = &t
		}
		quotas = append(quotas, AccountReportQuota{Key: "antigravity_model", Model: model, Unit: "percent", Percent: float64(value.Utilization), ResetAt: reset, UpdatedAt: usage.UpdatedAt})
	}
	sort.Slice(quotas, func(i, j int) bool { return quotas[i].Model < quotas[j].Model })
	return quotas
}

type kiroReportCredential struct {
	ID                  uint64 `json:"id"`
	Disabled            bool   `json:"disabled"`
	DisabledReason      string `json:"disabledReason"`
	FailureCount        int    `json:"failureCount"`
	RefreshFailureCount int    `json:"refreshFailureCount"`
	Email               string `json:"email"`
	AuthMethod          string `json:"authMethod"`
	RefreshTokenHash    string `json:"refreshTokenHash"`
	APIKeyHash          string `json:"apiKeyHash"`
}
type kiroReportBalance struct {
	SubscriptionTitle string   `json:"subscriptionTitle"`
	CurrentUsage      *float64 `json:"currentUsage"`
	UsageLimit        *float64 `json:"usageLimit"`
	NextResetAt       *float64 `json:"nextResetAt"`
}

func (s *AccountReportService) kiroGET(ctx context.Context, instance KiroReportInstance, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, instance.URL+path, nil)
	if err != nil {
		return errors.New("invalid_request")
	}
	req.Header.Set("x-api-key", instance.AdminKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return errors.New("source_unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("source_unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024+1))
	if err != nil || len(data) > 4*1024*1024 {
		return errors.New("invalid_response")
	}
	if json.Unmarshal(data, out) != nil {
		return errors.New("invalid_response")
	}
	return nil
}

func (s *AccountReportService) collectKiro(ctx context.Context, instance KiroReportInstance, oldSource AccountReportSource, previous []AccountReportRow, now time.Time) (AccountReportSource, []AccountReportRow) {
	source := AccountReportSource{ID: instance.ID, Name: instance.Name, Platform: "kiro", AttemptedAt: now}
	var list struct {
		Credentials *[]kiroReportCredential `json:"credentials"`
	}
	if err := s.kiroGET(ctx, instance, "/api/admin/credentials", &list); err != nil || list.Credentials == nil {
		source.Error = "source_unavailable"
		source.UpdatedAt = oldSource.UpdatedAt
		return source, previous
	}
	source.UpdatedAt = &now
	old := map[string]AccountReportRow{}
	for _, r := range previous {
		old[r.Locations[0].AccountID] = r
	}
	rows := make([]AccountReportRow, len(*list.Credentials))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup
	for i, cred := range *list.Credentials {
		id := strconv.FormatUint(cred.ID, 10)
		identity := instance.ID + ":" + id
		uncertain := true
		switch {
		case instance.AccountAliases[id] != "":
			identity = "alias:" + instance.AccountAliases[id]
			uncertain = false
		case cred.APIKeyHash != "":
			identity = "key:" + cred.APIKeyHash
			uncertain = false
		case cred.RefreshTokenHash != "":
			identity = "refresh:" + cred.RefreshTokenHash
			uncertain = false
		}
		status := "available"
		if cred.Disabled {
			status = "disabled"
			if cred.DisabledReason == "QuotaExceeded" {
				status = "exhausted"
			} else if cred.RefreshFailureCount > 0 || cred.FailureCount > 0 {
				status = "error"
			}
		}
		name := cred.Email
		if name == "" {
			name = instance.Name + " #" + id
		}
		rows[i] = AccountReportRow{ID: "kiro:" + reportHash(identity), Platform: "kiro", Name: name, Type: cred.AuthMethod, Status: status, IdentityUncertain: uncertain,
			Locations: []AccountReportLocation{{ID: instance.ID, Name: instance.Name, AccountID: id, Status: status, UpdatedAt: now}}, Quotas: []AccountReportQuota{}}
		// Retain a previous successful observation if the quota endpoint fails.
		if r, ok := old[id]; ok && r.ID == rows[i].ID {
			rows[i].Plan = r.Plan
			rows[i].Quotas = r.Quotas
		}
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				rows[i].QuotaError = true
				return
			}
			defer func() { <-sem }()
			var b kiroReportBalance
			if err := s.kiroGET(ctx, instance, "/api/admin/credentials/"+id+"/balance", &b); err != nil || b.CurrentUsage == nil || b.UsageLimit == nil || !finiteReportNumber(*b.CurrentUsage) || !finiteReportNumber(*b.UsageLimit) || *b.UsageLimit <= 0 {
				rows[i].QuotaError = true
				for j := range rows[i].Quotas {
					rows[i].Quotas[j].Stale = true
				}
				return
			}
			var reset *time.Time
			if b.NextResetAt != nil && finiteReportNumber(*b.NextResetAt) && *b.NextResetAt > 0 {
				t := time.Unix(int64(*b.NextResetAt), 0).UTC()
				reset = &t
			}
			rows[i].Plan = b.SubscriptionTitle
			rows[i].Quotas = []AccountReportQuota{{Key: "kiro_credits", Unit: "credits", Used: b.CurrentUsage, Limit: b.UsageLimit, Percent: *b.CurrentUsage / *b.UsageLimit * 100, ResetAt: reset, UpdatedAt: &now}}
		}(i, id)
	}
	wg.Wait()
	return source, rows
}

func buildAccountReport(snap accountReportSnapshot, instances []KiroReportInstance, now time.Time) *AccountReport {
	r := &AccountReport{Pending: snap.UpdatedAt.IsZero(), Complete: true, Sources: snap.Sources, Accounts: []AccountReportRow{}, Platforms: []AccountReportPlatform{}}
	if !snap.UpdatedAt.IsZero() {
		r.UpdatedAt = &snap.UpdatedAt
	}
	if len(r.Sources) == 0 {
		for _, p := range []string{"gpt", "gemini"} {
			r.Sources = append(r.Sources, AccountReportSource{ID: "sub2api-" + p, Name: "Sub2API", Platform: p})
		}
		for _, v := range instances {
			r.Sources = append(r.Sources, AccountReportSource{ID: v.ID, Name: v.Name, Platform: "kiro"})
		}
		r.Sources = append(r.Sources, AccountReportSource{ID: "sub2api-antigravity", Name: "Antigravity", Platform: "gemini"})
	}
	for i := range r.Sources {
		source := &r.Sources[i]
		ttl := 2 * reportInterval
		if source.Platform == "kiro" {
			ttl = reportKiroInterval + time.Minute
		}
		source.Stale = source.UpdatedAt == nil || source.Error != "" || now.Sub(*source.UpdatedAt) > ttl
		if source.Stale {
			r.Complete = false
		}
	}
	byID := map[string]int{}
	for _, row := range snap.Rows {
		allLocationsStale := true
		for i := range row.Locations {
			for _, source := range r.Sources {
				if source.ID == row.Locations[i].ID {
					row.Locations[i].Stale = source.Stale
				}
			}
			if !row.Locations[i].Stale {
				allLocationsStale = false
			}
		}
		for i := range row.Quotas {
			q := &row.Quotas[i]
			ttl := 10 * time.Minute
			if row.Platform == "kiro" {
				ttl = reportKiroInterval + time.Minute
			}
			if row.Platform == "gemini" {
				ttl = 2 * time.Minute
			}
			if q.Key == "antigravity_model" {
				ttl = 5 * time.Minute
			}
			q.Stale = q.Stale || allLocationsStale || q.UpdatedAt == nil || now.Sub(*q.UpdatedAt) > ttl || (q.ResetAt != nil && !now.Before(*q.ResetAt))
		}
		if i, ok := byID[row.ID]; ok {
			dst := &r.Accounts[i]
			dst.Locations = append(dst.Locations, row.Locations...)
			if newestReportQuota(row.Quotas).After(newestReportQuota(dst.Quotas)) || (dst.QuotaError && !row.QuotaError) {
				dst.Quotas = row.Quotas
				dst.Plan = row.Plan
				dst.QuotaError = row.QuotaError
			}
		} else {
			byID[row.ID] = len(r.Accounts)
			r.Accounts = append(r.Accounts, row)
		}
	}
	// Equal email addresses with distinct credentials need explicit operator aliases.
	names := map[string]int{}
	for _, a := range r.Accounts {
		if a.Platform == "kiro" && strings.Contains(a.Name, "@") {
			names[strings.ToLower(strings.TrimSpace(a.Name))]++
		}
	}
	for i := range r.Accounts {
		a := &r.Accounts[i]
		if a.Platform == "kiro" && names[strings.ToLower(strings.TrimSpace(a.Name))] > 1 {
			a.IdentityUncertain = true
		}
		a.Status = "unknown"
		for _, loc := range a.Locations {
			if !loc.Stale && reportStatusRank(loc.Status) > reportStatusRank(a.Status) {
				a.Status = loc.Status
			}
		}
		a.Status = reportQuotaStatus(a.Platform, a.Status, a.Quotas)
	}
	sort.Slice(r.Accounts, func(i, j int) bool {
		a, b := r.Accounts[i], r.Accounts[j]
		if a.Platform != b.Platform {
			return a.Platform < b.Platform
		}
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return a.ID < b.ID
	})
	r.Counts = reportCounts(r.Accounts)
	for _, p := range []string{"gpt", "gemini", "kiro"} {
		rows := []AccountReportRow{}
		for _, a := range r.Accounts {
			if a.Platform == p {
				rows = append(rows, a)
			}
		}
		platform := AccountReportPlatform{Platform: p, Counts: reportCounts(rows), Plans: []AccountReportPlan{}, Quotas: []AccountReportQuotaSummary{}}
		plans := map[string]int{}
		sums := map[string]*AccountReportQuotaSummary{}
		for _, a := range rows {
			plans[a.Plan]++
			for _, q := range a.Quotas {
				if q.Stale || a.IdentityUncertain {
					continue
				}
				plan := ""
				if p == "kiro" {
					plan = a.Plan
				}
				key := q.Key + ":" + plan + ":" + q.Model
				if sums[key] == nil {
					sums[key] = &AccountReportQuotaSummary{Key: q.Key, Model: q.Model, Plan: plan, Unit: q.Unit, Estimated: q.Estimated}
				}
				v := sums[key]
				v.Count++
				v.Percent += q.Percent
				if q.Used != nil && q.Limit != nil {
					if v.Used == nil {
						u, l := 0.0, 0.0
						v.Used = &u
						v.Limit = &l
					}
					*v.Used += *q.Used
					*v.Limit += *q.Limit
				}
			}
		}
		for name, count := range plans {
			platform.Plans = append(platform.Plans, AccountReportPlan{Name: name, Count: count})
		}
		for _, q := range sums {
			if q.Limit != nil && *q.Limit > 0 {
				q.Percent = *q.Used / *q.Limit * 100
			} else {
				q.Percent /= float64(q.Count)
			}
			platform.Quotas = append(platform.Quotas, *q)
		}
		sort.Slice(platform.Plans, func(i, j int) bool { return platform.Plans[i].Name < platform.Plans[j].Name })
		sort.Slice(platform.Quotas, func(i, j int) bool {
			return platform.Quotas[i].Key+platform.Quotas[i].Plan+platform.Quotas[i].Model < platform.Quotas[j].Key+platform.Quotas[j].Plan+platform.Quotas[j].Model
		})
		r.Platforms = append(r.Platforms, platform)
	}
	return r
}

func newestReportQuota(quotas []AccountReportQuota) time.Time {
	var t time.Time
	for _, q := range quotas {
		if !q.Stale && q.UpdatedAt != nil && q.UpdatedAt.After(t) {
			t = *q.UpdatedAt
		}
	}
	return t
}
func reportStatusRank(status string) int {
	switch status {
	case "available":
		return 6
	case "limited":
		return 5
	case "exhausted":
		return 4
	case "error":
		return 3
	case "disabled":
		return 2
	default:
		return 0
	}
}

func reportQuotaStatus(platform, status string, quotas []AccountReportQuota) string {
	if status != "available" {
		return status
	}
	exhausted := map[string]bool{}
	modelCount, exhaustedModels := 0, 0
	for _, q := range quotas {
		if q.Key == "antigravity_model" && !q.Stale {
			modelCount++
			if q.Percent >= 100 {
				exhaustedModels++
			}
		}
		if !q.Stale && q.Percent >= 100 {
			exhausted[q.Key] = true
		}
	}
	if modelCount > 0 {
		if exhaustedModels == modelCount {
			return "exhausted"
		}
		if exhaustedModels > 0 {
			return "limited"
		}
		return status
	}
	if len(exhausted) == 0 {
		return status
	}
	if platform != "gemini" || exhausted["gemini_shared_daily"] || (exhausted["gemini_pro_daily"] && exhausted["gemini_flash_daily"]) {
		return "exhausted"
	}
	return "limited" // One model or RPM window can be exhausted while other models remain usable.
}
func reportCounts(rows []AccountReportRow) AccountReportCounts {
	c := AccountReportCounts{Total: len(rows)}
	for _, a := range rows {
		switch a.Status {
		case "available":
			c.Available++
		case "limited":
			c.Limited++
		case "error":
			c.Error++
		case "disabled":
			c.Disabled++
		case "exhausted":
			c.Exhausted++
		default:
			c.Unknown++
		}
		known, near := false, false
		for _, q := range a.Quotas {
			if !q.Stale {
				known = true
				if q.Percent >= 80 {
					near = true
				}
			}
		}
		if !known {
			c.UnknownQuota++
		}
		if near {
			c.NearLimit++
		}
		if a.IdentityUncertain {
			c.IdentityUncertain++
		}
	}
	return c
}
