package agentindex

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

var semverRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)

// AgentVersion represents a specific published build of an agent
type AgentVersion struct {
	BuildID            string           `json:"build_id"`
	Spec               map[string]any   `json:"spec"`
	Readme             string           `json:"readme"`
	ValidationWarnings []map[string]any `json:"validation_warnings,omitempty"`
	PublishedAt        time.Time        `json:"published_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
}

// Agent represents an agent with all its versions (ordered newest first)
type Agent struct {
	AccountID string          `json:"account_id"`
	Name      string          `json:"name"`
	Registry  string          `json:"registry"`
	Versions  []*AgentVersion `json:"versions"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// PublishedVersion represents a semver-published build of an agent
type PublishedVersion struct {
	Version     string         `json:"version"`
	BuildID     string         `json:"build_id"`
	Spec        map[string]any `json:"spec"`
	Readme      string         `json:"readme"`
	PublishedAt time.Time      `json:"published_at"`
}

// PublicAgent represents an agent with only its published versions
type PublicAgent struct {
	AccountID  string              `json:"account_id"`
	Name       string              `json:"name"`
	Registry   string              `json:"registry"`
	Versions   []*PublishedVersion `json:"versions"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
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
		db.Close()
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
func (idx *Index) Register(accountID, name, buildID, registry string, spec map[string]any, readme string, validationWarnings string) error {
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
		INSERT INTO agents (account_id, name, registry, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, name) DO UPDATE SET registry = $3, updated_at = $5
	`, accountID, name, registry, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert agent: %w", err)
	}

	// Insert or update version using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agent_versions (account_id, name, build_id, spec_json, readme, validation_warnings, published_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (account_id, name, build_id) DO UPDATE SET spec_json = $4, readme = $5, validation_warnings = $6, updated_at = $8
	`, accountID, name, buildID, string(specJSON), readme, validationWarnings, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert version: %w", err)
	}

	return tx.Commit()
}

// Get retrieves an agent by account ID and name
func (idx *Index) Get(accountID, name string) (*Agent, error) {
	var agent Agent
	err := idx.db.QueryRow(`
		SELECT account_id, name, registry, created_at, updated_at
		FROM agents
		WHERE account_id = $1 AND name = $2
	`, accountID, name).Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.CreatedAt, &agent.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query agent: %w", err)
	}

	// Load versions ordered newest first
	rows, err := idx.db.Query(`
		SELECT build_id, spec_json, readme, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2
		ORDER BY published_at DESC
	`, accountID, name)
	if err != nil {
		return nil, fmt.Errorf("failed to query versions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var v AgentVersion
		var specJSON, warningsJSON string
		if err := rows.Scan(&v.BuildID, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
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
		SELECT build_id, spec_json, readme, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, accountID, name, buildID).Scan(&v.BuildID, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

	if err == sql.ErrNoRows {
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
		SELECT account_id, name, registry, created_at, updated_at
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
		if err := rows.Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}

		// Load versions ordered newest first
		versionRows, err := idx.db.Query(`
			SELECT build_id, spec_json, readme, validation_warnings, published_at, updated_at
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
			if err := versionRows.Scan(&v.BuildID, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
				versionRows.Close()
				return nil, fmt.Errorf("failed to scan version: %w", err)
			}
			if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
				versionRows.Close()
				return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
			}
			v.ValidationWarnings = parseValidationWarnings(warningsJSON)
			agent.Versions = append(agent.Versions, &v)
		}
		versionRows.Close()

		agents = append(agents, &agent)
	}

	return agents, nil
}

// ListForAccount returns all agents belonging to a specific account
func (idx *Index) ListForAccount(accountID string) ([]*Agent, error) {
	rows, err := idx.db.Query(`
		SELECT account_id, name, registry, created_at, updated_at
		FROM agents
		WHERE account_id = $1
		ORDER BY name
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close()

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		if err := rows.Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}

		versionRows, err := idx.db.Query(`
			SELECT build_id, spec_json, readme, validation_warnings, published_at, updated_at
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
			if err := versionRows.Scan(&v.BuildID, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
				versionRows.Close()
				return nil, fmt.Errorf("failed to scan version: %w", err)
			}
			if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
				versionRows.Close()
				return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
			}
			v.ValidationWarnings = parseValidationWarnings(warningsJSON)
			agent.Versions = append(agent.Versions, &v)
		}
		versionRows.Close()

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

// compareSemver compares two semver strings (without "v" prefix).
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Pre-release versions are compared lexicographically; build metadata is ignored.
func compareSemver(a, b string) int {
	// Strip build metadata
	if i := strings.IndexByte(a, '+'); i >= 0 {
		a = a[:i]
	}
	if i := strings.IndexByte(b, '+'); i >= 0 {
		b = b[:i]
	}

	// Split into core and pre-release
	aParts := strings.SplitN(a, "-", 2)
	bParts := strings.SplitN(b, "-", 2)

	// Compare major.minor.patch
	aCore := strings.Split(aParts[0], ".")
	bCore := strings.Split(bParts[0], ".")
	for i := 0; i < 3; i++ {
		av, _ := strconv.Atoi(aCore[i])
		bv, _ := strconv.Atoi(bCore[i])
		if av < bv {
			return -1
		}
		if av > bv {
			return 1
		}
	}

	// Core is equal; check pre-release (having a pre-release is lower precedence)
	aHasPre := len(aParts) > 1
	bHasPre := len(bParts) > 1
	if !aHasPre && !bHasPre {
		return 0
	}
	if !aHasPre && bHasPre {
		return 1 // release > pre-release
	}
	if aHasPre && !bHasPre {
		return -1
	}

	// Both have pre-release: lexicographic comparison
	if aParts[1] < bParts[1] {
		return -1
	}
	if aParts[1] > bParts[1] {
		return 1
	}
	return 0
}

// Publish assigns a semver version to a build, making it publicly visible.
// The version must be strictly greater than any existing published version.
func (idx *Index) Publish(accountID, name, buildID, version string) error {
	if !semverRegex.MatchString(version) {
		return fmt.Errorf("invalid semver: %s", version)
	}

	// Check the latest published version and enforce monotonic increase
	var latestVersion sql.NullString
	err := idx.db.QueryRow(`
		SELECT version FROM agent_published_versions
		WHERE account_id = $1 AND name = $2
		ORDER BY published_at DESC
		LIMIT 1
	`, accountID, name).Scan(&latestVersion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query latest version: %w", err)
	}

	if latestVersion.Valid {
		cmp := compareSemver(version, latestVersion.String)
		if cmp == 0 {
			return fmt.Errorf("version %s is already published", version)
		}
		if cmp < 0 {
			return fmt.Errorf("version %s is lower than the current latest %s", version, latestVersion.String)
		}
	}

	_, err = idx.db.Exec(`
		INSERT INTO agent_published_versions (account_id, name, version, build_id)
		VALUES ($1, $2, $3, $4)
	`, accountID, name, version, buildID)
	if err != nil {
		return fmt.Errorf("failed to publish version: %w", err)
	}
	return nil
}

// ListPublic returns agents with their latest published version only (one per agent)
func (idx *Index) ListPublic() ([]*PublicAgent, error) {
	rows, err := idx.db.Query(`
		SELECT a.account_id, a.name, a.registry, a.created_at, a.updated_at,
		       p.version, p.build_id, v.spec_json, v.readme, p.published_at
		FROM agents a
		JOIN agent_published_versions p ON a.account_id = p.account_id AND a.name = p.name
		JOIN agent_versions v ON p.account_id = v.account_id AND p.name = v.name AND p.build_id = v.build_id
		WHERE p.published_at = (
			SELECT MAX(p2.published_at) FROM agent_published_versions p2
			WHERE p2.account_id = a.account_id AND p2.name = a.name
		)
		ORDER BY a.name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query public agents: %w", err)
	}
	defer rows.Close()

	var agents []*PublicAgent
	for rows.Next() {
		var accountID, name, registry, version, buildID, specJSON, readme string
		var createdAt, updatedAt, publishedAt time.Time

		if err := rows.Scan(&accountID, &name, &registry, &createdAt, &updatedAt,
			&version, &buildID, &specJSON, &readme, &publishedAt); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		var spec map[string]any
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}

		agents = append(agents, &PublicAgent{
			AccountID: accountID,
			Name:      name,
			Registry:  registry,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
			Versions: []*PublishedVersion{{
				Version:     version,
				BuildID:     buildID,
				Spec:        spec,
				Readme:      readme,
				PublishedAt: publishedAt,
			}},
		})
	}

	return agents, nil
}

// GetPublicVersion resolves a semver version to a build and returns it
func (idx *Index) GetPublicVersion(accountID, name, version string) (*PublishedVersion, error) {
	var buildID, specJSON, readme string
	var publishedAt time.Time

	err := idx.db.QueryRow(`
		SELECT p.build_id, v.spec_json, v.readme, p.published_at
		FROM agent_published_versions p
		JOIN agent_versions v ON p.account_id = v.account_id AND p.name = v.name AND p.build_id = v.build_id
		WHERE p.account_id = $1 AND p.name = $2 AND p.version = $3
	`, accountID, name, version).Scan(&buildID, &specJSON, &readme, &publishedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("published version not found: %s@%s", name, version)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query published version: %w", err)
	}

	var spec map[string]any
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return &PublishedVersion{
		Version:     version,
		BuildID:     buildID,
		Spec:        spec,
		Readme:      readme,
		PublishedAt: publishedAt,
	}, nil
}

// GetPublishedVersionsForAgent returns all published semver mappings for one agent
func (idx *Index) GetPublishedVersionsForAgent(accountID, name string) ([]*PublishedVersion, error) {
	rows, err := idx.db.Query(`
		SELECT p.version, p.build_id, v.spec_json, v.readme, p.published_at
		FROM agent_published_versions p
		JOIN agent_versions v ON p.account_id = v.account_id AND p.name = v.name AND p.build_id = v.build_id
		WHERE p.account_id = $1 AND p.name = $2
		ORDER BY p.published_at DESC
	`, accountID, name)
	if err != nil {
		return nil, fmt.Errorf("failed to query published versions: %w", err)
	}
	defer rows.Close()

	var versions []*PublishedVersion
	for rows.Next() {
		var version, buildID, specJSON, readme string
		var publishedAt time.Time

		if err := rows.Scan(&version, &buildID, &specJSON, &readme, &publishedAt); err != nil {
			return nil, fmt.Errorf("failed to scan published version: %w", err)
		}

		var spec map[string]any
		if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
			return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
		}

		versions = append(versions, &PublishedVersion{
			Version:     version,
			BuildID:     buildID,
			Spec:        spec,
			Readme:      readme,
			PublishedAt: publishedAt,
		})
	}
	return versions, nil
}

// GetLatestVersion returns the most recently registered build for an agent
func (idx *Index) GetLatestVersion(accountID, name string) (*AgentVersion, error) {
	var v AgentVersion
	var specJSON, warningsJSON string
	err := idx.db.QueryRow(`
		SELECT build_id, spec_json, readme, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2
		ORDER BY published_at DESC
		LIMIT 1
	`, accountID, name).Scan(&v.BuildID, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

	if err == sql.ErrNoRows {
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

// GetLatestPublishedVersion returns the most recently published version for cross-account deploys
func (idx *Index) GetLatestPublishedVersion(accountID, name string) (*AgentVersion, error) {
	var v AgentVersion
	var specJSON, warningsJSON string
	err := idx.db.QueryRow(`
		SELECT v.build_id, v.spec_json, v.readme, v.validation_warnings, v.published_at, v.updated_at
		FROM agent_published_versions p
		JOIN agent_versions v ON p.account_id = v.account_id AND p.name = v.name AND p.build_id = v.build_id
		WHERE p.account_id = $1 AND p.name = $2
		ORDER BY p.published_at DESC
		LIMIT 1
	`, accountID, name).Scan(&v.BuildID, &specJSON, &v.Readme, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no published versions found for agent: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query latest published version: %w", err)
	}

	if err := json.Unmarshal([]byte(specJSON), &v.Spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}
	v.ValidationWarnings = parseValidationWarnings(warningsJSON)

	return &v, nil
}

// IsPublishedBuild checks if a specific build_id has a published semver version
func (idx *Index) IsPublishedBuild(accountID, name, buildID string) (bool, error) {
	var count int
	err := idx.db.QueryRow(`
		SELECT COUNT(*) FROM agent_published_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, accountID, name, buildID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check published build: %w", err)
	}
	return count > 0, nil
}
