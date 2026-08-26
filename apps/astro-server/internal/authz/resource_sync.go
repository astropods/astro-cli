package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

type AuthorizationResourceState string

const (
	AuthorizationResourceRegistered AuthorizationResourceState = "registered"
	AuthorizationResourceDeleted    AuthorizationResourceState = "deleted"
)

type ResourceSyncKey struct {
	OrganizationID string
	Resource       ResourceRef
}

type ResourceRegistration struct {
	AccountID      string
	OrganizationID string
	Resource       ResourceRef
	Parent         ResourceRef
	Name           string
	CreatorUserID  string
	CreatorRole    RoleSlug
}

type AuthorizationResourceWork struct {
	ResourceSyncKey
	AccountID                     string
	Parent                        ResourceRef
	Name                          string
	DesiredState                  AuthorizationResourceState
	DesiredVersion                int64
	SyncedState                   AuthorizationResourceState
	SyncedVersion                 int64
	WorkOSAuthorizationResourceID string
	CreatorUserID                 string
	CreatorRole                   RoleSlug
	CreatorIsMember               bool
	CreatorAssignmentPending      bool
	MembershipID                  string
	AttemptCount                  int
}

// AuthorizationResourceSyncStore is the durable lifecycle ledger for every
// Account-rooted WorkOS authorization resource.
type AuthorizationResourceSyncStore struct {
	db      *sql.DB
	enabled bool
}

// DeploymentResourceSyncRecorder preserves the deployment handler seam while
// the lifecycle implementation moves from a deployment-only table to the
// generic authorization resource ledger.
type DeploymentResourceSyncRecorder interface {
	RecordRegistrationTx(context.Context, *sql.Tx, string) (bool, error)
	RecordNameUpdateTx(context.Context, *sql.Tx, string) (bool, error)
	RecordDeletionTx(context.Context, *sql.Tx, string) (bool, error)
	HasPendingForAccount(context.Context, string) (bool, error)
}

func NewAuthorizationResourceSyncStore(db *sql.DB, enabled bool) *AuthorizationResourceSyncStore {
	return &AuthorizationResourceSyncStore{db: db, enabled: enabled && db != nil}
}

func (s *AuthorizationResourceSyncStore) Enabled() bool {
	return s != nil && s.enabled
}

func (s *AuthorizationResourceSyncStore) RecordAccountRegistration(ctx context.Context, accountID string) (ResourceSyncKey, bool, error) {
	if !s.Enabled() {
		return ResourceSyncKey{}, false, nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ResourceSyncKey{}, false, fmt.Errorf("begin account authorization registration: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck
	registration, err := loadAccountRegistration(ctx, tx, accountID)
	if err != nil {
		return ResourceSyncKey{}, false, err
	}
	changed, err := s.recordRegistrationTx(ctx, tx, registration)
	if err != nil {
		return ResourceSyncKey{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return ResourceSyncKey{}, false, fmt.Errorf("commit account authorization registration: %w", err)
	}
	return ResourceSyncKey{OrganizationID: registration.OrganizationID, Resource: registration.Resource}, changed, nil
}

// RecordRegistrationTx implements the deployment lifecycle recorder seam.
func (s *AuthorizationResourceSyncStore) RecordRegistrationTx(ctx context.Context, tx *sql.Tx, deploymentID string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	registration, err := loadDeploymentRegistration(ctx, tx, deploymentID)
	if err != nil {
		return false, err
	}
	if registration.Resource.ExternalID == "" {
		return false, nil
	}
	return s.recordChildRegistrationTx(ctx, tx, registration)
}

func (s *AuthorizationResourceSyncStore) RecordBlueprintRegistrationTx(ctx context.Context, tx *sql.Tx, accountID, name string) (ResourceSyncKey, bool, error) {
	if !s.Enabled() {
		return ResourceSyncKey{}, false, nil
	}
	if tx == nil {
		return ResourceSyncKey{}, false, errors.New("authorization resource sync transaction is required")
	}
	var registration ResourceRegistration
	var uid, creator sql.NullString
	err := tx.QueryRowContext(ctx, `
		SELECT a.account_id,
		       ao.workos_org_id,
		       a.uid::text,
		       a.name,
		       a.created_by
		FROM agents a
		JOIN accounts ac ON ac.id = a.account_id AND ac.type = 'organization'
		JOIN account_organizations ao ON ao.account_id = ac.id
		WHERE a.account_id = $1 AND a.name = $2
	`, accountID, name).Scan(
		&registration.AccountID,
		&registration.OrganizationID,
		&uid,
		&registration.Name,
		&creator,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceSyncKey{}, false, nil
	}
	if err != nil {
		return ResourceSyncKey{}, false, fmt.Errorf("load Blueprint authorization registration: %w", err)
	}
	if !uid.Valid || uid.String == "" {
		return ResourceSyncKey{}, false, errors.New("blueprint authorization id is missing")
	}
	registration.Resource = BlueprintResource(uid.String)
	registration.Parent = AccountResource(accountID)
	registration.CreatorUserID = creator.String
	registration.CreatorRole = RoleBlueprintAdmin
	changed, err := s.recordChildRegistrationTx(ctx, tx, registration)
	return ResourceSyncKey{OrganizationID: registration.OrganizationID, Resource: registration.Resource}, changed, err
}

func (s *AuthorizationResourceSyncStore) recordChildRegistrationTx(ctx context.Context, tx *sql.Tx, registration ResourceRegistration) (bool, error) {
	accountRegistration, err := loadAccountRegistration(ctx, tx, registration.AccountID)
	if err != nil {
		return false, err
	}
	parentChanged, err := s.recordRegistrationTx(ctx, tx, accountRegistration)
	if err != nil {
		return false, err
	}
	childChanged, err := s.recordRegistrationTx(ctx, tx, registration)
	return parentChanged || childChanged, err
}

func (s *AuthorizationResourceSyncStore) recordRegistrationTx(ctx context.Context, tx *sql.Tx, registration ResourceRegistration) (bool, error) {
	if tx == nil {
		return false, errors.New("authorization resource sync transaction is required")
	}
	if registration.AccountID == "" || registration.OrganizationID == "" {
		return false, errors.New("authorization resource account and organization are required")
	}
	if err := validateResource(registration.Resource); err != nil {
		return false, err
	}
	if err := validateResource(registration.Parent); err != nil {
		return false, fmt.Errorf("parent %w", err)
	}
	if registration.Name == "" {
		return false, errors.New("authorization resource name is required")
	}
	result, err := tx.ExecContext(ctx, `
		INSERT INTO authorization_resource_sync (
			account_id, organization_id, resource_type, resource_id,
			parent_resource_type, parent_resource_id, desired_name,
			desired_state, desired_version, creator_user_id, creator_role,
			next_attempt_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'registered', 1, NULLIF($8, ''), NULLIF($9, ''), NOW(), NOW())
		ON CONFLICT (organization_id, resource_type, resource_id) DO UPDATE
		SET account_id = EXCLUDED.account_id,
		    parent_resource_type = EXCLUDED.parent_resource_type,
		    parent_resource_id = EXCLUDED.parent_resource_id,
		    desired_name = EXCLUDED.desired_name,
		    desired_state = 'registered',
		    desired_version = authorization_resource_sync.desired_version + 1,
		    creator_user_id = COALESCE(authorization_resource_sync.creator_user_id, EXCLUDED.creator_user_id),
		    creator_role = COALESCE(authorization_resource_sync.creator_role, EXCLUDED.creator_role),
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    updated_at = NOW()
		WHERE authorization_resource_sync.desired_state <> 'registered'
		   OR authorization_resource_sync.desired_name IS DISTINCT FROM EXCLUDED.desired_name
		   OR authorization_resource_sync.parent_resource_type IS DISTINCT FROM EXCLUDED.parent_resource_type
		   OR authorization_resource_sync.parent_resource_id IS DISTINCT FROM EXCLUDED.parent_resource_id
	`,
		registration.AccountID,
		registration.OrganizationID,
		registration.Resource.Type,
		registration.Resource.ExternalID,
		registration.Parent.Type,
		registration.Parent.ExternalID,
		registration.Name,
		registration.CreatorUserID,
		registration.CreatorRole,
	)
	if err != nil {
		return false, fmt.Errorf("record %s authorization registration: %w", registration.Resource.Type, err)
	}
	return changed(result, "record authorization resource registration")
}

func (s *AuthorizationResourceSyncStore) RecordNameUpdateTx(ctx context.Context, tx *sql.Tx, deploymentID string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	var name string
	if err := tx.QueryRowContext(ctx, `
		SELECT COALESCE(NULLIF(display_name, ''), agent_name) FROM deployments WHERE id = $1
	`, deploymentID).Scan(&name); err != nil {
		return false, fmt.Errorf("load deployment authorization name: %w", err)
	}
	return s.recordNameUpdateTx(ctx, tx, DeploymentResource(deploymentID), name)
}

func (s *AuthorizationResourceSyncStore) recordNameUpdateTx(ctx context.Context, tx *sql.Tx, resource ResourceRef, name string) (bool, error) {
	result, err := tx.ExecContext(ctx, `
		UPDATE authorization_resource_sync
		SET desired_name = $3,
		    desired_version = desired_version + 1,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    updated_at = NOW()
		WHERE resource_type = $1 AND resource_id = $2
		  AND desired_state = 'registered'
		  AND desired_name IS DISTINCT FROM $3
	`, resource.Type, resource.ExternalID, name)
	if err != nil {
		return false, fmt.Errorf("record %s authorization name update: %w", resource.Type, err)
	}
	return changed(result, "record authorization resource name update")
}

func (s *AuthorizationResourceSyncStore) RecordDeletionTx(ctx context.Context, tx *sql.Tx, deploymentID string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	return s.recordDeletionTx(ctx, tx, DeploymentResource(deploymentID))
}

func (s *AuthorizationResourceSyncStore) RecordBlueprintDeletionTx(ctx context.Context, tx *sql.Tx, uid string) (bool, error) {
	if !s.Enabled() || uid == "" {
		return false, nil
	}
	return s.recordDeletionTx(ctx, tx, BlueprintResource(uid))
}

func (s *AuthorizationResourceSyncStore) recordDeletionTx(ctx context.Context, tx *sql.Tx, resource ResourceRef) (bool, error) {
	if tx == nil {
		return false, errors.New("authorization resource sync transaction is required")
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE authorization_resource_sync
		SET desired_state = 'deleted',
		    desired_version = desired_version + 1,
		    creator_assignment_pending = FALSE,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    updated_at = NOW()
		WHERE resource_type = $1 AND resource_id = $2 AND desired_state <> 'deleted'
	`, resource.Type, resource.ExternalID)
	if err != nil {
		return false, fmt.Errorf("record %s authorization deletion: %w", resource.Type, err)
	}
	return changed(result, "record authorization resource deletion")
}

func (s *AuthorizationResourceSyncStore) Pending(ctx context.Context, key ResourceSyncKey) (*AuthorizationResourceWork, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("authorization resource sync store is not configured")
	}
	var work AuthorizationResourceWork
	err := s.db.QueryRowContext(ctx, `
		SELECT s.account_id,
		       s.organization_id,
		       s.resource_type,
		       s.resource_id,
		       s.parent_resource_type,
		       s.parent_resource_id,
		       s.desired_name,
		       s.desired_state,
		       s.desired_version,
		       COALESCE(s.synced_state, ''),
		       COALESCE(s.synced_version, 0),
		       COALESCE(s.workos_authorization_resource_id, ''),
		       COALESCE(s.creator_user_id, ''),
		       COALESCE(s.creator_role, ''),
		       s.creator_assignment_pending,
		       s.attempt_count,
		       CASE WHEN s.creator_user_id IS NULL THEN FALSE ELSE EXISTS (
		         SELECT 1 FROM account_members am
		         WHERE am.account_id = s.account_id AND am.user_id = s.creator_user_id
		       ) END,
		       COALESCE(amw.workos_membership_id, '')
		FROM authorization_resource_sync s
		LEFT JOIN account_member_workos amw
		  ON amw.account_id = s.account_id AND amw.user_id = s.creator_user_id
		WHERE ($1 = '' OR s.organization_id = $1) AND s.resource_type = $2 AND s.resource_id = $3
		  AND (s.synced_state IS DISTINCT FROM s.desired_state
		       OR s.synced_version IS DISTINCT FROM s.desired_version
		       OR s.creator_assignment_pending)
	`, key.OrganizationID, key.Resource.Type, key.Resource.ExternalID).Scan(
		&work.AccountID,
		&work.OrganizationID,
		&work.Resource.Type,
		&work.Resource.ExternalID,
		&work.Parent.Type,
		&work.Parent.ExternalID,
		&work.Name,
		&work.DesiredState,
		&work.DesiredVersion,
		&work.SyncedState,
		&work.SyncedVersion,
		&work.WorkOSAuthorizationResourceID,
		&work.CreatorUserID,
		&work.CreatorRole,
		&work.CreatorAssignmentPending,
		&work.AttemptCount,
		&work.CreatorIsMember,
		&work.MembershipID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load pending authorization resource: %w", err)
	}
	return &work, nil
}

func (s *AuthorizationResourceSyncStore) Due(ctx context.Context, limit int) ([]ResourceSyncKey, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("authorization resource sync store is not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT organization_id, resource_type, resource_id
		FROM authorization_resource_sync
		WHERE (synced_state IS DISTINCT FROM desired_state
		       OR synced_version IS DISTINCT FROM desired_version
		       OR creator_assignment_pending)
		  AND next_attempt_at <= NOW()
		ORDER BY next_attempt_at, updated_at
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list due authorization resources: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	keys := make([]ResourceSyncKey, 0)
	for rows.Next() {
		var key ResourceSyncKey
		if err := rows.Scan(&key.OrganizationID, &key.Resource.Type, &key.Resource.ExternalID); err != nil {
			return nil, fmt.Errorf("scan due authorization resource: %w", err)
		}
		keys = append(keys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due authorization resources: %w", err)
	}
	return keys, nil
}

func (s *AuthorizationResourceSyncStore) MarkSynced(ctx context.Context, work AuthorizationResourceWork, workosID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE authorization_resource_sync
		SET synced_state = $4,
		    synced_version = $5,
		    workos_authorization_resource_id = COALESCE(NULLIF($6, ''), workos_authorization_resource_id),
		    creator_assignment_pending = FALSE,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW(),
		    synced_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND desired_state = $4 AND desired_version = $5
	`, work.OrganizationID, work.Resource.Type, work.Resource.ExternalID, work.DesiredState, work.DesiredVersion, workosID)
	if err != nil {
		return false, fmt.Errorf("mark authorization resource synced: %w", err)
	}
	return changed(result, "mark authorization resource synced")
}

// MarkRegisteredPendingCreator persists successful WorkOS registration while
// leaving the creator role assignment eligible for retry.
func (s *AuthorizationResourceSyncStore) MarkRegisteredPendingCreator(ctx context.Context, work AuthorizationResourceWork, workosID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE authorization_resource_sync
		SET synced_state = $4,
		    synced_version = $5,
		    workos_authorization_resource_id = COALESCE(NULLIF($6, ''), workos_authorization_resource_id),
		    creator_assignment_pending = TRUE,
		    synced_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND desired_state = $4 AND desired_version = $5
	`, work.OrganizationID, work.Resource.Type, work.Resource.ExternalID, work.DesiredState, work.DesiredVersion, workosID)
	if err != nil {
		return false, fmt.Errorf("mark authorization resource registered pending creator: %w", err)
	}
	return changed(result, "mark authorization resource registered pending creator")
}

func (s *AuthorizationResourceSyncStore) DeferCreatorAssignment(ctx context.Context, work AuthorizationResourceWork, workosID string) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
		UPDATE authorization_resource_sync
		SET synced_state = $4,
		    synced_version = $5,
		    workos_authorization_resource_id = COALESCE(NULLIF($6, ''), workos_authorization_resource_id),
		    creator_assignment_pending = TRUE,
		    attempt_count = 0,
		    last_error = NULL,
		    next_attempt_at = NOW() + INTERVAL '1 hour',
		    synced_at = NOW(),
		    updated_at = NOW()
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND desired_state = $4 AND desired_version = $5
	`, work.OrganizationID, work.Resource.Type, work.Resource.ExternalID, work.DesiredState, work.DesiredVersion, workosID)
	if err != nil {
		return false, fmt.Errorf("defer authorization resource creator assignment: %w", err)
	}
	return changed(result, "defer authorization resource creator assignment")
}

func (s *AuthorizationResourceSyncStore) RecordFailure(ctx context.Context, work AuthorizationResourceWork, cause error) error {
	message := "unknown reconciliation failure"
	if cause != nil {
		message = cause.Error()
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE authorization_resource_sync
		SET attempt_count = attempt_count + 1,
		    last_error = $4,
		    next_attempt_at = NOW() + POWER(2, LEAST(attempt_count + 1, 8)) * INTERVAL '1 second',
		    updated_at = NOW()
		WHERE organization_id = $1 AND resource_type = $2 AND resource_id = $3
		  AND desired_state = $5 AND desired_version = $6
	`, work.OrganizationID, work.Resource.Type, work.Resource.ExternalID, message, work.DesiredState, work.DesiredVersion)
	if err != nil {
		return fmt.Errorf("record authorization resource failure: %w", err)
	}
	return nil
}

func (s *AuthorizationResourceSyncStore) HasPendingForAccount(ctx context.Context, accountID string) (bool, error) {
	if !s.Enabled() {
		return false, nil
	}
	var pending bool
	err := s.db.QueryRowContext(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM authorization_resource_sync
		  WHERE account_id = $1
		    AND (synced_state IS DISTINCT FROM desired_state OR synced_version IS DISTINCT FROM desired_version)
		) OR EXISTS (
		  SELECT 1 FROM deployment_fga_sync s
		  JOIN deployments d ON d.id = s.deployment_id
		  WHERE d.account_id = $1
		    AND (s.synced_state IS DISTINCT FROM s.desired_state OR s.synced_version IS DISTINCT FROM s.desired_version)
		)
	`, accountID).Scan(&pending)
	if err != nil {
		return false, fmt.Errorf("check pending authorization resources: %w", err)
	}
	return pending, nil
}

func loadAccountRegistration(ctx context.Context, tx *sql.Tx, accountID string) (ResourceRegistration, error) {
	if tx == nil {
		return ResourceRegistration{}, errors.New("authorization resource sync transaction is required")
	}
	var registration ResourceRegistration
	err := tx.QueryRowContext(ctx, `
		SELECT a.id,
		       ao.workos_org_id,
		       COALESCE(NULLIF(a.display_name, ''), a.name),
		       a.owner_user_id
		FROM accounts a
		JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.id = $1 AND a.type = 'organization' AND a.deleted_at IS NULL
	`, accountID).Scan(
		&registration.AccountID,
		&registration.OrganizationID,
		&registration.Name,
		&registration.CreatorUserID,
	)
	if err != nil {
		return ResourceRegistration{}, fmt.Errorf("load Account authorization registration: %w", err)
	}
	registration.Resource = AccountResource(registration.AccountID)
	registration.Parent = ResourceRef{Type: ResourceOrganization, ExternalID: registration.OrganizationID}
	registration.CreatorRole = RoleAccountAdmin
	return registration, nil
}

func loadDeploymentRegistration(ctx context.Context, tx *sql.Tx, deploymentID string) (ResourceRegistration, error) {
	if tx == nil {
		return ResourceRegistration{}, errors.New("authorization resource sync transaction is required")
	}
	var registration ResourceRegistration
	err := tx.QueryRowContext(ctx, `
		SELECT d.account_id,
		       ao.workos_org_id,
		       d.id,
		       COALESCE(NULLIF(d.display_name, ''), d.agent_name),
		       COALESCE(d.deployed_by, '')
		FROM deployments d
		JOIN accounts a ON a.id = d.account_id AND a.type = 'organization'
		JOIN account_organizations ao ON ao.account_id = a.id
		WHERE d.id = $1
	`, deploymentID).Scan(
		&registration.AccountID,
		&registration.OrganizationID,
		&registration.Resource.ExternalID,
		&registration.Name,
		&registration.CreatorUserID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ResourceRegistration{}, nil
	}
	if err != nil {
		return ResourceRegistration{}, fmt.Errorf("load Deployment authorization registration: %w", err)
	}
	registration.Resource.Type = ResourceDeployment
	registration.Parent = AccountResource(registration.AccountID)
	registration.CreatorRole = RoleDeploymentAdmin
	return registration, nil
}
