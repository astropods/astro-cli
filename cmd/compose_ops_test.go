package cmd

import (
	"sort"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/assert"
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

func TestProjectForRun(t *testing.T) {
	p := &types.Project{
		Name: "test",
		Services: types.Services{
			"agent":              {Name: "agent"},
			"neo4j":              {Name: "neo4j"},
			"ingestion-startup":  {Name: "ingestion-startup", Profiles: []string{"ingestion"}},
			"ingestion-schedule": {Name: "ingestion-schedule", Profiles: []string{"ingestion"}},
		},
	}

	run := projectForRun(p, "ingestion-startup")

	assert.ElementsMatch(t, []string{"agent", "neo4j", "ingestion-startup"}, allServiceNames(run),
		"a run project carries the services up starts, plus the target ingestion only")
	assert.Contains(t, allServiceNames(p), "ingestion-schedule", "the original project must not be mutated")
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

func TestGroupStartupServices(t *testing.T) {
	astroSpec := &spec.AstroSpec{
		Ingestion: map[string]spec.Ingestion{
			"startup":  {Trigger: spec.IngestionTrigger{Type: "startup"}},
			"schedule": {Trigger: spec.IngestionTrigger{Type: "schedule"}},
			"webhook":  {Trigger: spec.IngestionTrigger{Type: "webhook"}},
		},
	}

	tests := []struct {
		name          string
		services      types.Services
		wantServices  []string
		wantIngestion []string
	}{
		{
			name: "profiled ingestions are reported apart from started services",
			services: types.Services{
				"agent":              {Name: "agent"},
				"knowledge-graph":    {Name: "knowledge-graph"},
				"ingestion-startup":  {Name: "ingestion-startup", Profiles: []string{"ingestion"}},
				"ingestion-schedule": {Name: "ingestion-schedule", Profiles: []string{"ingestion"}},
			},
			wantServices:  []string{"agent", "knowledge-graph"},
			wantIngestion: []string{"ingestion-schedule (on demand)", "ingestion-startup (once at start)"},
		},
		{
			name: "webhook ingestion is unprofiled, so it counts as a started service",
			services: types.Services{
				"agent":             {Name: "agent"},
				"ingestion-webhook": {Name: "ingestion-webhook"},
			},
			wantServices:  []string{"agent", "ingestion-webhook"},
			wantIngestion: nil,
		},
		{
			name: "a profiled service with no matching spec entry falls back to on demand",
			services: types.Services{
				"agent":            {Name: "agent"},
				"ingestion-absent": {Name: "ingestion-absent", Profiles: []string{"ingestion"}},
			},
			wantServices:  []string{"agent"},
			wantIngestion: []string{"ingestion-absent (on demand)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			services, ingestion := groupStartupServices(&types.Project{Services: tt.services}, astroSpec)
			assert.Equal(t, tt.wantServices, services)
			assert.Equal(t, tt.wantIngestion, ingestion)
		})
	}
}
