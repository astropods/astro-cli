package handlers

import "testing"

func TestMessagingSidecarReadiness(t *testing.T) {
	tests := []struct {
		name      string
		workloads []WorkloadDetail
		wantFound bool
		wantReady bool
	}{
		{
			name:      "no workloads",
			workloads: nil,
			wantFound: false,
			wantReady: false,
		},
		{
			name: "no messaging container present (fall back to Service presence)",
			workloads: []WorkloadDetail{
				{Name: "agent", Containers: []ContainerStatus{{Name: "agent", Ready: true}}},
			},
			wantFound: false,
			wantReady: false,
		},
		{
			name: "messaging sidecar ready",
			workloads: []WorkloadDetail{
				{Name: "agent", Containers: []ContainerStatus{
					{Name: "agent", Ready: true},
					{Name: "messaging", Ready: true},
				}},
			},
			wantFound: true,
			wantReady: true,
		},
		{
			name: "messaging sidecar present but wedged (the prod 5xx case)",
			workloads: []WorkloadDetail{
				{Name: "agent", Containers: []ContainerStatus{
					{Name: "agent", Ready: true},
					{Name: "messaging", Ready: false},
				}},
			},
			wantFound: true,
			wantReady: false,
		},
		{
			name: "messaging container found in a non-first workload",
			workloads: []WorkloadDetail{
				{Name: "collector", Containers: []ContainerStatus{{Name: "collector", Ready: true}}},
				{Name: "agent", Containers: []ContainerStatus{{Name: "messaging", Ready: true}}},
			},
			wantFound: true,
			wantReady: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found, ready := messagingSidecarReadiness(tt.workloads)
			if found != tt.wantFound || ready != tt.wantReady {
				t.Errorf("messagingSidecarReadiness() = (found=%v, ready=%v), want (found=%v, ready=%v)",
					found, ready, tt.wantFound, tt.wantReady)
			}
		})
	}
}
