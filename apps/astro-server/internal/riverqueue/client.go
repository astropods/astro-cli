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
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
)

// Config holds dependencies that River workers need.
type Config struct {
	DB                   *sql.DB
	Billing              billing.BillingProvider
	BillingBackend       string // active billing backend ("metronome"|"noop")
	AccountStore         *account.AccountStore
	AgentIndex           *agentindex.Index
	AvatarStore          *avatar.Store
	ReadmeAssetStore     *readmeassets.Store
	K8sRegistry          *k8s.Registry
	K8sCache             k8scache.Cache
	ServerConfig         *config.Config
	DeploymentFGASync    *authz.DeploymentFGASyncStore
	FGA                  authz.FGA
	OrgClient            *org.Client
	PromClient           *promquery.Client
	Logger               *logger.Logger
	WorkOSClient         *auth.WorkOSClient
	AccountRetentionDays int // days after soft-delete before hard-purge; default 7
	KMSClient            envelope.KMSClient
	// LangfuseStore is used by the DeployWorker to provision per-deployment
	// Langfuse datasets at deploy time. Optional — when nil, dataset
	// provisioning is skipped.
	LangfuseStore *langfuse.Store
	// GitHub build worker deps (optional — worker skipped if PipesClient is nil)
	PipesClient *pipes.Client
	GitHubStore *githubconnection.Store
	// ImagePreflighter is plumbed into the Deployer/Applier so the worker
	// validates tenant images against the registry alongside the handler-side
	// preflight in DeployAgent. Sharing the same instance across both call
	// sites keeps the 60s positive-result cache warm.
	ImagePreflighter *k8s.ImagePreflighter
	// InsightsSummaryComputer is injected by main so the InsightsRefreshWorker
	// can call the same compute path the request handler uses without
	// dragging the handlers package into the riverqueue import graph
	// (handlers→riverqueue already exists for the GitHub-build worker).
	InsightsSummaryComputer InsightsSummaryComputer
	// InsightsRollupProducer is injected by main so the roll-up workers can
	// build the durable fact table using the same Langfuse helpers the handlers
	// own. nil → the roll-up workers no-op.
	InsightsRollupProducer InsightsRollupProducer
	// ReconcileDeployment, when set, is called with a namespace right after the
	// DeployWorker marks a deployment "deploying", so the controller reconciles
	// it immediately instead of waiting for the next resync. Optional.
	ReconcileDeployment func(namespace string)
	// NotifyProvider delivers user alerts (Novu on the hosted path). When nil,
	// the NotifyWorker uses a no-op provider that logs and drops. Per-channel
	// preferences are owned and enforced by Novu, not gated here.
	NotifyProvider notify.Provider
}

// InsightsSummaryComputer is the contract for refreshing one account's
// Insights cache entries. main wires this to the three handlers.Compute*
// functions + JSON-marshaling so the worker stays decoupled from gin and
// the response types. nil → the InsightsRefreshWorker becomes a no-op
// (Redis still works for the agents-page cache, just no Insights
// pre-warming).
//
// Each method returns the JSON bytes to write into Redis; an error means
// every Langfuse sub-query in the underlying compute failed and the worker
// should skip the write so the previously cached value survives the outage.
type InsightsSummaryComputer interface {
	ComputeSummary(ctx context.Context, accountID, groupBy string, includeArchived bool) ([]byte, error)
	ComputeDeploymentsSummary(ctx context.Context, accountID string, includeArchived bool) ([]byte, error)
	ComputeUsersSummary(ctx context.Context, accountID string) ([]byte, error)
}

// Queue wraps a River client and its pgxpool connection.
type Queue struct {
	pool   *pgxpool.Pool
	client *river.Client[pgx.Tx]
	log    *logger.Logger
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
			river.QueueDefault: {MaxWorkers: 10},
			queueDeploy:        {MaxWorkers: 5},
			queueBuild:         {MaxWorkers: 3},
			queueBilling:       {MaxWorkers: 3},
			queueMetering:      {MaxWorkers: 3},
			queueInsights:      {MaxWorkers: 3},
			queueMaintenance:   {MaxWorkers: 5},
			queueEvalJudge:     {MaxWorkers: evalJudgeMaxWorkers},
			queueNotifications: {MaxWorkers: 3},
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
		pool:   pool,
		client: riverClient,
		log:    cfg.Logger,
	}

	// Set queue references on workers that need Insert capability.
	// This is safe because workers don't run until Start() is called.
	if wired.insights != nil {
		wired.insights.queue = q
	}
	if wired.insightsRollup != nil {
		wired.insightsRollup.queue = q
	}
	if wired.purge != nil {
		purgeWorker := wired.purge
		purgeWorker.enqueueUndeploy = func(ctx context.Context, deploymentID string) error {
			store := purgeWorker.deployStore
			if err := store.UpdateStatus(deploymentID, deploymentstore.StatusUpdate{Status: "undeploying"}); err != nil {
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
	if wired.provisionSweep != nil {
		wired.provisionSweep.queue = q
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
	})
	return nil
}

// Stop gracefully drains in-flight jobs and closes the pool.
func (q *Queue) Stop(ctx context.Context) error {
	if err := q.client.Stop(ctx); err != nil {
		q.log.Error("River queue stop error", "error", err)
	}
	q.pool.Close()
	q.log.Info("River queue stopped")
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

// InsertDeploymentFGAReconcileJob enqueues immediate WorkOS reconciliation.
// The durable desired-state row and periodic sweep recover a failed enqueue.
func (q *Queue) InsertDeploymentFGAReconcileJob(ctx context.Context, deploymentID string) error {
	_, err := q.Insert(ctx, DeploymentFGAReconcileArgs{DeploymentID: deploymentID}, nil)
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

// InsertEvalJudgePredictionJobs enqueues eval-dataset prediction targets in one
// River transaction.
func (q *Queue) InsertEvalJudgePredictionJobs(ctx context.Context, evalDatasetID string, traceIDs []string) error {
	if len(traceIDs) == 0 {
		return nil
	}
	_, err := q.client.InsertMany(ctx, evalJudgePredictionInsertManyParams(evalDatasetID, traceIDs))
	return err
}

func evalJudgePredictionInsertManyParams(evalDatasetID string, traceIDs []string) []river.InsertManyParams {
	params := make([]river.InsertManyParams, 0, len(traceIDs))
	for _, traceID := range traceIDs {
		params = append(params, river.InsertManyParams{
			Args: EvalJudgePredictionArgs{
				EvalDatasetID: evalDatasetID,
				TraceID:       traceID,
			},
		})
	}
	return params
}

// InsertBillingSuspend enqueues a billing suspend for an account (scale its
// deployments to zero).
func (q *Queue) InsertBillingSuspend(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, BillingSuspendArgs{AccountID: accountID}, nil)
	return err
}

// InsertBillingResume enqueues a billing resume for an account (restore the
// deployments billing suspended).
func (q *Queue) InsertBillingResume(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, BillingResumeArgs{AccountID: accountID}, nil)
	return err
}

// InsertBillingProvision enqueues rate-card + signup-credit provisioning for an
// account. Deduped by account, so repeat calls collapse.
func (q *Queue) InsertBillingProvision(ctx context.Context, accountID string) error {
	_, err := q.Insert(ctx, BillingProvisionArgs{AccountID: accountID}, nil)
	return err
}

// InsertMetronomeWebhook enqueues a verified Metronome webhook for processing.
// Queue routing + event-ID dedupe come from MetronomeWebhookArgs.InsertOpts.
func (q *Queue) InsertMetronomeWebhook(ctx context.Context, eventID, eventType, customerID string) error {
	_, err := q.Insert(ctx, MetronomeWebhookArgs{EventID: eventID, EventType: eventType, CustomerID: customerID}, nil)
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
func NewInsertOnly(ctx context.Context, databaseURL string, log *logger.Logger) (*Queue, error) {
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
		pool:   pool,
		client: riverClient,
		log:    log,
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
