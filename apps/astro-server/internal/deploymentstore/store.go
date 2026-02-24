package deploymentstore

import (
	"database/sql"
	"fmt"
	"time"
)

// Store manages deployment record persistence in PostgreSQL.
type Store struct {
	db *sql.DB
}

// NewStore creates a new deployment store with the given database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Deployment represents a single deployment record.
type Deployment struct {
	ID                 string     `json:"id"`
	AccountID          string     `json:"account_id"`
	AgentName          string     `json:"agent_name"`
	BuildID            string     `json:"build_id"`
	Namespace          string     `json:"namespace"`
	DeploymentSpecJSON string     `json:"deployment_spec_json"`
	Status             string     `json:"status"`
	DeployedAt         time.Time  `json:"deployed_at"`
	UndeployedAt       *time.Time `json:"undeployed_at,omitempty"`
}

// SaveDeployment marks any existing active deployment for the agent as undeployed
// and inserts a new active deployment row, all within a transaction.
func (s *Store) SaveDeployment(accountID, agentName, buildID, namespace, specJSON string) (*Deployment, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Mark existing active deployment as undeployed
	_, err = tx.Exec(`
		UPDATE deployments
		SET status = 'undeployed', undeployed_at = NOW()
		WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
	`, accountID, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to mark previous deployment: %w", err)
	}

	// Insert new active deployment
	var d Deployment
	err = tx.QueryRow(`
		INSERT INTO deployments (account_id, agent_name, build_id, namespace, deployment_spec_json, status, deployed_at)
		VALUES ($1, $2, $3, $4, $5, 'active', NOW())
		RETURNING id, account_id, agent_name, build_id, namespace, deployment_spec_json, status, deployed_at
	`, accountID, agentName, buildID, namespace, specJSON).Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
		&d.DeploymentSpecJSON, &d.Status, &d.DeployedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert deployment: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &d, nil
}

// GetActiveDeployment returns the currently active deployment for an agent, or nil if none.
func (s *Store) GetActiveDeployment(accountID, agentName string) (*Deployment, error) {
	var d Deployment
	err := s.db.QueryRow(`
		SELECT id, account_id, agent_name, build_id, namespace, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
	`, accountID, agentName).Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
		&d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active deployment: %w", err)
	}
	return &d, nil
}

// GetDeploymentHistory returns all deployment records for an agent, ordered by deployed_at DESC.
func (s *Store) GetDeploymentHistory(accountID, agentName string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, agent_name, build_id, namespace, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND agent_name = $2
		ORDER BY deployed_at DESC
	`, accountID, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployment history: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deployments []*Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(
			&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
			&d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan deployment row: %w", err)
		}
		deployments = append(deployments, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployment rows: %w", err)
	}
	return deployments, nil
}

// DeploymentWithAccount extends Deployment with the owning account name.
type DeploymentWithAccount struct {
	Deployment
	AccountName string `json:"account_name"`
}

// ListAllActive returns all active deployments across all accounts, joined with account names.
func (s *Store) ListAllActive() ([]*DeploymentWithAccount, error) {
	rows, err := s.db.Query(`
		SELECT d.id, d.account_id, d.agent_name, d.build_id, d.namespace,
		       d.deployment_spec_json, d.status, d.deployed_at, d.undeployed_at,
		       a.name AS account_name
		FROM deployments d
		JOIN accounts a ON d.account_id = a.id
		WHERE d.status = 'active'
		ORDER BY d.deployed_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all active deployments: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deployments []*DeploymentWithAccount
	for rows.Next() {
		var d DeploymentWithAccount
		if err := rows.Scan(
			&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
			&d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
			&d.AccountName,
		); err != nil {
			return nil, fmt.Errorf("failed to scan deployment row: %w", err)
		}
		deployments = append(deployments, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployment rows: %w", err)
	}
	return deployments, nil
}

// MarkUndeployed sets the active deployment for an agent to 'undeployed'.
func (s *Store) MarkUndeployed(accountID, agentName string) error {
	_, err := s.db.Exec(`
		UPDATE deployments
		SET status = 'undeployed', undeployed_at = NOW()
		WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
	`, accountID, agentName)
	if err != nil {
		return fmt.Errorf("failed to mark deployment as undeployed: %w", err)
	}
	return nil
}
