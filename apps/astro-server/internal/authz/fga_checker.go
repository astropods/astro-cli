package authz

import (
	"context"
	"errors"
	"fmt"
)

// ErrWorkOSMembershipUnavailable means an organization session cannot yet make
// a resource-scoped FGA decision. Refresh, switch-org, or re-login repairs it.
var ErrWorkOSMembershipUnavailable = errors.New("workos organization membership id is unavailable")

// ErrFGAResourceNotEnabled means the resource stays on legacy authorization
// during a scoped rollout; it is not an access denial.
var ErrFGAResourceNotEnabled = errors.New("fga is not enabled for resource")

// ResourceGate scopes incremental rollout before a live vendor check runs.
type ResourceGate interface {
	Enabled(context.Context, ResourceRef) (bool, error)
}

// OrganizationResolver returns the WorkOS organization that owns a resource.
type OrganizationResolver interface {
	OrganizationForResource(context.Context, ResourceRef) (organizationID string, personal bool, err error)
}

type conditionalResourceGate struct {
	active bool
	next   ResourceGate
}

// NewConditionalResourceGate layers an environment kill switch over a
// resource-scoped rollout gate.
func NewConditionalResourceGate(active bool, next ResourceGate) ResourceGate {
	return &conditionalResourceGate{active: active, next: next}
}

func (g *conditionalResourceGate) Enabled(ctx context.Context, resource ResourceRef) (bool, error) {
	if !g.active {
		return false, nil
	}
	return g.next.Enabled(ctx, resource)
}

// FGAChecker makes live resource-scoped decisions through WorkOS after the
// rollout gate excludes resources that must remain on legacy authorization.
type FGAChecker struct {
	fga           FGA
	gate          ResourceGate
	organizations OrganizationResolver
}

func NewFGAChecker(fga FGA, gate ResourceGate, organizations OrganizationResolver) *FGAChecker {
	return &FGAChecker{fga: fga, gate: gate, organizations: organizations}
}

func (c *FGAChecker) Authorize(ctx context.Context, subject Subject, action Action, resource ResourceRef) (bool, error) {
	if err := c.validateRequest(ctx, subject, resource); err != nil {
		if errors.Is(err, ErrResourceNotVisible) {
			return false, nil
		}
		return false, err
	}
	return c.fga.Check(ctx, subject.MembershipID, action, resource)
}

func (c *FGAChecker) EffectivePermissions(ctx context.Context, subject Subject, resource ResourceRef) ([]Action, error) {
	if err := c.validateRequest(ctx, subject, resource); err != nil {
		return nil, err
	}
	return c.fga.ListEffectivePermissions(ctx, subject.MembershipID, resource)
}

func (c *FGAChecker) validateRequest(ctx context.Context, subject Subject, resource ResourceRef) error {
	enabled, err := c.gate.Enabled(ctx, resource)
	if err != nil {
		return fmt.Errorf("resolve FGA rollout eligibility: %w", err)
	}
	if !enabled {
		return ErrFGAResourceNotEnabled
	}
	if c.organizations != nil {
		organizationID, personal, resolveErr := c.organizations.OrganizationForResource(ctx, resource)
		if resolveErr != nil {
			return fmt.Errorf("resolve FGA resource organization: %w", resolveErr)
		}
		if !personal && (organizationID == "" || !SessionOrgMatchesAccount(subject, "organization", organizationID)) {
			return ErrResourceNotVisible
		}
	}

	if subject.MembershipID == "" {
		return ErrWorkOSMembershipUnavailable
	}
	return nil
}

var _ Checker = (*FGAChecker)(nil)
