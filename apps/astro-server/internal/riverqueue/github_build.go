package riverqueue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/github"
	"github.com/astropods/astro/apps/astro-server/internal/githubbuild"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
)

const queueGitHubBuild = "github_build"

// GitHubBuildArgs are the job arguments for the GitHub build worker.
type GitHubBuildArgs struct {
	ConnectionID  string `json:"connection_id"`
	CommitSHA     string `json:"commit_sha"`
	BuildID       string `json:"build_id"`
	BuildRecordID string `json:"build_record_id"`
	Force         bool   `json:"force,omitempty"` // skip subpath-change filtering (manual rebuild)
}

func (GitHubBuildArgs) Kind() string { return "github_build" }

func (a GitHubBuildArgs) InsertOpts() river.InsertOpts {
	return river.InsertOpts{
		Queue:       queueGitHubBuild,
		MaxAttempts: 3,
		UniqueOpts: river.UniqueOpts{
			ByArgs: true,
			ByState: []rivertype.JobState{
				rivertype.JobStateAvailable,
				rivertype.JobStatePending,
				rivertype.JobStateRunning,
				rivertype.JobStateScheduled,
			},
		},
	}
}

// GitHubBuildWorker fetches a GitHub repo at a specific commit, builds the container
// image with BuildKit, and registers the result in the agent index.
type GitHubBuildWorker struct {
	river.WorkerDefaults[GitHubBuildArgs]
	pipesClient *pipes.Client
	ghStore     *githubconnection.Store
	agentIndex  *agentindex.Index
	builder     *githubbuild.Builder
	cfg         *config.Config
	log         *logger.Logger
	omClient    *openmeter.Client
	db          *sql.DB
}

// NewGitHubBuildWorker creates a GitHubBuildWorker with all dependencies wired.
func NewGitHubBuildWorker(pipesClient *pipes.Client, ghStore *githubconnection.Store, agentIndex *agentindex.Index, k8sClient k8s.ClusterClient, cfg *config.Config, log *logger.Logger, omClient *openmeter.Client, db *sql.DB) *GitHubBuildWorker {
	return &GitHubBuildWorker{
		pipesClient: pipesClient,
		ghStore:     ghStore,
		agentIndex:  agentIndex,
		builder:     githubbuild.New(k8sClient, cfg, log),
		cfg:         cfg,
		log:         log,
		omClient:    omClient,
		db:          db,
	}
}

// Timeout gives the build job 25 minutes — enough for any reasonable image build.
// River's default is much shorter, which was causing context deadline exceeded.
func (w *GitHubBuildWorker) Timeout(*river.Job[GitHubBuildArgs]) time.Duration {
	return 25 * time.Minute
}

func (w *GitHubBuildWorker) Work(ctx context.Context, job *river.Job[GitHubBuildArgs]) error {
	args := job.Args
	log := w.log.With("build_id", args.BuildID, "commit", args.CommitSHA[:min(7, len(args.CommitSHA))])

	// dbCtx is independent of the job context so status updates always reach the DB
	// even if the River job context is cancelled mid-flight (e.g. during a long K8s poll).
	dbCtx := context.Background()

	// Atomically transition pending→building. Returns false if the build is no
	// longer pending — either a newer push cancelled it, or a previous attempt
	// already moved it to "building" and the server restarted before completion.
	// In the restart case the K8s Job may have finished but nobody was watching,
	// so we mark the build as failed to avoid leaving it stuck in "building" forever.
	started, err := w.ghStore.StartBuildIfPending(dbCtx, args.BuildRecordID)
	if err != nil {
		log.Error("failed to start build", "error", err)
		// Non-fatal: continue with best-effort status tracking.
	} else if !started {
		log.Info("build no longer pending — cancelling River job", "record_id", args.BuildRecordID)
		_ = w.ghStore.UpdateBuildStatus(dbCtx, args.BuildRecordID, "failed", "build interrupted — likely server restart; please trigger a new build")
		return river.JobCancel(fmt.Errorf("build no longer pending"))
	}

	// isLastAttempt gates whether a retriable error should also update the DB to
	// "failed". On earlier attempts we leave the status as-is so the UI shows
	// progress rather than flipping to failed and back.
	isLastAttempt := job.Attempt >= job.MaxAttempts

	conn, err := w.ghStore.GetByID(ctx, args.ConnectionID)
	if err != nil {
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("load connection: %w", err))
	}

	token, err := w.pipesClient.GetAccessToken(ctx, pipes.GetAccessTokenInput{
		Provider:       "github",
		UserID:         conn.WorkOSUserID,
		OrganizationID: conn.WorkOSOrganizationID,
	})
	if err != nil {
		log.Error("failed to get GitHub token",
			"error", err,
			"workos_user_id", conn.WorkOSUserID,
			"repo", conn.RepoFullName,
			"connection_id", args.ConnectionID,
			"attempt", job.Attempt,
			"max_attempts", job.MaxAttempts,
			"not_installed", errors.Is(err, pipes.ErrNotInstalled),
			"needs_reauth", errors.Is(err, pipes.ErrNeedsReauthorization),
		)
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("get github token: %w", err))
	}

	// Subpath filtering: skip the build if nothing under the subpath changed
	// since the last registered build. Two GitHub Contents API calls compare
	// the tree SHA for the subpath directory at both commits — equal SHA means
	// identical contents. On any error, proceed with the build (conservative).
	subPath := githubconnection.RepoSubPath(conn.RepoFullName)
	if subPath != "" && !args.Force {
		if lastSHA, err := w.ghStore.GetLastRegisteredCommitSHA(ctx, conn.ID); err == nil {
			repoBase := githubconnection.RepoBase(conn.RepoFullName)
			ghClient := github.New(token.AccessToken)
			oldTree, err1 := ghClient.GetSubtreeSHA(ctx, repoBase, lastSHA, subPath)
			newTree, err2 := ghClient.GetSubtreeSHA(ctx, repoBase, args.CommitSHA, subPath)
			if err1 != nil || err2 != nil {
				log.Warn("could not check subpath changes — proceeding with build",
					"subpath", subPath, "err1", err1, "err2", err2)
				// Both guards are load-bearing: "" means the subpath didn't exist at
				// that commit, so "" == "" would incorrectly skip a build where the
				// subpath is absent from both refs.
			} else if oldTree != "" && newTree != "" && oldTree == newTree {
				_ = w.ghStore.UpdateBuildStatus(dbCtx, args.BuildRecordID, "skipped",
					fmt.Sprintf("no changes under %s/", subPath))
				return river.JobCancel(fmt.Errorf("no changes under %s/", subPath))
			}
		}
	}

	agentName := conn.AgentName
	local := w.cfg.Deployment.K8sClientMode == "local"

	pipeline := githubbuild.NewGitHubBuildPipeline(ctx, githubbuild.GitHubBuildConfig{
		Token:       token.AccessToken,
		RepoName:    conn.RepoFullName,
		CommitSHA:   args.CommitSHA,
		AgentName:   agentName,
		BuildID:     args.BuildID,
		AccountID:   conn.AccountID,
		ProxyHost:   w.cfg.Deployment.ProxyRegistryHost,
		RegistryURL: w.cfg.Deployment.RegistryURL,
		Local:       local,
		Builder:     w.builder,
		GHStore:     w.ghStore,
		AgentIndex:  w.agentIndex,
		RecordID:    args.BuildRecordID,
		Log:         w.log,
	})

	if err := pipeline.
		FetchSpec().
		CollectComponents().
		CreateComponentRecords().
		RunBuildJobs().
		FetchReadme().
		TransformSpec().
		StripSecrets().
		Register().
		Err(); err != nil {
		// Classify the error for River retry/cancel semantics
		if errors.Is(err, context.Canceled) {
			return w.cancel(dbCtx, args.BuildRecordID)
		}
		var pe githubbuild.PermanentError
		if errors.As(err, &pe) {
			return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(err))
		}
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, err)
	}

	if err := w.ghStore.UpdateBuildStatus(dbCtx, args.BuildRecordID, "registered", ""); err != nil {
		log.Error("failed to update build status to registered", "error", err, "record_id", args.BuildRecordID)
	}
	log.Info("GitHub build registered", "agent", agentName, "build_id", args.BuildID)

	// Emit synchronously — Work() is already a long-running background job so
	// blocking here is fine and keeps job completion atomic with metering.
	openmeter.EmitAgentBuild(ctx, w.omClient, w.log, conn.AccountID, agentName)
	openmeter.EmitActiveAgents(ctx, w.omClient, w.db, w.log, conn.AccountID)

	return nil
}

// cancel marks the build as cancelled (superseded by a newer push) and tells River not to retry.
func (w *GitHubBuildWorker) cancel(_ context.Context, buildRecordID string) error {
	updateCtx, done := context.WithTimeout(context.Background(), 10*time.Second)
	defer done()
	_ = w.ghStore.CancelBuild(updateCtx, buildRecordID)
	return river.JobCancel(fmt.Errorf("superseded by newer push"))
}

// fail marks the build record as failed and returns err (which may be river.JobCancel).
func (w *GitHubBuildWorker) fail(_ context.Context, buildRecordID string, err error) error {
	updateCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = w.ghStore.UpdateBuildStatus(updateCtx, buildRecordID, "failed", err.Error())
	return err
}

// failOrRetry marks the build as failed only on the last attempt; otherwise it
// returns the raw error so River can schedule a retry without touching DB state.
func (w *GitHubBuildWorker) failOrRetry(ctx context.Context, buildRecordID string, isLastAttempt bool, err error) error {
	if isLastAttempt {
		return w.fail(ctx, buildRecordID, err)
	}
	return err
}
