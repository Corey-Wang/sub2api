// Package redisnamespace keeps one deployment's atomic Redis operations in one
// Cluster slot while preserving the application's logical key names.
package redisnamespace

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	redis.UniversalClient
	prefix string
}

func Wrap(client redis.UniversalClient, prefix string) redis.UniversalClient {
	if prefix == "" {
		return client
	}
	client.AddHook(&namespaceHook{prefix: prefix})
	return &Client{UniversalClient: client, prefix: prefix}
}

func (c *Client) PhysicalKey(key string) string {
	if strings.HasPrefix(key, c.prefix) {
		return key
	}
	return c.prefix + key
}

// PhysicalKeyPrefix is for Lua arguments that are used to construct keys rather
// than passed in KEYS. Such arguments must share the namespace and hash tag.
func PhysicalKeyPrefix(client redis.UniversalClient, key string) string {
	if namespaced, ok := client.(interface{ PhysicalKey(string) string }); ok {
		return namespaced.PhysicalKey(key)
	}
	return key
}

func (c *Client) Scan(ctx context.Context, cursor uint64, match string, count int64) *redis.ScanCmd {
	if match == "" {
		match = "*"
	}
	return c.UniversalClient.Scan(ctx, cursor, match, count)
}

func (c *Client) channels(channels []string) []string {
	result := make([]string, len(channels))
	for i, channel := range channels {
		result[i] = c.PhysicalKey(channel)
	}
	return result
}

func (c *Client) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.UniversalClient.Subscribe(ctx, c.channels(channels)...)
}

func (c *Client) PSubscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.UniversalClient.PSubscribe(ctx, c.channels(channels)...)
}

func (c *Client) SSubscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.UniversalClient.SSubscribe(ctx, c.channels(channels)...)
}

func (c *Client) Watch(ctx context.Context, fn func(*redis.Tx) error, keys ...string) error {
	return c.UniversalClient.Watch(ctx, func(tx *redis.Tx) error {
		tx.AddHook(&namespaceHook{prefix: c.prefix})
		return fn(tx)
	}, c.channels(keys)...)
}

type namespaceHook struct {
	prefix string
}

func (h *namespaceHook) DialHook(next redis.DialHook) redis.DialHook { return next }

func (h *namespaceHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		if err := h.rewrite(cmd); err != nil {
			cmd.SetErr(err)
			return err
		}
		err := next(ctx, cmd)
		if err == nil {
			h.decodeKeys(cmd)
		}
		return err
	}
}

func (h *namespaceHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		for _, cmd := range cmds {
			if err := h.rewrite(cmd); err != nil {
				for _, pending := range cmds {
					pending.SetErr(err)
				}
				return err
			}
		}
		err := next(ctx, cmds)
		for _, cmd := range cmds {
			if cmd.Err() == nil {
				h.decodeKeys(cmd)
			}
		}
		return err
	}
}

func (h *namespaceHook) key(args []interface{}, pos int) error {
	if pos < 1 || pos >= len(args) {
		return fmt.Errorf("redis namespace: invalid key position")
	}
	var key string
	switch value := args[pos].(type) {
	case string:
		key = value
	case []byte:
		key = string(value)
	default:
		return fmt.Errorf("redis namespace: unsupported key type %T", value)
	}
	if !strings.HasPrefix(key, h.prefix) {
		args[pos] = h.prefix + key
	}
	return nil
}

func (h *namespaceHook) rewrite(cmd redis.Cmder) error {
	args := cmd.Args()
	first := 0
	positions := []int{}
	switch strings.ToLower(cmd.Name()) {
	case "ping", "auth", "hello", "select", "client", "command", "cluster", "info", "time", "script", "readonly", "readwrite", "asking", "echo", "quit", "multi", "exec", "discard", "unwatch", "pubsub":
		return nil
	case "eval", "evalsha", "eval_ro", "evalsha_ro":
		if len(args) < 3 {
			return fmt.Errorf("redis namespace: malformed Lua command")
		}
		count, err := strconv.Atoi(fmt.Sprint(args[2]))
		if err != nil || count < 0 || count > len(args)-3 {
			return fmt.Errorf("redis namespace: invalid Lua key count")
		}
		for i := 0; i < count; i++ {
			positions = append(positions, 3+i)
		}
	case "mget", "del", "unlink", "exists", "touch", "watch", "sdiff", "sinter", "sunion", "pfcount", "pfmerge":
		for i := 1; i < len(args); i++ {
			positions = append(positions, i)
		}
	case "mset", "msetnx":
		for i := 1; i < len(args); i += 2 {
			positions = append(positions, i)
		}
	case "rename", "renamenx", "rpoplpush", "brpoplpush", "smove", "copy":
		positions = []int{1, 2}
	case "scan":
		for i := 2; i+1 < len(args); i++ {
			if strings.EqualFold(fmt.Sprint(args[i]), "match") {
				positions = []int{i + 1}
				break
			}
		}
		if len(positions) == 0 {
			return fmt.Errorf("redis namespace: SCAN requires MATCH")
		}
	case "get", "getdel", "getex", "set", "setnx", "setex", "psetex", "getset", "expire", "pexpire", "expireat", "pexpireat", "persist", "ttl", "pttl", "type", "incr", "incrby", "incrbyfloat", "decr", "decrby", "append", "strlen", "dump", "restore", "restore-asking", "hget", "hgetall", "hset", "hsetnx", "hmget", "hmset", "hdel", "hexists", "hlen", "hkeys", "hvals", "hincrby", "hincrbyfloat", "hscan", "sadd", "srem", "smembers", "sismember", "smismember", "scard", "spop", "srandmember", "sscan", "zadd", "zrem", "zrange", "zrangebyscore", "zrevrange", "zrevrangebyscore", "zremrangebyscore", "zremrangebyrank", "zcard", "zcount", "zscore", "zmscore", "zrank", "zrevrank", "zincrby", "zscan", "lpush", "rpush", "lpop", "rpop", "llen", "lrange", "ltrim", "lrem", "lindex", "lset", "keys", "publish", "spublish":
		positions = []int{1}
	default:
		return fmt.Errorf("redis namespace: command %q is not supported", cmd.Name())
	}
	for _, pos := range positions {
		if err := h.key(args, pos); err != nil {
			return err
		}
		if first == 0 {
			first = pos
		}
	}
	if first != 0 {
		cmd.SetFirstKeyPos(int8(first))
	}
	return nil
}

func (h *namespaceHook) decodeKeys(cmd redis.Cmder) {
	switch strings.ToLower(cmd.Name()) {
	case "scan":
		if scan, ok := cmd.(*redis.ScanCmd); ok {
			keys, cursor := scan.Val()
			for i, key := range keys {
				keys[i] = strings.TrimPrefix(key, h.prefix)
			}
			scan.SetVal(keys, cursor)
		}
	case "keys":
		if keys, ok := cmd.(*redis.StringSliceCmd); ok {
			values := keys.Val()
			for i, key := range values {
				values[i] = strings.TrimPrefix(key, h.prefix)
			}
			keys.SetVal(values)
		}
	}
}
