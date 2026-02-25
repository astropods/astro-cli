package k8s

import (
	"context"
	"fmt"

	"github.com/postman/astro/apps/astro-server/internal/deployment"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
)

// Deleter handles deletion of Kubernetes resources
type Deleter struct {
	client        ClusterClient
	namespace     string
	dynamicClient dynamic.Interface
}

// DeleterOption configures a Deleter.
type DeleterOption func(*Deleter)

// WithDeleterDynamicClient sets the dynamic client for deleting CRDs (KEDA resources).
func WithDeleterDynamicClient(dc dynamic.Interface) DeleterOption {
	return func(d *Deleter) { d.dynamicClient = dc }
}

// NewDeleter creates a new Deleter
func NewDeleter(client ClusterClient, namespace string, opts ...DeleterOption) *Deleter {
	d := &Deleter{client: client, namespace: namespace}
	for _, opt := range opts {
		opt(d)
	}
	return d
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
	d.deleteJobs(ctx, agentName, result)
	d.deleteCronJobs(ctx, agentName, result)

	// Deployments and StatefulSets
	d.deleteDeployments(ctx, agentName, result)
	d.deleteStatefulSets(ctx, agentName, result)

	// KEDA scaling resources
	d.deleteKEDAResources(ctx, agentName, result)

	// Services
	d.deleteServices(ctx, agentName, result)

	// ConfigMaps and Secrets
	d.deleteConfigMap(ctx, agentName, buildIDSanitized, result)
	d.deleteSecret(ctx, agentName, buildIDSanitized, result)

	// PersistentVolumeClaims (created by StatefulSets)
	d.deletePVCs(ctx, agentName, result)

	// Finally, delete the namespace itself
	d.deleteNamespace(ctx, result)

	return result, nil
}

// deleteKEDAResources removes HTTPScaledObjects and ScaledObjects from the agent
// namespace, and interceptor Ingresses from the keda namespace.
func (d *Deleter) deleteKEDAResources(ctx context.Context, agentName string, result *DeleteResult) {
	if d.dynamicClient == nil {
		return
	}
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)
	listOpts := metav1.ListOptions{LabelSelector: labelSelector}

	// HTTPScaledObjects in agent namespace
	httpSOs, err := d.dynamicClient.Resource(httpScaledObjectGVR).Namespace(d.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range httpSOs.Items {
			if delErr := d.dynamicClient.Resource(httpScaledObjectGVR).Namespace(d.namespace).Delete(ctx, item.GetName(), metav1.DeleteOptions{}); delErr != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: item.GetName(), Kind: "HTTPScaledObject", Error: delErr.Error(),
				})
			} else {
				result.Resources = append(result.Resources, deployment.ResourceStatus{
					Kind: "HTTPScaledObject", Name: item.GetName(), Namespace: d.namespace, Status: "deleted",
				})
			}
		}
	}

	// ScaledObjects in agent namespace
	scaledObjs, err := d.dynamicClient.Resource(scaledObjectGVR).Namespace(d.namespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range scaledObjs.Items {
			if delErr := d.dynamicClient.Resource(scaledObjectGVR).Namespace(d.namespace).Delete(ctx, item.GetName(), metav1.DeleteOptions{}); delErr != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: item.GetName(), Kind: "ScaledObject", Error: delErr.Error(),
				})
			} else {
				result.Resources = append(result.Resources, deployment.ResourceStatus{
					Kind: "ScaledObject", Name: item.GetName(), Namespace: d.namespace, Status: "deleted",
				})
			}
		}
	}

	// Interceptor ingresses in keda namespace
	kedaIngresses, err := d.client.Clientset().NetworkingV1().Ingresses(kedaNamespace).List(ctx, listOpts)
	if err == nil {
		for _, item := range kedaIngresses.Items {
			if delErr := d.client.Clientset().NetworkingV1().Ingresses(kedaNamespace).Delete(ctx, item.Name, metav1.DeleteOptions{}); delErr != nil {
				result.Errors = append(result.Errors, deployment.DeploymentError{
					Resource: item.Name, Kind: "Ingress", Error: delErr.Error(),
				})
			} else {
				result.Resources = append(result.Resources, deployment.ResourceStatus{
					Kind: "Ingress", Name: item.Name, Namespace: kedaNamespace, Status: "deleted",
				})
			}
		}
	}
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

// deleteJobs deletes all Jobs matching the agent
func (d *Deleter) deleteJobs(ctx context.Context, agentName string, result *DeleteResult) {
	labelSelector := fmt.Sprintf("astro.dev/agent=%s", agentName)

	jobs, err := d.client.Clientset().BatchV1().Jobs(d.namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
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
		err := d.client.Clientset().BatchV1().Jobs(d.namespace).Delete(ctx, job.Name, metav1.DeleteOptions{
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
func (d *Deleter) deleteConfigMap(ctx context.Context, agentName, buildIDSanitized string, result *DeleteResult) {
	configMapName := deployment.GenerateConfigMapName(agentName, buildIDSanitized)

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
func (d *Deleter) deleteSecret(ctx context.Context, agentName, buildIDSanitized string, result *DeleteResult) {
	secretName := deployment.GenerateSecretName(agentName, buildIDSanitized)

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
