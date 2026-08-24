package authz

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sort"

	"github.com/astropods/astro/apps/astro-server/internal/account"
)

var (
	ErrAccessManagementUnavailable = errors.New("resource access management is unavailable")
	ErrAccessSubjectNotProvisioned = errors.New("access subject is not provisioned")
)

type AccessAssignment struct {
	ID                    string
	UserID                string
	Level                 AccessLevel
	Role                  RoleSlug
	Source                AssignmentSource
	GroupRoleAssignmentID string
}

type accessMemberStore interface {
	GetMemberContext(ctx context.Context, accountID, userID string) (*account.AccountMember, error)
	GetMembersByWorkosMembershipIDsContext(ctx context.Context, membershipIDs []string) (map[string]*account.AccountMember, error)
}

type AccessService struct {
	assignments   AccessAssignments
	gate          ResourceGate
	accounts      AccountResolver
	organizations OrganizationResolver
	members       accessMemberStore
	groups        Groups
	intents       accessIntentStore
}

func NewAccessService(
	assignments AccessAssignments,
	gate ResourceGate,
	accounts AccountResolver,
	organizations OrganizationResolver,
	members accessMemberStore,
	groups Groups,
	intents accessIntentStore,
) *AccessService {
	return &AccessService{
		assignments: assignments, gate: gate,
		accounts: accounts, organizations: organizations, members: members, groups: groups,
		intents: intents,
	}
}

func (s *AccessService) ListAccess(ctx context.Context, resource ResourceRef) ([]AccessAssignment, []AccessIntent, error) {
	accountID, organizationID, err := s.resourceScope(ctx, resource)
	if err != nil {
		return nil, nil, err
	}
	assignments, err := s.listAssignments(ctx, resource, accountID, organizationID)
	if err != nil {
		return nil, nil, err
	}
	intents, err := s.listIntents(ctx, resource, accountID)
	if err != nil {
		return nil, nil, err
	}
	return assignments, intents, nil
}

func (s *AccessService) listAssignments(
	ctx context.Context,
	resource ResourceRef,
	accountID, organizationID string,
) ([]AccessAssignment, error) {
	assignments, err := s.assignments.ListRoleAssignments(ctx, organizationID, resource)
	if err != nil {
		return nil, fmt.Errorf("list resource assignments: %w", err)
	}

	type supportedAssignment struct {
		assignment RoleAssignment
		level      AccessLevel
	}
	supported := make([]supportedAssignment, 0, len(assignments))
	membershipIDs := make([]string, 0, len(assignments))
	seenMembershipIDs := make(map[string]struct{})
	for _, assignment := range assignments {
		level, builtIn := AccessLevelForRole(resource.Type, assignment.Role)
		if !builtIn {
			slog.WarnContext(ctx, "skipping unsupported access assignment role",
				"assignment_id", assignment.ID, "role", assignment.Role,
				"resource_type", resource.Type, "resource_id", resource.ExternalID)
			continue
		}
		if assignment.Subject.Type != AssignmentSubjectMembership {
			slog.WarnContext(ctx, "skipping unsupported access assignment subject",
				"assignment_id", assignment.ID, "subject_type", assignment.Subject.Type, "subject_id", assignment.Subject.ID)
			continue
		}
		supported = append(supported, supportedAssignment{assignment: assignment, level: level})
		if _, seen := seenMembershipIDs[assignment.Subject.ID]; !seen {
			seenMembershipIDs[assignment.Subject.ID] = struct{}{}
			membershipIDs = append(membershipIDs, assignment.Subject.ID)
		}
	}
	membersByMembershipID := make(map[string]*account.AccountMember, len(membershipIDs))
	if len(membershipIDs) > 0 {
		membersByMembershipID, err = s.members.GetMembersByWorkosMembershipIDsContext(ctx, membershipIDs)
		if err != nil {
			return nil, fmt.Errorf("resolve access assignment memberships: %w", err)
		}
	}

	result := make([]AccessAssignment, 0, len(supported))
	for _, current := range supported {
		assignment := current.assignment
		member := membersByMembershipID[assignment.Subject.ID]
		if member == nil || member.AccountID != accountID {
			resolvedAccountID := ""
			if member != nil {
				resolvedAccountID = member.AccountID
			}
			slog.WarnContext(ctx, "skipping unresolved access assignment membership",
				"assignment_id", assignment.ID, "membership_id", assignment.Subject.ID,
				"account_id", accountID, "resolved_account_id", resolvedAccountID)
			continue
		}
		result = append(result, AccessAssignment{
			ID: assignment.ID, UserID: member.UserID,
			Level: current.level, Role: assignment.Role, Source: assignment.Source,
			GroupRoleAssignmentID: assignment.GroupRoleAssignmentID,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].UserID != result[j].UserID {
			return result[i].UserID < result[j].UserID
		}
		if result[i].Source != result[j].Source {
			return result[i].Source < result[j].Source
		}
		return result[i].Role < result[j].Role
	})
	return result, nil
}

func (s *AccessService) Assign(
	ctx context.Context,
	resource ResourceRef,
	subjectType AssignmentSubjectType,
	subjectID string,
	level AccessLevel,
) (AccessIntent, bool, error) {
	accountID, organizationID, err := s.resourceScope(ctx, resource)
	if err != nil {
		return AccessIntent{}, false, err
	}
	role, err := RoleForAccessLevel(resource.Type, level)
	if err != nil {
		return AccessIntent{}, false, err
	}
	return s.recordIntent(ctx, accountID, organizationID, resource, subjectType, subjectID, role)
}

func (s *AccessService) Remove(
	ctx context.Context,
	resource ResourceRef,
	subjectType AssignmentSubjectType,
	subjectID string,
) (AccessIntent, bool, error) {
	accountID, organizationID, err := s.resourceScope(ctx, resource)
	if err != nil {
		return AccessIntent{}, false, err
	}
	return s.recordIntent(ctx, accountID, organizationID, resource, subjectType, subjectID, "")
}

func (s *AccessService) listIntents(ctx context.Context, resource ResourceRef, accountID string) ([]AccessIntent, error) {
	if s.intents == nil {
		return nil, errors.New("resource access intent store is not configured")
	}
	return s.intents.ListForResource(ctx, accountID, resource)
}

func (s *AccessService) recordIntent(
	ctx context.Context,
	accountID, organizationID string,
	resource ResourceRef,
	subjectType AssignmentSubjectType,
	subjectID string,
	desiredRole RoleSlug,
) (AccessIntent, bool, error) {
	if s.intents == nil {
		return AccessIntent{}, false, errors.New("resource access intent store is not configured")
	}
	subject, err := s.resolveSubject(ctx, accountID, organizationID, subjectType, subjectID)
	if err != nil {
		return AccessIntent{}, false, err
	}
	return s.intents.Record(ctx, AccessIntent{
		AccountID: accountID, OrganizationID: organizationID, Resource: resource,
		Subject: subject, SubjectID: subjectID, DesiredRole: desiredRole,
	})
}

func (s *AccessService) resourceScope(ctx context.Context, resource ResourceRef) (string, string, error) {
	if s == nil || s.assignments == nil || s.gate == nil {
		return "", "", ErrAccessManagementUnavailable
	}
	if s.accounts == nil || s.organizations == nil || s.members == nil || s.intents == nil {
		return "", "", errors.New("access service is not configured")
	}
	enabled, err := s.gate.Enabled(ctx, resource)
	if err != nil {
		return "", "", fmt.Errorf("resolve access-management rollout: %w", err)
	}
	if !enabled {
		return "", "", ErrAccessManagementUnavailable
	}
	accountID, personal, err := s.accounts.AccountForResource(ctx, resource)
	if err != nil {
		return "", "", err
	}
	organizationID, organizationPersonal, err := s.organizations.OrganizationForResource(ctx, resource)
	if err != nil {
		return "", "", err
	}
	if personal || organizationPersonal || accountID == "" || organizationID == "" {
		return "", "", ErrAccessManagementUnavailable
	}
	return accountID, organizationID, nil
}

func (s *AccessService) resolveSubject(
	ctx context.Context,
	accountID, organizationID string,
	subjectType AssignmentSubjectType,
	subjectID string,
) (AssignmentSubject, error) {
	if subjectID == "" {
		return AssignmentSubject{}, errors.New("assignment subject id is required")
	}
	switch subjectType {
	case AssignmentSubjectMembership:
		member, err := s.members.GetMemberContext(ctx, accountID, subjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return AssignmentSubject{}, ErrResourceNotVisible
			}
			return AssignmentSubject{}, fmt.Errorf("resolve account member: %w", err)
		}
		if member == nil {
			return AssignmentSubject{}, ErrResourceNotVisible
		}
		if member.WorkOSMembershipID == "" {
			return AssignmentSubject{}, ErrAccessSubjectNotProvisioned
		}
		return MembershipAssignmentSubject(member.WorkOSMembershipID), nil
	case AssignmentSubjectGroup:
		if s.groups == nil {
			return AssignmentSubject{}, ErrAccessManagementUnavailable
		}
		group, err := s.groups.GetGroup(ctx, organizationID, subjectID)
		if errors.Is(err, ErrGroupNotFound) {
			return AssignmentSubject{}, ErrResourceNotVisible
		}
		if err != nil {
			return AssignmentSubject{}, fmt.Errorf("resolve assignment group: %w", err)
		}
		if group.ID != subjectID || group.OrganizationID != organizationID {
			return AssignmentSubject{}, ErrResourceNotVisible
		}
		return GroupAssignmentSubject(group.ID), nil
	default:
		return AssignmentSubject{}, fmt.Errorf("unsupported assignment subject type %q", subjectType)
	}
}

func directBuiltInAssignments(assignments []RoleAssignment, subject AssignmentSubject, resource ResourceRef) []RoleAssignment {
	result := make([]RoleAssignment, 0, len(assignments))
	for _, assignment := range assignments {
		if assignment.Subject != subject || assignment.Resource != resource || assignment.Source != AssignmentSourceDirect {
			continue
		}
		if _, builtIn := AccessLevelForRole(resource.Type, assignment.Role); builtIn {
			result = append(result, assignment)
		}
	}
	return result
}
