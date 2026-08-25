package riverqueue

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// budgetHarness wires the worker to a fake gateway and returns the limit the
// gateway was asked for.
func budgetHarness(t *testing.T, hasCard bool, bifrostID string) (float64, int) {
	t.Helper()

	var limit float64
	var puts int
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		puts++
		var body struct {
			Budgets []struct {
				MaxLimit float64 `json:"max_limit"`
			} `json:"budgets"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Budgets) == 1 {
			limit = body.Budgets[0].MaxLimit
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer gw.Close()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT bifrost_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"bifrost_customer_id"}).AddRow(bifrostID))
	if bifrostID != "" {
		mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
			WillReturnRows(sqlmock.NewRows([]string{
				"status", "reason", "dunning_since", "alert_active", "force_suspended",
				"credits_exhausted", "has_payment_method", "pay_link", "usage_limit_active", "not_provisioned",
			}).AddRow("active", "", nil, false, false, false, hasCard, nil, false, false))
	}

	w := &BillingGatewayBudgetWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7),
		gateway:  aigateway.NewClient("https://aig.example", gw.URL, ""),
		log:      logger.New("error", "json"),
	}
	if err := w.Work(context.Background(), &river.Job[BillingGatewayBudgetArgs]{
		Args: BillingGatewayBudgetArgs{AccountID: "acct-1"},
	}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
	return limit, puts
}

// Without a card the signup credit is the whole of what we can collect, so the
// gateway ceiling must not exceed it. A wider ceiling is uncollectible spend by
// construction.
func TestGatewayBudget_CardlessAccountGetsTheCreditCeiling(t *testing.T) {
	limit, puts := budgetHarness(t, false, "bf-1")
	if puts != 1 {
		t.Fatalf("gateway calls = %d, want 1", puts)
	}
	if limit != aigateway.CardlessBudgetUSD {
		t.Errorf("limit = %v, want %v", limit, aigateway.CardlessBudgetUSD)
	}
}

func TestGatewayBudget_CardedAccountGetsTheWiderCeiling(t *testing.T) {
	limit, puts := budgetHarness(t, true, "bf-1")
	if puts != 1 {
		t.Fatalf("gateway calls = %d, want 1", puts)
	}
	if limit != aigateway.CardedBudgetUSD {
		t.Errorf("limit = %v, want %v", limit, aigateway.CardedBudgetUSD)
	}
}

// An account that has never minted a key has no gateway customer, and creating
// one here would provision a tenant that never asked for the gateway.
func TestGatewayBudget_NoGatewayCustomerIsANoop(t *testing.T) {
	if _, puts := budgetHarness(t, false, ""); puts != 0 {
		t.Errorf("gateway calls = %d, want 0", puts)
	}
}

// limitProvider reports one spend limit and nothing else. The embedded interface
// is nil, so any other call would panic rather than pass silently.
type limitProvider struct {
	billing.BillingProvider
	limitCents float64
	hasLimit   bool
	err        error
}

func (p limitProvider) CustomerSpendThresholds(context.Context, string) (billing.SpendThresholds, error) {
	if p.err != nil {
		return billing.SpendThresholds{}, p.err
	}
	return billing.SpendThresholds{
		HasLimit: p.hasLimit,
		Limit:    billing.SpendThreshold{Amount: p.limitCents},
	}, nil
}

// limitHarness is budgetHarness with a provider that reports the account's own
// spend limit.
func limitHarness(t *testing.T, provider billing.BillingProvider, billingID string) (float64, error) {
	t.Helper()

	var limit float64
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Budgets []struct {
				MaxLimit float64 `json:"max_limit"`
			} `json:"budgets"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Budgets) == 1 {
			limit = body.Budgets[0].MaxLimit
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer gw.Close()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT bifrost_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"bifrost_customer_id"}).AddRow("bf-1"))
	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"status", "reason", "dunning_since", "alert_active", "force_suspended",
			"credits_exhausted", "has_payment_method", "pay_link", "usage_limit_active", "not_provisioned",
		}).AddRow("active", "", nil, false, false, false, true, nil, false, false))
	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow(billingID))

	w := &BillingGatewayBudgetWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7),
		gateway:  aigateway.NewClient("https://aig.example", gw.URL, ""),
		provider: provider,
		backend:  "metronome",
		log:      logger.New("error", "json"),
	}
	return limit, w.Work(context.Background(), &river.Job[BillingGatewayBudgetArgs]{
		Args: BillingGatewayBudgetArgs{AccountID: "acct-1"},
	})
}

// The customer sets one limit for total account spend. A gateway ceiling below
// it caps them under the figure they chose, which is the bug: $500 set, $20
// enforced.
func TestGatewayBudget_CeilingTracksTheAccountsOwnLimit(t *testing.T) {
	limit, err := limitHarness(t, limitProvider{hasLimit: true, limitCents: 50000}, "cust-1")
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	// 50000 cents is $500. Sending the minor units would ask the gateway for a
	// $50,000 ceiling.
	if limit != 500 {
		t.Errorf("limit = %v, want 500", limit)
	}
	if limit == aigateway.CardedBudgetUSD {
		t.Error("the ceiling is still the card default, so the limit was ignored")
	}
}

// An account with no limit of its own must not end up uncapped.
func TestGatewayBudget_NoLimitFallsBackToTheCardDefault(t *testing.T) {
	limit, err := limitHarness(t, limitProvider{hasLimit: false}, "cust-1")
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if limit != aigateway.CardedBudgetUSD {
		t.Errorf("limit = %v, want the carded default %v", limit, aigateway.CardedBudgetUSD)
	}
}

// An unreadable limit must not silently widen or narrow the ceiling: leave the
// budget as it stands and let the job retry.
func TestGatewayBudget_UnreadableLimitFails(t *testing.T) {
	limit, err := limitHarness(t, limitProvider{err: errors.New("provider down")}, "cust-1")
	if err == nil {
		t.Fatal("want the read failure")
	}
	if limit != 0 {
		t.Errorf("the gateway was written to anyway: limit = %v", limit)
	}
}

// A limit reaches the provider by paths that never touch the handler bounding
// it: an admin sets one directly, a backfill writes one. Clamping here is what
// stops a number nobody vetted becoming our exposure.
func TestGatewayBudget_ALimitAboveSelfServeIsClamped(t *testing.T) {
	limit, err := limitHarness(t, limitProvider{hasLimit: true, limitCents: 5_000_000}, "cust-1")
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if limit != billing.MaxSelfServeSpendUSD {
		t.Errorf("limit = %v, want the self-serve ceiling %v", limit, billing.MaxSelfServeSpendUSD)
	}
}

// fakeSweepStore serves one slice and records the stamps it was asked for. The
// stamps are the anti-starvation mechanism, so a test has to see them.
type fakeSweepStore struct {
	ids           []string
	limit         int
	stamped       []string
	stampDeadline bool
	listErr       error
	stampErr      error
}

func (f *fakeSweepStore) ListStaleGatewayBudgetAccounts(_ context.Context, limit int) ([]string, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	f.limit = limit
	return f.ids, nil
}

func (f *fakeSweepStore) MarkGatewayBudgetSwept(ctx context.Context, id string) error {
	// The real store fails on a dead context, so the fake has to as well or a
	// test cannot tell a detached stamp from one that shares the tick's deadline.
	if err := ctx.Err(); err != nil {
		return err
	}
	_, f.stampDeadline = ctx.Deadline()
	f.stamped = append(f.stamped, id)
	return f.stampErr
}

func sweepHarness(t *testing.T, store *fakeSweepStore, fail map[string]bool) ([]string, error) {
	t.Helper()
	var applied []string
	w := &BillingGatewayBudgetSweepWorker{
		accounts: store,
		log:      logger.New("error", "json"),
		apply: func(_ context.Context, id string) error {
			if fail[id] {
				return errors.New("gateway down")
			}
			applied = append(applied, id)
			return nil
		},
	}
	err := w.Work(context.Background(), &river.Job[BillingGatewayBudgetSweepArgs]{
		Args: BillingGatewayBudgetSweepArgs{},
	})
	return applied, err
}

func TestGatewayBudgetSweep_AppliesEveryAccountInTheSlice(t *testing.T) {
	store := &fakeSweepStore{ids: []string{"a", "b", "c"}}
	applied, err := sweepHarness(t, store, nil)
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(applied) != 3 {
		t.Errorf("applied = %v, want all three", applied)
	}
	if store.limit != gatewayBudgetSweepLimit {
		t.Errorf("limit = %d, want the tick bounded at %d", store.limit, gatewayBudgetSweepLimit)
	}
}

// The stamp is what moves an account to the back of the ordering. Skipping it on
// failure leaves that account first on every future tick, so a single
// permanently broken account would hold the slice and nothing else would ever be
// swept.
func TestGatewayBudgetSweep_StampsFailedAccountsToo(t *testing.T) {
	store := &fakeSweepStore{ids: []string{"a", "b", "c"}}
	if _, err := sweepHarness(t, store, map[string]bool{"b": true}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(store.stamped) != 3 {
		t.Fatalf("stamped = %v, want every account including the failure", store.stamped)
	}
	if !slices.Contains(store.stamped, "b") {
		t.Errorf("stamped = %v, want the failed account b stamped", store.stamped)
	}
}

// Stopping on the first blip would cost every later account its sweep.
func TestGatewayBudgetSweep_OneFailureDoesNotStopTheRest(t *testing.T) {
	applied, err := sweepHarness(t, &fakeSweepStore{ids: []string{"a", "b", "c"}}, map[string]bool{"b": true})
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if len(applied) != 2 || applied[0] != "a" || applied[1] != "c" {
		t.Errorf("applied = %v, want a and c", applied)
	}
}

// An unreadable worklist is not a per-account failure: returning it lets River
// retry instead of logging a sweep that silently did nothing.
func TestGatewayBudgetSweep_ListFailureIsRetried(t *testing.T) {
	if _, err := sweepHarness(t, &fakeSweepStore{listErr: errors.New("db down")}, nil); err == nil {
		t.Error("want the list failure returned")
	}
}

// A tick killed part way has to stop, not run on with a dead context. The
// accounts it never reached stay unstamped, which is what puts them at the front
// of the next tick.
func TestGatewayBudgetSweep_CancellationLeavesTheRestUnstamped(t *testing.T) {
	store := &fakeSweepStore{ids: []string{"a", "b", "c", "d"}}
	ctx, cancel := context.WithCancel(context.Background())
	var applied int
	w := &BillingGatewayBudgetSweepWorker{
		accounts: store,
		log:      logger.New("error", "json"),
		apply: func(_ context.Context, _ string) error {
			applied++
			if applied == 2 {
				cancel()
			}
			return nil
		},
	}
	defer cancel()
	if err := w.Work(ctx, &river.Job[BillingGatewayBudgetSweepArgs]{}); err == nil {
		t.Fatal("want the cancellation returned so River retries the tick")
	}
	if len(store.stamped) != 2 {
		t.Errorf("stamped = %v, want only the two that were applied", store.stamped)
	}
}

// An account slow enough to exhaust the tick's deadline is the one case where
// the stamp matters most, and the one where it shares the failure. Unstamped, it
// leads the ordering on every tick and nothing behind it is ever swept.
func TestGatewayBudgetSweep_StampsAnAccountWhoseApplyExhaustedTheDeadline(t *testing.T) {
	store := &fakeSweepStore{ids: []string{"slow", "b"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &BillingGatewayBudgetSweepWorker{
		accounts: store,
		log:      logger.New("error", "json"),
		apply: func(applyCtx context.Context, _ string) error {
			cancel()
			return applyCtx.Err()
		},
	}
	if err := w.Work(ctx, &river.Job[BillingGatewayBudgetSweepArgs]{}); err == nil {
		t.Fatal("want the cancellation returned so River retries the tick")
	}
	if !slices.Contains(store.stamped, "slow") {
		t.Errorf("stamped = %v, want the slow account stamped so it stops leading every tick", store.stamped)
	}
	// Detached from the tick, so it needs a deadline of its own or a stuck
	// database holds the tick open past the one River gave it.
	if !store.stampDeadline {
		t.Error("the stamp context has no deadline")
	}
}

// The tick has to outlive its bounded slice, or the count above stops being the
// bound and River's default becomes an invisible one.
func TestGatewayBudgetSweep_TimeoutExceedsRiverDefault(t *testing.T) {
	w := &BillingGatewayBudgetSweepWorker{}
	if got := w.Timeout(nil); got <= time.Minute {
		t.Errorf("Timeout = %v, want more than River's one-minute default", got)
	}
}

// The sweep and the per-account job must derive the ceiling the same way, so the
// wiring has to reach the real applyBudget, not a copy of its logic.
func TestGatewayBudgetSweep_AppliesThroughTheRealWorker(t *testing.T) {
	var puts int
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		puts++
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer gw.Close()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	for _, id := range []string{"acct-1", "acct-2"} {
		mock.ExpectQuery("SELECT bifrost_customer_id FROM accounts").WithArgs(id).
			WillReturnRows(sqlmock.NewRows([]string{"bifrost_customer_id"}).AddRow("bf-" + id))
		mock.ExpectQuery("FROM account_billing_status").WithArgs(id).
			WillReturnRows(sqlmock.NewRows([]string{
				"status", "reason", "dunning_since", "alert_active", "force_suspended",
				"credits_exhausted", "has_payment_method", "pay_link", "usage_limit_active", "not_provisioned",
			}).AddRow("active", "", nil, false, false, false, true, nil, false, false))
	}

	budget := &BillingGatewayBudgetWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7),
		gateway:  aigateway.NewClient("https://aig.example", gw.URL, ""),
		log:      logger.New("error", "json"),
	}
	sweep := &BillingGatewayBudgetSweepWorker{
		apply:    budget.applyBudget,
		accounts: &fakeSweepStore{ids: []string{"acct-1", "acct-2"}},
		log:      logger.New("error", "json"),
	}
	if err := sweep.Work(context.Background(), &river.Job[BillingGatewayBudgetSweepArgs]{}); err != nil {
		t.Fatalf("Work: %v", err)
	}
	if puts != 2 {
		t.Errorf("gateway calls = %d, want 2", puts)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("db: %v", err)
	}
}

// exemptLimitHarness is limitHarness with the account in the never-suspend set.
func exemptLimitHarness(t *testing.T, provider billing.BillingProvider, hasCard bool) (float64, error) {
	t.Helper()

	var limit float64
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Budgets []struct {
				MaxLimit float64 `json:"max_limit"`
			} `json:"budgets"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if len(body.Budgets) == 1 {
			limit = body.Budgets[0].MaxLimit
		}
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer gw.Close()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	mock.ExpectQuery("SELECT bifrost_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"bifrost_customer_id"}).AddRow("bf-1"))
	mock.ExpectQuery("FROM account_billing_status").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{
			"status", "reason", "dunning_since", "alert_active", "force_suspended",
			"credits_exhausted", "has_payment_method", "pay_link", "usage_limit_active", "not_provisioned",
		}).AddRow("active", "", nil, false, false, false, hasCard, nil, false, false))
	mock.ExpectQuery("SELECT metronome_customer_id FROM accounts").WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"metronome_customer_id"}).AddRow("cust-1"))

	w := &BillingGatewayBudgetWorker{
		accounts: account.NewAccountStore(db),
		status:   billing.NewStatusStore(db, 7).WithExemptAccounts([]string{"acct-1"}),
		gateway:  aigateway.NewClient("https://aig.example", gw.URL, ""),
		provider: provider,
		backend:  "metronome",
		log:      logger.New("error", "json"),
	}
	return limit, w.Work(context.Background(), &river.Job[BillingGatewayBudgetArgs]{
		Args: BillingGatewayBudgetArgs{AccountID: "acct-1"},
	})
}

// Provisioning seeds every account a $20 limit. An exempt account is one billing
// never suspends, so letting that seeded figure through would stop it at $20 by
// the one control the exemption does not reach.
func TestGatewayBudget_ExemptAccountFloorsAtTheStandardCeiling(t *testing.T) {
	limit, err := exemptLimitHarness(t, limitProvider{hasLimit: true, limitCents: 2000}, true)
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if limit != billing.MaxSelfServeSpendUSD {
		t.Errorf("limit = %v, want the standard ceiling %v", limit, billing.MaxSelfServeSpendUSD)
	}
}

// An account with no limit falls through to the card default, which is lower
// still, so the floor has to cover that path too.
func TestGatewayBudget_ExemptAccountWithNoLimitStillFloors(t *testing.T) {
	limit, err := exemptLimitHarness(t, limitProvider{hasLimit: false}, false)
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if limit == aigateway.CardlessBudgetUSD {
		t.Fatalf("limit = %v, the cardless default capped an exempt account", limit)
	}
	if limit != billing.MaxSelfServeSpendUSD {
		t.Errorf("limit = %v, want the standard ceiling %v", limit, billing.MaxSelfServeSpendUSD)
	}
}

// Raising an exempt account past the floor is an operator action: a limit set
// above the self-serve bound has to survive, since that bound governs what a
// customer can choose for itself and not what an operator grants.
func TestGatewayBudget_ExemptOperatorLimitIsNotClamped(t *testing.T) {
	limit, err := exemptLimitHarness(t, limitProvider{hasLimit: true, limitCents: 2_500_000}, true)
	if err != nil {
		t.Fatalf("Work: %v", err)
	}
	if limit == billing.MaxSelfServeSpendUSD {
		t.Fatalf("limit = %v, the self-serve bound clamped an operator raise", limit)
	}
	if limit != 25000 {
		t.Errorf("limit = %v, want the operator limit 25000", limit)
	}
}

// An exempt account can be raised above the floor by an operator, so falling
// back to the floor on an unreadable limit would demote it every time the
// provider is unreachable. Writing nothing leaves the granted ceiling standing.
func TestGatewayBudget_ExemptUnreadableLimitWritesNothing(t *testing.T) {
	limit, err := exemptLimitHarness(t, limitProvider{err: errors.New("provider down")}, false)
	if err == nil {
		t.Fatal("want the read failure returned so the job retries")
	}
	if limit != 0 {
		t.Errorf("the gateway was written to anyway: limit = %v", limit)
	}
}
