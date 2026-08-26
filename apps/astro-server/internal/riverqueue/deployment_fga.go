package riverqueue

import (
	"context"
	"errors"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/org"
)

const (
	deploymentFGASweepLimit           = 100
	deploymentFGAMembershipRetryLimit = 5
)

var errDeploymentCreatorMembershipUnavailable = errors.New("deployment creator has no WorkOS membership id")

// DeploymentFGAReconcileArgs reconciles one deployment when DeploymentID is
// present, or sweeps due durable work when it is empty.
type DeploymentFGAReconcileArgs struct {
	DeploymentID string `json:"deployment_id,omitempty"`
}

func (DeploymentFGAReconcileArgs) Kind() string { return "deployment.fga_reconcile" }

func (DeploymentFGAReconcileArgs) InsertOpts() river.InsertOpts {
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
	registerJobKind[DeploymentFGAReconcileArgs]()
}

type deploymentFGAOrganizationClient interface {
	GetOrganization(context.Context, string) (org.Organization, error)
}

type deploymentFGAQueue interface {
	InsertDeploymentFGAReconcileJob(context.Context, string) error
}

// DeploymentFGAReconcileWorker applies Astro's desired deployment resource
// state to WorkOS after the authoritative database transaction commits.
type DeploymentFGAReconcileWorker struct {
	river.WorkerDefaults[DeploymentFGAReconcileArgs]
	fga           authz.FGA
	sync          *authz.DeploymentFGASyncStore
	organizations deploymentFGAOrganizationClient
	queue         deploymentFGAQueue
	log           *logger.Logger
}

func (w *DeploymentFGAReconcileWorker) Work(ctx context.Context, job *river.Job[DeploymentFGAReconcileArgs]) error {
	if w.sync == nil {
		return errors.New("deployment FGA sync store is not configured")
	}
	maintenance, err := w.sync.MaintenanceEnabled(ctx)
	if err != nil || maintenance {
		return err
	}
	if job.Args.DeploymentID != "" {
		return w.reconcile(ctx, job.Args.DeploymentID)
	}

	ids, err := w.sync.DueDeploymentIDs(ctx, deploymentFGASweepLimit)
	if err != nil {
		return err
	}
	var enqueueErr error
	for _, id := range ids {
		if w.queue == nil {
			enqueueErr = errors.Join(enqueueErr, errors.New("deployment FGA reconciliation queue is not configured"))
			break
		}
		if err := w.queue.InsertDeploymentFGAReconcileJob(ctx, id); err != nil {
			enqueueErr = errors.Join(enqueueErr, fmt.Errorf("enqueue deployment %s: %w", id, err))
			w.log.Warn("deployment fga: enqueue deployment FGA reconciliation failed",
				"deployment_id", id,
				"error", err,
			)
		}
	}
	return enqueueErr
}

func (w *DeploymentFGAReconcileWorker) reconcile(ctx context.Context, deploymentID string) error {
	work, err := w.sync.Pending(ctx, deploymentID)
	if err != nil || work == nil {
		return err
	}
	if !w.sync.Enabled() {
		_, err := w.sync.MarkSynced(ctx, work.DeploymentID, work.DesiredState, work.DesiredVersion)
		return err
	}
	if w.fga == nil {
		return w.fail(ctx, work, errors.New("deployment FGA client is not configured"))
	}
	resource := authz.DeploymentResource(work.DeploymentID)
	switch work.DesiredState {
	case authz.DeploymentFGARegistered:
		if work.WorkOSOrgID == "" {
			return w.fail(ctx, work, errors.New("deployment account has no WorkOS organization id"))
		}
		firstRegistration := work.SyncedState != authz.DeploymentFGARegistered
		if firstRegistration {
			registerErr := w.fga.RegisterResource(ctx, work.WorkOSOrgID, resource, work.Name)
			if registerErr != nil && !errors.Is(registerErr, authz.ErrResourceExists) {
				return w.fail(ctx, work, registerErr)
			}
			if errors.Is(registerErr, authz.ErrResourceExists) {
				if err := w.fga.UpdateResourceName(ctx, work.WorkOSOrgID, resource, work.Name); err != nil {
					return w.fail(ctx, work, err)
				}
			}
		} else if work.SyncedVersion != work.DesiredVersion {
			if err := w.fga.UpdateResourceName(ctx, work.WorkOSOrgID, resource, work.Name); err != nil {
				return w.fail(ctx, work, err)
			}
		}

		if firstRegistration || work.CreatorAssignmentPending {
			switch {
			case !work.CreatorIsMember:
				w.log.Info("deployment fga: deployment creator is not a current organization member; skipping creator role assignment",
					"deployment_id", work.DeploymentID,
					"organization_id", work.WorkOSOrgID,
				)
			case work.MembershipID != "":
				if err := w.fga.AssignRole(ctx, authz.MembershipAssignmentSubject(work.MembershipID), authz.RoleDeploymentAdmin, resource); err != nil && !errors.Is(err, authz.ErrRoleAssignmentExists) {
					return w.fail(ctx, work, err)
				}
			case firstRegistration && work.AttemptCount < deploymentFGAMembershipRetryLimit:
				return w.fail(ctx, work, errDeploymentCreatorMembershipUnavailable)
			default:
				deferred, err := w.sync.DeferCreatorAssignment(ctx, work.DeploymentID, work.DesiredState, work.DesiredVersion)
				if err != nil {
					return err
				}
				if deferred {
					w.log.Warn("deployment fga: deployment creator membership unavailable; deferring role assignment",
						"deployment_id", work.DeploymentID,
						"organization_id", work.WorkOSOrgID,
					)
				}
				return nil
			}
		}
	case authz.DeploymentFGADeleted:
		if work.WorkOSOrgID == "" {
			w.log.Info("deployment fga: deployment account has no WorkOS organization id; treating resource deletion as complete",
				"deployment_id", work.DeploymentID,
			)
			break
		}
		deleteErr := w.fga.DeleteResource(ctx, work.WorkOSOrgID, resource)
		if deleteErr != nil && !errors.Is(deleteErr, authz.ErrResourceNotFound) {
			if w.organizations == nil {
				return w.fail(ctx, work, deleteErr)
			}
			_, orgErr := w.organizations.GetOrganization(ctx, work.WorkOSOrgID)
			switch {
			case errors.Is(orgErr, org.ErrOrganizationNotFound):
				w.log.Info("deployment fga: WorkOS organization is gone; treating deployment resource deletion as complete",
					"deployment_id", work.DeploymentID,
					"organization_id", work.WorkOSOrgID,
				)
			case orgErr != nil:
				return w.fail(ctx, work, errors.Join(deleteErr, fmt.Errorf("check WorkOS organization: %w", orgErr)))
			default:
				return w.fail(ctx, work, deleteErr)
			}
		}
	default:
		return w.fail(ctx, work, fmt.Errorf("unsupported deployment FGA desired state %q", work.DesiredState))
	}

	synced, err := w.sync.MarkSynced(ctx, work.DeploymentID, work.DesiredState, work.DesiredVersion)
	if err != nil {
		return err
	}
	if !synced {
		w.log.Debug("deployment fga: desired state changed during reconciliation",
			"deployment_id", work.DeploymentID,
			"attempted_state", work.DesiredState,
		)
		return nil
	}
	w.log.Info("deployment fga: reconciliation complete",
		"deployment_id", work.DeploymentID,
		"desired_state", work.DesiredState,
		"desired_version", work.DesiredVersion,
	)
	return nil
}

func (w *DeploymentFGAReconcileWorker) fail(ctx context.Context, work *authz.DeploymentFGAWork, cause error) error {
	if err := w.sync.RecordFailure(ctx, work.DeploymentID, work.DesiredState, work.DesiredVersion, cause); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
