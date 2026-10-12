//go:build unit

package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSettingsHiddenMenuItemsRoundTrip(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeySubscriptionEnabled: "true",
		service.SettingKeyPromoCodeEnabled:    "true",
	})
	want := []string{"subscriptions", "channels", "announcements", "redeem", "promo_codes"}
	rec := doUpdateSettings(t, h, map[string]any{"hidden_menu_items": want}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	assertResponse := func(rec *httptest.ResponseRecorder) {
		t.Helper()
		var body struct {
			Data struct {
				HiddenMenuItems []string `json:"hidden_menu_items"`
			} `json:"data"`
		}
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		require.Equal(t, want, body.Data.HiddenMenuItems)
	}
	assertResponse(rec)

	// The next settings page load and initial HTML must see the same selection.
	getRec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(getRec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/admin/settings", nil)
	h.GetSettings(c)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())
	assertResponse(getRec)
	public, err := h.settingService.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, public.HiddenMenuItems)
	injection, err := h.settingService.GetPublicSettingsForInjection(context.Background())
	require.NoError(t, err)
	raw, err := json.Marshal(injection)
	require.NoError(t, err)
	var injected struct {
		HiddenMenuItems []string `json:"hidden_menu_items"`
	}
	require.NoError(t, json.Unmarshal(raw, &injected))
	require.Equal(t, want, injected.HiddenMenuItems)
	require.Equal(t, "true", repo.values[service.SettingKeySubscriptionEnabled])
	require.Equal(t, "true", repo.values[service.SettingKeyPromoCodeEnabled])

	// Saving an unrelated setting must not show the hidden menus again.
	rec = doUpdateSettings(t, h, map[string]any{"site_name": "Example"}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertResponse(rec)

	// Explicitly clearing the selection restores every menu.
	want = []string{}
	rec = doUpdateSettings(t, h, map[string]any{"hidden_menu_items": want}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assertResponse(rec)
	require.JSONEq(t, "[]", repo.values[service.SettingKeyHiddenMenuItems])
}

func TestSettingsHiddenMenuItemsRejectUnknownEntries(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{
		service.SettingKeyHiddenMenuItems: `["redeem"]`,
	})
	rec := doUpdateSettings(t, h, map[string]any{
		"hidden_menu_items": []string{"unknown-menu"},
	}, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.JSONEq(t, `["redeem"]`, repo.values[service.SettingKeyHiddenMenuItems])
}

func TestSettingsExpandedHiddenMenusPersistAndClear(t *testing.T) {
	h, repo := newStepUpSwitchTestHandler(t, map[string]string{service.SettingKeyHiddenMenuItems: `["subscriptions","redeem"]`})
	want := []string{"settings", "accounts", "keys", "channel_pricing", "channel_monitor", "security_audit", "content_moderation", "order_management", "orders_admin", "profile", "onboarding", "docs", "logout", "custom:docs"}
	rec := doUpdateSettings(t, h, map[string]any{"hidden_menu_items": want}, nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var stored []string
	require.NoError(t, json.Unmarshal([]byte(repo.values[service.SettingKeyHiddenMenuItems]), &stored))
	require.Equal(t, want, stored)
	public, err := h.settingService.GetPublicSettings(context.Background())
	require.NoError(t, err)
	require.Equal(t, want, public.HiddenMenuItems)
	rec = doUpdateSettings(t, h, map[string]any{"hidden_menu_items": []string{}}, nil)
	require.Equal(t, http.StatusOK, rec.Code)
	require.JSONEq(t, "[]", repo.values[service.SettingKeyHiddenMenuItems])
}
