package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

const cardExpiryPageSize = 200

// cardExpiryPageCap bounds one tick against a page that never advances. At the
// page size above it allows 100k carded accounts.
const cardExpiryPageCap = 500

type CardExpirySweepArgs struct{}

func (CardExpirySweepArgs) Kind() string { return "billing.card_expiry_sweep" }

func (CardExpirySweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueBilling}
}

func init() {
	registerJobKind[CardExpirySweepArgs]()
}

type cardExpiryQueue interface {
	InsertBillingSuspend(ctx context.Context, accountID string) error
	InsertBillingGatewayBudget(ctx context.Context, accountID string) error
	EmitBillingNotify(ctx context.Context, ev notify.Event) error
}

type cardExpiryAccounts interface {
	accountNamer
	GetStripeCustomerID(accountID string) (string, error)
}

// Expiry is the one card change no provider event announces, so the latch only
// goes stale here, and only a re-read of the vault finds it.
type CardExpirySweepWorker struct {
	river.WorkerDefaults[CardExpirySweepArgs]
	status   *billing.StatusStore
	cards    cardReader
	accounts cardExpiryAccounts // left unset rather than holding a nil *AccountStore
	queue    cardExpiryQueue    // set post-construction in New()
	log      *logger.Logger
}

func (w *CardExpirySweepWorker) Work(ctx context.Context, _ *river.Job[CardExpirySweepArgs]) error {
	if w.status == nil || w.cards == nil || w.accounts == nil {
		return nil
	}
	now := time.Now()
	var evaluated, cleared int
	after := ""
	for page := 0; page < cardExpiryPageCap; page++ {
		ids, err := w.status.ListCarded(ctx, after, cardExpiryPageSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			break
		}
		// Taken before the page is walked: clearing a card drops the row from the
		// filter, so the cursor cannot come from the rows that survive.
		after = ids[len(ids)-1]
		evaluated += len(ids)
		for _, id := range ids {
			expired, err := w.cardExpired(ctx, id, now)
			if err != nil {
				w.log.Error("card expiry sweep: read card failed", "account_id", id, "error", err)
				continue
			}
			if !expired {
				continue
			}
			cleared++
			if err := w.clearCardFact(ctx, id, now); err != nil {
				w.log.Error("card expiry sweep: clear payment method failed", "account_id", id, "error", err)
			}
		}
		if len(ids) < cardExpiryPageSize {
			break
		}
		if page == cardExpiryPageCap-1 {
			w.log.Error("card expiry sweep: stopped at the page cap, some cards went unchecked",
				"pages", cardExpiryPageCap, "evaluated", evaluated)
		}
	}
	if evaluated > 0 {
		w.log.Info("billing card expiry: sweep", "evaluated", evaluated, "cleared", cleared)
	}
	return nil
}

func (w *CardExpirySweepWorker) cardExpired(ctx context.Context, accountID string, now time.Time) (bool, error) {
	customerID, err := w.accounts.GetStripeCustomerID(accountID)
	if err != nil {
		return false, err
	}
	if customerID == "" {
		return false, nil
	}
	card, err := w.cards.DefaultCard(ctx, customerID)
	if err != nil {
		return false, err
	}
	// A card that is gone belongs to the detach webhook.
	return card != nil && card.Expired(now), nil
}

func (w *CardExpirySweepWorker) clearCardFact(ctx context.Context, accountID string, now time.Time) error {
	status, changed, err := billing.ApplySignal(ctx, w.status, accountID, billing.SignalCardRemoved, now)
	if err != nil {
		return err
	}
	w.log.Info("billing card expiry: card expired, payment method cleared",
		"account_id", accountID, "status", string(status))
	if w.queue == nil {
		return nil
	}
	// The suspend goes first and nothing after it can be skipped by an earlier
	// failure. Clearing the latch drops the account out of ListCarded and no sweep
	// scans a suspended row, so a suspend lost here is lost for good, while the
	// gateway ceiling re-derives on its own fifteen-minute sweep.
	if changed && status == billing.StatusSuspended {
		if err := w.queue.InsertBillingSuspend(ctx, accountID); err != nil {
			w.log.Error("card expiry sweep: enqueue suspend failed", "account_id", accountID, "error", err)
		}
		if err := w.queue.EmitBillingNotify(ctx,
			notify.BillingSuspended(accountID, notifyAccountName(w.accounts, w.log, accountID))); err != nil {
			w.log.Warn("card expiry sweep: emit suspended notification failed", "account_id", accountID, "error", err)
		}
	}
	if err := w.queue.InsertBillingGatewayBudget(ctx, accountID); err != nil {
		w.log.Error("card expiry sweep: re-derive gateway budget failed", "account_id", accountID, "error", err)
	}
	return nil
}
