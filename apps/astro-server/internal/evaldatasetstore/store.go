package evaldatasetstore

import (
	"context"
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

// GetByID returns the dataset record with the supplied ID, or nil if it no
// longer exists.
func (s *Store) GetByID(ctx context.Context, id string) (*EvalDataset, error) {
	var d EvalDataset
	err := s.db.QueryRowContext(ctx, `
		SELECT id, deployment_id, account_id, langfuse_dataset_name,
		       created_at, updated_at
		FROM eval_datasets
		WHERE id = $1
	`, id).Scan(
		&d.ID, &d.DeploymentID, &d.AccountID, &d.LangfuseDatasetName,
		&d.CreatedAt, &d.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("dataset store get by id: %w", err)
	}
	return &d, nil
}

// GetByDeploymentID returns the dataset record for a deployment, or nil if none exists.
func (s *Store) GetByDeploymentID(deploymentID string) (*EvalDataset, error) {
	var d EvalDataset
	err := s.db.QueryRow(`
		SELECT id, deployment_id, account_id, langfuse_dataset_name,
		       created_at, updated_at
		FROM eval_datasets
		WHERE deployment_id = $1
	`, deploymentID).Scan(
		&d.ID, &d.DeploymentID, &d.AccountID, &d.LangfuseDatasetName,
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

// RepointByDeploymentID flips the Langfuse dataset name a row points at.
// Used to heal pre-flip dep-* rows to the eval-* naming convention.
func (s *Store) RepointByDeploymentID(deploymentID, langfuseDatasetName string) error {
	_, err := s.db.Exec(`
		UPDATE eval_datasets
		SET langfuse_dataset_name = $1,
		    updated_at = NOW()
		WHERE deployment_id = $2
	`, langfuseDatasetName, deploymentID)
	if err != nil {
		return fmt.Errorf("dataset store repoint: %w", err)
	}
	return nil
}
