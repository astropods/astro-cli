package openmeter

import (
	"context"
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

	hb := &Heartbeat{provider: NewProvider(client), db: db, log: log}
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

	hb := &Heartbeat{provider: NewProvider(client), db: db, log: log}
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




func TestHeartbeat_EmitKnowledgeStorage(t *testing.T) {
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

	mock.ExpectQuery("SELECT account_id, name, provider, storage FROM knowledge_stores WHERE mode = 'managed'").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "provider", "storage"}).
			AddRow("acct-1", "my-pg", "postgres", "10Gi").
			AddRow("acct-1", "my-redis", "redis", "1Gi").
			AddRow("acct-2", "docs-db", "postgres", "50Gi"))

	hb := &Heartbeat{provider: NewProvider(client), db: db, log: log}
	hb.emitKnowledgeStorage(context.Background())

	if len(received) != 3 {
		t.Fatalf("expected 3 events, got %d", len(received))
	}
	for _, ev := range received {
		if ev.Type != "knowledge_storage_provisioned" {
			t.Errorf("expected type 'knowledge_storage_provisioned', got %q", ev.Type)
		}
	}

	// Verify per-store granularity
	data0 := received[0].Data.(map[string]any)
	if data0["store_name"] != "my-pg" {
		t.Errorf("expected store_name='my-pg', got %v", data0["store_name"])
	}
	if gb := data0["storage_gb"].(float64); math.Abs(gb-10) > 0.01 {
		t.Errorf("expected storage_gb=10, got %v", gb)
	}

	data2 := received[2].Data.(map[string]any)
	if data2["store_name"] != "docs-db" {
		t.Errorf("expected store_name='docs-db', got %v", data2["store_name"])
	}
	if gb := data2["storage_gb"].(float64); math.Abs(gb-50) > 0.01 {
		t.Errorf("expected storage_gb=50, got %v", gb)
	}
}

func TestHeartbeat_EmitKnowledgeCompute(t *testing.T) {
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

	mock.ExpectQuery("SELECT account_id, name, provider FROM knowledge_stores WHERE mode = 'managed' AND status = 'ready'").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "provider"}).
			AddRow("acct-1", "my-pg", "postgres").
			AddRow("acct-1", "my-redis", "redis"))

	hb := &Heartbeat{provider: NewProvider(client), db: db, log: log}
	hb.emitKnowledgeCompute(context.Background())

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	for _, ev := range received {
		if ev.Type != "knowledge_compute_usage" {
			t.Errorf("expected type 'knowledge_compute_usage', got %q", ev.Type)
		}
		if ev.Subject != "acct-1" {
			t.Errorf("expected subject 'acct-1', got %q", ev.Subject)
		}
	}

	// Verify per-store data with correct CU calculation
	byStore := map[string]map[string]any{}
	for _, ev := range received {
		data := ev.Data.(map[string]any)
		byStore[data["store_name"].(string)] = data
	}

	pgData := byStore["my-pg"]
	if pgData["provider"] != "postgres" {
		t.Errorf("expected provider='postgres', got %v", pgData["provider"])
	}
	if pgData["cpu"] != "250m" {
		t.Errorf("expected cpu='250m', got %v", pgData["cpu"])
	}
	if pgData["memory"] != "256Mi" {
		t.Errorf("expected memory='256Mi', got %v", pgData["memory"])
	}
	// postgres CU = 0.25, interval = 5min = 1/12 hr, CU-hours = 0.25/12 ≈ 0.02083
	pgCUH := pgData["compute_unit_hours"].(float64)
	expectedPgCUH := 0.25 * (5.0 / 60.0)
	if math.Abs(pgCUH-expectedPgCUH) > 0.001 {
		t.Errorf("postgres CU-hours: expected %f, got %f", expectedPgCUH, pgCUH)
	}

	redisData := byStore["my-redis"]
	// redis CU = 0.05, CU-hours = 0.05/12 ≈ 0.00417
	redisCUH := redisData["compute_unit_hours"].(float64)
	expectedRedisCUH := 0.05 * (5.0 / 60.0)
	if math.Abs(redisCUH-expectedRedisCUH) > 0.001 {
		t.Errorf("redis CU-hours: expected %f, got %f", expectedRedisCUH, redisCUH)
	}
}


func TestHeartbeat_EmitKnowledgeCompute_SkipsNonReady(t *testing.T) {
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

	// Query only returns ready stores — provisioning/error stores are excluded by SQL
	mock.ExpectQuery("SELECT account_id, name, provider FROM knowledge_stores WHERE mode = 'managed' AND status = 'ready'").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "name", "provider"}))

	hb := &Heartbeat{provider: NewProvider(client), db: db, log: log}
	hb.emitKnowledgeCompute(context.Background())

	if len(received) != 0 {
		t.Errorf("expected 0 events for no ready stores, got %d", len(received))
	}
}


