package redisnamespace

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

func TestLogicalKeysValuesAndScan(t *testing.T) {
	server := miniredis.RunT(t)
	raw := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = raw.Close() })
	client := Wrap(raw, "{test-deployment}:")
	ctx := context.Background()
	require.NoError(t, client.Set(ctx, "plain", "value:{must-not-change}", time.Minute).Err())
	require.Equal(t, []string{"{test-deployment}:plain"}, server.Keys())
	value, err := client.Get(ctx, "plain").Result()
	require.NoError(t, err)
	require.Equal(t, "value:{must-not-change}", value)
	keys, _, err := client.Scan(ctx, 0, "", 100).Result()
	require.NoError(t, err)
	require.Equal(t, []string{"plain"}, keys)
	require.NoError(t, client.Set(ctx, "{test-deployment}:plain", "updated", 0).Err())
	require.Equal(t, []string{"{test-deployment}:plain"}, server.Keys())
}

func TestMultiKeyLuaAndTransactions(t *testing.T) {
	server := miniredis.RunT(t)
	raw := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = raw.Close() })
	client := Wrap(raw, "{test-deployment}:")
	ctx := context.Background()
	pipe := client.TxPipeline()
	pipe.Set(ctx, "left", "A", time.Minute)
	pipe.Set(ctx, "right", "B", time.Minute)
	_, err := pipe.Exec(ctx)
	require.NoError(t, err)
	script := redis.NewScript(`return {redis.call('GET',KEYS[1]),redis.call('GET',KEYS[2])}`)
	result, err := script.Run(ctx, client, []string{"left", "right"}).Slice()
	require.NoError(t, err)
	require.Equal(t, []interface{}{"A", "B"}, result)
	// Repeat to exercise EVALSHA after the first EVAL fallback.
	_, err = script.Run(ctx, client, []string{"left", "right"}).Result()
	require.NoError(t, err)
	values, err := client.MGet(ctx, "left", "right").Result()
	require.NoError(t, err)
	require.Equal(t, []interface{}{"A", "B"}, values)
	require.NoError(t, client.Watch(ctx, func(tx *redis.Tx) error {
		v, err := tx.Get(ctx, "left").Result()
		require.NoError(t, err)
		require.Equal(t, "A", v)
		_, err = tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			pipe.Set(ctx, "left", "C", 0)
			return nil
		})
		return err
	}, "left"))
	v, err := client.Get(ctx, "left").Result()
	require.NoError(t, err)
	require.Equal(t, "C", v)
}

func TestNamespacedPubSub(t *testing.T) {
	server := miniredis.RunT(t)
	a := Wrap(redis.NewClient(&redis.Options{Addr: server.Addr()}), "{test-deployment}:")
	b := Wrap(redis.NewClient(&redis.Options{Addr: server.Addr()}), "{test-deployment}:")
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	ctx := context.Background()
	sub := a.Subscribe(ctx, "updates")
	t.Cleanup(func() { _ = sub.Close() })
	_, err := sub.Receive(ctx)
	require.NoError(t, err)
	require.NoError(t, b.Publish(ctx, "updates", "invalidate").Err())
	message, err := sub.ReceiveMessage(ctx)
	require.NoError(t, err)
	require.Equal(t, "invalidate", message.Payload)
}
