package authorizationadmin

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

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

type Service struct {
	db              *sql.DB
	workos          workOSAdmin
	workOSCacheMu   sync.Mutex
	workOSCache     *workOSInventorySnapshot
	workOSCacheLoad singleflight.Group
}

type workOSInventorySnapshot struct {
	resources   []authz.AuthorizationResource
	assignments map[string][]authz.RoleAssignment
	expiresAt   time.Time
}

func NewService(db *sql.DB, workos workOSAdmin) *Service {
	return &Service{db: db, workos: workos}
}

func (s *Service) Inventory(ctx context.Context) (*Inventory, error) {
	if s == nil || s.db == nil || s.workos == nil {
		return nil, ErrNotConfigured
	}
	resources, assignments, err := s.load(ctx)
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
		} else if resource.Resource.Type != authz.ResourceAccount {
			row.SyncState = "workos_only"
		}
		for _, assignment := range assignments[resourceKey(resource.Resource)] {
			row.AssignmentCount++
			if strings.HasSuffix(string(assignment.Role), "-admin") {
				row.DirectAdmins = append(row.DirectAdmins, directAdminLabel(assignment.Subject))
			}
		}
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

func (s *Service) load(ctx context.Context) ([]authz.AuthorizationResource, map[string][]authz.RoleAssignment, error) {
	if s == nil || s.workos == nil {
		return nil, nil, ErrNotConfigured
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	if snapshot := s.cachedWorkOSInventory(); snapshot != nil {
		return snapshot.resources, snapshot.assignments, nil
	}
	loaded, err, _ := s.workOSCacheLoad.Do("inventory", func() (any, error) {
		if snapshot := s.cachedWorkOSInventory(); snapshot != nil {
			return snapshot, nil
		}
		loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), workOSInventoryTimeout)
		defer cancel()
		resources, err := s.workos.ListAuthorizationResources(loadCtx)
		if err != nil {
			return nil, err
		}
		assignments, err := s.directAssignments(loadCtx, productResources(resources))
		if err != nil {
			return nil, err
		}
		snapshot := &workOSInventorySnapshot{
			resources: resources, assignments: assignments, expiresAt: time.Now().Add(workOSInventoryCacheTTL),
		}
		s.workOSCacheMu.Lock()
		s.workOSCache = snapshot
		s.workOSCacheMu.Unlock()
		return snapshot, nil
	})
	if err != nil {
		return nil, nil, err
	}
	snapshot := loaded.(*workOSInventorySnapshot)
	return snapshot.resources, snapshot.assignments, nil
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

func (s *Service) directAssignments(ctx context.Context, resources []authz.AuthorizationResource) (map[string][]authz.RoleAssignment, error) {
	result := make(map[string][]authz.RoleAssignment)
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
		return nil, err
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
			return nil, err
		}
		for _, currentGroup := range groups {
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
		return nil, err
	}
	return result, nil
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
		       CASE
		         WHEN f.deployment_id IS NULL THEN 'registered'
		         WHEN f.synced_state IS NOT DISTINCT FROM f.desired_state
		          AND f.synced_version IS NOT DISTINCT FROM f.desired_version THEN 'synced'
		         ELSE 'pending'
		       END,
		       COALESCE(f.last_error, '')
		FROM deployments d
		JOIN accounts a ON a.id = d.account_id
		LEFT JOIN deployment_fga_sync f ON f.deployment_id = d.id
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

func resourceKey(resource authz.ResourceRef) string {
	return string(resource.Type) + "\x00" + resource.ExternalID
}

func directAdminLabel(subject authz.AssignmentSubject) string {
	if subject.Type == authz.AssignmentSubjectGroup {
		return "group:" + subject.ID
	}
	return subject.ID
}
