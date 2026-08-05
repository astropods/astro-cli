// Package watcher tracks which members are subscribed to a deployment's alerts.
// Membership is implicit: acting on a deployment enrolls you. The store is the
// write side (Record, driven off the audit-log seam) and the read side that
// notify uses to resolve an alert's recipients.
package watcher

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

// Store persists deployment watchers in PostgreSQL.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// Watcher is one member's subscription to a deployment.
type Watcher struct {
	UserID string
	Reason string
	Muted  bool
}

// Record enrolls a member as a watcher of a deployment, or refreshes an existing
// enrollment. It deliberately does not touch `muted` or `reason` on conflict: a
// member who opted out stays opted out no matter how many times they deploy
// again, and the reason records what first enrolled them.
func (s *Store) Record(ctx context.Context, accountID, deploymentID, userID, reason string) error {
	if accountID == "" || deploymentID == "" || userID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_watchers (deployment_id, user_id, account_id, reason)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (deployment_id, user_id)
		DO UPDATE SET last_active_at = now()
	`, deploymentID, userID, accountID, reason)
	if err != nil {
		return fmt.Errorf("watcher: record %s/%s: %w", deploymentID, userID, err)
	}
	return nil
}

// ActiveUserIDs returns the user ids alerts for this deployment should reach:
// every watcher who has not muted it. An empty result is normal (nobody has
// acted on the deployment yet) and the caller decides the fallback.
func (s *Store) ActiveUserIDs(ctx context.Context, deploymentID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id FROM deployment_watchers
		WHERE deployment_id = $1 AND NOT muted
		ORDER BY created_at ASC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("watcher: active user ids for %s: %w", deploymentID, err)
	}
	defer rows.Close() //nolint:errcheck

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("watcher: scan user id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// List returns every watcher of a deployment, muted ones included, so the API
// can show a member their own state alongside everyone else's.
func (s *Store) List(ctx context.Context, deploymentID string) ([]Watcher, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT user_id, reason, muted FROM deployment_watchers
		WHERE deployment_id = $1
		ORDER BY created_at ASC
	`, deploymentID)
	if err != nil {
		return nil, fmt.Errorf("watcher: list %s: %w", deploymentID, err)
	}
	defer rows.Close() //nolint:errcheck

	var out []Watcher
	for rows.Next() {
		var w Watcher
		if err := rows.Scan(&w.UserID, &w.Reason, &w.Muted); err != nil {
			return nil, fmt.Errorf("watcher: scan watcher: %w", err)
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// SetMuted records a member's explicit choice. Muting an unenrolled member
// inserts the row already muted, so "unwatch before ever deploying" sticks and
// their first deploy does not enroll them.
func (s *Store) SetMuted(ctx context.Context, accountID, deploymentID, userID string, muted bool) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO deployment_watchers (deployment_id, user_id, account_id, reason, muted)
		VALUES ($1, $2, $3, 'explicit', $4)
		ON CONFLICT (deployment_id, user_id)
		DO UPDATE SET muted = EXCLUDED.muted
	`, deploymentID, userID, accountID, muted)
	if err != nil {
		return fmt.Errorf("watcher: set muted=%t on %s/%s: %w", muted, deploymentID, userID, err)
	}
	return nil
}

// deploymentActionPrefix is the audit action namespace that enrolls a watcher.
// Reads (deployment.get, …) are not audited, so every recorded deployment.*
// action is a mutation by definition.
const deploymentActionPrefix = "deployment."

// Enrolls reports whether an audit action should enroll its actor. Kept next to
// the store so the policy lives with the table it writes.
func Enrolls(action, resourceType, actorType string) bool {
	return actorType == "user" &&
		resourceType == "deployment" &&
		strings.HasPrefix(action, deploymentActionPrefix)
}
