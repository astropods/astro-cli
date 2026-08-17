//go:build integration

// Integration coverage for webhook job deduplication against a real River and a
// real Postgres. CI job: `Integration tests (astro-server + Postgres)` in
// test.yml.
//
// Providers redeliver. Stripe repeats an event until it sees a 2xx, and a
// redelivered gating event applied twice is a suspension applied twice. The
// defence is River's unique-by-args index on the event ID, and every existing
// test of it asserts the InsertOpts struct rather than inserting two jobs and
// watching one disappear. A struct assertion cannot tell whether the index
// exists, whether River consults it, or which fields it covers.
package e2e

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/riverqueue"
	_ "github.com/lib/pq"
)

func dedupeQueue(t *testing.T) *riverqueue.Queue {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Fatal("DATABASE_URL must be set for integration tests")
	}
	q, err := riverqueue.NewInsertOnly(context.Background(), dsn, logger.New("error", "json"), true)
	if err != nil {
		t.Fatalf("insert-only queue: %v", err)
	}
	t.Cleanup(q.Close)
	return q
}

// countJobs reports how many River jobs carry this event ID, which is the
// question the dedupe exists to answer. It reads the stored args rather than
// River's own bookkeeping, so a unique index that silently stopped applying
// shows up as a second row.
func countJobs(t *testing.T, db *sql.DB, kind, eventID string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM river.river_job WHERE kind = $1 AND args->>'event_id' = $2`,
		kind, eventID,
	).Scan(&n); err != nil {
		t.Fatalf("count %s jobs: %v", kind, err)
	}
	return n
}

func cleanupJobs(t *testing.T, db *sql.DB, eventIDs ...string) {
	t.Helper()
	t.Cleanup(func() {
		for _, id := range eventIDs {
			_, _ = db.Exec(`DELETE FROM river.river_job WHERE args->>'event_id' = $1`, id)
		}
	})
}

// A Stripe redelivery must produce one job, not two. The second insert also
// carries a different hosted invoice URL, because only the event ID is tagged
// `river:"unique"`: a redelivery whose sibling fields drift must still collapse,
// or the tag is decoration and every field has to match for the guard to hold.
func TestWebhookDedupe_StripeRedeliveryCollapses(t *testing.T) {
	db := testDB(t)
	q := dedupeQueue(t)
	ctx := context.Background()
	const eventID = "evt_dedupe_stripe_e2e"
	cleanupJobs(t, db, eventID)

	if err := q.InsertStripeWebhook(ctx, eventID, "invoice.payment_failed", "cus_1", "https://invoice.stripe.com/first"); err != nil {
		t.Fatalf("first insert: %v", err)
	}
	if err := q.InsertStripeWebhook(ctx, eventID, "invoice.payment_failed", "cus_1", "https://invoice.stripe.com/second"); err != nil {
		t.Fatalf("redelivery insert: %v", err)
	}

	if n := countJobs(t, db, "webhook.stripe", eventID); n != 1 {
		t.Fatalf("stored %d jobs for one event, want 1: the redelivery would apply the signal twice", n)
	}
}

// The guard must not be so wide that it swallows real events. Two distinct
// failures on one customer are two suspensible facts.
func TestWebhookDedupe_ADifferentEventStillEnqueues(t *testing.T) {
	db := testDB(t)
	q := dedupeQueue(t)
	ctx := context.Background()
	const first, second = "evt_dedupe_distinct_a_e2e", "evt_dedupe_distinct_b_e2e"
	cleanupJobs(t, db, first, second)

	for _, id := range []string{first, second} {
		if err := q.InsertStripeWebhook(ctx, id, "invoice.payment_failed", "cus_1", ""); err != nil {
			t.Fatalf("insert %s: %v", id, err)
		}
	}

	for _, id := range []string{first, second} {
		if n := countJobs(t, db, "webhook.stripe", id); n != 1 {
			t.Errorf("event %s stored %d jobs, want 1", id, n)
		}
	}
}

// Metronome carries the gating signal for spend and credit, and it redelivers
// on the same terms. The two webhook kinds share webhookInsertOpts, so this
// proves the shared helper applies to both rather than only to the one the
// Stripe case exercised.
func TestWebhookDedupe_MetronomeRedeliveryCollapses(t *testing.T) {
	db := testDB(t)
	q := dedupeQueue(t)
	ctx := context.Background()
	const eventID = "evt_dedupe_metronome_e2e"
	cleanupJobs(t, db, eventID)

	// CurrentSpend differs per attempt. ByArgs hashes only the river:"unique"
	// EventID, so a redelivery whose siblings moved must still collapse: whole-args
	// equality would let a recalculated amount through as a second gating job.
	for i := 0; i < 3; i++ {
		if err := q.InsertMetronomeWebhook(ctx, riverqueue.MetronomeWebhookArgs{
			EventID:      eventID,
			EventType:    "alerts.spend_threshold_reached",
			CustomerID:   "cus_1",
			AlertName:    "Spend limit",
			Threshold:    8000,
			CurrentSpend: int64(8100 + i),
		}); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	if n := countJobs(t, db, "webhook.metronome", eventID); n != 1 {
		t.Fatalf("stored %d jobs for one event, want 1", n)
	}
}

// A webhook with no event ID cannot be deduped, and webhookInsertOpts leaves the
// unique options off for that case. Enqueueing both is the intended outcome:
// dropping the second would silently discard a real event on the theory that it
// matched something.
func TestWebhookDedupe_NoEventIDEnqueuesEach(t *testing.T) {
	db := testDB(t)
	q := dedupeQueue(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = db.Exec(`DELETE FROM river.river_job WHERE kind = 'webhook.stripe' AND args->>'customer_id' = 'cus_dedupe_noid_e2e'`)
	})

	for i := 0; i < 2; i++ {
		if err := q.InsertStripeWebhook(ctx, "", "invoice.payment_failed", "cus_dedupe_noid_e2e", ""); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}

	var n int
	if err := db.QueryRow(
		`SELECT count(*) FROM river.river_job WHERE kind = 'webhook.stripe' AND args->>'customer_id' = 'cus_dedupe_noid_e2e'`,
	).Scan(&n); err != nil {
		t.Fatalf("count jobs: %v", err)
	}
	if n != 2 {
		t.Fatalf("stored %d jobs, want 2: an unidentified event must not be dropped", n)
	}
}
