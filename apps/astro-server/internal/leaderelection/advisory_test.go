package leaderelection

import (
	"context"
	"database/sql"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	_ "github.com/lib/pq"
)

func TestKey(t *testing.T) {
	a, b := Key("deploy-controller"), Key("deploy-controller")
	if a != b {
		t.Fatal("Key is not deterministic")
	}
	if Key("deploy-controller") == Key("something-else") {
		t.Fatal("distinct names collided on the same key")
	}
}

func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatalf("ping database: %v", err)
	}
	return db
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// leadershipTracker records how many onElected callbacks are concurrently
// active and the peak, so a test can assert the lock is mutually exclusive.
type leadershipTracker struct {
	active atomic.Int32
	peak   atomic.Int32
	wins   atomic.Int32
}

func (lt *leadershipTracker) onElected(ctx context.Context) {
	lt.wins.Add(1)
	n := lt.active.Add(1)
	for {
		p := lt.peak.Load()
		if n <= p || lt.peak.CompareAndSwap(p, n) {
			break
		}
	}
	<-ctx.Done()
	lt.active.Add(-1)
}

// TestSingleLeaderAndFailover starts two electors on the same lock key and
// asserts (a) only one leads at a time and (b) when the leader steps down the
// standby takes over.
func TestSingleLeaderAndFailover(t *testing.T) {
	db := testDB(t)
	cfg := Config{
		LockKey:           Key("test-" + t.Name()),
		RetryInterval:     50 * time.Millisecond,
		HeartbeatInterval: 50 * time.Millisecond,
		Name:              "test",
		Logger:            logger.New("error", "text"),
	}
	tr := &leadershipTracker{}

	ctxA, cancelA := context.WithCancel(context.Background())
	doneA := make(chan struct{})
	go func() { defer close(doneA); Run(ctxA, db, cfg, tr.onElected) }()

	waitFor(t, "A to lead", func() bool { return tr.active.Load() == 1 })

	ctxB, cancelB := context.WithCancel(context.Background())
	doneB := make(chan struct{})
	go func() { defer close(doneB); Run(ctxB, db, cfg, tr.onElected) }()

	// Give B ample time to (wrongly) grab the lock; it must not.
	time.Sleep(300 * time.Millisecond)
	if got := tr.active.Load(); got != 1 {
		t.Fatalf("expected exactly 1 leader while A holds the lock, got %d", got)
	}
	if tr.wins.Load() != 1 {
		t.Fatalf("B acquired leadership while A still held it (wins=%d)", tr.wins.Load())
	}

	// A steps down -> B must take over.
	cancelA()
	<-doneA
	waitFor(t, "B to take over", func() bool { return tr.wins.Load() == 2 && tr.active.Load() == 1 })

	cancelB()
	<-doneB

	if tr.peak.Load() > 1 {
		t.Fatalf("two leaders were active simultaneously (peak=%d)", tr.peak.Load())
	}
	if tr.active.Load() != 0 {
		t.Fatalf("a leader is still active after shutdown (active=%d)", tr.active.Load())
	}
}
