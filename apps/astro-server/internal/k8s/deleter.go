package k8s

import (
	"context"
	"fmt"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Deleter handles deletion of Kubernetes resources
type Deleter struct {
	clientset kubernetes.Interface
	namespace string
}

// NewDeleter creates a new Deleter
func NewDeleter(clientset kubernetes.Interface, namespace string) *Deleter {
	return &Deleter{
		clientset: clientset,
		namespace: namespace,
	}
}

// DeleteResult holds the result of a delete operation
type DeleteResult struct {
	Resources []deployment.ResourceStatus
	Errors    []deployment.DeploymentError
}

// Delete deletes all resources for an agent deployment
func (d *Deleter) Delete(ctx context.Context, agentName, buildID string) (*DeleteResult, error) {
	result := &DeleteResult{
		Resources: []deployment.ResourceStatus{},
		Errors:    []deployment.DeploymentError{},
	}

	// Sanitize buildID for resource names (replace dots with hyphens)
	buildIDSanitized := deployment.SanitizeName(buildID)

	// Delete resources in reverse order (opposite of creation)
	// Jobs and CronJobs first
	d.deleteJobs(ctx, result)
	d.deleteCronJobs(ctx, result)

	// Ingresses
	d.deleteIngresses(ctx, result)

	// Deployments and StatefulSets
	d.deleteDeployments(ctx, result)
	d.deleteStatefulSets(ctx, result)

	// Services
	d.deleteServices(ctx, result)

	// ConfigMaps and Secrets
	d.deleteConfigMap(ctx, agentName, buildIDSanitized, result)
	d.deleteSecret(ctx, agentName, buildIDSanitized, result)

	// PersistentVolumeClaims (created by StatefulSets)
	d.deletePVCs(ctx, result)

	// Finally, delete the namespace itself
	d.deleteNamespace(ctx, result)

	return result, nil
}

// deleteCronJobs deletes all CronJobs in the namespace
func (d *Deleter) deleteCronJobs(ctx context.Context, result *DeleteResult) {
	cronJobs, err := d.clientset.BatchV1().CronJobs(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "CronJobs",
			Kind:     "CronJob",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, cronJob := range cronJobs.Items {
		err := d.clientset.BatchV1().CronJobs(d.namespace).Delete(ctx, cronJob.Name, metav1.DeleteOptions{})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: cronJob.Name,
				Kind:     "CronJob",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "CronJob",
				Name:      cronJob.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteJobs deletes all Jobs in the namespace
func (d *Deleter) deleteJobs(ctx context.Context, result *DeleteResult) {
	jobs, err := d.clientset.BatchV1().Jobs(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "Jobs",
			Kind:     "Job",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	propagation := metav1.DeletePropagationForeground
	for _, job := range jobs.Items {
		err := d.clientset.BatchV1().Jobs(d.namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
			PropagationPolicy: &propagation,
		})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: job.Name,
				Kind:     "Job",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "Job",
				Name:      job.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteIngresses deletes all Ingresses in the namespace
func (d *Deleter) deleteIngresses(ctx context.Context, result *DeleteResult) {
	ingresses, err := d.clientset.NetworkingV1().Ingresses(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "Ingresses",
			Kind:     "Ingress",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, ing := range ingresses.Items {
		err := d.clientset.NetworkingV1().Ingresses(d.namespace).Delete(ctx, ing.Name, metav1.DeleteOptions{})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: ing.Name,
				Kind:     "Ingress",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "Ingress",
				Name:      ing.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteDeployments deletes all Deployments in the namespace
func (d *Deleter) deleteDeployments(ctx context.Context, result *DeleteResult) {
	deployments, err := d.clientset.AppsV1().Deployments(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "Deployments",
			Kind:     "Deployment",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, dep := range deployments.Items {
		err := d.clientset.AppsV1().Deployments(d.namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: dep.Name,
				Kind:     "Deployment",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "Deployment",
				Name:      dep.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteStatefulSets deletes all StatefulSets in the namespace
func (d *Deleter) deleteStatefulSets(ctx context.Context, result *DeleteResult) {
	statefulSets, err := d.clientset.AppsV1().StatefulSets(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "StatefulSets",
			Kind:     "StatefulSet",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, sts := range statefulSets.Items {
		err := d.clientset.AppsV1().StatefulSets(d.namespace).Delete(ctx, sts.Name, metav1.DeleteOptions{})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: sts.Name,
				Kind:     "StatefulSet",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "StatefulSet",
				Name:      sts.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteServices deletes all Services in the namespace
func (d *Deleter) deleteServices(ctx context.Context, result *DeleteResult) {
	services, err := d.clientset.CoreV1().Services(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "Services",
			Kind:     "Service",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, svc := range services.Items {
		err := d.clientset.CoreV1().Services(d.namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: svc.Name,
				Kind:     "Service",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "Service",
				Name:      svc.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteConfigMap deletes the ConfigMap for the agent
func (d *Deleter) deleteConfigMap(ctx context.Context, agentName, buildIDSanitized string, result *DeleteResult) {
	configMapName := deployment.GenerateConfigMapName(agentName, buildIDSanitized)

	err := d.clientset.CoreV1().ConfigMaps(d.namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: configMapName,
			Kind:     "ConfigMap",
			Error:    err.Error(),
		})
	} else {
		result.Resources = append(result.Resources, deployment.ResourceStatus{
			Kind:      "ConfigMap",
			Name:      configMapName,
			Namespace: d.namespace,
			Status:    "deleted",
		})
	}
}

// deleteSecret deletes the Secret for the agent
func (d *Deleter) deleteSecret(ctx context.Context, agentName, buildIDSanitized string, result *DeleteResult) {
	secretName := deployment.GenerateSecretName(agentName, buildIDSanitized)

	err := d.clientset.CoreV1().Secrets(d.namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: secretName,
			Kind:     "Secret",
			Error:    err.Error(),
		})
	} else {
		result.Resources = append(result.Resources, deployment.ResourceStatus{
			Kind:      "Secret",
			Name:      secretName,
			Namespace: d.namespace,
			Status:    "deleted",
		})
	}
}

// deletePVCs deletes all PersistentVolumeClaims in the namespace (created by StatefulSets)
func (d *Deleter) deletePVCs(ctx context.Context, result *DeleteResult) {
	pvcs, err := d.clientset.CoreV1().PersistentVolumeClaims(d.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "PersistentVolumeClaims",
			Kind:     "PersistentVolumeClaim",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, pvc := range pvcs.Items {
		err := d.clientset.CoreV1().PersistentVolumeClaims(d.namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
		if err != nil {
			result.Errors = append(result.Errors, deployment.DeploymentError{
				Resource: pvc.Name,
				Kind:     "PersistentVolumeClaim",
				Error:    err.Error(),
			})
		} else {
			result.Resources = append(result.Resources, deployment.ResourceStatus{
				Kind:      "PersistentVolumeClaim",
				Name:      pvc.Name,
				Namespace: d.namespace,
				Status:    "deleted",
			})
		}
	}
}

// deleteNamespace deletes the Kubernetes namespace
func (d *Deleter) deleteNamespace(ctx context.Context, result *DeleteResult) {
	err := d.clientset.CoreV1().Namespaces().Delete(ctx, d.namespace, metav1.DeleteOptions{})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: d.namespace,
			Kind:     "Namespace",
			Error:    err.Error(),
		})
	} else {
		result.Resources = append(result.Resources, deployment.ResourceStatus{
			Kind:   "Namespace",
			Name:   d.namespace,
			Status: "deleted",
		})
	}
}
