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
	LastSyncAttemptedAt *time.Time
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
		SELECT deployment_id, account_id, langfuse_dataset_name, item_count, last_trace_at, last_sync_attempted_at, last_synced_at, created_at, updated_at
		FROM eval_datasets
		WHERE deployment_id = $1
	`, deploymentID).Scan(
		&d.DeploymentID, &d.AccountID, &d.LangfuseDatasetName, &d.ItemCount,
		&d.LastTraceAt, &d.LastSyncAttemptedAt, &d.LastSyncedAt, &d.CreatedAt, &d.UpdatedAt,
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

// FinalizeSync marks the sync attempt and updates any summary values that were
// refreshed during the run. Nil values leave the existing column unchanged.
func (s *Store) FinalizeSync(deploymentID string, itemCount *int, lastTraceAt *time.Time, syncSucceeded bool) error {
	var itemCountValue any
	if itemCount != nil {
		itemCountValue = *itemCount
	}

	var lastTraceAtValue any
	if lastTraceAt != nil {
		lastTraceAtValue = *lastTraceAt
	}

	_, err := s.db.Exec(`
		UPDATE eval_datasets
		SET item_count = COALESCE($1, item_count),
			last_trace_at = COALESCE($2, last_trace_at),
			last_sync_attempted_at = NOW(),
			last_synced_at = CASE WHEN $3 THEN NOW() ELSE last_synced_at END,
			updated_at = NOW()
		WHERE deployment_id = $4
	`, itemCountValue, lastTraceAtValue, syncSucceeded, deploymentID)
	if err != nil {
		return fmt.Errorf("dataset store finalize sync: %w", err)
	}
	return nil
}
