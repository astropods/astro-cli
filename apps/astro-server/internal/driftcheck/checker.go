// Package driftcheck compares desired deployment state (from normalized DB tables)
// against actual K8s cluster state and reports drift. Report-only — no remediation.
package driftcheck

import (
	"context"
	"fmt"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DriftType classifies what kind of drift was detected.
type DriftType string

const (
	DriftMissing  DriftType = "missing"  // Resource exists in DB but not in K8s
	DriftReplicas DriftType = "replicas" // Replica count mismatch
	DriftImage    DriftType = "image"    // Container image mismatch
	DriftSchedule DriftType = "schedule" // CronJob schedule mismatch
)

// Drift represents a single detected difference between desired and actual state.
type Drift struct {
	DeploymentID string    `json:"deployment_id"`
	Namespace    string    `json:"namespace"`
	AgentName    string    `json:"agent_name"`
	Resource     string    `json:"resource"`
	ResourceKind string    `json:"resource_kind"`
	Type         DriftType `json:"type"`
	Detail       string    `json:"detail"`
}

// Report holds the result of a single drift check pass.
type Report struct {
	Timestamp          time.Time `json:"timestamp"`
	DeploymentsChecked int       `json:"deployments_checked"`
	Drifts             []Drift   `json:"drifts"`
}

// Checker performs periodic drift detection.
type Checker struct {
	deployStore *deploymentstore.Store
	k8sClient   k8s.ClusterClient
	log         *logger.Logger
}

// New creates a new drift checker.
func New(deployStore *deploymentstore.Store, k8sClient k8s.ClusterClient, log *logger.Logger) *Checker {
	return &Checker{
		deployStore: deployStore,
		k8sClient:   k8sClient,
		log:         log,
	}
}

// Start runs the drift check loop. Non-blocking.
func (c *Checker) Start(ctx context.Context, interval time.Duration) {
	go func() {
		report := c.Check(ctx)
		c.LogReport(report)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				c.log.Info("Drift checker stopping")
				return
			case <-ticker.C:
				report := c.Check(ctx)
				c.LogReport(report)
			}
		}
	}()
}

// Check performs a single drift detection pass across all active deployments.
func (c *Checker) Check(ctx context.Context) *Report {
	report := &Report{Timestamp: time.Now().UTC()}

	allDeps, err := c.deployStore.ListAllActive()
	if err != nil {
		c.log.Error("Drift check: failed to list deployments", "error", err)
		return report
	}

	for _, dwa := range allDeps {
		dep := &dwa.Deployment

		workloads, err := c.deployStore.GetWorkloads(dep.ID)
		if err != nil {
			c.log.Error("Drift check: failed to get workloads", "deployment_id", dep.ID, "error", err)
			continue
		}
		if len(workloads) == 0 {
			continue // Pre-normalization deployment, skip
		}

		report.DeploymentsChecked++
		drifts := c.checkDeployment(ctx, dep, workloads)
		report.Drifts = append(report.Drifts, drifts...)
	}

	return report
}

func (c *Checker) checkDeployment(ctx context.Context, dep *deploymentstore.Deployment, workloads []*deploymentstore.Workload) []Drift {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	ns := dep.Namespace
	var drifts []Drift

	for _, w := range workloads {
		switch w.WorkloadType {
		case "deployment":
			drifts = append(drifts, c.checkK8sDeployment(ctx, dep, w, ns)...)
		case "statefulset":
			drifts = append(drifts, c.checkStatefulSet(ctx, dep, w, ns)...)
		case "cronjob":
			drifts = append(drifts, c.checkCronJob(ctx, dep, w, ns)...)
		}
	}

	// Check services
	services, err := c.deployStore.GetServices(dep.ID)
	if err == nil {
		drifts = append(drifts, c.checkServices(ctx, dep, services, ns)...)
	}

	// Check ingresses
	ingresses, err := c.deployStore.GetIngresses(dep.ID)
	if err == nil {
		drifts = append(drifts, c.checkIngresses(ctx, dep, ingresses, ns)...)
	}

	return drifts
}

func (c *Checker) checkK8sDeployment(ctx context.Context, dep *deploymentstore.Deployment, w *deploymentstore.Workload, ns string) []Drift {
	var drifts []Drift
	clientset := c.k8sClient.Clientset()

	actual, err := clientset.AppsV1().Deployments(ns).Get(ctx, w.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			drifts = append(drifts, Drift{
				DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
				Resource: w.Name, ResourceKind: "Deployment", Type: DriftMissing,
				Detail: fmt.Sprintf("Deployment %q expected but not found in cluster", w.Name),
			})
		}
		return drifts
	}

	actualReplicas := int32(1)
	if actual.Spec.Replicas != nil {
		actualReplicas = *actual.Spec.Replicas
	}
	if int(actualReplicas) != w.Replicas {
		drifts = append(drifts, Drift{
			DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
			Resource: w.Name, ResourceKind: "Deployment", Type: DriftReplicas,
			Detail: fmt.Sprintf("replicas: desired=%d actual=%d", w.Replicas, actualReplicas),
		})
	}

	if len(actual.Spec.Template.Spec.Containers) > 0 {
		actualImage := actual.Spec.Template.Spec.Containers[0].Image
		if actualImage != w.Image {
			drifts = append(drifts, Drift{
				DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
				Resource: w.Name, ResourceKind: "Deployment", Type: DriftImage,
				Detail: fmt.Sprintf("image: desired=%q actual=%q", w.Image, actualImage),
			})
		}
	}

	return drifts
}

func (c *Checker) checkStatefulSet(ctx context.Context, dep *deploymentstore.Deployment, w *deploymentstore.Workload, ns string) []Drift {
	var drifts []Drift
	clientset := c.k8sClient.Clientset()

	actual, err := clientset.AppsV1().StatefulSets(ns).Get(ctx, w.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			drifts = append(drifts, Drift{
				DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
				Resource: w.Name, ResourceKind: "StatefulSet", Type: DriftMissing,
				Detail: fmt.Sprintf("StatefulSet %q expected but not found in cluster", w.Name),
			})
		}
		return drifts
	}

	actualReplicas := int32(1)
	if actual.Spec.Replicas != nil {
		actualReplicas = *actual.Spec.Replicas
	}
	if int(actualReplicas) != w.Replicas {
		drifts = append(drifts, Drift{
			DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
			Resource: w.Name, ResourceKind: "StatefulSet", Type: DriftReplicas,
			Detail: fmt.Sprintf("replicas: desired=%d actual=%d", w.Replicas, actualReplicas),
		})
	}

	if len(actual.Spec.Template.Spec.Containers) > 0 {
		actualImage := actual.Spec.Template.Spec.Containers[0].Image
		if actualImage != w.Image {
			drifts = append(drifts, Drift{
				DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
				Resource: w.Name, ResourceKind: "StatefulSet", Type: DriftImage,
				Detail: fmt.Sprintf("image: desired=%q actual=%q", w.Image, actualImage),
			})
		}
	}

	return drifts
}

func (c *Checker) checkCronJob(ctx context.Context, dep *deploymentstore.Deployment, w *deploymentstore.Workload, ns string) []Drift {
	var drifts []Drift
	clientset := c.k8sClient.Clientset()

	actual, err := clientset.BatchV1().CronJobs(ns).Get(ctx, w.Name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			drifts = append(drifts, Drift{
				DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
				Resource: w.Name, ResourceKind: "CronJob", Type: DriftMissing,
				Detail: fmt.Sprintf("CronJob %q expected but not found in cluster", w.Name),
			})
		}
		return drifts
	}

	if w.TriggerSchedule != nil && actual.Spec.Schedule != *w.TriggerSchedule {
		drifts = append(drifts, Drift{
			DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
			Resource: w.Name, ResourceKind: "CronJob", Type: DriftSchedule,
			Detail: fmt.Sprintf("schedule: desired=%q actual=%q", *w.TriggerSchedule, actual.Spec.Schedule),
		})
	}

	return drifts
}

func (c *Checker) checkServices(ctx context.Context, dep *deploymentstore.Deployment, services []*deploymentstore.Service, ns string) []Drift {
	var drifts []Drift
	clientset := c.k8sClient.Clientset()

	for _, desired := range services {
		_, err := clientset.CoreV1().Services(ns).Get(ctx, desired.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				drifts = append(drifts, Drift{
					DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
					Resource: desired.Name, ResourceKind: "Service", Type: DriftMissing,
					Detail: fmt.Sprintf("Service %q (port %d) expected but not found", desired.Name, desired.Port),
				})
			}
		}
	}

	return drifts
}

func (c *Checker) checkIngresses(ctx context.Context, dep *deploymentstore.Deployment, ingresses []*deploymentstore.Ingress, ns string) []Drift {
	var drifts []Drift
	clientset := c.k8sClient.Clientset()

	actualList, err := clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/managed-by=astro-server",
	})
	if err != nil {
		return drifts
	}

	actualHosts := map[string]bool{}
	for _, ing := range actualList.Items {
		for _, rule := range ing.Spec.Rules {
			actualHosts[rule.Host] = true
		}
	}

	for _, desired := range ingresses {
		if !actualHosts[desired.Hostname] {
			drifts = append(drifts, Drift{
				DeploymentID: dep.ID, Namespace: ns, AgentName: dep.AgentName,
				Resource: desired.Hostname, ResourceKind: "Ingress", Type: DriftMissing,
				Detail: fmt.Sprintf("Ingress for hostname %q expected but not found", desired.Hostname),
			})
		}
	}

	return drifts
}

func (c *Checker) LogReport(r *Report) {
	if len(r.Drifts) == 0 {
		c.log.Info("Drift check complete — no drift detected",
			"deployments_checked", r.DeploymentsChecked,
		)
		return
	}
	c.log.Warn("Drift check complete — drift detected",
		"deployments_checked", r.DeploymentsChecked,
		"drift_count", len(r.Drifts),
	)
	for _, d := range r.Drifts {
		c.log.Warn("Drift",
			"deployment_id", d.DeploymentID,
			"namespace", d.Namespace,
			"agent", d.AgentName,
			"resource", d.Resource,
			"kind", d.ResourceKind,
			"type", d.Type,
			"detail", d.Detail,
		)
	}
}
