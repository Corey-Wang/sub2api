//go:build embed

package web

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestHTMLCacheExpiresWithoutLocalSettingsUpdate(t *testing.T) {
	cache := NewHTMLCache()
	cache.SetBaseHTML([]byte("base"))
	cache.Set([]byte("old settings"), []byte(`{"site_name":"old"}`))
	require.NotNil(t, cache.Get())
	cache.expiresAt = time.Now().Add(-time.Second)
	require.Nil(t, cache.Get(), "another node's settings must not remain cached forever")
	cache.Set([]byte("new settings"), []byte(`{"site_name":"new"}`))
	require.Equal(t, []byte("new settings"), cache.Get().Content)
}
