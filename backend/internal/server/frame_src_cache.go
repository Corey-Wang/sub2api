package server

import (
	"context"
	"sync"
	"time"
)

const (
	frameSrcRefreshInterval = 5 * time.Second
	frameSrcRefreshTimeout  = 5 * time.Second
)

// Each process periodically rereads the shared settings. Local saves still
// refresh immediately; a failed read retains the last successful allowlist.
type frameSrcOriginCache struct {
	mu          sync.Mutex
	origins     []string
	nextRefresh time.Time
	load        func(context.Context) ([]string, error)
}

func newFrameSrcOriginCache(load func(context.Context) ([]string, error)) *frameSrcOriginCache {
	return &frameSrcOriginCache{load: load}
}

func (c *frameSrcOriginCache) Get() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !time.Now().Before(c.nextRefresh) {
		c.refreshLocked()
	}
	return c.origins
}

func (c *frameSrcOriginCache) Refresh() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.refreshLocked()
}

func (c *frameSrcOriginCache) refreshLocked() {
	ctx, cancel := context.WithTimeout(context.Background(), frameSrcRefreshTimeout)
	defer cancel()
	if origins, err := c.load(ctx); err == nil {
		c.origins = origins
	}
	// Bound retries during a database outage as well as successful reads.
	c.nextRefresh = time.Now().Add(frameSrcRefreshInterval)
}
