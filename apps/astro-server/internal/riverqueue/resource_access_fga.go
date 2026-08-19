package riverqueue

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

const resourceAccessFGASweepLimit = 100

// ResourceAccessFGAReconcileArgs identifies one resource, or a due-work sweep
// when ResourceID is empty.
type ResourceAccessFGAReconcileArgs struct {
	OrganizationID string `json:"organization_id,omitempty"`
	ResourceType   string `json:"resource_type,omitempty"`
	ResourceID     string `json:"resource_id,omitempty"`
}

func (ResourceAccessFGAReconcileArgs) Kind() string { return "resource_access.fga_reconcile" }

func (ResourceAccessFGAReconcileArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueMaintenance,
		MaxAttempts: 10,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateRetryable,
				rivertype.JobStateScheduled,
			},
		},
	}
}

func init() {
	registerJobKind[ResourceAccessFGAReconcileArgs]()
}

type resourceAccessFGAQueue interface {
	InsertResourceAccessFGAReconcileJob(context.Context, authz.AccessIntentKey) error
}

type ResourceAccessFGAReconcileWorker struct {
	river.WorkerDefaults[ResourceAccessFGAReconcileArgs]
	reconciler *authz.AccessReconciler
	store      *authz.ResourceAccessSyncStore
	queue      resourceAccessFGAQueue
	log        *logger.Logger
}

func (w *ResourceAccessFGAReconcileWorker) Work(ctx context.Context, job *river.Job[ResourceAccessFGAReconcileArgs]) error {
	if w.store == nil || w.reconciler == nil {
		return errors.New("resource access FGA reconciliation is not configured")
	}
	if job.Args.ResourceID != "" {
		resource := authz.ResourceRef{Type: authz.ResourceType(job.Args.ResourceType), ExternalID: job.Args.ResourceID}
		synced, err := w.reconciler.ReconcileResource(ctx, job.Args.OrganizationID, resource)
		if err != nil || synced {
			return err
		}
		w.log.Debug("Resource access intents changed repeatedly during reconciliation",
			"resource_type", job.Args.ResourceType,
			"resource_id", job.Args.ResourceID,
		)
		return nil
	}

	keys, err := w.store.Due(ctx, resourceAccessFGASweepLimit)
	if err != nil {
		return err
	}
	resources := make(map[authz.AccessIntentKey]struct{}, len(keys))
	for _, key := range keys {
		key.Subject = authz.AssignmentSubject{}
		resources[key] = struct{}{}
	}
	var enqueueErr error
	for key := range resources {
		if w.queue == nil {
			return errors.New("resource access reconciliation queue is not configured")
		}
		if err := w.queue.InsertResourceAccessFGAReconcileJob(ctx, key); err != nil {
			enqueueErr = errors.Join(enqueueErr, fmt.Errorf("enqueue %s:%s: %w", key.Resource.Type, key.Resource.ExternalID, err))
		}
	}
	return enqueueErr
}
