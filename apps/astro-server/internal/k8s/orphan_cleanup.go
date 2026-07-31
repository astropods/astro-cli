package k8s

import (
	"context"
	"fmt"
	"log"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// computeExpectedResourceNames derives the set of resource names that
// ApplyDeploymentSpec would create for the given deployment spec. The returned
// map is keyed by resource kind (Service, Deployment, StatefulSet, Ingress,
// CronJob, Job) and each value is a set of expected names.
func computeExpectedResourceNames(
	ds *deployment.AstroDeploymentSpec,
	ingressDomain string,
	ingestionIngressDomain string,
) map[string]map[string]bool {
	agentName := ds.Source.Name

	expected := map[string]map[string]bool{
		"Service":     {},
		"Deployment":  {},
		"StatefulSet": {},
		"Ingress":     {},
		"CronJob":     {},
		"Job":         {},
	}

	// Agent service + workload. Every agent runs as a StatefulSet + PVC (the
	// applier guarantees a volume via normalizeAgentStorageDefaults), so mirror
	// that here — and expecting only the StatefulSet means a legacy agent's
	// stale Deployment is correctly torn down as an orphan on reconcile.
	agentResourceName := deployment.GenerateAgentResourceName(agentName, "agent")
	expected["Service"][agentResourceName] = true
	expected["StatefulSet"][agentResourceName] = true

	// Agent ingress
	if ep := deployment.ExposedEndpoint(ds.Agent.Endpoints); ep != nil {
		host := ""
		if ep.Expose != nil {
			host = ep.Expose.Domain
		}
		if host == "" && ingressDomain != "" {
			host = "has-domain" // just need to know it would be created
		}
		if host != "" {
			ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-agent")
			expected["Ingress"][ingressName] = true
		}
	}

	// Models
	for name, model := range ds.Models {
		resourceName := deployment.GenerateResourceName(agentName, "model", name)
		expected["Service"][resourceName] = true
		if model.Persistent {
			expected["StatefulSet"][resourceName] = true
		} else {
			expected["Deployment"][resourceName] = true
		}
	}

	// Knowledge
	for name, knowledge := range ds.Knowledge {
		resourceName := deployment.GenerateResourceName(agentName, "knowledge", name)
		expected["Service"][resourceName] = true
		if knowledge.Persistent {
			expected["StatefulSet"][resourceName] = true
		} else {
			expected["Deployment"][resourceName] = true
		}
	}

	// Tools
	for name := range ds.Integrations {
		resourceName := deployment.GenerateResourceName(agentName, "integration", name)
		expected["Service"][resourceName] = true
		expected["Deployment"][resourceName] = true
	}

	// Interfaces (messaging)
	if ds.Interfaces != nil && len(ds.Interfaces.Adapters) > 0 {
		msgResourceName := deployment.GenerateAgentResourceName(agentName, "messaging")
		expected["Service"][msgResourceName] = true

		// Web adapter ingress
		webEnabled := false
		for _, adapter := range ds.Interfaces.Adapters {
			if adapter == "web" {
				webEnabled = true
			}
		}
		if webEnabled {
			host := ""
			if ep := deployment.EndpointByName(ds.Interfaces.Endpoints, "http"); ep != nil && ep.Expose != nil {
				host = ep.Expose.Domain
			}
			if host == "" && ingressDomain != "" {
				host = "has-domain"
			}
			if host != "" {
				ingressName := deployment.GenerateAgentResourceName(agentName, "ingress-messaging")
				expected["Ingress"][ingressName] = true
			}
		}
	}

	// Observability (collector) — standalone deployment
	if ds.Observability.Enabled {
		collectorResourceName := deployment.GenerateAgentResourceName(agentName, "collector")
		expected["Service"][collectorResourceName] = true
		expected["Deployment"][collectorResourceName] = true
	}

	// Ingestion
	for name, ingestion := range ds.Ingestion {
		resourceName := deployment.GenerateResourceName(agentName, "ingestion", name)
		switch ingestion.Trigger.Type {
		case "schedule":
			if ingestion.Trigger.Schedule != "" {
				expected["CronJob"][resourceName] = true
			}
		case "startup":
			expected["Job"][resourceName] = true
		case "webhook":
			expected["Service"][resourceName] = true
			expected["Deployment"][resourceName] = true
			if ingestionIngressDomain != "" {
				ingressName := deployment.GenerateResourceName(agentName, "ingress", name)
				expected["Ingress"][ingressName] = true
			}
		}
	}

	return expected
}

// cleanupOrphanedResources lists existing resources by agent label and deletes
// any whose name is not in the expected set. Errors are logged but not fatal.
func (a *Applier) cleanupOrphanedResources(
	ctx context.Context,
	accountName string,
	agentName string,
	expected map[string]map[string]bool,
) []error {
	labelSelector := fmt.Sprintf("app.kubernetes.io/managed-by=astro-server,%s=%s", deployment.LabelKeyAgent, deployment.AgentLabelValue(accountName, agentName))
	propagation := metav1.DeletePropagationBackground
	deleteOpts := metav1.DeleteOptions{PropagationPolicy: &propagation}
	listOpts := metav1.ListOptions{LabelSelector: labelSelector}

	var errs []error

	// Ingresses
	if expectedNames, ok := expected["Ingress"]; ok {
		ingresses, err := a.clientset.NetworkingV1().Ingresses(a.namespace).List(ctx, listOpts)
		if err == nil {
			for _, item := range ingresses.Items {
				if !expectedNames[item.Name] {
					log.Printf("[orphan-cleanup] deleting orphaned Ingress %s", item.Name)
					if err := a.clientset.NetworkingV1().Ingresses(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
						errs = append(errs, fmt.Errorf("delete ingress %s: %w", item.Name, err))
					}
				}
			}
		}
	}

	// Services
	if expectedNames, ok := expected["Service"]; ok {
		services, err := a.clientset.CoreV1().Services(a.namespace).List(ctx, listOpts)
		if err == nil {
			for _, item := range services.Items {
				if !expectedNames[item.Name] {
					log.Printf("[orphan-cleanup] deleting orphaned Service %s", item.Name)
					if err := a.clientset.CoreV1().Services(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
						errs = append(errs, fmt.Errorf("delete service %s: %w", item.Name, err))
					}
				}
			}
		}
	}

	// Deployments
	if expectedNames, ok := expected["Deployment"]; ok {
		deployments, err := a.clientset.AppsV1().Deployments(a.namespace).List(ctx, listOpts)
		if err == nil {
			for _, item := range deployments.Items {
				if !expectedNames[item.Name] {
					log.Printf("[orphan-cleanup] deleting orphaned Deployment %s", item.Name)
					if err := a.clientset.AppsV1().Deployments(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
						errs = append(errs, fmt.Errorf("delete deployment %s: %w", item.Name, err))
					}
				}
			}
		}
	}

	// StatefulSets
	if expectedNames, ok := expected["StatefulSet"]; ok {
		statefulSets, err := a.clientset.AppsV1().StatefulSets(a.namespace).List(ctx, listOpts)
		if err == nil {
			for _, item := range statefulSets.Items {
				if !expectedNames[item.Name] {
					log.Printf("[orphan-cleanup] deleting orphaned StatefulSet %s", item.Name)
					if err := a.clientset.AppsV1().StatefulSets(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
						errs = append(errs, fmt.Errorf("delete statefulset %s: %w", item.Name, err))
					}
				}
			}
		}
	}

	// CronJobs
	if expectedNames, ok := expected["CronJob"]; ok {
		cronJobs, err := a.clientset.BatchV1().CronJobs(a.namespace).List(ctx, listOpts)
		if err == nil {
			for _, item := range cronJobs.Items {
				if !expectedNames[item.Name] {
					log.Printf("[orphan-cleanup] deleting orphaned CronJob %s", item.Name)
					if err := a.clientset.BatchV1().CronJobs(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
						errs = append(errs, fmt.Errorf("delete cronjob %s: %w", item.Name, err))
					}
				}
			}
		}
	}

	// Jobs
	if expectedNames, ok := expected["Job"]; ok {
		jobs, err := a.clientset.BatchV1().Jobs(a.namespace).List(ctx, listOpts)
		if err == nil {
			for _, item := range jobs.Items {
				if !expectedNames[item.Name] {
					log.Printf("[orphan-cleanup] deleting orphaned Job %s", item.Name)
					if err := a.clientset.BatchV1().Jobs(a.namespace).Delete(ctx, item.Name, deleteOpts); err != nil {
						errs = append(errs, fmt.Errorf("delete job %s: %w", item.Name, err))
					}
				}
			}
		}
	}

	return errs
}
