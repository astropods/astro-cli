package k8s

import (
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StatefulSetConfig holds configuration for building a StatefulSet
type StatefulSetConfig struct {
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
	StorageSize     string
	StorageClass    string                            // Optional storage class name
	AccessMode      corev1.PersistentVolumeAccessMode // Defaults to ReadWriteOnce
	Healthcheck     *spec.Healthcheck
	Provider        string            // Provider name (e.g., "redis", "postgres", "qdrant", "ollama")
	ProviderSection string            // Provider section for registry lookup ("models", "knowledge")
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
	// Deployment-spec driven fields (optional — zero values preserve existing behavior)
	Replicas         int32                             // 0 means use default (1)
	Resources        *corev1.ResourceRequirements      // nil means derive from Container.GPU
	Strategy         *appsv1.StatefulSetUpdateStrategy // nil means k8s default
	NodeSelector     map[string]string                 // nil means no node selector
	Tolerations      []corev1.Toleration               // Tolerations for tainted nodes
	PostStartCommand []string                          // Lifecycle postStart exec command
	LocalMode        bool                              // Skip security hardening (local K8s only)
}

// BuildStatefulSet creates a Kubernetes StatefulSet manifest for persistent storage.
// Returns an error if the provider is missing required fields (port, mount path).
func BuildStatefulSet(cfg StatefulSetConfig) (*appsv1.StatefulSet, error) {
	prov, _ := spec.LookupBuiltin(cfg.ProviderSection, cfg.Provider)

	port := cfg.Port
	if port == 0 {
		port = int32(prov.DefaultPort) //nolint:gosec
	}
	if port == 0 {
		return nil, fmt.Errorf("StatefulSet %s: no port specified and provider %q has no default port", cfg.Name, cfg.Provider)
	}

	mountPath := prov.MountPath
	if mountPath == "" {
		return nil, fmt.Errorf("StatefulSet %s: provider %q has no mount path", cfg.Name, cfg.Provider)
	}

	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AccountID, cfg.AgentName, cfg.Component)

	replicas := cfg.Replicas
	if replicas == 0 {
		replicas = 1
	}
	serviceName := cfg.Name

	storageSize := cfg.StorageSize
	if storageSize == "" {
		storageSize = "10Gi"
	}

	ssPullPolicy := cfg.ImagePullPolicy
	if ssPullPolicy == "" {
		ssPullPolicy = corev1.PullAlways
	}

	containerPorts := []corev1.ContainerPort{
		{Name: "app", ContainerPort: port, Protocol: corev1.ProtocolTCP},
	}
	for _, ep := range prov.ExtraPorts {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name: ep.Name, ContainerPort: int32(ep.Port), Protocol: corev1.ProtocolTCP, //nolint:gosec
		})
	}

	volumeMounts := []corev1.VolumeMount{
		{
			Name:      "data",
			MountPath: mountPath,
		},
	}
	for i, dir := range prov.ExtraEmptyDirs {
		volumeMounts = append(volumeMounts, corev1.VolumeMount{
			Name:      fmt.Sprintf("extra-%d", i),
			MountPath: dir,
		})
	}

	container := corev1.Container{
		Name:            "app",
		Image:           cfg.Container.Image,
		Ports:           containerPorts,
		VolumeMounts:    volumeMounts,
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
		probe := buildProbe(cfg.Healthcheck, cfg.Provider, cfg.ProviderSection, port)
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

	if !cfg.LocalMode {
		hardenContainer(&container)

		// Some providers (e.g. qdrant) write to paths outside their data mount
		// and need a writable root filesystem.
		if prov.WritableRootFS && container.SecurityContext != nil {
			container.SecurityContext.ReadOnlyRootFilesystem = boolPtr(false)
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
				Spec: func() corev1.PodSpec {
					var extraVolumes []corev1.Volume
					for i := range prov.ExtraEmptyDirs {
						extraVolumes = append(extraVolumes, corev1.Volume{
							Name:         fmt.Sprintf("extra-%d", i),
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						})
					}
					tolerations := []corev1.Toleration{{
						Key: "astro.dev/tenant", Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule,
					}}
					if len(cfg.Tolerations) > 0 {
						tolerations = append(tolerations, cfg.Tolerations...)
					}
					ps := corev1.PodSpec{
						Containers:   []corev1.Container{container},
						NodeSelector: cfg.NodeSelector,
						Tolerations:  tolerations,
						Volumes:      extraVolumes,
					}
					if !cfg.LocalMode {
						hardenPodSpec(&ps)
					}
					return ps
				}(),
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{volumeClaimTemplate},
		},
	}

	// Apply update strategy if provided
	if cfg.Strategy != nil {
		statefulSet.Spec.UpdateStrategy = *cfg.Strategy
	}

	return statefulSet, nil
}
