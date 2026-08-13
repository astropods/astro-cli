package authz

import (
	"context"
	"errors"
)

type DecisionLogger interface {
	Debug(msg string, args ...any)
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
}

// ShadowChecker returns the primary decision while observing a second checker.
// Shadow failures and mismatches never change the enforced result.
type ShadowChecker struct {
	log     DecisionLogger
	primary Checker
	shadow  Checker
}

type gatedShadowChecker struct {
	log        DecisionLogger
	gate       ResourceGate
	comparison Checker
}

func NewShadowChecker(log DecisionLogger, primary, shadow Checker) *ShadowChecker {
	return &ShadowChecker{log: log, primary: primary, shadow: shadow}
}

// NewGatedShadowChecker skips the comparison when a resource is outside the rollout.
func NewGatedShadowChecker(log DecisionLogger, gate ResourceGate, primary, shadow Checker) Checker {
	return &gatedShadowChecker{
		log:        log,
		gate:       gate,
		comparison: NewShadowChecker(log, primary, shadow),
	}
}

func (c *gatedShadowChecker) Authorize(ctx context.Context, subject Subject, action Action, resource ResourceRef) (bool, error) {
	enabled, err := c.gate.Enabled(ctx, resource)
	if err != nil {
		return false, err
	}
	if !enabled {
		c.log.Debug("FGA shadow check skipped",
			"route", authorizationRoute(ctx),
			"action", action,
			"resource_type", resource.Type,
			"resource_id", resource.ExternalID,
			"user_id", subject.UserID,
		)
		return false, nil
	}
	return c.comparison.Authorize(ctx, subject, action, resource)
}

func (c *ShadowChecker) Authorize(ctx context.Context, subject Subject, action Action, resource ResourceRef) (bool, error) {
	primaryAllowed, err := c.primary.Authorize(ctx, subject, action, resource)
	if err != nil {
		return false, err
	}

	shadowAllowed, shadowErr := c.shadow.Authorize(ctx, subject, action, resource)
	logDecisionComparison(c.log, ctx, subject, action, resource, primaryAllowed, shadowAllowed, shadowErr)
	return primaryAllowed, nil
}

func logDecisionComparison(
	log DecisionLogger,
	ctx context.Context,
	subject Subject,
	action Action,
	resource ResourceRef,
	primaryAllowed bool,
	shadowAllowed bool,
	shadowErr error,
) {
	attrs := []any{
		"route", authorizationRoute(ctx),
		"action", action,
		"resource_type", resource.Type,
		"resource_id", resource.ExternalID,
		"user_id", subject.UserID,
		"membership_id", subject.MembershipID,
		"organization_id", subject.OrgID,
		"membership_allowed", primaryAllowed,
	}
	if shadowErr != nil {
		attrs = append(attrs, "error", shadowErr)
		switch {
		case errors.Is(shadowErr, ErrFGAResourceNotEnabled):
			log.Debug("FGA shadow check skipped", attrs...)
		case errors.Is(shadowErr, ErrWorkOSMembershipUnavailable):
			log.Debug("FGA shadow identity unavailable", attrs...)
		default:
			log.Warn("FGA shadow check failed", attrs...)
		}
		return
	}

	attrs = append(attrs, "fga_allowed", shadowAllowed)
	if primaryAllowed != shadowAllowed {
		log.Warn("FGA shadow decision mismatch", attrs...)
	} else {
		log.Info("FGA shadow decision matched", attrs...)
	}
}

var _ Checker = (*ShadowChecker)(nil)
var _ Checker = (*gatedShadowChecker)(nil)
