package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

const (
	checkObservedPortMarker = "SELECT EXISTS(SELECT 1 FROM slack_observed_port_marker)"
	writeObservedPortMarker = "\n\t\tINSERT INTO slack_observed_port_marker (id, completed_at)\n\t\tVALUES (1, now())\n\t\tON CONFLICT (id) DO NOTHING\n\t"
	portObservedRowsQuery   = "\n\t\tINSERT INTO slack_observed_users (team_id, slack_user_id, first_seen_at, last_seen_at)\n\t\tSELECT team_id, slack_user_id, created_at, updated_at\n\t\tFROM slack_identity_mappings\n\t\tWHERE source = 'observed' AND revoked_at IS NULL\n\t\tON CONFLICT (team_id, slack_user_id) DO NOTHING\n\t"
)

func newPortWorker(t *testing.T) (*SlackObservedPortWorker, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &SlackObservedPortWorker{
		slackStore: slackidentity.NewStore(db),
		log:        logger.New("error", "json"),
	}, mock
}

// SlackObservedPortArgs.Kind is the River job-kind string. Pinning it
// prevents an accidental rename from silently orphaning queued jobs
// (River matches workers by kind string).
func TestSlackObservedPortArgs_Kind(t *testing.T) {
	if got := (SlackObservedPortArgs{}.Kind()); got != "slack.observed_port" {
		t.Errorf("kind = %q; want %q", got, "slack.observed_port")
	}
}

// First run path: marker absent → port runs → marker written.
func TestSlackObservedPortWorker_FirstRun(t *testing.T) {
	w, mock := newPortWorker(t)

	mock.ExpectQuery(checkObservedPortMarker).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(portObservedRowsQuery).
		WillReturnResult(sqlmock.NewResult(0, 17))
	mock.ExpectExec(writeObservedPortMarker).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := w.Work(t.Context(), &river.Job[SlackObservedPortArgs]{}); err != nil {
		t.Fatalf("work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// Marker-present path: worker bails before touching any data. Critical
// for the "never runs twice" guarantee — a duplicate enqueue (e.g. from
// a rolling restart) must be a fast no-op.
func TestSlackObservedPortWorker_AlreadyComplete(t *testing.T) {
	w, mock := newPortWorker(t)

	mock.ExpectQuery(checkObservedPortMarker).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))
	// No INSERT, no marker write expected.

	if err := w.Work(t.Context(), &river.Job[SlackObservedPortArgs]{}); err != nil {
		t.Fatalf("work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// Copy-step failure: worker logs and returns nil (queue mustn't wedge).
// Marker is NOT written so the next pod restart retries. The copy is
// idempotent (ON CONFLICT DO NOTHING) so retries are safe.
func TestSlackObservedPortWorker_CopyFails_NoMarker(t *testing.T) {
	w, mock := newPortWorker(t)

	mock.ExpectQuery(checkObservedPortMarker).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))
	mock.ExpectExec(portObservedRowsQuery).
		WillReturnError(errors.New("connection refused"))
	// No marker-write expected: copy failed, retry on next deploy.

	if err := w.Work(t.Context(), &river.Job[SlackObservedPortArgs]{}); err != nil {
		t.Fatalf("work should swallow copy errors, got: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// Marker-check failure: worker logs and returns nil. No copy attempt.
// Same shape as the directory backfill — DB transient should not
// produce a noisy retry storm.
func TestSlackObservedPortWorker_MarkerCheckFails(t *testing.T) {
	w, mock := newPortWorker(t)

	mock.ExpectQuery(checkObservedPortMarker).
		WillReturnError(errors.New("query failed"))

	if err := w.Work(t.Context(), &river.Job[SlackObservedPortArgs]{}); err != nil {
		t.Fatalf("work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("expectations: %v", err)
	}
}

// Missing dependencies: the worker exits as a no-op. Defends against a
// future refactor that could land the worker registration before the
// dependency wiring.
func TestSlackObservedPortWorker_NilStore(t *testing.T) {
	w := &SlackObservedPortWorker{
		slackStore: nil,
		log:        logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[SlackObservedPortArgs]{}); err != nil {
		t.Errorf("work: %v", err)
	}
}
