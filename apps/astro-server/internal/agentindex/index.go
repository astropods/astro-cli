package agentindex

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
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

// Index manages the registry of published agents using SQLite
type Index struct {
	db *sql.DB
}

// NewIndex creates a new agent index with SQLite backend
func NewIndex(dbPath string) (*Index, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create index directory: %w", err)
	}

	// Open SQLite database
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	idx := &Index{db: db}

	// Initialize schema
	if err := idx.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return idx, nil
}

// Close closes the database connection
func (idx *Index) Close() error {
	return idx.db.Close()
}

// initSchema creates the database tables
func (idx *Index) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		name TEXT NOT NULL PRIMARY KEY,
		registry TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS agent_versions (
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		spec_json TEXT NOT NULL,
		published_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		PRIMARY KEY (name, version),
		FOREIGN KEY (name) REFERENCES agents(name) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_versions_agent ON agent_versions(name);
	`

	_, err := idx.db.Exec(schema)
	return err
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

	// Insert or update agent using REPLACE which is more compatible
	// First check if agent exists to preserve created_at
	var existingCreatedAt *time.Time
	err = tx.QueryRow(`SELECT created_at FROM agents WHERE name = ?`, name).Scan(&existingCreatedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query existing agent: %w", err)
	}

	createdAt := now
	if existingCreatedAt != nil {
		createdAt = *existingCreatedAt
	}

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO agents (name, registry, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`, name, registry, createdAt, now)
	if err != nil {
		return fmt.Errorf("failed to insert agent: %w", err)
	}

	// Insert or update version using REPLACE
	// Check if version exists to preserve published_at
	var existingPublishedAt *time.Time
	err = tx.QueryRow(`SELECT published_at FROM agent_versions WHERE name = ? AND version = ?`, name, version).Scan(&existingPublishedAt)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query existing version: %w", err)
	}

	publishedAt := now
	if existingPublishedAt != nil {
		publishedAt = *existingPublishedAt
	}

	_, err = tx.Exec(`
		INSERT OR REPLACE INTO agent_versions (name, version, spec_json, published_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, name, version, string(specJSON), publishedAt, now)
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
		WHERE name = ?
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
		WHERE name = ?
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
		WHERE name = ? AND version = ?
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
			WHERE name = ?
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
		WHERE name = ?
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
		WHERE name = ? AND version = ?
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
		WHERE name = ?
	`, name).Scan(&count)
	if err != nil {
		return fmt.Errorf("failed to count versions: %w", err)
	}

	if count == 0 {
		_, err = idx.db.Exec(`
			DELETE FROM agents
			WHERE name = ?
		`, name)
		if err != nil {
			return fmt.Errorf("failed to delete agent: %w", err)
		}
	}

	return nil
}
