package agentindex

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/blueprintcache"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/lib/pq"
)

// AgentVersion represents a specific published build of an agent
type AgentVersion struct {
	BuildID            string           `json:"build_id"`
	ECRNamespace       string           `json:"ecr_namespace"`
	Spec               map[string]any   `json:"spec"`
	Readme             string           `json:"readme"`
	AgentCardJSON      string           `json:"agent_card_json,omitempty"`
	ValidationWarnings []map[string]any `json:"validation_warnings,omitempty"`
	PublishedAt        time.Time        `json:"published_at"`
	UpdatedAt          time.Time        `json:"updated_at"`
	// CommitMessage, CommitSHA, and RepoFullName describe the git commit that
	// produced this build. They are populated only for GitHub-sourced builds and
	// only by list queries that join github_builds. Empty for direct CLI pushes.
	CommitMessage string `json:"commit_message,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	RepoFullName  string `json:"repo_full_name,omitempty"`
}

// Agent represents an agent with all its versions (ordered newest first)
type Agent struct {
	AccountID  string          `json:"account_id"`
	Name       string          `json:"name"`
	Registry   string          `json:"registry"`
	Visibility string          `json:"visibility"`
	Versions   []*AgentVersion `json:"versions"`
	// VersionCount is set by list queries (total builds); use instead of len(Versions) when only the latest version is loaded.
	VersionCount    int              `json:"-"`
	ArchivedAt      *time.Time       `json:"archived_at,omitempty"`
	NameReserved    bool             `json:"name_reserved"`
	AvatarColors    *json.RawMessage `json:"avatar_colors,omitempty"`
	AvatarUpdatedAt *time.Time       `json:"avatar_updated_at,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`
}

// Index manages the registry of published agents using PostgreSQL
type Index struct {
	db    *sql.DB
	cache k8scache.Cache
}

// NewIndexWithDB creates a new agent index with a provided database connection
func NewIndexWithDB(db *sql.DB) *Index {
	return &Index{db: db}
}

// WithCache enables invalidation for the cross-account blueprint list.
func (idx *Index) WithCache(cache k8scache.Cache) *Index {
	idx.cache = cache
	return idx
}

func (idx *Index) invalidateBlueprintLists(accountIDs ...string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	for _, accountID := range accountIDs {
		_ = blueprintcache.Invalidate(ctx, idx.cache, accountID)
	}
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
func (idx *Index) Register(accountID, name, buildID, registry, ecrNamespace string, spec map[string]any, readme string, agentCardJSON string, validationWarnings string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()

	// Strip name from spec before storage — the canonical name lives in the agents table.
	delete(spec, "name")

	// Marshal spec to JSON
	specJSON, err := json.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec: %w", err)
	}

	// Insert or update agent using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agents (account_id, name, registry, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (account_id, name) DO UPDATE SET registry = $3, updated_at = $5, archived_at = NULL
	`, accountID, name, registry, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert agent: %w", err)
	}

	if agentCardJSON == "" {
		agentCardJSON = "null"
	}

	// Insert or update version using ON CONFLICT
	_, err = tx.Exec(`
		INSERT INTO agent_versions (account_id, name, build_id, ecr_namespace, spec_json, readme, agent_card_json, validation_warnings, published_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (account_id, name, build_id) DO UPDATE SET spec_json = $5, readme = $6, agent_card_json = $7, validation_warnings = $8, updated_at = $10
	`, accountID, name, buildID, ecrNamespace, string(specJSON), readme, agentCardJSON, validationWarnings, now, now)
	if err != nil {
		return fmt.Errorf("failed to insert version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	idx.invalidateBlueprintLists(accountID)
	return nil
}

// ErrAlreadyExists is returned by Create when an active (non-archived) agent with the same name exists.
var ErrAlreadyExists = fmt.Errorf("agent already exists")

// Create inserts a new agent record. If an archived agent with the same name exists it is
// unarchived instead and its old versions are cleared so it starts as a clean draft.
// Returns ErrAlreadyExists if a non-archived agent with that name exists.
func (idx *Index) Create(accountID, name string) error {
	tx, err := idx.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	now := time.Now()
	result, err := tx.Exec(`
		INSERT INTO agents (account_id, name, registry, created_at, updated_at)
		VALUES ($1, $2, '', $3, $4)
		ON CONFLICT (account_id, name) DO UPDATE SET
		  archived_at = NULL,
		  registry    = '',
		  updated_at  = $4
		WHERE agents.archived_at IS NOT NULL
	`, accountID, name, now, now)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrAlreadyExists
	}

	// Clear any stale versions from before archival so the agent starts as a
	// clean draft — build IDs are only created by `ast push`, not the UI flow.
	if _, err := tx.Exec(`
		DELETE FROM agent_versions WHERE account_id = $1 AND name = $2
	`, accountID, name); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	idx.invalidateBlueprintLists(accountID)
	return nil
}

// Exists reports whether the account owns a live agent by that name, without
// loading the agent or its versions. An archived agent does not exist.
func (idx *Index) Exists(accountID, name string) (bool, error) {
	var exists bool
	if err := idx.db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM agents
			WHERE account_id = $1 AND name = $2 AND archived_at IS NULL
		)
	`, accountID, name).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to query agent: %w", err)
	}
	return exists, nil
}

// Get retrieves an agent by account ID and name
func (idx *Index) Get(accountID, name string) (*Agent, error) {
	var agent Agent
	err := idx.db.QueryRow(`
		SELECT account_id, name, registry, visibility, archived_at, name_reserved, avatar_colors, avatar_updated_at, created_at, updated_at
		FROM agents
		WHERE account_id = $1 AND name = $2
	`, accountID, name).Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.ArchivedAt, &agent.NameReserved, &agent.AvatarColors, &agent.AvatarUpdatedAt, &agent.CreatedAt, &agent.UpdatedAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("agent not found: %s", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query agent: %w", err)
	}

	// Load versions ordered newest first
	rows, err := idx.db.Query(`
		SELECT build_id, ecr_namespace, spec_json, readme, agent_card_json, validation_warnings, published_at, updated_at
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
		if err := rows.Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &v.AgentCardJSON, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
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
		SELECT build_id, ecr_namespace, spec_json, readme, agent_card_json, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2 AND build_id = $3
	`, accountID, name, buildID).Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &v.AgentCardJSON, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

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

// ValidateLineage reports whether (accountID, name, buildID) refers to a real
// published agent version. Returns nil when the row exists, otherwise the same
// error GetVersion would produce ("build not found: …" for the missing case,
// "failed to query version: …" for operational DB failures).
//
// This lets *Index implicitly satisfy deploymentstore.LineageValidator without
// the deploymentstore package having to import this one. The error-only
// signature is deliberate: the Store only needs to know whether the tuple
// resolves, not what the version contains, so the method discards the
// loaded *AgentVersion. If validation ever shows up on a hot path, swap the
// body for a dedicated `SELECT 1 ... LIMIT 1` query that skips the spec
// unmarshal — semantics are unchanged.
//
// Note on lifecycle: this check is about row existence, not lifecycle state.
// It does NOT filter on agents.archived_at, so a version published before the
// agent was archived still passes — by design. Existing deployments must
// remain redeployable after their source agent is archived; the Store would
// otherwise reject every redeploy of a legacy deployment as a side effect of
// archive. Tightening this to exclude archived agents would break that path
// and should only be done with an explicit policy decision (and a migration
// for already-deployed rows).
func (idx *Index) ValidateLineage(accountID, name, buildID string) error {
	_, err := idx.GetVersion(accountID, name, buildID)
	return err
}

// List returns all agents in the index (global browse), excluding archived
func (idx *Index) List() ([]*Agent, error) {
	rows, err := idx.db.Query(`
		SELECT account_id, name, registry, visibility, avatar_colors, avatar_updated_at, created_at, updated_at
		FROM agents
		WHERE archived_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query agents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var agents []*Agent
	for rows.Next() {
		var agent Agent
		if err := rows.Scan(&agent.AccountID, &agent.Name, &agent.Registry, &agent.Visibility, &agent.AvatarColors, &agent.AvatarUpdatedAt, &agent.CreatedAt, &agent.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan agent: %w", err)
		}

		// Load versions ordered newest first
		versionRows, err := idx.db.Query(`
			SELECT build_id, ecr_namespace, spec_json, readme, agent_card_json, validation_warnings, published_at, updated_at
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
			if err := versionRows.Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &v.AgentCardJSON, &warningsJSON, &v.PublishedAt, &v.UpdatedAt); err != nil {
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

// AgentNames returns the names of all non-archived agents for an account.
func (idx *Index) AgentNames(accountID string) ([]string, error) {
	rows, err := idx.db.Query(`
		SELECT name FROM agents
		WHERE account_id = $1 AND archived_at IS NULL
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query agent names: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("failed to scan agent name: %w", err)
		}
		names = append(names, name)
	}
	return names, nil
}

// Archive soft-deletes an agent by setting its archived_at timestamp.
// The agent and its versions are preserved but hidden from list queries.
//
// Required migration:
//
//	ALTER TABLE agents ADD COLUMN archived_at TIMESTAMP;
func (idx *Index) Archive(accountID, name string) error {
	result, err := idx.db.Exec(`
		UPDATE agents SET archived_at = $1, updated_at = $1
		WHERE account_id = $2 AND name = $3 AND archived_at IS NULL
	`, time.Now(), accountID, name)
	if err != nil {
		return fmt.Errorf("failed to archive agent: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("agent not found or already archived: %s", name)
	}

	idx.invalidateBlueprintLists(accountID)
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

	idx.invalidateBlueprintLists(accountID)
	return nil
}

// SetVisibility updates the visibility of an agent (public or private)
func (idx *Index) SetVisibility(accountID, name, visibility string) error {
	if visibility != "public" && visibility != "private" {
		return fmt.Errorf("invalid visibility: %s (must be 'public' or 'private')", visibility)
	}

	// Going public permanently reserves the name regardless of future visibility changes.
	result, err := idx.db.Exec(`
		UPDATE agents SET visibility = $1, updated_at = $2,
		    name_reserved = (name_reserved OR $3)
		WHERE account_id = $4 AND name = $5
	`, visibility, time.Now(), visibility == "public", accountID, name)
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

	idx.invalidateBlueprintLists(accountID)
	return nil
}

// MarkNameReserved permanently reserves the agent's name so it cannot be reused after
// archival. Called best-effort after a deployment is created for the agent.
func (idx *Index) MarkNameReserved(accountID, name string) error {
	_, err := idx.db.Exec(`
		UPDATE agents SET name_reserved = true
		WHERE account_id = $1 AND name = $2
	`, accountID, name)
	if err == nil {
		idx.invalidateBlueprintLists(accountID)
	}
	return err
}

// SetAvatarColors stores the extracted avatar color scheme for an agent.
func (idx *Index) SetAvatarColors(accountID, name string, colorsJSON []byte) error {
	_, err := idx.db.Exec(`
		UPDATE agents SET avatar_colors = $1, updated_at = now()
		WHERE account_id = $2 AND name = $3
	`, colorsJSON, accountID, name)
	if err == nil {
		idx.invalidateBlueprintLists(accountID)
	}
	return err
}

// TouchAvatarUpdatedAt stamps avatar_updated_at = now() for an agent, bumping
// the cache-busting token on its avatar URL, and returns the persisted value so
// callers embed the DB clock (not the app clock) in the immediate response.
// Called after every avatar write.
func (idx *Index) TouchAvatarUpdatedAt(accountID, name string) (time.Time, error) {
	var ts time.Time
	err := idx.db.QueryRow(`
		UPDATE agents SET avatar_updated_at = now()
		WHERE account_id = $1 AND name = $2
		RETURNING avatar_updated_at
	`, accountID, name).Scan(&ts)
	if err == nil {
		idx.invalidateBlueprintLists(accountID)
	}
	return ts, err
}

// GetLatestVersion returns the most recently registered build for an agent
func (idx *Index) GetLatestVersion(accountID, name string) (*AgentVersion, error) {
	var v AgentVersion
	var specJSON, warningsJSON string
	err := idx.db.QueryRow(`
		SELECT build_id, ecr_namespace, spec_json, readme, agent_card_json, validation_warnings, published_at, updated_at
		FROM agent_versions
		WHERE account_id = $1 AND name = $2
		ORDER BY published_at DESC
		LIMIT 1
	`, accountID, name).Scan(&v.BuildID, &v.ECRNamespace, &specJSON, &v.Readme, &v.AgentCardJSON, &warningsJSON, &v.PublishedAt, &v.UpdatedAt)

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

// AgentVersionRef identifies a single agent across an account boundary; used
// for batch latest-build lookups.
type AgentVersionRef struct {
	AccountID string
	Name      string
}

// LatestBuildInfo is the list-safe projection used to surface upgrade signals
// without resolving agents one at a time.
type LatestBuildInfo struct {
	BuildID    string
	Visibility string
}

// batchLatestBuildIDsQuery expands parallel account ID and name arrays via
// unnest; corresponding pg arguments are bound as pq.Array in BatchLatestBuildIDs.
const batchLatestBuildIDsQuery = `
WITH wanted AS (
	SELECT q.account_id::uuid AS account_id, q.name
	FROM unnest($1::text[], $2::text[]) AS q(account_id, name)
),
ranked AS (
	SELECT v.account_id, v.name, v.build_id,
		ROW_NUMBER() OVER (PARTITION BY v.account_id, v.name ORDER BY v.published_at DESC) AS rn
	FROM agent_versions v
	INNER JOIN wanted w ON w.account_id = v.account_id AND w.name = v.name
)
SELECT account_id::text, name, build_id FROM ranked WHERE rn = 1
`

const batchLatestBuildInfoQuery = `
WITH wanted AS (
	SELECT q.account_id::uuid AS account_id, q.name
	FROM unnest($1::text[], $2::text[]) AS q(account_id, name)
)
SELECT w.account_id::text, w.name, latest.build_id, a.visibility
FROM wanted w
INNER JOIN agents a ON a.account_id = w.account_id AND a.name = w.name
INNER JOIN LATERAL (
	SELECT v.build_id
	FROM agent_versions v
	WHERE v.account_id = w.account_id AND v.name = w.name
	ORDER BY v.published_at DESC
	LIMIT 1
) latest ON true
`

// BatchLatestBuildIDs returns the latest build_id per (account_id, name) pair,
// keyed by accountID + "/" + name. Refs whose agent has no published versions
// are simply absent from the map (callers should treat absence as "no upgrade
// signal available", not as an error). Single SQL round-trip; safe to call
// with empty input.
//
// Implemented via a window-function CTE so a single index lookup per
// (account_id, name) returns just the newest row, instead of N round trips.
func (idx *Index) BatchLatestBuildIDs(refs []AgentVersionRef) (map[string]string, error) {
	out := make(map[string]string, len(refs))
	if len(refs) == 0 {
		return out, nil
	}

	accountIDs := make([]string, 0, len(refs))
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.AccountID == "" || ref.Name == "" {
			continue
		}
		accountIDs = append(accountIDs, ref.AccountID)
		names = append(names, ref.Name)
	}
	if len(accountIDs) == 0 {
		return out, nil
	}

	rows, err := idx.db.Query(batchLatestBuildIDsQuery, pq.Array(accountIDs), pq.Array(names))
	if err != nil {
		return nil, fmt.Errorf("batch latest builds: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var accountID, name, buildID string
		if err := rows.Scan(&accountID, &name, &buildID); err != nil {
			return nil, fmt.Errorf("scan batch latest build: %w", err)
		}
		out[accountID+"/"+name] = buildID
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
}

// BatchLatestBuildInfo returns latest build IDs and blueprint visibility for
// a set of lineage references in one SQL round trip.
func (idx *Index) BatchLatestBuildInfo(ctx context.Context, refs []AgentVersionRef) (map[string]LatestBuildInfo, error) {
	out := make(map[string]LatestBuildInfo, len(refs))
	accountIDs := make([]string, 0, len(refs))
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.AccountID == "" || ref.Name == "" {
			continue
		}
		accountIDs = append(accountIDs, ref.AccountID)
		names = append(names, ref.Name)
	}
	if len(accountIDs) == 0 {
		return out, nil
	}

	rows, err := idx.db.QueryContext(ctx, batchLatestBuildInfoQuery, pq.Array(accountIDs), pq.Array(names))
	if err != nil {
		return nil, fmt.Errorf("batch latest build info: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	for rows.Next() {
		var accountID, name string
		var info LatestBuildInfo
		if err := rows.Scan(&accountID, &name, &info.BuildID, &info.Visibility); err != nil {
			return nil, fmt.Errorf("scan batch latest build info: %w", err)
		}
		out[accountID+"/"+name] = info
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows: %w", err)
	}
	return out, nil
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

	// Re-key the agent. The agent_versions.(account_id, name) and
	// agent_hearts.(account_id, agent_name) FKs are declared
	// ON UPDATE CASCADE, so child rows move atomically with the
	// parent. Without that cascade this UPDATE would orphan
	// agent_versions for the duration of the statement and trip the
	// immediate FK check, aborting the whole transfer.
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

	// Audit-bump the now-cascaded version rows. The FK cascade above
	// only touches the FK columns (account_id, name), not updated_at,
	// so we record the transfer event explicitly. WHERE filters on the
	// target account because the rows have already moved.
	_, err = tx.Exec(`
		UPDATE agent_versions SET updated_at = $1
		WHERE account_id = $2 AND name = $3
	`, now, targetAccountID, agentName)
	if err != nil {
		return fmt.Errorf("failed to bump version timestamps: %w", err)
	}

	// Repoint cross-account deployments at the new owner. Without this,
	// resolveSourceAccountName falls through to the spec-JSON fallback under
	// the old account name, breaking upgrade signals and leaving lineage
	// stale (the old account may even be deleted/reclaimed). The FK on
	// deployments.source_account_id is single-column to accounts.id, so
	// it doesn't ride the agents cascade — this UPDATE is what moves it.
	_, err = tx.Exec(`
		UPDATE deployments SET source_account_id = $1
		WHERE source_account_id = $2 AND agent_name = $3
	`, targetAccountID, sourceAccountID, agentName)
	if err != nil {
		return fmt.Errorf("failed to transfer deployments source_account_id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	idx.invalidateBlueprintLists(sourceAccountID, targetAccountID)
	return nil
}
