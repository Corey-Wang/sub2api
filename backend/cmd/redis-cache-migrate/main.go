package main

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strconv"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/repository"
	"github.com/redis/go-redis/v9"
)

func run() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if os.Getenv("REDIS_SOURCE_ADDR") == "" || os.Getenv("REDIS_HOST") == "" {
		return fmt.Errorf("REDIS_SOURCE_ADDR and REDIS_HOST are required")
	}
	port := 10000
	if value := os.Getenv("REDIS_PORT"); value != "" {
		var err error
		port, err = strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("REDIS_PORT must be between 1 and 65535")
		}
	}
	source := redis.NewClient(&redis.Options{Addr: os.Getenv("REDIS_SOURCE_ADDR"), Password: os.Getenv("REDIS_SOURCE_PASSWORD")})
	defer source.Close()
	cfg := &config.Config{Redis: config.RedisConfig{Host: os.Getenv("REDIS_HOST"), Port: port, Username: os.Getenv("REDIS_USERNAME"), Password: os.Getenv("REDIS_PASSWORD"), EnableTLS: true, ClusterEnabled: true, KeyPrefix: os.Getenv("REDIS_KEY_PREFIX"), DialTimeoutSeconds: 5, ReadTimeoutSeconds: 5, WriteTimeoutSeconds: 5, PoolSize: 8, MinIdleConns: 1}}
	if err := cfg.Redis.ValidateCluster(); err != nil {
		return err
	}
	target := repository.InitRedis(cfg)
	defer target.Close()
	if err := source.Ping(ctx).Err(); err != nil {
		return err
	}
	if err := target.Ping(ctx).Err(); err != nil {
		return err
	}
	iterator := source.Scan(ctx, 0, "*", 1000).Iterator()
	counts := map[string]int{}
	for iterator.Next(ctx) {
		key := iterator.Val()
		started := time.Now()
		ttl, err := source.PTTL(ctx, key).Result()
		if err != nil {
			return err
		}
		if ttl == -2*time.Nanosecond {
			continue
		}
		kind, err := source.Type(ctx, key).Result()
		if err != nil {
			return err
		}
		pipe := target.TxPipeline()
		pipe.Del(ctx, key)
		var expected interface{}
		switch kind {
		case "none":
			continue
		case "string":
			value, err := source.Get(ctx, key).Result()
			if err == redis.Nil {
				continue
			}
			if err != nil {
				return err
			}
			pipe.Set(ctx, key, value, 0)
			expected = value
		case "hash":
			value, err := source.HGetAll(ctx, key).Result()
			if err != nil {
				return err
			}
			if len(value) == 0 {
				continue
			}
			pipe.HSet(ctx, key, value)
			expected = value
		case "set":
			value, err := source.SMembers(ctx, key).Result()
			if err != nil {
				return err
			}
			if len(value) == 0 {
				continue
			}
			members := make([]interface{}, len(value))
			for i := range value {
				members[i] = value[i]
			}
			pipe.SAdd(ctx, key, members...)
			sort.Strings(value)
			expected = value
		case "zset":
			value, err := source.ZRangeWithScores(ctx, key, 0, -1).Result()
			if err != nil {
				return err
			}
			if len(value) == 0 {
				continue
			}
			pipe.ZAdd(ctx, key, value...)
			expected = value
		case "list":
			value, err := source.LRange(ctx, key, 0, -1).Result()
			if err != nil {
				return err
			}
			if len(value) == 0 {
				continue
			}
			members := make([]interface{}, len(value))
			for i := range value {
				members[i] = value[i]
			}
			pipe.RPush(ctx, key, members...)
			expected = value
		default:
			return fmt.Errorf("unsupported source cache type %s; migration stopped", kind)
		}
		if ttl > 0 {
			remaining := ttl - time.Since(started)
			if remaining <= 0 {
				continue
			}
			pipe.PExpire(ctx, key, remaining)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("copy %s cache: %w", kind, err)
		}
		var actual interface{}
		switch kind {
		case "string":
			actual, err = target.Get(ctx, key).Result()
		case "hash":
			actual, err = target.HGetAll(ctx, key).Result()
		case "set":
			var members []string
			members, err = target.SMembers(ctx, key).Result()
			sort.Strings(members)
			actual = members
		case "zset":
			actual, err = target.ZRangeWithScores(ctx, key, 0, -1).Result()
		case "list":
			actual, err = target.LRange(ctx, key, 0, -1).Result()
		}
		if err != nil {
			return fmt.Errorf("verify %s cache: %w", kind, err)
		}
		if !reflect.DeepEqual(expected, actual) {
			return fmt.Errorf("verify %s cache: value differs after copy", kind)
		}
		targetTTL, err := target.PTTL(ctx, key).Result()
		if err != nil {
			return err
		}
		if (ttl == -time.Nanosecond && targetTTL != ttl) || (ttl > 0 && (targetTTL <= 0 || targetTTL > ttl)) {
			return fmt.Errorf("verify %s cache: remaining TTL differs after copy", kind)
		}
		counts[kind]++
	}
	if err := iterator.Err(); err != nil {
		return err
	}
	fmt.Println("cache migration complete; every copied value and remaining TTL verified; copied by type:", counts)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
