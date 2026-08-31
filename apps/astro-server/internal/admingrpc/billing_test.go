package admingrpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// stubInspector reports a fixed coverage verdict and spend, standing in for
// Metronome.
type stubInspector struct {
	billing.BillingProvider
	coverage billing.Coverage
	err      error
	spend    billing.Spend
	spendErr error
}

func (s stubInspector) ContractCoverage(context.Context, string) (billing.Coverage, error) {
	return s.coverage, s.err
}

func (s stubInspector) CustomerSpend(context.Context, string) (billing.Spend, error) {
	return s.spend, s.spendErr
}

func expectBillingIDs(mock sqlmock.Sqlmock, accountID, metronomeID string, provisioned bool) {
	var provisionedAt any
	if provisioned {
		provisionedAt = time.Now()
	}
	mock.ExpectQuery("SELECT[\\s\\S]+FROM accounts WHERE id").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{
			"metronome_customer_id", "stripe_customer_id", "bifrost_customer_id", "billing_provisioned_at",
		}).AddRow(metronomeID, "", "", provisionedAt))
}

func expectNoBillingStatusRow(mock sqlmock.Sqlmock, accountID string) {
	mock.ExpectQuery("FROM account_billing_status").
		WithArgs(accountID).
		WillReturnRows(sqlmock.NewRows([]string{"status", "reason", "dunning_since", "alert_active", "updated_at"}))
}

func expectSuspendedWorkloads(mock sqlmock.Sqlmock, accountID string, suspended bool) {
	mock.ExpectQuery("SELECT EXISTS").
		WithArgs(accountID, deploymentstore.StatusSuspended).
		WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(suspended))
}

func expectNoProvisionJob(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM river.river_job").
		WillReturnRows(sqlmock.NewRows([]string{"id", "state", "attempt", "created_at", "finalized_at", "error"}))
}

func expectNoSpendCeiling(mock sqlmock.Sqlmock) {
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))
}

// A contract created outside provisioning must still report as covered:
// "none" reads as safe to provision, which is the double-billing case.
func TestGetAccountBillingDetail_ReportsContractCreatedByHand(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		db:          db,
		deployStore: deploymentstore.NewStore(db),
		log:         logger.New("error", "json"),
		billingProvider: stubInspector{coverage: billing.Coverage{
			State: billing.CoverageCovered,
			Contracts: []billing.Contract{{
				ID:         "con_by_hand",
				StartingAt: time.Now().Add(-time.Hour),
			}},
		}},
	}

	expectBillingIDs(mock, "acct-1", "cust-1", false)
	expectNoBillingStatusRow(mock, "acct-1")
	expectSuspendedWorkloads(mock, "acct-1", false)
	expectNoProvisionJob(mock)
	expectNoSpendCeiling(mock)

	resp, err := s.GetAccountBillingDetail(context.Background(), &adminv1.GetAccountBillingDetailRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccountBillingDetail: %v", err)
	}
	if resp.Coverage != billing.CoverageCovered {
		t.Errorf("coverage = %q, want %q", resp.Coverage, billing.CoverageCovered)
	}
	if len(resp.Contracts) != 1 || resp.Contracts[0].ID != "con_by_hand" {
		t.Errorf("contracts = %+v, want the covering contract", resp.Contracts)
	}
	if resp.MetronomeURL == "" {
		t.Error("metronome_url is empty; the operator cannot reach the contract")
	}
}

// The environment segment is what makes the link resolve. Preview bills against
// sandbox, so a link built without it opens the default environment and reports
// no such customer, which reads as missing data rather than a wrong link.
func TestMetronomeCustomerURL_CarriesEnvironment(t *testing.T) {
	for _, tc := range []struct{ env, want string }{
		{"sandbox", "https://app.metronome.com/sandbox/customers/cust-1"},
		{"", "https://app.metronome.com/customers/cust-1"},
	} {
		s := &Server{metronomeDashboardEnv: tc.env}
		if got := s.metronomeCustomerURL("cust-1"); got != tc.want {
			t.Errorf("metronomeCustomerURL(env=%q) = %q, want %q", tc.env, got, tc.want)
		}
	}
}

// A vendor failure must degrade the view, not empty it: the astro-side record
// is what says whether provisioning ever completed.
func TestGetAccountBillingDetail_ContractLookupFailureWarns(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		db:              db,
		deployStore:     deploymentstore.NewStore(db),
		log:             logger.New("error", "json"),
		billingProvider: stubInspector{err: errors.New("metronome down")},
	}

	expectBillingIDs(mock, "acct-1", "cust-1", true)
	expectNoBillingStatusRow(mock, "acct-1")
	expectSuspendedWorkloads(mock, "acct-1", false)
	expectNoProvisionJob(mock)
	expectNoSpendCeiling(mock)

	resp, err := s.GetAccountBillingDetail(context.Background(), &adminv1.GetAccountBillingDetailRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccountBillingDetail: %v", err)
	}
	if resp.Coverage != "unknown" {
		t.Errorf("coverage = %q, want unknown when the lookup failed", resp.Coverage)
	}
	if len(resp.Warnings) != 1 {
		t.Fatalf("warnings = %v, want one", resp.Warnings)
	}
	if resp.ProvisionedAt == "" {
		t.Error("provisioned_at is empty; the astro-side record must survive a vendor failure")
	}
}

// Zero and absent are different: an exhausted account reports 0 remaining, and
// a failed lookup must not be rendered as the same thing.
func TestGetAccountBillingDetail_SpendDistinguishesZeroFromMissing(t *testing.T) {
	for _, tc := range []struct {
		name        string
		spend       billing.Spend
		spendErr    error
		wantSpend   bool
		wantWarning int
	}{
		{
			name:      "exhausted credit reports zero",
			spend:     billing.Spend{Currency: "USD", CreditRemaining: 0, HasCredit: true},
			wantSpend: true,
		},
		{
			name:        "failed lookup reports nothing",
			spendErr:    errors.New("metronome down"),
			wantSpend:   false,
			wantWarning: 1,
		},
		// Spend is read in parts. A part that failed must not discard the credit
		// balance a part that succeeded already returned, or one slow invoice
		// endpoint hides the number gating turns on.
		{
			name:        "partial read keeps what was read",
			spend:       billing.Spend{Currency: "USD", CreditRemaining: 250, HasCredit: true},
			spendErr:    errors.New("metronome list DRAFT invoices: 500"),
			wantSpend:   true,
			wantWarning: 1,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			s := &Server{
				db:          db,
				deployStore: deploymentstore.NewStore(db),
				log:         logger.New("error", "json"),
				billingProvider: stubInspector{
					coverage: billing.Coverage{State: billing.CoverageCovered},
					spend:    tc.spend,
					spendErr: tc.spendErr,
				},
			}

			expectBillingIDs(mock, "acct-1", "cust-1", true)
			expectNoBillingStatusRow(mock, "acct-1")
			expectSuspendedWorkloads(mock, "acct-1", false)
			expectNoProvisionJob(mock)
			expectNoSpendCeiling(mock)

			resp, err := s.GetAccountBillingDetail(context.Background(), &adminv1.GetAccountBillingDetailRequest{AccountID: "acct-1"})
			if err != nil {
				t.Fatalf("GetAccountBillingDetail: %v", err)
			}
			if (resp.Spend != nil) != tc.wantSpend {
				t.Fatalf("spend = %+v, want present=%v", resp.Spend, tc.wantSpend)
			}
			if tc.wantSpend && !resp.Spend.HasCredit {
				t.Error("has_credit is false; zero remaining would render as no data")
			}
			if len(resp.Warnings) != tc.wantWarning {
				t.Errorf("warnings = %v, want %d", resp.Warnings, tc.wantWarning)
			}
		})
	}
}

func TestRetryBillingProvision_EnqueuesWhenUnprovisioned(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{db: db, queue: q, log: logger.New("error", "json")}

	mock.ExpectQuery("SELECT billing_provisioned_at FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"billing_provisioned_at"}).AddRow(nil))

	resp, err := s.RetryBillingProvision(context.Background(), &adminv1.RetryBillingProvisionRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("RetryBillingProvision: %v", err)
	}
	if resp.Status != "enqueued" {
		t.Errorf("status = %q, want enqueued", resp.Status)
	}
	if len(q.billingProvisionCalls) != 1 {
		t.Errorf("provision enqueues = %v, want one", q.billingProvisionCalls)
	}
}

// Re-provisioning a done account would grant a second signup credit if the
// uniqueness key ever changed, so the guard is here rather than only provider-side.
func TestRetryBillingProvision_SkipsProvisionedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{db: db, queue: q, log: logger.New("error", "json")}

	mock.ExpectQuery("SELECT billing_provisioned_at FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"billing_provisioned_at"}).AddRow(time.Now()))

	resp, err := s.RetryBillingProvision(context.Background(), &adminv1.RetryBillingProvisionRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("RetryBillingProvision: %v", err)
	}
	if resp.Status != "already_provisioned" {
		t.Errorf("status = %q, want already_provisioned", resp.Status)
	}
	if len(q.billingProvisionCalls) != 0 {
		t.Errorf("provision enqueues = %v, want none", q.billingProvisionCalls)
	}
}

// Force is the only way to clear a credits_exhausted latch from Queen: the job
// ends by granting credit, and every account holding a latch is by definition
// already provisioned, so the unforced guard would always refuse.
func TestRetryBillingProvision_ForceRerunsProvisionedAccount(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	q := &mockAdminJobQueue{}
	s := &Server{db: db, queue: q, log: logger.New("error", "json")}

	mock.ExpectQuery("SELECT billing_provisioned_at FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"billing_provisioned_at"}).AddRow(time.Now()))

	resp, err := s.RetryBillingProvision(context.Background(),
		&adminv1.RetryBillingProvisionRequest{AccountID: "acct-1", Force: true})
	if err != nil {
		t.Fatalf("RetryBillingProvision: %v", err)
	}
	if resp.Status != "enqueued" {
		t.Errorf("status = %q, want enqueued", resp.Status)
	}
	if len(q.billingProvisionCalls) != 1 {
		t.Errorf("provision enqueues = %v, want one", q.billingProvisionCalls)
	}
}

func TestForceBillingResume_OnlyWhenSuspended(t *testing.T) {
	for _, tc := range []struct {
		name       string
		suspended  bool
		wantStatus string
		wantCalls  int
	}{
		{"suspended", true, "enqueued", 1},
		{"nothing suspended", false, "nothing_suspended", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			q := &mockAdminJobQueue{}
			s := &Server{
				db:          db,
				queue:       q,
				deployStore: deploymentstore.NewStore(db),
				log:         logger.New("error", "json"),
			}

			expectSuspendedWorkloads(mock, "acct-1", tc.suspended)

			resp, err := s.ForceBillingResume(context.Background(), &adminv1.ForceBillingResumeRequest{AccountID: "acct-1"})
			if err != nil {
				t.Fatalf("ForceBillingResume: %v", err)
			}
			if resp.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", resp.Status, tc.wantStatus)
			}
			if len(q.billingResumeCalls) != tc.wantCalls {
				t.Errorf("resume enqueues = %v, want %d", q.billingResumeCalls, tc.wantCalls)
			}
		})
	}
}

type stubThresholdWriter struct {
	billing.BillingProvider
	set     []float64
	cleared int
	err     error
}

func (w *stubThresholdWriter) SetCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind, amount float64) error {
	if kind == billing.SpendThresholdLimit {
		w.set = append(w.set, amount)
	}
	return w.err
}

func (w *stubThresholdWriter) ClearCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind) error {
	if kind == billing.SpendThresholdLimit {
		w.cleared++
	}
	return w.err
}

func spendLimitServer(t *testing.T, writer billing.BillingProvider) (*Server, sqlmock.Sqlmock, *mockAdminJobQueue) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	q := &mockAdminJobQueue{}
	return &Server{db: db, queue: q, billingProvider: writer, log: logger.New("error", "json")}, mock, q
}

func TestSetAccountSpendLimit_WritesTheLimitInMinorUnits(t *testing.T) {
	w := &stubThresholdWriter{}
	s, mock, q := spendLimitServer(t, w)

	mock.ExpectQuery("SELECT COALESCE\\(metronome_customer_id").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))
	mock.ExpectExec("INSERT INTO account_limits").
		WithArgs("acct-1", quota.KeySpendLimit, int64(5000)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(5000))

	resp, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", LimitUSD: 5000,
	})
	if err != nil {
		t.Fatalf("SetAccountSpendLimit: %v", err)
	}
	if len(w.set) != 1 || w.set[0] != 500_000 {
		t.Errorf("provider got %v, want one write of 500000 cents", w.set)
	}
	if resp.Status != "set" || resp.LimitUSD != 5000 || resp.CeilingUSD != 5000 {
		t.Errorf("resp = %+v", resp)
	}
	if len(q.gatewayBudgetCalls) != 1 {
		t.Errorf("gateway budget re-derives = %v, want one", q.gatewayBudgetCalls)
	}
}

// Leaving the ceiling behind would clamp the gateway back to the lower number.
func TestSetAccountSpendLimit_RaisesTheCeilingToMatch(t *testing.T) {
	w := &stubThresholdWriter{}
	s, mock, _ := spendLimitServer(t, w)

	mock.ExpectQuery("SELECT COALESCE\\(metronome_customer_id").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))
	mock.ExpectExec("INSERT INTO account_limits").
		WithArgs("acct-1", quota.KeySpendLimit, int64(25000)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(25000))

	resp, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", LimitUSD: 25000,
	})
	if err != nil {
		t.Fatalf("SetAccountSpendLimit: %v", err)
	}
	if resp.CeilingUSD != 25000 {
		t.Errorf("ceiling = %v, want 25000", resp.CeilingUSD)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
}

func TestSetAccountSpendLimit_LimitUnderTheDefaultWritesNoCeiling(t *testing.T) {
	w := &stubThresholdWriter{}
	s, mock, _ := spendLimitServer(t, w)

	mock.ExpectQuery("SELECT COALESCE\\(metronome_customer_id").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}))

	if _, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", LimitUSD: 200,
	}); err != nil {
		t.Fatalf("SetAccountSpendLimit: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
}

func TestSetAccountSpendLimit_ClearLeavesTheCeiling(t *testing.T) {
	w := &stubThresholdWriter{}
	s, mock, _ := spendLimitServer(t, w)

	mock.ExpectQuery("SELECT COALESCE\\(metronome_customer_id").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))
	mock.ExpectQuery("FROM account_limits").
		WillReturnRows(sqlmock.NewRows([]string{"limit_value"}).AddRow(5000))

	resp, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", Clear: true,
	})
	if err != nil {
		t.Fatalf("SetAccountSpendLimit: %v", err)
	}
	if w.cleared != 1 || len(w.set) != 0 {
		t.Errorf("cleared = %d, set = %v", w.cleared, w.set)
	}
	if resp.Status != "cleared" || resp.CeilingUSD != 5000 {
		t.Errorf("resp = %+v, want the ceiling to survive", resp)
	}
}

func TestSetAccountSpendLimit_RejectsNonPositiveWithoutClear(t *testing.T) {
	s, _, _ := spendLimitServer(t, &stubThresholdWriter{})
	if _, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", LimitUSD: 0,
	}); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("err = %v, want InvalidArgument", err)
	}
}

func TestSetAccountSpendLimit_NoBillingCustomer(t *testing.T) {
	s, mock, _ := spendLimitServer(t, &stubThresholdWriter{})
	mock.ExpectQuery("SELECT COALESCE\\(metronome_customer_id").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow(""))

	if _, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", LimitUSD: 500,
	}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("err = %v, want FailedPrecondition", err)
	}
}

func TestSetAccountSpendLimit_ProviderFailureIsAnError(t *testing.T) {
	w := &stubThresholdWriter{err: errors.New("metronome down")}
	s, mock, _ := spendLimitServer(t, w)
	mock.ExpectQuery("SELECT COALESCE\\(metronome_customer_id").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	if _, err := s.SetAccountSpendLimit(context.Background(), &adminv1.SetAccountSpendLimitRequest{
		AccountID: "acct-1", LimitUSD: 500,
	}); err == nil {
		t.Fatal("want the provider failure")
	}
}
