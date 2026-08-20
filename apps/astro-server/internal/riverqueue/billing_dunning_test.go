package riverqueue

import (
	"context"
	"errors"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// dunningRow is the locked read Recompute performs per account.
func dunningRow(status billing.Status, reason string, dunningSince any) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"status", "reason", "dunning_since", "alert_active",
		"force_suspended", "credits_exhausted", "has_payment_method", "pay_link",
		"usage_limit_active", "not_provisioned",
	}).AddRow(string(status), reason, dunningSince, false, false, false, true, nil, false, false)
}

// expectRecompute queues the statements one Recompute issues when the status
// changes: locked read, persist, commit.
func expectRecompute(mock sqlmock.Sqlmock, accountID string, since any) {
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs(accountID).
		WillReturnRows(dunningRow(billing.StatusPastDue, billing.ReasonDunning, since))
	mock.ExpectExec("account_billing_status").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()
}

// The sweep is the only thing that ages a failed payment into a suspension, and
// it processes the whole work set in one job. One account whose read fails must
// not stop the accounts behind it, or a single locked row grants every later
// account free service until someone notices.
func TestDunningSweep_OneBadAccountDoesNotStopTheRest(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	expired := time.Now().Add(-30 * 24 * time.Hour)
	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).
			AddRow("acct_1").AddRow("acct_bad").AddRow("acct_3"))

	expectRecompute(mock, "acct_1", expired)
	// The middle account cannot be read. Recompute returns the error and the
	// sweep must log it and carry on.
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").WithArgs("acct_bad").WillReturnError(errors.New("row is locked"))
	mock.ExpectRollback()
	expectRecompute(mock, "acct_3", expired)

	w := &DunningSweepWorker{
		status: billing.NewStatusStore(db, 7),
		log:    logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the sweep stopped early: %v", err)
	}
}

// An account still inside its grace window recomputes to past_due, which is the
// status it already holds. Recompute reports no transition and writes nothing,
// so the sweep must not treat a re-evaluation as a fresh suspension.
func TestDunningSweep_LeavesAccountsInsideTheGraceAlone(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1"))
	mock.ExpectBegin()
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(dunningRow(billing.StatusPastDue, billing.ReasonDunning, time.Now().Add(-24*time.Hour)))
	// No Exec and no Commit: an unchanged status must not write.
	mock.ExpectRollback()

	w := &DunningSweepWorker{
		status: billing.NewStatusStore(db, 7),
		log:    logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("an in-grace account was written: %v", err)
	}
}

// Without a status store the sweep has no work set. It must no-op rather than
// dereference nil, because the worker is registered whether or not billing is
// configured.
func TestDunningSweep_NoOpsWithoutAStatusStore(t *testing.T) {
	w := &DunningSweepWorker{log: logger.New("error", "json")}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
}

// fakeDunningQueue records the sweep's output instead of enqueueing it.
type fakeDunningQueue struct {
	suspended     []string
	notified      []string
	notifiedNames []string
	suspender     error
}

func (f *fakeDunningQueue) InsertBillingSuspend(_ context.Context, accountID string) error {
	f.suspended = append(f.suspended, accountID)
	return f.suspender
}

func (f *fakeDunningQueue) EmitBillingNotify(_ context.Context, ev notify.Event) error {
	f.notified = append(f.notified, ev.AccountID)
	name, _ := ev.Payload[notify.PayloadAccount].(string)
	f.notifiedNames = append(f.notifiedNames, name)
	return nil
}

// fakeAccountNamer resolves ids the sweep asks about, or fails when err is set.
type fakeAccountNamer struct {
	names map[string]string
	err   error
}

func (f fakeAccountNamer) GetByID(id string) (*account.Account, error) {
	if f.err != nil {
		return nil, f.err
	}
	name, ok := f.names[id]
	if !ok {
		return nil, nil
	}
	return &account.Account{ID: id, Name: name}, nil
}

// A suspension notification has to name the account. The sweep works off ids, so
// without a lookup the message reaches the owner with a blank where the account
// name belongs.
func TestDunningSweep_SuspensionNamesTheAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1"))
	expectRecompute(mock, "acct_1", time.Now().Add(-30*24*time.Hour))

	q := &fakeDunningQueue{}
	w := &DunningSweepWorker{
		status:   billing.NewStatusStore(db, 7),
		queue:    q,
		accounts: fakeAccountNamer{names: map[string]string{"acct_1": "acme"}},
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(q.notifiedNames) != 1 || q.notifiedNames[0] != "acme" {
		t.Errorf("notified names = %q, want [acme]: the suspension email would be missing the account name", q.notifiedNames)
	}
}

// A failed name lookup must not swallow the notification: a suspension is worth
// sending unnamed, and the template handles the empty case.
func TestDunningSweep_NotifiesWhenTheNameIsUnavailable(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1"))
	expectRecompute(mock, "acct_1", time.Now().Add(-30*24*time.Hour))

	q := &fakeDunningQueue{}
	w := &DunningSweepWorker{
		status:   billing.NewStatusStore(db, 7),
		queue:    q,
		accounts: fakeAccountNamer{err: errors.New("account lookup down")},
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(q.notified) != 1 {
		t.Errorf("notified = %v, want one send: a name lookup failure must not drop a suspension notice", q.notified)
	}

	// A nil namer is the unconfigured path and must behave the same way.
	if got := (&DunningSweepWorker{log: logger.New("error", "json")}).accountName("acct_1"); got != "" {
		t.Errorf("accountName = %q, want empty with no namer", got)
	}
}

// Aging an account past its grace has to stop the workloads. The recompute only
// records the decision; this enqueue is what takes the service away, and it is
// the reason the sweep exists.
func TestDunningSweep_EnqueuesSuspendAndNotifiesOnTransition(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1"))
	expectRecompute(mock, "acct_1", time.Now().Add(-30*24*time.Hour))

	q := &fakeDunningQueue{}
	w := &DunningSweepWorker{status: billing.NewStatusStore(db, 7), queue: q, log: logger.New("error", "json")}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(q.suspended) != 1 || q.suspended[0] != "acct_1" {
		t.Errorf("suspended = %v, want [acct_1]: the account was aged out but nothing stopped it", q.suspended)
	}
	if len(q.notified) != 1 || q.notified[0] != "acct_1" {
		t.Errorf("notified = %v, want [acct_1]: the owner was not told", q.notified)
	}
}

// An account already suspended recomputes to the same status, so Recompute
// reports no transition. Re-enqueueing on every tick would re-suspend and
// re-notify hourly for as long as the account stays unpaid.
func TestDunningSweep_DoesNotReSuspendOnEveryTick(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close() //nolint:errcheck

	mock.ExpectQuery("FROM account_billing_status WHERE status").
		WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow("acct_1"))
	mock.ExpectBegin()
	// Already suspended for the same reason: recompute changes nothing.
	mock.ExpectQuery("FOR UPDATE").
		WithArgs("acct_1").
		WillReturnRows(sqlmock.NewRows([]string{
			"status", "reason", "dunning_since", "alert_active",
			"force_suspended", "credits_exhausted", "has_payment_method",
		}).AddRow("suspended", billing.ReasonPaymentFailed,
			time.Now().Add(-30*24*time.Hour), false, false, false, true))
	mock.ExpectRollback()

	q := &fakeDunningQueue{}
	w := &DunningSweepWorker{status: billing.NewStatusStore(db, 7), queue: q, log: logger.New("error", "json")}
	if err := w.Work(context.Background(), &river.Job[DunningSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(q.suspended) != 0 || len(q.notified) != 0 {
		t.Errorf("re-fired on an unchanged status: suspended=%v notified=%v", q.suspended, q.notified)
	}
}
