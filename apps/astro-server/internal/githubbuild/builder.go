// Package githubbuild handles building container images from GitHub repos using
// BuildKit running as Kubernetes Jobs.
package githubbuild

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro-spec"
)

// Pinned init container images — update these explicitly to get new versions.
const (
	BuildKitImage = "moby/buildkit:v0.21.0-rootless"
	GitCloneImage = "alpine/git:2.47.2"
	ECRLoginImage = "amazon/aws-cli:2.24.21"
)

// workspaceDir is the shared emptyDir mount path used by the git-clone init
// container and the BuildKit container within each build Job pod.
const workspaceDir = "/workspace"

// BuildFailedError marks a build failure as permanent (bad Dockerfile or code),
// distinguishing it from retriable infrastructure errors.
type BuildFailedError struct{ Cause error }

func (e BuildFailedError) Error() string { return e.Cause.Error() }
func (e BuildFailedError) Unwrap() error { return e.Cause }

// PermanentError wraps an error that should never be retried (e.g. spec not
// found, YAML parse error, build code failure). Pipeline steps wrap permanent
// failures with this type; the River worker uses it to choose JobCancel vs retry.
type PermanentError struct{ Err error }

func (e PermanentError) Error() string { return e.Err.Error() }
func (e PermanentError) Unwrap() error { return e.Err }

// SpecError marks a failure the reader fixes in their own repo: astropods.yml is
// missing, unparsable, or invalid. Reason reaches them verbatim in the
// build.failed notification, so it names the commit or the offending line
// instead of describing the category. Err keeps the engineer phrasing for the
// log and the build record, and may be nil when the two would say the same
// thing.
type SpecError struct {
	Reason string
	Err    error
}

func (e SpecError) Error() string {
	if e.Err != nil {
		return e.Err.Error()
	}
	return e.Reason
}

func (e SpecError) Unwrap() error { return e.Err }

// ecrAPI is the subset of the ECR client used by EnsureRepository.
type ecrAPI interface {
	DescribeRepositories(ctx context.Context, params *ecr.DescribeRepositoriesInput, optFns ...func(*ecr.Options)) (*ecr.DescribeRepositoriesOutput, error)
	CreateRepository(ctx context.Context, params *ecr.CreateRepositoryInput, optFns ...func(*ecr.Options)) (*ecr.CreateRepositoryOutput, error)
}

// Builder runs BuildKit K8s Jobs to build and push container images.
type Builder struct {
	k8sClient k8s.ClusterClient
	cfg       *config.Config
	log       *logger.Logger
	ecr       ecrAPI // nil → created lazily from AWS config
}

// New creates a Builder. k8sClient may be nil (local dev — builds run without pushing).
func New(k8sClient k8s.ClusterClient, cfg *config.Config, log *logger.Logger) *Builder {
	return &Builder{k8sClient: k8sClient, cfg: cfg, log: log}
}

// ECRImagePath returns the full ECR image URI for a component build.
func (b *Builder) ECRImagePath(accountID, imageName, buildID string) string {
	cfg := b.cfg.Deployment
	return fmt.Sprintf("%s/%s-tenant-%s/%s:%s",
		strings.TrimPrefix(cfg.RegistryURL, "https://"),
		cfg.Environment, accountID,
		imageName, buildID,
	)
}

// effectivePaths resolves the BuildKit context directory and dockerfile path
// given an optional subpath within the cloned workspace.
func effectivePaths(subPath, buildContext, dockerfile string) (contextDir, effectiveDockerfile string) {
	base := workspaceDir
	if subPath != "" {
		base = workspaceDir + "/" + subPath
	}
	if buildContext == "." {
		contextDir = base
	} else {
		contextDir = base + "/" + buildContext
	}
	if subPath != "" {
		effectiveDockerfile = subPath + "/" + dockerfile
	} else {
		effectiveDockerfile = dockerfile
	}
	return contextDir, effectiveDockerfile
}

// ecrRepoName extracts the ECR repository name from a full image destination.
// E.g. "123456.dkr.ecr.us-east-1.amazonaws.com/prod-tenant-abc/myapp:build123"
// → "prod-tenant-abc/myapp"
func ecrRepoName(destination string) (string, error) {
	slashIdx := strings.Index(destination, "/")
	colonIdx := strings.LastIndex(destination, ":")
	if slashIdx < 0 || colonIdx < 0 || colonIdx <= slashIdx {
		return "", fmt.Errorf("invalid ECR destination: %s", destination)
	}
	return destination[slashIdx+1 : colonIdx], nil
}

// EnsureRepository ensures the ECR repository for the given destination exists,
// creating it if necessary. Must be called before RunJob on the first build for
// a given agent, since BuildKit pushes directly to ECR (bypassing the registry proxy
// where repository auto-creation normally happens).
func (b *Builder) EnsureRepository(ctx context.Context, destination string) error {
	repoName, err := ecrRepoName(destination)
	if err != nil {
		return err
	}

	client, err := b.getECRClient(ctx)
	if err != nil {
		return err
	}

	_, err = client.DescribeRepositories(ctx, &ecr.DescribeRepositoriesInput{
		RepositoryNames: []string{repoName},
	})
	if err == nil {
		return nil // already exists
	}

	b.log.Info("Creating ECR repository", "repo", repoName)
	_, err = client.CreateRepository(ctx, &ecr.CreateRepositoryInput{
		RepositoryName: &repoName,
	})
	if err != nil {
		if strings.Contains(err.Error(), "RepositoryAlreadyExistsException") {
			return nil
		}
		return fmt.Errorf("create ECR repository %s: %w", repoName, err)
	}
	return nil
}

// getECRClient returns the ECR client, creating one from AWS config if not injected.
func (b *Builder) getECRClient(ctx context.Context) (ecrAPI, error) {
	if b.ecr != nil {
		return b.ecr, nil
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(b.cfg.Deployment.AWSRegion))
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}
	return ecr.NewFromConfig(cfg), nil
}

// EnsureInfrastructure creates the build namespace and service account if they
// don't already exist. Call once at server startup rather than per-build.
func (b *Builder) EnsureInfrastructure(ctx context.Context) error {
	if b.k8sClient == nil {
		return nil
	}
	ns := b.cfg.GitHub.BuildNamespace
	sa := b.cfg.GitHub.BuildServiceAccount
	clientset := b.k8sClient.Clientset()

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

// RunJob creates a BuildKit K8s Job and waits for completion (max 20 min).
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
// without pushing. Returns (logs, nil) on success, (logs, BuildFailedError) for
// permanent failures, or ("", error) for retriable infrastructure failures.
func (b *Builder) RunJob(ctx context.Context, jobName, githubToken, repoFullName, commitSHA string, build spec.BuildConfig, destination string) (string, error) {
	if b.k8sClient == nil {
		return "", fmt.Errorf("k8s client not configured")
	}

	ns := b.cfg.GitHub.BuildNamespace
	sa := b.cfg.GitHub.BuildServiceAccount
	clientset := b.k8sClient.Clientset()

	buildContext := build.Context
	dockerfile := build.Dockerfile
	if buildContext == "" {
		buildContext = "."
	}
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	// Split repoFullName into base repo (for clone URL) and subpath (for file paths).
	base := githubconnection.RepoBase(repoFullName)
	subPath := githubconnection.RepoSubPath(repoFullName)
	contextDir, effectiveDockerfile := effectivePaths(subPath, buildContext, dockerfile)

	// Create an ephemeral Secret for the GitHub token so it is not visible in Job args.
	tokenSecretName := fmt.Sprintf("build-gh-%s", jobName)
	tokenSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: tokenSecretName, Namespace: ns},
		StringData: map[string]string{"token": githubToken},
	}
	if _, err := clientset.CoreV1().Secrets(ns).Create(ctx, tokenSecret, metav1.CreateOptions{}); err != nil {
		return "", fmt.Errorf("create token secret: %w", err)
	}

	// git clone command: shallow clone at the exact commit.
	// HOME=/tmp gives git a writable directory for the global config (user 1000 has no home).
	// safe.directory bypasses Git's ownership check on the emptyDir volume (owned by root).
	cloneCmd := fmt.Sprintf(
		"HOME=/tmp git config --global --add safe.directory "+workspaceDir+" && git clone --depth 1 https://x-access-token:$(cat /token/token)@github.com/%s.git "+workspaceDir+" && cd "+workspaceDir+" && HOME=/tmp git fetch --depth 1 origin %s && HOME=/tmp git -c advice.detachedHead=false checkout %s",
		base, commitSHA, commitSHA,
	)

	// buildctl args: always use --local so BuildKit reads from the workspace volume.
	// dockerfile dir is always /workspace; filename is relative to workspace root.
	buildctlArgs := []string{
		"build",
		"--frontend", "dockerfile.v0",
		"--local", "context=" + contextDir,
		"--local", "dockerfile=" + workspaceDir,
		"--opt", "filename=" + effectiveDockerfile,
	}
	if build.Target != "" {
		buildctlArgs = append(buildctlArgs, "--opt", "target="+build.Target)
	}
	for k, v := range build.Args {
		buildctlArgs = append(buildctlArgs, "--opt", "build-arg:"+k+"="+v)
	}
	if destination != "" {
		buildctlArgs = append(buildctlArgs, "--output", "type=image,name="+destination+",push=true")
		// Use a stable :cache tag in the same ECR repo for layer caching.
		// import-cache is best-effort — BuildKit ignores it if no cache exists yet.
		cacheRef := destination[:strings.LastIndex(destination, ":")] + ":cache"
		buildctlArgs = append(buildctlArgs,
			"--import-cache", "type=registry,ref="+cacheRef,
			"--export-cache", "type=registry,ref="+cacheRef+",mode=max,image-manifest=true,oci-mediatypes=true",
		)
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

	buildkitdFlags := "--oci-worker-no-process-sandbox"
	if b.cfg.GitHub.BuildKitConfigMap != "" {
		buildkitdFlags += " --config=/etc/buildkit/buildkitd.toml"
	}
	buildKitEnv := []corev1.EnvVar{
		{Name: "BUILDKITD_FLAGS", Value: buildkitdFlags},
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
			Image:           GitCloneImage,
			Command:         []string{"sh", "-c", cloneCmd},
			SecurityContext: initSecCtx,
			VolumeMounts: []corev1.VolumeMount{
				{Name: "workspace", MountPath: workspaceDir},
				{Name: "token", MountPath: "/token", ReadOnly: true},
			},
		},
	}
	buildKitVolumeMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: workspaceDir, ReadOnly: true},
		{Name: "buildkitd", MountPath: "/home/user/.local/share/buildkit"},
	}
	if b.cfg.GitHub.BuildKitConfigMap != "" {
		volumes = append(volumes, corev1.Volume{
			Name: "buildkit-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: b.cfg.GitHub.BuildKitConfigMap},
				},
			},
		})
		buildKitVolumeMounts = append(buildKitVolumeMounts, corev1.VolumeMount{
			Name:      "buildkit-config",
			MountPath: "/etc/buildkit",
			ReadOnly:  true,
		})
	}

	// Production only: ECR login init container writes docker credentials to a shared volume.
	if destination != "" {
		registryHost := strings.TrimPrefix(b.cfg.Deployment.RegistryURL, "https://")
		repoName, _ := ecrRepoName(destination) // destination is always valid (produced by ECRImagePath)
		ecrLoginCmd := fmt.Sprintf(
			`export HOME=/tmp; aws ecr create-repository --region %s --repository-name %s 2>/dev/null || true; TOKEN=$(aws ecr get-login-password --region %s) || { echo "ERROR: ECR login failed" >&2; exit 1; }; AUTH=$(printf "AWS:%%s" "$TOKEN" | base64 | tr -d '\n'); printf '{"auths":{"%s":{"auth":"%%s"}}}' "$AUTH" > /docker-config/config.json && echo "ECR login successful for %s"`,
			b.cfg.Deployment.AWSRegion, repoName, b.cfg.Deployment.AWSRegion, registryHost, registryHost,
		)
		volumes = append(volumes, corev1.Volume{
			Name:         "docker-config",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
		initContainers = append(initContainers, corev1.Container{
			Name:            "ecr-login",
			Image:           ECRLoginImage,
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
	k8sJob := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: ns},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoff,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					ServiceAccountName: sa,
					RestartPolicy:      corev1.RestartPolicyNever,
					NodeSelector: map[string]string{
						"workload-type": "build",
					},
					Tolerations: []corev1.Toleration{{
						Key:      "astro.dev/build",
						Operator: corev1.TolerationOpExists,
						Effect:   corev1.TaintEffectNoSchedule,
					}},
					Volumes:        volumes,
					InitContainers: initContainers,
					Containers: []corev1.Container{
						{
							Name:            "buildkit",
							Image:           BuildKitImage,
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

	createdJob, err := clientset.BatchV1().Jobs(ns).Create(ctx, k8sJob, metav1.CreateOptions{})
	if err != nil {
		return "", fmt.Errorf("create build job: %w", err)
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
		b.log.Warn("failed to set owner reference on token secret", "error", err)
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
				return "", ctx.Err()
			}
			return "", fmt.Errorf("build job %s timed out after 20 minutes", jobName)
		case <-time.After(15 * time.Second):
		}
		j, err := clientset.BatchV1().Jobs(ns).Get(pollCtx, jobName, metav1.GetOptions{})
		if err != nil {
			b.log.Warn("failed to get build job status", "job", jobName, "error", err)
			if k8serrors.IsNotFound(err) {
				return "", fmt.Errorf("build job %s was deleted before completion", jobName)
			}
			continue
		}
		if j.Status.Succeeded > 0 {
			succeeded = true
			logs := b.fetchJobLogs(context.Background(), ns, jobName)
			return logs, nil
		}
		if j.Status.Failed > 0 {
			logs := b.fetchJobLogs(context.Background(), ns, jobName)
			return logs, BuildFailedError{fmt.Errorf("build job failed: %s", extractBuildError(logs))}
		}
	}
}

// fetchJobLogs retrieves the tail of logs from all containers of a Job's pod.
// Returns a best-effort string — never errors, so it's safe to use in error messages.
func (b *Builder) fetchJobLogs(ctx context.Context, ns, jobName string) string {
	clientset := b.k8sClient.Clientset()
	pods, err := clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "job-name=" + jobName,
	})
	if err != nil || len(pods.Items) == 0 {
		return "(could not retrieve pod logs)"
	}
	pod := pods.Items[0]

	var sb strings.Builder
	tailLines := int64(500)

	// Events first — scheduling/pull errors are the most common build issues.
	events, evErr := clientset.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: "involvedObject.name=" + pod.Name,
	})
	if evErr == nil && len(events.Items) > 0 {
		fmt.Fprintf(&sb, "=== events ===\n")
		for _, ev := range events.Items {
			fmt.Fprintf(&sb, "[%s] %s: %s\n", ev.Type, ev.Reason, ev.Message)
		}
	}

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
// truncated to 500 chars.
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
