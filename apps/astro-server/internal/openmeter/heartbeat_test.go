package openmeter

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "modernc.org/sqlite"
)

func TestParseCPU(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"100m", 0.1},
		{"250m", 0.25},
		{"1", 1},
		{"2", 2},
		{"1.5", 1.5},
		{"", 0},
		{"0m", 0},
	}
	for _, tt := range tests {
		got := parseCPU(tt.input)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("parseCPU(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestParseMemory(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"1Gi", 1},
		{"256Mi", 0.25},
		{"512Mi", 0.5},
		{"2Gi", 2},
		{"128Mi", 0.125},
		{"1G", 1},
		{"500M", 0.5},
		{"", 0},
	}
	for _, tt := range tests {
		got := parseMemory(tt.input)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("parseMemory(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}

func TestContainerBreakdown_BasicAgent(t *testing.T) {
	s := &spec.AstroDeploymentSpec{
		Agent: spec.DeploymentAgent{
			Replicas:  1,
			Resources: spec.DeploymentResources{CPU: "1", Memory: "2Gi"},
		},
	}
	containers := containerBreakdown(s)
	if len(containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(containers))
	}
	if containers[0].Component != "agent" {
		t.Errorf("expected component 'agent', got %q", containers[0].Component)
	}
	// CU = max(1, 2/2) = 1
	if math.Abs(containers[0].CU-1) > 0.001 {
		t.Errorf("expected CU=1, got %f", containers[0].CU)
	}
}

func TestContainerBreakdown_MemoryHeavy(t *testing.T) {
	s := &spec.AstroDeploymentSpec{
		Agent: spec.DeploymentAgent{
			Replicas:  1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "4Gi"},
		},
	}
	containers := containerBreakdown(s)
	// CU = max(0.1, 4/2) = 2
	if math.Abs(containers[0].CU-2) > 0.001 {
		t.Errorf("expected CU=2, got %f", containers[0].CU)
	}
}

func TestContainerBreakdown_WithReplicas(t *testing.T) {
	s := &spec.AstroDeploymentSpec{
		Agent: spec.DeploymentAgent{
			Replicas:  3,
			Resources: spec.DeploymentResources{CPU: "1", Memory: "2Gi"},
		},
	}
	containers := containerBreakdown(s)
	// CU = max(1, 1) * 3 = 3
	if math.Abs(containers[0].CU-3) > 0.001 {
		t.Errorf("expected CU=3, got %f", containers[0].CU)
	}
	if containers[0].Replicas != 3 {
		t.Errorf("expected replicas=3, got %d", containers[0].Replicas)
	}
}

func TestContainerBreakdown_MultipleContainers(t *testing.T) {
	s := &spec.AstroDeploymentSpec{
		Agent: spec.DeploymentAgent{
			Replicas:  1,
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "256Mi"},
		},
		Models: map[string]spec.DeploymentModel{
			"llm": {
				Replicas:  1,
				Resources: spec.DeploymentResources{CPU: "2", Memory: "8Gi"},
			},
		},
		Interfaces: &spec.DeploymentInterfaces{
			Resources: spec.DeploymentResources{CPU: "100m", Memory: "128Mi"},
		},
	}
	containers := containerBreakdown(s)
	if len(containers) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(containers))
	}

	// Find each by component name
	byName := map[string]containerUsage{}
	for _, c := range containers {
		byName[c.Component] = c
	}

	// Agent: max(0.1, 0.25/2) = 0.125
	if math.Abs(byName["agent"].CU-0.125) > 0.01 {
		t.Errorf("agent CU: expected 0.125, got %f", byName["agent"].CU)
	}
	// Model: max(2, 8/2) = 4
	if math.Abs(byName["model/llm"].CU-4) > 0.01 {
		t.Errorf("model/llm CU: expected 4, got %f", byName["model/llm"].CU)
	}
	// Interfaces: max(0.1, 0.125/2) = 0.1
	if math.Abs(byName["interfaces"].CU-0.1) > 0.01 {
		t.Errorf("interfaces CU: expected 0.1, got %f", byName["interfaces"].CU)
	}
}

func TestHeartbeat_EmitComputeUsage_Normalized(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	client := NewClient(srv.URL)
	log := logger.New("error", "json")

	// Normalized workloads query returns data — JSON fallback should not be called
	mock.ExpectQuery("SELECT d.account_id.+FROM deployments d.+JOIN deployment_workloads w").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "agent_name", "namespace",
			"component_kind", "component_key", "replicas", "cpu_request", "memory_request",
		}).
			AddRow("acct-1", "my-agent", "astro-abc", "agent", "", 1, "1", "2Gi").
			AddRow("acct-1", "my-agent", "astro-abc", "model", "llm", 1, "2", "8Gi"))

	hb := &Heartbeat{client: client, db: db, log: log}
	hb.emitComputeUsage(context.Background())

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	// Verify component names use kind/key format
	byComponent := map[string]CloudEvent{}
	for _, ev := range received {
		data := ev.Data.(map[string]any)
		byComponent[data["component"].(string)] = ev
	}
	if _, ok := byComponent["agent"]; !ok {
		t.Error("missing 'agent' component event")
	}
	if _, ok := byComponent["model/llm"]; !ok {
		t.Error("missing 'model/llm' component event")
	}
	for _, ev := range received {
		if ev.Type != "compute_usage" {
			t.Errorf("expected type 'compute_usage', got %q", ev.Type)
		}
		if ev.Subject != "acct-1" {
			t.Errorf("expected subject 'acct-1', got %q", ev.Subject)
		}
	}
}

func TestHeartbeat_EmitComputeUsage_FallbackToJSON(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	client := NewClient(srv.URL)
	log := logger.New("error", "json")

	// Normalized query returns empty (no workload data for old deployments)
	mock.ExpectQuery("SELECT d.account_id.+FROM deployments d.+JOIN deployment_workloads w").
		WillReturnRows(sqlmock.NewRows([]string{
			"account_id", "agent_name", "namespace",
			"component_kind", "component_key", "replicas", "cpu_request", "memory_request",
		}))

	// Falls back to JSON parsing
	depSpec := spec.AstroDeploymentSpec{
		Agent: spec.DeploymentAgent{
			Replicas:  1,
			Resources: spec.DeploymentResources{CPU: "1", Memory: "2Gi"},
		},
		Models: map[string]spec.DeploymentModel{
			"llm": {
				Replicas:  1,
				Resources: spec.DeploymentResources{CPU: "2", Memory: "8Gi"},
			},
		},
	}
	specJSON, _ := json.Marshal(depSpec)

	mock.ExpectQuery("SELECT .+ FROM deployments WHERE status = 'active'").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "agent_name", "namespace", "deployment_spec_json"}).
			AddRow("acct-1", "my-agent", "astro-abc", string(specJSON)))

	hb := &Heartbeat{client: client, db: db, log: log}
	hb.emitComputeUsage(context.Background())

	// Should still get 2 events from JSON fallback
	if len(received) != 2 {
		t.Fatalf("expected 2 events from JSON fallback, got %d", len(received))
	}
	for _, ev := range received {
		if ev.Type != "compute_usage" {
			t.Errorf("expected type 'compute_usage', got %q", ev.Type)
		}
	}
}

func TestHeartbeat_EmitActiveDeployments(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	client := NewClient(srv.URL)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT account_id, COUNT.+ FROM deployments WHERE status = 'active'").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "cnt"}).
			AddRow("acct-1", 3).
			AddRow("acct-2", 1))

	hb := &Heartbeat{client: client, db: db, log: log}
	hb.emitActiveDeployments(context.Background())

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != "active_deployments" {
		t.Errorf("expected type 'active_deployments', got %q", received[0].Type)
	}
	if received[0].Subject != "acct-1" {
		t.Errorf("expected subject 'acct-1', got %q", received[0].Subject)
	}
}

func TestHeartbeat_EmitActiveAgents(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	client := NewClient(srv.URL)
	log := logger.New("error", "json")

	// Query must filter out archived agents (archived_at IS NULL).
	mock.ExpectQuery("SELECT account_id, COUNT.+ FROM agents WHERE archived_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "cnt"}).
			AddRow("acct-1", 5))

	hb := &Heartbeat{client: client, db: db, log: log}
	hb.emitActiveAgents(context.Background())

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "active_agents" {
		t.Errorf("expected type 'active_agents', got %q", received[0].Type)
	}
}

func TestHeartbeat_EmitActiveAgents_ExcludesArchived(t *testing.T) {
	// Use an in-memory SQLite DB so we can insert real rows and verify the
	// WHERE archived_at IS NULL filter actually excludes archived blueprints.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE agents (
		account_id  TEXT NOT NULL,
		archived_at DATETIME
	)`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// acct-1: 2 active, 1 archived
	db.Exec(`INSERT INTO agents VALUES ('acct-1', NULL)`)
	db.Exec(`INSERT INTO agents VALUES ('acct-1', NULL)`)
	db.Exec(`INSERT INTO agents VALUES ('acct-1', '2026-01-01 00:00:00')`)

	db.Exec(`INSERT INTO agents VALUES ('acct-2', '2026-01-01 00:00:00')`)

	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	hb := &Heartbeat{client: NewClient(srv.URL), db: db, log: logger.New("error", "json")}
	hb.emitActiveAgents(context.Background())

	if len(received) != 1 {
		t.Fatalf("expected 1 event (acct-1 only), got %d", len(received))
	}
	if received[0].Subject != "acct-1" {
		t.Errorf("expected subject 'acct-1', got %q", received[0].Subject)
	}
	data := received[0].Data.(map[string]any)
	if count := data["count"].(float64); count != 2 {
		t.Errorf("expected count=2 (archived excluded), got %v", count)
	}
}
