package experiment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Key is a server-owned organization experiment identifier.
type Key string

const (
	FineGrainedAccess         Key = "fine_grained_access"
	PromptClassificationStats Key = "prompt_classification_stats"
)

// Store persists organization experiment choices. A missing row is disabled.
type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Enabled(ctx context.Context, accountID string, key Key) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("experiment store is not configured")
	}
	if accountID == "" || key == "" {
		return false, errors.New("account id and experiment are required")
	}

	var enabled bool
	err := s.db.QueryRowContext(ctx, `
		SELECT enabled
		FROM account_experiments
		WHERE account_id = $1 AND experiment = $2
	`, accountID, key).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read account experiment %q: %w", key, err)
	}
	return enabled, nil
}

func (s *Store) SetEnabled(ctx context.Context, accountID string, key Key, enabled bool) error {
	if s == nil || s.db == nil {
		return errors.New("experiment store is not configured")
	}
	if accountID == "" || key == "" {
		return errors.New("account id and experiment are required")
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO account_experiments (account_id, experiment, enabled, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (account_id, experiment) DO UPDATE
		SET enabled = EXCLUDED.enabled,
		    updated_at = now()
	`, accountID, key, enabled)
	if err != nil {
		return fmt.Errorf("set account experiment %q: %w", key, err)
	}
	return nil
}

// Gate binds one experiment key for authorization rollout checks.
type Gate struct {
	store *Store
	key   Key
}

func NewGate(store *Store, key Key) *Gate {
	return &Gate{store: store, key: key}
}

func (g *Gate) Enabled(ctx context.Context, accountID string) (bool, error) {
	if g == nil {
		return false, errors.New("experiment gate is not configured")
	}
	return g.store.Enabled(ctx, accountID, g.key)
}
