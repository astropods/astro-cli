package listcache

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
)

func TestCacheKeepsRedisFreeEnvironmentsHot(t *testing.T) {
	cache := New(k8scache.NoopCache{}, "test:", time.Minute, time.Minute, 4)
	var loads atomic.Int32
	load := func(context.Context) (LoadResult, error) {
		loads.Add(1)
		return LoadResult{
			Response:        Response{Data: []byte(`{"ok":true}`), ResultCount: 7, NextCursorPresent: true},
			RemoteCacheable: true,
		}, nil
	}

	first, source, err := cache.GetOrLoad(context.Background(), "key", load)
	if err != nil || source != "miss" || string(first.Data) != `{"ok":true}` {
		t.Fatalf("first = %s, %q, %v", first.Data, source, err)
	}
	second, source, err := cache.GetOrLoad(context.Background(), "key", load)
	if err != nil || source != "l1" || string(second.Data) != string(first.Data) {
		t.Fatalf("second = %s, %q, %v", second.Data, source, err)
	}
	if second.ResultCount != 7 || !second.NextCursorPresent {
		t.Fatalf("second metadata = (%d, %t), want (7, true)", second.ResultCount, second.NextCursorPresent)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
}

func TestCacheBoundsLocalEntries(t *testing.T) {
	cache := New(nil, "test:", time.Minute, time.Minute, 2)
	for _, key := range []string{"one", "two", "three"} {
		_, _, err := cache.GetOrLoad(context.Background(), key, func(context.Context) (LoadResult, error) {
			return LoadResult{Response: Response{Data: []byte(key)}, RemoteCacheable: true}, nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.entries) != 2 {
		t.Fatalf("local entries = %d, want 2", len(cache.entries))
	}
}

type recordingRemote struct {
	mu   sync.Mutex
	data map[string][]byte
	sets atomic.Int32
}

type observingContext struct {
	context.Context
	entered chan struct{}
	once    sync.Once
}

func (c *observingContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.entered) })
	return c.Context.Done()
}

func (c *recordingRemote) Get(_ context.Context, key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, ok := c.data[key]
	return append([]byte(nil), value...), ok
}
func (c *recordingRemote) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = make(map[string][]byte)
	}
	c.data[key] = append([]byte(nil), value...)
	c.sets.Add(1)
	return nil
}
func (c *recordingRemote) Invalidate(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, key)
	return nil
}

func TestCacheKeepsDegradedLoadsOutOfRemoteTier(t *testing.T) {
	remote := &recordingRemote{}
	cache := New(remote, "test:", time.Minute, time.Minute, 4)
	var loads atomic.Int32
	load := func(context.Context) (LoadResult, error) {
		loads.Add(1)
		return LoadResult{Response: Response{Data: []byte(`{"partial":true}`)}, RemoteCacheable: false}, nil
	}

	first, source, err := cache.GetOrLoad(context.Background(), "key", load)
	if err != nil || source != "miss" || string(first.Data) != `{"partial":true}` {
		t.Fatalf("first = %s, %q, %v", first.Data, source, err)
	}
	second, source, err := cache.GetOrLoad(context.Background(), "key", load)
	if err != nil || source != "l1" || string(second.Data) != string(first.Data) {
		t.Fatalf("second = %s, %q, %v", second.Data, source, err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want 1", loads.Load())
	}
	if remote.sets.Load() != 0 {
		t.Fatalf("remote sets = %d, want 0", remote.sets.Load())
	}
}

func TestCacheCarriesTelemetryThroughRemoteTier(t *testing.T) {
	remote := &recordingRemote{data: make(map[string][]byte)}
	warm := New(remote, "test:", 0, time.Minute, 0)
	want := Response{Data: []byte(`{"items":[1,2,3]}`), ResultCount: 3, NextCursorPresent: true}
	got, source, err := warm.GetOrLoad(context.Background(), "key", func(context.Context) (LoadResult, error) {
		return LoadResult{Response: want, RemoteCacheable: true}, nil
	})
	if err != nil || source != "miss" || string(got.Data) != string(want.Data) {
		t.Fatalf("warm = %#v, %q, %v", got, source, err)
	}

	cold := New(remote, "test:", 0, time.Minute, 0)
	got, source, err = cold.GetOrLoad(context.Background(), "key", func(context.Context) (LoadResult, error) {
		t.Fatal("remote hit invoked loader")
		return LoadResult{}, nil
	})
	if err != nil || source != "l2" {
		t.Fatalf("cold source = %q, err = %v", source, err)
	}
	if string(got.Data) != string(want.Data) || got.ResultCount != 3 || !got.NextCursorPresent {
		t.Fatalf("cold response = %#v, want %#v", got, want)
	}
}

func TestCacheCallerCancellationDoesNotPoisonSharedLoad(t *testing.T) {
	cache := New(nil, "test:", time.Minute, time.Minute, 4)
	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	load := func(ctx context.Context) (LoadResult, error) {
		loads.Add(1)
		close(started)
		select {
		case <-release:
			return LoadResult{Response: Response{Data: []byte(`{"ok":true}`)}}, nil
		case <-ctx.Done():
			return LoadResult{}, ctx.Err()
		}
	}

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, _, err := cache.GetOrLoad(leaderCtx, "key", load)
		leaderDone <- err
	}()
	<-started

	waiterCtx := &observingContext{Context: context.Background(), entered: make(chan struct{})}
	waiterDone := make(chan struct {
		response Response
		err      error
	}, 1)
	go func() {
		response, _, err := cache.GetOrLoad(waiterCtx, "key", load)
		waiterDone <- struct {
			response Response
			err      error
		}{response: response, err: err}
	}()

	<-waiterCtx.entered
	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context canceled", err)
	}
	close(release)
	result := <-waiterDone
	if result.err != nil || string(result.response.Data) != `{"ok":true}` {
		t.Fatalf("waiter = %s, %v", result.response.Data, result.err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads = %d, want one shared load", loads.Load())
	}
}

func TestCacheBoundsDetachedLoadDuration(t *testing.T) {
	cache := New(nil, "test:", time.Minute, time.Minute, 4)
	cache.loadTimeout = 20 * time.Millisecond
	startedAt := time.Now()
	_, _, err := cache.GetOrLoad(context.Background(), "key", func(ctx context.Context) (LoadResult, error) {
		<-ctx.Done()
		return LoadResult{}, ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(startedAt); elapsed > time.Second {
		t.Fatalf("detached load took %s, want a bounded wait", elapsed)
	}
}
