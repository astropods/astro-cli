// Package deployeval declaratively checks running deployments for
// configuration drift and fixes it on demand. An Evaluator names one check
// (e.g. "does this deployment's Ingress reflect the current routing shape")
// alongside the fix for it; Store persists the last check result per
// (deployment, evaluator) so astro-queen can list what's drifted without
// re-checking the whole fleet on every page load.
//
// To add a new evaluator for a future migration: implement Evaluator in a
// new file and register a Factory for it from that file's init(), e.g.
//
//	func init() { Register(func(d Deps) Evaluator { return NewFoo(d.Deployer) }) }
//
// See tenant_router_ingress.go for a complete example. No other file needs
// to change — admingrpc and astro-queen already list, sweep, and fix
// whatever BuildAll returns.
package deployeval

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// CheckResult is the outcome of running an Evaluator's Check against one
// deployment.
type CheckResult struct {
	// Drifted reports whether the deployment fails this evaluator's check.
	Drifted bool
	// Detail is a short human-readable reason, shown in the admin UI. Empty
	// when Drifted is false.
	Detail string
}

// Evaluator names one configuration-drift check and its fix. Implementations
// must be safe to run concurrently across many deployments and must not
// mutate anything in Check.
type Evaluator interface {
	// ID is a stable, unique slug persisted in the tracking table. Never
	// rename an existing evaluator's ID — it would orphan its history.
	ID() string
	// Name is the operator-facing label shown in astro-queen.
	Name() string
	// Description explains what the check verifies and why it matters.
	Description() string
	// Check reports whether dep currently drifts from this evaluator's
	// desired state. Read-only.
	Check(ctx context.Context, dep *deploymentstore.Deployment) (CheckResult, error)
	// Fix corrects dep so it no longer drifts. Idempotent: safe to call on a
	// deployment that isn't actually drifted.
	Fix(ctx context.Context, dep *deploymentstore.Deployment) error
}
