package service

import (
	"encoding/json"
	"fmt"
	"regexp"
)

// These IDs cover built-in navigation and preserve the original five switches.
var configurableMenuIDs = map[string]struct{}{
	"account_report":       {},
	"onboarding":           {},
	"admin_dashboard":      {},
	"ops":                  {},
	"users":                {},
	"groups":               {},
	"channels":             {},
	"channel_pricing":      {},
	"channel_monitor":      {},
	"accounts":             {},
	"plugins":              {},
	"announcements":        {},
	"proxies":              {},
	"security_audit":       {},
	"content_moderation":   {},
	"prompt_audit":         {},
	"promo_codes":          {},
	"affiliate_management": {},
	"affiliate_invites":    {},
	"affiliate_rebates":    {},
	"affiliate_transfers":  {},
	"order_management":     {},
	"payment_dashboard":    {},
	"orders_admin":         {},
	"order_plans":          {},
	"admin_usage":          {},
	"audit_logs":           {},
	"settings":             {},
	"dashboard":            {},
	"keys":                 {},
	"batch_image":          {},
	"usage":                {},
	"available_channels":   {},
	"channel_status":       {},
	"purchase":             {},
	"orders":               {},
	"affiliate":            {},
	"profile":              {},
	"docs":                 {},
	"model_plaza":          {},
	"github":               {},
	"announcements_user":   {},
	"contact_support":      {},
	"language_switcher":    {},
	"logout":               {},
	"theme_switcher":       {},
	"sidebar_collapse":     {},
	"subscriptions":        {},
	"redeem":               {},
}
var customHiddenMenuID = regexp.MustCompile(`^custom:[a-zA-Z0-9_-]{1,32}$`)

// Visibility affects navigation only, never endpoint authorization.
func NormalizeHiddenMenuItems(items []string) ([]string, error) {
	if len(items) > 256 {
		return nil, fmt.Errorf("too many hidden menu items")
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if _, ok := configurableMenuIDs[item]; !ok && !customHiddenMenuID.MatchString(item) {
			return nil, fmt.Errorf("unsupported hidden menu item: %s", item)
		}
		if !seen[item] {
			result = append(result, item)
			seen[item] = true
		}
	}
	return result, nil
}
func parseHiddenMenuItems(raw string) []string {
	var items []string
	if json.Unmarshal([]byte(raw), &items) != nil {
		return []string{}
	}
	result, err := NormalizeHiddenMenuItems(items)
	if err != nil {
		return []string{}
	}
	return result
}
