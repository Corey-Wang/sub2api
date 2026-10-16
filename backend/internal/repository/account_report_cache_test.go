package repository

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestAccountReportCacheLeaseOwnershipAndSharedSnapshot(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer client.Close()
	a, b := NewAccountReportCache(client), NewAccountReportCache(client)
	ctx := context.Background()
	locked, err := a.Lock(ctx, "fixture", "owner-a", time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
	locked, err = b.Lock(ctx, "fixture", "owner-b", time.Minute)
	require.NoError(t, err)
	require.False(t, locked)
	require.NoError(t, b.Unlock(ctx, "fixture", "owner-b"))
	locked, err = b.Lock(ctx, "fixture", "owner-b", time.Minute)
	require.NoError(t, err)
	require.False(t, locked, "a different owner must not release the collection lease")
	require.NoError(t, a.Save(ctx, "fixture", []byte(`{"accounts":[]}`)))
	data, err := b.Load(ctx, "fixture")
	require.NoError(t, err)
	require.JSONEq(t, `{"accounts":[]}`, string(data))
	mr.FastForward(time.Minute)
	locked, err = b.Lock(ctx, "fixture", "owner-b", time.Minute)
	require.NoError(t, err)
	require.True(t, locked)
	require.NoError(t, a.Unlock(ctx, "fixture", "owner-a"))
	locked, err = a.Lock(ctx, "fixture", "owner-c", time.Minute)
	require.NoError(t, err)
	require.False(t, locked)
}
