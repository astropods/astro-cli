package deployeval

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/deployer"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// TenantRouterIngressEvaluatorID is the stable id for the tenant-router
// ingress-drift evaluator, persisted in deployment_evaluator_state.
const TenantRouterIngressEvaluatorID = "tenant-router-ingress"

func init() {
	Register(func(d Deps) Evaluator { return NewTenantRouterIngressEvaluator(d.Deployer) })
}

type tenantRouterIngressEvaluator struct {
	deployer *deployer.Deployer
}

// NewTenantRouterIngressEvaluator checks whether a deployment's Ingress
// objects reflect the tenant-router migration (ingress class, host rules its
// current spec wants) and fixes them by re-applying just the Ingress
// objects. See docs/plans/tenant-router-migration.md in astro-infra.
func NewTenantRouterIngressEvaluator(d *deployer.Deployer) Evaluator {
	return &tenantRouterIngressEvaluator{deployer: d}
}

func (e *tenantRouterIngressEvaluator) ID() string   { return TenantRouterIngressEvaluatorID }
func (e *tenantRouterIngressEvaluator) Name() string { return "Tenant-router ingress" }
func (e *tenantRouterIngressEvaluator) Description() string {
	return "Checks whether a deployment's Ingress objects use the tenant-router class and the host rules its current spec wants. A deployment redeployed since the tenant-router migration landed already matches; an older one needs a fix to pick it up."
}

func (e *tenantRouterIngressEvaluator) Check(ctx context.Context, dep *deploymentstore.Deployment) (CheckResult, error) {
	drifted, detail, err := e.deployer.CheckIngressDrift(ctx, dep)
	if err != nil {
		return CheckResult{}, err
	}
	return CheckResult{Drifted: drifted, Detail: detail}, nil
}

func (e *tenantRouterIngressEvaluator) Fix(ctx context.Context, dep *deploymentstore.Deployment) error {
	_, err := e.deployer.SyncIngresses(ctx, dep)
	return err
}
