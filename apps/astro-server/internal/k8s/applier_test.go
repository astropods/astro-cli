package k8s

import (
	"context"
	"strings"
	"testing"
	"time"

	spec "github.com/astropods/astro/packages/astro-spec"
	batchv1 "k8s.io/api/batch/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/fake"
	clienttesting "k8s.io/client-go/testing"
)

// TestApplyDeploymentSpec_RedeployRecreatesStartupJob covers the redeploy flow
// end to end: a startup ingestion becomes a Job with a stable, build-independent
// name, so the second apply hits applyJob's already-exists branch. It must
// delete the prior Job and recreate it for the new build without surfacing a
// partial failure — the regression that produced
// `object is being deleted ... already exists`.
func TestApplyDeploymentSpec_RedeployRecreatesStartupJob(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()

	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"init": {
			Image:   "test-registry.example.com/ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "startup"},
		},
	}

	// First deploy (build-123 from minimalDeploymentSpec).
	if _, err := a.ApplyDeploymentSpec(ctx, ds); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	first, err := a.clientset.BatchV1().Jobs("default").Get(ctx, "my-agent-ingestion-init", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("expected startup Job after first apply: %v", err)
	}
	if got := first.Labels["app.kubernetes.io/version"]; got != "build-123" {
		t.Fatalf("first build version label: expected build-123, got %q", got)
	}

	// Redeploy with a new build. The Job name is unchanged, so this exercises
	// delete-then-recreate.
	ds.Source.Build = "build-456"
	result, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("redeploy apply: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("redeploy surfaced errors (race regression): %v", result.Errors)
	}

	// The Job resource status must report the recreate, not a failure.
	var jobStatus string
	for _, r := range result.Resources {
		if r.Kind == "Job" && r.Name == "my-agent-ingestion-init" {
			jobStatus = r.Status
		}
	}
	if jobStatus != "updated" {
		t.Fatalf("redeploy Job status: expected updated, got %q", jobStatus)
	}

	// Exactly one Job survives and carries the new build's version label,
	// proving it was recreated (not the stale build-123 Job left behind).
	jobs, err := a.clientset.BatchV1().Jobs("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected exactly 1 startup Job after redeploy, got %d", len(jobs.Items))
	}
	if got := jobs.Items[0].Labels["app.kubernetes.io/version"]; got != "build-456" {
		t.Fatalf("redeploy version label: expected build-456, got %q", got)
	}
}

// TestApplyDeploymentSpec_RedeploySurvivesTerminatingStartupJob reproduces the
// actual reported race at the full apply level. It models a real apiserver's
// async (foreground) deletion: after the startup Job is deleted it lingers
// "terminating" for one poll, during which a premature recreate fails with
// AlreadyExists ("object is being deleted"). The old immediate-recreate code
// hits this and reports a partial failure; the fix waits for the object to
// vanish first, so the redeploy succeeds. This test fails without the fix.
func TestApplyDeploymentSpec_RedeploySurvivesTerminatingStartupJob(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	fakeClient := a.clientset.(*fake.Clientset)

	const jobName = "my-agent-ingestion-init"

	ds := minimalDeploymentSpec()
	ds.Ingestion = map[string]spec.DeploymentIngestion{
		"init": {
			Image:   "test-registry.example.com/ingest:latest",
			Trigger: spec.DeploymentTrigger{Type: "startup"},
		},
	}

	// First deploy creates the startup Job normally (no reactors yet).
	if _, err := a.ApplyDeploymentSpec(ctx, ds); err != nil {
		t.Fatalf("first apply: %v", err)
	}

	// Install the terminating-window simulation for the startup Job only.
	terminating := false
	fakeClient.PrependReactor("delete", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.(clienttesting.DeleteAction).GetName() == jobName {
			terminating = true
		}
		return false, nil, nil // fall through so the tracker actually removes it
	})
	fakeClient.PrependReactor("get", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		if action.(clienttesting.GetAction).GetName() == jobName && terminating {
			// Report the object as still terminating for exactly one poll, then
			// let subsequent gets fall through to the (now empty) tracker.
			terminating = false
			ts := metav1.Now()
			return true, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
				Name: jobName, Namespace: "default", DeletionTimestamp: &ts,
			}}, nil
		}
		return false, nil, nil
	})
	fakeClient.PrependReactor("create", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		obj, _ := action.(clienttesting.CreateAction).GetObject().(*batchv1.Job)
		if obj != nil && obj.Name == jobName && terminating {
			// A recreate attempted before the object is gone is what fails on a
			// real cluster with "object is being deleted".
			return true, nil, apierrors.NewAlreadyExists(schema.GroupResource{Group: "batch", Resource: "jobs"}, jobName)
		}
		return false, nil, nil
	})

	// Redeploy with a new build. The stable Job name forces delete + recreate
	// through the terminating window.
	ds.Source.Build = "build-456"
	result, err := a.ApplyDeploymentSpec(ctx, ds)
	if err != nil {
		t.Fatalf("redeploy apply: %v", err)
	}
	if len(result.Errors) > 0 {
		t.Fatalf("redeploy hit the terminating-Job race (regression): %v", result.Errors)
	}

	jobs, err := a.clientset.BatchV1().Jobs("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected exactly 1 startup Job after redeploy, got %d", len(jobs.Items))
	}
	if got := jobs.Items[0].Labels["app.kubernetes.io/version"]; got != "build-456" {
		t.Fatalf("redeploy version label: expected build-456, got %q", got)
	}
}

// TestApplyJob_WaitsForTerminatingJobBeforeRecreate proves the core of the fix:
// when the existing Job is still terminating, applyJob polls until it is gone
// before recreating, instead of racing the delete with an immediate create. The
// fake tracker deletes synchronously, so a reactor simulates the object
// lingering for one poll tick.
func TestApplyJob_WaitsForTerminatingJobBeforeRecreate(t *testing.T) {
	a := newTestApplier()
	ctx := context.Background()
	fakeClient := a.clientset.(*fake.Clientset)

	const jobName = "my-agent-ingestion-init"

	// Seed a prior Job so the Create below returns AlreadyExists.
	if _, err := a.clientset.BatchV1().Jobs("default").Create(ctx, &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "default"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	// Report the Job as still present for the first poll, then gone. The delete
	// itself succeeds against the tracker; the reactor overrides Get so the
	// object appears to linger while terminating.
	deletedAt := metav1.Now()
	terminating := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: jobName, Namespace: "default", DeletionTimestamp: &deletedAt,
	}}
	var getCount int
	fakeClient.PrependReactor("get", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		getCount++
		if getCount <= 1 {
			return true, terminating, nil
		}
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: "batch", Resource: "jobs"}, jobName)
	})

	newJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
		Name: jobName, Namespace: "default",
		Labels: map[string]string{"app.kubernetes.io/version": "build-2"},
	}}

	status, err := a.applyJob(ctx, newJob)
	if err != nil {
		t.Fatalf("applyJob: unexpected error: %v", err)
	}
	if status.Status != "updated" {
		t.Fatalf("status: expected updated, got %q", status.Status)
	}
	if getCount < 2 {
		t.Fatalf("expected applyJob to poll until the Job was gone (>=2 gets), got %d", getCount)
	}

	// The recreated Job is the new one. List (not Get) to bypass the reactor.
	jobs, err := a.clientset.BatchV1().Jobs("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(jobs.Items) != 1 {
		t.Fatalf("expected 1 recreated Job, got %d", len(jobs.Items))
	}
	if got := jobs.Items[0].Labels["app.kubernetes.io/version"]; got != "build-2" {
		t.Fatalf("recreated Job version label: expected build-2, got %q", got)
	}
}

// TestApplyJob_FailsWhenExistingJobNeverTerminates covers the bounded-wait
// failure mode: if the existing Job never disappears (e.g. a stuck finalizer),
// applyJob returns an error rather than blocking forever or racing a create.
// A short-deadline context stands in for the 60s cap so the test is fast.
func TestApplyJob_FailsWhenExistingJobNeverTerminates(t *testing.T) {
	a := newTestApplier()
	fakeClient := a.clientset.(*fake.Clientset)

	const jobName = "my-agent-ingestion-init"
	if _, err := a.clientset.BatchV1().Jobs("default").Create(context.Background(), &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "default"},
	}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("seed job: %v", err)
	}

	// Always report the Job as still present.
	stuck := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "default"}}
	fakeClient.PrependReactor("get", "jobs", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, stuck, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	newJob := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: jobName, Namespace: "default"}}
	status, err := a.applyJob(ctx, newJob)
	if err == nil {
		t.Fatal("expected applyJob to fail when the existing Job never terminates")
	}
	if status.Status != "failed" {
		t.Fatalf("status: expected failed, got %q", status.Status)
	}
	if !strings.Contains(status.Message, "still terminating") {
		t.Fatalf("message: expected it to mention 'still terminating', got %q", status.Message)
	}
}
