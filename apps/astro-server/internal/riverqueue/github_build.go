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

// buildKitImage is the BuildKit daemonless executor image (Docker Hub, public).
const buildKitImage = "moby/buildkit:v0.21.0"

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
		MaxAttempts: 1,
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
	k8sClient   k8s.ClusterClient
	s3Client    *s3.Client
	cfg         *config.Config
	log         *logger.Logger
}

func (w *GitHubBuildWorker) Work(ctx context.Context, job *river.Job[GitHubBuildArgs]) error {
	args := job.Args
	log := w.log.With("build_id", args.BuildID, "commit", args.CommitSHA[:min(7, len(args.CommitSHA))])

	_ = w.ghStore.UpdateBuildStatus(ctx, args.BuildRecordID, "building", "")

	conn, err := w.ghStore.GetByID(ctx, args.ConnectionID)
	if err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("load connection: %w", err))
	}

	token, err := w.pipesClient.GetAccessToken(ctx, pipes.GetAccessTokenInput{
		Provider: "github",
		UserID:   conn.WorkOSUserID,
	})
	if err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("get github token: %w", err))
	}

	astroSpec, specYAML, err := fetchAstroSpec(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA)
	if err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("fetch astropods.yml: %w", err))
	}

	agentName := strings.TrimPrefix(astroSpec.Name, "@"+conn.AccountID+"/")
	agentName = strings.TrimPrefix(agentName, "@")
	if idx := strings.Index(agentName, "/"); idx >= 0 {
		agentName = agentName[idx+1:]
	}

	buildCtx := astroSpec.Agent.Build
	jobName := fmt.Sprintf("build-%s-agent", args.BuildID)
	local := w.cfg.Deployment.K8sClientMode == "local"

	if local {
		// Local dev: BuildKit uses a git URL as context and skips the push.
		gitURL := fmt.Sprintf("https://x-access-token:%s@github.com/%s.git#%s",
			token.AccessToken, conn.RepoFullName, args.CommitSHA)
		log.Info("Starting BuildKit build (local, no-push)", "repo", conn.RepoFullName)
		if err := w.runBuildKitJob(ctx, jobName, gitURL, buildCtx.Context, buildCtx.Dockerfile, "", true); err != nil {
			return w.fail(ctx, args.BuildRecordID, fmt.Errorf("build: %w", err))
		}
	} else {
		// Production: upload tarball to S3, build with BuildKit, push to ECR.
		contextKey := fmt.Sprintf("github-builds/%s/%s/%s.tar.gz", conn.AccountID, args.BuildID, args.BuildID)
		if err := w.uploadBuildContext(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, contextKey); err != nil {
			return w.fail(ctx, args.BuildRecordID, fmt.Errorf("upload build context: %w", err))
		}
		ecrImage := w.ecrImagePath(conn.AccountID, agentName, args.BuildID)
		log.Info("Starting BuildKit build", "repo", conn.RepoFullName, "image", ecrImage)
		if err := w.runBuildKitJob(ctx, jobName, contextKey, buildCtx.Context, buildCtx.Dockerfile, ecrImage, false); err != nil {
			return w.fail(ctx, args.BuildRecordID, fmt.Errorf("build: %w", err))
		}
	}

	readme, _ := fetchFileContent(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, "AGENT.md")

	var specMap map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("parse spec YAML: %w", err))
	}

	agentCardJSON := buildAgentCardJSONFromSpec(readme, specMap)

	if err := w.agentIndex.Register(
		conn.AccountID, agentName, args.BuildID,
		w.cfg.Deployment.RegistryURL, conn.AccountID,
		specMap, readme, agentCardJSON, "[]",
	); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("register agent: %w", err))
	}

	_ = w.ghStore.UpdateBuildStatus(ctx, args.BuildRecordID, "registered", "")
	log.Info("GitHub build registered", "agent", agentName, "build_id", args.BuildID)
	return nil
}

func (w *GitHubBuildWorker) fail(ctx context.Context, buildRecordID string, err error) error {
	_ = w.ghStore.UpdateBuildStatus(ctx, buildRecordID, "failed", err.Error())
	return err
}

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

// runBuildKitJob creates a BuildKit K8s Job and waits for completion (max 20 min).
//
// Local dev (noPush=true): single BuildKit container with a git URL context; no
// --output flag so BuildKit builds without pushing anywhere.
//
// Production (noPush=false): init container downloads + extracts the S3 tarball
// into a shared volume; BuildKit reads from that volume and pushes to ECR.
func (w *GitHubBuildWorker) runBuildKitJob(ctx context.Context, jobName, contextArg, buildContext, dockerfile, destination string, noPush bool) error {
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

	ttl := int32(3600)
	backoff := int32(0)
	privileged := true

	var jobSpec batchv1.JobSpec

	if noPush {
		// Local: BuildKit fetches the git context itself; no init container needed.
		jobSpec = batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					RestartPolicy:      corev1.RestartPolicyNever,
					Containers: []corev1.Container{
						{
							Name:  "buildkit",
							Image: buildKitImage,
							Command: []string{
								"buildctl-daemonless.sh", "build",
								"--frontend", "dockerfile.v0",
								"--opt", "context=" + contextArg,
								"--opt", "context-subpath=" + buildContext,
								"--opt", "filename=" + dockerfile,
								// No --output → build only, no push.
							},
							SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
							Resources:       buildKitResources(),
						},
					},
				},
			},
		}
	} else {
		// Production: init container extracts the S3 tarball; BuildKit reads from shared volume.
		const workspaceDir = "/workspace"
		s3URI := fmt.Sprintf("s3://%s/%s", w.cfg.GitHub.BuildContextBucket, contextArg)

		jobSpec = batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes: []corev1.Volume{
						{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "fetch-context",
							Image: "amazon/aws-cli:latest",
							Command: []string{
								"sh", "-c",
								fmt.Sprintf("aws s3 cp %s - | tar -xz -C %s --strip-components=1", s3URI, workspaceDir),
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: workspaceDir},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "buildkit",
							Image: buildKitImage,
							Command: []string{
								"buildctl-daemonless.sh", "build",
								"--frontend", "dockerfile.v0",
								"--local", "context=" + workspaceDir + "/" + buildContext,
								"--local", "dockerfile=" + workspaceDir + "/" + buildContext,
								"--opt", "filename=" + dockerfile,
								"--output", "type=image,name=" + destination + ",push=true",
							},
							SecurityContext: &corev1.SecurityContext{Privileged: &privileged},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "workspace", MountPath: workspaceDir},
							},
							Resources: buildKitResources(),
						},
					},
				},
			},
		}
	}

	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
		Spec:       jobSpec,
	}

	clientset := w.k8sClient.Clientset()
	if _, err := clientset.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create build job: %w", err)
	}

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
			return fmt.Errorf("build job %s failed", jobName)
		}
	}
	return fmt.Errorf("build job %s timed out after 20 minutes", jobName)
}

func buildKitResources() corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
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
