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

// buildKitImage is the BuildKit rootless daemonless image (Docker Hub, public).
const buildKitImage = "moby/buildkit:v0.21.0-rootless"

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

	local := w.cfg.Deployment.K8sClientMode == "local"
	buildCtx := astroSpec.Agent.Build
	jobName := fmt.Sprintf("build-%s-agent", args.BuildID)

	var destination string
	if !local {
		destination = w.ecrImagePath(conn.AccountID, agentName, args.BuildID)
	}

	log.Info("Starting BuildKit build", "repo", conn.RepoFullName, "local", local)
	if err := w.runBuildKitJob(ctx, jobName, token.AccessToken, conn.RepoFullName, args.CommitSHA, buildCtx.Context, buildCtx.Dockerfile, destination); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("build: %w", err))
	}

	readme, _ := fetchFileContent(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, "AGENT.md")

	var specMap map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
		return w.fail(ctx, args.BuildRecordID, fmt.Errorf("parse spec YAML: %w", err))
	}

	if err := w.agentIndex.Register(
		conn.AccountID, agentName, args.BuildID,
		w.cfg.Deployment.RegistryURL, conn.AccountID,
		specMap, readme, buildAgentCardJSONFromSpec(readme, specMap), "[]",
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

// runBuildKitJob creates a BuildKit K8s Job and waits for completion (max 20 min).
//
// Flow:
//  1. Init container (git-clone): clones the repo at commitSHA into /workspace using
//     an ephemeral K8s Secret for the GitHub token (avoids embedding it in command args).
//  2. Init container (ecr-login, production only): calls ECR via IRSA and writes
//     ~/.docker/config.json to a shared volume for BuildKit to use when pushing.
//  3. BuildKit container: reads /workspace via --local, builds the image, and pushes
//     when destination is set.
//
// When destination is empty (local dev), step 2 is skipped and BuildKit builds
// without pushing.
func (w *GitHubBuildWorker) runBuildKitJob(ctx context.Context, jobName, githubToken, repoFullName, commitSHA, buildContext, dockerfile, destination string) error {
	if w.k8sClient == nil {
		return fmt.Errorf("k8s client not configured")
	}

	ns := w.cfg.GitHub.BuildNamespace
	sa := w.cfg.GitHub.BuildServiceAccount
	clientset := w.k8sClient.Clientset()

	if buildContext == "" {
		buildContext = "."
	}
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	// Create an ephemeral Secret for the GitHub token so it is not visible in Job args.
	tokenSecretName := fmt.Sprintf("build-gh-%s", jobName)
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tokenSecretName, Namespace: ns},
		StringData: map[string]string{"token": githubToken},
	}
	if _, err := clientset.CoreV1().Secrets(ns).Create(ctx, tokenSecret, metav1.CreateOptions{}); err != nil {
		return fmt.Errorf("create token secret: %w", err)
	}
	defer clientset.CoreV1().Secrets(ns).Delete(context.Background(), tokenSecretName, metav1.DeleteOptions{}) //nolint:errcheck

	// git clone command: shallow clone at the exact commit.
	cloneCmd := fmt.Sprintf(
		"git clone --depth 1 https://x-access-token:$(cat /token/token)@github.com/%s.git /workspace && cd /workspace && git fetch --depth 1 origin %s && git checkout %s",
		repoFullName, commitSHA, commitSHA,
	)

	// buildctl args: always use --local so BuildKit reads from the workspace volume.
	contextDir := "/workspace"
	if buildContext != "." {
		contextDir = "/workspace/" + buildContext
	}
	// dockerfile dir is always /workspace; filename is relative to repo root.
	buildctlArgs := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + contextDir,
		"--local", "dockerfile=/workspace",
		"--opt", "filename=" + dockerfile,
	}
	if destination != "" {
		buildctlArgs = append(buildctlArgs, "--output", "type=image,name="+destination+",push=true")
	}

	runAsUser := int64(1000)
	runAsGroup := int64(1000)
	buildKitSecCtx := &corev1.SecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeUnconfined,
		},
	}
	initSecCtx := &corev1.SecurityContext{
		RunAsUser:  &runAsUser,
		RunAsGroup: &runAsGroup,
	}

	buildKitEnv := []corev1.EnvVar{
		{Name: "BUILDKITD_FLAGS", Value: "--oci-worker-no-process-sandbox"},
	}
	volumes := []corev1.Volume{
		{Name: "workspace", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "buildkitd", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
		{Name: "token", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{SecretName: tokenSecretName},
		}},
	}

	initContainers := []corev1.Container{
		{
			Name:            "git-clone",
			Image:           "alpine/git:latest",
			Command:         []string{"sh", "-c", cloneCmd},
			SecurityContext: initSecCtx,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: "/workspace"},
				{Name: "token", MountPath: "/token", ReadOnly: true},
			},
		},
	}

	buildKitVolumeMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: "/workspace", ReadOnly: true},
		{Name: "buildkitd", MountPath: "/home/user/.local/share/buildkit"},
	}

	// Production only: ECR login init container writes docker credentials to a shared volume.
	if destination != "" {
		registryHost := strings.TrimPrefix(w.cfg.Deployment.RegistryURL, "https://")
		ecrLoginCmd := fmt.Sprintf(
			`TOKEN=$(aws ecr get-login-password --region %s); AUTH=$(printf "AWS:%%s" "$TOKEN" | base64 | tr -d '\n'); printf '{"auths":{"%s":{"auth":"%%s"}}}' "$AUTH" > /docker-config/config.json`,
			w.cfg.Deployment.AWSRegion, registryHost,
		)
		volumes = append(volumes, corev1.Volume{
			Name:         "docker-config",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
		initContainers = append(initContainers, corev1.Container{
			Name:            "ecr-login",
			Image:           "amazon/aws-cli:latest",
			Command:         []string{"sh", "-c", ecrLoginCmd},
			SecurityContext: initSecCtx,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "docker-config", MountPath: "/docker-config"},
			},
		})
		buildKitEnv = append(buildKitEnv, corev1.EnvVar{Name: "DOCKER_CONFIG", Value: "/docker-config"})
		buildKitVolumeMounts = append(buildKitVolumeMounts, corev1.VolumeMount{
			Name: "docker-config", MountPath: "/docker-config", ReadOnly: true,
		})
	}

	ttl := int32(3600)
	backoff := int32(0)
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					RestartPolicy:      corev1.RestartPolicyNever,
					Volumes:            volumes,
					InitContainers:     initContainers,
					Containers: []corev1.Container{
						{
							Name:            "buildkit",
							Image:           buildKitImage,
							Command:         []string{"buildctl-daemonless.sh"},
							Args:            buildctlArgs,
							Env:             buildKitEnv,
							SecurityContext: buildKitSecCtx,
							VolumeMounts:    buildKitVolumeMounts,
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

// fetchAstroSpec downloads astropods.yml via the GitHub contents API at a specific SHA.
func fetchAstroSpec(ctx context.Context, token, repoFullName, commitSHA string) (*spec.AstroSpec, string, error) {
	content, err := fetchFileContent(ctx, token, repoFullName, commitSHA, "astropods.yml")
	if err != nil {
		return nil, "", fmt.Errorf("fetch astropods.yml: %w", err)
	}
	var s spec.AstroSpec
	if err := yaml.Unmarshal([]byte(content), &s); err != nil {
		return nil, "", fmt.Errorf("parse astropods.yml: %w", err)
	}
	return &s, content, nil
}

// fetchFileContent fetches a file's raw content from GitHub at a specific ref.
func fetchFileContent(ctx context.Context, token, repoFullName, ref, filePath string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/contents/%s?ref=%s", repoFullName, filePath, ref)
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
