package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/aigateway"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/quota"
)

// BillingGatewayBudgetArgs re-derives one account's AI gateway spend ceiling.
type BillingGatewayBudgetArgs struct {
	AccountID string `json:"account_id"`
}

func (BillingGatewayBudgetArgs) Kind() string { return "billing.gateway_budget" }

func (BillingGatewayBudgetArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

func init() {
	registerJobKind[BillingGatewayBudgetArgs]()
}

// BillingGatewayBudgetWorker keeps the gateway ceiling in step with the inputs it
// is derived from. The gateway enforces in real time and the billing provider
// does not, so this is the only control that stops an uncollectible account
// inside the minutes it takes to run up a bill. Every run re-derives and
// re-applies rather than acting on a transition.
type BillingGatewayBudgetWorker struct {
	river.WorkerDefaults[BillingGatewayBudgetArgs]
	accounts *account.AccountStore
	status   *billing.StatusStore
	gateway  *aigateway.Client
	// provider reads the account's own spend limit. Absent, or not a
	// SpendThresholdReader, and the ceiling falls back to the card default.
	provider billing.BillingProvider
	backend  string
	// db reads the granted spend ceiling. Nil leaves the self-serve default.
	db  *sql.DB
	log *logger.Logger
}

func (w *BillingGatewayBudgetWorker) Work(ctx context.Context, job *river.Job[BillingGatewayBudgetArgs]) error {
	return w.applyBudget(ctx, job.Args.AccountID)
}

// applyBudget is shared with the sweep, so the periodic and event paths cannot
// derive a different ceiling.
func (w *BillingGatewayBudgetWorker) applyBudget(ctx context.Context, accountID string) error {
	if w.accounts == nil || w.status == nil || w.gateway == nil {
		return nil
	}
	customerID, err := w.accounts.GetBifrostCustomerID(accountID)
	if err != nil {
		return err
	}
	// No key ever minted, so no gateway customer to hold a budget. The one it
	// gets at creation already carries the card-less ceiling.
	if customerID == "" {
		return nil
	}
	rec, err := w.status.Record(ctx, accountID)
	if err != nil {
		return err
	}
	limit, source, err := w.ceilingUSD(ctx, accountID, rec)
	if err != nil {
		return err
	}
	if err := w.gateway.SetCustomerBudget(ctx, customerID, limit); err != nil {
		return err
	}
	w.log.Info("billing gateway budget: applied", "account_id", accountID, "limit_usd", limit, "source", source)
	return nil
}

// ceilingUSD is the account's own spend limit, in dollars.
//
// A customer sets one limit for total account spend, compute units and gateway
// together. Holding a separate gateway number either caps them below the figure
// they chose or lets the gateway alone exhaust it, so the ceiling tracks the
// limit instead. Matching it does mean the gateway could spend the whole limit
// on its own; the provider still suspends on the combined total, and this is the
// control that stops an uncollectible account inside the minutes that takes.
//
// An account with no limit falls back to the card-derived default rather than
// running uncapped.
func (w *BillingGatewayBudgetWorker) ceilingUSD(ctx context.Context, accountID string, rec billing.StatusRecord) (float64, string, error) {
	exempt := w.status.IsExempt(accountID)
	// A failed read means the limit is unknown, so nothing is written. Falling
	// back to the floor here would overwrite an operator-raised ceiling with a
	// lower one every time the provider is unreachable, which is worse than
	// leaving the account on the number it already has.
	limitUSD, hasLimit, err := w.spendLimitUSD(ctx, accountID)
	if err != nil {
		return 0, "", err
	}
	ceiling, err := quota.SpendCeilingUSD(ctx, w.db, accountID)
	if err != nil {
		return 0, "", err
	}

	if exempt {
		// The floor is the account's ceiling, not the seeded limit, which is far
		// below it. Raising an exempt account past the floor is an operator
		// action: set a higher limit on the account and it is honoured here
		// unclamped, because the self-serve bound governs what a customer can
		// choose for itself, not what an operator grants.
		if hasLimit && limitUSD > ceiling {
			return limitUSD, "exempt_operator_limit", nil
		}
		return ceiling, "exempt_floor", nil
	}

	if hasLimit {
		// The handler bounds what a customer can set, but a limit can reach the
		// provider without passing it: an admin or a backfill writes there
		// directly.
		if limitUSD > ceiling {
			return ceiling, "spend_limit_clamped", nil
		}
		return limitUSD, "spend_limit", nil
	}
	if rec.HasPaymentMethod {
		return aigateway.CardedBudgetUSD, "default_carded", nil
	}
	return aigateway.CardlessBudgetUSD, "default_cardless", nil
}

// spendLimitUSD reads the account's own spend limit, reporting false when it has
// none or the provider cannot supply one.
func (w *BillingGatewayBudgetWorker) spendLimitUSD(ctx context.Context, accountID string) (float64, bool, error) {
	reader, ok := w.provider.(billing.SpendThresholdReader)
	if !ok || w.backend == "" {
		return 0, false, nil
	}
	billingID, err := w.accounts.GetBillingCustomerID(accountID, w.backend)
	if err != nil {
		return 0, false, fmt.Errorf("load billing customer id: %w", err)
	}
	if billingID == "" {
		return 0, false, nil
	}
	thresholds, err := reader.CustomerSpendThresholds(ctx, billingID)
	if err != nil {
		return 0, false, fmt.Errorf("read spend limit: %w", err)
	}
	if !thresholds.HasLimit {
		return 0, false, nil
	}
	// The provider stores thresholds in minor units.
	return thresholds.Limit.Amount / 100, true, nil
}

// gatewayBudgetSweepLimit bounds one tick. The bound is a count we choose rather
// than the job timeout, which would otherwise impose one we cannot see.
const gatewayBudgetSweepLimit = 500

// gatewayBudgetStampTimeout bounds the stamp that outlives a cut-short tick.
const gatewayBudgetStampTimeout = 5 * time.Second

// BillingGatewayBudgetSweepArgs re-applies the gateway ceiling for the accounts
// whose ceiling was applied longest ago.
type BillingGatewayBudgetSweepArgs struct{}

func (BillingGatewayBudgetSweepArgs) Kind() string { return "billing.gateway_budget_sweep" }

func (BillingGatewayBudgetSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

func init() {
	registerJobKind[BillingGatewayBudgetSweepArgs]()
}

// gatewayBudgetSweepStore is satisfied by *account.AccountStore.
type gatewayBudgetSweepStore interface {
	ListStaleGatewayBudgetAccounts(ctx context.Context, limit int) ([]string, error)
	MarkGatewayBudgetSwept(ctx context.Context, accountID string) error
}

// BillingGatewayBudgetSweepWorker re-derives the ceiling for the accounts that
// have gone longest without one.
//
// Nothing forces a writer that moves one of the ceiling's inputs to enqueue a
// re-derive, and a miss is wrong in both directions: too high and the spend is
// uncollectible, too low and a paying customer is blocked under the limit they
// chose. Sweeping makes a miss a delay instead.
//
// Each tick takes a bounded, staleness-ordered slice rather than the whole table.
// That is what stops a bound from becoming starvation: an account left out of one
// tick is staler next tick, so it sorts earlier, and a tick cut short by
// cancellation loses only work that comes first when the sweep next runs.
type BillingGatewayBudgetSweepWorker struct {
	river.WorkerDefaults[BillingGatewayBudgetSweepArgs]
	// apply is BillingGatewayBudgetWorker.applyBudget, so a swept account and an
	// enqueued one derive their ceiling the same way.
	apply    func(ctx context.Context, accountID string) error
	accounts gatewayBudgetSweepStore
	log      *logger.Logger
}

// Timeout gives the tick room to finish its bounded slice, so the count above is
// what limits it. River's default would cut a full slice short.
func (w *BillingGatewayBudgetSweepWorker) Timeout(*river.Job[BillingGatewayBudgetSweepArgs]) time.Duration {
	return 5 * time.Minute
}

// stampSwept records the attempt on a context detached from the tick's deadline.
// The apply and the stamp would otherwise share one, and an account slow enough
// to exhaust it would never be stamped: it would lead the staleness ordering on
// every tick and starve everything behind it. Bounded, so a stuck database
// cannot hold the tick open past its own deadline.
func (w *BillingGatewayBudgetSweepWorker) stampSwept(ctx context.Context, accountID string) {
	stampCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), gatewayBudgetStampTimeout)
	defer cancel()
	if err := w.accounts.MarkGatewayBudgetSwept(stampCtx, accountID); err != nil {
		w.log.Error("billing gateway budget: stamp sweep failed", "account_id", accountID, "error", err)
	}
}

func (w *BillingGatewayBudgetSweepWorker) Work(ctx context.Context, _ *river.Job[BillingGatewayBudgetSweepArgs]) error {
	if w.apply == nil || w.accounts == nil {
		return nil
	}
	ids, err := w.accounts.ListStaleGatewayBudgetAccounts(ctx, gatewayBudgetSweepLimit)
	if err != nil {
		return err
	}
	var applied, failed int
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			// Whatever is left is the stalest, so it leads the next tick.
			w.log.Warn("billing gateway budget: sweep cut short", "applied", applied, "remaining", len(ids)-applied-failed)
			return err
		}
		// One unreachable account must not cost the rest their sweep; it is
		// stamped either way and retried on a later tick.
		if aerr := w.apply(ctx, id); aerr != nil {
			failed++
			w.log.Warn("billing gateway budget: sweep apply failed", "account_id", id, "error", aerr)
		} else {
			applied++
		}
		w.stampSwept(ctx, id)
	}
	if len(ids) > 0 {
		// A full slice means the worklist is longer than one tick covers, so the
		// staleness window is wider than the interval suggests.
		w.log.Info("billing gateway budget: sweep", "applied", applied, "failed", failed,
			"slice_full", len(ids) == gatewayBudgetSweepLimit)
	}
	return nil
}
