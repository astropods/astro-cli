package riverqueue

import (
	"context"
	"errors"
	"time"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
)

const authorizationResourceBackfillTimeout = 30 * time.Minute

type AuthorizationResourceBackfillArgs struct {
	OperationID string `json:"operation_id"`
}

func (AuthorizationResourceBackfillArgs) Kind() string { return "authorization.resource_backfill" }

func (AuthorizationResourceBackfillArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance, MaxAttempts: 1}
}

func init() {
	registerJobKind[AuthorizationResourceBackfillArgs]()
}

type AuthorizationResourceBackfillWorker struct {
	river.WorkerDefaults[AuthorizationResourceBackfillArgs]
	service *authorizationadmin.Service
}

func (w *AuthorizationResourceBackfillWorker) Timeout(*river.Job[AuthorizationResourceBackfillArgs]) time.Duration {
	return authorizationResourceBackfillTimeout
}

func (w *AuthorizationResourceBackfillWorker) Work(ctx context.Context, job *river.Job[AuthorizationResourceBackfillArgs]) error {
	if w.service == nil {
		return errors.New("authorization resource backfill service is not configured")
	}
	if job.Args.OperationID == "" {
		return errors.New("authorization operation id is required")
	}
	return w.service.RunBackfill(ctx, job.Args.OperationID)
}
