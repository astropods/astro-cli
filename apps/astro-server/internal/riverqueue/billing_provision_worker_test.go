package riverqueue

import (
	"context"
	"errors"
	"strings"
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
	plan            billing.Plan
	spendLimits     []float64
	spendLimitErr   error
}

func (f *fakeProvisioner) CreateCustomer(context.Context, billing.Account) (string, error) {
	f.createCalls++
	return f.createdCustomer, nil
}

func (f *fakeProvisioner) ProvisionCustomer(_ context.Context, _, _ string, plan billing.Plan) (bool, error) {
	f.provisionCalls++
	f.plan = plan
	return f.provisioned, f.provisionErr
}

func (f *fakeProvisioner) SetCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind, amount float64) error {
	if kind == billing.SpendThresholdLimit {
		f.spendLimits = append(f.spendLimits, amount)
	}
	return f.spendLimitErr
}

func (f *fakeProvisioner) ClearCustomerSpendThreshold(context.Context, string, billing.SpendThresholdKind) error {
	return nil
}

const (
	acctRe          = `SELECT a\.id, a\.name, a\.type`
	getCustomerRe   = `SELECT metronome_customer_id FROM accounts`
	setCustomerRe   = `UPDATE accounts SET metronome_customer_id`
	ownerEmailRe    = `SELECT me\.email\s+FROM account_members`
	bifrostIDRe     = `SELECT bifrost_customer_id FROM accounts`
	markProvisionRe = `UPDATE accounts SET billing_provisioned_at`
	readStampRe     = `SELECT billing_provisioned_at FROM accounts`
	creatorRe       = `SELECT user_id FROM account_members`
	claimCreditRe   = `INSERT INTO billing_credit_grants`
)

func expectStamp(mock sqlmock.Sqlmock, stamped bool) {
	var at any
	if stamped {
		at = time.Unix(0, 0)
	}
	mock.ExpectQuery(readStampRe).WillReturnRows(sqlmock.NewRows([]string{"billing_provisioned_at"}).AddRow(at))
}

// expectCreditClaim stands in for the per-person credit ledger. holder is the
// account the user's one claim belongs to, so passing the account under test
// means the credit is due and passing another means it was already spent.
func expectCreditClaim(mock sqlmock.Sqlmock, holder string) {
	mock.ExpectQuery(creatorRe).WillReturnRows(sqlmock.NewRows([]string{"user_id"}).AddRow("user_1"))
	mock.ExpectQuery(claimCreditRe).WillReturnRows(sqlmock.NewRows([]string{"account_id"}).AddRow(holder))
}

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
		"display_name", "avatar_colors", "avatar_updated_at",
		"account_number", "bio", "location", "local_timezone", "pronouns", "website",
		"social_links", "blueprint_order",
	}
	now := time.Unix(0, 0)
	row := sqlmock.NewRows(cols).AddRow(
		"acct_1", "acme", "org", nil, nil, now, now,
		"", nil, nil,
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
	expectCreditClaim(mock, "acct_1")
	expectStamp(mock, false)
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
	expectCreditClaim(mock, "acct_1")
	// No mark expectation: stamping here is the bug.

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected statements: %v", err)
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
	expectCreditClaim(mock, "acct_1")
	expectStamp(mock, false)
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

// Signup credit belongs to a person. Nothing caps how many accounts one user
// creates, so granting per account lets a user mint organizations for free
// credit indefinitely.
func TestProvisionWorker_SignupCreditGoesToTheFirstAccountOnly(t *testing.T) {
	cases := []struct {
		name     string
		holder   string
		wantPlan billing.Plan
	}{
		// The ledger row names this account, so the claim is ours.
		{"first account for this user", "acct_1", billing.PlanCredit},
		// The user already spent their claim on an earlier account.
		{"user already claimed it", "acct_earlier", billing.PlanNoCredit},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &fakeProvisioner{provisioned: true}
			w, mock := provisionWorker(t, p)

			expectAccount(mock)
			mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
			expectCreditClaim(mock, tc.holder)
			expectStamp(mock, false)
			mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

			if err := runProvision(t, w); err != nil {
				t.Fatalf("Work: %v", err)
			}
			if p.plan != tc.wantPlan {
				t.Errorf("provisioned plan = %q, want %q", p.plan, tc.wantPlan)
			}
		})
	}
}

// A grant that cannot be attributed to anyone is the case this guards against,
// so an unresolvable creator withholds the credit rather than defaulting to it.
// The account is still provisioned: without a contract its usage is never rated.
func TestProvisionWorker_NoCreatorMeansNoCredit(t *testing.T) {
	p := &fakeProvisioner{provisioned: true}
	w, mock := provisionWorker(t, p)

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
	mock.ExpectQuery(creatorRe).WillReturnRows(sqlmock.NewRows([]string{"user_id"}))
	expectStamp(mock, false)
	mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if p.provisionCalls != 1 {
		t.Fatalf("provision calls = %d, want 1: the account must still be billable", p.provisionCalls)
	}
	if p.plan != billing.PlanNoCredit {
		t.Errorf("plan = %q, want %q: credit needs a resolvable creator", p.plan, billing.PlanNoCredit)
	}
}

// The no-credit plan is the one the provider will never signal about: it starts
// with no balance, so no low-balance alert can fire, so provisioning is the only
// place the latch is ever raised. An account that keeps the granted signal here
// runs with neither credit nor a card behind it.
func TestCreditSignal_OnlyTheNoCreditPlanLatches(t *testing.T) {
	cases := map[billing.Plan]billing.Signal{
		billing.PlanNoCredit: billing.SignalCreditsExhausted,
		billing.PlanCredit:   billing.SignalCreditsGranted,
		"":                   billing.SignalCreditsGranted,
	}
	for plan, want := range cases {
		if got := creditSignal(plan); got != want {
			t.Errorf("creditSignal(%q) = %q, want %q", plan, got, want)
		}
	}
}

// Thresholds are cents, so dollars would cap the account a hundred times too low.
func TestProvisionWorker_SeedsDefaultSpendLimit(t *testing.T) {
	p := &fakeProvisioner{provisioned: true}
	w, mock := provisionWorker(t, p)

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
	expectCreditClaim(mock, "acct_1")
	expectStamp(mock, false)
	mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if want := []float64{2000}; len(p.spendLimits) != 1 || p.spendLimits[0] != want[0] {
		t.Errorf("spend limits set = %v, want %v cents", p.spendLimits, want)
	}
}

// An operator credit grant re-runs this job, and reseeding reimposes a cleared cap.
func TestProvisionWorker_DoesNotReseedSpendLimitOnAlreadyProvisionedAccount(t *testing.T) {
	p := &fakeProvisioner{provisioned: true}
	w, mock := provisionWorker(t, p)

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
	expectCreditClaim(mock, "acct_1")
	expectStamp(mock, true)
	mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

	if err := runProvision(t, w); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(p.spendLimits) != 0 {
		t.Errorf("reseeded the spend limit on a provisioned account: %v", p.spendLimits)
	}
}

// The stamp removes the account from the sweep, so a failed seed must not stamp.
func TestProvisionWorker_FailedSeedLeavesAccountUnstamped(t *testing.T) {
	p := &fakeProvisioner{provisioned: true, spendLimitErr: errors.New("provider down")}
	w, mock := provisionWorker(t, p)

	expectAccount(mock)
	mock.ExpectQuery(getCustomerRe).WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cus_1"))
	expectCreditClaim(mock, "acct_1")
	expectStamp(mock, false)
	// Expected so it can be asserted unfulfilled. Omitting it makes sqlmock error
	// on the stamp, and the test passes either way.
	mock.ExpectExec(markProvisionRe).WillReturnResult(sqlmock.NewResult(0, 1))

	err := runProvision(t, w)
	if err == nil || !strings.Contains(err.Error(), "seed default spend limit") {
		t.Fatalf("Work error = %v, want the seed failure so the job retries", err)
	}
	if mock.ExpectationsWereMet() == nil {
		t.Fatal("the account was stamped despite the failed seed")
	}
}
