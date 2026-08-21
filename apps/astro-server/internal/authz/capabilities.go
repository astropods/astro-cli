package authz

import (
	"context"
	"errors"
	"fmt"
)

var ErrResourceNotVisible = errors.New("resource is not visible to subject")

type CapabilityMode string

const (
	CapabilityModeLegacy CapabilityMode = "legacy"
	CapabilityModeFGA    CapabilityMode = "fga"
)

type CapabilitySet struct {
	Mode    CapabilityMode
	Actions map[Action]bool
}

type EffectivePermissionLister interface {
	EffectivePermissions(context.Context, Subject, ResourceRef) ([]Action, error)
}

// CapabilityService evaluates the complete resource action catalog in one
// server request. WorkOS decisions stay live and are not cached.
type CapabilityService struct {
	log         DecisionLogger
	gate        ResourceGate
	primary     Checker
	permissions EffectivePermissionLister
	visibility  Action
}

func NewCapabilityService(
	log DecisionLogger,
	gate ResourceGate,
	primary Checker,
	permissions EffectivePermissionLister,
	visibility Action,
) *CapabilityService {
	return &CapabilityService{
		log:         log,
		gate:        gate,
		primary:     primary,
		permissions: permissions,
		visibility:  visibility,
	}
}

func (s *CapabilityService) Evaluate(
	ctx context.Context,
	subject Subject,
	resource ResourceRef,
	actions []Action,
) (CapabilitySet, error) {
	if len(actions) == 0 {
		return CapabilitySet{}, errors.New("capability actions are required")
	}

	// Today's primary policy is account membership for every action. Evaluate its
	// baseline visibility action once before revealing resource capabilities.
	primaryAllowed, err := s.primary.Authorize(ctx, subject, s.visibility, resource)
	if err != nil {
		return CapabilitySet{}, err
	}
	if !primaryAllowed {
		return CapabilitySet{}, ErrResourceNotVisible
	}

	enabled, err := s.gate.Enabled(ctx, resource)
	if err != nil {
		return CapabilitySet{}, fmt.Errorf("resolve FGA rollout eligibility: %w", err)
	}
	if !enabled {
		legacy := make(map[Action]bool, len(actions))
		for _, action := range actions {
			legacy[action] = legacyCapability(primaryAllowed, action)
		}
		return CapabilitySet{Mode: CapabilityModeLegacy, Actions: legacy}, nil
	}
	if subject.MembershipID == "" {
		return CapabilitySet{}, ErrWorkOSMembershipUnavailable
	}

	effective, err := s.permissions.EffectivePermissions(ctx, subject, resource)
	if err != nil {
		return CapabilitySet{}, fmt.Errorf("list effective permissions: %w", err)
	}
	allowed := make(map[Action]struct{}, len(effective))
	for _, action := range effective {
		allowed[action] = struct{}{}
	}

	capabilities := make(map[Action]bool, len(actions))
	for _, action := range actions {
		_, permitted := allowed[action]
		capabilities[action] = permitted
		logCapabilityComparison(s.log, ctx, subject, action, resource, legacyCapability(primaryAllowed, action), permitted)
	}
	return CapabilitySet{Mode: CapabilityModeFGA, Actions: capabilities}, nil
}

func logCapabilityComparison(log DecisionLogger, ctx context.Context, subject Subject, action Action, resource ResourceRef, legacyAllowed, fgaAllowed bool) {
	log.Debug("capabilities: deployment capability comparison",
		"route", authorizationRoute(ctx),
		"action", action,
		"resource_type", resource.Type,
		"resource_id", resource.ExternalID,
		"user_id", subject.UserID,
		"membership_id", subject.MembershipID,
		"organization_id", subject.OrgID,
		"membership_allowed", legacyAllowed,
		"fga_allowed", fgaAllowed,
	)
}

func legacyCapability(memberAllowed bool, action Action) bool {
	// Access management has no legacy API or equivalent behavior.
	return memberAllowed && action != ActionDeploymentManageAccess
}
