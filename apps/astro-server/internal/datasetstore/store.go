package datasetstore

import (
	"database/sql"
	"fmt"
	"time"
)

// EvalDataset is a row in the eval_datasets table.
type EvalDataset struct {
	DeploymentID        string
	AccountID           string
	LangfuseDatasetName string
	ItemCount           int
	LastTraceAt         *time.Time
	LastSyncedAt        *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Store manages the eval_datasets table.
type Store struct {
	db *sql.DB
}

// NewStore creates a new dataset store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Get returns the dataset record for a deployment, or nil if none exists.
func (s *Store) Get(deploymentID string) (*EvalDataset, error) {
	var d EvalDataset
	err := s.db.QueryRow(`
		SELECT deployment_id, account_id, langfuse_dataset_name, item_count, last_trace_at, last_synced_at, created_at, updated_at
		FROM eval_datasets
		WHERE deployment_id = $1
	`, deploymentID).Scan(
		&d.DeploymentID, &d.AccountID, &d.LangfuseDatasetName, &d.ItemCount,
		&d.LastTraceAt, &d.LastSyncedAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dataset store get: %w", err)
	}
	return &d, nil
}

// Create inserts a new eval_datasets row.
func (s *Store) Create(d *EvalDataset) error {
	_, err := s.db.Exec(`
		INSERT INTO eval_datasets (deployment_id, account_id, langfuse_dataset_name)
		VALUES ($1, $2, $3)
		ON CONFLICT (deployment_id) DO NOTHING
	`, d.DeploymentID, d.AccountID, d.LangfuseDatasetName)
	if err != nil {
		return fmt.Errorf("dataset store create: %w", err)
	}
	return nil
}

// UpdateLastTraceAt sets last_trace_at and overwrites item_count with the
// authoritative total fetched from Langfuse, avoiding drift from re-syncs.
func (s *Store) UpdateLastTraceAt(deploymentID string, t time.Time, totalItems int) error {
	_, err := s.db.Exec(`
		UPDATE eval_datasets
		SET last_trace_at = $1, item_count = $2, updated_at = NOW()
		WHERE deployment_id = $3
	`, t, totalItems, deploymentID)
	if err != nil {
		return fmt.Errorf("dataset store update last_trace_at: %w", err)
	}
	return nil
}

// MarkSynced sets last_synced_at to now, recording when the sync job last completed.
func (s *Store) MarkSynced(deploymentID string) error {
	_, err := s.db.Exec(`
		UPDATE eval_datasets
		SET last_synced_at = NOW(), updated_at = NOW()
		WHERE deployment_id = $1
	`, deploymentID)
	if err != nil {
		return fmt.Errorf("dataset store mark synced: %w", err)
	}
	return nil
}
