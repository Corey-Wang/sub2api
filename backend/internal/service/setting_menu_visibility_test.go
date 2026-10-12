package service

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestNormalizeHiddenMenuItemsAllNavigationAndCustom(t *testing.T) {
	items := make([]string, 0, len(configurableMenuIDs)+1)
	for id := range configurableMenuIDs {
		items = append(items, id)
	}
	items = append(items, "custom:docs", "custom:docs", "settings")
	normalized, err := NormalizeHiddenMenuItems(items)
	require.NoError(t, err)
	require.Len(t, normalized, len(configurableMenuIDs)+1)
	require.Contains(t, normalized, "settings")
	require.Contains(t, normalized, "order_management")
	require.Contains(t, normalized, "orders_admin")
	require.Contains(t, normalized, "onboarding")
	require.ElementsMatch(t, []string{"subscriptions", "redeem"}, parseHiddenMenuItems(`["subscriptions","redeem"]`))
	for _, item := range []string{"unknown", "custom:", "custom:../settings", "custom:docs?x=1", "custom:abcdefghijklmnopqrstuvwxyz0123456789"} {
		_, err := NormalizeHiddenMenuItems([]string{item})
		require.Error(t, err, item)
	}
	_, err = NormalizeHiddenMenuItems(make([]string, 257))
	require.Error(t, err)
}
