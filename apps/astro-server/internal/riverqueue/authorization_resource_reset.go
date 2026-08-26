package riverqueue

import (
	"context"
	"errors"

	"github.com/riverqueue/river"

	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
)

type AuthorizationResourceResetArgs struct {
	OperationID string `json:"operation_id"`
}

func (AuthorizationResourceResetArgs) Kind() string { return "authorization.resource_reset" }

func (AuthorizationResourceResetArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{Queue: queueMaintenance, MaxAttempts: 10}
}

func init() {
	registerJobKind[AuthorizationResourceResetArgs]()
}

type AuthorizationResourceResetWorker struct {
	river.WorkerDefaults[AuthorizationResourceResetArgs]
	service *authorizationadmin.Service
}

func (w *AuthorizationResourceResetWorker) Work(ctx context.Context, job *river.Job[AuthorizationResourceResetArgs]) error {
	if w.service == nil {
		return errors.New("authorization resource reset service is not configured")
	}
	if job.Args.OperationID == "" {
		return errors.New("authorization operation id is required")
	}
	return w.service.RunReset(ctx, job.Args.OperationID)
}
