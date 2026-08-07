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
)

// fakeProvisioner is a billing.BillingProvider that also provisions, so the
// worker's interface assertion succeeds.
type fakeProvisioner struct {
	billing.BillingProvider
	createdCustomer string
	createCalls     int
	provisioned     bool
	provisionErr    error
	provisionCalls  int
}

func (f *fakeProvisioner) CreateCustomer(context.Context, billing.Account) (string, error) {
	f.createCalls++
	return f.createdCustomer, nil
}

func (f *fakeProvisioner) ProvisionCustomer(context.Context, string, string) (bool, error) {
	f.provisionCalls++
	return f.provisioned, f.provisionErr
}

const (
	acctRe          = `SELECT a\.id, a\.name, a\.type`
	getCustomerRe   = `SELECT metronome_customer_id FROM accounts`
	setCustomerRe   = `UPDATE accounts SET metronome_customer_id`
	ownerEmailRe    = `SELECT me\.email`
	bifrostIDRe     = `SELECT bifrost_customer_id FROM accounts`
	markProvisionRe = `UPDATE accounts SET billing_provisioned_at`
)

func provisionWorker(t *testing.T, p *fakeProvisioner) (*BillingProvisionWorker, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	mock.MatchExpectationsInOrder(false)
	return &BillingProvisionWorker{
		accounts: account.NewAccountStore(db),
		provider: p,
		backend:  "metronome",
		log:      logger.New("error", "json"),
	}, mock
}

// expectAccount stubs GetByID. Only the first three columns are read here, but
// scanAccount consumes the full projection.
func expectAccount(mock sqlmock.Sqlmock) {
	cols := []string{
		"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at",
		"display_name", "avatar_colors", "avatar_updated_at", "cluster_id",
		"account_number", "bio", "location", "local_timezone", "pronouns", "website",
		"social_links", "blueprint_order",
	}
	now := time.Unix(0, 0)
	row := sqlmock.NewRows(cols).AddRow(
		"acct_1", "acme", "org", nil, nil, now, now,
		"", nil, nil, nil,
		nil, nil, nil, nil, nil, nil, "{}", "{}",
	)
	mock.ExpectQuery(acctRe).WillReturnRows(row)
}

func runProvision(t *testing.T, w *BillingProvisionWorker) error {
	t.Helper()
	return w.Work(context.Background(), &river.Job[BillingProvisionArgs]{
		Args: BillingProvisionArgs{AccountID: "acct_1"},
	})
}

// The happy path stamps the account exactly once, which is what removes it from
// the hourly sweep.
func TestProvisionWorker_StampsOnceWhenProvisioned(t *testing.T) {
	p := &fakeProvisioner{provisioned: true}
	w, mock := provisionWorker(t, p)

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
	mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if p.createCalls != 0 {
		t.Errorf("created a customer despite one already being stored (%d calls)", p.createCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("account was not stamped: %v", err)
	}
}

// An unconfigured provider did nothing, so stamping would hide the account from
// the sweep for good.
func TestProvisionWorker_LeavesUnstampedWhenNothingProvisioned(t *testing.T) {
	w, mock := provisionWorker(t, &fakeProvisioner{provisioned: false})

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
	// No mark expectation: stamping here is the bug.

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected statements: %v", err)
	}
}

// A blocked account needs an operator, so the job must cancel rather than
// return a plain error and burn its backoff schedule every sweep tick.
func TestProvisionWorker_CancelsWhenBlocked(t *testing.T) {
	w, mock := provisionWorker(t, &fakeProvisioner{provisionErr: billing.ErrProvisionBlocked})

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))

	err := runProvision(t, w)
	if err == nil {
		t.Fatal("expected the job to fail")
	}
	if !errors.Is(err, billing.ErrProvisionBlocked) {
		t.Errorf("lost the cause: %v", err)
	}
	// A plain return surfaces the sentinel verbatim; JobCancel wraps it.
	if err.Error() == billing.ErrProvisionBlocked.Error() {
		t.Error("returned a plain error; the job would retry instead of cancelling")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("account was stamped despite being blocked: %v", err)
	}
}

// Creating the customer must persist its id in the same run, or a retry makes a
// second customer and splits the account's usage.
func TestProvisionWorker_PersistsNewCustomerID(t *testing.T) {
	p := &fakeProvisioner{createdCustomer: "cus_new", provisioned: true}
	w, mock := provisionWorker(t, p)

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow(nil))
	mock.ExpectQuery(bifrostIDRe).WillReturnRows(sqlmock.NewRows([]string{"bifrost_customer_id"}).AddRow(nil))
	mock.ExpectQuery(ownerEmailRe).WillReturnRows(sqlmock.NewRows([]string{"email"}).AddRow("owner@example.com"))
	mock.ExpectExec(setCustomerRe).WithArgs("cus_new", sqlmock.AnyArg(), "acct_1").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if p.createCalls != 1 {
		t.Errorf("CreateCustomer called %d times, want 1", p.createCalls)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("customer id was not persisted: %v", err)
	}
}
