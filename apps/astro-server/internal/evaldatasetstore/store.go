package evaldatasetstore

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
	GoodCount           int
	BadCount            int
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Total returns the sum of good and bad judgments backing this dataset.
func (d *EvalDataset) Total() int {
	return d.GoodCount + d.BadCount
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
		SELECT id, deployment_id, account_id, langfuse_dataset_name, item_count,
		       good_count, bad_count, created_at, updated_at
		FROM eval_datasets
		WHERE deployment_id = $1
	`, deploymentID).Scan(
		&d.ID, &d.DeploymentID, &d.AccountID, &d.LangfuseDatasetName, &d.ItemCount,
		&d.GoodCount, &d.BadCount, &d.CreatedAt, &d.UpdatedAt,
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

// RepointByDeploymentID flips the Langfuse dataset name a row points at and
// resets cached counts, since the new Langfuse dataset starts empty. Used to
// heal pre-flip dep-* rows to the eval-* naming convention.
//
// Current legacy dep-* rows predate eval_dataset_judgments, so this heal path
// does not clear judgment rows. If a future caller repoints rows that may have
// judgments, clear eval_dataset_judgments for this dataset id in the same path
// or replace the dataset row with a fresh id.
func (s *Store) RepointByDeploymentID(deploymentID, langfuseDatasetName string) error {
	_, err := s.db.Exec(`
		UPDATE eval_datasets
		SET langfuse_dataset_name = $1,
		    item_count = 0,
		    good_count = 0,
		    bad_count = 0,
		    updated_at = NOW()
		WHERE deployment_id = $2
	`, langfuseDatasetName, deploymentID)
	if err != nil {
		return fmt.Errorf("dataset store repoint: %w", err)
	}
	return nil
}

// BumpCountsByID increments good_count and bad_count by the supplied deltas
// for a specific eval dataset row. Either may be zero. Negative deltas are
// allowed for rollback, but callers must keep the resulting counts >= 0;
// schema CHECK constraints reject negative totals with a constraint error.
func (s *Store) BumpCountsByID(evalDatasetID string, goodDelta, badDelta int) error {
	res, err := s.db.Exec(`
		UPDATE eval_datasets
		SET good_count = good_count + $1,
		    bad_count = bad_count + $2,
		    updated_at = NOW()
		WHERE id = $3
	`, goodDelta, badDelta, evalDatasetID)
	if err != nil {
		return fmt.Errorf("dataset store bump counts: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("dataset store bump counts rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("dataset store bump counts: eval dataset %q not found", evalDatasetID)
	}
	return nil
}
