package riverqueue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
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

// githubHTTPClient is used for GitHub API calls inside the build worker.
// A 30-second timeout bounds any individual request without cutting off
// the overall 25-minute job budget.
var githubHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Pinned init container images — update these explicitly to get new versions.
const (
	buildKitImage = "moby/buildkit:v0.21.0-rootless"
	gitCloneImage = "alpine/git:2.47.2"
	ecrLoginImage = "amazon/aws-cli:2.24.21"
)

// buildFailedError marks a build failure as permanent (bad Dockerfile or code).
// It is distinguished from infrastructure errors, which are retriable.
type buildFailedError struct{ cause error }

func (e buildFailedError) Error() string { return e.cause.Error() }
func (e buildFailedError) Unwrap() error { return e.cause }

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
	k8sClient   k8s.ClusterClient
	cfg         *config.Config
	log         *logger.Logger
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

	if err := w.ghStore.UpdateBuildStatus(dbCtx, args.BuildRecordID, "building", ""); err != nil {
		log.Error("failed to update build status to building", "error", err)
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
		Provider: "github",
		UserID:   conn.WorkOSUserID,
	})
	if err != nil {
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("get github token: %w", err))
	}

	w.updateStep(dbCtx, args.BuildRecordID, "fetching-spec")
	astroSpec, specYAML, err := fetchAstroSpec(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA)
	if err != nil {
		return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("fetch astropods.yml: %w", err))
	}
	if specYAML == "" {
		// Permanent: file doesn't exist at this commit — no point retrying.
		return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(fmt.Errorf("astropods.yml not found in repo at commit %s", args.CommitSHA[:min(7, len(args.CommitSHA))])))
	}

	agentName := conn.AgentName
	local := w.cfg.Deployment.K8sClientMode == "local"

	// Collect all component builds from the spec, mirroring astro push behaviour.
	// Each entry is (jobSuffix, imageName, buildConfig).
	type componentBuild struct {
		suffix string
		name   string
		build  spec.BuildConfig
	}
	var builds []componentBuild

	// Agent (always built if it has a build block; agent.Build is never nil in the struct)
	if astroSpec.Agent.Build != nil {
		builds = append(builds, componentBuild{"agent", agentName, *astroSpec.Agent.Build})
	} else {
		builds = append(builds, componentBuild{"agent", agentName, spec.BuildConfig{}})
	}

	for modelName, model := range astroSpec.Models {
		if model.Container != nil && model.Container.Build != nil {
			builds = append(builds, componentBuild{
				"model-" + modelName,
				fmt.Sprintf("%s-model-%s", agentName, modelName),
				*model.Container.Build,
			})
		}
	}
	for knowledgeName, knowledge := range astroSpec.Knowledge {
		c := knowledge.ResolvedContainer()
		if c.Build != nil {
			builds = append(builds, componentBuild{
				"knowledge-" + knowledgeName,
				fmt.Sprintf("%s-knowledge-%s", agentName, knowledgeName),
				*c.Build,
			})
		}
	}
	for toolName, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			builds = append(builds, componentBuild{
				"tool-" + toolName,
				fmt.Sprintf("%s-tool-%s", agentName, toolName),
				*tool.Container.Build,
			})
		}
	}
	for ingestionName, ingestion := range astroSpec.Ingestion {
		if ingestion.Container.Build != nil {
			builds = append(builds, componentBuild{
				"ingestion-" + ingestionName,
				fmt.Sprintf("%s-ingestion-%s", agentName, ingestionName),
				*ingestion.Container.Build,
			})
		}
	}

	w.updateStep(dbCtx, args.BuildRecordID, "building")
	log.Info("Starting BuildKit builds", "repo", conn.RepoFullName, "components", len(builds), "local", local)

	for _, cb := range builds {
		jobName := fmt.Sprintf("build-%s-%s", args.BuildID, cb.suffix)
		var destination string
		if !local {
			destination = w.ecrImagePath(conn.AccountID, cb.name, args.BuildID)
		}
		log.Info("Building component", "component", cb.suffix, "destination", destination)
		if err := w.runBuildKitJob(ctx, jobName, token.AccessToken, conn.RepoFullName, args.CommitSHA, cb.build, destination); err != nil {
			var bfe buildFailedError
			if errors.As(err, &bfe) {
				return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(fmt.Errorf("build %s: %w", cb.suffix, err)))
			}
			return w.failOrRetry(dbCtx, args.BuildRecordID, isLastAttempt, fmt.Errorf("build %s: %w", cb.suffix, err))
		}
	}

	readme, _ := fetchFileContent(ctx, token.AccessToken, conn.RepoFullName, args.CommitSHA, "AGENT.md")

	var specMap map[string]any
	if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
		// Permanent: spec was already fetched — malformed YAML won't fix itself.
		return w.fail(dbCtx, args.BuildRecordID, river.JobCancel(fmt.Errorf("parse spec YAML: %w", err)))
	}

	w.updateStep(dbCtx, args.BuildRecordID, "registering")
	if err := w.agentIndex.Register(
		conn.AccountID, agentName, args.BuildID,
		w.cfg.Deployment.RegistryURL, conn.AccountID,
		specMap, readme, buildAgentCardJSONFromSpec(readme, specMap), "[]",
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

// EnsureBuildInfrastructure creates the build namespace and service account if
// they don't already exist. Called once at server startup rather than per-build.
func (w *GitHubBuildWorker) EnsureBuildInfrastructure(ctx context.Context) error {
	if w.k8sClient == nil {
		return nil
	}
	ns := w.cfg.GitHub.BuildNamespace
	sa := w.cfg.GitHub.BuildServiceAccount
	clientset := w.k8sClient.Clientset()

	_, nsErr := clientset.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if nsErr != nil && !k8serrors.IsAlreadyExists(nsErr) {
		return fmt.Errorf("ensure build namespace: %w", nsErr)
	}
	_, saErr := clientset.CoreV1().ServiceAccounts(ns).Create(ctx, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: sa, Namespace: ns},
	}, metav1.CreateOptions{})
	if saErr != nil && !k8serrors.IsAlreadyExists(saErr) {
		return fmt.Errorf("ensure build service account: %w", saErr)
	}
	return nil
}

func (w *GitHubBuildWorker) ecrImagePath(accountID, agentName, buildID string) string {
	cfg := w.cfg.Deployment
	return fmt.Sprintf("%s/%s-tenant-%s/%s:%s",
		strings.TrimPrefix(cfg.RegistryURL, "https://"),
		cfg.Environment, accountID,
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
func (w *GitHubBuildWorker) runBuildKitJob(ctx context.Context, jobName, githubToken, repoFullName, commitSHA string, build spec.BuildConfig, destination string) error {
	if w.k8sClient == nil {
		return fmt.Errorf("k8s client not configured")
	}

	ns := w.cfg.GitHub.BuildNamespace
	sa := w.cfg.GitHub.BuildServiceAccount
	clientset := w.k8sClient.Clientset()

	buildContext := build.Context
	dockerfile := build.Dockerfile
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

	// git clone command: shallow clone at the exact commit.
	// HOME=/tmp gives git a writable directory for the global config (user 1000 has no home).
	// safe.directory bypasses Git's ownership check on the emptyDir volume (owned by root).
	cloneCmd := fmt.Sprintf(
		"HOME=/tmp git config --global --add safe.directory /workspace && git clone --depth 1 https://x-access-token:$(cat /token/token)@github.com/%s.git /workspace && cd /workspace && HOME=/tmp git fetch --depth 1 origin %s && HOME=/tmp git checkout %s",
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
	if build.Target != "" {
		buildctlArgs = append(buildctlArgs, "--opt", "target="+build.Target)
	}
	for k, v := range build.Args {
		buildctlArgs = append(buildctlArgs, "--opt", "build-arg:"+k+"="+v)
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
			Image:           gitCloneImage,
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
			Image:           ecrLoginImage,
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

	createdJob, err := clientset.BatchV1().Jobs(ns).Create(ctx, job, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create build job: %w", err)
	}

	// Bind the token secret to the job via an owner reference so K8s garbage-collects
	// it automatically whenever the job is deleted (TTL or the defer below).
	isController := true
	tokenSecret.OwnerReferences = []metav1.OwnerReference{{
		APIVersion: "batch/v1",
		Kind:       "Job",
		Name:       createdJob.Name,
		UID:        createdJob.UID,
		Controller: &isController,
	}}
	if _, err := clientset.CoreV1().Secrets(ns).Update(ctx, tokenSecret, metav1.UpdateOptions{}); err != nil {
		// Non-fatal: the secret will still be cleaned up by the job deletion defer.
		w.log.Warn("failed to set owner reference on token secret", "error", err)
	}

	// Delete the job on any non-success path (cancel, timeout, build failure) to
	// stop wasted compute. On success the TTL handles cleanup.
	succeeded := false
	defer func() {
		if !succeeded {
			deleteCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			propagation := metav1.DeletePropagationBackground
			_ = clientset.BatchV1().Jobs(ns).Delete(deleteCtx, jobName, metav1.DeleteOptions{
				PropagationPolicy: &propagation,
			})
		}
	}()

	pollCtx, pollCancel := context.WithTimeout(ctx, 20*time.Minute)
	defer pollCancel()

	for {
		select {
		case <-pollCtx.Done():
			if ctx.Err() != nil {
				return ctx.Err() // parent cancelled
			}
			return fmt.Errorf("build job %s timed out after 20 minutes", jobName)
		case <-time.After(15 * time.Second):
		}
		j, err := clientset.BatchV1().Jobs(ns).Get(pollCtx, jobName, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if j.Status.Succeeded > 0 {
			succeeded = true
			return nil
		}
		if j.Status.Failed > 0 {
			logs := w.fetchJobLogs(context.Background(), ns, jobName)
			return buildFailedError{fmt.Errorf("build job failed: %s", extractBuildError(logs))}
		}
	}
}

// fetchJobLogs retrieves the tail of logs from all containers of a Job's pod.
// Tries each container in order (init containers first) and concatenates the output.
// Returns a best-effort string — never errors, so it's safe to use in error messages.
func (w *GitHubBuildWorker) fetchJobLogs(ctx context.Context, ns, jobName string) string {
	clientset := w.k8sClient.Clientset()
	pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil || len(pods.Items) == 0 {
		return "(could not retrieve pod logs)"
	}
	pod := pods.Items[0]

	var sb strings.Builder
	tailLines := int64(100)

	// Collect container names in order: init containers first, then main.
	var containers []string
	for _, c := range pod.Spec.InitContainers {
		containers = append(containers, c.Name)
	}
	for _, c := range pod.Spec.Containers {
		containers = append(containers, c.Name)
	}

	for _, name := range containers {
		req := clientset.CoreV1().Pods(ns).GetLogs(pod.Name, &corev1.PodLogOptions{
			Container: name,
			TailLines: &tailLines,
		})
		stream, err := req.Stream(ctx)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(stream)
		_ = stream.Close()
		if len(body) > 0 {
			fmt.Fprintf(&sb, "=== %s ===\n%s\n", name, string(body))
		}
	}
	if sb.Len() == 0 {
		return "(no logs available)"
	}
	return sb.String()
}

// extractBuildError returns a concise failure reason from raw job logs.
// It walks lines in reverse and returns the last non-empty, non-header line,
// truncated to 500 chars. Full logs are still readable via the logs endpoint.
func extractBuildError(logs string) string {
	lines := strings.Split(logs, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" || strings.HasPrefix(line, "===") {
			continue
		}
		if len(line) > 500 {
			line = line[:500]
		}
		return line
	}
	return "build job failed (no output captured)"
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
	req.Header.Set("X-Github-Api-Version", "2022-11-28")

	resp, err := githubHTTPClient.Do(req)
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
	out, err := json.Marshal(card)
	if err != nil {
		return ""
	}
	return string(out)
}
