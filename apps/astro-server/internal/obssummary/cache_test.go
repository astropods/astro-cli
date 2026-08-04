package obssummary

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

type recordingBatchCache struct {
	values       map[string][]byte
	getManyCalls int
}

func (c *recordingBatchCache) Get(_ context.Context, key string) ([]byte, bool) {
	value, ok := c.values[key]
	return value, ok
}

func (c *recordingBatchCache) GetMany(_ context.Context, keys []string) map[string][]byte {
	c.getManyCalls++
	values := make(map[string][]byte, len(keys))
	for _, key := range keys {
		if value, ok := c.values[key]; ok {
			values[key] = value
		}
	}
	return values
}

func (c *recordingBatchCache) Set(_ context.Context, key string, data []byte, _ time.Duration) error {
	c.values[key] = data
	return nil
}

func (c *recordingBatchCache) Invalidate(_ context.Context, key string) error {
	delete(c.values, key)
	return nil
}

func TestGetManyUsesOneBatchReadAndKeepsHealthyEntries(t *testing.T) {
	healthy, err := json.Marshal(&Entry{TotalTraces: 7})
	if err != nil {
		t.Fatalf("marshal entry: %v", err)
	}
	cache := &recordingBatchCache{values: map[string][]byte{
		KeyFor("dep-a"): healthy,
		KeyFor("dep-b"): []byte("not-json"),
	}}

	entries, err := GetMany(context.Background(), cache, []string{"dep-a", "dep-b", "dep-c"})
	if err == nil {
		t.Fatal("expected malformed dep-b to be reported")
	}
	if cache.getManyCalls != 1 {
		t.Fatalf("GetMany calls = %d, want 1", cache.getManyCalls)
	}
	if len(entries) != 1 || entries["dep-a"].TotalTraces != 7 {
		t.Fatalf("entries = %#v, want only healthy dep-a", entries)
	}
}
