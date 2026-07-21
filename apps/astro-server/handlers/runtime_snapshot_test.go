package handlers

import (
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

func TestMessagingReachableFromSnapshot(t *testing.T) {
	const agent = "my-agent"
	svc := deployment.GenerateAgentResourceName(agent, "messaging")
	msgService := []deploymentstore.RuntimeService{{Name: svc, Type: "ClusterIP"}}

	agentPod := func(containers ...deploymentstore.RuntimeContainer) []deploymentstore.RuntimeWorkload {
		return []deploymentstore.RuntimeWorkload{{
			Name: "agent", Kind: "Deployment",
			Pods: []deploymentstore.RuntimePod{{Name: "agent-0", Containers: containers}},
		}}
	}

	tests := []struct {
		name string
		snap deploymentstore.RuntimeSnapshot
		want bool
	}{
		{
			name: "no messaging Service → unreachable",
			snap: deploymentstore.RuntimeSnapshot{Workloads: agentPod(
				deploymentstore.RuntimeContainer{Name: "messaging", Ready: true},
			)},
			want: false,
		},
		{
			name: "Service present, sidecar not observed → reachable (fall back to Service presence)",
			snap: deploymentstore.RuntimeSnapshot{
				Services:  msgService,
				Workloads: agentPod(deploymentstore.RuntimeContainer{Name: "agent", Ready: true}),
			},
			want: true,
		},
		{
			name: "Service present, sidecar ready → reachable",
			snap: deploymentstore.RuntimeSnapshot{
				Services: msgService,
				Workloads: agentPod(
					deploymentstore.RuntimeContainer{Name: "agent", Ready: true},
					deploymentstore.RuntimeContainer{Name: "messaging", Ready: true},
				),
			},
			want: true,
		},
		{
			name: "Service present, sidecar wedged → unreachable (the prod 5xx case)",
			snap: deploymentstore.RuntimeSnapshot{
				Services: msgService,
				Workloads: agentPod(
					deploymentstore.RuntimeContainer{Name: "agent", Ready: true},
					deploymentstore.RuntimeContainer{Name: "messaging", Ready: false},
				),
			},
			want: false,
		},
		{
			name: "sidecar in a non-first workload → found and used",
			snap: deploymentstore.RuntimeSnapshot{
				Services: msgService,
				Workloads: []deploymentstore.RuntimeWorkload{
					{Name: "collector", Pods: []deploymentstore.RuntimePod{{Name: "c-0", Containers: []deploymentstore.RuntimeContainer{{Name: "collector", Ready: true}}}}},
					{Name: "agent", Pods: []deploymentstore.RuntimePod{{Name: "a-0", Containers: []deploymentstore.RuntimeContainer{{Name: "messaging", Ready: true}}}}},
				},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := messagingReachableFromSnapshot(&tt.snap, agent); got != tt.want {
				t.Errorf("messagingReachableFromSnapshot() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRepresentativePod(t *testing.T) {
	old := time.Now().Add(-time.Hour)
	newer := time.Now()

	tests := []struct {
		name string
		pods []deploymentstore.RuntimePod
		want string // expected pod name, "" for nil
	}{
		{name: "no pods", pods: nil, want: ""},
		{
			name: "prefers Running over Pending regardless of age",
			pods: []deploymentstore.RuntimePod{
				{Name: "pending-new", Phase: "Pending", CreatedAt: newer},
				{Name: "running-old", Phase: "Running", CreatedAt: old},
			},
			want: "running-old",
		},
		{
			name: "among same phase prefers newest",
			pods: []deploymentstore.RuntimePod{
				{Name: "running-old", Phase: "Running", CreatedAt: old},
				{Name: "running-new", Phase: "Running", CreatedAt: newer},
			},
			want: "running-new",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := representativePod(tt.pods)
			if tt.want == "" {
				if got != nil {
					t.Fatalf("expected nil, got %q", got.Name)
				}
				return
			}
			if got == nil || got.Name != tt.want {
				t.Fatalf("representativePod() = %v, want %q", got, tt.want)
			}
		})
	}
}

func TestWorkloadRuntimesFromSnapshot(t *testing.T) {
	workloads := []deploymentstore.RuntimeWorkload{
		{
			Name:      "agent",
			Kind:      "Deployment",
			CreatedAt: time.Now().Add(-2 * time.Minute),
			Pods: []deploymentstore.RuntimePod{
				{Name: "agent-old", Phase: "Running", CreatedAt: time.Now().Add(-time.Hour), Containers: []deploymentstore.RuntimeContainer{{Name: "app", State: "Running", Ready: true}}},
				{Name: "agent-new", Phase: "Running", CreatedAt: time.Now(), Containers: []deploymentstore.RuntimeContainer{{Name: "app", State: "Running", Ready: true, RestartCount: 2}}},
			},
		},
		{Name: "ingest", Kind: "Job", Status: "Succeeded"},
	}

	got := workloadRuntimesFromSnapshot(workloads)
	if len(got) != 2 {
		t.Fatalf("expected 2 workloads, got %d", len(got))
	}
	// Agent workload picks the newest Running pod and its containers.
	if got[0].PodName != "agent-new" {
		t.Errorf("expected representative pod agent-new, got %q", got[0].PodName)
	}
	if len(got[0].Containers) != 1 || got[0].Containers[0].RestartCount != 2 {
		t.Errorf("expected the newer pod's containers, got %+v", got[0].Containers)
	}
	if got[0].Age == "" {
		t.Errorf("expected a formatted age for the agent workload")
	}
	// Job workload carries its status and has no representative pod.
	if got[1].Status != "Succeeded" || got[1].PodName != "" {
		t.Errorf("unexpected job projection: %+v", got[1])
	}
}
