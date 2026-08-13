package authz

import (
	"context"
	"errors"
	"fmt"
)

// AccountExperimentGate reports whether an account opted into a server-owned experiment.
type AccountExperimentGate interface {
	Enabled(context.Context, string) (bool, error)
}

type accountExperimentResourceGate struct {
	resources  ResourceGate
	accounts   AccountResolver
	experiment AccountExperimentGate
}

// NewAccountExperimentResourceGate adds organization opt-in to an existing
// resource gate. Personal resources are never eligible.
func NewAccountExperimentResourceGate(
	resources ResourceGate,
	accounts AccountResolver,
	experiment AccountExperimentGate,
) ResourceGate {
	return &accountExperimentResourceGate{
		resources:  resources,
		accounts:   accounts,
		experiment: experiment,
	}
}

func (g *accountExperimentResourceGate) Enabled(ctx context.Context, resource ResourceRef) (bool, error) {
	if g == nil || g.resources == nil || g.accounts == nil || g.experiment == nil {
		return false, errors.New("account experiment resource gate is not configured")
	}

	ready, err := g.resources.Enabled(ctx, resource)
	if err != nil || !ready {
		return ready, err
	}
	accountID, personal, err := g.accounts.AccountForResource(ctx, resource)
	if err != nil {
		return false, fmt.Errorf("resolve experiment account: %w", err)
	}
	if personal {
		return false, nil
	}
	enabled, err := g.experiment.Enabled(ctx, accountID)
	if err != nil {
		return false, fmt.Errorf("resolve account experiment: %w", err)
	}
	return enabled, nil
}
