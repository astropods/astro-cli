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
)

// stubInspector stands in for Metronome.
type stubInspector struct {
	billing.BillingProvider
	coverage billing.Coverage
	err      error
	spend    billing.Spend
	spendErr error
}

func (s stubInspector) ContractCoverage(context.Context, string, string) (billing.Coverage, error) {
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

// A stray contract must be reported as foreign rather than absent: "none" reads
// as safe to provision, which is exactly the double-billing case.
func TestGetAccountBillingDetail_ForeignContractBlocks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		db:          db,
		deployStore: deploymentstore.NewStore(db),
		log:         logger.New("error", "json"),
		billingProvider: stubInspector{coverage: billing.Coverage{
			State: billing.CoverageForeign,
			Contracts: []billing.Contract{{
				ID:         "con_stray",
				StartingAt: time.Now().Add(-time.Hour),
			}},
		}},
	}

	expectBillingIDs(mock, "acct-1", "cust-1", false)
	expectNoBillingStatusRow(mock, "acct-1")
	expectSuspendedWorkloads(mock, "acct-1", false)
	expectNoProvisionJob(mock)

	resp, err := s.GetAccountBillingDetail(context.Background(), &adminv1.GetAccountBillingDetailRequest{AccountID: "acct-1"})
	if err != nil {
		t.Fatalf("GetAccountBillingDetail: %v", err)
	}
	if resp.Coverage != billing.CoverageForeign {
		t.Errorf("coverage = %q, want %q", resp.Coverage, billing.CoverageForeign)
	}
	if len(resp.Contracts) != 1 || resp.Contracts[0].Ours {
		t.Errorf("contracts = %+v, want one contract not marked ours", resp.Contracts)
	}
	if resp.MetronomeURL == "" {
		t.Error("metronome_url is empty; the operator has no way to reach the stray contract")
	}
}

// Without the environment segment the link opens the default environment and
// reports no such customer, which reads as missing data rather than a bad link.
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

// A vendor failure must degrade the view, not empty it.
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

// Zero and absent differ: exhausted reports 0, a failed lookup reports nothing.
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
		// A failed part must not discard a balance an earlier part returned.
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
					coverage: billing.Coverage{State: billing.CoverageOurs},
					spend:    tc.spend,
					spendErr: tc.spendErr,
				},
			}

			expectBillingIDs(mock, "acct-1", "cust-1", true)
			expectNoBillingStatusRow(mock, "acct-1")
			expectSuspendedWorkloads(mock, "acct-1", false)
			expectNoProvisionJob(mock)

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

// A second signup credit if the uniqueness key ever changed, so guard here too.
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
