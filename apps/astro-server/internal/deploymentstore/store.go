package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
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
	ID                 string          `json:"id"`
	AccountID          string          `json:"account_id"`
	AgentName          string          `json:"agent_name"`
	BuildID            string          `json:"build_id"`
	Namespace          string          `json:"namespace"`
	DisplayName        string          `json:"display_name,omitempty"`
	DeploymentSpecJSON string          `json:"deployment_spec_json"`
	EncryptedDataKey   []byte          `json:"-"`
	KMSKeyARN          *string         `json:"-"`
	Status             string          `json:"status"`
	ErrorMessage       *string         `json:"error_message,omitempty"`
	ErrorDetails       json.RawMessage `json:"error_details,omitempty"`
	StatusChangedAt    time.Time       `json:"status_changed_at"`
	CurrentRevision    *int            `json:"current_revision,omitempty"`
	DeployedAt         time.Time       `json:"deployed_at"`
	UndeployedAt       *time.Time      `json:"undeployed_at,omitempty"`
}

// SaveDeploymentParams holds the parameters for saving a deployment with normalized spec data.
type SaveDeploymentParams struct {
	ID               string
	AccountID        string
	AgentName        string
	DisplayName      string
	BuildID          string
	Namespace        string
	SpecJSON         string
	EncryptedDataKey []byte
	KMSKeyARN        string
}

func nilIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// deploymentColumns is the SELECT column list for full deployment reads.
const deploymentColumns = `id, account_id, agent_name, build_id, namespace, display_name,
       deployment_spec_json, encrypted_data_key, kms_key_arn,
       status, error_message, error_details, status_changed_at, current_revision,
       deployed_at, undeployed_at`

// scanDeployment scans a full deployment row into a Deployment struct.
func scanDeployment(row interface{ Scan(dest ...any) error }) (*Deployment, error) {
	var d Deployment
	var errorDetails []byte
	err := row.Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace, &d.DisplayName,
		&d.DeploymentSpecJSON, &d.EncryptedDataKey, &d.KMSKeyARN,
		&d.Status, &d.ErrorMessage, &errorDetails, &d.StatusChangedAt, &d.CurrentRevision,
		&d.DeployedAt, &d.UndeployedAt,
	)
	if errorDetails != nil {
		d.ErrorDetails = errorDetails
	}
	return &d, err
}

// GetDeploymentByID returns a deployment by its ID, or nil if not found.
func (s *Store) GetDeploymentByID(id string) (*Deployment, error) {
	d, err := scanDeployment(s.db.QueryRow(`
		SELECT `+deploymentColumns+`
		FROM deployments
		WHERE id = $1
	`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment by ID: %w", err)
	}
	return d, nil
}

// GetActiveDeployment returns the currently active deployment for an agent, or nil if none.
// If the agent has multiple active deployments, returns the most recently deployed.
func (s *Store) GetActiveDeployment(accountID, agentName string) (*Deployment, error) {
	var d Deployment
	err := s.db.QueryRow(`
		SELECT id, account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
		ORDER BY deployed_at DESC
		LIMIT 1
	`, accountID, agentName).Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
		&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active deployment: %w", err)
	}
	return &d, nil
}

// GetActiveDeployments returns all active deployments for an agent.
func (s *Store) GetActiveDeployments(accountID, agentName string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
		ORDER BY deployed_at DESC
	`, accountID, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to query active deployments: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deployments []*Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(
			&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
			&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
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

// GetActiveDeploymentByDisplayName returns the active deployment with the given display name, or nil.
func (s *Store) GetActiveDeploymentByDisplayName(accountID, displayName string) (*Deployment, error) {
	var d Deployment
	err := s.db.QueryRow(`
		SELECT id, account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND display_name = $2 AND status = 'active'
	`, accountID, displayName).Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
		&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get active deployment by display name: %w", err)
	}
	return &d, nil
}

// GetActiveDeploymentsByAccount returns all active deployments for an account.
func (s *Store) GetActiveDeploymentsByAccount(accountID string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND status = 'active'
		ORDER BY deployed_at DESC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query active deployments by account: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deployments []*Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(
			&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
			&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
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

// GetDeploymentHistory returns all deployment records for an agent, ordered by deployed_at DESC.
func (s *Store) GetDeploymentHistory(accountID, agentName string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
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
			&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt, &d.UndeployedAt,
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
		SELECT d.id, d.account_id, d.agent_name, d.build_id, d.namespace, d.display_name,
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
			&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace, &d.DisplayName,
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

// ListAllWithAccount returns all non-undeployed deployments across all accounts,
// joined with account names. Includes async fields (status, error_message, etc).
func (s *Store) ListAllWithAccount() ([]*DeploymentWithAccount, error) {
	rows, err := s.db.Query(`
		SELECT d.id, d.account_id, d.agent_name, d.build_id, d.namespace, d.display_name,
		       d.deployment_spec_json, d.status, d.error_message, d.status_changed_at,
		       d.current_revision, d.deployed_at, d.undeployed_at,
		       a.name AS account_name
		FROM deployments d
		JOIN accounts a ON d.account_id = a.id
		WHERE d.status != 'undeployed'
		ORDER BY d.deployed_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query all deployments with account: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deployments []*DeploymentWithAccount
	for rows.Next() {
		var d DeploymentWithAccount
		if err := rows.Scan(
			&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace, &d.DisplayName,
			&d.DeploymentSpecJSON, &d.Status, &d.ErrorMessage, &d.StatusChangedAt,
			&d.CurrentRevision, &d.DeployedAt, &d.UndeployedAt,
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

// GetDeploymentByNamespace returns a deployment by its namespace, or nil if not found.
// Only returns non-undeployed deployments.
func (s *Store) GetDeploymentByNamespace(namespace string) (*Deployment, error) {
	d, err := scanDeployment(s.db.QueryRow(`
		SELECT `+deploymentColumns+`
		FROM deployments
		WHERE namespace = $1 AND status != 'undeployed'
		ORDER BY deployed_at DESC
		LIMIT 1
	`, namespace))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment by namespace: %w", err)
	}
	return d, nil
}

// MarkUndeployedByID sets a specific deployment to 'undeployed'.
func (s *Store) MarkUndeployedByID(deploymentID string) error {
	_, err := s.db.Exec(`
		UPDATE deployments
		SET status = 'undeployed', undeployed_at = NOW()
		WHERE id = $1 AND status = 'active'
	`, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to mark deployment as undeployed: %w", err)
	}
	return nil
}

// updateStatusTx updates a deployment's status and records an event within an existing transaction.
func updateStatusTx(tx *sql.Tx, id, status, errorMsg string, errorDetails json.RawMessage) error {
	_, err := tx.Exec(`
		UPDATE deployments
		SET status = $2, error_message = $3, error_details = $4, status_changed_at = NOW()
		WHERE id = $1
	`, id, status, nilIfEmpty(errorMsg), errorDetails)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO deployment_events (deployment_id, status, message, details)
		VALUES ($1, $2, $3, $4)
	`, id, status, nilIfEmpty(errorMsg), errorDetails)
	if err != nil {
		return fmt.Errorf("failed to insert deployment event: %w", err)
	}

	return nil
}

// UpdateStatus is the single entry point for all deployment status changes.
// It updates the deployment row and inserts a deployment_events row in one transaction.
func (s *Store) UpdateStatus(id, status, errorMsg string, errorDetails json.RawMessage) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := updateStatusTx(tx, id, status, errorMsg, errorDetails); err != nil {
		return err
	}

	return tx.Commit()
}

// SaveDeploymentPending saves a new deployment with status='pending' and creates revision 1.
// The txFn callback runs in the same transaction, allowing the caller to enqueue a River
// job and save normalized spec data atomically.
func (s *Store) SaveDeploymentPending(p SaveDeploymentParams, txFn func(tx *sql.Tx, deploymentID string) error) (*Deployment, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Mark existing active deployment with same display_name as undeployed
	if p.DisplayName != "" {
		_, err = tx.Exec(`
			UPDATE deployments
			SET status = 'undeployed', undeployed_at = NOW(), status_changed_at = NOW()
			WHERE account_id = $1 AND display_name = $2 AND status = 'active'
		`, p.AccountID, p.DisplayName)
	} else {
		_, err = tx.Exec(`
			UPDATE deployments
			SET status = 'undeployed', undeployed_at = NOW(), status_changed_at = NOW()
			WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
			AND (SELECT COUNT(*) FROM deployments WHERE account_id = $1 AND agent_name = $2 AND status = 'active') = 1
		`, p.AccountID, p.AgentName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to mark previous deployment: %w", err)
	}

	// Insert new deployment with status='pending' and current_revision=1
	var d Deployment
	err = tx.QueryRow(`
		INSERT INTO deployments (id, account_id, agent_name, build_id, namespace, display_name,
		    deployment_spec_json, encrypted_data_key, kms_key_arn,
		    status, status_changed_at, current_revision, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), 1, NOW())
		RETURNING id, account_id, agent_name, build_id, namespace, display_name,
		    deployment_spec_json, status, deployed_at
	`, p.ID, p.AccountID, p.AgentName, p.BuildID, p.Namespace, p.DisplayName,
		p.SpecJSON, p.EncryptedDataKey, nilIfEmpty(p.KMSKeyARN), StatusPending).Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
		&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to insert deployment: %w", err)
	}

	// Create revision 1
	_, err = tx.Exec(`
		INSERT INTO deployment_revisions (deployment_id, revision, build_id, spec_json, kms_ciphertext, kms_key_id)
		VALUES ($1, 1, $2, $3, $4, $5)
	`, d.ID, p.BuildID, p.SpecJSON, p.EncryptedDataKey, nilIfEmpty(p.KMSKeyARN))
	if err != nil {
		return nil, fmt.Errorf("failed to insert revision: %w", err)
	}

	// Record initial event
	_, err = tx.Exec(`
		INSERT INTO deployment_events (deployment_id, status, message)
		VALUES ($1, $2, 'Deployment queued')
	`, d.ID, StatusPending)
	if err != nil {
		return nil, fmt.Errorf("failed to insert deployment event: %w", err)
	}

	if txFn != nil {
		if err := txFn(tx, d.ID); err != nil {
			return nil, fmt.Errorf("failed to run tx callback: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &d, nil
}

// UpdateDeploymentPending updates an existing deployment for a redeploy, creating a new revision.
// Sets status='pending'. The txFn callback runs in the same transaction.
func (s *Store) UpdateDeploymentPending(p SaveDeploymentParams, txFn func(tx *sql.Tx, deploymentID string) error) (*Deployment, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Determine next revision number
	var nextRevision int
	err = tx.QueryRow(`
		SELECT COALESCE(MAX(revision), 0) + 1 FROM deployment_revisions WHERE deployment_id = $1
	`, p.ID).Scan(&nextRevision)
	if err != nil {
		return nil, fmt.Errorf("failed to get next revision: %w", err)
	}

	// Update deployment row
	var d Deployment
	err = tx.QueryRow(`
		UPDATE deployments
		SET build_id = $2, deployment_spec_json = $3, encrypted_data_key = $4,
		    kms_key_arn = $5, display_name = $6, status = $7,
		    error_message = NULL, error_details = NULL,
		    status_changed_at = NOW(), current_revision = $8, deployed_at = NOW()
		WHERE id = $1
		RETURNING id, account_id, agent_name, build_id, namespace, display_name,
		    deployment_spec_json, status, deployed_at
	`, p.ID, p.BuildID, p.SpecJSON, p.EncryptedDataKey, nilIfEmpty(p.KMSKeyARN),
		p.DisplayName, StatusPending, nextRevision).Scan(
		&d.ID, &d.AccountID, &d.AgentName, &d.BuildID, &d.Namespace,
		&d.DisplayName, &d.DeploymentSpecJSON, &d.Status, &d.DeployedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to update deployment: %w", err)
	}

	// Create new revision
	_, err = tx.Exec(`
		INSERT INTO deployment_revisions (deployment_id, revision, build_id, spec_json, kms_ciphertext, kms_key_id)
		VALUES ($1, $2, $3, $4, $5, $6)
	`, d.ID, nextRevision, p.BuildID, p.SpecJSON, p.EncryptedDataKey, nilIfEmpty(p.KMSKeyARN))
	if err != nil {
		return nil, fmt.Errorf("failed to insert revision: %w", err)
	}

	// Delete old normalized data (cascades from deployment_workloads)
	_, err = tx.Exec(`DELETE FROM deployment_workloads WHERE deployment_id = $1`, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old workloads: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM deployment_variables WHERE deployment_id = $1`, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old variables: %w", err)
	}

	// Record event
	_, err = tx.Exec(`
		INSERT INTO deployment_events (deployment_id, status, message)
		VALUES ($1, $2, $3)
	`, d.ID, StatusPending, fmt.Sprintf("Redeploy queued (revision %d)", nextRevision))
	if err != nil {
		return nil, fmt.Errorf("failed to insert deployment event: %w", err)
	}

	if txFn != nil {
		if err := txFn(tx, d.ID); err != nil {
			return nil, fmt.Errorf("failed to run tx callback: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return &d, nil
}

// GetDeploymentsInStatus returns all deployments matching any of the given statuses.
func (s *Store) GetDeploymentsInStatus(statuses ...string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT `+deploymentColumns+`
		FROM deployments
		WHERE status = ANY($1)
		ORDER BY deployed_at DESC
	`, pq.Array(statuses))
	if err != nil {
		return nil, fmt.Errorf("failed to query deployments by status: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var deployments []*Deployment
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, fmt.Errorf("failed to scan deployment: %w", err)
		}
		deployments = append(deployments, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployments: %w", err)
	}
	return deployments, nil
}

// MarkScaledDown records that a namespace has been scaled down by KEDA and updates
// the deployment status to scaled_down.
func (s *Store) MarkScaledDown(deploymentID, namespace string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`
		INSERT INTO scaled_namespaces (namespace, deployment_id)
		VALUES ($1, $2)
		ON CONFLICT (namespace) DO NOTHING
	`, namespace, deploymentID)
	if err != nil {
		return fmt.Errorf("failed to insert scaled namespace: %w", err)
	}

	if err := updateStatusTx(tx, deploymentID, StatusScaledDown, "KEDA scaled namespace to zero", nil); err != nil {
		return err
	}

	return tx.Commit()
}

// ClearScaledDown removes a namespace from the scaled_namespaces table.
func (s *Store) ClearScaledDown(namespace string) error {
	_, err := s.db.Exec(`DELETE FROM scaled_namespaces WHERE namespace = $1`, namespace)
	if err != nil {
		return fmt.Errorf("failed to clear scaled namespace: %w", err)
	}
	return nil
}

// IsScaledDown checks whether a namespace is currently tracked as scaled down by KEDA.
func (s *Store) IsScaledDown(namespace string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM scaled_namespaces WHERE namespace = $1)
	`, namespace).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check scaled namespace: %w", err)
	}
	return exists, nil
}
