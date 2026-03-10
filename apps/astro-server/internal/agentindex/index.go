package agentindex

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// AgentVersion represents a specific published build of an agent
type AgentVersion struct {
	BuildID            string           `json:"build_id"`
	ECRNamespace       string           `json:"ecr_namespace"`
	Spec               map[string]any   `json:"spec"`
	Readme             string           `json:"readme"`
	ValidationWarnings []map[string]any `json:"validation_warnings,omitempty"`
	PublishedAt        time.Time        `json:"published_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// Agent represents an agent with all its versions (ordered newest first)
type Agent struct {
	AccountID  string          `json:"account_id"`
	Name       string          `json:"name"`
	Registry   string          `json:"registry"`
	Visibility string          `json:"visibility"`
	Versions   []*AgentVersion `json:"versions"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

// Index manages the registry of published agents using PostgreSQL
type Index struct {
	db *sql.DB
}

// NewIndexWithDB creates a new agent index with a provided database connection
func NewIndexWithDB(db *sql.DB) *Index {
	return &Index{db: db}
}

// NewIndex creates a new agent index with PostgreSQL backend
func NewIndex(databaseURL string) (*Index, error) {
	// Open PostgreSQL database
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Verify connection
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Index{db: db}, nil
}

// Close closes the database connection
func (idx *Index) Close() error {
	return idx.db.Close()
}

// parseValidationWarnings parses a JSON string into a slice of validation warning maps.
// Returns nil for empty strings or invalid JSON.
func parseValidationWarnings(raw string) []map[string]any {
	if raw == "" {
		return nil
	}
	var warnings []map[string]any
	if err := json.Unmarshal([]byte(raw), &warnings); err != nil {
		return nil
	}
	if len(warnings) == 0 {
		return nil
	}
	return warnings
}

// Register adds or updates an agent build in the index
func (idx *Index) Register(accountID, name, buildID, registry, ecrNamespace string, spec map[string]any, readme string, validationWarnings string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()

	// Marshal spec to JSON
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}

	// Insert or update agent using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agents (account_id, name, registry, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, name) DO UPDATE SET registry = $3, updated_at = $5
	`, accountID, name, registry, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert agent: %w", err)
	}

	// Insert or update version using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agent_versions (account_id, name, build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (account_id, name, build_id) DO UPDATE SET spec_json = $5, readme = $6, validation_warnings = $7, updated_at = $9
	`, accountID, name, buildID, ecrNamespace, string(specJSON), readme, validationWarnings, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert version: %w", err)
	}

	return tx.Commit()
}

// Get retrieves an agent by account ID and name
func (idx *Index) Get(accountID, name string) (*Agent, error) {
	var agent Agent
	err := idx.db.QueryRow(`
		SELECT account_id, name, registry, visibility, created_at, updated_at
		FROM agents
		WHERE account_id = $1 AND name = $2
	`, accountID, name).Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.CreatedAt, &agent.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query agent: %w", err)
	}

	// Load versions ordered newest first
	rows, err := idx.db.Query(`
		SELECT build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2
		ORDER BY published_at DESC
	`, accountID, name)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var v AgentVersion
		var specJSON, warningsJSON string
		if err := rows.Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}

		if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}
		v.ValidationWarnings = parseValidationWarnings(warningsJSON)

		agent.Versions = append(agent.Versions, &v)
	}

	return &agent, nil
}

// GetVersion retrieves a specific build of an agent by build ID
func (idx *Index) GetVersion(accountID, name, buildID string) (*AgentVersion, error) {
	var v AgentVersion
	var specJSON, warningsJSON string
	err := idx.db.QueryRow(`
		SELECT build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, accountID, name, buildID).Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("build not found: %s", buildID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query version: %w", err)
	}

	if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}
	v.ValidationWarnings = parseValidationWarnings(warningsJSON)

	return &v, nil
}

// List returns all agents in the index (global browse)
func (idx *Index) List() ([]*Agent, error) {
	rows, err := idx.db.Query(`
		SELECT account_id, name, registry, visibility, created_at, updated_at
		FROM agents
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		if err := rows.Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}

		// Load versions ordered newest first
		versionRows, err := idx.db.Query(`
			SELECT build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at
			FROM agent_versions
			WHERE account_id = $1 AND name = $2
			ORDER BY published_at DESC
		`, agent.AccountID, agent.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to query versions: %w", err)
		}

		for versionRows.Next() {
			var v AgentVersion
			var specJSON, warningsJSON string
			if err := versionRows.Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
				_ = versionRows.Close()
				return nil, fmt.Errorf("failed to scan version: %w", err)
			}
			if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
				_ = versionRows.Close()
				return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
			}
			v.ValidationWarnings = parseValidationWarnings(warningsJSON)
			agent.Versions = append(agent.Versions, &v)
		}
		_ = versionRows.Close()

		agents = append(agents, &agent)
	}

	return agents, nil
}

// ListForAccount returns all agents belonging to a specific account
func (idx *Index) ListForAccount(accountID string) ([]*Agent, error) {
	rows, err := idx.db.Query(`
		SELECT account_id, name, registry, visibility, created_at, updated_at
		FROM agents
		WHERE account_id = $1
		ORDER BY name
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		if err := rows.Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}

		versionRows, err := idx.db.Query(`
			SELECT build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at
			FROM agent_versions
			WHERE account_id = $1 AND name = $2
			ORDER BY published_at DESC
		`, agent.AccountID, agent.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to query versions: %w", err)
		}

		for versionRows.Next() {
			var v AgentVersion
			var specJSON, warningsJSON string
			if err := versionRows.Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
				_ = versionRows.Close()
				return nil, fmt.Errorf("failed to scan version: %w", err)
			}
			if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
				_ = versionRows.Close()
				return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
			}
			v.ValidationWarnings = parseValidationWarnings(warningsJSON)
			agent.Versions = append(agent.Versions, &v)
		}
		_ = versionRows.Close()

		agents = append(agents, &agent)
	}

	return agents, nil
}

// Delete removes an agent and all its versions from the index
func (idx *Index) Delete(accountID, name string) error {
	result, err := idx.db.Exec(`
		DELETE FROM agents
		WHERE account_id = $1 AND name = $2
	`, accountID, name)
	if err != nil {
		return fmt.Errorf("failed to delete agent: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("agent not found: %s", name)
	}

	return nil
}

// DeleteVersion removes a specific build of an agent
func (idx *Index) DeleteVersion(accountID, name, buildID string) error {
	result, err := idx.db.Exec(`
		DELETE FROM agent_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, accountID, name, buildID)
	if err != nil {
		return fmt.Errorf("failed to delete version: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("build not found: %s", buildID)
	}

	// Check if this was the last version, if so delete the agent
	var count int
	err = idx.db.QueryRow(`
		SELECT COUNT(*) FROM agent_versions
		WHERE account_id = $1 AND name = $2
	`, accountID, name).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count versions: %w", err)
	}

	if count == 0 {
		_, err = idx.db.Exec(`
			DELETE FROM agents
			WHERE account_id = $1 AND name = $2
		`, accountID, name)
		if err != nil {
			return fmt.Errorf("failed to delete agent: %w", err)
		}
	}

	return nil
}

// SetVisibility updates the visibility of an agent (public or private)
func (idx *Index) SetVisibility(accountID, name, visibility string) error {
	if visibility != "public" && visibility != "private" {
		return fmt.Errorf("invalid visibility: %s (must be 'public' or 'private')", visibility)
	}

	result, err := idx.db.Exec(`
		UPDATE agents SET visibility = $1, updated_at = $2
		WHERE account_id = $3 AND name = $4
	`, visibility, time.Now(), accountID, name)
	if err != nil {
		return fmt.Errorf("failed to update visibility: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", name)
	}

	return nil
}

// ListPublicAgents returns agents with visibility='public' and their latest version
func (idx *Index) ListPublicAgents() ([]*Agent, error) {
	rows, err := idx.db.Query(`
		SELECT a.account_id, a.name, a.registry, a.visibility, a.created_at, a.updated_at,
		       v.build_id, v.ecr_namespace, v.spec_json, v.readme, v.published_at, v.updated_at
		FROM agents a
		JOIN agent_versions v ON a.account_id = v.account_id AND a.name = v.name
		WHERE a.visibility = 'public'
		AND v.published_at = (
			SELECT MAX(v2.published_at) FROM agent_versions v2
			WHERE v2.account_id = a.account_id AND v2.name = a.name
		)
		ORDER BY a.name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query public agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		var v AgentVersion
		var specJSON string

		if err := rows.Scan(
			&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.CreatedAt, &agent.UpdatedAt,
			&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &v.PublishedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}

		agent.Versions = []*AgentVersion{&v}
		agents = append(agents, &agent)
	}

	return agents, nil
}

// GetLatestVersion returns the most recently registered build for an agent
func (idx *Index) GetLatestVersion(accountID, name string) (*AgentVersion, error) {
	var v AgentVersion
	var specJSON, warningsJSON string
	err := idx.db.QueryRow(`
		SELECT build_id, ecr_namespace, spec_json, readme, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2
		ORDER BY published_at DESC
		LIMIT 1
	`, accountID, name).Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("no builds found for agent: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query latest version: %w", err)
	}

	if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}
	v.ValidationWarnings = parseValidationWarnings(warningsJSON)

	return &v, nil
}

// Transfer moves an agent and all its versions from one account to another.
// The ecr_namespace on each version is intentionally left unchanged so that
// existing images continue to resolve to their original ECR repositories.
func (idx *Index) Transfer(sourceAccountID, targetAccountID, agentName string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()

	result, err := tx.Exec(`
		UPDATE agents SET account_id = $1, updated_at = $2
		WHERE account_id = $3 AND name = $4
	`, targetAccountID, now, sourceAccountID, agentName)
	if err != nil {
		return fmt.Errorf("failed to transfer agent: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("agent not found: %s", agentName)
	}

	_, err = tx.Exec(`
		UPDATE agent_versions SET account_id = $1, updated_at = $2
		WHERE account_id = $3 AND name = $4
	`, targetAccountID, now, sourceAccountID, agentName)
	if err != nil {
		return fmt.Errorf("failed to transfer versions: %w", err)
	}

	return tx.Commit()
}
