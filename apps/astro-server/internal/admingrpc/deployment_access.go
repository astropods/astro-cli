package admingrpc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const organizationMembershipPageSize = 100

type deploymentAccessAssignments interface {
	ListRoleAssignments(context.Context, string, authz.ResourceRef) ([]authz.RoleAssignment, error)
}

type deploymentAccessMemberships interface {
	ListMemberships(context.Context, string, authz.ResourceRef, authz.Action) ([]string, error)
}

type organizationMembershipLister interface {
	ListMembershipsPage(context.Context, string, org.ListOpts) (org.MembershipPage, error)
}

// SetDeploymentAccessInspector wires the shared WorkOS clients used by Queen's
// read-only deployment access inspector.
func (s *Server) SetDeploymentAccessInspector(fga authz.FGA, memberships organizationMembershipLister) {
	if fga == nil || memberships == nil {
		return
	}
	assignments, assignmentsOK := fga.(authz.AccessAssignments)
	resourceMemberships, membershipsOK := fga.(authz.ResourceMembershipDiscovery)
	if !assignmentsOK || !membershipsOK {
		return
	}
	s.deploymentAccessAssignments = assignments
	s.deploymentAccessMemberships = resourceMemberships
	s.organizationMemberships = memberships
}

// GetDeploymentAccess returns an operator-facing explanation of every
// organization member's effective access to one deployment.
func (s *Server) GetDeploymentAccess(ctx context.Context, req *adminv1.GetDeploymentAccessRequest) (*adminv1.GetDeploymentAccessResponse, error) {
	if req.DeploymentId == "" {
		return nil, status.Error(codes.InvalidArgument, "deployment_id is required")
	}
	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentId)
	if err != nil {
		return nil, fmt.Errorf("get deployment for access inspection: %w", err)
	}
	if dep == nil {
		return nil, fmt.Errorf("deployment not found for id %q", req.DeploymentId)
	}

	var accountType, organizationID string
	err = s.db.QueryRowContext(ctx, `
		SELECT a.type, COALESCE(ao.workos_org_id, '')
		FROM accounts a
		LEFT JOIN account_organizations ao ON ao.account_id = a.id
		WHERE a.id = $1
	`, dep.AccountID).Scan(&accountType, &organizationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("account not found for deployment %q", dep.ID)
	}
	if err != nil {
		return nil, fmt.Errorf("resolve deployment organization: %w", err)
	}

	permissions := actionStrings(authz.DeploymentActions())
	if accountType != "organization" {
		return &adminv1.GetDeploymentAccessResponse{
			Status: "personal", Message: "Fine-grained access applies only to organization deployments.",
			Permissions: permissions,
		}, nil
	}
	if organizationID == "" || s.deploymentAccessAssignments == nil || s.deploymentAccessMemberships == nil || s.organizationMemberships == nil {
		return &adminv1.GetDeploymentAccessResponse{
			Status: "not_configured", Message: "WorkOS fine-grained access inspection is not configured.",
			Permissions: permissions,
		}, nil
	}

	memberships, err := s.listAllOrganizationMemberships(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("list organization memberships for access inspection: %w", err)
	}
	resource := authz.DeploymentResource(dep.ID)
	roleAssignments, permissionsByMembership, err := s.deploymentAccessEvidence(ctx, organizationID, resource)
	if errors.Is(err, authz.ErrResourceNotFound) {
		return &adminv1.GetDeploymentAccessResponse{
			Status: "not_registered", Message: "This deployment does not have a WorkOS authorization resource.",
			Permissions: permissions,
		}, nil
	}
	if err != nil {
		return nil, err
	}
	emails, err := s.memberEmails(ctx, dep.AccountID)
	if err != nil {
		return nil, err
	}

	members := make([]*adminv1.AdminDeploymentAccessMember, 0, len(memberships))
	for _, membership := range memberships {
		assignments := roleAssignments[membership.ID]
		memberPermissions := orderedPermissions(permissionsByMembership[membership.ID])
		member := &adminv1.AdminDeploymentAccessMember{
			UserID:            membership.UserID,
			Email:             emails[membership.UserID],
			MembershipStatus:  membership.Status,
			OrganizationRoles: orderedOrganizationRoles(membership.RoleSlugs),
			DeploymentRoles:   orderedDeploymentRoles(assignments),
			Permissions:       memberPermissions,
			Sources:           accessSources(membership.RoleSlugs, assignments, memberPermissions),
		}
		members = append(members, member)
	}
	sort.SliceStable(members, func(i, j int) bool {
		left, right := accessMemberRank(members[i]), accessMemberRank(members[j])
		if left != right {
			return left < right
		}
		return strings.ToLower(memberLabel(members[i])) < strings.ToLower(memberLabel(members[j]))
	})

	return &adminv1.GetDeploymentAccessResponse{
		Status: "available", Permissions: permissions, Members: members,
	}, nil
}

func (s *Server) listAllOrganizationMemberships(ctx context.Context, organizationID string) ([]org.Membership, error) {
	var memberships []org.Membership
	after := ""
	seen := map[string]struct{}{}
	for {
		page, err := s.organizationMemberships.ListMembershipsPage(ctx, organizationID, org.ListOpts{
			Limit: organizationMembershipPageSize, After: after,
		})
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, page.Memberships...)
		if page.NextCursor == "" {
			return memberships, nil
		}
		if _, duplicate := seen[page.NextCursor]; duplicate {
			return nil, errors.New("WorkOS membership pagination repeated a cursor")
		}
		seen[page.NextCursor] = struct{}{}
		after = page.NextCursor
	}
}

func (s *Server) deploymentAccessEvidence(
	ctx context.Context,
	organizationID string,
	resource authz.ResourceRef,
) (map[string][]authz.RoleAssignment, map[string]map[authz.Action]struct{}, error) {
	rolesByMembership := make(map[string][]authz.RoleAssignment)
	permissionsByMembership := make(map[string]map[authz.Action]struct{})
	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(len(authz.DeploymentActions()) + 1)
	group.Go(func() error {
		assignments, err := s.deploymentAccessAssignments.ListRoleAssignments(groupCtx, organizationID, resource)
		if err != nil {
			return fmt.Errorf("list deployment role assignments: %w", err)
		}
		for _, assignment := range assignments {
			rolesByMembership[assignment.Subject.ID] = append(rolesByMembership[assignment.Subject.ID], assignment)
		}
		return nil
	})
	for _, action := range authz.DeploymentActions() {
		group.Go(func() error {
			membershipIDs, err := s.deploymentAccessMemberships.ListMemberships(groupCtx, organizationID, resource, action)
			if err != nil {
				return fmt.Errorf("list memberships with %s: %w", action, err)
			}
			mu.Lock()
			defer mu.Unlock()
			for _, membershipID := range membershipIDs {
				if permissionsByMembership[membershipID] == nil {
					permissionsByMembership[membershipID] = make(map[authz.Action]struct{})
				}
				permissionsByMembership[membershipID][action] = struct{}{}
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, nil, err
	}
	return rolesByMembership, permissionsByMembership, nil
}

func (s *Server) memberEmails(ctx context.Context, accountID string) (map[string]string, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT am.user_id, COALESCE((
			SELECT e.email FROM account_member_emails e
			WHERE e.user_id = am.user_id
			ORDER BY e.verified DESC, e.updated_at DESC
			LIMIT 1
		), '')
		FROM account_members am
		WHERE am.account_id = $1
	`, accountID)
	if err != nil {
		return nil, fmt.Errorf("list deployment member emails: %w", err)
	}
	defer rows.Close() //nolint:errcheck
	emails := make(map[string]string)
	for rows.Next() {
		var userID, email string
		if err := rows.Scan(&userID, &email); err != nil {
			return nil, fmt.Errorf("scan deployment member email: %w", err)
		}
		emails[userID] = email
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("deployment member email rows: %w", err)
	}
	return emails, nil
}

func actionStrings(actions []authz.Action) []string {
	result := make([]string, len(actions))
	for i, action := range actions {
		result[i] = string(action)
	}
	return result
}

func orderedPermissions(found map[authz.Action]struct{}) []string {
	result := make([]string, 0, len(found))
	for _, action := range authz.DeploymentActions() {
		if _, ok := found[action]; ok {
			result = append(result, string(action))
		}
	}
	return result
}

func orderedOrganizationRoles(roles []string) []string {
	return sortedUnique(roles, func(role string) int {
		switch role {
		case "owner":
			return 0
		case "admin":
			return 1
		default:
			return 2
		}
	})
}

func orderedDeploymentRoles(assignments []authz.RoleAssignment) []string {
	roles := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		roles = append(roles, string(assignment.Role))
	}
	return sortedUnique(roles, deploymentRoleRank)
}

func accessSources(organizationRoles []string, assignments []authz.RoleAssignment, permissions []string) []string {
	sources := map[string]struct{}{}
	for _, assignment := range assignments {
		switch assignment.Source {
		case authz.AssignmentSourceGroup:
			sources["group"] = struct{}{}
		default:
			sources["direct"] = struct{}{}
		}
	}
	if len(permissions) > 0 && (len(assignments) == 0 || containsRole(organizationRoles, "owner") || containsRole(organizationRoles, "admin")) {
		sources["organization"] = struct{}{}
	}
	ordered := make([]string, 0, len(sources))
	for _, source := range []string{"organization", "direct", "group"} {
		if _, ok := sources[source]; ok {
			ordered = append(ordered, source)
		}
	}
	return ordered
}

func accessMemberRank(member *adminv1.AdminDeploymentAccessMember) int {
	for _, role := range member.OrganizationRoles {
		if role == "owner" {
			return 0
		}
		if role == "admin" {
			return 1
		}
	}
	for _, role := range member.DeploymentRoles {
		if rank := deploymentRoleRank(role); rank < 4 {
			return rank + 2
		}
	}
	if len(member.Permissions) > 0 {
		return 6
	}
	return 7
}

func memberLabel(member *adminv1.AdminDeploymentAccessMember) string {
	if member.Email != "" {
		return member.Email
	}
	return member.UserID
}

func deploymentRoleRank(role string) int {
	level, ok := authz.AccessLevelForRole(authz.ResourceDeployment, authz.RoleSlug(role))
	if !ok {
		return 4
	}
	switch level {
	case authz.AccessLevelAdmin:
		return 0
	case authz.AccessLevelMaintainer:
		return 1
	case authz.AccessLevelWriter:
		return 2
	case authz.AccessLevelViewer:
		return 3
	default:
		return 4
	}
}

func sortedUnique(values []string, rank func(string) int) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool {
		left, right := rank(result[i]), rank(result[j])
		if left != right {
			return left < right
		}
		return result[i] < result[j]
	})
	return result
}

func containsRole(roles []string, wanted string) bool {
	for _, role := range roles {
		if role == wanted {
			return true
		}
	}
	return false
}
