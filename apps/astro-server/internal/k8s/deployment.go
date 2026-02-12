package k8s

import (
	"fmt"
	"strconv"
	"time"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/packages/astro-spec"
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
	Version         string
	Component       string
	Container       spec.ContainerConfig
	Port            int32
	SecretName      string
	ConfigMapName   string
	Healthcheck     *spec.Healthcheck
	Provider        string            // Provider type for health check generation (e.g., "redis", "postgres", "qdrant")
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
}

// MessagingDeploymentConfig holds configuration for building a messaging sidecar Deployment
type MessagingDeploymentConfig struct {
	Name            string
	Namespace       string
	AgentName       string
	Version         string
	Component       string
	Image           string
	Port            int32
	SecretName      string
	AgentURL        string
	InterfaceType   string
	WebEnabled      bool              // Whether web adapter is enabled (exposes HTTP endpoint)
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
}

// BuildDeployment creates a Kubernetes Deployment manifest
func BuildDeployment(cfg DeploymentConfig) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.Version, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	replicas := int32(1)

	// Build container spec
	container := buildContainer(cfg)

	// Build pod spec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
	}

	// Add GPU node selector if needed
	if cfg.Container.GPU {
		podSpec.NodeSelector = map[string]string{
			"accelerator": "nvidia-gpu",
		}
	}

	deployment := &appsv1.Deployment{
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

	return deployment
}

// BuildMessagingDeployment creates a Kubernetes Deployment for messaging sidecars
func BuildMessagingDeployment(cfg MessagingDeploymentConfig) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.Version, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	replicas := int32(1)

	// Build container with messaging-specific env vars
	container := buildMessagingContainer(cfg)

	// Build pod spec
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
	}

	deploy := &appsv1.Deployment{
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

	return deploy
}

// buildMessagingContainer creates a container spec for messaging sidecars
func buildMessagingContainer(cfg MessagingDeploymentConfig) corev1.Container {
	port := cfg.Port
	if port == 0 {
		port = 9090
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
		{
			Name:  "GRPC_ENABLED",
			Value: "true",
		},
		{
			Name:  "GRPC_LISTEN_ADDR",
			Value: ":9090",
		},
		{
			Name:  "STORAGE_TYPE",
			Value: "memory",
		},
		{
			Name:  "DEPLOYMENT_MODE",
			Value: "all",
		},
	}

	// Enable adapters based on interface type
	if cfg.InterfaceType == "slack" {
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "SLACK_ENABLED",
				Value: "true",
			},
			corev1.EnvVar{
				Name:  "SLACK_SOCKET_MODE",
				Value: "true",
			},
		)
	}

	// Enable web adapter if configured
	if cfg.WebEnabled {
		container.Env = append(container.Env,
			corev1.EnvVar{
				Name:  "WEB_ENABLED",
				Value: "true",
			},
			corev1.EnvVar{
				Name:  "WEB_LISTEN_ADDR",
				Value: ":8080",
			},
		)
		// Add web port
		container.Ports = append(container.Ports, corev1.ContainerPort{
			Name:          "http",
			ContainerPort: 8080,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	// Add Secret as envFrom for credentials
	if cfg.SecretName != "" {
		container.EnvFrom = []corev1.EnvFromSource{
			{
				SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: cfg.SecretName,
					},
				},
			},
		}
	}

	// Set resource requests and limits
	resources := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}
	container.Resources = resources

	return container
}

// CollectorDeploymentConfig holds configuration for building a collector sidecar Deployment
type CollectorDeploymentConfig struct {
	Name            string
	Namespace       string
	AgentName       string
	Version         string
	Component       string
	Image           string
	ConfigMapName   string
	ImagePullPolicy corev1.PullPolicy
	// Galileo credentials (server-level config, injected directly)
	GalileoAPIKey  string
	GalileoProject string
}

// BuildCollectorDeployment creates a Kubernetes Deployment for the observability collector sidecar
func BuildCollectorDeployment(cfg CollectorDeploymentConfig) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.Version, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	replicas := int32(1)

	container := buildCollectorContainer(cfg)

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
	}

	deploy := &appsv1.Deployment{
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

	return deploy
}

// buildCollectorContainer creates a container spec for the collector sidecar
func buildCollectorContainer(cfg CollectorDeploymentConfig) corev1.Container {
	pullPolicy := cfg.ImagePullPolicy
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	container := corev1.Container{
		Name:  "collector",
		Image: cfg.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "otlp-grpc",
				ContainerPort: 4317,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "otlp-http",
				ContainerPort: 4318,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		ImagePullPolicy: pullPolicy,
	}

	// ConfigMap provides agent metadata (ASTRO_AGENT_NAME, etc.)
	if cfg.ConfigMapName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.ConfigMapName,
				},
			},
		})
	}

	// Galileo credentials are server-level config, injected directly as env vars
	if cfg.GalileoAPIKey != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "GALILEO_API_KEY",
			Value: cfg.GalileoAPIKey,
		})
	}
	if cfg.GalileoProject != "" {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "GALILEO_PROJECT",
			Value: cfg.GalileoProject,
		})
	}

	// Resource limits — collector is lightweight
	container.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("50m"),
			corev1.ResourceMemory: resource.MustParse("128Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("250m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
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
		probe := buildProbe(cfg.Healthcheck, cfg.Provider, port)
		if probe != nil {
			container.LivenessProbe = probe
			container.ReadinessProbe = probe
		}
	}

	// Set resource requests and limits
	resources := buildResourceRequirements(cfg.Container.GPU)
	container.Resources = resources

	return container
}

// buildProbe creates a probe from healthcheck config
func buildProbe(healthcheck *spec.Healthcheck, provider string, port int32) *corev1.Probe {
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
		handler := buildProbeHandler(provider, port, healthcheck.Path)
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
		probe.FailureThreshold = int32(healthcheck.Retries)
	}

	return probe
}

// buildProbeHandler generates a provider-specific probe handler
func buildProbeHandler(provider string, port int32, path string) *corev1.ProbeHandler {
	prov := spec.GetProvider(provider)

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
			port = int32(prov.DefaultPort)
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

// buildResourceRequirements creates resource requirements based on GPU needs
func buildResourceRequirements(gpu bool) corev1.ResourceRequirements {
	if gpu {
		// GPU resources
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
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
	}
}

// ParsePort parses a port from string or int
func ParsePort(portValue any) (int32, error) {
	switch v := portValue.(type) {
	case int:
		return int32(v), nil
	case int32:
		return v, nil
	case int64:
		return int32(v), nil
	case string:
		port, err := strconv.ParseInt(v, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("invalid port format: %v", err)
		}
		return int32(port), nil
	default:
		return 0, fmt.Errorf("unsupported port type: %T", portValue)
	}
}
