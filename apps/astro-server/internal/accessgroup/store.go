package accessgroup

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
)

var (
	ErrNotFound           = errors.New("access group not found")
	ErrNameExists         = errors.New("access group name already exists")
	errStoreNotConfigured = errors.New("access group store is not configured")
)

const groupColumns = `
	id, account_id, COALESCE(workos_group_id, ''), name, description, status,
	management_source, created_by_user_id, COALESCE(archived_by_user_id, ''),
	archived_at, classification_metadata, sync_status, COALESCE(sync_error, ''),
	created_at, updated_at`

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
	if len(params.Name) > 100 || len(params.Description) > 500 {
		return nil, errors.New("group name must be at most 100 characters and description at most 500 characters")
	}
	metadata := params.ClassificationMetadata
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{"schema_version":1}`)
	}
	if err := validateClassificationMetadata(metadata); err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("create access group: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	row := tx.QueryRowContext(ctx, `
		INSERT INTO access_groups (
			account_id, name, description, created_by_user_id,
			classification_metadata, sync_status
		)
		VALUES ($1, $2, $3, $4, $5, 'pending')
		RETURNING `+groupColumns,
		params.AccountID, params.Name, params.Description, params.CreatedByUserID, []byte(metadata),
	)
	group, err := scanGroup(row)
	if err != nil {
		return nil, classifyWriteError("create access group", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO access_group_memberships (
			group_id, account_id, user_id, role, added_by_user_id, sync_status
		)
		VALUES ($1, $2, $3, 'admin', $3, 'pending')
	`, group.ID, group.AccountID, params.CreatedByUserID); err != nil {
		return nil, fmt.Errorf("create access group admin membership: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("create access group: commit transaction: %w", err)
	}
	return group, nil
}

func (s *Store) Get(ctx context.Context, accountID, groupID string) (*Group, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	group, err := scanGroup(s.db.QueryRowContext(ctx, `
		SELECT `+groupColumns+`
		FROM access_groups
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

func (s *Store) List(ctx context.Context, accountID string, filter ListFilter) ([]Summary, error) {
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
		SELECT `+groupColumns+`,
		       (SELECT COUNT(*)
		        FROM access_group_memberships memberships
		        WHERE memberships.group_id = access_groups.id
		          AND memberships.removed_at IS NULL) AS member_count,
		       ARRAY(
		        SELECT memberships.user_id
		        FROM access_group_memberships memberships
		        WHERE memberships.group_id = access_groups.id
		          AND memberships.removed_at IS NULL
		        ORDER BY memberships.added_at, memberships.user_id
		        LIMIT 3
		       ) AS preview_user_ids
		FROM access_groups
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

	groups := make([]Summary, 0)
	for rows.Next() {
		var summary Summary
		var metadata []byte
		if err := rows.Scan(
			&summary.ID, &summary.AccountID, &summary.WorkOSGroupID, &summary.Name, &summary.Description,
			&summary.Status, &summary.ManagementSource, &summary.CreatedByUserID, &summary.ArchivedByUserID,
			&summary.ArchivedAt, &metadata, &summary.SyncStatus, &summary.SyncError,
			&summary.CreatedAt, &summary.UpdatedAt, &summary.MemberCount, pq.Array(&summary.PreviewUserIDs),
		); err != nil {
			return nil, fmt.Errorf("scan access group summary: %w", err)
		}
		summary.ClassificationMetadata = append(json.RawMessage(nil), metadata...)
		groups = append(groups, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access groups: %w", err)
	}
	return groups, nil
}

func (s *Store) Update(ctx context.Context, accountID, groupID, name, description string, metadata json.RawMessage) (*Group, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	description = strings.TrimSpace(description)
	if name == "" || len(name) > 100 || len(description) > 500 {
		return nil, errors.New("group name must be 1-100 characters and description at most 500 characters")
	}
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{"schema_version":1}`)
	}
	if err := validateClassificationMetadata(metadata); err != nil {
		return nil, err
	}
	group, err := scanGroup(s.db.QueryRowContext(ctx, `
		UPDATE access_groups
		SET name = $3, description = $4, classification_metadata = $5,
		    sync_status = 'pending', sync_error = NULL, updated_at = now()
		WHERE account_id = $1 AND id = $2 AND status <> 'archived'
		RETURNING `+groupColumns,
		accountID, groupID, name, description, []byte(metadata),
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, classifyWriteError("update access group", err)
	}
	return group, nil
}

func (s *Store) SetProjection(ctx context.Context, accountID, groupID, workOSGroupID string, status SyncStatus, syncError string) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE access_groups
		SET workos_group_id = NULLIF($3, ''), sync_status = $4,
		    sync_error = NULLIF($5, ''), updated_at = now()
		WHERE account_id = $1 AND id = $2
	`, accountID, groupID, workOSGroupID, status, syncError)
	if err != nil {
		return fmt.Errorf("set access group projection: %w", err)
	}
	return requireChanged(result, "set access group projection")
}

func (s *Store) SetStatus(ctx context.Context, accountID, groupID, actorUserID string, status Status) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	if status != StatusActive && status != StatusArchiving && status != StatusArchived && status != StatusRestoring {
		return errors.New("invalid access group status")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE access_groups
		SET status = $3,
		    archived_at = CASE WHEN $3 = 'archived' THEN now() WHEN $3 IN ('active', 'restoring') THEN NULL ELSE archived_at END,
		    archived_by_user_id = CASE WHEN $3 IN ('archiving', 'archived') THEN NULLIF($4, '') WHEN $3 IN ('active', 'restoring') THEN NULL ELSE archived_by_user_id END,
		    sync_status = CASE WHEN $3 IN ('archiving', 'restoring') THEN 'pending' ELSE sync_status END,
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
	result, err := s.db.ExecContext(ctx, `DELETE FROM access_groups WHERE account_id = $1 AND id = $2`, accountID, groupID)
	if err != nil {
		return fmt.Errorf("delete access group: %w", err)
	}
	return requireChanged(result, "delete access group")
}

func (s *Store) UpsertMembership(ctx context.Context, membership Membership) (*Membership, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	if membership.GroupID == "" || membership.AccountID == "" || membership.UserID == "" || membership.AddedByUserID == "" {
		return nil, errors.New("group id, account id, user id, and actor user id are required")
	}
	if membership.Role == "" {
		membership.Role = MembershipRoleMember
	}
	return scanMembership(s.db.QueryRowContext(ctx, `
		INSERT INTO access_group_memberships (
			group_id, account_id, user_id, role, added_by_user_id,
			sync_status, sync_error, updated_at
		)
		VALUES ($1, $2, $3, $4, $5, 'pending', NULL, now())
		ON CONFLICT (group_id, user_id) DO UPDATE
		SET role = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN EXCLUDED.role ELSE access_group_memberships.role END,
		    added_by_user_id = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN EXCLUDED.added_by_user_id ELSE access_group_memberships.added_by_user_id END,
		    added_at = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN now() ELSE access_group_memberships.added_at END,
		    removed_by_user_id = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN NULL ELSE access_group_memberships.removed_by_user_id END,
		    removed_at = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN NULL ELSE access_group_memberships.removed_at END,
		    sync_status = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN 'pending' ELSE access_group_memberships.sync_status END,
		    sync_error = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN NULL ELSE access_group_memberships.sync_error END,
		    updated_at = CASE WHEN access_group_memberships.removed_at IS NOT NULL THEN now() ELSE access_group_memberships.updated_at END
		RETURNING group_id, account_id, user_id, role, added_by_user_id,
		          COALESCE(removed_by_user_id, ''), added_at, removed_at,
		          sync_status, COALESCE(sync_error, ''), updated_at
	`, membership.GroupID, membership.AccountID, membership.UserID, membership.Role, membership.AddedByUserID))
}

func (s *Store) SetMembershipRole(ctx context.Context, accountID, groupID, userID string, role MembershipRole) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	if role != MembershipRoleMember && role != MembershipRoleAdmin {
		return errors.New("membership role must be member or admin")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE access_group_memberships
		SET role = $4, updated_at = now()
		WHERE account_id = $1 AND group_id = $2 AND user_id = $3 AND removed_at IS NULL
	`, accountID, groupID, userID, role)
	if err != nil {
		return fmt.Errorf("set access group membership role: %w", err)
	}
	return requireChanged(result, "set access group membership role")
}

func (s *Store) RemoveMembership(ctx context.Context, accountID, groupID, userID, actorUserID string) error {
	if err := s.ensureConfigured(); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE access_group_memberships
		SET removed_by_user_id = NULLIF($4, ''), removed_at = now(),
		    sync_status = 'pending', sync_error = NULL, updated_at = now()
		WHERE account_id = $1 AND group_id = $2 AND user_id = $3 AND removed_at IS NULL
	`, accountID, groupID, userID, actorUserID)
	if err != nil {
		return fmt.Errorf("remove access group membership: %w", err)
	}
	return requireChanged(result, "remove access group membership")
}

func (s *Store) ListMemberships(ctx context.Context, accountID, groupID string, includeRemoved bool) ([]Membership, error) {
	if err := s.ensureConfigured(); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT group_id, account_id, user_id, role, added_by_user_id,
		       COALESCE(removed_by_user_id, ''), added_at, removed_at,
		       sync_status, COALESCE(sync_error, ''), updated_at
		FROM access_group_memberships
		WHERE account_id = $1 AND group_id = $2 AND ($3 OR removed_at IS NULL)
		ORDER BY (role = 'admin') DESC, added_at, user_id
	`, accountID, groupID, includeRemoved)
	if err != nil {
		return nil, fmt.Errorf("list access group memberships: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	memberships := make([]Membership, 0)
	for rows.Next() {
		membership, err := scanMembership(rows)
		if err != nil {
			return nil, fmt.Errorf("scan access group membership: %w", err)
		}
		memberships = append(memberships, *membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate access group memberships: %w", err)
	}
	return memberships, nil
}

func (s *Store) ActiveAdminCount(ctx context.Context, accountID, groupID string) (int, error) {
	if err := s.ensureConfigured(); err != nil {
		return 0, err
	}
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM access_group_memberships
		WHERE account_id = $1 AND group_id = $2 AND role = 'admin' AND removed_at IS NULL
	`, accountID, groupID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count access group admins: %w", err)
	}
	return count, nil
}

func scanGroup(row interface{ Scan(...any) error }) (*Group, error) {
	group := &Group{}
	if err := scanGroupFields(row, group); err != nil {
		return nil, err
	}
	return group, nil
}

func scanGroupFields(row interface{ Scan(...any) error }, group *Group) error {
	var metadata []byte
	err := row.Scan(
		&group.ID, &group.AccountID, &group.WorkOSGroupID, &group.Name, &group.Description,
		&group.Status, &group.ManagementSource, &group.CreatedByUserID, &group.ArchivedByUserID,
		&group.ArchivedAt, &metadata, &group.SyncStatus, &group.SyncError,
		&group.CreatedAt, &group.UpdatedAt,
	)
	if err == nil {
		group.ClassificationMetadata = append(json.RawMessage(nil), metadata...)
	}
	return err
}

func scanMembership(row interface{ Scan(...any) error }) (*Membership, error) {
	membership := &Membership{}
	err := row.Scan(
		&membership.GroupID, &membership.AccountID, &membership.UserID, &membership.Role,
		&membership.AddedByUserID, &membership.RemovedByUserID, &membership.AddedAt,
		&membership.RemovedAt, &membership.SyncStatus, &membership.SyncError, &membership.UpdatedAt,
	)
	return membership, err
}

func classifyWriteError(operation string, err error) error {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return fmt.Errorf("%s: %w", operation, ErrNameExists)
	}
	return fmt.Errorf("%s: %w", operation, err)
}

func validateClassificationMetadata(metadata json.RawMessage) error {
	if !json.Valid(metadata) {
		return errors.New("classification metadata must be valid JSON")
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(metadata, &object); err != nil || object == nil {
		return errors.New("classification metadata must be a JSON object")
	}
	return nil
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
