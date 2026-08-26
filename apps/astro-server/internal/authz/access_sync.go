package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

type AccessSyncStatus string

const (
	AccessSyncPending  AccessSyncStatus = "pending"
	AccessSyncRetrying AccessSyncStatus = "retrying"
	AccessSyncSynced   AccessSyncStatus = "synced"
)

// AccessIntentKey identifies one subject's direct built-in role on a resource.
type AccessIntentKey struct {
	OrganizationID string
	Resource       ResourceRef
	Subject        AssignmentSubject
}

// AccessIntent records the latest role change Astro asked WorkOS to apply.
// An empty DesiredRole means direct built-in access should be removed.
type AccessIntent struct {
	AccountID      string
	OrganizationID string
	Resource       ResourceRef
	Subject        AssignmentSubject
	SubjectID      string
	DesiredRole    RoleSlug
	DesiredVersion int64
	SyncedRole     RoleSlug
	SyncedVersion  int64
	AttemptCount   int
	LastError      string
	NextAttemptAt  time.Time
	SyncedAt       time.Time
	UpdatedAt      time.Time
}

func (i AccessIntent) Key() AccessIntentKey {
	return AccessIntentKey{OrganizationID: i.OrganizationID, Resource: i.Resource, Subject: i.Subject}
}

func (i AccessIntent) Status() AccessSyncStatus {
	if i.DesiredVersion > 0 && i.SyncedVersion == i.DesiredVersion {
		return AccessSyncSynced
	}
	if i.LastError != "" {
		return AccessSyncRetrying
	}
	return AccessSyncPending
}

type accessIntentStore interface {
	Record(context.Context, AccessIntent) (AccessIntent, bool, error)
	ListForResource(context.Context, string, ResourceRef) ([]AccessIntent, error)
}

// ResourceAccessSyncStore is a durable operation ledger; WorkOS remains the
// source of truth for effective authorization.
type ResourceAccessSyncStore struct {
	db *sql.DB
}

func NewResourceAccessSyncStore(db *sql.DB) *ResourceAccessSyncStore {
	return &ResourceAccessSyncStore{db: db}
}

func (s *ResourceAccessSyncStore) Record(ctx context.Context, intent AccessIntent) (AccessIntent, bool, error) {
	if s == nil || s.db == nil {
		return AccessIntent{}, false, errors.New("resource access sync store is not configured")
	}
	if err := validateAccessIntent(intent); err != nil {
		return AccessIntent{}, false, err
	}
	const query = `
		WITH changed AS (
			INSERT INTO resource_access_fga_sync (
				account_id, organization_id, resource_type, resource_id,
				subject_type, subject_id, workos_subject_id, desired_role,
				desired_version, next_attempt_at, updated_at
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, NULLIF($8, ''), 1, NOW(), NOW())
			ON CONFLICT (organization_id, resource_type, resource_id, subject_type, workos_subject_id)
			DO UPDATE SET
				account_id = EXCLUDED.account_id,
				subject_id = EXCLUDED.subject_id,
				desired_role = EXCLUDED.desired_role,
				desired_version = resource_access_fga_sync.desired_version + 1,
				attempt_count = 0,
				last_error = NULL,
				next_attempt_at = NOW(),
				updated_at = NOW()
			WHERE resource_access_fga_sync.desired_role IS DISTINCT FROM EXCLUDED.desired_role
			RETURNING *, TRUE AS changed
		)
		SELECT account_id, organization_id, resource_type, resource_id,
		       subject_type, subject_id, workos_subject_id, desired_role,
		       desired_version, synced_role, synced_version, attempt_count,
		       last_error, next_attempt_at, synced_at, updated_at, changed
		FROM changed
		UNION ALL
		SELECT account_id, organization_id, resource_type, resource_id,
		       subject_type, subject_id, workos_subject_id, desired_role,
		       desired_version, synced_role, synced_version, attempt_count,
		       last_error, next_attempt_at, synced_at, updated_at, FALSE
		FROM resource_access_fga_sync
		WHERE organization_id = $2 AND resource_type = $3 AND resource_id = $4
		  AND subject_type = $5 AND workos_subject_id = $7
		  AND NOT EXISTS (SELECT 1 FROM changed)
		LIMIT 1`
	args := []any{
		intent.AccountID, intent.OrganizationID, intent.Resource.Type, intent.Resource.ExternalID,
		intent.Subject.Type, intent.SubjectID, intent.Subject.ID, intent.DesiredRole,
	}
	for range 2 {
		var changed bool
		recorded, err := scanAccessIntent(s.db.QueryRowContext(ctx, query, args...), &changed)
		if err == nil {
			return recorded, changed, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return AccessIntent{}, false, fmt.Errorf("record resource access intent: %w", err)
		}
	}
	return AccessIntent{}, false, errors.New("record resource access intent returned no row")
}

func (s *ResourceAccessSyncStore) ListForResource(ctx context.Context, accountID string, resource ResourceRef) ([]AccessIntent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("resource access sync store is not configured")
	}
	if accountID == "" {
		return nil, errors.New("account id is required")
	}
	if err := validateResource(resource); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT account_id, organization_id, resource_type, resource_id,
		       subject_type, subject_id, workos_subject_id, desired_role,
		       desired_version, synced_role, synced_version, attempt_count,
		       last_error, next_attempt_at, synced_at, updated_at
		FROM resource_access_fga_sync
		WHERE account_id = $1 AND resource_type = $2 AND resource_id = $3
		ORDER BY updated_at DESC, subject_type, subject_id
	`, accountID, resource.Type, resource.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("list resource access intents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var intents []AccessIntent
	for rows.Next() {
		intent, err := scanAccessIntent(rows, nil)
		if err != nil {
			return nil, fmt.Errorf("scan resource access intent: %w", err)
		}
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate resource access intents: %w", err)
	}
	return intents, nil
}

// PendingForResource returns current unsynchronized intents for one resource.
func (s *ResourceAccessSyncStore) PendingForResource(ctx context.Context, organizationID string, resource ResourceRef) ([]AccessIntent, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("resource access sync store is not configured")
	}
	if organizationID == "" {
		return nil, errors.New("organization id is required")
	}
	if err := validateResource(resource); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT account_id, organization_id, resource_type, resource_id,
		       subject_type, subject_id, workos_subject_id, desired_role,
		       desired_version, synced_role, synced_version, attempt_count,
		       last_error, next_attempt_at, synced_at, updated_at
		FROM resource_access_fga_sync
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND synced_version IS DISTINCT FROM desired_version
		ORDER BY subject_type, workos_subject_id
	`, organizationID, resource.Type, resource.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("list pending resource access intents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var intents []AccessIntent
	for rows.Next() {
		intent, err := scanAccessIntent(rows, nil)
		if err != nil {
			return nil, fmt.Errorf("scan pending resource access intent: %w", err)
		}
		intents = append(intents, intent)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending resource access intents: %w", err)
	}
	return intents, nil
}

func (s *ResourceAccessSyncStore) Due(ctx context.Context, limit int) ([]AccessIntentKey, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("resource access sync store is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT organization_id, resource_type, resource_id, subject_type, workos_subject_id
		FROM resource_access_fga_sync
		WHERE synced_version IS DISTINCT FROM desired_version
		  AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at, updated_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due resource access intents: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var keys []AccessIntentKey
	for rows.Next() {
		var organizationID, resourceType, resourceID, subjectType, subjectID string
		if err := rows.Scan(&organizationID, &resourceType, &resourceID, &subjectType, &subjectID); err != nil {
			return nil, fmt.Errorf("scan due resource access intent: %w", err)
		}
		keys = append(keys, AccessIntentKey{
			OrganizationID: organizationID,
			Resource:       ResourceRef{Type: ResourceType(resourceType), ExternalID: resourceID},
			Subject:        AssignmentSubject{Type: AssignmentSubjectType(subjectType), ID: subjectID},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due resource access intents: %w", err)
	}
	return keys, nil
}

func (s *ResourceAccessSyncStore) ResourceDeleted(ctx context.Context, resource ResourceRef) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("resource access sync store is not configured")
	}
	if resource.Type != ResourceDeployment || resource.ExternalID == "" {
		return false, nil
	}
	var deleted bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM deployment_fga_sync
			WHERE deployment_id = $1 AND desired_state = $2
		) OR EXISTS (
			SELECT 1
			FROM authorization_resource_sync
			WHERE resource_type = 'deployment' AND resource_id = $1 AND desired_state = $2
		)
	`, resource.ExternalID, DeploymentFGADeleted).Scan(&deleted)
	if err != nil {
		return false, fmt.Errorf("confirm deployment FGA deletion: %w", err)
	}
	return deleted, nil
}

func (s *ResourceAccessSyncStore) MarkSynced(ctx context.Context, intent AccessIntent) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("resource access sync store is not configured")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE resource_access_fga_sync
		SET synced_role = NULLIF($6, ''),
		    synced_version = $7,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    synced_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND subject_type = $4 AND workos_subject_id = $5
		  AND desired_role IS NOT DISTINCT FROM NULLIF($6, '')
		  AND desired_version = $7
	`, intent.OrganizationID, intent.Resource.Type, intent.Resource.ExternalID, intent.Subject.Type, intent.Subject.ID, intent.DesiredRole, intent.DesiredVersion)
	if err != nil {
		return false, fmt.Errorf("mark resource access intent synced: %w", err)
	}
	return changed(result, "mark resource access intent synced")
}

// Discard removes work for a resource that no longer exists in WorkOS.
func (s *ResourceAccessSyncStore) Discard(ctx context.Context, intent AccessIntent) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("resource access sync store is not configured")
	}
	result, err := s.db.ExecContext(ctx, `
		DELETE FROM resource_access_fga_sync
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND subject_type = $4 AND workos_subject_id = $5
		  AND desired_version = $6
	`, intent.OrganizationID, intent.Resource.Type, intent.Resource.ExternalID, intent.Subject.Type, intent.Subject.ID, intent.DesiredVersion)
	if err != nil {
		return false, fmt.Errorf("discard resource access intent: %w", err)
	}
	return changed(result, "discard resource access intent")
}

func (s *ResourceAccessSyncStore) RecordFailure(ctx context.Context, intent AccessIntent, cause error) error {
	if s == nil || s.db == nil {
		return errors.New("resource access sync store is not configured")
	}
	message := "unknown reconciliation failure"
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE resource_access_fga_sync
		SET attempt_count = attempt_count + 1,
		    last_error = $6,
		    next_attempt_at = NOW() + POWER(2, LEAST(attempt_count + 1, 8)) * INTERVAL '1 second',
		    updated_at = NOW()
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND subject_type = $4 AND workos_subject_id = $5
		  AND desired_version = $7
	`, intent.OrganizationID, intent.Resource.Type, intent.Resource.ExternalID, intent.Subject.Type, intent.Subject.ID, message, intent.DesiredVersion)
	if err != nil {
		return fmt.Errorf("record resource access reconciliation failure: %w", err)
	}
	return nil
}

type accessIntentScanner interface {
	Scan(...any) error
}

func scanAccessIntent(scanner accessIntentScanner, changed *bool) (AccessIntent, error) {
	var intent AccessIntent
	var resourceType, subjectType string
	var desiredRole, syncedRole sql.NullString
	var syncedVersion sql.NullInt64
	var lastError sql.NullString
	var syncedAt sql.NullTime
	dest := []any{
		&intent.AccountID, &intent.OrganizationID, &resourceType, &intent.Resource.ExternalID,
		&subjectType, &intent.SubjectID, &intent.Subject.ID, &desiredRole,
		&intent.DesiredVersion, &syncedRole, &syncedVersion, &intent.AttemptCount,
		&lastError, &intent.NextAttemptAt, &syncedAt, &intent.UpdatedAt,
	}
	if changed != nil {
		dest = append(dest, changed)
	}
	if err := scanner.Scan(dest...); err != nil {
		return AccessIntent{}, err
	}
	intent.Resource.Type = ResourceType(resourceType)
	intent.Subject.Type = AssignmentSubjectType(subjectType)
	intent.DesiredRole = RoleSlug(desiredRole.String)
	intent.SyncedRole = RoleSlug(syncedRole.String)
	intent.SyncedVersion = syncedVersion.Int64
	intent.LastError = lastError.String
	intent.SyncedAt = syncedAt.Time
	return intent, nil
}

func validateAccessIntent(intent AccessIntent) error {
	if intent.AccountID == "" || intent.SubjectID == "" {
		return errors.New("account id and subject id are required")
	}
	return validateAccessIntentKey(intent.Key())
}

func validateAccessIntentKey(key AccessIntentKey) error {
	if key.OrganizationID == "" {
		return errors.New("organization id is required")
	}
	if err := validateResource(key.Resource); err != nil {
		return err
	}
	if key.Subject.ID == "" {
		return errors.New("assignment subject id is required")
	}
	switch key.Subject.Type {
	case AssignmentSubjectMembership, AssignmentSubjectGroup:
		return nil
	default:
		return fmt.Errorf("unsupported assignment subject type %q", key.Subject.Type)
	}
}
