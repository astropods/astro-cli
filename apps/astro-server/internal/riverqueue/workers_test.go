package riverqueue

import (
	"testing"

	"github.com/riverqueue/river/rivertype"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// ---------------------------------------------------------------------------
// statusOrNil
// ---------------------------------------------------------------------------

func TestStatusOrNil_NilDeployment(t *testing.T) {
	got := statusOrNil(nil)
	if got != "<nil>" {
		t.Errorf("statusOrNil(nil) = %q, want %q", got, "<nil>")
	}
}

func TestStatusOrNil_NonNilDeployment(t *testing.T) {
	dep := &deploymentstore.Deployment{Status: "active"}
	got := statusOrNil(dep)
	if got != "active" {
		t.Errorf("statusOrNil(dep) = %q, want %q", got, "active")
	}
}

// ---------------------------------------------------------------------------
// DeployArgs
// ---------------------------------------------------------------------------

func TestDeployArgs_Kind(t *testing.T) {
	args := DeployArgs{DeploymentID: "dep-123"}
	if kind := args.Kind(); kind != "deploy" {
		t.Errorf("DeployArgs.Kind() = %q, want %q", kind, "deploy")
	}
}

func TestDeployArgs_InsertOpts(t *testing.T) {
	opts := DeployArgs{}.InsertOpts()

	if opts.Queue != queueDeploy {
		t.Errorf("Queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Error("UniqueOpts.ByArgs should be true")
	}

	// ByState excludes completed/discarded so re-apply and reconciler
	// re-enqueue can create new jobs after the original finishes.
	wantStates := map[rivertype.JobState]bool{
		rivertype.JobStateAvailable: true,
		rivertype.JobStatePending:   true,
		rivertype.JobStateRunning:   true,
		rivertype.JobStateRetryable: true,
		rivertype.JobStateScheduled: true,
	}
	if len(opts.UniqueOpts.ByState) != len(wantStates) {
		t.Fatalf("ByState length = %d, want %d", len(opts.UniqueOpts.ByState), len(wantStates))
	}
	for _, s := range opts.UniqueOpts.ByState {
		if !wantStates[s] {
			t.Errorf("unexpected state in ByState: %v", s)
		}
	}
}

// ---------------------------------------------------------------------------
// UndeployArgs
// ---------------------------------------------------------------------------

func TestUndeployArgs_Kind(t *testing.T) {
	args := UndeployArgs{DeploymentID: "dep-456"}
	if kind := args.Kind(); kind != "undeploy" {
		t.Errorf("UndeployArgs.Kind() = %q, want %q", kind, "undeploy")
	}
}

func TestUndeployArgs_InsertOpts(t *testing.T) {
	opts := UndeployArgs{}.InsertOpts()

	if opts.Queue != queueDeploy {
		t.Errorf("Queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// WakeUpArgs
// ---------------------------------------------------------------------------

func TestWakeUpArgs_Kind(t *testing.T) {
	args := WakeUpArgs{DeploymentID: "dep-789"}
	if kind := args.Kind(); kind != "wakeup" {
		t.Errorf("WakeUpArgs.Kind() = %q, want %q", kind, "wakeup")
	}
}

func TestWakeUpArgs_InsertOpts(t *testing.T) {
	opts := WakeUpArgs{}.InsertOpts()

	if opts.Queue != queueDeploy {
		t.Errorf("Queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// ReconcileArgs
// ---------------------------------------------------------------------------

func TestReconcileArgs_Kind(t *testing.T) {
	args := ReconcileArgs{}
	if kind := args.Kind(); kind != "reconcile" {
		t.Errorf("ReconcileArgs.Kind() = %q, want %q", kind, "reconcile")
	}
}

// ---------------------------------------------------------------------------
// scaledObjectIsInactive
// ---------------------------------------------------------------------------

func TestScaledObjectIsInactive_ActiveFalse(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Active",
						"status": "False",
					},
				},
			},
		},
	}
	if !scaledObjectIsInactive(obj) {
		t.Error("expected inactive when Active condition is False")
	}
}

func TestScaledObjectIsInactive_ActiveTrue(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Active",
						"status": "True",
					},
				},
			},
		},
	}
	if scaledObjectIsInactive(obj) {
		t.Error("expected not inactive when Active condition is True")
	}
}

func TestScaledObjectIsInactive_NoConditions(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{},
		},
	}
	if scaledObjectIsInactive(obj) {
		t.Error("expected not inactive when no conditions present")
	}
}

func TestScaledObjectIsInactive_NoStatus(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{},
	}
	if scaledObjectIsInactive(obj) {
		t.Error("expected not inactive when no status present")
	}
}

func TestScaledObjectIsInactive_MalformedCondition(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					"not-a-map",
				},
			},
		},
	}
	if scaledObjectIsInactive(obj) {
		t.Error("expected not inactive when conditions contain non-map entry")
	}
}

func TestScaledObjectIsInactive_MultipleConditions(t *testing.T) {
	obj := unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
					},
					map[string]interface{}{
						"type":   "Active",
						"status": "False",
					},
				},
			},
		},
	}
	if !scaledObjectIsInactive(obj) {
		t.Error("expected inactive when Active=False among multiple conditions")
	}
}
