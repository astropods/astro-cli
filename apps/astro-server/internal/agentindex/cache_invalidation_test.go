package agentindex

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

func TestArchiveInvalidatesUserBlueprintPages(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck
	cache := &generationRecordingCache{}
	index := NewIndexWithDB(db).WithCache(cache)
	mock.ExpectBegin()
	mock.ExpectQuery(`UPDATE agents SET archived_at`).
		WithArgs(sqlmock.AnyArg(), "acct-1", "agent-a").
		WillReturnRows(sqlmock.NewRows([]string{"uid"}).AddRow("11111111-1111-1111-1111-111111111111"))
	mock.ExpectCommit()

	if err := index.Archive("acct-1", "agent-a"); err != nil {
		t.Fatal(err)
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.keys) != 1 || !strings.HasSuffix(cache.keys[0], "acct-1") {
		t.Fatalf("generation keys = %#v", cache.keys)
	}
}
