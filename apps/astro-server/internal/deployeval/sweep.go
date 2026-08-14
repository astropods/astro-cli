package deployeval

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// SweepResult summarizes one RunSweep call for a single evaluator.
type SweepResult struct {
	CheckedCount int
	DriftedCount int
}

// activeLister is the subset of *deploymentstore.Store a sweep needs.
type activeLister interface {
	ListAllActive() ([]*deploymentstore.DeploymentWithAccount, error)
}

// RunSweep runs one evaluator's Check against every active deployment and
// upserts the result into store. A per-deployment Check failure is logged and
// skipped so one flaky check doesn't fail the whole sweep.
func RunSweep(ctx context.Context, ev Evaluator, deploys activeLister, store *Store, log *logger.Logger) (SweepResult, error) {
	all, err := deploys.ListAllActive()
	if err != nil {
		return SweepResult{}, fmt.Errorf("deployeval: list active deployments: %w", err)
	}

	var result SweepResult
	for _, dwa := range all {
		dep := dwa.Deployment
		res, err := ev.Check(ctx, &dep)
		if err != nil {
			if log != nil {
				log.Warn("deployeval: check failed", "evaluator", ev.ID(), "deployment", dep.ID, "error", err)
			}
			continue
		}
		result.CheckedCount++
		status := StatusOK
		if res.Drifted {
			status = StatusDrifted
			result.DriftedCount++
		}
		if err := store.Upsert(ctx, dep.ID, ev.ID(), status, res.Detail); err != nil {
			if log != nil {
				log.Warn("deployeval: upsert failed", "evaluator", ev.ID(), "deployment", dep.ID, "error", err)
			}
		}
	}
	return result, nil
}

// FixDeployment runs an evaluator's Fix for one deployment, re-checks, and
// persists the resulting status. It never blindly marks a deployment "ok"
// just because Fix returned no error — Fix succeeding is not the same as the
// drift actually being resolved, so the post-fix state comes from a real
// Check.
func FixDeployment(ctx context.Context, ev Evaluator, dep *deploymentstore.Deployment, store *Store) error {
	if err := ev.Fix(ctx, dep); err != nil {
		_ = store.UpsertAfterFix(ctx, dep.ID, ev.ID(), StatusFixFailed, err.Error())
		return err
	}
	res, err := ev.Check(ctx, dep)
	if err != nil {
		_ = store.UpsertAfterFix(ctx, dep.ID, ev.ID(), StatusFixFailed, fmt.Sprintf("fix applied but re-check failed: %v", err))
		return err
	}
	status := StatusOK
	if res.Drifted {
		status = StatusFixFailed
	}
	return store.UpsertAfterFix(ctx, dep.ID, ev.ID(), status, res.Detail)
}
