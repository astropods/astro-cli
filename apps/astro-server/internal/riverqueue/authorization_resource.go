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
	authorizationResourceSweepLimit   = 100
	authorizationMembershipRetryLimit = 5
)

var errAuthorizationCreatorMembershipUnavailable = errors.New("resource creator has no WorkOS membership id")

type AuthorizationResourceReconcileArgs struct {
	OrganizationID string `json:"organization_id,omitempty"`
	ResourceType   string `json:"resource_type,omitempty" river:"unique"`
	ResourceID     string `json:"resource_id,omitempty" river:"unique"`
}

func (AuthorizationResourceReconcileArgs) Kind() string { return "authorization.resource_reconcile" }

func (AuthorizationResourceReconcileArgs) InsertOpts() river.InsertOpts {
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
	registerJobKind[AuthorizationResourceReconcileArgs]()
}

type authorizationResourceQueue interface {
	InsertAuthorizationResourceReconcileJob(context.Context, authz.ResourceSyncKey) error
}

// AuthorizationResourceReconcileWorker applies Astro's desired resource tree
// to WorkOS after the authoritative Astro transaction commits.
type AuthorizationResourceReconcileWorker struct {
	river.WorkerDefaults[AuthorizationResourceReconcileArgs]
	lifecycle     authz.AuthorizationResourceLifecycle
	sync          *authz.AuthorizationResourceSyncStore
	organizations deploymentFGAOrganizationClient
	queue         authorizationResourceQueue
	log           *logger.Logger
}

func (w *AuthorizationResourceReconcileWorker) Work(ctx context.Context, job *river.Job[AuthorizationResourceReconcileArgs]) error {
	if w.sync == nil {
		return errors.New("authorization resource sync store is not configured")
	}
	if job.Args.ResourceID != "" {
		return w.reconcile(ctx, authz.ResourceSyncKey{
			OrganizationID: job.Args.OrganizationID,
			Resource: authz.ResourceRef{
				Type:       authz.ResourceType(job.Args.ResourceType),
				ExternalID: job.Args.ResourceID,
			},
		})
	}

	keys, err := w.sync.Due(ctx, authorizationResourceSweepLimit)
	if err != nil {
		return err
	}
	var enqueueErr error
	for _, key := range keys {
		if w.queue == nil {
			return errors.New("authorization resource reconciliation queue is not configured")
		}
		if err := w.queue.InsertAuthorizationResourceReconcileJob(ctx, key); err != nil {
			enqueueErr = errors.Join(enqueueErr, fmt.Errorf("enqueue %s:%s: %w", key.Resource.Type, key.Resource.ExternalID, err))
		}
	}
	return enqueueErr
}

func (w *AuthorizationResourceReconcileWorker) reconcile(ctx context.Context, key authz.ResourceSyncKey) error {
	work, err := w.sync.Pending(ctx, key)
	if err != nil || work == nil {
		return err
	}
	if !w.sync.Enabled() {
		_, err := w.sync.MarkSynced(ctx, *work, work.WorkOSAuthorizationResourceID)
		return err
	}
	if w.lifecycle == nil {
		return w.fail(ctx, work, errors.New("authorization resource client is not configured"))
	}

	if work.DesiredState == authz.AuthorizationResourceRegistered && work.Parent.Type == authz.ResourceAccount {
		parentKey := authz.ResourceSyncKey{OrganizationID: work.OrganizationID, Resource: work.Parent}
		if err := w.reconcile(ctx, parentKey); err != nil {
			return w.fail(ctx, work, fmt.Errorf("reconcile Account parent: %w", err))
		}
	}

	workosID := work.WorkOSAuthorizationResourceID
	switch work.DesiredState {
	case authz.AuthorizationResourceRegistered:
		firstRegistration := work.SyncedState != authz.AuthorizationResourceRegistered
		if firstRegistration {
			workosID, err = w.lifecycle.RegisterResourceWithParent(ctx, work.OrganizationID, work.Resource, work.Parent, work.Name)
			if err != nil {
				return w.fail(ctx, work, err)
			}
		} else if work.SyncedVersion != work.DesiredVersion {
			if err := w.lifecycle.UpdateResourceName(ctx, work.OrganizationID, work.Resource, work.Name); err != nil {
				return w.fail(ctx, work, err)
			}
		}

		if (firstRegistration || work.CreatorAssignmentPending) && work.CreatorRole != "" && work.CreatorUserID != "" {
			switch {
			case !work.CreatorIsMember:
				w.log.Info("authorization resource: skipped creator admin assignment, not a member",
					"resource_type", work.Resource.Type,
					"resource_id", work.Resource.ExternalID,
					"creator_user_id", work.CreatorUserID,
				)
			case work.MembershipID != "":
				err := w.lifecycle.AssignRole(ctx, authz.MembershipAssignmentSubject(work.MembershipID), work.CreatorRole, work.Resource)
				if err != nil && !errors.Is(err, authz.ErrRoleAssignmentExists) {
					return w.fail(ctx, work, err)
				}
			case firstRegistration && work.AttemptCount < authorizationMembershipRetryLimit:
				if _, err := w.sync.MarkRegisteredPendingCreator(ctx, *work, workosID); err != nil {
					return err
				}
				return w.fail(ctx, work, errAuthorizationCreatorMembershipUnavailable)
			default:
				deferred, err := w.sync.DeferCreatorAssignment(ctx, *work, workosID)
				if err != nil {
					return err
				}
				if deferred {
					w.log.Warn("authorization resource: deferred creator admin assignment, membership unavailable",
						"resource_type", work.Resource.Type,
						"resource_id", work.Resource.ExternalID,
						"creator_user_id", work.CreatorUserID,
					)
				}
				return nil
			}
		}
	case authz.AuthorizationResourceDeleted:
		deleteErr := w.lifecycle.DeleteResource(ctx, work.OrganizationID, work.Resource)
		if deleteErr != nil && !errors.Is(deleteErr, authz.ErrResourceNotFound) {
			if w.organizations == nil {
				return w.fail(ctx, work, deleteErr)
			}
			_, orgErr := w.organizations.GetOrganization(ctx, work.OrganizationID)
			switch {
			case errors.Is(orgErr, org.ErrOrganizationNotFound):
				w.log.Info("authorization resource: completed deletion, organization unavailable",
					"resource_type", work.Resource.Type,
					"resource_id", work.Resource.ExternalID,
				)
			case orgErr != nil:
				return w.fail(ctx, work, errors.Join(deleteErr, fmt.Errorf("check WorkOS organization: %w", orgErr)))
			default:
				return w.fail(ctx, work, deleteErr)
			}
		}
	default:
		return w.fail(ctx, work, fmt.Errorf("unsupported authorization resource desired state %q", work.DesiredState))
	}

	synced, err := w.sync.MarkSynced(ctx, *work, workosID)
	if err != nil {
		return err
	}
	if synced {
		w.log.Info("authorization resource: reconciliation complete",
			"resource_type", work.Resource.Type,
			"resource_id", work.Resource.ExternalID,
			"desired_state", work.DesiredState,
			"desired_version", work.DesiredVersion,
		)
	}
	return nil
}

func (w *AuthorizationResourceReconcileWorker) fail(ctx context.Context, work *authz.AuthorizationResourceWork, cause error) error {
	if err := w.sync.RecordFailure(ctx, *work, cause); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
