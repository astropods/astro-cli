package auditlog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Store manages audit log persistence in PostgreSQL.
type Store struct {
	db *sql.DB
}

// NewStore creates a new audit log store.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Log synchronously inserts an audit log entry.
func (s *Store) Log(ctx context.Context, e Event) error {
	var metadataParam any
	if e.Metadata != nil {
		b, err := json.Marshal(e.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal audit metadata: %w", err)
		}
		metadataParam = b
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_logs (account_id, actor_id, actor_type, action, resource_type, resource_id, resource_name, description, metadata, ip_address, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`,
		e.AccountID,
		e.ActorID,
		string(e.ActorType),
		e.Action,
		e.ResourceType,
		e.ResourceID,
		nullIfEmpty(e.ResourceName),
		nullIfEmpty(e.Description),
		metadataParam,
		nullIfEmpty(e.IPAddress),
		nullIfEmpty(e.UserAgent),
	)
	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}
	return nil
}

// LogAsync inserts an audit log entry in a background goroutine.
// Errors are logged but do not block the caller. Safe to call on a nil Store.
func (s *Store) LogAsync(log *logger.Logger, e Event) {
	if s == nil {
		return
	}
	go func() {
		if err := s.Log(context.Background(), e); err != nil {
			log.Error("Failed to write audit log", "error", err, "action", e.Action, "resource", e.ResourceType+"/"+e.ResourceID)
		}
	}()
}

// Query returns audit log entries matching the given params, newest first.
// Uses cursor-based pagination via QueryParams.Before.
func (s *Store) Query(ctx context.Context, p QueryParams) ([]Entry, error) {
	if p.Limit <= 0 {
		p.Limit = 50
	}
	if p.Limit > 200 {
		p.Limit = 200
	}

	var conditions []string
	var args []any
	argIdx := 1

	conditions = append(conditions, fmt.Sprintf("account_id = $%d", argIdx))
	args = append(args, p.AccountID)
	argIdx++

	if p.ActorID != "" {
		conditions = append(conditions, fmt.Sprintf("actor_id = $%d", argIdx))
		args = append(args, p.ActorID)
		argIdx++
	}
	if p.ResourceType != "" {
		conditions = append(conditions, fmt.Sprintf("resource_type = $%d", argIdx))
		args = append(args, p.ResourceType)
		argIdx++
	}
	if p.ResourceID != "" {
		conditions = append(conditions, fmt.Sprintf("resource_id = $%d", argIdx))
		args = append(args, p.ResourceID)
		argIdx++
	}
	if p.Action != "" {
		conditions = append(conditions, fmt.Sprintf("action = $%d", argIdx))
		args = append(args, p.Action)
		argIdx++
	}
	if p.Before != nil {
		if p.BeforeID > 0 {
			// Composite cursor: break ties on entries with the same timestamp
			conditions = append(conditions, fmt.Sprintf("(created_at, id) < ($%d, $%d)", argIdx, argIdx+1))
			args = append(args, *p.Before, p.BeforeID)
			argIdx += 2
		} else {
			conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
			args = append(args, *p.Before)
			argIdx++
		}
	}

	// Fetch one extra to determine has_more
	var qb strings.Builder
	qb.WriteString(`SELECT id, account_id, actor_id, actor_type, action, resource_type, resource_id,
		       resource_name, description, metadata, ip_address, user_agent, created_at
		FROM audit_logs WHERE `)
	qb.WriteString(strings.Join(conditions, " AND "))
	qb.WriteString(" ORDER BY created_at DESC, id DESC LIMIT $")
	qb.WriteString(strconv.Itoa(argIdx))
	query := qb.String()
	args = append(args, p.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var entries []Entry
	for rows.Next() {
		var e Entry
		var resourceName, description, ipAddress, userAgent sql.NullString
		var metadata []byte
		if err := rows.Scan(
			&e.ID, &e.AccountID, &e.ActorID, &e.ActorType, &e.Action,
			&e.ResourceType, &e.ResourceID,
			&resourceName, &description, &metadata, &ipAddress, &userAgent,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan audit log entry: %w", err)
		}
		e.ResourceName = resourceName.String
		e.Description = description.String
		e.IPAddress = ipAddress.String
		e.UserAgent = userAgent.String
		if metadata != nil {
			e.Metadata = metadata
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating audit log rows: %w", err)
	}
	return entries, nil
}

// FilterOptions holds the distinct resource types and actions for an account.
type FilterOptions struct {
	ResourceTypes []string `json:"resource_types"`
	Actions       []string `json:"actions"`
}

// Filters returns the distinct resource types and actions recorded for an account.
func (s *Store) Filters(ctx context.Context, accountID string) (*FilterOptions, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT kind, val FROM (
			SELECT 'rt' AS kind, resource_type AS val FROM audit_logs WHERE account_id = $1
			UNION
			SELECT 'act', action FROM audit_logs WHERE account_id = $1
		) t ORDER BY kind, val
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit log filters: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var resourceTypes, actions []string
	for rows.Next() {
		var kind, val string
		if err := rows.Scan(&kind, &val); err != nil {
			return nil, fmt.Errorf("failed to scan audit log filter row: %w", err)
		}
		switch kind {
		case "rt":
			resourceTypes = append(resourceTypes, val)
		case "act":
			actions = append(actions, val)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &FilterOptions{
		ResourceTypes: resourceTypes,
		Actions:       actions,
	}, nil
}

// ResourceLatest holds the most recent audit log info for a resource.
type ResourceLatest struct {
	UpdatedAt time.Time
	ActorID   string
}

// LatestPerResource returns the most recent audit log entry (timestamp + actor)
// for each resource_id in the given set, scoped to an account and resource_type.
// Missing entries have no audit history.
func (s *Store) LatestPerResource(ctx context.Context, accountID, resourceType string, resourceIDs []string) (map[string]ResourceLatest, error) {
	if len(resourceIDs) == 0 {
		return nil, nil
	}

	// Build ($1, $2, ...) placeholders — $1=account_id, $2=resource_type, $3+=resource IDs
	placeholders := make([]string, len(resourceIDs))
	args := make([]any, 0, len(resourceIDs)+2)
	args = append(args, accountID, resourceType)
	for i, id := range resourceIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args = append(args, id)
	}

	var qb strings.Builder
	qb.WriteString(`SELECT DISTINCT ON (resource_id) resource_id, created_at, actor_id
		FROM audit_logs
		WHERE account_id = $1 AND resource_type = $2 AND resource_id IN (`)
	qb.WriteString(strings.Join(placeholders, ", "))
	qb.WriteString(`) ORDER BY resource_id, created_at DESC`)
	query := qb.String()

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest audit timestamps: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]ResourceLatest, len(resourceIDs))
	for rows.Next() {
		var resourceID, actorID string
		var ts time.Time
		if err := rows.Scan(&resourceID, &ts, &actorID); err != nil {
			return nil, fmt.Errorf("failed to scan latest audit entry: %w", err)
		}
		result[resourceID] = ResourceLatest{UpdatedAt: ts, ActorID: actorID}
	}
	return result, rows.Err()
}

// LatestPerResourceByAction returns the most recent audit log entry for each
// resource_id in the given set, restricted to a single action.
func (s *Store) LatestPerResourceByAction(
	ctx context.Context,
	accountID, action, resourceType string,
	resourceIDs []string,
) (map[string]ResourceLatest, error) {
	if len(resourceIDs) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(resourceIDs))
	args := make([]any, 0, len(resourceIDs)+3)
	args = append(args, accountID, action, resourceType)
	for i, id := range resourceIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+4)
		args = append(args, id)
	}

	var qb strings.Builder
	qb.WriteString(`SELECT DISTINCT ON (resource_id) resource_id, created_at, actor_id
		FROM audit_logs
		WHERE account_id = $1 AND action = $2 AND resource_type = $3 AND resource_id IN (`)
	qb.WriteString(strings.Join(placeholders, ", "))
	qb.WriteString(`) ORDER BY resource_id, created_at DESC, id DESC`)

	rows, err := s.db.QueryContext(ctx, qb.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest audit timestamps by action: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]ResourceLatest, len(resourceIDs))
	for rows.Next() {
		var resourceID, actorID string
		var ts time.Time
		if err := rows.Scan(&resourceID, &ts, &actorID); err != nil {
			return nil, fmt.Errorf("failed to scan latest audit entry by action: %w", err)
		}
		result[resourceID] = ResourceLatest{UpdatedAt: ts, ActorID: actorID}
	}
	return result, rows.Err()
}

// BulkDistinctActorsFor returns distinct actor IDs per resource. When resourceIDs is nil, all resources for the account are included.
func (s *Store) BulkDistinctActorsFor(ctx context.Context, accountID, action, resourceType string, resourceIDs []string) (map[string][]string, error) {
	args := []any{accountID, action, resourceType}
	query := `SELECT resource_id, actor_id FROM audit_logs
		 WHERE account_id = $1 AND action = $2 AND resource_type = $3`

	if len(resourceIDs) > 0 {
		parts := make([]string, len(resourceIDs))
		for i, id := range resourceIDs {
			parts[i] = fmt.Sprintf("$%d", i+4)
			args = append(args, id)
		}
		query += " AND resource_id IN (" + strings.Join(parts, ", ") + ")" //nolint:gosec
	}

	query += " GROUP BY resource_id, actor_id ORDER BY MIN(created_at) ASC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query bulk distinct actors: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string][]string)
	for rows.Next() {
		var resourceID, actorID string
		if err := rows.Scan(&resourceID, &actorID); err != nil {
			return nil, fmt.Errorf("failed to scan bulk distinct actors: %w", err)
		}
		result[resourceID] = append(result[resourceID], actorID)
	}
	return result, rows.Err()
}

// DistinctActorsFor returns the unique actor IDs that performed the given action
// on a specific resource, ordered by their first occurrence ascending.
func (s *Store) DistinctActorsFor(ctx context.Context, accountID, action, resourceType, resourceID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT actor_id FROM audit_logs
		 WHERE account_id = $1 AND action = $2 AND resource_type = $3 AND resource_id = $4
		 GROUP BY actor_id
		 ORDER BY MIN(created_at) ASC`,
		accountID, action, resourceType, resourceID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck
	var actors []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		actors = append(actors, id)
	}
	return actors, rows.Err()
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// ParseLimit parses a limit string with a default and max.
func ParseLimit(s string, defaultLimit, maxLimit int) int {
	if s == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}

// ParseBefore parses an RFC3339 timestamp for cursor pagination (backward compat).
func ParseBefore(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	return &t
}

// ParseCursor parses a composite cursor "timestamp,id" for pagination.
// Falls back to timestamp-only if the cursor has no ID component.
func ParseCursor(s string) (*time.Time, int64) {
	if s == "" {
		return nil, 0
	}
	parts := strings.SplitN(s, ",", 2)
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, 0
	}
	var id int64
	if len(parts) == 2 {
		id, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	return &t, id
}

// FormatCursor formats a composite cursor from an Entry for next-page links.
func FormatCursor(e Entry) string {
	return e.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00") + "," + strconv.FormatInt(e.ID, 10)
}
