package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadRedisClusterConfig(t *testing.T) {
	t.Run("standalone defaults", func(t *testing.T) {
		resetViperWithJWTSecret(t)
		t.Setenv("REDIS_DB", "2")
		cfg, err := Load()
		require.NoError(t, err)
		require.False(t, cfg.Redis.ClusterEnabled)
		require.Empty(t, cfg.Redis.KeyPrefix)
		require.Equal(t, 2, cfg.Redis.DB)
	})
	t.Run("cluster environment configuration", func(t *testing.T) {
		resetViperWithJWTSecret(t)
		t.Setenv("REDIS_CLUSTER_ENABLED", "true")
		t.Setenv("REDIS_KEY_PREFIX", "{trial-app}:")
		t.Setenv("REDIS_DB", "0")
		t.Setenv("REDIS_ENABLE_TLS", "true")
		cfg, err := Load()
		require.NoError(t, err)
		require.True(t, cfg.Redis.ClusterEnabled)
		require.True(t, cfg.Redis.EnableTLS)
		require.Equal(t, "{trial-app}:", cfg.Redis.KeyPrefix)
	})
	for _, tc := range []struct{ name, prefix, db, message string }{
		{"missing namespace", "", "0", "nonempty leading hash tag"},
		{"untagged namespace", "app:", "0", "nonempty leading hash tag"},
		{"empty hash tag", "{}:", "0", "nonempty leading hash tag"},
		{"unclosed hash tag", "{app:", "0", "nonempty leading hash tag"},
		{"nonzero database", "{app}:", "1", "DB 0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetViperWithJWTSecret(t)
			t.Setenv("REDIS_CLUSTER_ENABLED", "true")
			t.Setenv("REDIS_KEY_PREFIX", tc.prefix)
			t.Setenv("REDIS_DB", tc.db)
			_, err := Load()
			require.ErrorContains(t, err, tc.message)
		})
	}
}
