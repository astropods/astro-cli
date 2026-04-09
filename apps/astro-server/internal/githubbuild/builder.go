// Package githubbuild handles building container images from GitHub repos using
// BuildKit running as Kubernetes Jobs.
package githubbuild

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// Pinned init container images — update these explicitly to get new versions.
const (
	BuildKitImage = "moby/buildkit:v0.21.0-rootless"
	GitCloneImage = "alpine/git:2.47.2"
	ECRLoginImage = "amazon/aws-cli:2.24.21"
)

// BuildFailedError marks a build failure as permanent (bad Dockerfile or code),
// distinguishing it from retriable infrastructure errors.
type BuildFailedError struct{ Cause error }

func (e BuildFailedError) Error() string { return e.Cause.Error() }
func (e BuildFailedError) Unwrap() error { return e.Cause }

// Builder runs BuildKit K8s Jobs to build and push container images.
type Builder struct {
	k8sClient k8s.ClusterClient
	cfg       *config.Config
	log       *logger.Logger
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
// without pushing. Returns BuildFailedError for permanent failures (bad Dockerfile),
// or a plain error for retriable infrastructure failures.
func (b *Builder) RunJob(ctx context.Context, jobName, githubToken, repoFullName, commitSHA string, build spec.BuildConfig, destination string) error {
	if b.k8sClient == nil {
		return fmt.Errorf("k8s client not configured")
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
				{Name: "workspace", MountPath: "/workspace"},
				{Name: "token", MountPath: "/token", ReadOnly: true},
			},
		},
	}
	buildKitVolumeMounts := []corev1.VolumeMount{
		{Name: "workspace", MountPath: "/workspace", ReadOnly: true},
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
		ecrLoginCmd := fmt.Sprintf(
			`HOME=/tmp TOKEN=$(aws ecr get-login-password --region %s); AUTH=$(printf "AWS:%%s" "$TOKEN" | base64 | tr -d '\n'); printf '{"auths":{"%s":{"auth":"%%s"}}}' "$AUTH" > /docker-config/config.json`,
			b.cfg.Deployment.AWSRegion, registryHost,
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
				return ctx.Err()
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
			logs := b.fetchJobLogs(context.Background(), ns, jobName)
			return BuildFailedError{fmt.Errorf("build job failed: %s", extractBuildError(logs))}
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
	tailLines := int64(100)

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
