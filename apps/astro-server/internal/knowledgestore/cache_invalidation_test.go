package knowledgestore

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

type generationRecordingCache struct {
	mu   sync.Mutex
	keys []string
}

func (*generationRecordingCache) Get(context.Context, string) ([]byte, bool) { return nil, false }
func (c *generationRecordingCache) Set(_ context.Context, key string, _ []byte, _ time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = append(c.keys, key)
	return nil
}
func (*generationRecordingCache) Invalidate(context.Context, string) error { return nil }

func TestSetStatusInvalidatesUserKnowledgePages(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	cache := &generationRecordingCache{}
	store := NewStore(db, cache)
	mock.ExpectQuery(`UPDATE knowledge_stores SET status`).
		WithArgs(StatusReady, "store-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct-1"))

	if err := store.SetStatus("store-1", StatusReady); err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.keys) != 1 || !strings.HasSuffix(cache.keys[0], "acct-1") {
		t.Fatalf("generation keys = %#v", cache.keys)
	}
}
