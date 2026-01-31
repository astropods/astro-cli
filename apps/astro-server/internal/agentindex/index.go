package agentindex

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

// AgentVersion represents a specific version of an agent
type AgentVersion struct {
	Version     string                 `json:"version"`
	Spec        map[string]interface{} `json:"spec"`
	PublishedAt time.Time              `json:"published_at"`
	UpdatedAt   time.Time              `json:"updated_at"`
}

// Agent represents an agent with all its versions
type Agent struct {
	Name      string                   `json:"name"`
	Registry  string                   `json:"registry"`
	Versions  map[string]*AgentVersion `json:"versions"`
	CreatedAt time.Time                `json:"created_at"`
	UpdatedAt time.Time                `json:"updated_at"`
}

// Index manages the registry of published agents using PostgreSQL
type Index struct {
	db *sql.DB
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
		db.Close()
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	return &Index{db: db}, nil
}

// Close closes the database connection
func (idx *Index) Close() error {
	return idx.db.Close()
}

// Register adds or updates an agent version in the index
func (idx *Index) Register(name, version, registry string, spec map[string]interface{}) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now()

	// Marshal spec to JSON
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}

	// Insert or update agent using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agents (name, registry, created_at, updated_at)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (name) DO UPDATE SET registry = $2, updated_at = $4
	`, name, registry, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert agent: %w", err)
	}

	// Insert or update version using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agent_versions (name, version, spec_json, published_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name, version) DO UPDATE SET spec_json = $3, updated_at = $5
	`, name, version, string(specJSON), now, now)
	if err != nil {
		return fmt.Errorf("failed to insert version: %w", err)
	}

	return tx.Commit()
}

// Get retrieves an agent by name
func (idx *Index) Get(name string) (*Agent, error) {
	var agent Agent
	err := idx.db.QueryRow(`
		SELECT name, registry, created_at, updated_at
		FROM agents
		WHERE name = $1
	`, name).Scan(&agent.Name, &agent.Registry, &agent.CreatedAt, &agent.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query agent: %w", err)
	}

	// Load versions
	agent.Versions = make(map[string]*AgentVersion)
	rows, err := idx.db.Query(`
		SELECT version, spec_json, published_at, updated_at
		FROM agent_versions
		WHERE name = $1
	`, name)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v AgentVersion
		var specJSON string
		if err := rows.Scan(&v.Version, &specJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan version: %w", err)
		}

		// Unmarshal spec JSON
		if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}

		agent.Versions[v.Version] = &v
	}

	return &agent, nil
}

// GetVersion retrieves a specific version of an agent
func (idx *Index) GetVersion(name, version string) (*AgentVersion, error) {
	var v AgentVersion
	var specJSON string
	err := idx.db.QueryRow(`
		SELECT version, spec_json, published_at, updated_at
		FROM agent_versions
		WHERE name = $1 AND version = $2
	`, name, version).Scan(&v.Version, &specJSON, &v.PublishedAt, &v.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("version not found: %s", version)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query version: %w", err)
	}

	// Unmarshal spec JSON
	if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return &v, nil
}

// List returns all agents in the index
func (idx *Index) List() ([]*Agent, error) {
	rows, err := idx.db.Query(`
		SELECT name, registry, created_at, updated_at
		FROM agents
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		if err := rows.Scan(&agent.Name, &agent.Registry, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}

		// Load versions for this agent
		agent.Versions = make(map[string]*AgentVersion)
		versionRows, err := idx.db.Query(`
			SELECT version, spec_json, published_at, updated_at
			FROM agent_versions
			WHERE name = $1
		`, agent.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to query versions: %w", err)
		}

		for versionRows.Next() {
			var v AgentVersion
			var specJSON string
			if err := versionRows.Scan(&v.Version, &specJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
				versionRows.Close()
				return nil, fmt.Errorf("failed to scan version: %w", err)
			}

			// Unmarshal spec JSON
			if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
				versionRows.Close()
				return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
			}

			agent.Versions[v.Version] = &v
		}
		versionRows.Close()

		agents = append(agents, &agent)
	}

	return agents, nil
}

// Delete removes an agent and all its versions from the index
func (idx *Index) Delete(name string) error {
	result, err := idx.db.Exec(`
		DELETE FROM agents
		WHERE name = $1
	`, name)
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

// DeleteVersion removes a specific version of an agent
func (idx *Index) DeleteVersion(name, version string) error {
	result, err := idx.db.Exec(`
		DELETE FROM agent_versions
		WHERE name = $1 AND version = $2
	`, name, version)
	if err != nil {
		return fmt.Errorf("failed to delete version: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("version not found: %s", version)
	}

	// Check if this was the last version, if so delete the agent
	var count int
	err = idx.db.QueryRow(`
		SELECT COUNT(*) FROM agent_versions
		WHERE name = $1
	`, name).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count versions: %w", err)
	}

	if count == 0 {
		_, err = idx.db.Exec(`
			DELETE FROM agents
			WHERE name = $1
		`, name)
		if err != nil {
			return fmt.Errorf("failed to delete agent: %w", err)
		}
	}

	return nil
}
