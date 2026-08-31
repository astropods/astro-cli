package accessgroup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/lib/pq"
)

var (
	ErrNotFound           = errors.New("access group not found")
	ErrNameExists         = errors.New("access group name already exists")
	ErrProjectionConflict = errors.New("access group WorkOS projection already exists")
	errStoreNotConfigured = errors.New("access group store is not configured")
)

const (
	groupColumns = `
	id, account_id, COALESCE(workos_group_id, ''), name, description, status,
	created_by_user_id, COALESCE(archived_by_user_id, ''), archived_at,
	created_at, updated_at`
)

type Store struct {
	db *sql.DB
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Create(ctx context.Context, params CreateParams) (*Group, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	params.Name = strings.TrimSpace(params.Name)
	params.Description = strings.TrimSpace(params.Description)
	if params.AccountID == "" || params.CreatedByUserID == "" || params.Name == "" {
		return nil, errors.New("account id, creator user id, and group name are required")
	}
	if utf8.RuneCountInString(params.Name) > 100 || utf8.RuneCountInString(params.Description) > 500 {
		return nil, errors.New("group name must be at most 100 characters and description at most 500 characters")
	}
	group, err := scanGroup(s.db.QueryRowContext(ctx, `
		INSERT INTO groups (
			account_id, name, description, created_by_user_id
		)
		VALUES ($1, $2, $3, $4)
		RETURNING `+groupColumns,
		params.AccountID, params.Name, params.Description, params.CreatedByUserID,
	))
	if err != nil {
		return nil, classifyWriteError("create access group", err)
	}
	return group, nil
}

func (s *Store) Get(ctx context.Context, accountID, groupID string) (*Group, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	group, err := scanGroup(s.db.QueryRowContext(ctx, `
		SELECT `+groupColumns+`
		FROM groups
		WHERE account_id = $1 AND id = $2
	`, accountID, groupID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get access group: %w", err)
	}
	return group, nil
}

func (s *Store) List(ctx context.Context, accountID string, filter ListFilter) ([]Group, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if accountID == "" {
		return nil, errors.New("account id is required")
	}
	if filter.Limit < 1 || filter.Limit > 100 {
		filter.Limit = 50
	}
	statuses := make([]string, 0, len(filter.Statuses))
	for _, status := range filter.Statuses {
		statuses = append(statuses, string(status))
	}
	if len(statuses) == 0 {
		statuses = []string{string(StatusActive), string(StatusArchiving), string(StatusRestoring)}
	}
	search := escapeLikeSearch(strings.TrimSpace(filter.Search))
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+groupColumns+`
		FROM groups
		WHERE account_id = $1
		  AND status = ANY($2::text[])
		  AND ($3 = '' OR name ILIKE '%' || $3 || '%' ESCAPE '\' OR description ILIKE '%' || $3 || '%' ESCAPE '\')
		ORDER BY created_at DESC, id DESC
		LIMIT $4 OFFSET $5
	`, accountID, pq.Array(statuses), search, filter.Limit, filter.Offset)
	if err != nil {
		return nil, fmt.Errorf("list access groups: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	groups := make([]Group, 0)
	for rows.Next() {
		var group Group
		if err := rows.Scan(
			&group.ID, &group.AccountID, &group.WorkOSGroupID, &group.Name, &group.Description,
			&group.Status, &group.CreatedByUserID, &group.ArchivedByUserID,
			&group.ArchivedAt, &group.CreatedAt, &group.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan access group summary: %w", err)
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access groups: %w", err)
	}
	return groups, nil
}

func (s *Store) Update(ctx context.Context, accountID, groupID, name, description string) (*Group, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || utf8.RuneCountInString(name) > 100 || utf8.RuneCountInString(description) > 500 {
		return nil, errors.New("group name must be 1-100 characters and description at most 500 characters")
	}
	group, err := scanGroup(s.db.QueryRowContext(ctx, `
		UPDATE groups
		SET name = $3, description = $4, updated_at = now()
		WHERE account_id = $1 AND id = $2 AND status <> 'archived'
		RETURNING `+groupColumns,
		accountID, groupID, name, description,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, classifyWriteError("update access group", err)
	}
	return group, nil
}

func (s *Store) SetWorkOSGroupID(ctx context.Context, accountID, groupID, workOSGroupID string) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE groups
		SET workos_group_id = NULLIF($3, ''), updated_at = now()
		WHERE account_id = $1 AND id = $2
	`, accountID, groupID, workOSGroupID)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "23505" {
			return fmt.Errorf("set access group WorkOS id: %w", ErrProjectionConflict)
		}
		return fmt.Errorf("set access group WorkOS id: %w", err)
	}
	return requireChanged(result, "set access group WorkOS id")
}

func (s *Store) SetStatus(ctx context.Context, accountID, groupID, actorUserID string, status Status) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	if status != StatusActive && status != StatusArchiving && status != StatusArchived && status != StatusRestoring {
		return errors.New("invalid access group status")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE groups
		SET status = $3,
		    archived_at = CASE WHEN $3 = 'archived' THEN now() WHEN $3 IN ('active', 'restoring') THEN NULL ELSE archived_at END,
		    archived_by_user_id = CASE WHEN $3 IN ('archiving', 'archived') THEN NULLIF($4, '') WHEN $3 IN ('active', 'restoring') THEN NULL ELSE archived_by_user_id END,
		    updated_at = now()
		WHERE account_id = $1 AND id = $2
	`, accountID, groupID, status, actorUserID)
	if err != nil {
		return classifyWriteError("set access group status", err)
	}
	return requireChanged(result, "set access group status")
}

func (s *Store) Delete(ctx context.Context, accountID, groupID string) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE account_id = $1 AND id = $2`, accountID, groupID)
	if err != nil {
		return fmt.Errorf("delete access group: %w", err)
	}
	return requireChanged(result, "delete access group")
}

func scanGroup(row interface{ Scan(...any) error }) (*Group, error) {
	group := &Group{}
	if err := scanGroupFields(row, group); err != nil {
		return nil, err
	}
	return group, nil
}

func scanGroupFields(row interface{ Scan(...any) error }, group *Group) error {
	err := row.Scan(
		&group.ID, &group.AccountID, &group.WorkOSGroupID, &group.Name, &group.Description,
		&group.Status, &group.CreatedByUserID, &group.ArchivedByUserID,
		&group.ArchivedAt,
		&group.CreatedAt, &group.UpdatedAt,
	)
	return err
}

func classifyWriteError(operation string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return fmt.Errorf("%s: %w", operation, ErrNameExists)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func escapeLikeSearch(search string) string {
	return strings.NewReplacer(
		`\`, `\\`,
		`%`, `\%`,
		`_`, `\_`,
		`*`, `\*`,
	).Replace(search)
}

func (s *Store) ensureConfigured() error {
	if s == nil || s.db == nil {
		return errStoreNotConfigured
	}
	return nil
}

func requireChanged(result sql.Result, operation string) error {
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s: inspect rows: %w", operation, err)
	}
	if count == 0 {
		return ErrNotFound
	}
	return nil
}
