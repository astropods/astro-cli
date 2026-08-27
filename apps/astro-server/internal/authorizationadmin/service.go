package authorizationadmin

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

const (
	assignmentConcurrency   = 5
	workOSInventoryCacheTTL = 30 * time.Second
	workOSInventoryTimeout  = 10 * time.Second
)

type workOSAdmin interface {
	authz.AuthorizationResourceCatalog
	authz.AccessAssignments
	authz.Groups
}

type operationStore interface {
	Start(context.Context, string) (*Operation, error)
	Progress(context.Context, string, int, int, int, int, []ReportEntry) error
	Complete(context.Context, string, int, int, int, []ReportEntry) error
	Fail(context.Context, string, int, int, int, int, []ReportEntry, error) error
}

type Service struct {
	db              *sql.DB
	workos          workOSAdmin
	store           operationStore
	workOSCacheMu   sync.Mutex
	workOSCache     *workOSInventorySnapshot
	workOSCacheLoad singleflight.Group
}

type workOSInventorySnapshot struct {
	resources   []authz.AuthorizationResource
	assignments map[string][]authz.RoleAssignment
	groupNames  map[string]string
	expiresAt   time.Time
}

func NewService(db *sql.DB, workos workOSAdmin, store *Store) *Service {
	return newService(db, workos, store)
}

func newService(db *sql.DB, workos workOSAdmin, store operationStore) *Service {
	return &Service{db: db, workos: workos, store: store}
}

func (s *Service) Inventory(ctx context.Context) (*Inventory, error) {
	if s == nil || s.db == nil || s.workos == nil || s.store == nil {
		return nil, ErrNotConfigured
	}
	resources, assignments, groupNames, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	membershipLabels, err := s.membershipLabels(ctx, assignments)
	if err != nil {
		return nil, err
	}
	local, err := s.localDeploymentMetadata(ctx)
	if err != nil {
		return nil, err
	}
	accountsByWorkOSID := make(map[string]authz.AuthorizationResource)
	for _, resource := range resources {
		if resource.Resource.Type == authz.ResourceAccount {
			accountsByWorkOSID[resource.ID] = resource
		}
	}

	result := make([]Resource, 0, len(resources))
	for _, resource := range resources {
		if resource.Resource.Type == authz.ResourceOrganization {
			continue
		}
		row := Resource{
			Type:             string(resource.Resource.Type),
			Name:             resource.Name,
			ExternalID:       resource.Resource.ExternalID,
			WorkOSResourceID: resource.ID,
			CreatedAt:        resource.CreatedAt,
			SyncState:        "registered",
		}
		if resource.Resource.Type == authz.ResourceAccount {
			row.AccountID = resource.Resource.ExternalID
			row.AccountName = resource.Name
		} else if accountResource, ok := accountsByWorkOSID[resource.ParentResourceID]; ok {
			row.AccountID = accountResource.Resource.ExternalID
			row.AccountName = accountResource.Name
		}
		if metadata, ok := local[resource.Resource.ExternalID]; ok {
			if row.AccountID == "" {
				row.AccountID = metadata.AccountID
				row.AccountName = metadata.AccountName
			}
			row.SyncState = metadata.SyncState
			row.LastError = metadata.LastError
		} else if resource.Resource.Type == authz.ResourceDeployment {
			row.SyncState = "workos_only"
		}
		for _, assignment := range assignments[resourceKey(resource.Resource)] {
			label := assignment.Subject.ID
			if assignment.Subject.Type == authz.AssignmentSubjectGroup {
				if name := groupNames[assignment.Subject.ID]; name != "" {
					label = name
				}
			} else if resolved := membershipLabels[assignment.Subject.ID]; resolved != "" {
				label = resolved
			}
			row.Assignments = append(row.Assignments, Assignment{
				SubjectType: string(assignment.Subject.Type), SubjectID: assignment.Subject.ID,
				SubjectLabel: label, Role: string(assignment.Role), Source: string(assignment.Source),
			})
			if strings.HasSuffix(string(assignment.Role), "-admin") {
				row.DirectAdmins = append(row.DirectAdmins, directAdminLabel(assignment.Subject, label))
			}
		}
		row.AssignmentCount = len(row.Assignments)
		sort.Slice(row.Assignments, func(i, j int) bool {
			if row.Assignments[i].SubjectLabel != row.Assignments[j].SubjectLabel {
				return row.Assignments[i].SubjectLabel < row.Assignments[j].SubjectLabel
			}
			return row.Assignments[i].Role < row.Assignments[j].Role
		})
		sort.Strings(row.DirectAdmins)
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			return result[i].Type < result[j].Type
		}
		if result[i].Name != result[j].Name {
			return result[i].Name < result[j].Name
		}
		return result[i].ExternalID < result[j].ExternalID
	})
	return &Inventory{Resources: result}, nil
}

func (s *Service) RunReset(ctx context.Context, operationID string) error {
	if s == nil || s.workos == nil || s.store == nil {
		return ErrNotConfigured
	}
	operation, err := s.store.Start(ctx, operationID)
	if err != nil {
		return err
	}
	resources, assignments, err := s.loadAccount(ctx, operation.AccountID)
	if err != nil {
		_ = s.store.Fail(
			ctx,
			operationID,
			operation.TargetCount,
			operation.ProcessedCount,
			operation.SucceededCount,
			operation.FailedCount+1,
			nil,
			err,
		)
		return err
	}
	report := make([]ReportEntry, 0, len(resources))
	if operation.DryRun {
		for _, resource := range resources {
			report = append(report, reportEntry(resource, "target", nil))
		}
		return s.store.Complete(ctx, operationID, len(resources), len(resources), len(resources), report)
	}
	if operation.SucceededCount == 0 && (operation.ConfirmedCount == nil || *operation.ConfirmedCount != len(resources)) {
		err := fmt.Errorf("confirmed resource count does not match current WorkOS count: confirmed %s, current %d", confirmedCount(operation.ConfirmedCount), len(resources))
		_ = s.store.Fail(ctx, operationID, len(resources), 0, 0, 1, report, err)
		return err
	}
	sort.SliceStable(resources, func(i, j int) bool {
		return deletionRank(resources[i].Resource.Type) < deletionRank(resources[j].Resource.Type)
	})
	processed, succeeded, failed := operation.SucceededCount, operation.SucceededCount, 0
	targetCount := max(operation.TargetCount, succeeded+len(resources))
	var resetErr error
	for _, resource := range resources {
		entryErr := s.deleteResource(ctx, resource, assignments[resourceKey(resource.Resource)])
		processed++
		if entryErr != nil {
			failed++
			resetErr = errors.Join(resetErr, entryErr)
			report = append(report, reportEntry(resource, "failed", entryErr))
		} else {
			succeeded++
			report = append(report, reportEntry(resource, "deleted", nil))
		}
		if err := s.store.Progress(ctx, operationID, targetCount, processed, succeeded, failed, report); err != nil {
			return errors.Join(resetErr, err)
		}
	}
	if resetErr != nil {
		if err := s.store.Fail(ctx, operationID, targetCount, processed, succeeded, failed, report, resetErr); err != nil {
			return errors.Join(resetErr, err)
		}
		return resetErr
	}
	return s.store.Complete(ctx, operationID, targetCount, processed, succeeded, report)
}

func (s *Service) loadAccount(ctx context.Context, accountID string) ([]authz.AuthorizationResource, map[string][]authz.RoleAssignment, error) {
	organizationID, err := s.workOSOrganizationForAccount(ctx, accountID)
	if err != nil {
		return nil, nil, err
	}
	resources, err := s.workos.ListAuthorizationResourcesForOrganization(ctx, organizationID)
	if err != nil {
		return nil, nil, err
	}
	resources = productResourcesForOrganization(resources, organizationID)
	assignments, _, err := s.directAssignments(ctx, resources)
	if err != nil {
		return nil, nil, err
	}
	return resources, assignments, nil
}

func (s *Service) workOSOrganizationForAccount(ctx context.Context, accountID string) (string, error) {
	if s == nil || s.db == nil {
		return "", ErrNotConfigured
	}
	if accountID == "" {
		return "", ErrAccountNotFound
	}
	var organizationID string
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(ao.workos_org_id, '')
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.id = $1 AND a.deleted_at IS NULL
	`, accountID).Scan(&organizationID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrAccountNotFound
	}
	if err != nil {
		return "", fmt.Errorf("resolve authorization reset account: %w", err)
	}
	if organizationID == "" {
		return "", ErrAccountNotLinked
	}
	return organizationID, nil
}

func (s *Service) load(ctx context.Context) ([]authz.AuthorizationResource, map[string][]authz.RoleAssignment, map[string]string, error) {
	if s == nil || s.workos == nil {
		return nil, nil, nil, ErrNotConfigured
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}
	if snapshot := s.cachedWorkOSInventory(); snapshot != nil {
		return snapshot.resources, snapshot.assignments, snapshot.groupNames, nil
	}
	loaded, err, _ := s.workOSCacheLoad.Do("inventory", func() (any, error) {
		if snapshot := s.cachedWorkOSInventory(); snapshot != nil {
			return snapshot, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workOSInventoryTimeout)
		defer cancel()
		resources, err := s.listAuthorizationResources(loadCtx)
		if err != nil {
			return nil, err
		}
		assignments, groupNames, err := s.directAssignments(loadCtx, productResources(resources))
		if err != nil {
			return nil, err
		}
		snapshot := &workOSInventorySnapshot{
			resources: resources, assignments: assignments, groupNames: groupNames,
			expiresAt: time.Now().Add(workOSInventoryCacheTTL),
		}
		s.workOSCacheMu.Lock()
		s.workOSCache = snapshot
		s.workOSCacheMu.Unlock()
		return snapshot, nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	snapshot := loaded.(*workOSInventorySnapshot)
	return snapshot.resources, snapshot.assignments, snapshot.groupNames, nil
}

func (s *Service) listAuthorizationResources(ctx context.Context) ([]authz.AuthorizationResource, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT ao.workos_org_id
		FROM account_organizations ao
		JOIN accounts a ON a.id = ao.account_id
		WHERE a.deleted_at IS NULL AND ao.workos_org_id <> ''
		ORDER BY ao.workos_org_id
	`)
	if err != nil {
		return nil, fmt.Errorf("list linked WorkOS organizations: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	organizationIDs := make([]string, 0)
	for rows.Next() {
		var organizationID string
		if err := rows.Scan(&organizationID); err != nil {
			return nil, fmt.Errorf("scan linked WorkOS organization: %w", err)
		}
		organizationIDs = append(organizationIDs, organizationID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked WorkOS organizations: %w", err)
	}

	resources := make([]authz.AuthorizationResource, 0)
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(assignmentConcurrency)
	for _, organizationID := range organizationIDs {
		group.Go(func() error {
			listed, err := s.workos.ListAuthorizationResourcesForOrganization(groupCtx, organizationID)
			if err != nil {
				return err
			}
			mu.Lock()
			resources = append(resources, listed...)
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return resources, nil
}

func (s *Service) cachedWorkOSInventory() *workOSInventorySnapshot {
	s.workOSCacheMu.Lock()
	defer s.workOSCacheMu.Unlock()
	if s.workOSCache == nil || time.Now().After(s.workOSCache.expiresAt) {
		s.workOSCache = nil
		return nil
	}
	return s.workOSCache
}

func (s *Service) directAssignments(ctx context.Context, resources []authz.AuthorizationResource) (map[string][]authz.RoleAssignment, map[string]string, error) {
	result := make(map[string][]authz.RoleAssignment)
	groupNames := make(map[string]string)
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(assignmentConcurrency)
	for _, resource := range resources {
		group.Go(func() error {
			listed, err := s.workos.ListRoleAssignments(groupCtx, resource.OrganizationID, resource.Resource)
			if err != nil {
				return err
			}
			mu.Lock()
			defer mu.Unlock()
			for _, assignment := range listed {
				if assignment.Source == authz.AssignmentSourceDirect {
					result[resourceKey(resource.Resource)] = append(result[resourceKey(resource.Resource)], assignment)
				}
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, nil, err
	}

	organizationIDs := make(map[string]struct{})
	for _, resource := range resources {
		organizationIDs[resource.OrganizationID] = struct{}{}
	}
	groupAssignments, groupAssignmentsCtx := errgroup.WithContext(ctx)
	groupAssignments.SetLimit(assignmentConcurrency)
	for organizationID := range organizationIDs {
		groups, err := s.allGroups(ctx, organizationID)
		if err != nil {
			return nil, nil, err
		}
		for _, currentGroup := range groups {
			groupNames[currentGroup.ID] = currentGroup.Name
			groupAssignments.Go(func() error {
				listed, err := s.workos.ListGroupRoleAssignments(groupAssignmentsCtx, currentGroup.ID)
				if err != nil {
					return err
				}
				mu.Lock()
				defer mu.Unlock()
				for _, assignment := range listed {
					result[resourceKey(assignment.Resource)] = append(result[resourceKey(assignment.Resource)], assignment)
				}
				return nil
			})
		}
	}
	if err := groupAssignments.Wait(); err != nil {
		return nil, nil, err
	}
	return result, groupNames, nil
}

func (s *Service) membershipLabels(ctx context.Context, assignments map[string][]authz.RoleAssignment) (map[string]string, error) {
	ids := make([]string, 0)
	seen := make(map[string]struct{})
	for _, resourceAssignments := range assignments {
		for _, assignment := range resourceAssignments {
			if assignment.Subject.Type != authz.AssignmentSubjectMembership {
				continue
			}
			if _, ok := seen[assignment.Subject.ID]; !ok {
				seen[assignment.Subject.ID] = struct{}{}
				ids = append(ids, assignment.Subject.ID)
			}
		}
	}
	labels := make(map[string]string, len(ids))
	if len(ids) == 0 {
		return labels, nil
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT mw.workos_membership_id,
		       COALESCE(NULLIF(me.email, ''), mw.user_id)
		FROM account_member_workos mw
		LEFT JOIN LATERAL (
			SELECT email
			FROM account_member_emails
			WHERE user_id = mw.user_id
			ORDER BY (source = 'workos') DESC, updated_at DESC
			LIMIT 1
		) me ON TRUE
		WHERE mw.workos_membership_id = ANY($1)
	`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("resolve authorization assignment labels: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	for rows.Next() {
		var id, label string
		if err := rows.Scan(&id, &label); err != nil {
			return nil, fmt.Errorf("scan authorization assignment label: %w", err)
		}
		labels[id] = label
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorization assignment labels: %w", err)
	}
	return labels, nil
}

func (s *Service) allGroups(ctx context.Context, organizationID string) ([]authz.Group, error) {
	groups := make([]authz.Group, 0)
	after := ""
	for {
		page, err := s.workos.ListGroups(ctx, organizationID, authz.PageRequest{After: after, Limit: 100})
		if err != nil {
			return nil, err
		}
		groups = append(groups, page.Groups...)
		if page.NextCursor == "" {
			return groups, nil
		}
		after = page.NextCursor
	}
}

func (s *Service) deleteResource(ctx context.Context, resource authz.AuthorizationResource, assignments []authz.RoleAssignment) error {
	var result error
	for _, assignment := range assignments {
		err := s.workos.RemoveRole(ctx, assignment.Subject, assignment.Role, resource.Resource)
		if err != nil && !errors.Is(err, authz.ErrRoleAssignmentNotFound) {
			result = errors.Join(result, fmt.Errorf("remove %s from %s: %w", assignment.Role, directAdminLabel(assignment.Subject, assignment.Subject.ID), err))
		}
	}
	if result != nil {
		return result
	}
	if err := s.workos.DeleteAuthorizationResource(ctx, resource.ID); err != nil && !errors.Is(err, authz.ErrResourceNotFound) {
		return err
	}
	return nil
}

type deploymentMetadata struct {
	AccountID   string
	AccountName string
	SyncState   string
	LastError   string
}

func (s *Service) localDeploymentMetadata(ctx context.Context) (map[string]deploymentMetadata, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT d.id,
		       d.account_id,
		       COALESCE(NULLIF(a.display_name, ''), a.name),
		       'registered',
		       ''
		FROM deployments d
		JOIN accounts a ON a.id = d.account_id
	`)
	if err != nil {
		return nil, fmt.Errorf("load authorization inventory deployment metadata: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	metadata := make(map[string]deploymentMetadata)
	for rows.Next() {
		var id string
		var current deploymentMetadata
		if err := rows.Scan(&id, &current.AccountID, &current.AccountName, &current.SyncState, &current.LastError); err != nil {
			return nil, fmt.Errorf("scan authorization inventory deployment metadata: %w", err)
		}
		metadata[id] = current
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate authorization inventory deployment metadata: %w", err)
	}
	return metadata, nil
}

func productResources(resources []authz.AuthorizationResource) []authz.AuthorizationResource {
	result := make([]authz.AuthorizationResource, 0, len(resources))
	for _, resource := range resources {
		if resource.Resource.Type != authz.ResourceOrganization {
			result = append(result, resource)
		}
	}
	return result
}

func productResourcesForOrganization(resources []authz.AuthorizationResource, organizationID string) []authz.AuthorizationResource {
	result := make([]authz.AuthorizationResource, 0, len(resources))
	for _, resource := range resources {
		if resource.OrganizationID == organizationID && resource.Resource.Type != authz.ResourceOrganization {
			result = append(result, resource)
		}
	}
	return result
}

func resourceKey(resource authz.ResourceRef) string {
	return string(resource.Type) + "\x00" + resource.ExternalID
}

func directAdminLabel(subject authz.AssignmentSubject, label string) string {
	if subject.Type == authz.AssignmentSubjectGroup {
		return "group:" + label
	}
	return label
}

func reportEntry(resource authz.AuthorizationResource, status string, err error) ReportEntry {
	entry := ReportEntry{
		ResourceID: resource.ID,
		Type:       string(resource.Resource.Type),
		ExternalID: resource.Resource.ExternalID,
		Name:       resource.Name,
		Status:     status,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	return entry
}

func deletionRank(resourceType authz.ResourceType) int {
	switch resourceType {
	case authz.ResourceAccount:
		return 1
	default:
		return 0
	}
}

func confirmedCount(count *int) string {
	if count == nil {
		return "missing"
	}
	return fmt.Sprintf("%d", *count)
}
