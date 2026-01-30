package k8s

import (
	"encoding/json"
	"fmt"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/packages/astro-spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobConfig holds configuration for building a CronJob
type CronJobConfig struct {
	Name           string
	Namespace      string
	AgentName      string
	Version        string
	Component      string
	Schedule       string
	Image          string // Injection worker image
	SecretName     string
	ConfigMapName  string
	Injection      spec.Injection
	CollectionName string // Target collection name
	VectorSize     int    // Vector dimensions
	RegistryURL    string // Registry URL for default images
}

// BuildCronJob creates a Kubernetes CronJob manifest for injections
func BuildCronJob(cfg CronJobConfig) *batchv1.CronJob {
	labels := deployment.GenerateLabels(cfg.AgentName, cfg.Version, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AgentName, cfg.Component)

	// Use a default injection worker image if not specified
	image := cfg.Image
	if image == "" {
		if cfg.RegistryURL == "" {
			// This should never happen as config validation should catch it
			panic("REGISTRY_URL is required but not set")
		}
		image = fmt.Sprintf("%s/astro-injection-worker:latest", cfg.RegistryURL)
	}

	// Serialize source config to JSON
	sourceConfigJSON, _ := json.Marshal(cfg.Injection.Source.Config)

	// Serialize pipeline to JSON
	pipelineJSON, _ := json.Marshal(cfg.Injection.Pipeline)

	// Build container for injection worker
	container := corev1.Container{
		Name:  "injection-worker",
		Image: image,
		Env: []corev1.EnvVar{
			{
				Name:  "INJECTION_SOURCE_TYPE",
				Value: cfg.Injection.Source.Type,
			},
			{
				Name:  "INJECTION_SOURCE_CONFIG",
				Value: string(sourceConfigJSON),
			},
			{
				Name:  "INJECTION_PIPELINE",
				Value: string(pipelineJSON),
			},
			{
				Name:  "INJECTION_COLLECTION_NAME",
				Value: cfg.CollectionName,
			},
			{
				Name:  "INJECTION_VECTOR_SIZE",
				Value: fmt.Sprintf("%d", cfg.VectorSize),
			},
			{
				Name:  "DRY_RUN",
				Value: "false",
			},
		},
		ImagePullPolicy: corev1.PullIfNotPresent,
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

	// Note: Persistent flag would be used when we implement actual storage

	// Set resource limits for injection workers
	container.Resources = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("256Mi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("500m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	}

	// Create pod template
	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: corev1.PodSpec{
			Containers:    []corev1.Container{container},
			RestartPolicy: corev1.RestartPolicyOnFailure,
		},
	}

	// Create job template
	successfulJobsHistoryLimit := int32(3)
	failedJobsHistoryLimit := int32(1)
	concurrencyPolicy := batchv1.ForbidConcurrent

	cronJob := &batchv1.CronJob{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch/v1",
			Kind:       "CronJob",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      cfg.Name,
			Namespace: cfg.Namespace,
			Labels:    labels,
		},
		Spec: batchv1.CronJobSpec{
			Schedule:                   cfg.Schedule,
			ConcurrencyPolicy:          concurrencyPolicy,
			SuccessfulJobsHistoryLimit: &successfulJobsHistoryLimit,
			FailedJobsHistoryLimit:     &failedJobsHistoryLimit,
			JobTemplate: batchv1.JobTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: selector,
				},
				Spec: batchv1.JobSpec{
					Template: podTemplate,
				},
			},
		},
	}

	return cronJob
}
