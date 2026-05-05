package deploymentstore

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// LineageValidator verifies that a deployment's source-lineage tuple
// (sourceAccountID, agentName, buildID) refers to a real published version.
//
// The interface is satisfied by *agentindex.Index via its ValidateLineage
// method (a thin wrapper over GetVersion). Returning only error keeps this
// package free of an agentindex import: the Store only needs to know
// whether the tuple exists, not what the version contains.
type LineageValidator interface {
	ValidateLineage(accountID, name, buildID string) error
}

// Store manages deployment record persistence in PostgreSQL.
//
// validator is optional. When non-nil, SaveDeploymentPending and
// UpdateDeploymentPending reject writes whose SourceAccountID is set but
// does not match a published agent_versions row. Tests construct Store
// without a validator and the gate becomes a no-op; production wires
// agentindex.Index via WithLineageValidator in main.go.
type Store struct {
	db        *sql.DB
	validator LineageValidator
}

// NewStore creates a new deployment store with the given database connection.
//
// The returned Store has no lineage validator; callers that want write-time
// lineage enforcement should chain WithLineageValidator. This shape preserves
// every existing test call site (which would otherwise need to construct a
// no-op validator) while letting production opt in with a single line.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// WithLineageValidator installs a LineageValidator on the Store and returns
// the Store for chaining. Calling it with nil disables validation.
func (s *Store) WithLineageValidator(v LineageValidator) *Store {
	s.validator = v
	return s
}

// validateLineage runs the optional LineageValidator gate. It is a no-op when
// the validator is unset or when SourceAccountID is empty (legacy/ancient
// rows that predate the column resolve via the spec-JSON fallback on read,
// not via the validator). Callers wrap the returned error with their own
// context.
func (s *Store) validateLineage(p SaveDeploymentParams) error {
	if s.validator == nil || p.SourceAccountID == "" {
		return nil
	}
	if err := s.validator.ValidateLineage(p.SourceAccountID, p.AgentName, p.BuildID); err != nil {
		return fmt.Errorf("lineage validation failed for %s/%s@%s: %w",
			p.SourceAccountID, p.AgentName, p.BuildID, err)
	}
	return nil
}

// Deployment represents a single deployment record.
//
// SourceAccountID is the account that published the agent blueprint. On cross-
// account deployments this differs from AccountID (the target account that
// owns the deployment). Nil for legacy rows predating the column; read paths
// that need the source account should fall back to parsing
// deployment_spec_json via SourceAccountFromSpec + account lookup.
type Deployment struct {
	ID                 string           `json:"id"`
	AccountID          string           `json:"account_id"`
	SourceAccountID    *string          `json:"source_account_id,omitempty"`
	AgentName          string           `json:"agent_name"`
	BuildID            string           `json:"build_id"`
	Namespace          string           `json:"namespace"`
	DisplayName        string           `json:"display_name,omitempty"`
	DeploymentSpecJSON string           `json:"deployment_spec_json"`
	EncryptedDataKey   []byte           `json:"-"`
	KMSKeyARN          *string          `json:"-"`
	Status             string           `json:"status"`
	ErrorMessage       *string          `json:"error_message,omitempty"`
	ErrorDetails       json.RawMessage  `json:"error_details,omitempty"`
	StatusChangedAt    time.Time        `json:"status_changed_at"`
	CurrentRevision    *int             `json:"current_revision,omitempty"`
	DeployedAt         time.Time        `json:"deployed_at"`
	UndeployedAt       *time.Time       `json:"undeployed_at,omitempty"`
	AvatarColors       *json.RawMessage `json:"avatar_colors,omitempty"`
}

// SaveDeploymentParams holds the parameters for saving a deployment with normalized spec data.
type SaveDeploymentParams struct {
	ID               string
	AccountID        string
	SourceAccountID  string
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
const deploymentColumns = `id, account_id, source_account_id, agent_name, build_id, namespace, display_name,
       deployment_spec_json, encrypted_data_key, kms_key_arn,
       status, error_message, error_details, status_changed_at, current_revision,
       deployed_at, undeployed_at, avatar_colors`

// scanDeployment scans a full deployment row into a Deployment struct.
func scanDeployment(row interface{ Scan(dest ...any) error }) (*Deployment, error) {
	var d Deployment
	var errorDetails []byte
	err := row.Scan(
		&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace, &d.DisplayName,
		&d.DeploymentSpecJSON, &d.EncryptedDataKey, &d.KMSKeyARN,
		&d.Status, &d.ErrorMessage, &errorDetails, &d.StatusChangedAt, &d.CurrentRevision,
		&d.DeployedAt, &d.UndeployedAt, &d.AvatarColors,
	)
	if errorDetails != nil {
		d.ErrorDetails = errorDetails
	}
	return &d, err
}

// SetAvatarColors stores the extracted avatar color scheme for a deployment.
func (s *Store) SetAvatarColors(deploymentID string, colorsJSON []byte) error {
	_, err := s.db.Exec(`UPDATE deployments SET avatar_colors = $1 WHERE id = $2`, colorsJSON, deploymentID)
	return err
}

// BulkDeploymentCounts returns the total deployment count per agent name for the given account.
func (s *Store) BulkDeploymentCounts(accountID string) (map[string]int64, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	rows, err := s.db.Query(`
		SELECT agent_name, COUNT(*) FROM deployments
		WHERE account_id = $1
		GROUP BY agent_name
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query deployment counts: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	counts := make(map[string]int64)
	for rows.Next() {
		var name string
		var count int64
		if err := rows.Scan(&name, &count); err != nil {
			return nil, fmt.Errorf("failed to scan deployment count row: %w", err)
		}
		counts[name] = count
	}
	return counts, rows.Err()
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
		SELECT id, account_id, source_account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
		ORDER BY deployed_at DESC
		LIMIT 1
	`, accountID, agentName).Scan(
		&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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
		SELECT id, account_id, source_account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
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
			&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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
		SELECT id, account_id, source_account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
		FROM deployments
		WHERE account_id = $1 AND display_name = $2 AND status = 'active'
	`, accountID, displayName).Scan(
		&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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
		SELECT id, account_id, source_account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
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
			&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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

// GetVisibleDeploymentsByAccount returns all deployments that exist for an
// account — every status except fully undeployed (torn down).
// CountVisibleDeploymentsByAccount returns the number of non-undeployed deployments for an account.
func (s *Store) CountVisibleDeploymentsByAccount(accountID string) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM deployments
		WHERE account_id = $1 AND status != 'undeployed'
	`, accountID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count deployments: %w", err)
	}
	return count, nil
}

func (s *Store) GetVisibleDeploymentsByAccount(accountID string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT `+deploymentColumns+`
		FROM deployments
		WHERE account_id = $1 AND status != 'undeployed'
		ORDER BY deployed_at DESC
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query visible deployments by account: %w", err)
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
		return nil, fmt.Errorf("error iterating deployment rows: %w", err)
	}
	return deployments, nil
}

// GetDeploymentHistory returns all deployment records for an agent, ordered by deployed_at DESC.
func (s *Store) GetDeploymentHistory(accountID, agentName string) ([]*Deployment, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, source_account_id, agent_name, build_id, namespace, display_name, deployment_spec_json, status, deployed_at, undeployed_at
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
			&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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
	AccountName     string  `json:"account_name"`
	DriftReportJSON *string `json:"-"` // raw JSONB from DB, parsed by caller
	OwnerUserID     string  `json:"-"` // first member's user_id, resolved by caller
}

// ListAllActive returns all active deployments across all accounts, joined with account names.
func (s *Store) ListAllActive() ([]*DeploymentWithAccount, error) {
	rows, err := s.db.Query(`
		SELECT d.id, d.account_id, d.source_account_id, d.agent_name, d.build_id, d.namespace, d.display_name,
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
			&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace, &d.DisplayName,
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
		SELECT d.id, d.account_id, d.source_account_id, d.agent_name, d.build_id, d.namespace, d.display_name,
		       d.deployment_spec_json, d.status, d.error_message, d.status_changed_at,
		       d.current_revision, d.deployed_at, d.undeployed_at,
		       a.name AS account_name, d.drift_report,
		       COALESCE((SELECT user_id FROM account_members WHERE account_id = a.id ORDER BY created_at ASC LIMIT 1), '') AS owner_user_id
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
			&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace, &d.DisplayName,
			&d.DeploymentSpecJSON, &d.Status, &d.ErrorMessage, &d.StatusChangedAt,
			&d.CurrentRevision, &d.DeployedAt, &d.UndeployedAt,
			&d.AccountName, &d.DriftReportJSON, &d.OwnerUserID,
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

// nilIfEmptyJSON returns nil (SQL NULL) if the JSON is nil or empty, otherwise the value.
func nilIfEmptyJSON(j json.RawMessage) interface{} {
	if len(j) == 0 {
		return nil
	}
	return j
}

// updateStatusTx updates a deployment's status and records an event within an existing transaction.
func updateStatusTx(tx *sql.Tx, id, status, errorMsg string, errorDetails json.RawMessage) error {
	details := nilIfEmptyJSON(errorDetails)
	_, err := tx.Exec(`
		UPDATE deployments
		SET status = $2, error_message = $3, error_details = $4, status_changed_at = NOW()
		WHERE id = $1
	`, id, status, nilIfEmpty(errorMsg), details)
	if err != nil {
		return fmt.Errorf("failed to update deployment status: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO deployment_events (deployment_id, status, message, details)
		VALUES ($1, $2, $3, $4)
	`, id, status, nilIfEmpty(errorMsg), details)
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
	if err := s.validateLineage(p); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	// Mark existing active deployment with same display_name as undeployed
	// and clean up its normalized data (workloads, variables).
	var oldIDs []string
	var rows *sql.Rows
	if p.DisplayName != "" {
		rows, err = tx.Query(`
			UPDATE deployments
			SET status = 'undeployed', undeployed_at = NOW(), status_changed_at = NOW()
			WHERE account_id = $1 AND display_name = $2 AND status = 'active'
			RETURNING id
		`, p.AccountID, p.DisplayName)
	} else {
		rows, err = tx.Query(`
			UPDATE deployments
			SET status = 'undeployed', undeployed_at = NOW(), status_changed_at = NOW()
			WHERE account_id = $1 AND agent_name = $2 AND status = 'active'
			AND (SELECT COUNT(*) FROM deployments WHERE account_id = $1 AND agent_name = $2 AND status = 'active') = 1
			RETURNING id
		`, p.AccountID, p.AgentName)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to mark previous deployment: %w", err)
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			oldIDs = append(oldIDs, id)
		}
	}
	rows.Close() //nolint:errcheck,gosec

	// Clean up normalized data for superseded deployments
	for _, oldID := range oldIDs {
		if _, err := tx.Exec(`DELETE FROM deployment_workloads WHERE deployment_id = $1`, oldID); err != nil {
			return nil, fmt.Errorf("failed to delete old workloads for %s: %w", oldID, err)
		}
		if _, err := tx.Exec(`DELETE FROM deployment_sidecars WHERE deployment_id = $1`, oldID); err != nil {
			return nil, fmt.Errorf("failed to delete old sidecars for %s: %w", oldID, err)
		}
		if _, err := tx.Exec(`DELETE FROM deployment_build_env WHERE deployment_id = $1`, oldID); err != nil {
			return nil, fmt.Errorf("failed to delete old variables for %s: %w", oldID, err)
		}
	}

	// Insert new deployment with status='pending' and current_revision=1
	var d Deployment
	err = tx.QueryRow(`
		INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id, namespace, display_name,
		    deployment_spec_json, encrypted_data_key, kms_key_arn,
		    status, status_changed_at, current_revision, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), 1, NOW())
		RETURNING id, account_id, source_account_id, agent_name, build_id, namespace, display_name,
		    deployment_spec_json, status, deployed_at
	`, p.ID, p.AccountID, nilIfEmpty(p.SourceAccountID), p.AgentName, p.BuildID, p.Namespace, p.DisplayName,
		p.SpecJSON, p.EncryptedDataKey, nilIfEmpty(p.KMSKeyARN), StatusPending).Scan(
		&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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

// UpdateDeploymentSpecJSON updates only the stored deployment spec JSON.
// Used by repair to persist a re-generated template without changing status or revision.
func (s *Store) UpdateDisplayName(deploymentID, displayName string) error {
	_, err := s.db.Exec(`UPDATE deployments SET display_name = $2 WHERE id = $1`, deploymentID, displayName)
	if err != nil {
		return fmt.Errorf("update deployment display name: %w", err)
	}
	return nil
}

func (s *Store) UpdateDeploymentSpecJSON(deploymentID, specJSON string) error {
	_, err := s.db.Exec(`UPDATE deployments SET deployment_spec_json = $2 WHERE id = $1`, deploymentID, specJSON)
	if err != nil {
		return fmt.Errorf("update deployment spec JSON: %w", err)
	}
	return nil
}

// UpdateDeploymentPending updates an existing deployment for a redeploy, creating a new revision.
// Sets status='pending'. The txFn callback runs in the same transaction.
func (s *Store) UpdateDeploymentPending(p SaveDeploymentParams, txFn func(tx *sql.Tx, deploymentID string) error) (*Deployment, error) {
	if err := s.validateLineage(p); err != nil {
		return nil, err
	}

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

	// Update deployment row. source_account_id is refreshed on every redeploy
	// so the column stays truthful even if the publisher is re-keyed.
	var d Deployment
	err = tx.QueryRow(`
		UPDATE deployments
		SET build_id = $2, deployment_spec_json = $3, encrypted_data_key = $4,
		    kms_key_arn = $5, display_name = $6, status = $7,
		    error_message = NULL, error_details = NULL,
		    status_changed_at = NOW(), current_revision = $8, deployed_at = NOW(),
		    source_account_id = COALESCE($9, source_account_id)
		WHERE id = $1
		RETURNING id, account_id, source_account_id, agent_name, build_id, namespace, display_name,
		    deployment_spec_json, status, deployed_at
	`, p.ID, p.BuildID, p.SpecJSON, p.EncryptedDataKey, nilIfEmpty(p.KMSKeyARN),
		p.DisplayName, StatusPending, nextRevision, nilIfEmpty(p.SourceAccountID)).Scan(
		&d.ID, &d.AccountID, &d.SourceAccountID, &d.AgentName, &d.BuildID, &d.Namespace,
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

	// Delete old normalized data (cascades from deployment_workloads/sidecars)
	_, err = tx.Exec(`DELETE FROM deployment_workloads WHERE deployment_id = $1`, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old workloads: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM deployment_sidecars WHERE deployment_id = $1`, p.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to delete old sidecars: %w", err)
	}
	_, err = tx.Exec(`DELETE FROM deployment_build_env WHERE deployment_id = $1`, p.ID)
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

// RecoverOrphanedDeployment inserts a stub deployment record for an orphaned K8s
// namespace that has no matching database row. The deployment is created with status
// 'failed' and no revisions, so the user can redeploy or undeploy to fix it.
//
// sourceAccountID is the account that originally published the build (read from
// the namespace's astro.dev/source-account-id label by the reconciler). Pre-PR2
// namespaces lack the label and the reconciler defaults sourceAccountID to
// accountID before calling here, so this routine never has to make that
// inference itself — the caller is the right place to log the fallback.
func (s *Store) RecoverOrphanedDeployment(id, accountID, sourceAccountID, agentName, buildID, namespace string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.Exec(`
		INSERT INTO deployments (id, account_id, source_account_id, agent_name, build_id, namespace,
		    deployment_spec_json, status, error_message, status_changed_at, deployed_at)
		VALUES ($1, $2, $3, $4, $5, $6, '{}', $7, $8, NOW(), NOW())
		ON CONFLICT (id) DO NOTHING
	`, id, accountID, sourceAccountID, agentName, buildID, namespace, StatusFailed,
		"Recovered from orphaned K8s namespace — redeploy or undeploy to fix")
	if err != nil {
		return fmt.Errorf("failed to insert recovered deployment: %w", err)
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		// Deployment already exists — nothing to recover.
		return nil
	}

	_, err = tx.Exec(`
		INSERT INTO deployment_events (deployment_id, status, message)
		VALUES ($1, $2, $3)
	`, id, StatusFailed, "Deployment recovered from orphaned K8s namespace")
	if err != nil {
		return fmt.Errorf("failed to insert recovery event: %w", err)
	}

	return tx.Commit()
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

// DeploymentSummary is a lightweight projection of a deployment for listing UIs.
type DeploymentSummary struct {
	ID           string           `json:"id"`
	AccountID    string           `json:"account_id"`
	AgentName    string           `json:"agent_name"`
	DisplayName  string           `json:"display_name,omitempty"`
	Status       string           `json:"status"`
	AvatarColors *json.RawMessage `json:"avatar_colors,omitempty"`
	DeployedAt   time.Time        `json:"deployed_at"`
}

// GetSummariesForAccounts returns lightweight deployment summaries for all
// visible deployments across the given account IDs in a single query.
func (s *Store) GetSummariesForAccounts(accountIDs []string) ([]*DeploymentSummary, error) {
	rows, err := s.db.Query(`
		SELECT id, account_id, agent_name, display_name, status, avatar_colors, deployed_at
		FROM deployments
		WHERE account_id = ANY($1) AND status != 'undeployed'
		ORDER BY deployed_at DESC
	`, pq.Array(accountIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to query deployment summaries: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var summaries []*DeploymentSummary
	for rows.Next() {
		var d DeploymentSummary
		if err := rows.Scan(&d.ID, &d.AccountID, &d.AgentName, &d.DisplayName, &d.Status, &d.AvatarColors, &d.DeployedAt); err != nil {
			return nil, fmt.Errorf("failed to scan deployment summary: %w", err)
		}
		summaries = append(summaries, &d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating deployment summaries: %w", err)
	}
	return summaries, nil
}
