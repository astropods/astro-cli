package riverqueue

import (
	"context"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/observation"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

// ObservationSweepArgs triggers one pass of the observation evaluator.
type ObservationSweepArgs struct{}

func (ObservationSweepArgs) Kind() string { return "observation.sweep" }

func (ObservationSweepArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueInsights}
}

func init() { registerJobKind[ObservationSweepArgs]() }

// ObservationSweepWorker evaluates resource/health conditions against metrics
// and emits alerts on firing edges. The queue reference is wired
// post-construction (see wiredWorkers); the worker is a no-op without it.
type ObservationSweepWorker struct {
	river.WorkerDefaults[ObservationSweepArgs]
	prom    *promquery.Client
	deploys *deploymentstore.Store
	state   *observation.Store
	queue   *Queue
	log     *logger.Logger
}

func (w *ObservationSweepWorker) Work(ctx context.Context, _ *river.Job[ObservationSweepArgs]) error {
	if w.prom == nil || w.queue == nil {
		return nil
	}
	engines := map[observation.Engine]observation.Querier{
		observation.EnginePromQL: observation.NewPromQLEngine(w.prom),
	}
	eval := observation.NewEvaluator(engines, w.deploys, w.state, w.queue.EmitNotify, w.log)
	return eval.Sweep(ctx)
}
