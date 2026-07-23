package deploycontroller

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestHumanizeEvent(t *testing.T) {
	t.Run("FailedScheduling is a stuck/needs-action event", func(t *testing.T) {
		title, guidance, severity, ok := HumanizeEvent("FailedScheduling", "")
		if !ok || title == "" || severity != "stuck" {
			t.Fatalf("expected stuck humanization, got ok=%v title=%q severity=%q", ok, title, severity)
		}
		if !strings.Contains(guidance, "Advanced sizing") {
			t.Errorf("expected guidance to mention Advanced sizing, got %q", guidance)
		}
	})

	for _, reason := range []string{
		"Scheduled", "Pulling", "Pulled", "Created", "Started",
		"Unhealthy", "BackOff",
		"FailedScheduling", "FailedMount", "FailedAttachVolume",
	} {
		t.Run("mapped "+reason, func(t *testing.T) {
			title, guidance, severity, ok := HumanizeEvent(reason, "")
			if !ok || title == "" || guidance == "" || severity == "" {
				t.Errorf("expected %q humanized, got ok=%v title=%q guidance=%q severity=%q", reason, ok, title, guidance, severity)
			}
		})
	}

	stuckCases := []struct{ reason, message string }{
		{"ImagePullBackOff", ""},
		{"ErrImagePull", ""},
		{"CrashLoopBackOff", ""},
		{"Failed", `Failed to pull image "acme/agent:latest": not found`},
	}
	for _, tc := range stuckCases {
		t.Run("stuck "+tc.reason+" "+tc.message, func(t *testing.T) {
			title, guidance, severity, ok := HumanizeEvent(tc.reason, tc.message)
			if !ok || severity != "stuck" || title == "" || guidance == "" {
				t.Errorf("expected stuck for (%q,%q), got ok=%v title=%q severity=%q", tc.reason, tc.message, ok, title, severity)
			}
		})
	}

	// BackOff is a retry, not a terminal error — transient, never "Action required".
	for _, msg := range []string{`Back-off pulling image "acme/agent:latest"`, "Back-off restarting failed container agent", ""} {
		t.Run("backoff transient "+msg, func(t *testing.T) {
			title, _, severity, ok := HumanizeEvent("BackOff", msg)
			if !ok || severity != "transient" {
				t.Errorf("expected transient, got ok=%v severity=%q", ok, severity)
			}
			if strings.Contains(title, "Action required") {
				t.Errorf("BackOff title should not say Action required, got %q", title)
			}
		})
	}

	t.Run("generic Failed is left raw", func(t *testing.T) {
		title, guidance, severity, ok := HumanizeEvent("Failed", "Error: something else")
		if ok || title != "" || guidance != "" || severity != "" {
			t.Errorf("expected no humanization, got ok=%v title=%q guidance=%q severity=%q", ok, title, guidance, severity)
		}
	})

	for _, reason := range []string{"Killing", "Preempted", "SandboxChanged", ""} {
		t.Run("unmapped "+reason, func(t *testing.T) {
			_, _, _, ok := HumanizeEvent(reason, "")
			if ok {
				t.Errorf("expected %q left raw", reason)
			}
		})
	}
}

func TestBuildDeploymentEvents(t *testing.T) {
	ns := "astro-abc"
	base := time.Date(2026, 4, 16, 9, 0, 0, 0, time.UTC)

	older := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "e-older", Namespace: ns},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "agent-0"},
		Reason:         "Scheduled", Type: "Normal", Count: 1,
		FirstTimestamp: metav1.NewTime(base),
		LastTimestamp:  metav1.NewTime(base),
	}
	// A repeating event: LastTimestamp zero (events.k8s.io style), Series carries
	// the newest occurrence — must sort ahead of `older` despite a later creation.
	newer := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "e-newer", Namespace: ns, CreationTimestamp: metav1.NewTime(base)},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "agent-0"},
		Reason:         "BackOff", Message: "Back-off restarting failed container", Type: "Warning", Count: 5,
		EventTime: metav1.NewMicroTime(base),
		Series:    &corev1.EventSeries{Count: 5, LastObservedTime: metav1.NewMicroTime(base.Add(10 * time.Minute))},
	}
	// Event for an object not in the current set — must be dropped.
	stale := &corev1.Event{
		ObjectMeta:     metav1.ObjectMeta{Name: "e-stale", Namespace: ns},
		InvolvedObject: corev1.ObjectReference{Kind: "Pod", Name: "agent-OLD"},
		Reason:         "Killing", Type: "Normal", Count: 1,
		LastTimestamp: metav1.NewTime(base.Add(time.Hour)),
	}

	cs := fake.NewSimpleClientset(older, newer, stale)
	current := map[string]bool{"agent-0": true, "sasbot-agent": true}

	events, err := buildDeploymentEvents(context.Background(), cs, ns, current)
	if err != nil {
		t.Fatalf("buildDeploymentEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events (stale dropped), got %d: %+v", len(events), events)
	}
	// Newest-first: the Series event outranks the older one.
	if events[0].Reason != "BackOff" || events[1].Reason != "Scheduled" {
		t.Fatalf("unexpected order: %q, %q", events[0].Reason, events[1].Reason)
	}
	if events[0].LastTimestamp != base.Add(10*time.Minute).Format(time.RFC3339) {
		t.Errorf("expected Series LastObservedTime used, got %q", events[0].LastTimestamp)
	}
}
