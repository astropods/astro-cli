package k8s

import (
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobConfig holds configuration for building a one-shot Job or ingestion Deployment
type JobConfig struct {
	Name            string
	Namespace       string
	AccountID       string
	AgentName       string
	BuildID         string
	Component       string
	SecretName      string
	ConfigMapName   string
	Ingestion       spec.Ingestion
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
}

// buildIngestionContainer creates the container spec shared by CronJob, Job, and ingestion Deployment
func buildIngestionContainer(ingestion spec.Ingestion, configMapName, secretName string, pullPolicy corev1.PullPolicy) corev1.Container {
	if pullPolicy == "" {
		pullPolicy = corev1.PullAlways
	}

	var envVars []corev1.EnvVar
	for key, val := range ingestion.Container.Environment {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: val,
		})
	}

	container := corev1.Container{
		Name:            "ingestion-worker",
		Image:           ingestion.Container.Image,
		Env:             envVars,
		ImagePullPolicy: pullPolicy,
	}

	if configMapName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: configMapName,
				},
			},
		})
	}

	if secretName != "" {
		container.EnvFrom = append(container.EnvFrom, corev1.EnvFromSource{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: secretName,
				},
			},
		})
	}

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

	hardenContainer(&container)
	return container
}

// BuildJob creates a one-shot Kubernetes Job manifest for ingestion (startup/manual triggers)
func BuildJob(cfg JobConfig) *batchv1.Job {
	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	container := buildIngestionContainer(cfg.Ingestion, cfg.ConfigMapName, cfg.SecretName, cfg.ImagePullPolicy)

	backoffLimit := int32(3)
	ttl := int32(86400) // 1 day

	job := &batchv1.Job{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "Job",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            &backoffLimit,
			TTLSecondsAfterFinished: &ttl,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: func() corev1.PodSpec {
					ps := corev1.PodSpec{
						Containers:    []corev1.Container{container},
						RestartPolicy: corev1.RestartPolicyOnFailure,
					}
					hardenPodSpec(&ps)
					return ps
				}(),
			},
		},
	}

	return job
}

// BuildIngestionDeployment creates a long-running Deployment for webhook-triggered ingestion
func BuildIngestionDeployment(cfg JobConfig, port int32) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AccountID, cfg.AgentName, cfg.Component)
	container := buildIngestionContainer(cfg.Ingestion, cfg.ConfigMapName, cfg.SecretName, cfg.ImagePullPolicy)

	container.Ports = []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	replicas := int32(1)

	podSpec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		RestartPolicy: corev1.RestartPolicyAlways,
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
}
