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
		conditions = append(conditions, fmt.Sprintf("created_at < $%d", argIdx))
		args = append(args, *p.Before)
		argIdx++
	}

	// Fetch one extra to determine has_more
	query := fmt.Sprintf(`
		SELECT id, account_id, actor_id, actor_type, action, resource_type, resource_id,
		       resource_name, description, metadata, ip_address, user_agent, created_at
		FROM audit_logs
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d
	`, strings.Join(conditions, " AND "), argIdx)
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

// ParseBefore parses an RFC3339 timestamp for cursor pagination.
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
