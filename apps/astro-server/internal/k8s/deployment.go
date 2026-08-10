package k8s

import (
	"fmt"
	"path"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// DeploymentConfig holds configuration for building a Deployment
type DeploymentConfig struct {
	Name            string
	Namespace       string
	AccountID       string
	AgentName       string
	BuildID         string
	Component       string
	Container       spec.ContainerConfig
	Port            int32
	SecretName      string
	ConfigMapName   string
	Healthcheck     *spec.Healthcheck
	Provider        string            // Provider name (e.g., "redis", "postgres", "qdrant")
	ProviderSection string            // Provider section for registry lookup ("models", "knowledge")
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
	// Deployment-spec driven fields (optional — zero values preserve existing behavior)
	Replicas         int32                        // 0 means use default (1)
	Resources        *corev1.ResourceRequirements // nil means derive from Container.GPU
	Strategy         *appsv1.DeploymentStrategy   // nil means k8s default
	NodeSelector     map[string]string            // nil means no node selector (unless Container has GPU)
	Tolerations      []corev1.Toleration          // Tolerations for tainted nodes (e.g., GPU)
	ExtraEnv         []corev1.EnvVar              // Additional env vars to inject
	ExtraSecretNames []string                     // Additional Secrets to mount as envFrom (e.g., knowledge credentials)
	PostStartCommand []string                     // Lifecycle postStart exec command (e.g., model pull)
	LocalMode        bool                         // Skip security hardening for provider containers (local K8s only)
	EnvHash          string                       // Content hash of ConfigMap+Secret data; triggers rolling restart on env-only changes
}

// MessagingDeploymentConfig holds configuration for building a messaging sidecar Deployment
type MessagingDeploymentConfig struct {
	Name            string
	Namespace       string
	AgentName       string
	BuildID         string
	DeploymentID    string // Surfaced as ASTRO_AGENT_ID
	Component       string
	Image           string
	Port            int32
	SecretName      string
	ConfigMapName   string // ConfigMap with resolved env from interfaces.environment
	AgentURL        string
	SlackEnabled    bool                         // Whether slack adapter is enabled
	WebEnabled      bool                         // Whether web adapter is enabled (exposes HTTP endpoint)
	WebPort         int32                        // HTTP port for web adapter (default 8080)
	ImagePullPolicy corev1.PullPolicy            // Defaults to PullAlways if empty
	Resources       *corev1.ResourceRequirements // From interfaces.resources; nil means hardcoded defaults
	Environment     map[string]string            // Resolved env from interfaces.environment
	DeployToken     string                       // Signed token injected as ASTRO_AUTHZ_TOKEN. The token's iss claim carries astro-server's base URL, so no separate URL env var is needed.
	AuthTestUserID  string                       // When set, surfaced as AUTH_TEST_USER_ID — messaging treats every web request as this user. Local mode only.

	// Shared volume mounted into the messaging sidecar. When VolumeName is set,
	// the sidecar mounts the named pod volume (the agent's "data" PVC) at
	// VolumeMountPath, isolated under VolumeSubPath. Empty VolumeName means no
	// mount (e.g. the Deployment fallback path, which has no "data" volume).
	VolumeName      string
	VolumeMountPath string
	VolumeSubPath   string

	// FilesMountPath / FilesSubPath mount a second subtree of the same shared
	// volume for the agent files API (upload/download). Set together with
	// VolumeName; the sidecar mounts VolumeName at FilesMountPath under
	// FilesSubPath and reads/writes uploaded files there. The agent container
	// (whole volume at /data, no subPath) sees the same bytes under
	// /data/<FilesSubPath>. Empty FilesSubPath disables the files feature.
	FilesMountPath string
	FilesSubPath   string
}

// messagingChatDBFile is the SQLite chat database filename. Its directory is the
// messaging sidecar's shared-volume mount (cfg.VolumeMountPath), so the DB lives
// on the agent's shared persistent disk and is durable across reschedules. The
// path is derived from the mount rather than hardcoded so the two can't drift —
// a path outside the mount would land on the read-only container root.
const messagingChatDBFile = "chat.db"

// messagingFilesMount is where the messaging sidecar mounts the files subtree of
// the shared volume. It is a distinct mount point from the agent's /data so the
// sidecar addresses uploaded files by a clean root (FILES_DIR); the same bytes
// appear to the agent under /data/<filesVolumeSubPath>.
const messagingFilesMount = "/files"

// BuildDeployment creates a Kubernetes Deployment manifest.
// Optional sidecar containers (messaging, collector) are colocated in the same
// pod when provided, so they share localhost networking with the main container.
// deploymentProgressDeadlineSeconds bounds how long Kubernetes waits for a
// rollout to make progress before flipping the Deployment's Progressing
// condition to ProgressDeadlineExceeded. Kept well below the 600s default so
// the deployment controller observes a terminal rollout failure (bad image,
// crash loop) within a few minutes instead of ten.
const deploymentProgressDeadlineSeconds int32 = 180

func BuildDeployment(cfg DeploymentConfig) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AccountID, cfg.AgentName, cfg.Component)

	replicas := cfg.Replicas
	if replicas == 0 {
		replicas = 1
	}
	progressDeadline := deploymentProgressDeadlineSeconds

	// Build container spec
	container := buildContainer(cfg)

	// Build pod spec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
	}

	// Add node selector: explicit config takes precedence over GPU auto-detection,
	// which takes precedence over the tenant-pool default. The default keeps
	// agent pods off the system pool (reserved for cluster fabric like CoreDNS,
	// the ALB controller, and Contour) when nothing else pins their placement.
	if cfg.NodeSelector != nil {
		podSpec.NodeSelector = cfg.NodeSelector
	} else if cfg.Container.HasGPU() {
		podSpec.NodeSelector = map[string]string{"workload-type": "gpu"}
	} else {
		podSpec.NodeSelector = map[string]string{"workload-type": "tenant"}
	}

	// Seed tolerations so the array is never nil after K8s protobuf roundtrip.
	podSpec.Tolerations = []corev1.Toleration{{
		Key: "astro.dev/tenant", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule,
	}}
	if len(cfg.Tolerations) > 0 {
		podSpec.Tolerations = append(podSpec.Tolerations, cfg.Tolerations...)
	}

	isProvider := cfg.Provider != ""
	if !cfg.LocalMode || !isProvider {
		hardenPodSpec(&podSpec)
	}

	// Pod template annotations — used to force rolling restarts on env-only changes.
	podAnnotations := map[string]string{}
	if cfg.EnvHash != "" {
		podAnnotations["astro.dev/env-hash"] = cfg.EnvHash
	}

	depl := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:                &replicas,
			ProgressDeadlineSeconds: &progressDeadline,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: podAnnotations,
				},
				Spec: podSpec,
			},
		},
	}

	// Apply update strategy if provided
	if cfg.Strategy != nil {
		depl.Spec.Strategy = *cfg.Strategy
	}

	return depl
}

// buildMessagingContainer creates a container spec for messaging sidecars
func buildMessagingContainer(cfg MessagingDeploymentConfig) corev1.Container {
	port := cfg.Port
	if port == 0 {
		port = 9090
	}

	webPort := cfg.WebPort
	if webPort == 0 {
		webPort = 8080
	}

	msgPullPolicy := cfg.ImagePullPolicy
	if msgPullPolicy == "" {
		msgPullPolicy = corev1.PullAlways
	}

	container := corev1.Container{
		Name:  "messaging",
		Image: cfg.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "grpc",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics",
				ContainerPort: 9091,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		ImagePullPolicy: msgPullPolicy,
	}

	// Mount the shared agent volume so messaging can persist data (e.g. sqlite
	// history) on the same PVC. Isolated under a subPath so it never collides
	// with the agent's own files. Only set when the agent runs with a volume.
	if cfg.VolumeName != "" {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      cfg.VolumeName,
			MountPath: cfg.VolumeMountPath,
			SubPath:   cfg.VolumeSubPath,
		})
	}

	// Mount a second subtree of the same volume for the agent files API. Kept on
	// its own mount + subPath (not under the messaging subtree) so uploaded files
	// are visible to the agent at /data/<FilesSubPath> and never mix with chat's
	// SQLite. Only set when a volume and a files subPath are configured.
	if cfg.VolumeName != "" && cfg.FilesSubPath != "" && cfg.FilesMountPath != "" {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      cfg.VolumeName,
			MountPath: cfg.FilesMountPath,
			SubPath:   cfg.FilesSubPath,
		})
	}

	// Add messaging-specific environment variables
	container.Env = []corev1.EnvVar{
		{Name: "GRPC_ENABLED", Value: "true"},
		{Name: "GRPC_LISTEN_ADDR", Value: fmt.Sprintf(":%d", port)},
		{Name: "STORAGE_TYPE", Value: "memory"},
		{Name: "DEPLOYMENT_MODE", Value: "all"},
	}

	// Persist the chat SQLite DB on the shared agent volume, only when one is
	// mounted. Derived from the mount path (set just above) so the DB can't drift
	// onto the read-only container root. No volume means no CHAT_DB_PATH, which
	// disables chat persistence rather than crashing on an unwritable path.
	if cfg.VolumeName != "" && cfg.VolumeMountPath != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "CHAT_DB_PATH",
			Value: path.Join(cfg.VolumeMountPath, messagingChatDBFile),
		})
	}

	// Point the files API at its mount. Gated the same way as the mount above so
	// the sidecar only enables uploads/downloads when durable storage is present;
	// unset FILES_DIR disables the feature rather than writing to a read-only root.
	if cfg.VolumeName != "" && cfg.FilesSubPath != "" && cfg.FilesMountPath != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "FILES_DIR",
			Value: cfg.FilesMountPath,
		})
	}
	if cfg.DeploymentID != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: "ASTRO_AGENT_ID", Value: cfg.DeploymentID})
	}

	// Enable adapters based on configuration.
	// Behavioral settings are injected via interface-targeted variables
	// through interfaces.environment, not hardcoded here.
	if cfg.SlackEnabled {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "SLACK_ENABLED", Value: "true"},
		)
	}

	// Enable web adapter if configured. The chat UI is served by the CLI /
	// astro-client, not the messaging sidecar, so no playground flag is set.
	if cfg.WebEnabled {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "WEB_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "WEB_LISTEN_ADDR", Value: fmt.Sprintf(":%d", webPort)},
		)
		container.Ports = append(container.Ports, corev1.ContainerPort{
			Name: "msg-http", ContainerPort: webPort, Protocol: corev1.ProtocolTCP,
		})
	}

	// Inject the signed deploy token. Its iss claim carries astro-server's
	// base URL — the messaging container reads it to know where to call
	// back, so no separate URL env var is needed.
	if cfg.DeployToken != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: "ASTRO_AUTHZ_TOKEN", Value: cfg.DeployToken})
	}

	// In local mode astro-server pins a fixed identity (the account owner)
	// so messaging behaves as if the user is signed in via OIDC — no real
	// ingress is in front to inject x-amzn-oidc-identity per request.
	if cfg.AuthTestUserID != "" {
		container.Env = append(container.Env, corev1.EnvVar{Name: "WEB_AUTHN_TEST_USER_ID", Value: cfg.AuthTestUserID})
	}

	// Add resolved environment from interfaces.environment (credential refs, etc.)
	for key, value := range cfg.Environment {
		container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: value})
	}

	// Add ConfigMap envFrom for resolved env vars
	if cfg.ConfigMapName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cfg.ConfigMapName},
			},
		})
	}

	// Add Secret as envFrom for credentials
	if cfg.SecretName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cfg.SecretName},
			},
		})
	}

	// Set resource requests and limits from deployment spec, or fall back to defaults
	if cfg.Resources != nil {
		container.Resources = *cfg.Resources
	} else {
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("50m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("250m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		}
	}

	hardenContainer(&container)
	return container
}

// CollectorDeploymentConfig holds configuration for building a collector Deployment
type CollectorDeploymentConfig struct {
	Name            string
	Namespace       string
	AccountID       string
	AgentName       string
	AgentVersion    string
	BuildID         string
	DeploymentID    string
	Component       string
	Image           string
	Port            int32 // OTLP HTTP port (default 4318)
	ConfigMapName   string
	SecretName      string // Secret with credentials
	ImagePullPolicy corev1.PullPolicy
	Resources       *corev1.ResourceRequirements // From observability.resources; nil means hardcoded defaults
	Environment     map[string]string            // Resolved env from observability.environment
	// Langfuse credentials (per-account)
	LangfuseAuthToken string // base64(pk:sk)
	LangfuseBaseURL   string // e.g. https://langfuse.adhoc.dev.astropod.ai
}

// buildCollectorContainer creates a container spec for the collector sidecar
func buildCollectorContainer(cfg CollectorDeploymentConfig) corev1.Container {
	pullPolicy := cfg.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	otlpHTTPPort := cfg.Port
	if otlpHTTPPort == 0 {
		otlpHTTPPort = 4318
	}
	// gRPC port is conventionally one below the HTTP port
	otlpGRPCPort := otlpHTTPPort - 1

	container := corev1.Container{
		Name:  "collector",
		Image: cfg.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "otlp-grpc",
				ContainerPort: otlpGRPCPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "otlp-http",
				ContainerPort: otlpHTTPPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		ImagePullPolicy: pullPolicy,
	}

	// Astro identity env vars — required by the astro processor
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "ASTRO_AGENT_NAME", Value: cfg.AgentName},
		corev1.EnvVar{Name: "ASTRO_AGENT_VERSION", Value: cfg.AgentVersion},
		corev1.EnvVar{Name: "ASTRO_AGENT_ID", Value: cfg.DeploymentID},
		corev1.EnvVar{Name: "ASTRO_DEPLOYMENT_ID", Value: cfg.DeploymentID},
	)
	if cfg.LangfuseAuthToken != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "LANGFUSE_AUTH_TOKEN", Value: cfg.LangfuseAuthToken,
		})
	}
	if cfg.LangfuseBaseURL != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "LANGFUSE_BASE_URL", Value: cfg.LangfuseBaseURL,
		})
	}

	// Add resolved environment from observability.environment
	for key, value := range cfg.Environment {
		container.Env = append(container.Env, corev1.EnvVar{Name: key, Value: value})
	}

	// ConfigMap provides agent metadata (ASTRO_AGENT_NAME, etc.)
	if cfg.ConfigMapName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cfg.ConfigMapName},
			},
		})
	}

	// Secret for credentials
	if cfg.SecretName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: cfg.SecretName},
			},
		})
	}

	// Set resource requests and limits from deployment spec, or fall back to defaults
	if cfg.Resources != nil {
		container.Resources = *cfg.Resources
	} else {
		container.Resources = corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("25m"),
				corev1.ResourceMemory: resource.MustParse("64Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		}
	}

	hardenContainer(&container)
	return container
}

// BuildCollectorDeployment creates a standalone Kubernetes Deployment for the
// collector. Unlike the previous sidecar approach, this runs the collector in
// its own pod so it can be targeted by NetworkPolicy independently.
func BuildCollectorDeployment(cfg CollectorDeploymentConfig) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AccountID, cfg.AgentName, cfg.Component)

	var replicas int32 = 1
	progressDeadline := deploymentProgressDeadlineSeconds
	container := buildCollectorContainer(cfg)
	podSpec := corev1.PodSpec{
		Containers:   []corev1.Container{container},
		NodeSelector: map[string]string{"workload-type": "tenant"},
	}
	hardenPodSpec(&podSpec)

	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas:                &replicas,
			ProgressDeadlineSeconds: &progressDeadline,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: podSpec,
			},
		},
	}
}

// buildContainer creates a container spec
func buildContainer(cfg DeploymentConfig) corev1.Container {
	port := cfg.Port
	if port == 0 {
		port = 8080
	}

	pullPolicy := cfg.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	container := corev1.Container{
		Name:  "app",
		Image: cfg.Container.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "http",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		ImagePullPolicy: pullPolicy,
	}

	// Add container-specific environment variables
	for key, value := range cfg.Container.Environment {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Add ConfigMap as envFrom for all keys
	if cfg.ConfigMapName != "" {
		container.EnvFrom = []corev1.EnvFromSource{
			{
				ConfigMapRef: &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.ConfigMapName,
					},
				},
			},
		}
	}

	// Add Secret as envFrom for all keys
	if cfg.SecretName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.SecretName,
				},
			},
		})
	}

	// Mount additional secrets (e.g., knowledge store credentials)
	for _, extraSecret := range cfg.ExtraSecretNames {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: extraSecret},
			},
		})
	}

	// Add health checks if specified
	if cfg.Healthcheck != nil {
		probe := buildProbe(cfg.Healthcheck, cfg.Provider, cfg.ProviderSection, port)
		if probe != nil {
			container.LivenessProbe = probe
			container.ReadinessProbe = probe
		}
	}

	// Set resource requests and limits: explicit config takes precedence
	if cfg.Resources != nil {
		container.Resources = *cfg.Resources
	} else {
		container.Resources = buildResourceRequirements(cfg.Container.GPU)
	}

	// Add extra env vars (from deployment spec resolution)
	container.Env = append(container.Env, cfg.ExtraEnv...)

	// Add lifecycle postStart hook (e.g., model pull command)
	if len(cfg.PostStartCommand) > 0 {
		container.Lifecycle = &corev1.Lifecycle{
			PostStart: &corev1.LifecycleHandler{
				Exec: &corev1.ExecAction{
					Command: cfg.PostStartCommand,
				},
			},
		}
	}

	isProvider := cfg.Provider != ""
	if !cfg.LocalMode || !isProvider {
		hardenContainer(&container)
	}
	return container
}

// buildProbe creates a probe from healthcheck config
func buildProbe(healthcheck *spec.Healthcheck, provider, providerSection string, port int32) *corev1.Probe {
	probe := &corev1.Probe{
		InitialDelaySeconds: 10,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		SuccessThreshold:    1,
		FailureThreshold:    3,
	}

	// If custom test command is provided, use it
	if len(healthcheck.Test) > 0 {
		probe.ProbeHandler = corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: healthcheck.Test,
			},
		}
	} else {
		// Generate provider-specific health check
		handler := buildProbeHandler(provider, providerSection, port, healthcheck.Path)
		if handler == nil {
			// No suitable health check could be generated
			return nil
		}
		probe.ProbeHandler = *handler
	}

	// Parse interval and timeout if provided
	if healthcheck.Interval != "" {
		if duration, err := time.ParseDuration(healthcheck.Interval); err == nil {
			probe.PeriodSeconds = int32(duration.Seconds())
		}
	}

	if healthcheck.Timeout != "" {
		if duration, err := time.ParseDuration(healthcheck.Timeout); err == nil {
			probe.TimeoutSeconds = int32(duration.Seconds())
		}
	}

	if healthcheck.Retries > 0 {
		probe.FailureThreshold = int32(healthcheck.Retries) //nolint:gosec
	}

	return probe
}

// buildProbeHandler generates a provider-specific probe handler
func buildProbeHandler(provider, providerSection string, port int32, path string) *corev1.ProbeHandler {
	prov, _ := spec.LookupBuiltin(providerSection, provider)

	// Exec-based health check from provider registry
	if len(prov.HealthCheck) > 0 {
		return &corev1.ProbeHandler{
			Exec: &corev1.ExecAction{
				Command: prov.HealthCheck,
			},
		}
	}

	// HTTP health check from provider registry
	if prov.HealthPath != "" {
		if port == 0 {
			port = int32(prov.DefaultPort) //nolint:gosec
		}
		return &corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: prov.HealthPath,
				Port: intstr.FromInt(int(port)),
			},
		}
	}

	// Fallback: if a path is provided, use HTTP health check
	if path != "" {
		if port == 0 {
			port = 8080
		}
		return &corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: path,
				Port: intstr.FromInt(int(port)),
			},
		}
	}

	// No suitable health check
	return nil
}

// buildResourceRequirements creates resource requirements based on GPU hints.
// When gpu is non-nil the spec is treated as a scheduling hint: Count (default
// 1) sets the nvidia.com/gpu resource request and fixed CPU/memory defaults are
// applied. The server uses these hints on a best-effort basis.
func buildResourceRequirements(gpu *spec.GPUConfig) corev1.ResourceRequirements {
	if gpu != nil {
		return corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
				"nvidia.com/gpu":      resource.MustParse("1"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
				"nvidia.com/gpu":      resource.MustParse("1"),
			},
		}
	}

	// Standard resources
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
}

// ParsePort parses a port from string or int
func ParsePort(portValue any) (int32, error) {
	switch v := portValue.(type) {
	case int:
		return int32(v), nil //nolint:gosec
	case int32:
		return v, nil
	case int64:
		return int32(v), nil //nolint:gosec
	case string:
		port, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid port format: %w", err)
		}
		return int32(port), nil //nolint:gosec
	default:
		return 0, fmt.Errorf("unsupported port type: %T", portValue)
	}
}
