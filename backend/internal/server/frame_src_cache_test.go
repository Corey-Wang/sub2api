//go:build unit

package server

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFrameSrcOriginCacheRefreshesOtherNodeAndRemovals(t *testing.T) {
	origins := []string{"https://old.example"}
	load := func(ctx context.Context) ([]string, error) {
		_, ok := ctx.Deadline()
		require.True(t, ok)
		return origins, nil
	}
	first, second := newFrameSrcOriginCache(load), newFrameSrcOriginCache(load)
	require.Equal(t, origins, first.Get())
	require.Equal(t, origins, second.Get())
	origins = []string{"http://20.243.186.104:9489"}
	first.Refresh()
	require.Equal(t, origins, first.Get())
	require.Equal(t, []string{"https://old.example"}, second.Get())
	second.nextRefresh = time.Now().Add(-time.Second)
	require.Equal(t, origins, second.Get())
	origins = nil
	second.nextRefresh = time.Now().Add(-time.Second)
	require.Empty(t, second.Get())
}

func TestFrameSrcOriginCacheRetainsLastSuccessAndBoundsRetries(t *testing.T) {
	calls := 0
	fail := false
	cache := newFrameSrcOriginCache(func(context.Context) ([]string, error) {
		calls++
		if fail {
			return nil, errors.New("database unavailable")
		}
		return []string{"https://allowed.example"}, nil
	})
	require.Equal(t, []string{"https://allowed.example"}, cache.Get())
	fail = true
	cache.nextRefresh = time.Now().Add(-time.Second)
	require.Equal(t, []string{"https://allowed.example"}, cache.Get())
	require.Equal(t, []string{"https://allowed.example"}, cache.Get())
	require.Equal(t, 2, calls)
}

func TestFrameSrcOriginCacheCoalescesConcurrentRefreshes(t *testing.T) {
	calls := 0
	cache := newFrameSrcOriginCache(func(context.Context) ([]string, error) {
		calls++
		return []string{"https://allowed.example"}, nil
	})
	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() { cache.Get() })
	}
	wg.Wait()
	require.Equal(t, 1, calls)
}
