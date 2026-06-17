package datasetstore

import (
	"database/sql"
	"fmt"
	"time"
)

// EvalDataset is a row in the eval_datasets table.
type EvalDataset struct {
	ID                  string
	DeploymentID        string
	AccountID           string
	LangfuseDatasetName string
	ItemCount           int
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

// GetByDeploymentID returns the dataset record for a deployment, or nil if none exists.
func (s *Store) GetByDeploymentID(deploymentID string) (*EvalDataset, error) {
	var d EvalDataset
	err := s.db.QueryRow(`
		SELECT id, deployment_id, account_id, langfuse_dataset_name, item_count, created_at, updated_at
		FROM eval_datasets
		WHERE deployment_id = $1
	`, deploymentID).Scan(
		&d.ID, &d.DeploymentID, &d.AccountID, &d.LangfuseDatasetName, &d.ItemCount,
		&d.CreatedAt, &d.UpdatedAt,
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

// RepointByDeploymentID flips the Langfuse dataset name for a deployment row
// and resets the cached item count, since the new Langfuse dataset starts
// empty. Used to heal pre-flip dep-* rows to the eval-* naming convention.
func (s *Store) RepointByDeploymentID(deploymentID, langfuseDatasetName string) error {
	_, err := s.db.Exec(`
		UPDATE eval_datasets
		SET langfuse_dataset_name = $1, item_count = 0, updated_at = NOW()
		WHERE deployment_id = $2
	`, langfuseDatasetName, deploymentID)
	if err != nil {
		return fmt.Errorf("dataset store repoint: %w", err)
	}
	return nil
}
