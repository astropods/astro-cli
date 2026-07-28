package k8s

import (
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro-spec"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CronJobConfig holds configuration for building a CronJob
type CronJobConfig struct {
	Name            string
	Namespace       string
	AccountID       string
	AgentName       string
	BuildID         string
	Component       string
	Schedule        string
	SecretName      string
	ConfigMapName   string
	Ingestion       spec.Ingestion
	ImagePullPolicy corev1.PullPolicy // Defaults to PullAlways if empty
	// ExtraEnv: see JobConfig.ExtraEnv. Same role for cron-triggered
	// ingestion containers.
	ExtraEnv []corev1.EnvVar
}

// BuildCronJob creates a Kubernetes CronJob manifest for ingestion jobs
func BuildCronJob(cfg CronJobConfig) *batchv1.CronJob {
	labels := deployment.GenerateLabels(cfg.AccountID, cfg.AgentName, cfg.BuildID, cfg.Component)
	selector := deployment.GenerateSelector(cfg.AccountID, cfg.AgentName, cfg.Component)
	container := buildIngestionContainer(cfg.Ingestion, cfg.ConfigMapName, cfg.SecretName, cfg.ImagePullPolicy, cfg.ExtraEnv)

	podSpec := corev1.PodSpec{
		Containers:    []corev1.Container{container},
		RestartPolicy: corev1.RestartPolicyOnFailure,
	}
	hardenPodSpec(&podSpec)

	podTemplate := corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: podSpec,
	}

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
