package pgnotify

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	_ "github.com/lib/pq"
)

// TestNotifyListen round-trips a payload: a Listen subscriber receives what a
// Notify publisher sends. NOTIFY is lost if it fires before LISTEN is
// established, so the publish is retried until received or the deadline hits.
func TestNotifyListen(t *testing.T) {
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const channel = "test_pgnotify_roundtrip"
	got := make(chan string, 8)
	go Listen(ctx, dsn, channel, logger.New("error", "text"), func(p string) { got <- p })

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := Notify(ctx, db, channel, "ns-1"); err != nil {
			t.Fatalf("notify: %v", err)
		}
		select {
		case p := <-got:
			if p != "ns-1" {
				t.Fatalf("got payload %q, want %q", p, "ns-1")
			}
			return
		case <-time.After(200 * time.Millisecond):
			// Listener may not have subscribed yet; retry the publish.
		}
	}
	t.Fatal("timed out waiting for notification")
}
