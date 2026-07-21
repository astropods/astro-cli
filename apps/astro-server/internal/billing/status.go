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
)

// StatusRecord is the gating-relevant state for one account.
type StatusRecord struct {
	Status       Status
	Reason       string
	DunningSince *time.Time
	AlertActive  bool
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

// computeStatus is the pure state machine. First match wins:
//  1. an uncleared hard alert  → suspended (balance_alert)
//  2. dunning past the grace   → suspended (payment_failed)
//  3. dunning within grace     → past_due  (dunning)
//  4. otherwise                → active
func computeStatus(dunningSince *time.Time, alertActive bool, grace time.Duration, now time.Time) (Status, string) {
	if alertActive {
		return StatusSuspended, ReasonBalanceAlert
	}
	if dunningSince != nil {
		if now.Sub(*dunningSince) > grace {
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

// inputs reads the raw signal columns (missing row ⇒ zero values ⇒ active).
func (s *StatusStore) inputs(ctx context.Context, accountID string) (dunningSince *time.Time, alertActive bool, curStatus Status, err error) {
	var ds sql.NullTime
	var alert sql.NullBool
	var status sql.NullString
	scanErr := s.db.QueryRowContext(ctx,
		`SELECT dunning_since, alert_active, status FROM account_billing_status WHERE account_id = $1`, accountID,
	).Scan(&ds, &alert, &status)
	if scanErr == sql.ErrNoRows {
		return nil, false, StatusActive, nil
	}
	if scanErr != nil {
		return nil, false, StatusActive, fmt.Errorf("read billing status inputs: %w", scanErr)
	}
	if ds.Valid {
		dunningSince = &ds.Time
	}
	curStatus = StatusActive
	if status.String != "" {
		curStatus = Status(status.String)
	}
	return dunningSince, alert.Bool, curStatus, nil
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

// Recompute reads the account's signals, applies the state machine, and persists
// the status when it changes. Returns the new status and whether it transitioned.
func (s *StatusStore) Recompute(ctx context.Context, accountID string, now time.Time) (Status, bool, error) {
	dunningSince, alertActive, cur, err := s.inputs(ctx, accountID)
	if err != nil {
		return StatusActive, false, err
	}
	next, reason := computeStatus(dunningSince, alertActive, s.grace, now)
	if next == cur {
		return next, false, nil
	}
	// active + no existing row ⇒ nothing to write (absence already means active).
	if next == StatusActive && dunningSince == nil && !alertActive {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE account_billing_status SET status = $1, reason = NULL, updated_at = now() WHERE account_id = $2`,
			string(StatusActive), accountID); err != nil {
			return StatusActive, false, fmt.Errorf("persist active: %w", err)
		}
		return StatusActive, true, nil
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO account_billing_status (account_id, status, reason, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id) DO UPDATE SET status = EXCLUDED.status, reason = EXCLUDED.reason, updated_at = now()`,
		accountID, string(next), nullIfEmpty(reason)); err != nil {
		return next, false, fmt.Errorf("persist billing status: %w", err)
	}
	return next, true, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
