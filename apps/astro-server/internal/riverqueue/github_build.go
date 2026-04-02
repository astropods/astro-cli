package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	spec "github.com/astropods/astro/packages/astro-spec"
	"gopkg.in/yaml.v3"
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
		MaxAttempts: 1, // failures are surfaced as build records, not retried
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

// GitHubBuildWorker clones a GitHub repo at a specific commit, builds container
// images via Kaniko, and registers the result in the agent index.
type GitHubBuildWorker struct {
	river.WorkerDefaults[GitHubBuildArgs]
	pipesClient *pipes.Client
	ghStore     *githubconnection.Store
	agentIndex  *agentindex.Index
	k8sClient   k8s.ClusterClient
	s3Client    *s3.Client
	cfg         *config.Config
	log         *logger.Logger
}

func (w *GitHubBuildWorker) Work(ctx context.Context, job *river.Job[GitHubBuildArgs]) error {
	args := job.Args
	log := w.log.With("build_id", args.BuildID, "commit", args.CommitSHA[:min(7, len(args.CommitSHA))])

	// Mark as building.
	_ = w.ghStore.UpdateBuildStatus(ctx, args.BuildRecordID, "building", "")

	conn, err := w.ghStore.GetByID(ctx, args.ConnectionID)
	if err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("load connection: %w", err))
	}

	// Fetch GitHub token from WorkOS Pipes.
	token, err := w.pipesClient.GetAccessToken(ctx, pipes.GetAccessTokenInput{
		Provider: "github",
		UserID:   conn.WorkOSUserID,
	})
	if err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("get github token: %w", err))
	}

	// Fetch astropods.yml from GitHub at the exact commit SHA.
	astroSpec, specYAML, err := fetchAstroSpec(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA)
	if err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("fetch astropods.yml: %w", err))
	}

	agentName := strings.TrimPrefix(astroSpec.Name, "@"+conn.AccountID+"/")
	agentName = strings.TrimPrefix(agentName, "@")
	if idx := strings.Index(agentName, "/"); idx >= 0 {
		agentName = agentName[idx+1:]
	}

	// Upload repo tarball to S3 as Kaniko build context.
	contextKey := fmt.Sprintf("github-builds/%s/%s/%s.tar.gz", conn.AccountID, args.BuildID, args.BuildID)
	if err := w.uploadBuildContext(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, contextKey); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("upload build context: %w", err))
	}

	// Build agent container image via Kaniko.
	ecrImage := w.ecrImagePath(conn.AccountID, agentName, args.BuildID)
	buildCtx := astroSpec.Agent.Build

	log.Info("Starting Kaniko build", "repo", conn.RepoFullName, "image", ecrImage)
	jobName := fmt.Sprintf("build-%s-agent", args.BuildID)
	dockerfile := buildCtx.Dockerfile
	buildContext := buildCtx.Context
	if err := w.runKanikoJob(ctx, jobName, contextKey, buildContext, dockerfile, ecrImage); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("kaniko build: %w", err))
	}

	// Fetch README if present.
	readme, _ := fetchFileContent(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, "AGENT.md")

	// Register agent in index.
	var specMap map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("parse spec YAML: %w", err))
	}

	agentCardJSON := buildAgentCardJSONFromSpec(readme, specMap)

	registryURL := w.cfg.Deployment.RegistryURL
	if err := w.agentIndex.Register(
		conn.AccountID, agentName, args.BuildID,
		registryURL, conn.AccountID,
		specMap, readme, agentCardJSON, "[]",
	); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("register agent: %w", err))
	}

	_ = w.ghStore.UpdateBuildStatus(ctx, args.BuildRecordID, "registered", "")
	log.Info("GitHub build registered", "agent", agentName, "build_id", args.BuildID)
	return nil
}

// fail marks the build as failed and returns the error so River logs it.
func (w *GitHubBuildWorker) fail(ctx context.Context, buildRecordID string, err error) error {
	_ = w.ghStore.UpdateBuildStatus(ctx, buildRecordID, "failed", err.Error())
	return err
}

// ecrImagePath constructs the ECR destination for a Kaniko push.
func (w *GitHubBuildWorker) ecrImagePath(accountName, agentName, buildID string) string {
	cfg := w.cfg.Deployment
	return fmt.Sprintf("%s/%s-tenant-%s/%s:%s",
		strings.TrimPrefix(cfg.RegistryURL, "https://"),
		cfg.Environment, accountName,
		agentName, buildID,
	)
}

// uploadBuildContext downloads the GitHub repo tarball at commitSHA and uploads it to S3.
func (w *GitHubBuildWorker) uploadBuildContext(ctx context.Context, token, repoFullName, commitSHA, s3Key string) error {
	url := fmt.Sprintf("https://api.github.com/repos/%s/tarball/%s", repoFullName, commitSHA)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("download tarball: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("download tarball: status %d", resp.StatusCode)
	}

	_, err = w.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(w.cfg.GitHub.BuildContextBucket),
		Key:         aws.String(s3Key),
		Body:        resp.Body,
		ContentType: aws.String("application/gzip"),
	})
	return err
}

// runKanikoJob creates a Kaniko K8s Job and waits for it to complete (max 20 min).
func (w *GitHubBuildWorker) runKanikoJob(ctx context.Context, jobName, s3ContextKey, buildContext, dockerfile, destination string) error {
	if w.k8sClient == nil {
		return fmt.Errorf("k8s client not configured")
	}

	ns := w.cfg.GitHub.BuildNamespace
	sa := w.cfg.GitHub.BuildServiceAccount

	if buildContext == "" {
		buildContext = "."
	}
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	s3URL := fmt.Sprintf("s3://%s/%s", w.cfg.GitHub.BuildContextBucket, s3ContextKey)

	ttl := int32(3600)
	backoff := int32(0)
	kanikoJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: ns,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "kaniko",
							Image: "gcr.io/kaniko-project/executor:latest",
							Args: []string{
								"--context=" + s3URL,
								"--context-sub-path=" + buildContext,
								"--dockerfile=" + dockerfile,
								"--destination=" + destination,
								"--cache=true",
								"--cache-repo=" + strings.Split(destination, ":")[0],
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
						},
					},
				},
			},
		},
	}

	clientset := w.k8sClient.Clientset()

	if _, err := clientset.BatchV1().Jobs(ns).Create(ctx, kanikoJob, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create kaniko job: %w", err)
	}

	// Poll for completion (max 20 minutes).
	deadline := time.Now().Add(20 * time.Minute)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(15 * time.Second):
		}

		j, err := clientset.BatchV1().Jobs(ns).Get(ctx, jobName, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if j.Status.Succeeded > 0 {
			return nil
		}
		if j.Status.Failed > 0 {
			return fmt.Errorf("kaniko job %s failed", jobName)
		}
	}
	return fmt.Errorf("kaniko job %s timed out after 20 minutes", jobName)
}

// fetchAstroSpec downloads astropods.yml via the GitHub contents API at a specific SHA.
func fetchAstroSpec(ctx context.Context, token, repoFullName, commitSHA string) (*spec.AstroSpec, string, error) {
	content, err := fetchFileContent(ctx, token, repoFullName, commitSHA, "astropods.yml")
	if err != nil {
		return nil, "", fmt.Errorf("fetch astropods.yml: %w", err)
	}

	var astroSpec spec.AstroSpec
	if err := yaml.Unmarshal([]byte(content), &astroSpec); err != nil {
		return nil, "", fmt.Errorf("parse astropods.yml: %w", err)
	}
	return &astroSpec, content, nil
}

// fetchFileContent fetches a file's raw content from GitHub at a specific ref.
func fetchFileContent(ctx context.Context, token, repoFullName, ref, path string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", repoFullName, path, ref)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("GET %s returned %d", url, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// buildAgentCardJSONFromSpec mirrors the logic in handlers/agents.go for agent card generation.
func buildAgentCardJSONFromSpec(readme string, specMap map[string]any) string {
	if readme == "" {
		return ""
	}
	card, err := spec.ParseAgentCard(readme)
	if err != nil || card == nil {
		return ""
	}
	// Merge providers from spec integrations.
	var providers []string
	if integrations, ok := specMap["integrations"].(map[string]any); ok {
		for _, v := range integrations {
			if entry, ok := v.(map[string]any); ok {
				if p, ok := entry["provider"].(string); ok && p != "" {
					providers = append(providers, p)
				}
			}
		}
	}
	card.ResolvedIntegrations = spec.MergeResolvedIntegrations(card.ResolvedIntegrations, providers)
	out, _ := json.Marshal(card)
	return string(out)
}
