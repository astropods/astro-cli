package riverqueue

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/githubbuild"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
)

const queueGitHubBuild = "github_build"

// GitHubBuildArgs are the job arguments for the GitHub build worker.
type GitHubBuildArgs struct {
	ConnectionID  string `json:"connection_id"`
	CommitSHA     string `json:"commit_sha"`
	BuildID       string `json:"build_id"`
	BuildRecordID string `json:"build_record_id"`
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
}

// NewGitHubBuildWorker creates a GitHubBuildWorker with all dependencies wired.
func NewGitHubBuildWorker(pipesClient *pipes.Client, ghStore *githubconnection.Store, agentIndex *agentindex.Index, k8sClient k8s.ClusterClient, cfg *config.Config, log *logger.Logger) *GitHubBuildWorker {
	return &GitHubBuildWorker{
		pipesClient: pipesClient,
		ghStore:     ghStore,
		agentIndex:  agentIndex,
		builder:     githubbuild.New(k8sClient, cfg, log),
		cfg:         cfg,
		log:         log,
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

	// Atomically transition pending→building. Returns false if a newer push already
	// cancelled this build before the worker picked it up.
	started, err := w.ghStore.StartBuildIfPending(dbCtx, args.BuildRecordID)
	if err != nil {
		log.Error("failed to start build", "error", err)
		// Non-fatal: continue with best-effort status tracking.
	} else if !started {
		log.Info("build superseded by newer push — skipping")
		return river.JobCancel(fmt.Errorf("superseded by newer push"))
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

	w.updateStep(dbCtx, args.BuildRecordID, "fetching-spec")
	astroSpec, specYAML, err := githubbuild.FetchAstroSpec(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA)
	if err != nil {
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("fetch astropods.yml: %w", err))
	}
	if specYAML == "" {
		return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(fmt.Errorf("astropods.yml not found in repo at commit %s", args.CommitSHA[:min(7, len(args.CommitSHA))])))
	}

	agentName := conn.AgentName
	local := w.cfg.Deployment.K8sClientMode == "local"
	builds := githubbuild.CollectComponentBuilds(astroSpec, agentName)

	w.updateStep(dbCtx, args.BuildRecordID, "building")
	log.Info("Starting BuildKit builds", "repo", conn.RepoFullName, "components", len(builds), "local", local)

	for _, cb := range builds {
		jobName := fmt.Sprintf("build-%s-%s", args.BuildID, cb.Suffix)
		var destination string
		if !local {
			destination = w.builder.ECRImagePath(conn.AccountID, cb.Name, args.BuildID)
		}
		log.Info("Building component", "component", cb.Suffix, "destination", destination)
		if err := w.builder.RunJob(ctx, jobName, token.AccessToken, conn.RepoFullName, args.CommitSHA, cb.Build, destination); err != nil {
			if errors.Is(err, context.Canceled) {
				return w.cancel(dbCtx, args.BuildRecordID)
			}
			var bfe githubbuild.BuildFailedError
			if errors.As(err, &bfe) {
				return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(fmt.Errorf("build %s: %w", cb.Suffix, err)))
			}
			return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("build %s: %w", cb.Suffix, err))
		}
	}

	readme, _ := githubbuild.FetchFileContent(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, "AGENT.md")

	var specMap map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
		return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(fmt.Errorf("parse spec YAML: %w", err)))
	}

	// Set the agent image using the proxy registry format so it follows the same
	// resolveImage path as a CLI push: {proxyHost}/{accountID}/{agentName}:{buildID}
	if proxyHost := w.cfg.Deployment.ProxyRegistryHost; proxyHost != "" {
		agentMap, _ := specMap["agent"].(map[string]any)
		if agentMap == nil {
			agentMap = map[string]any{}
			specMap["agent"] = agentMap
		}
		agentMap["image"] = fmt.Sprintf("%s/%s/%s:%s", proxyHost, conn.AccountID, agentName, args.BuildID)
	}

	w.updateStep(dbCtx, args.BuildRecordID, "registering")
	if err := w.agentIndex.Register(
		conn.AccountID, agentName, args.BuildID,
		w.cfg.Deployment.RegistryURL, conn.AccountID,
		specMap, readme, githubbuild.BuildAgentCardJSON(readme, specMap), "[]",
	); err != nil {
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("register agent: %w", err))
	}

	if err := w.ghStore.UpdateBuildStatus(dbCtx, args.BuildRecordID, "registered", ""); err != nil {
		log.Error("failed to update build status to registered", "error", err, "record_id", args.BuildRecordID)
	}
	log.Info("GitHub build registered", "agent", agentName, "build_id", args.BuildID)
	return nil
}

func (w *GitHubBuildWorker) updateStep(ctx context.Context, buildRecordID, step string) {
	if err := w.ghStore.UpdateBuildStep(ctx, buildRecordID, step); err != nil {
		w.log.Error("failed to update build step", "step", step, "error", err)
	}
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
