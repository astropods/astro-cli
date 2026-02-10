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
	Version         string
	Component       string
	Container       spec.ContainerConfig
	Port            int32
	SecretName      string
	ConfigMapName   string
	StorageSize     string
	Healthcheck     *spec.Healthcheck
	Provider        string            // Provider type for health check generation (e.g., "redis", "postgres", "qdrant")
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
}

// BuildStatefulSet creates a Kubernetes StatefulSet manifest for persistent storage
func BuildStatefulSet(cfg StatefulSetConfig) *appsv1.StatefulSet {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.Version, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	replicas := int32(1)
	serviceName := cfg.Name

	port := cfg.Port
	if port == 0 {
		port = 6333
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

	container := corev1.Container{
		Name:  "app",
		Image: cfg.Container.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "rest",
				ContainerPort: 6333,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "grpc",
				ContainerPort: 6334,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{
				Name:      "data",
				MountPath: "/qdrant/storage",
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

	// Set resources
	container.Resources = buildResourceRequirements(cfg.Container.GPU)

	// Add health checks if specified
	if cfg.Healthcheck != nil {
		probe := buildProbe(cfg.Healthcheck, cfg.Provider, port)
		if probe != nil {
			container.LivenessProbe = probe
			container.ReadinessProbe = probe
		}
	}

	// Create VolumeClaimTemplate
	volumeClaimTemplate := corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "data",
			Labels: labels,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
		},
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
					Containers: []corev1.Container{container},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{volumeClaimTemplate},
		},
	}

	return statefulSet
}
