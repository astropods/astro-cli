package riverqueue

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
)

const queueDeploy = "deploy"

// Config holds dependencies that River workers need.
type Config struct {
	DB                   *sql.DB
	OMClient             *openmeter.Client
	AccountStore         *account.AccountStore
	AgentIndex           *agentindex.Index
	AvatarStore          *avatar.Store
	K8sClient            k8s.ClusterClient
	K8sCache             k8scache.Cache
	ServerConfig         *config.Config
	WorkOSAPIKey         string
	OrgClient            *org.Client
	PromClient           *promquery.Client
	Logger               *logger.Logger
	WorkOSClient         *auth.WorkOSClient
	AccountRetentionDays int // days after soft-delete before hard-purge; default 7
	// GitHub build worker deps (optional — worker skipped if PipesClient is nil)
	PipesClient *pipes.Client
	GitHubStore *githubconnection.Store
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
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("riverqueue: pgxpool: %w", err)
	}

	workers := river.NewWorkers()
	reconcileWorker, purgeWorker := addWorkers(workers, cfg)

	riverClient, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Schema: "river",
		Queues: map[string]river.QueueConfig{
			river.QueueDefault: {MaxWorkers: 10},
			queueDeploy:        {MaxWorkers: 5},
			queueWorkOS:        {MaxWorkers: 1},
			queueGitHubBuild:   {MaxWorkers: 3},
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
	if reconcileWorker != nil {
		reconcileWorker.queue = q
	}
	if purgeWorker != nil {
		purgeWorker.enqueueUndeploy = func(ctx context.Context, deploymentID string) error {
			store := purgeWorker.deployStore
			if err := store.UpdateStatus(deploymentID, "undeploying", "", nil); err != nil {
				return fmt.Errorf("update status: %w", err)
			}
			return q.InsertUndeployJob(ctx, deploymentID)
		}
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
	q.log.Info("river: queue started", "queues", []string{river.QueueDefault, queueDeploy, queueWorkOS})
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

// InsertDeployJob enqueues a deploy job.
func (q *Queue) InsertDeployJob(ctx context.Context, deploymentID string) error {
	_, err := q.Insert(ctx, DeployArgs{DeploymentID: deploymentID}, nil)
	return err
}

// InsertUndeployJob enqueues an undeploy job.
func (q *Queue) InsertUndeployJob(ctx context.Context, deploymentID string) error {
	_, err := q.Insert(ctx, UndeployArgs{DeploymentID: deploymentID}, nil)
	return err
}

// InsertWakeUpJob enqueues a wakeup job.
func (q *Queue) InsertWakeUpJob(ctx context.Context, deploymentID string) error {
	_, err := q.Insert(ctx, WakeUpArgs{DeploymentID: deploymentID}, nil)
	return err
}

// EnqueueGitHubBuild enqueues a GitHub build job.
func (q *Queue) EnqueueGitHubBuild(ctx context.Context, args GitHubBuildArgs) error {
	_, err := q.Insert(ctx, args, nil)
	return err
}

// InsertOpenMeterBackfillJob enqueues an immediate OpenMeter customer backfill job.
func (q *Queue) InsertOpenMeterBackfillJob(ctx context.Context) error {
	_, err := q.Insert(ctx, OpenMeterBackfillArgs{}, nil)
	return err
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
