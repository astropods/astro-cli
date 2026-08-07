package billing

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Status is an account's cached billing/gating status. Absence of a row means
// StatusActive. Driven by Metronome webhook signals + a dunning timer — never by
// a balance the server reads (Metronome owns balance/credit math).
type Status string

const (
	StatusActive    Status = "active"
	StatusPastDue   Status = "past_due"
	StatusSuspended Status = "suspended"
)

// Status reasons (persisted in account_billing_status.reason).
const (
	ReasonDunning       = "dunning"        // payment failed, within grace
	ReasonPaymentFailed = "payment_failed" // grace expired
	ReasonBalanceAlert  = "balance_alert"  // Metronome hard threshold/spend alert
	ReasonUncollectible = "uncollectible"  // Stripe marked an invoice uncollectible (write-off)
	// ReasonCreditsExhausted is the free tier running dry with no card on file.
	// An account with a card never reaches it — it bills pay-as-you-go instead.
	ReasonCreditsExhausted = "credits_exhausted"
)

// StatusRecord is the gating-relevant state for one account.
type StatusRecord struct {
	Status           Status
	Reason           string
	DunningSince     *time.Time
	AlertActive      bool
	ForceSuspended   bool
	CreditsExhausted bool
	HasPaymentMethod bool
}

// signals projects the record onto the state machine's inputs.
func (r StatusRecord) signals() signals {
	return signals{
		dunningSince:     r.DunningSince,
		alertActive:      r.AlertActive,
		forceSuspended:   r.ForceSuspended,
		creditsExhausted: r.CreditsExhausted,
		hasPaymentMethod: r.HasPaymentMethod,
	}
}

// StatusStore persists account_billing_status and owns the pure state machine.
// It is called only off the request path (webhook, timer, card save); the gate
// reads Status with a cheap keyed lookup.
type StatusStore struct {
	db    *sql.DB
	grace time.Duration
}

// NewStatusStore builds the store. graceDays is the dunning window before a
// past_due account ages to suspended (BILLING_DUNNING_GRACE_DAYS).
func NewStatusStore(db *sql.DB, graceDays int) *StatusStore {
	if graceDays <= 0 {
		graceDays = 7
	}
	return &StatusStore{db: db, grace: time.Duration(graceDays) * 24 * time.Hour}
}

// signals is the raw per-account state the status machine reads.
type signals struct {
	dunningSince     *time.Time
	alertActive      bool
	forceSuspended   bool
	creditsExhausted bool
	hasPaymentMethod bool
}

// anyFlagSet reports whether any collection flag is raised. hasPaymentMethod is
// excluded: it is a fact about the account, not a reason to gate it.
func (s signals) anyFlagSet() bool {
	return s.dunningSince != nil || s.alertActive || s.forceSuspended || s.creditsExhausted
}

// computeStatus is the pure state machine. First match wins:
//  1. a terminal write-off       → suspended (uncollectible)
//  2. an uncleared hard alert    → suspended (balance_alert)
//  3. credits gone, no card      → suspended (credits_exhausted)
//  4. dunning past the grace     → suspended (payment_failed)
//  5. dunning within grace       → past_due  (dunning)
//  6. otherwise                  → active
//
// Case 3 is the free tier's floor and is deliberately conditional on the card:
// with one on file the account bills pay-as-you-go, so a spent balance is
// expected rather than a reason to stop it.
func computeStatus(s signals, grace time.Duration, now time.Time) (Status, string) {
	if s.forceSuspended {
		return StatusSuspended, ReasonUncollectible
	}
	if s.alertActive {
		return StatusSuspended, ReasonBalanceAlert
	}
	if s.creditsExhausted && !s.hasPaymentMethod {
		return StatusSuspended, ReasonCreditsExhausted
	}
	if s.dunningSince != nil {
		if now.Sub(*s.dunningSince) > grace {
			return StatusSuspended, ReasonPaymentFailed
		}
		return StatusPastDue, ReasonDunning
	}
	return StatusActive, ""
}

// Get returns the account's cached status; a missing row means active.
func (s *StatusStore) Get(ctx context.Context, accountID string) (Status, string, error) {
	var status, reason sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT status, reason FROM account_billing_status WHERE account_id = $1`, accountID,
	).Scan(&status, &reason)
	if err == sql.ErrNoRows {
		return StatusActive, "", nil
	}
	if err != nil {
		return StatusActive, "", fmt.Errorf("get billing status: %w", err)
	}
	st := Status(status.String)
	if st == "" {
		st = StatusActive
	}
	return st, reason.String, nil
}

// rowQuerier is satisfied by both *sql.DB and *sql.Tx, so Record can be read
// inside Recompute's transaction or standalone.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const recordSelect = `
	SELECT status, reason, dunning_since, alert_active, force_suspended, credits_exhausted, has_payment_method
	FROM account_billing_status WHERE account_id = $1`

// Record returns the full gating state for one account: the status machine's
// inputs plus the status and reason it last produced. A missing row means
// active with no flags set.
func (s *StatusStore) Record(ctx context.Context, accountID string) (StatusRecord, error) {
	return readRecord(ctx, s.db, accountID, recordSelect)
}

func readRecord(ctx context.Context, q rowQuerier, accountID, query string) (StatusRecord, error) {
	var ds sql.NullTime
	var status, reason sql.NullString
	rec := StatusRecord{Status: StatusActive}
	err := q.QueryRowContext(ctx, query, accountID).
		Scan(&status, &reason, &ds, &rec.AlertActive, &rec.ForceSuspended, &rec.CreditsExhausted, &rec.HasPaymentMethod)
	if err == sql.ErrNoRows {
		return StatusRecord{Status: StatusActive}, nil
	}
	if err != nil {
		return StatusRecord{}, fmt.Errorf("read billing status record: %w", err)
	}
	if status.String != "" {
		rec.Status = Status(status.String)
	}
	rec.Reason = reason.String
	if ds.Valid {
		rec.DunningSince = &ds.Time
	}
	return rec, nil
}

// SetDunningSince marks the start of dunning (idempotent — keeps the earliest
// timestamp). Creates the row if absent.
func (s *StatusStore) SetDunningSince(ctx context.Context, accountID string, t time.Time) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, dunning_since, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (account_id) DO UPDATE
		SET dunning_since = COALESCE(account_billing_status.dunning_since, EXCLUDED.dunning_since),
		    updated_at = now()`, accountID, t)
	if err != nil {
		return fmt.Errorf("set dunning_since: %w", err)
	}
	return nil
}

// ClearDunning clears the dunning marker (on payment recovery).
func (s *StatusStore) ClearDunning(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE account_billing_status SET dunning_since = NULL, updated_at = now() WHERE account_id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("clear dunning: %w", err)
	}
	return nil
}

// SetAlert records an uncleared Metronome hard alert (balance/spend threshold).
func (s *StatusStore) SetAlert(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, alert_active, updated_at)
		VALUES ($1, true, now())
		ON CONFLICT (account_id) DO UPDATE SET alert_active = true, updated_at = now()`, accountID)
	if err != nil {
		return fmt.Errorf("set alert: %w", err)
	}
	return nil
}

// ClearAlert clears the hard-alert flag (on recovery).
func (s *StatusStore) ClearAlert(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE account_billing_status SET alert_active = false, updated_at = now() WHERE account_id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("clear alert: %w", err)
	}
	return nil
}

// SetForceSuspend records a terminal write-off (Stripe marked an invoice
// uncollectible). It forces suspended immediately, bypassing the dunning grace.
func (s *StatusStore) SetForceSuspend(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, force_suspended, updated_at)
		VALUES ($1, true, now())
		ON CONFLICT (account_id) DO UPDATE SET force_suspended = true, updated_at = now()`, accountID)
	if err != nil {
		return fmt.Errorf("set force_suspended: %w", err)
	}
	return nil
}

// ClearForceSuspend lifts the write-off flag. Reached when the underlying
// invoice is voided (SignalVoided) or by admin action — not by an unrelated
// payment, since the write-off is terminal until the debt itself is resolved.
func (s *StatusStore) ClearForceSuspend(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE account_billing_status SET force_suspended = false, updated_at = now() WHERE account_id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("clear force_suspended: %w", err)
	}
	return nil
}

// SetCreditsExhausted latches the signup balance as spent. We don't depend on
// Metronome signalling a recovery, so the latch is cleared by whoever grants
// the next credit (ClearCreditsExhausted), not by the provider.
func (s *StatusStore) SetCreditsExhausted(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, credits_exhausted, updated_at)
		VALUES ($1, true, now())
		ON CONFLICT (account_id) DO UPDATE SET credits_exhausted = true, updated_at = now()`, accountID)
	if err != nil {
		return fmt.Errorf("set credits_exhausted: %w", err)
	}
	return nil
}

// ClearCreditsExhausted lifts the latch after a fresh credit grant.
func (s *StatusStore) ClearCreditsExhausted(ctx context.Context, accountID string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE account_billing_status SET credits_exhausted = false, updated_at = now() WHERE account_id = $1`, accountID)
	if err != nil {
		return fmt.Errorf("clear credits_exhausted: %w", err)
	}
	return nil
}

// SetPaymentMethod records whether a card is on file. This is the fact that
// turns an exhausted free account into a pay-as-you-go one, so it is written
// synchronously by the card save/remove handlers rather than by a webhook.
func (s *StatusStore) SetPaymentMethod(ctx context.Context, accountID string, present bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, has_payment_method, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (account_id) DO UPDATE SET has_payment_method = EXCLUDED.has_payment_method, updated_at = now()`,
		accountID, present)
	if err != nil {
		return fmt.Errorf("set has_payment_method: %w", err)
	}
	return nil
}

// ListInDunning returns account IDs currently in past_due (the timer's work set).
func (s *StatusStore) ListInDunning(ctx context.Context, limit int) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT account_id FROM account_billing_status WHERE status = $1 ORDER BY updated_at LIMIT $2`,
		string(StatusPastDue), limit)
	if err != nil {
		return nil, fmt.Errorf("list in dunning: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan account id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// Recompute reads the account's signals, applies the state machine, and
// persists status and reason when either changes. The read takes a row lock and
// the write shares its transaction, so concurrent signals for one account
// serialize instead of racing to write a status computed from stale flags.
//
// The returned bool means the *status* transitioned; a reason-only change
// persists without reporting one, so callers that notify on a transition
// (the dunning sweep) don't re-fire.
func (s *StatusStore) Recompute(ctx context.Context, accountID string, now time.Time) (Status, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return StatusActive, false, fmt.Errorf("begin recompute: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	rec, err := readRecord(ctx, tx, accountID, recordSelect+" FOR UPDATE")
	if err != nil {
		return StatusActive, false, err
	}
	sig := rec.signals()
	next, reason := computeStatus(sig, s.grace, now)
	if next == rec.Status && reason == rec.Reason {
		return next, false, nil
	}
	changed := next != rec.Status

	// active + no flags ⇒ never insert a row; absence already means active.
	if next == StatusActive && !sig.anyFlagSet() {
		if _, err := tx.ExecContext(ctx,
			`UPDATE account_billing_status SET status = $1, reason = NULL, updated_at = now() WHERE account_id = $2`,
			string(StatusActive), accountID); err != nil {
			return StatusActive, false, fmt.Errorf("persist active: %w", err)
		}
	} else if _, err := tx.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, status, reason, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id) DO UPDATE SET status = EXCLUDED.status, reason = EXCLUDED.reason, updated_at = now()`,
		accountID, string(next), nullIfEmpty(reason)); err != nil {
		return next, false, fmt.Errorf("persist billing status: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return next, false, fmt.Errorf("commit recompute: %w", err)
	}
	return next, changed, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
