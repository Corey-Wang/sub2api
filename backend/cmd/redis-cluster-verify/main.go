package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if os.Getenv("REDIS_HOST") == "" {
		return fmt.Errorf("REDIS_HOST is required")
	}
	port := 10000
	if value := os.Getenv("REDIS_PORT"); value != "" {
		var err error
		port, err = strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("REDIS_PORT must be between 1 and 65535")
		}
	}
	prefix := fmt.Sprintf("{sub2api-green-verify-%d}:", time.Now().UnixNano())
	cfg := &config.Config{Redis: config.RedisConfig{Host: os.Getenv("REDIS_HOST"), Port: port, Username: os.Getenv("REDIS_USERNAME"), Password: os.Getenv("REDIS_PASSWORD"), EnableTLS: true, ClusterEnabled: true, KeyPrefix: prefix, DialTimeoutSeconds: 5, ReadTimeoutSeconds: 5, WriteTimeoutSeconds: 5, PoolSize: 8, MinIdleConns: 1}}
	a := repository.InitRedis(cfg)
	b := repository.InitRedis(cfg)
	defer a.Close()
	defer b.Close()
	defer func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		iterator := a.Scan(cleanupCtx, 0, "*", 1000).Iterator()
		var keys []string
		for iterator.Next(cleanupCtx) {
			keys = append(keys, iterator.Val())
		}
		if len(keys) > 0 {
			_ = a.Del(cleanupCtx, keys...).Err()
		}
		fmt.Println("isolated verification keys cleaned; prefix:", prefix)
	}()
	if err := a.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("cluster TLS/PING: %w", err)
	}
	pipe := a.TxPipeline()
	pipe.Set(ctx, "left", "A", time.Minute)
	pipe.Set(ctx, "right", "B", time.Minute)
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("transaction: %w", err)
	}
	values, err := b.MGet(ctx, "left", "right").Result()
	if err != nil || len(values) != 2 || values[0] != "A" || values[1] != "B" {
		return fmt.Errorf("cross-client MGET: %v", err)
	}
	fmt.Println("cluster TLS, transaction, cross-client MGET: PASS")
	script := redis.NewScript(`return {redis.call('GET',KEYS[1]),redis.call('GET',KEYS[2])}`)
	for i := 0; i < 2; i++ {
		if _, err := script.Run(ctx, a, []string{"left", "right"}).Result(); err != nil {
			return fmt.Errorf("multi-key EVAL/EVALSHA: %w", err)
		}
	}
	fmt.Println("multi-key Lua EVAL and EVALSHA: PASS")
	keys, _, err := b.Scan(ctx, 0, "*", 1000).Result()
	if err != nil || len(keys) < 2 {
		return fmt.Errorf("slot-routed SCAN: count=%d err=%v", len(keys), err)
	}
	for _, key := range keys {
		if key == prefix+"left" {
			return fmt.Errorf("SCAN leaked physical key")
		}
	}
	fmt.Println("slot-routed SCAN with logical keys: PASS")
	sub := a.Subscribe(ctx, "verify-updates")
	defer sub.Close()
	if _, err := sub.Receive(ctx); err != nil {
		return fmt.Errorf("SUBSCRIBE: %w", err)
	}
	if err := b.Publish(ctx, "verify-updates", "invalidate").Err(); err != nil {
		return err
	}
	message, err := sub.ReceiveMessage(ctx)
	if err != nil || message.Payload != "invalidate" {
		return fmt.Errorf("cross-client PubSub: %v", err)
	}
	fmt.Println("cross-client cache invalidation PubSub: PASS")
	concurrency := repository.NewConcurrencyCache(a, 1, 60)
	acquired, err := concurrency.AcquireAccountSlot(ctx, 501, 1, "request-a")
	if err != nil || !acquired {
		return fmt.Errorf("acquire slot: %v", err)
	}
	peer := repository.NewConcurrencyCache(b, 1, 60)
	acquired, err = peer.AcquireAccountSlot(ctx, 501, 1, "request-b")
	if err != nil || acquired {
		return fmt.Errorf("shared concurrency bound failed: %v", err)
	}
	if err := concurrency.ReleaseAccountSlot(ctx, 501, "request-a"); err != nil {
		return err
	}
	fmt.Println("actual application concurrency Lua and shared limit: PASS")
	scheduler := repository.NewSchedulerCache(a)
	bucket := service.SchedulerBucket{GroupID: 501, Platform: "openai", Mode: service.SchedulerModeSingle}
	token, err := scheduler.CaptureBucketWriteToken(ctx, bucket)
	if err != nil {
		return err
	}
	accounts := []service.Account{{ID: 501, Name: "verification-only", Platform: "openai", Type: "apikey", Status: "active", Schedulable: true}}
	if err := scheduler.SetSnapshot(ctx, bucket, token, accounts); err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	if err := scheduler.SetSnapshot(ctx, bucket, token, accounts); err != nil {
		return fmt.Errorf("snapshot replacement with derived keys: %w", err)
	}
	_, ready, err := repository.NewSchedulerCache(b).GetSnapshot(ctx, bucket)
	if err != nil || !ready {
		return fmt.Errorf("peer snapshot: ready=%v err=%v", ready, err)
	}
	if err := scheduler.RetireBucket(ctx, bucket); err != nil {
		return fmt.Errorf("retire bucket: %w", err)
	}
	if _, err := scheduler.ReopenBucket(ctx, bucket); err != nil {
		return fmt.Errorf("reopen bucket: %w", err)
	}
	fmt.Println("actual application snapshot activation/retirement Lua: PASS")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
