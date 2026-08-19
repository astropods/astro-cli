// Package memberemails maintains a local mirror of member (WorkOS user) email
// addresses. External dev-tool telemetry is stamped with the developer's
// user.email; mirroring email → user_id lets us attribute that spend to a
// member with a single indexed lookup instead of a per-request WorkOS call. The
// mirror is kept fresh by the WorkOS events poller and a periodic reconcile.
package memberemails

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// UpsertWorkOS records a WorkOS-synced email for a user, replacing any prior
// WorkOS email for that user so a change of primary email doesn't leave a stale
// row. `verified` carries WorkOS's own email-verification state. Emails from
// other sources are left untouched.
func (s *Store) UpsertWorkOS(ctx context.Context, userID, email string, verified bool) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if userID == "" || email == "" {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("memberemails: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // rolled back only if Commit didn't run
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM account_member_emails WHERE user_id = $1 AND source = 'workos' AND email <> $2`,
		userID, email); err != nil {
		return fmt.Errorf("memberemails: prune stale workos email: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO account_member_emails (user_id, email, source, verified)
		 VALUES ($1, $2, 'workos', $3)
		 ON CONFLICT (email) DO UPDATE SET user_id = EXCLUDED.user_id, source = 'workos', verified = EXCLUDED.verified, updated_at = now()`,
		userID, email, verified); err != nil {
		return fmt.Errorf("memberemails: upsert workos email: %w", err)
	}
	return tx.Commit()
}

// DeleteForUser removes all of a user's emails and any reconcile-backoff record
// (WorkOS user deletion).
func (s *Store) DeleteForUser(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM account_member_emails WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("memberemails: delete for user: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM member_email_reconcile_attempts WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("memberemails: delete attempts for user: %w", err)
	}
	return nil
}

// EmailsForAccount returns lowercased-email → user_id for every member of the
// account — the attribution join for dev-tool telemetry.
func (s *Store) EmailsForAccount(ctx context.Context, accountID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT me.email, me.user_id
		 FROM account_member_emails me
		 JOIN account_members am ON am.user_id = me.user_id
		 WHERE am.account_id = $1`, accountID)
	if err != nil {
		return nil, fmt.Errorf("memberemails: emails for account: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	out := map[string]string{}
	for rows.Next() {
		var email, userID string
		if err := rows.Scan(&email, &userID); err != nil {
			return nil, fmt.Errorf("memberemails: scan: %w", err)
		}
		out[email] = userID
	}
	return out, rows.Err()
}

// RetryBackoff is how long a user who resolved to nothing is left alone before
// another WorkOS lookup, whether the reconcile job or a listing asks.
const RetryBackoff = 6 * time.Hour

// MemberEmail is a mirrored address alongside the last unresolved lookup for
// that user, so callers can honor RetryBackoff instead of re-querying WorkOS.
type MemberEmail struct {
	Email       string
	AttemptedAt time.Time
}

// EmailsForUsers returns user_id → mirrored email and last attempt for the
// given users, preferring the WorkOS-synced address. Users with neither a
// recorded email nor a recorded attempt are absent.
func (s *Store) EmailsForUsers(ctx context.Context, userIDs []string) (map[string]MemberEmail, error) {
	if len(userIDs) == 0 {
		return map[string]MemberEmail{}, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT ON (u.user_id) u.user_id, COALESCE(me.email, ''), a.attempted_at
		 FROM unnest($1::text[]) AS u(user_id)
		 LEFT JOIN account_member_emails me ON me.user_id = u.user_id
		 LEFT JOIN member_email_reconcile_attempts a ON a.user_id = u.user_id
		 ORDER BY u.user_id, (me.source = 'workos') DESC, me.updated_at DESC`, pq.Array(userIDs))
	if err != nil {
		return nil, fmt.Errorf("memberemails: emails for users: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	out := make(map[string]MemberEmail, len(userIDs))
	for rows.Next() {
		var (
			userID    string
			email     string
			attempted sql.NullTime
		)
		if err := rows.Scan(&userID, &email, &attempted); err != nil {
			return nil, fmt.Errorf("memberemails: scan: %w", err)
		}
		if email == "" && !attempted.Valid {
			continue
		}
		out[userID] = MemberEmail{Email: email, AttemptedAt: attempted.Time}
	}
	return out, rows.Err()
}

// UserIDsMissingEmail returns up to limit distinct member user_ids with no
// recorded email that weren't attempted since retryBefore — the reconcile work
// list, ordered for deterministic paging so the cap advances.
func (s *Store) UserIDsMissingEmail(ctx context.Context, limit int, retryBefore time.Time) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT am.user_id
		 FROM account_members am
		 LEFT JOIN account_member_emails me ON me.user_id = am.user_id
		 LEFT JOIN member_email_reconcile_attempts a ON a.user_id = am.user_id
		 WHERE me.user_id IS NULL
		   AND (a.user_id IS NULL OR a.attempted_at < $2)
		 ORDER BY am.user_id
		 LIMIT $1`, limit, retryBefore)
	if err != nil {
		return nil, fmt.Errorf("memberemails: users missing email: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	var out []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("memberemails: scan: %w", err)
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// RecordReconcileAttempt marks a user as reconciled-but-unresolved so the
// backfill job backs off from re-querying WorkOS for them every run.
func (s *Store) RecordReconcileAttempt(ctx context.Context, userID string) error {
	if _, err := s.db.ExecContext(ctx,
		`INSERT INTO member_email_reconcile_attempts (user_id, attempted_at)
		 VALUES ($1, now())
		 ON CONFLICT (user_id) DO UPDATE SET attempted_at = now()`, userID); err != nil {
		return fmt.Errorf("memberemails: record reconcile attempt: %w", err)
	}
	return nil
}
