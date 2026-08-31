package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/eventstream"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

// Bounds how far behind a client can be and still catch up by replay. Past it
// the client refetches, so retention buys latency, not correctness.
const agentEventRetention = 7 * 24 * time.Hour

type AgentEventTrimArgs struct{}

func (AgentEventTrimArgs) Kind() string { return "agent_event.trim" }

func (AgentEventTrimArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance}
}

func init() {
	registerJobKind[AgentEventTrimArgs]()
}

type AgentEventTrimWorker struct {
	river.WorkerDefaults[AgentEventTrimArgs]
	events *eventstream.Store
	log    *logger.Logger
}

func (w *AgentEventTrimWorker) Work(ctx context.Context, _ *river.Job[AgentEventTrimArgs]) error {
	deleted, err := w.events.Trim(ctx, agentEventRetention)
	if err != nil {
		return err
	}
	if deleted > 0 {
		w.log.Info("agent event trim: deleted expired events", "rows", deleted)
	}
	return nil
}
