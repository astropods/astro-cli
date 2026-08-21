package riverqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// The classification has to survive the wrapping the producer applies, which is
// the part that would silently break: if the typed error were ever stringified
// on the way up, errors.As would stop matching and a permanent failure would go
// back to burning River's retry budget every day.
func TestIsUpstreamAuthFailure(t *testing.T) {
	wrapped := func(status int) error {
		// Same shape as the producer: fmt.Errorf with %w around the client error.
		return fmt.Errorf("insights rollup: usage grain: %w",
			&langfuse.APIError{StatusCode: status, Body: `{"message":"Invalid credentials"}`})
	}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"401 wrapped", wrapped(http.StatusUnauthorized), true},
		{"403 wrapped", wrapped(http.StatusForbidden), true},
		// Transient: must stay retryable.
		{"500", wrapped(http.StatusInternalServerError), false},
		{"429", wrapped(http.StatusTooManyRequests), false},
		{"plain error", errors.New("dial tcp: connection refused"), false},
		{"nil", nil, false},
		// Stringified rather than wrapped — the regression this guards against.
		{"stringified 401", errors.New("langfuse: unexpected status 401: nope"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isUpstreamAuthFailure(tt.err); got != tt.want {
				t.Errorf("isUpstreamAuthFailure(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// A deleted account must end the job, not retry it. Discovery can enqueue an
// account that is purged before the job runs, and every attempt then fails the
// same way. Worse, the state row cascaded away with the account, so recording
// the failure violates its foreign key and the job retries on an error it can
// never write down.
func TestDeletedAccountEndsTheJobWithoutWritingState(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close() //nolint:errcheck

	// No watermark yet, so the worker has days to roll and reaches the producer.
	mock.ExpectQuery("SELECT rolled_up_through").
		WithArgs("acct_gone", insightsrollup.SourceAgents).
		WillReturnError(sql.ErrNoRows)

	w := &InsightsRollupAccountWorker{
		producer: producerFunc(func(_ context.Context, accountID string, _ time.Time) error {
			// Exactly what the producer returns when GetByID reports the account
			// is gone; the wrapping is the part errors.Is has to see through.
			return fmt.Errorf("%w: %s", insightsrollup.ErrAccountGone, accountID)
		}),
		rollups: insightsrollup.NewStore(db),
		log:     logger.New("error", "json"),
	}

	if err := w.Work(t.Context(), &river.Job[InsightsRollupAccountArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args:   InsightsRollupAccountArgs{AccountID: "acct_gone"},
	}); err != nil {
		t.Fatalf("Work() = %v, want nil so River stops retrying", err)
	}
	// No RecordFailure and no Advance: an unexpected write here is the foreign
	// key violation this guards against.
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

type producerFunc func(ctx context.Context, accountID string, day time.Time) error

func (f producerFunc) RollUpDay(ctx context.Context, accountID string, day time.Time) error {
	return f(ctx, accountID, day)
}

// A reconcile re-reads the full retention window. The watermark exists to skip
// history, which is exactly wrong after an upstream fix — so a reconcile has to
// ignore it, and a normal run has to keep trusting it.
func TestReconcileIgnoresTheWatermark(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	current := insightsrollup.State{RolledUpThrough: time.Date(2026, 8, 4, 0, 0, 0, 0, time.UTC)}

	// Normal run: trailing re-roll only.
	if got := len(insightsrollup.DaysToRoll(current, now)); got != insightsrollup.TrailingReRollDays+1 {
		t.Errorf("normal run days = %d, want %d", got, insightsrollup.TrailingReRollDays+1)
	}
	// Reconcile drops the watermark, which is what the worker does for this run.
	if got := len(insightsrollup.DaysToRoll(insightsrollup.State{}, now)); got != insightsrollup.MaxBackfillDays {
		t.Errorf("reconcile days = %d, want %d", got, insightsrollup.MaxBackfillDays)
	}
}

// A run that dies partway must keep the days it finished. Without a per-day
// watermark write, a backfill longer than the job timeout redoes the same
// opening days on every attempt and never reaches the end: the watermark only
// moved after all 90 days, which is the point the run never got to.
func TestPartialRunKeepsTheDaysItFinished(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT rolled_up_through").
		WithArgs("acct_1", insightsrollup.SourceAgents).
		WillReturnError(sql.ErrNoRows)

	// Two days commit, the third fails. Each committed day writes its own
	// watermark; the failure records itself and stops the run.
	const committed = 2
	for i := range committed {
		mock.ExpectExec("INSERT INTO insights_rollup_state").
			WithArgs("acct_1", insightsrollup.SourceAgents, dayOffsetFromBackfillStart(i)).
			WillReturnResult(sqlmock.NewResult(0, 1))
	}
	mock.ExpectExec("INSERT INTO insights_rollup_state").
		WithArgs("acct_1", insightsrollup.SourceAgents, "upstream is down").
		WillReturnResult(sqlmock.NewResult(0, 1))

	var seen int
	w := &InsightsRollupAccountWorker{
		producer: producerFunc(func(_ context.Context, _ string, _ time.Time) error {
			seen++
			if seen > committed {
				return errors.New("upstream is down")
			}
			return nil
		}),
		rollups: insightsrollup.NewStore(db),
		log:     logger.New("error", "json"),
	}

	if err := w.Work(t.Context(), &river.Job[InsightsRollupAccountArgs]{
		JobRow: &rivertype.JobRow{Attempt: 1},
		Args:   InsightsRollupAccountArgs{AccountID: "acct_1"},
	}); err == nil {
		t.Fatal("Work() = nil, want an error so River retries the remaining days")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected database calls: %v", err)
	}
}

// dayOffsetFromBackfillStart returns the nth day a cold account rolls, formatted
// the way the store binds it. A cold run starts MaxBackfillDays back from
// yesterday, which is what DaysToRoll plans when there is no watermark.
func dayOffsetFromBackfillStart(n int) string {
	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	start := yesterday.AddDate(0, 0, -(insightsrollup.MaxBackfillDays - 1))
	return start.AddDate(0, 0, n).Format(time.DateOnly)
}

// The failure write must outlive the deadline that caused the failure. Reusing
// the job context means the row explaining a timeout is the one write
// guaranteed to fail, so consecutive_errors stays at zero and last_error stays
// empty for exactly the accounts that are stuck.
func TestFailureIsRecordedAfterTheJobDeadlinePasses(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("SELECT rolled_up_through").
		WithArgs("acct_1", insightsrollup.SourceAgents).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec("INSERT INTO insights_rollup_state").
		WithArgs("acct_1", insightsrollup.SourceAgents, "context deadline exceeded").
		WillReturnResult(sqlmock.NewResult(0, 1))

	// The deadline has to expire *during* the run, the way it does in production:
	// the state read succeeds, then the first day burns what is left.
	budgeted, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()

	w := &InsightsRollupAccountWorker{
		producer: producerFunc(func(ctx context.Context, _ string, _ time.Time) error {
			<-ctx.Done()
			return ctx.Err()
		}),
		rollups: insightsrollup.NewStore(db),
		log:     logger.New("error", "json"),
	}

	_ = w.Work(budgeted, &river.Job[InsightsRollupAccountArgs]{
		JobRow: &rivertype.JobRow{Attempt: 8},
		Args:   InsightsRollupAccountArgs{AccountID: "acct_1"},
	})
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("failure was not recorded past the deadline: %v", err)
	}
}

// The args must round-trip through JSON, because the admin console sends them as
// JSON against the schema derived from the zero value. A missing tag would make
// the flag unsettable from the one place it exists to be used.
func TestRollupArgsJSONRoundTrip(t *testing.T) {
	var discovery InsightsRollupArgs
	if err := json.Unmarshal([]byte(`{"force":true,"reconcile":true}`), &discovery); err != nil {
		t.Fatalf("unmarshal discovery args: %v", err)
	}
	if !discovery.Force || !discovery.Reconcile {
		t.Errorf("discovery args = %+v, want both set", discovery)
	}

	var account InsightsRollupAccountArgs
	if err := json.Unmarshal([]byte(`{"account_id":"acct_1","reconcile":true}`), &account); err != nil {
		t.Fatalf("unmarshal account args: %v", err)
	}
	if account.AccountID != "acct_1" || !account.Reconcile {
		t.Errorf("account args = %+v", account)
	}

	// Defaults: the scheduled tick sends neither flag, so both must be false.
	var scheduled InsightsRollupArgs
	if err := json.Unmarshal([]byte(`{}`), &scheduled); err != nil {
		t.Fatalf("unmarshal empty: %v", err)
	}
	if scheduled.Force || scheduled.Reconcile {
		t.Errorf("scheduled args = %+v, want both false", scheduled)
	}
}
