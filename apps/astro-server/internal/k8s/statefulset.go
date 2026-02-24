package k8s

import (
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatefulSetConfig holds configuration for building a StatefulSet
type StatefulSetConfig struct {
	Name            string
	Namespace       string
	AgentName       string
	BuildID         string
	Component       string
	Container       spec.ContainerConfig
	Port            int32
	SecretName      string
	ConfigMapName   string
	StorageSize     string
	StorageClass    string // Optional storage class name
	AccessMode      corev1.PersistentVolumeAccessMode // Defaults to ReadWriteOnce
	Healthcheck     *spec.Healthcheck
	Provider        string            // Provider type for health check generation (e.g., "redis", "postgres", "qdrant")
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
	// Deployment-spec driven fields (optional — zero values preserve existing behavior)
	Replicas         int32                                // 0 means use default (1)
	Resources        *corev1.ResourceRequirements         // nil means derive from Container.GPU
	Strategy         *appsv1.StatefulSetUpdateStrategy    // nil means k8s default
	NodeSelector     map[string]string                    // nil means no node selector
	Tolerations      []corev1.Toleration                  // Tolerations for tainted nodes
	PostStartCommand []string                             // Lifecycle postStart exec command
}

// BuildStatefulSet creates a Kubernetes StatefulSet manifest for persistent storage
func BuildStatefulSet(cfg StatefulSetConfig) *appsv1.StatefulSet {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	replicas := cfg.Replicas
	if replicas == 0 {
		replicas = 1
	}
	serviceName := cfg.Name

	prov := spec.GetProvider(cfg.Provider)

	port := cfg.Port
	if port == 0 {
		port = int32(prov.DefaultPort) //nolint:gosec
	}

	storageSize := cfg.StorageSize
	if storageSize == "" {
		storageSize = "10Gi"
	}

	// Build container
	ssPullPolicy := cfg.ImagePullPolicy
	if ssPullPolicy == "" {
		ssPullPolicy = corev1.PullAlways
	}

	// Build container ports from provider registry
	containerPorts := []corev1.ContainerPort{
		{Name: "app", ContainerPort: int32(prov.DefaultPort), Protocol: corev1.ProtocolTCP}, //nolint:gosec
	}
	for _, ep := range prov.ExtraPorts {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name: ep.Name, ContainerPort: int32(ep.Port), Protocol: corev1.ProtocolTCP, //nolint:gosec
		})
	}

	container := corev1.Container{
		Name:  "app",
		Image: cfg.Container.Image,
		Ports: containerPorts,
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "data",
				MountPath: prov.MountPath,
			},
		},
		ImagePullPolicy: ssPullPolicy,
	}

	// Add container-specific environment variables
	for key, value := range cfg.Container.Environment {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Add ConfigMap and Secret env vars
	if cfg.ConfigMapName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.ConfigMapName,
				},
			},
		})
	}

	if cfg.SecretName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: cfg.SecretName,
				},
			},
		})
	}

	// Set resources: explicit config takes precedence
	if cfg.Resources != nil {
		container.Resources = *cfg.Resources
	} else {
		container.Resources = buildResourceRequirements(cfg.Container.GPU)
	}

	// Add health checks if specified
	if cfg.Healthcheck != nil {
		probe := buildProbe(cfg.Healthcheck, cfg.Provider, port)
		if probe != nil {
			container.LivenessProbe = probe
			container.ReadinessProbe = probe
		}
	}

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

	// Create VolumeClaimTemplate
	accessMode := cfg.AccessMode
	if accessMode == "" {
		accessMode = corev1.ReadWriteOnce
	}

	pvcSpec := corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{accessMode},
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: resource.MustParse(storageSize),
			},
		},
	}
	if cfg.StorageClass != "" {
		pvcSpec.StorageClassName = &cfg.StorageClass
	}

	volumeClaimTemplate := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "data",
			Labels: labels,
		},
		Spec: pvcSpec,
	}

	statefulSet := &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apps/v1",
			Kind:       "StatefulSet",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: serviceName,
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers:   []corev1.Container{container},
					NodeSelector: cfg.NodeSelector,
					Tolerations:  cfg.Tolerations,
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{volumeClaimTemplate},
		},
	}

	// Apply update strategy if provided
	if cfg.Strategy != nil {
		statefulSet.Spec.UpdateStrategy = *cfg.Strategy
	}

	return statefulSet
}
