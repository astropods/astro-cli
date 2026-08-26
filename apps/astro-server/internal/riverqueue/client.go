package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/authorizationadmin"
	"github.com/astropods/astro/apps/astro-server/internal/authz"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
)

// Config holds dependencies that River workers need.
type Config struct {
	DB                        *sql.DB
	Billing                   billing.BillingProvider
	BillingBackend            string // active billing backend ("metronome"|"noop")
	AccountStore              *account.AccountStore
	AgentIndex                *agentindex.Index
	AvatarStore               *avatar.Store
	ReadmeAssetStore          *readmeassets.Store
	K8sRegistry               *k8s.Registry
	K8sCache                  k8scache.Cache
	ServerConfig              *config.Config
	DeploymentFGASync         *authz.DeploymentFGASyncStore
	AuthorizationResourceSync *authz.AuthorizationResourceSyncStore
	DeploymentResourceSync    authz.DeploymentResourceSyncRecorder
	ResourceAccessSync        *authz.ResourceAccessSyncStore
	AccessReconciler          *authz.AccessReconciler
	AuthorizationAdmin        *authorizationadmin.Service
	// AuthorizationAdminResetEnabled registers the reset worker only when explicitly configured.
	AuthorizationAdminResetEnabled bool
	FGA                            authz.FGA
	AuthorizationResourceLifecycle authz.AuthorizationResourceLifecycle
	OrgClient                      *org.Client
	PromClient                     *promquery.Client
	Logger                         *logger.Logger
	WorkOSClient                   *auth.WorkOSClient
	Vault                          *envelope.Vault
	// LangfuseStore is used by the DeployWorker to provision per-deployment
	// Langfuse datasets at deploy time. Optional — when nil, dataset
	// provisioning is skipped.
	LangfuseStore *langfuse.Store
	// PaymentProvider lets the Stripe webhook worker re-read a customer's cards,
	// so a payment_method.detached during a card replacement isn't mistaken for
	// a removal. nil → card events are ignored rather than guessed at.
	PaymentProvider payment.Provider
	// GitHub build worker deps (optional — worker skipped if PipesClient is nil)
	PipesClient *pipes.Client
	GitHubStore *githubconnection.Store
	// ImagePreflighter is plumbed into the Deployer/Applier so the worker
	// validates tenant images against the registry alongside the handler-side
	// preflight in DeployAgent. Sharing the same instance across both call
	// sites keeps the 60s positive-result cache warm.
	ImagePreflighter *k8s.ImagePreflighter
	// InsightsRollupProducer is injected by main so the roll-up workers can
	// build the durable fact table using the same Langfuse helpers the handlers
	// own. nil → the roll-up workers no-op.
	InsightsRollupProducer InsightsRollupProducer
	// ClassificationProducer is injected by main so the classification workers
	// can reuse the handlers package's dev-tool adapter registry and identity
	// mapping. nil → the classification workers no-op.
	ClassificationProducer ClassificationProducer
	// ReconcileDeployment, when set, is called with a namespace right after the
	// DeployWorker marks a deployment "deploying", so the controller reconciles
	// it immediately instead of waiting for the next resync. Optional.
	ReconcileDeployment func(namespace string)
	// NotifyProvider delivers user alerts (Novu on the hosted path). When nil,
	// the NotifyWorker uses a no-op provider that logs and drops. Per-channel
	// preferences are owned and enforced by Novu, not gated here.
	NotifyProvider notify.Provider
}

// Queue wraps a River client and its pgxpool connection.
type Queue struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
	log    *logger.Logger
	// billingEnforce is BILLING_GATE_ENFORCE; false is observe mode.
	billingEnforce bool
}

// New creates a Queue: opens a pgxpool, registers workers, and builds the River client.
// The River schema tables must already exist (managed via Bytebase).
func New(ctx context.Context, databaseURL string, cfg Config) (*Queue, error) {
	if cfg.DeploymentFGASync == nil {
		cfg.DeploymentFGASync = authz.NewDeploymentFGASyncStore(cfg.DB, false)
	}
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("riverqueue: pgxpool: %w", err)
	}

	workers := river.NewWorkers()
	wired := addWorkers(workers, cfg)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "river",
		Queues: map[string]river.QueueConfig{
			river.QueueDefault:  {MaxWorkers: 10},
			queueDeploy:         {MaxWorkers: 5},
			queueBuild:          {MaxWorkers: 3},
			queueBilling:        {MaxWorkers: 3},
			queueMetering:       {MaxWorkers: 3},
			queueInsights:       {MaxWorkers: 3},
			queueMaintenance:    {MaxWorkers: 5},
			queueEvalJudge:      {MaxWorkers: evalJudgeMaxWorkers},
			queueEvaluation:     {MaxWorkers: evaluationMaxWorkers},
			queueNotifications:  {MaxWorkers: 3},
			queueClassification: {MaxWorkers: classificationMaxWorkers},
		},
		Workers:      workers,
		PeriodicJobs: periodicJobs(cfg),
		Logger:       slog.New(levelHandler{minLevel: slog.LevelWarn, inner: cfg.Logger.Handler()}),
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("riverqueue: client: %w", err)
	}

	q := &Queue{
		pool:           pool,
		client:         riverClient,
		log:            cfg.Logger,
		billingEnforce: cfg.ServerConfig != nil && cfg.ServerConfig.BillingGateEnforce,
	}

	// Set queue references on workers that need Insert capability.
	// This is safe because workers don't run until Start() is called.
	if wired.insightsRollup != nil {
		wired.insightsRollup.queue = q
	}
	if wired.classification != nil {
		wired.classification.queue = q
	}
	if wired.purge != nil {
		wired.purge.purger.Undeploy = q.UndeployFunc(wired.purge.purger.Deployments)
	}
	if wired.migrate != nil {
		wired.migrate.queue = q
	}
	if wired.dunning != nil {
		wired.dunning.queue = q
	}
	if wired.billingResume != nil {
		wired.billingResume.queue = q
	}
	if wired.metronomeHook != nil {
		wired.metronomeHook.queue = q
	}
	if wired.stripeHook != nil {
		wired.stripeHook.queue = q
	}
	if wired.provisionWorker != nil {
		wired.provisionWorker.queue = q
	}
	if wired.provisionSweep != nil {
		wired.provisionSweep.queue = q
	}
	if wired.orgProvisionSweep != nil {
		wired.orgProvisionSweep.queue = q
	}
	if wired.ghBuild != nil {
		wired.ghBuild.queue = q
	}
	if wired.observation != nil {
		wired.observation.queue = q
	}
	if wired.undeploy != nil {
		wired.undeploy.fgaQueue = q
	}
	if wired.deploymentFGA != nil {
		wired.deploymentFGA.queue = q
	}
	if wired.authorizationResource != nil {
		wired.authorizationResource.queue = q
	}
	if wired.resourceAccess != nil {
		wired.resourceAccess.queue = q
	}

	return q, nil
}

// Client returns the underlying River client (e.g. for River UI).
func (q *Queue) Client() *river.Client[pgx.Tx] {
	return q.client
}

// Start starts the River client (begins processing jobs).
func (q *Queue) Start(ctx context.Context) error {
	if err := q.client.Start(ctx); err != nil {
		return fmt.Errorf("riverqueue: start: %w", err)
	}
	q.log.Info("river: queue started", "queues", []string{
		river.QueueDefault,
		queueDeploy,
		queueBuild,
		queueBilling,
		queueMetering,
		queueInsights,
		queueMaintenance,
		queueEvalJudge,
		queueEvaluation,
	})
	return nil
}

// Stop gracefully drains in-flight jobs and closes the pool.
func (q *Queue) Stop(ctx context.Context) error {
	if err := q.client.Stop(ctx); err != nil {
		q.log.Error("river: queue stop error", "error", err)
	}
	q.pool.Close()
	q.log.Info("river: queue stopped")
	return nil
}

// InsertTx enqueues a job within an existing transaction (for atomicity with DB writes).
func (q *Queue) InsertTx(ctx context.Context, tx pgx.Tx, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	return q.client.InsertTx(ctx, tx, args, opts)
}

// Insert enqueues a job in its own transaction.
func (q *Queue) Insert(ctx context.Context, args river.JobArgs, opts *river.InsertOpts) (*rivertype.JobInsertResult, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("riverqueue: begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	result, err := q.client.InsertTx(ctx, tx, args, opts)
	if err != nil {
		return nil, fmt.Errorf("riverqueue: insert: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("riverqueue: commit: %w", err)
	}
	return result, nil
}

// EmitNotify enqueues one alert for off-request delivery. This is the emit seam
// sources call; recipient resolution and the Novu trigger happen in NotifyWorker.
func (q *Queue) EmitNotify(ctx context.Context, ev notify.Event) error {
	_, err := q.Insert(ctx, NotifyArgs{Event: ev.Stamped(time.Now())}, nil)
	return err
}

// EmitNotifyTx enqueues an alert inside an existing transaction, so the alert
// and the state change that warrants it commit atomically.
func (q *Queue) EmitNotifyTx(ctx context.Context, tx pgx.Tx, ev notify.Event) error {
	_, err := q.InsertTx(ctx, tx, NotifyArgs{Event: ev.Stamped(time.Now())}, nil)
	return err
}

// InsertDeployJob enqueues a deploy job. clusterID is empty when the deployment
// targets the primary cluster (deployments.cluster_id IS NULL).
func (q *Queue) InsertDeployJob(ctx context.Context, deploymentID, clusterID string) error {
	_, err := q.Insert(ctx, DeployArgs{DeploymentID: deploymentID, ClusterID: clusterID}, nil)
	return err
}

// InsertUndeployJob enqueues an undeploy job.
func (q *Queue) InsertUndeployJob(ctx context.Context, deploymentID, clusterID string) error {
	_, err := q.Insert(ctx, UndeployArgs{DeploymentID: deploymentID, ClusterID: clusterID}, nil)
	return err
}

// UndeployFunc returns the teardown hook accountlifecycle.Purger calls for a
// deployment whose undeploy never made it onto the queue. It moves the row to
// undeploying first, so a lost job is visible as state rather than as a
// deployment that looks live with nothing behind it.
func (q *Queue) UndeployFunc(store *deploymentstore.Store) func(ctx context.Context, deploymentID string) error {
	return func(ctx context.Context, deploymentID string) error {
		if err := store.UpdateStatus(deploymentID, deploymentstore.StatusUpdate{Status: deploymentstore.StatusUndeploying}); err != nil {
			return fmt.Errorf("update status: %w", err)
		}
		dep, derr := store.GetDeploymentByID(deploymentID)
		cid := ""
		if derr == nil && dep != nil {
			cid = dep.EffectiveClusterID()
		}
		return q.InsertUndeployJob(ctx, deploymentID, cid)
	}
}

// InsertDeploymentFGAReconcileJob enqueues generic deployment-resource reconciliation.
func (q *Queue) InsertDeploymentFGAReconcileJob(ctx context.Context, deploymentID string) error {
	return q.InsertAuthorizationResourceReconcileJob(ctx, authz.ResourceSyncKey{
		Resource: authz.DeploymentResource(deploymentID),
	})
}

// InsertLegacyDeploymentFGAReconcileJob drains rows written before the generic ledger.
func (q *Queue) InsertLegacyDeploymentFGAReconcileJob(ctx context.Context, deploymentID string) error {
	_, err := q.Insert(ctx, DeploymentFGAReconcileArgs{DeploymentID: deploymentID}, nil)
	return err
}

// InsertAuthorizationResourceReconcileJob enqueues one generic lifecycle key.
func (q *Queue) InsertAuthorizationResourceReconcileJob(ctx context.Context, key authz.ResourceSyncKey) error {
	_, err := q.Insert(ctx, AuthorizationResourceReconcileArgs{
		OrganizationID: key.OrganizationID,
		ResourceType:   string(key.Resource.Type),
		ResourceID:     key.Resource.ExternalID,
	}, nil)
	return err
}

// InsertResourceAccessFGAReconcileJob enqueues reconciliation for one resource.
func (q *Queue) InsertResourceAccessFGAReconcileJob(ctx context.Context, key authz.AccessIntentKey) error {
	_, err := q.Insert(ctx, ResourceAccessFGAReconcileArgs{
		OrganizationID: key.OrganizationID,
		ResourceType:   string(key.Resource.Type),
		ResourceID:     key.Resource.ExternalID,
	}, nil)
	return err
}

// InsertMigrateDeploymentClusterJob enqueues a cross-cluster migration job.
func (q *Queue) InsertMigrateDeploymentClusterJob(ctx context.Context, deploymentID, targetClusterID, sourceClusterID string) error {
	_, err := q.Insert(ctx, MigrateDeploymentClusterArgs{
		DeploymentID:    deploymentID,
		TargetClusterID: targetClusterID,
		SourceClusterID: sourceClusterID,
	}, nil)
	return err
}

// InsertWakeUpJob enqueues a wakeup job.
func (q *Queue) InsertWakeUpJob(ctx context.Context, deploymentID, clusterID string) error {
	_, err := q.Insert(ctx, WakeUpArgs{DeploymentID: deploymentID, ClusterID: clusterID}, nil)
	return err
}

// InsertEvalDatasetEvaluationJobs enqueues one evaluation job per trace in one
// River transaction.
func (q *Queue) InsertEvalDatasetEvaluationJobs(ctx context.Context, evalDatasetID string, traceIDs []string) error {
	if len(traceIDs) == 0 {
		return nil
	}
	_, err := q.client.InsertMany(ctx, evalDatasetEvaluationInsertManyParams(evalDatasetID, traceIDs))
	return err
}

func evalDatasetEvaluationInsertManyParams(evalDatasetID string, traceIDs []string) []river.InsertManyParams {
	params := make([]river.InsertManyParams, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		params = append(params, river.InsertManyParams{
			Args: EvalDatasetEvaluationArgs{
				EvalDatasetID: evalDatasetID,
				TraceID:       traceID,
			},
		})
	}
	return params
}

// InsertBillingSuspend enqueues a billing suspend for an account (scale its
// deployments to zero). No-op in observe mode.
func (q *Queue) InsertBillingSuspend(ctx context.Context, accountID string) error {
	if !q.billingActs("suspend", accountID) {
		return nil
	}
	_, err := q.Insert(ctx, BillingSuspendArgs{AccountID: accountID}, nil)
	return err
}

// InsertBillingResume enqueues a billing resume for an account (restore the
// deployments billing suspended).
//
// Deliberately not gated. Resume is remediation, not enforcement: it only
// restores deployments left in StatusSuspended, which nothing but billing sets,
// so with enforcement off there is nothing for it to find. Gating it with
// suspend would mean turning the flag off after a real suspension could not
// undo it — an account that then fixed its card would stay at zero replicas.
func (q *Queue) InsertBillingResume(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, BillingResumeArgs{AccountID: accountID}, nil)
	return err
}

// InsertBillingGatewayBudget enqueues a re-derive of the account's AI gateway
// spend ceiling.
//
// Deliberately not gated. The ceiling is a provider-side limit rather than an
// action against the account's workloads, and leaving it stale with enforcement
// off would let a card-less account keep the wider one.
func (q *Queue) InsertBillingGatewayBudget(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, BillingGatewayBudgetArgs{AccountID: accountID}, nil)
	return err
}

// InsertBillingCollect enqueues a charge attempt against an account's open
// invoices.
//
// Deliberately not gated, for the reason InsertBillingResume is not: collection
// is remediation. The invoices are real debt whether or not enforcement is on,
// and the provider would charge them on its own schedule anyway.
func (q *Queue) InsertBillingCollect(ctx context.Context, accountID, stripeCustomerID string) error {
	_, err := q.Insert(ctx, BillingCollectArgs{AccountID: accountID, StripeCustomerID: stripeCustomerID}, nil)
	return err
}

// EmitBillingNotify sends an owner-facing billing alert, or logs what it would
// have sent in observe mode. Separate from EmitNotify so only billing traffic
// is gated.
func (q *Queue) EmitBillingNotify(ctx context.Context, ev notify.Event) error {
	if !q.billingActs("notify "+string(ev.Type), ev.AccountID) {
		return nil
	}
	return q.EmitNotify(ctx, ev)
}

// billingActs reports whether billing may take a user-visible action, logging
// the decision it skipped when it may not.
func (q *Queue) billingActs(action, accountID string) bool {
	if q.billingEnforce {
		return true
	}
	q.log.Info("billing gate (observe): would "+action, "account_id", accountID)
	return false
}

// InsertBillingProvision enqueues rate-card + signup-credit provisioning for an
// account. Deduped by account, so repeat calls collapse.
func (q *Queue) InsertBillingProvision(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, BillingProvisionArgs{AccountID: accountID}, nil)
	return err
}

// InsertAccountOrgProvision enqueues WorkOS organization provisioning for an
// account.
func (q *Queue) InsertAccountOrgProvision(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, AccountOrgProvisionArgs{AccountID: accountID}, nil)
	return err
}

// InsertMetronomeWebhook enqueues a verified Metronome webhook for processing.
// Queue routing + event-ID dedupe come from MetronomeWebhookArgs.InsertOpts.
func (q *Queue) InsertMetronomeWebhook(ctx context.Context, args MetronomeWebhookArgs) error {
	_, err := q.Insert(ctx, args, nil)
	return err
}

// InsertStripeWebhook enqueues a verified Stripe webhook for processing.
// Queue routing + event-ID dedupe come from StripeWebhookArgs.InsertOpts.
func (q *Queue) InsertStripeWebhook(ctx context.Context, eventID, eventType, customerID, hostedInvoiceURL string) error {
	_, err := q.Insert(ctx, StripeWebhookArgs{EventID: eventID, EventType: eventType, CustomerID: customerID, HostedInvoiceURL: hostedInvoiceURL}, nil)
	return err
}

// EnqueueGitHubBuild enqueues a GitHub build job.
func (q *Queue) EnqueueGitHubBuild(ctx context.Context, args GitHubBuildArgs) error {
	_, err := q.Insert(ctx, args, nil)
	return err
}

// CancelGitHubBuildsForConnection cancels all active River jobs for a GitHub connection.
// Called when a new push arrives so older in-flight builds are interrupted and their
// K8s jobs are cleaned up via the RunJob defer.
func (q *Queue) CancelGitHubBuildsForConnection(ctx context.Context, connectionID string) {
	rows, err := q.pool.Query(ctx, `
		SELECT id FROM river.river_jobs
		WHERE kind = 'build.github'
		  AND args->>'connection_id' = $1
		  AND state IN ('available', 'pending', 'running', 'scheduled')
	`, connectionID)
	if err != nil {
		q.log.Warn("cancel github builds: query jobs", "error", err, "connection_id", connectionID)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if _, err := q.client.JobCancel(ctx, id); err != nil {
			q.log.Warn("cancel github builds: cancel job", "error", err, "job_id", id)
		}
	}
}

// InsertPrivateLinkProvisionJob enqueues a job to create a VPC endpoint.
func (q *Queue) InsertPrivateLinkProvisionJob(ctx context.Context, storeID string) error {
	_, err := q.Insert(ctx, PrivateLinkProvisionArgs{StoreID: storeID}, nil)
	return err
}

// InsertPrivateLinkDeleteJob enqueues a job to delete a VPC endpoint.
func (q *Queue) InsertPrivateLinkDeleteJob(ctx context.Context, storeID, endpointID string) error {
	_, err := q.Insert(ctx, PrivateLinkDeleteArgs{StoreID: storeID, EndpointID: endpointID}, nil)
	return err
}

// CancelJob cancels a single River job by ID.
func (q *Queue) CancelJob(ctx context.Context, id int64) error {
	_, err := q.client.JobCancel(ctx, id)
	return err
}

// RetryJob resets a failed/cancelled/completed job back to available state.
// It returns false when the job does not exist or is not in a retryable state.
func (q *Queue) RetryJob(ctx context.Context, id int64) (bool, error) {
	tag, err := q.pool.Exec(ctx, `
		UPDATE river.river_job
		SET state = 'available',
		    finalized_at = NULL,
		    scheduled_at = now()
		WHERE id = $1
		  AND state IN ('discarded', 'cancelled', 'completed')
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// PauseQueue pauses processing of a River queue by name.
func (q *Queue) PauseQueue(ctx context.Context, name string) error {
	return q.client.QueuePause(ctx, name, nil)
}

// ResumeQueue resumes processing of a paused River queue by name.
func (q *Queue) ResumeQueue(ctx context.Context, name string) error {
	return q.client.QueueResume(ctx, name, nil)
}

// NewInsertOnly creates a Queue that can only insert jobs (no workers, no periodic jobs).
// Used by the API process to enqueue deploy/undeploy/wakeup jobs without running workers.
//
// billingEnforce is BILLING_GATE_ENFORCE and is a parameter rather than a
// config field so it cannot be left unset: the API process reconciles workloads
// on card add/remove, and a zero value silently downgrades that to observe mode.
func NewInsertOnly(ctx context.Context, databaseURL string, log *logger.Logger, billingEnforce bool) (*Queue, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("riverqueue: pgxpool: %w", err)
	}

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "river",
		Logger: slog.New(levelHandler{minLevel: slog.LevelWarn, inner: log.Handler()}),
	})
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("riverqueue: client: %w", err)
	}

	return &Queue{
		pool:           pool,
		client:         riverClient,
		log:            log,
		billingEnforce: billingEnforce,
	}, nil
}

// Close closes the pool without stopping workers (for insert-only queues).
func (q *Queue) Close() {
	q.pool.Close()
}

// levelHandler wraps an slog.Handler and drops records below minLevel.
// Used to suppress River's noisy DEBUG/INFO logs (e.g. QueryCacher).
type levelHandler struct {
	minLevel slog.Level
	inner    slog.Handler
}

func (h levelHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h levelHandler) Handle(ctx context.Context, r slog.Record) error {
	return h.inner.Handle(ctx, r)
}

func (h levelHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return levelHandler{minLevel: h.minLevel, inner: h.inner.WithAttrs(attrs)}
}

func (h levelHandler) WithGroup(name string) slog.Handler {
	return levelHandler{minLevel: h.minLevel, inner: h.inner.WithGroup(name)}
}
