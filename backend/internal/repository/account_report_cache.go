package repository

import (
	"context"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/redis/go-redis/v9"
)

type accountReportCache struct{ redis redis.UniversalClient }

func NewAccountReportCache(client redis.UniversalClient) service.AccountReportCache {
	return &accountReportCache{redis: client}
}
func (c *accountReportCache) Load(ctx context.Context, id string) ([]byte, error) {
	return c.redis.Get(ctx, "account-report:"+id+":latest").Bytes()
}
func (c *accountReportCache) Save(ctx context.Context, id string, data []byte) error {
	return c.redis.Set(ctx, "account-report:"+id+":latest", data, 7*24*time.Hour).Err()
}
func (c *accountReportCache) Lock(ctx context.Context, id, token string, ttl time.Duration) (bool, error) {
	return c.redis.SetNX(ctx, "account-report:"+id+":lock", token, ttl).Result()
}
func (c *accountReportCache) Unlock(ctx context.Context, id, token string) error {
	return c.redis.Eval(ctx, `if redis.call('get', KEYS[1]) == ARGV[1] then return redis.call('del', KEYS[1]) else return 0 end`, []string{"account-report:" + id + ":lock"}, token).Err()
}
