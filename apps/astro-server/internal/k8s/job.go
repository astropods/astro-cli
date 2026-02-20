package k8s

import (
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/packages/astro-spec"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// JobConfig holds configuration for building a one-shot Job or ingestion Deployment
type JobConfig struct {
	Name          string
	Namespace     string
	AgentName     string
	BuildID       string
	Component     string
	SecretName    string
	ConfigMapName string
	Ingestion     spec.Ingestion
}

// buildIngestionContainer creates the container spec shared by CronJob, Job, and ingestion Deployment
func buildIngestionContainer(ingestion spec.Ingestion, configMapName, secretName string) corev1.Container {
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
		ImagePullPolicy: corev1.PullAlways,
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

	return container
}

// BuildJob creates a one-shot Kubernetes Job manifest for ingestion (startup/manual triggers)
func BuildJob(cfg JobConfig) *batchv1.Job {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component)
	container := buildIngestionContainer(cfg.Ingestion, cfg.ConfigMapName, cfg.SecretName)

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
				Spec: corev1.PodSpec{
					Containers:    []corev1.Container{container},
					RestartPolicy: corev1.RestartPolicyOnFailure,
				},
			},
		},
	}

	return job
}

// BuildIngestionDeployment creates a long-running Deployment for webhook-triggered ingestion
func BuildIngestionDeployment(cfg JobConfig, port int32, imagePullPolicy corev1.PullPolicy) *appsv1.Deployment {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)
	container := buildIngestionContainer(cfg.Ingestion, cfg.ConfigMapName, cfg.SecretName)

	// Override container port for the webhook listener
	container.Ports = []corev1.ContainerPort{
		{
			Name:          "http",
			ContainerPort: port,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	if imagePullPolicy != "" {
		container.ImagePullPolicy = imagePullPolicy
	}

	replicas := int32(1)

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
				Spec: corev1.PodSpec{
					Containers:    []corev1.Container{container},
					RestartPolicy: corev1.RestartPolicyAlways,
				},
			},
		},
	}
}
