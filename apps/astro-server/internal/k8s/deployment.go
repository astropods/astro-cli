package k8s

import (
	"fmt"
	"strconv"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
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
	AgentName       string
	BuildID         string
	Component       string
	Container       spec.ContainerConfig
	Port            int32
	SecretName      string
	ConfigMapName   string
	Healthcheck     *spec.Healthcheck
	Provider        string            // Provider name (e.g., "redis", "postgres", "qdrant", "ollama")
	ProviderSection string            // Provider section for registry lookup ("models", "knowledge")
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
	// Deployment-spec driven fields (optional — zero values preserve existing behavior)
	Replicas         int32                        // 0 means use default (1)
	Resources        *corev1.ResourceRequirements // nil means derive from Container.GPU
	Strategy         *appsv1.DeploymentStrategy   // nil means k8s default
	NodeSelector     map[string]string            // nil means no node selector (unless Container has GPU)
	Tolerations      []corev1.Toleration          // Tolerations for tainted nodes (e.g., GPU)
	ExtraEnv         []corev1.EnvVar              // Additional env vars to inject
	PostStartCommand []string                     // Lifecycle postStart exec command (e.g., model pull)
	// Sidecar containers colocated in the same pod
	Messaging *MessagingDeploymentConfig // nil means no messaging sidecar
	Collector *CollectorDeploymentConfig // nil means no collector sidecar
}

// MessagingDeploymentConfig holds configuration for building a messaging sidecar Deployment
type MessagingDeploymentConfig struct {
	Name            string
	Namespace       string
	AgentName       string
	BuildID         string
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
}

// BuildDeployment creates a Kubernetes Deployment manifest.
// Optional sidecar containers (messaging, collector) are colocated in the same
// pod when provided, so they share localhost networking with the main container.
func BuildDeployment(cfg DeploymentConfig) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	replicas := cfg.Replicas
	if replicas == 0 {
		replicas = 1
	}

	// Build container spec
	container := buildContainer(cfg)

	containers := []corev1.Container{container}

	// Colocate messaging sidecar in the same pod
	if cfg.Messaging != nil {
		containers = append(containers, buildMessagingContainer(*cfg.Messaging))
	}

	// Colocate collector sidecar in the same pod
	if cfg.Collector != nil {
		containers = append(containers, buildCollectorContainer(*cfg.Collector))
	}

	// Build pod spec
	podSpec := corev1.PodSpec{
		Containers: containers,
	}

	// Add node selector: explicit config takes precedence over GPU auto-detection
	if cfg.NodeSelector != nil {
		podSpec.NodeSelector = cfg.NodeSelector
	} else if cfg.Container.HasGPU() {
		podSpec.NodeSelector = map[string]string{"workload-type": "gpu"}
	}

	// Add tolerations
	if len(cfg.Tolerations) > 0 {
		podSpec.Tolerations = cfg.Tolerations
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
			Replicas: &replicas,
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
		},
		ImagePullPolicy: msgPullPolicy,
	}

	// Add messaging-specific environment variables
	container.Env = []corev1.EnvVar{
		{Name: "GRPC_ENABLED", Value: "true"},
		{Name: "GRPC_LISTEN_ADDR", Value: fmt.Sprintf(":%d", port)},
		{Name: "STORAGE_TYPE", Value: "memory"},
		{Name: "DEPLOYMENT_MODE", Value: "all"},
	}

	// Enable adapters based on configuration
	if cfg.SlackEnabled {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "SLACK_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "SLACK_SOCKET_MODE", Value: "true"},
		)
	}

	// Enable web adapter if configured
	if cfg.WebEnabled {
		container.Env = append(container.Env,
			corev1.EnvVar{Name: "WEB_ENABLED", Value: "true"},
			corev1.EnvVar{Name: "WEB_LISTEN_ADDR", Value: fmt.Sprintf(":%d", webPort)},
		)
		container.Ports = append(container.Ports, corev1.ContainerPort{
			Name: "http", ContainerPort: webPort, Protocol: corev1.ProtocolTCP,
		})
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

	return container
}

// CollectorDeploymentConfig holds configuration for building a collector sidecar Deployment
type CollectorDeploymentConfig struct {
	Name            string
	Namespace       string
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
	// Galileo credentials (server-level config, injected directly)
	GalileoAPIKey    string
	GalileoProject   string
	GalileoLogStream string
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

	// Galileo credentials are server-level config, injected directly as env vars
	if cfg.GalileoAPIKey != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "GALILEO_API_KEY", Value: cfg.GalileoAPIKey,
		})
	}
	if cfg.GalileoProject != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "GALILEO_PROJECT", Value: cfg.GalileoProject,
		})
	}

	// Astro identity env vars — required by the astro processor
	container.Env = append(container.Env,
		corev1.EnvVar{Name: "ASTRO_AGENT_NAME", Value: cfg.AgentName},
		corev1.EnvVar{Name: "ASTRO_AGENT_VERSION", Value: cfg.AgentVersion},
		corev1.EnvVar{Name: "ASTRO_DEPLOYMENT_ID", Value: cfg.DeploymentID},
	)
	if cfg.GalileoLogStream != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name: "GALILEO_LOG_STREAM", Value: cfg.GalileoLogStream,
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

	return container
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
