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

// FGAChecker makes live resource-scoped decisions through WorkOS after the
// rollout gate excludes resources that must remain on legacy authorization.
type FGAChecker struct {
	fga  FGA
	gate ResourceGate
}

func NewFGAChecker(fga FGA, gate ResourceGate) *FGAChecker {
	return &FGAChecker{fga: fga, gate: gate}
}

func (c *FGAChecker) Authorize(ctx context.Context, subject Subject, action Action, resource ResourceRef) (bool, error) {
	enabled, err := c.gate.Enabled(ctx, resource)
	if err != nil {
		return false, fmt.Errorf("resolve FGA rollout eligibility: %w", err)
	}
	if !enabled {
		return false, ErrFGAResourceNotEnabled
	}

	if subject.MembershipID == "" {
		return false, ErrWorkOSMembershipUnavailable
	}

	return c.fga.Check(ctx, subject.MembershipID, action, resource)
}

var _ Checker = (*FGAChecker)(nil)
