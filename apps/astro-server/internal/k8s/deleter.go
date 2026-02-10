package k8s

import (
	"context"
	"fmt"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Deleter handles deletion of Kubernetes resources
type Deleter struct {
	client    ClusterClient
	namespace string
}

// NewDeleter creates a new Deleter
func NewDeleter(client ClusterClient, namespace string) *Deleter {
	return &Deleter{
		client:    client,
		namespace: namespace,
	}
}

// DeleteResult holds the result of a delete operation
type DeleteResult struct {
	Resources []deployment.ResourceStatus
	Errors    []deployment.DeploymentError
}

// Delete deletes all resources for an agent deployment
func (d *Deleter) Delete(ctx context.Context, agentName, version string) (*DeleteResult, error) {
	result := &DeleteResult{
		Resources: []deployment.ResourceStatus{},
		Errors:    []deployment.DeploymentError{},
	}

	// Sanitize version for resource names (replace dots with hyphens)
	versionSanitized := deployment.SanitizeName(version)

	// Delete resources in reverse order (opposite of creation)
	// CronJobs first
	d.deleteCronJobs(ctx, agentName, result)

	// Deployments and StatefulSets
	d.deleteDeployments(ctx, agentName, result)
	d.deleteStatefulSets(ctx, agentName, result)

	// Services
	d.deleteServices(ctx, agentName, result)

	// ConfigMaps and Secrets
	d.deleteConfigMap(ctx, agentName, versionSanitized, result)
	d.deleteSecret(ctx, agentName, versionSanitized, result)

	// PersistentVolumeClaims (created by StatefulSets)
	d.deletePVCs(ctx, agentName, result)

	// Finally, delete the namespace itself
	d.deleteNamespace(ctx, result)

	return result, nil
}

// deleteCronJobs deletes all CronJobs matching the agent
func (d *Deleter) deleteCronJobs(ctx context.Context, agentName string, result *DeleteResult) {
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)

	cronJobs, err := d.client.Clientset().BatchV1().CronJobs(d.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "CronJobs",
			Kind:     "CronJob",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, cronJob := range cronJobs.Items {
		err := d.client.Clientset().BatchV1().CronJobs(d.namespace).Delete(ctx, cronJob.Name, metav1.DeleteOptions{})
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

// deleteDeployments deletes all Deployments matching the agent
func (d *Deleter) deleteDeployments(ctx context.Context, agentName string, result *DeleteResult) {
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)

	deployments, err := d.client.Clientset().AppsV1().Deployments(d.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "Deployments",
			Kind:     "Deployment",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, dep := range deployments.Items {
		err := d.client.Clientset().AppsV1().Deployments(d.namespace).Delete(ctx, dep.Name, metav1.DeleteOptions{})
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

// deleteStatefulSets deletes all StatefulSets matching the agent
func (d *Deleter) deleteStatefulSets(ctx context.Context, agentName string, result *DeleteResult) {
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)

	statefulSets, err := d.client.Clientset().AppsV1().StatefulSets(d.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "StatefulSets",
			Kind:     "StatefulSet",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, sts := range statefulSets.Items {
		err := d.client.Clientset().AppsV1().StatefulSets(d.namespace).Delete(ctx, sts.Name, metav1.DeleteOptions{})
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

// deleteServices deletes all Services matching the agent
func (d *Deleter) deleteServices(ctx context.Context, agentName string, result *DeleteResult) {
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)

	services, err := d.client.Clientset().CoreV1().Services(d.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "Services",
			Kind:     "Service",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, svc := range services.Items {
		err := d.client.Clientset().CoreV1().Services(d.namespace).Delete(ctx, svc.Name, metav1.DeleteOptions{})
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
func (d *Deleter) deleteConfigMap(ctx context.Context, agentName, versionSanitized string, result *DeleteResult) {
	configMapName := deployment.GenerateConfigMapName(agentName, versionSanitized)

	err := d.client.Clientset().CoreV1().ConfigMaps(d.namespace).Delete(ctx, configMapName, metav1.DeleteOptions{})
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
func (d *Deleter) deleteSecret(ctx context.Context, agentName, versionSanitized string, result *DeleteResult) {
	secretName := deployment.GenerateCredentialSecretName(agentName, versionSanitized)

	err := d.client.Clientset().CoreV1().Secrets(d.namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
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

// deletePVCs deletes all PersistentVolumeClaims matching the agent (created by StatefulSets)
func (d *Deleter) deletePVCs(ctx context.Context, agentName string, result *DeleteResult) {
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)

	pvcs, err := d.client.Clientset().CoreV1().PersistentVolumeClaims(d.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		result.Errors = append(result.Errors, deployment.DeploymentError{
			Resource: "PersistentVolumeClaims",
			Kind:     "PersistentVolumeClaim",
			Error:    fmt.Sprintf("failed to list: %v", err),
		})
		return
	}

	for _, pvc := range pvcs.Items {
		err := d.client.Clientset().CoreV1().PersistentVolumeClaims(d.namespace).Delete(ctx, pvc.Name, metav1.DeleteOptions{})
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
	err := d.client.Clientset().CoreV1().Namespaces().Delete(ctx, d.namespace, metav1.DeleteOptions{})
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
