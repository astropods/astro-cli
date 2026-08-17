//go:build k8s

// Coverage for the step that makes a billing suspension mean something: scaling
// an account's workloads to zero against a real cluster.
//
// Everything upstream of this is well covered. The status machine, the dunning
// timer, and the 402 all have tests, and all of them stop at the point where
// BillingSuspendWorker calls StopNamespaceWorkloads. That call is what actually
// stops a customer's agents, and a suspension that leaves them running bills the
// account for the period it was supposed to have stopped.
//
// Run: go test -tags k8s ./e2e/...
// CI job: `K8s integration tests (vcluster + Postgres)` in .github/workflows/test.yml.
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/k8s"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// workloadReplicas reports the replica count of every managed deployment and
// statefulset in the namespace, keyed by name.
func workloadReplicas(t *testing.T, clientset kubernetes.Interface, ns string) map[string]int32 {
	t.Helper()
	ctx := context.Background()
	opts := metav1.ListOptions{LabelSelector: k8s.ManagedByLabel}
	out := map[string]int32{}

	deps, err := clientset.AppsV1().Deployments(ns).List(ctx, opts)
	if err != nil {
		t.Fatalf("list deployments: %v", err)
	}
	for _, d := range deps.Items {
		var n int32
		if d.Spec.Replicas != nil {
			n = *d.Spec.Replicas
		}
		out["deployment/"+d.Name] = n
	}

	sets, err := clientset.AppsV1().StatefulSets(ns).List(ctx, opts)
	if err != nil {
		t.Fatalf("list statefulsets: %v", err)
	}
	for _, s := range sets.Items {
		var n int32
		if s.Spec.Replicas != nil {
			n = *s.Spec.Replicas
		}
		out["statefulset/"+s.Name] = n
	}
	return out
}

// A suspension has to reach both workload kinds. The agent is a Deployment and
// persistent knowledge is a StatefulSet, so stopping only one leaves the account
// running half its footprint.
func TestK8s_BillingStopScalesEveryWorkloadToZero(t *testing.T) {
	client := clusterClient(t)
	clientset := client.Clientset()
	ns := uniqueNS(t)
	cleanupNamespace(t, clientset, ns)

	applyMinimalSpec(t, client, ns)

	before := workloadReplicas(t, clientset, ns)
	if len(before) < 2 {
		t.Fatalf("applied %d workloads, want a deployment and a statefulset: %v", len(before), before)
	}
	var running int
	for _, n := range before {
		if n > 0 {
			running++
		}
	}
	if running == 0 {
		t.Fatalf("nothing was running before the suspension: %v", before)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := k8s.StopNamespaceWorkloads(ctx, clientset, ns); err != nil {
		t.Fatalf("StopNamespaceWorkloads: %v", err)
	}

	for name, n := range workloadReplicas(t, clientset, ns) {
		if n != 0 {
			t.Errorf("%s still has %d replicas after the suspension", name, n)
		}
	}
}

// Resume re-applies the spec rather than remembering a replica count, so the
// applier has to raise a workload it previously scaled to zero. An applier that
// treated replicas as immutable, or skipped a resource whose spec had not
// changed, would leave a paying account stopped.
func TestK8s_BillingResumeRestoresTheReplicas(t *testing.T) {
	client := clusterClient(t)
	clientset := client.Clientset()
	ns := uniqueNS(t)
	cleanupNamespace(t, clientset, ns)

	applyMinimalSpec(t, client, ns)
	before := workloadReplicas(t, clientset, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := k8s.StopNamespaceWorkloads(ctx, clientset, ns); err != nil {
		t.Fatalf("StopNamespaceWorkloads: %v", err)
	}
	// Without this the case passes on a stop that never stopped anything.
	for name, n := range workloadReplicas(t, clientset, ns) {
		if n != 0 {
			t.Fatalf("%s was not stopped, so the restore proves nothing", name)
		}
	}

	applyMinimalSpec(t, client, ns)

	after := workloadReplicas(t, clientset, ns)
	for name, want := range before {
		if after[name] != want {
			t.Errorf("%s resumed at %d replicas, want %d", name, after[name], want)
		}
	}
}

// The sweep can suspend an account that is already suspended, and a card change
// enqueues a reconcile on every save rather than only on a transition. A second
// pass must be a no-op rather than an error that fails the job and retries.
func TestK8s_BillingRepeatedStopIsANoop(t *testing.T) {
	client := clusterClient(t)
	clientset := client.Clientset()
	ns := uniqueNS(t)
	cleanupNamespace(t, clientset, ns)

	applyMinimalSpec(t, client, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	for i := 0; i < 2; i++ {
		if err := k8s.StopNamespaceWorkloads(ctx, clientset, ns); err != nil {
			t.Fatalf("StopNamespaceWorkloads pass %d: %v", i+1, err)
		}
	}

	for name, n := range workloadReplicas(t, clientset, ns) {
		if n != 0 {
			t.Errorf("%s has %d replicas after two passes", name, n)
		}
	}
}

// A scheduled ingestion keeps consuming after the agent stops, because a
// CronJob has no replica count to zero. Suspending it is a separate branch from
// the two scale calls, and it is the one a replica-only reading of "stop the
// workloads" misses.
func TestK8s_BillingCronJobsAreSuspended(t *testing.T) {
	client := clusterClient(t)
	clientset := client.Clientset()
	ns := uniqueNS(t)
	cleanupNamespace(t, clientset, ns)

	applyIngestionScheduleSpec(t, client, ns)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	opts := metav1.ListOptions{LabelSelector: k8s.ManagedByLabel}
	cronJobs, err := clientset.BatchV1().CronJobs(ns).List(ctx, opts)
	if err != nil {
		t.Fatalf("list cronjobs: %v", err)
	}
	if len(cronJobs.Items) == 0 {
		t.Fatal("the spec created no cronjob, so this proves nothing")
	}

	if err := k8s.StopNamespaceWorkloads(ctx, clientset, ns); err != nil {
		t.Fatalf("StopNamespaceWorkloads: %v", err)
	}

	after, err := clientset.BatchV1().CronJobs(ns).List(ctx, opts)
	if err != nil {
		t.Fatalf("list cronjobs after: %v", err)
	}
	for _, cj := range after.Items {
		if cj.Spec.Suspend == nil || !*cj.Spec.Suspend {
			t.Errorf("cronjob %s is still scheduled after the suspension", cj.Name)
		}
	}
}
