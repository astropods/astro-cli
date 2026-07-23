package handlers

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

func wl(name, kind string) *deploymentstore.Workload {
	return &deploymentstore.Workload{Name: name, ComponentKind: kind}
}

func TestWaitingOnFromWorkloads(t *testing.T) {
	expected := []*deploymentstore.Workload{
		wl("sasbot-agent", "agent"),
		wl("sasbot-collector", "collector"),
		wl("sasbot-knowledge-ingest", "ingestion"),
	}

	t.Run("all ready -> empty", func(t *testing.T) {
		observed := []deploymentstore.WorkloadStatus{
			{WorkloadName: "sasbot-agent", Phase: deploymentstore.WorkloadPhaseReady},
			{WorkloadName: "sasbot-collector", Phase: deploymentstore.WorkloadPhaseReady},
			{WorkloadName: "sasbot-knowledge-ingest", Phase: deploymentstore.WorkloadPhaseComplete},
		}
		if got := waitingOnFromWorkloads(expected, observed); len(got) != 0 {
			t.Fatalf("expected no waiting workloads, got %+v", got)
		}
	})

	t.Run("declared but unobserved -> missing", func(t *testing.T) {
		observed := []deploymentstore.WorkloadStatus{
			{WorkloadName: "sasbot-agent", Phase: deploymentstore.WorkloadPhaseReady},
			{WorkloadName: "sasbot-collector", Phase: deploymentstore.WorkloadPhaseReady},
		}
		got := waitingOnFromWorkloads(expected, observed)
		if len(got) != 1 {
			t.Fatalf("expected 1 waiting workload, got %+v", got)
		}
		if got[0].Workload != "sasbot-knowledge-ingest" || got[0].Phase != WaitingPhaseMissing {
			t.Fatalf("expected missing ingestion workload, got %+v", got[0])
		}
		if got[0].Component != "ingestion" {
			t.Fatalf("expected component carried through, got %q", got[0].Component)
		}
	})

	t.Run("observed but not ready -> phase + reason", func(t *testing.T) {
		observed := []deploymentstore.WorkloadStatus{
			{WorkloadName: "sasbot-agent", Phase: deploymentstore.WorkloadPhaseProgressing, Reason: "ContainerCreating", Message: "pulling image"},
			{WorkloadName: "sasbot-collector", Phase: deploymentstore.WorkloadPhaseReady},
			{WorkloadName: "sasbot-knowledge-ingest", Phase: deploymentstore.WorkloadPhaseComplete},
		}
		got := waitingOnFromWorkloads(expected, observed)
		if len(got) != 1 {
			t.Fatalf("expected 1 waiting workload, got %+v", got)
		}
		if got[0].Workload != "sasbot-agent" || got[0].Phase != deploymentstore.WorkloadPhaseProgressing ||
			got[0].Message != "Starting up" {
			t.Fatalf("unexpected waiting workload: %+v", got[0])
		}
	})
}

func TestDeployingDetail(t *testing.T) {
	t.Run("names missing and progressing", func(t *testing.T) {
		got := deployingDetail([]WorkloadIssue{
			{Workload: "ingest", Phase: WaitingPhaseMissing},
			{Workload: "agent", Phase: deploymentstore.WorkloadPhaseProgressing, Message: "Starting up"},
		})
		want := "Waiting for ingest (not yet created), agent (Starting up)"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("elides beyond max named", func(t *testing.T) {
		got := deployingDetail([]WorkloadIssue{
			{Workload: "a", Phase: WaitingPhaseMissing},
			{Workload: "b", Phase: WaitingPhaseMissing},
			{Workload: "c", Phase: WaitingPhaseMissing},
			{Workload: "d", Phase: WaitingPhaseMissing},
			{Workload: "e", Phase: WaitingPhaseMissing},
		})
		want := "Waiting for a (not yet created), b (not yet created), c (not yet created) and 2 more"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})

	t.Run("falls back to phase when no reason", func(t *testing.T) {
		got := deployingDetail([]WorkloadIssue{
			{Workload: "agent", Phase: deploymentstore.WorkloadPhaseProgressing},
		})
		want := "Waiting for agent (progressing)"
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}

func TestFailedWorkloads(t *testing.T) {
	expected := []*deploymentstore.Workload{
		wl("sasbot-agent", "agent"),
		wl("sasbot-collector", "collector"),
	}
	observed := []deploymentstore.WorkloadStatus{
		{WorkloadName: "sasbot-agent", Phase: deploymentstore.WorkloadPhaseFailed, Reason: "ImagePullBackOff", Message: "app: back-off pulling image"},
		{WorkloadName: "sasbot-collector", Phase: deploymentstore.WorkloadPhaseReady},
	}
	got := failedWorkloads(expected, observed)
	if len(got) != 1 {
		t.Fatalf("expected 1 failed workload, got %+v", got)
	}
	// The raw K8s reason is humanized; the cryptic code is never surfaced.
	if got[0].Workload != "sasbot-agent" || got[0].Component != "agent" ||
		got[0].Message != "Couldn't pull the container image" {
		t.Fatalf("unexpected failed workload: %+v", got[0])
	}
}

func TestFailedDetail(t *testing.T) {
	got := failedDetail([]WorkloadIssue{
		{Workload: "sasbot-agent", Phase: deploymentstore.WorkloadPhaseFailed, Message: "Couldn't pull the container image"},
	})
	want := "Deployment failed: sasbot-agent (Couldn't pull the container image)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestHumanizeWorkloadReason(t *testing.T) {
	if got := humanizeWorkloadReason("ImagePullBackOff"); got != "Couldn't pull the container image" {
		t.Fatalf("ImagePullBackOff => %q", got)
	}
	if got := humanizeWorkloadReason("SomeUnknownCode"); got != "" {
		t.Fatalf("unknown code should humanize to empty, got %q", got)
	}
}
