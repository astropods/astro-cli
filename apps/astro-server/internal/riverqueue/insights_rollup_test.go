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
		Args: InsightsRollupAccountArgs{AccountID: "acct_gone"},
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
