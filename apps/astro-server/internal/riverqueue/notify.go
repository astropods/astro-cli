package riverqueue

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/novu"
)

// NotifyArgs carries one alert to deliver off the request path. The full
// notify.Event rides along; recipient resolution and the Novu trigger happen in
// the worker so every send is retried as a River job. Idempotency is handled at
// Novu via the event's transaction id, so no River-level uniqueness is set —
// distinct emits (e.g. repeated "Send test" clicks) must each deliver.
type NotifyArgs struct {
	Event notify.Event `json:"event"`
}

func (NotifyArgs) Kind() string { return "notify.deliver" }

func (NotifyArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueNotifications}
}

func init() {
	registerJobKind[NotifyArgs]()
}

// NotifyWorker resolves recipients and triggers the workflow for an emitted
// event. deliverer is nil only when notifications are not wired (worker then
// no-ops); New() always constructs one, falling back to the no-op provider.
type NotifyWorker struct {
	river.WorkerDefaults[NotifyArgs]
	deliverer *notify.Deliverer
	log       *logger.Logger
}

func (w *NotifyWorker) Work(ctx context.Context, job *river.Job[NotifyArgs]) error {
	if w.deliverer == nil {
		if w.log != nil {
			w.log.Warn("notify: worker has no deliverer, dropping", "type", job.Args.Event.Type)
		}
		return nil
	}
	err := w.deliverer.Deliver(ctx, job.Args.Event)
	if err != nil && permanentDeliveryError(err) {
		// A 4xx (e.g. unknown workflow, bad payload) will not fix itself on
		// retry — cancel the job instead of hammering Novu until attempts run out.
		if w.log != nil {
			w.log.Error("notify: permanent delivery failure, not retrying",
				"type", job.Args.Event.Type, "error", err)
		}
		return river.JobCancel(err)
	}
	return err
}

// permanentDeliveryError reports whether a Novu error is a client error that a
// retry cannot fix. 408 (timeout) and 429 (rate limited) are transient.
func permanentDeliveryError(err error) bool {
	var apiErr *novu.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	s := apiErr.StatusCode
	return s >= 400 && s < 500 && s != 408 && s != 429
}
