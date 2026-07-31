package cmd

import (
	"sort"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
)

func TestProjectForUp(t *testing.T) {
	p := &types.Project{
		Name: "test",
		Services: types.Services{
			"agent": {Name: "agent"},
			"model": {Name: "model"},
			"ingestion-run": {
				Name:     "ingestion-run",
				Profiles: []string{"ingestion"},
			},
		},
	}

	up := projectForUp(p)

	if _, ok := up.Services["agent"]; !ok {
		t.Error("agent should be included in up project")
	}
	if _, ok := up.Services["model"]; !ok {
		t.Error("model should be included in up project")
	}
	if _, ok := up.Services["ingestion-run"]; ok {
		t.Error("profiled ingestion-run should be excluded from up project")
	}

	// original project must not be mutated
	if _, ok := p.Services["ingestion-run"]; !ok {
		t.Error("original project should still contain ingestion-run")
	}
}

func TestProjectForUp_WebhookIngestionIncluded(t *testing.T) {
	// Webhook ingestions have no profiles and must survive projectForUp.
	p := &types.Project{
		Name: "test",
		Services: types.Services{
			"agent":              {Name: "agent"},
			"ingestion-webhook":  {Name: "ingestion-webhook"},
			"ingestion-schedule": {Name: "ingestion-schedule", Profiles: []string{"ingestion"}},
		},
	}

	up := projectForUp(p)

	if _, ok := up.Services["ingestion-webhook"]; !ok {
		t.Error("webhook ingestion (no profiles) should be included in up project")
	}
	if _, ok := up.Services["ingestion-schedule"]; ok {
		t.Error("schedule ingestion (profiled) should be excluded from up project")
	}
}

func TestAllServiceNames(t *testing.T) {
	p := &types.Project{
		Services: types.Services{
			"agent":   {Name: "agent"},
			"model-x": {Name: "model-x"},
			"kb":      {Name: "kb"},
		},
	}

	names := allServiceNames(p)
	sort.Strings(names)

	want := []string{"agent", "kb", "model-x"}
	if len(names) != len(want) {
		t.Fatalf("allServiceNames() = %v, want %v", names, want)
	}
	for i, n := range names {
		if n != want[i] {
			t.Errorf("names[%d] = %q, want %q", i, n, want[i])
		}
	}
}

func TestComposeEventIcon(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{api.StatusPulled, "⬇️ "},
		{api.StatusBuilt, "🔨"},
		{api.StatusCreated, "📦"},
		{api.StatusStarted, "✅"},
		{api.StatusRunning, "✅"},
		{api.StatusHealthy, "💚"},
		{api.StatusStopped, "⏹️ "},
		{api.StatusKilled, "⏹️ "},
		{api.StatusExited, "⏹️ "},
		{api.StatusRemoved, "🗑️ "},
		{"unknown-status", "✅"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got := composeEventIcon(tt.text)
			if got != tt.want {
				t.Errorf("composeEventIcon(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}
