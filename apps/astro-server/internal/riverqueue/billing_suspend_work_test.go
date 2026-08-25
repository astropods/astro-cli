package riverqueue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"k8s.io/client-go/kubernetes"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

type recordingResumeQueue struct{ woken []string }

func (q *recordingResumeQueue) InsertWakeUpJob(_ context.Context, deploymentID, _ string) error {
	q.woken = append(q.woken, deploymentID)
	return nil
}

var testTime = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

// Mirrors the column list GetActiveDeploymentsByAccount scans; a drift there
// surfaces here rather than as a nil row.
func activeDeploymentRows(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace",
		"display_name", "deployment_spec_json", "status", "deployed_at", "undeployed_at",
	})
	for _, id := range ids {
		rows.AddRow(id, "acct-1", nil, "agent", "build-1", "ns-"+id, "Agent", "{}", "active", testTime, nil)
	}
	return rows
}

// Resume selects the wider deploymentColumns list through scanDeployment.
func suspendedDeploymentRows(ids ...string) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "account_id", "source_account_id", "agent_name", "build_id", "namespace", "display_name",
		"deployment_spec_json", "encrypted_data_key", "kms_key_arn", "cluster_id",
		"status", "error_message", "error_details", "status_changed_at", "current_revision",
		"deployed_at", "undeployed_at", "avatar_colors", "avatar_updated_at",
	})
	for _, id := range ids {
		rows.AddRow(id, "acct-1", nil, "agent", "build-1", "ns-"+id, "Agent",
			"{}", nil, nil, nil,
			deploymentstore.StatusSuspended, nil, nil, testTime, nil,
			testTime, nil, nil, nil)
	}
	return rows
}

func expectStatusWrite(mock sqlmock.Sqlmock) {
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("INSERT INTO deployment_events").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

func suspendTestWorker(t *testing.T, db *sql.DB, stop func(context.Context, kubernetes.Interface, string) error) *BillingSuspendWorker {
	t.Helper()
	return &BillingSuspendWorker{
		store:         deploymentstore.NewStore(db),
		status:        billing.NewStatusStore(db, 0),
		reg:           k8s.NewRegistryWithPrimary(primaryClient{}),
		cache:         k8scache.New(nil),
		log:           logger.New("error", "json"),
		stopWorkloads: stop,
	}
}

// One row left running is an account still spending after it was gated.
func TestBillingSuspendWork_StopsEveryActiveDeployment(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).
			AddRow("suspended", billing.ReasonCreditsExhausted))
	mock.ExpectQuery("FROM deployments").WithArgs("acct-1").
		WillReturnRows(activeDeploymentRows("dep-1", "dep-2"))
	expectStatusWrite(mock)
	expectStatusWrite(mock)

	var stopped []string
	w := suspendTestWorker(t, db, func(_ context.Context, _ kubernetes.Interface, ns string) error {
		stopped = append(stopped, ns)
		return nil
	})

	if err := w.Work(context.Background(), &river.Job[BillingSuspendArgs]{Args: BillingSuspendArgs{AccountID: "acct-1"}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if len(stopped) != 2 || stopped[0] != "ns-dep-1" || stopped[1] != "ns-dep-2" {
		t.Errorf("stopped namespaces = %v, want [ns-dep-1 ns-dep-2]", stopped)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
}

// Without a failed job nothing retries the deployment that is still running.
func TestBillingSuspendWork_OneFailureStopsTheRestAndRetries(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).AddRow("suspended", "dunning"))
	mock.ExpectQuery("FROM deployments").WithArgs("acct-1").
		WillReturnRows(activeDeploymentRows("dep-1", "dep-2"))
	// Only the second deployment reaches its status write.
	expectStatusWrite(mock)

	var stopped []string
	w := suspendTestWorker(t, db, func(_ context.Context, _ kubernetes.Interface, ns string) error {
		stopped = append(stopped, ns)
		if ns == "ns-dep-1" {
			return errors.New("cluster unreachable")
		}
		return nil
	})

	err = w.Work(context.Background(), &river.Job[BillingSuspendArgs]{Args: BillingSuspendArgs{AccountID: "acct-1"}})
	var snooze *rivertype.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("err = %v, want a snooze; a plain error is discarded after MaxAttempts and leaves the deployment running", err)
	}

	if len(stopped) != 2 {
		t.Errorf("attempted namespaces = %v, want both tried before failing", stopped)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
}

// The job has done nothing yet, so returning nil would drop the suspension.
func TestBillingSuspendWork_ListFailureSnoozes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).AddRow("suspended", "dunning"))
	mock.ExpectQuery("FROM deployments").WithArgs("acct-1").WillReturnError(errors.New("db down"))

	w := suspendTestWorker(t, db, func(context.Context, kubernetes.Interface, string) error {
		t.Fatal("cluster was called despite an unreadable deployment list")
		return nil
	})

	err = w.Work(context.Background(), &river.Job[BillingSuspendArgs]{Args: BillingSuspendArgs{AccountID: "acct-1"}})
	var snooze *rivertype.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("err = %v, want a snooze so the suspension is not discarded", err)
	}
}

func TestBillingSuspendWork_NoDeploymentsIsANoop(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).AddRow("suspended", "dunning"))
	mock.ExpectQuery("FROM deployments").WithArgs("acct-1").WillReturnRows(activeDeploymentRows())

	w := suspendTestWorker(t, db, func(context.Context, kubernetes.Interface, string) error {
		t.Fatal("cluster was called for an account with no active deployments")
		return nil
	})

	if err := w.Work(context.Background(), &river.Job[BillingSuspendArgs]{Args: BillingSuspendArgs{AccountID: "acct-1"}}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// Credit exhaustion carries its own copy because the fix differs from dunning.
func TestSuspendEvent_ReasonPicksTheStatusAndCopy(t *testing.T) {
	tests := []struct {
		name    string
		reason  string
		wantMsg string
	}{
		{"credits exhausted", billing.ReasonCreditsExhausted, "Stopped: free credits used up and no payment method on file"},
		{"any other reason", "dunning", "Stopped by billing"},
		{"unknown", reasonUnknown, "Stopped by billing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := suspendEvent(tt.reason)
			if ev.Status != deploymentstore.StatusSuspended {
				t.Errorf("status = %q, want %q so resume only restores what billing stopped",
					ev.Status, deploymentstore.StatusSuspended)
			}
			if ev.EventMsg != tt.wantMsg {
				t.Errorf("message = %q, want %q", ev.EventMsg, tt.wantMsg)
			}
			var got map[string]string
			if err := json.Unmarshal(ev.EventDetails, &got); err != nil {
				t.Fatalf("details are not valid JSON: %v", err)
			}
			if got["source"] != "billing" || got["reason"] != tt.reason {
				t.Errorf("details = %v, want source=billing reason=%s", got, tt.reason)
			}
		})
	}
}

func TestBillingEventDetails_ReasonCannotBreakOutOfTheJSON(t *testing.T) {
	raw := billingEventDetails(`bad","injected":"yes`)

	var got map[string]string
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("details are not valid JSON: %v", err)
	}
	if _, ok := got["injected"]; ok {
		t.Errorf("details = %v, want no injected key", got)
	}
}

// With no status store the job still acts, and labels the timeline rather than
// guessing a reason.
func TestSuspendState_NoStatusStoreActsWithAnUnknownReason(t *testing.T) {
	w := &BillingSuspendWorker{log: logger.New("error", "json")}
	status, reason, err := w.suspendState(context.Background(), "acct-1")
	if err != nil {
		t.Fatalf("suspendState: %v", err)
	}
	if status != billing.StatusSuspended {
		t.Errorf("status = %q, want %q so the job still acts", status, billing.StatusSuspended)
	}
	if reason != reasonUnknown {
		t.Errorf("reason = %q, want %q", reason, reasonUnknown)
	}
}

// Suspend writes StatusSuspended, not StatusStopped, so a user-stopped
// deployment stays stopped.
func TestBillingResumeWork_RestoresOnlySuspendedDeployments(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM deployments").WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(suspendedDeploymentRows("dep-1"))
	expectStatusWrite(mock)

	q := &recordingResumeQueue{}
	w := &BillingResumeWorker{
		store: deploymentstore.NewStore(db),
		queue: q,
		log:   logger.New("error", "json"),
	}

	if err := w.Work(context.Background(), &river.Job[BillingResumeArgs]{Args: BillingResumeArgs{AccountID: "acct-1"}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if len(q.woken) != 1 || q.woken[0] != "dep-1" {
		t.Errorf("woken deployments = %v, want [dep-1]", q.woken)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
}

// A wakeup after a failed write leaves the row pending while nothing restarts.
func TestBillingResumeWork_FailedStatusWriteSkipsTheWakeup(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM deployments").WithArgs("acct-1", sqlmock.AnyArg()).
		WillReturnRows(suspendedDeploymentRows("dep-1"))
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE deployments").WillReturnError(errors.New("write failed"))
	mock.ExpectRollback()

	q := &recordingResumeQueue{}
	w := &BillingResumeWorker{
		store: deploymentstore.NewStore(db),
		queue: q,
		log:   logger.New("error", "json"),
	}

	if err := w.Work(context.Background(), &river.Job[BillingResumeArgs]{Args: BillingResumeArgs{AccountID: "acct-1"}}); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if len(q.woken) != 0 {
		t.Errorf("woken deployments = %v, want none after a failed status write", q.woken)
	}
}

// Recovery fires its resume once, so anything stopped after it stays stopped.
func TestBillingSuspendWork_RecoveredAccountIsLeftAlone(t *testing.T) {
	for _, status := range []string{"active", "past_due"} {
		t.Run(status, func(t *testing.T) {
			db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()

			mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
				WillReturnRows(sqlmock.NewRows([]string{"status", "reason"}).AddRow(status, ""))

			w := suspendTestWorker(t, db, func(context.Context, kubernetes.Interface, string) error {
				t.Fatalf("cluster was called for an account in %s", status)
				return nil
			})

			if err := w.Work(context.Background(), &river.Job[BillingSuspendArgs]{Args: BillingSuspendArgs{AccountID: "acct-1"}}); err != nil {
				t.Fatalf("Work: %v", err)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("db: %v", err)
			}
		})
	}
}

// An unreadable status is neither recovery nor permission to act.
func TestBillingSuspendWork_UnreadableStatusSnoozes(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnError(errors.New("db down"))

	w := suspendTestWorker(t, db, func(context.Context, kubernetes.Interface, string) error {
		t.Fatal("cluster was called on an unreadable billing status")
		return nil
	})

	err = w.Work(context.Background(), &river.Job[BillingSuspendArgs]{Args: BillingSuspendArgs{AccountID: "acct-1"}})
	var snooze *rivertype.JobSnoozeError
	if !errors.As(err, &snooze) {
		t.Fatalf("err = %v, want a snooze so the suspension is not discarded", err)
	}
}

// A snooze carries no message, so the guard against swallowing the read failure
// lives here: without it the run reaches the deployment list and snoozes anyway.
func TestSuspendState_ReadFailureNamesItsSource(t *testing.T) {
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnError(errors.New("db down"))

	w := &BillingSuspendWorker{status: billing.NewStatusStore(db, 0), log: logger.New("error", "json")}
	_, _, err = w.suspendState(context.Background(), "acct-1")
	if err == nil {
		t.Fatal("err is nil; a failed status read must not read as recovery or as permission to act")
	}
	if !strings.Contains(err.Error(), "read billing status") {
		t.Errorf("error = %v, want it to name the status read", err)
	}
}
