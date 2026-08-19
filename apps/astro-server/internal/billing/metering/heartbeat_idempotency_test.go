package metering

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	_ "modernc.org/sqlite"
)

// recordThenFail collects every batch it receives and then answers with the
// status the caller chooses. It stands in for an ingest that Metronome accepted
// while the response never made it back, which Metronome's own guidance calls
// out as the case a deterministic transaction ID exists to survive.
func recordThenFail(t *testing.T, status *int) (*httptest.Server, *[]CloudEvent, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var collected []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		mu.Lock()
		collected = append(collected, events...)
		code := *status
		mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(srv.Close)
	return srv, &collected, &mu
}

// chargedHours totals cu_hours the way Metronome does: one event per transaction
// ID, repeats ignored for 34 days. Summing the raw wire traffic instead would
// measure what we sent rather than what the customer pays.
func chargedHours(events []CloudEvent) float64 {
	seen := map[string]bool{}
	var total float64
	for _, ev := range events {
		if seen[ev.ID] {
			continue
		}
		seen[ev.ID] = true
		if v, ok := eventData(ev)["cu_hours"].(float64); ok {
			total += v
		}
	}
	return total
}

func transactionIDs(events []CloudEvent) []string {
	ids := make([]string, 0, len(events))
	for _, ev := range events {
		ids = append(ids, ev.ID)
	}
	return ids
}

// An account must be billed for the time it actually held the reservation, and
// no more. The anchor only advances after a successful ingest, so an ingest that
// Metronome accepted but reported as failed leaves the anchor behind and the next
// tick re-bills the same minutes. Metronome cannot collapse the repeat, because
// every event carries a fresh transaction ID.
func TestEmitActiveBilling_AmbiguousIngestFailureBillsTheSameSpanTwice(t *testing.T) {
	db := setupSQLiteDB(t)
	status := http.StatusInternalServerError
	srv, collected, mu := recordThenFail(t, &status)
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, logger.New("error", "json"))

	start := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status)
		VALUES ('dep-1', 'acct-1', 'agent', 'ns-1', 'active')`); err != nil {
		t.Fatal(err)
	}
	// 1 CU: max(cpu 1, mem 2Gi / 2) * 1 replica.
	if _, err := db.Exec(`INSERT INTO deployment_billing_state
		(deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, start); err != nil {
		t.Fatal(err)
	}

	// Tick one, five minutes in. Metronome records the usage and answers 500.
	m.emitActiveBilling(context.Background(), start.Add(5*time.Minute))

	// Tick two, ten minutes in, with the transport healthy again.
	mu.Lock()
	status = http.StatusNoContent
	mu.Unlock()
	m.emitActiveBilling(context.Background(), start.Add(10*time.Minute))

	mu.Lock()
	events := append([]CloudEvent(nil), *collected...)
	mu.Unlock()

	const wantHours = 10.0 / 60.0 // ten minutes of one CU
	got := chargedHours(events)
	if math.Abs(got-wantHours) > 1e-9 {
		t.Errorf("billed %.6f CU-hours for %.6f hours of reservation (events: %d, ids: %v)",
			got, wantHours, len(events), transactionIDs(events))
	}
}

// The same overcount from the other direction: the ingest succeeds and the
// anchor write fails. The code logs that failure and carries on, so the next
// tick bills from the stale anchor over minutes already paid for.
func TestEmitActiveBilling_FailedAnchorWriteBillsTheSameSpanTwice(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, collected, mu := collectEvents(t)
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, logger.New("error", "json"))

	start := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status)
		VALUES ('dep-1', 'acct-1', 'agent', 'ns-1', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO deployment_billing_state
		(deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, start); err != nil {
		t.Fatal(err)
	}

	// Refuse the anchor write only, leaving the read and the ingest working.
	if _, err := db.Exec(`CREATE TRIGGER refuse_anchor BEFORE UPDATE ON deployment_billing_state
		BEGIN SELECT RAISE(ABORT, 'anchor write failed'); END`); err != nil {
		t.Fatal(err)
	}
	m.emitActiveBilling(context.Background(), start.Add(5*time.Minute))
	if _, err := db.Exec(`DROP TRIGGER refuse_anchor`); err != nil {
		t.Fatal(err)
	}

	m.emitActiveBilling(context.Background(), start.Add(10*time.Minute))

	mu.Lock()
	events := append([]CloudEvent(nil), *collected...)
	mu.Unlock()

	const wantHours = 10.0 / 60.0
	got := chargedHours(events)
	if math.Abs(got-wantHours) > 1e-9 {
		t.Errorf("billed %.6f CU-hours for %.6f hours of reservation (events: %d, ids: %v)",
			got, wantHours, len(events), transactionIDs(events))
	}
}

// Metronome deduplicates on transaction ID for 34 days, which is what makes a
// repeated heartbeat safe. The same span has to carry the same ID, or the repeat
// is a second charge.
func TestEmitActiveBilling_RepeatedSpanReusesItsTransactionID(t *testing.T) {
	db := setupSQLiteDB(t)
	status := http.StatusInternalServerError
	srv, collected, mu := recordThenFail(t, &status)
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, logger.New("error", "json"))

	start := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status)
		VALUES ('dep-1', 'acct-1', 'agent', 'ns-1', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO deployment_billing_state
		(deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, start); err != nil {
		t.Fatal(err)
	}

	m.emitActiveBilling(context.Background(), start.Add(5*time.Minute))
	mu.Lock()
	status = http.StatusNoContent
	mu.Unlock()
	m.emitActiveBilling(context.Background(), start.Add(5*time.Minute))

	mu.Lock()
	ids := transactionIDs(*collected)
	mu.Unlock()

	if len(ids) != 2 {
		t.Fatalf("expected two emits of the same span, got %d", len(ids))
	}
	if ids[0] != ids[1] {
		t.Errorf("the same span emitted twice carries ids %q and %q, so Metronome bills it twice", ids[0], ids[1])
	}
}

// A window that has not closed carries no final value yet, so emitting it would
// mean sending a different amount later under the same ID. Metronome keeps the
// first, and the account is undercharged.
func TestEmitActiveBilling_OpenWindowIsNotBilled(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, collected, mu := collectEvents(t)
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, logger.New("error", "json"))

	start := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	if _, err := db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status)
		VALUES ('dep-1', 'acct-1', 'agent', 'ns-1', 'active')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO deployment_billing_state
		(deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, start); err != nil {
		t.Fatal(err)
	}

	m.emitActiveBilling(context.Background(), start.Add(3*time.Minute))

	mu.Lock()
	n := len(*collected)
	mu.Unlock()
	if n != 0 {
		t.Errorf("emitted %d events for a window still open", n)
	}

	var anchor time.Time
	if err := db.QueryRow(`SELECT last_emitted_at FROM deployment_billing_state`).Scan(&anchor); err != nil {
		t.Fatal(err)
	}
	if !anchor.UTC().Equal(start) {
		t.Errorf("anchor moved to %s without billing anything; those minutes are now lost", anchor.UTC())
	}
}

// Metronome rejects usage older than 34 days. The anchor advances past whatever
// is emitted, so a row further behind than that would have its oldest hours
// dropped with nothing to show for it.
func TestCompletedWindows_ClampsAtTheBackdatingLimit(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)

	spans, skipped := completedWindows(now.Add(-40*24*time.Hour), now)
	if skipped <= 0 {
		t.Fatal("skipped = 0, want the unbillable stretch reported")
	}
	if want := 7 * 24 * time.Hour; skipped < want-meterWindow || skipped > want+meterWindow {
		t.Errorf("skipped = %s, want about %s", skipped, want)
	}
	if len(spans) == 0 {
		t.Fatal("no spans emitted; the billable remainder was dropped too")
	}
	oldest := spans[0].start
	if oldest.Before(now.Add(-maxBackdate).Add(-meterWindow)) {
		t.Errorf("oldest span starts %s, which Metronome would reject", oldest)
	}
}

// A row inside the limit reports nothing skipped, so the error log stays quiet
// for every normal tick.
func TestCompletedWindows_InsideTheLimitSkipsNothing(t *testing.T) {
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	spans, skipped := completedWindows(now.Add(-15*time.Minute), now)
	if skipped != 0 {
		t.Errorf("skipped = %s, want 0", skipped)
	}
	if len(spans) != 3 {
		t.Errorf("spans = %d, want 3", len(spans))
	}
}
