package k8s

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const ManagedByLabel = "app.kubernetes.io/managed-by=astro-server"

// StopNamespaceWorkloads scales all Deployments and StatefulSets to zero
// and suspends all CronJobs in the given namespace that carry the astro-server
// managed-by label. It does not delete any resources.
func StopNamespaceWorkloads(ctx context.Context, clientset *kubernetes.Clientset, ns string) error {
	opts := metav1.ListOptions{LabelSelector: ManagedByLabel}
	var zero int32 = 0

	// Scale Deployments to 0
	deps, err := clientset.AppsV1().Deployments(ns).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list deployments: %w", err)
	}
	for i := range deps.Items {
		item := deps.Items[i].DeepCopy()
		if item.Spec.Replicas != nil && *item.Spec.Replicas == 0 {
			continue
		}
		item.Spec.Replicas = &zero
		if _, err := clientset.AppsV1().Deployments(ns).Update(ctx, item, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("scale deployment %s to 0: %w", item.Name, err)
		}
	}

	// Scale StatefulSets to 0
	statefulSets, err := clientset.AppsV1().StatefulSets(ns).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list statefulsets: %w", err)
	}
	for i := range statefulSets.Items {
		item := statefulSets.Items[i].DeepCopy()
		if item.Spec.Replicas != nil && *item.Spec.Replicas == 0 {
			continue
		}
		item.Spec.Replicas = &zero
		if _, err := clientset.AppsV1().StatefulSets(ns).Update(ctx, item, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("scale statefulset %s to 0: %w", item.Name, err)
		}
	}

	// Suspend CronJobs
	cronJobs, err := clientset.BatchV1().CronJobs(ns).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("list cronjobs: %w", err)
	}
	suspend := true
	for i := range cronJobs.Items {
		item := cronJobs.Items[i].DeepCopy()
		if item.Spec.Suspend != nil && *item.Spec.Suspend {
			continue
		}
		item.Spec.Suspend = &suspend
		if _, err := clientset.BatchV1().CronJobs(ns).Update(ctx, item, metav1.UpdateOptions{}); err != nil {
			return fmt.Errorf("suspend cronjob %s: %w", item.Name, err)
		}
	}

	return nil
}
