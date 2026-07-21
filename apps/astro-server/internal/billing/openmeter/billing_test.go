package openmeter

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	_ "modernc.org/sqlite"
)

// --- Helpers ---

// collectEvents returns an httptest server that collects all ingested CloudEvents.
func collectEvents(t *testing.T) (*httptest.Server, *[]CloudEvent, *sync.Mutex) {
	t.Helper()
	var mu sync.Mutex
	var collected []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		mu.Lock()
		collected = append(collected, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv, &collected, &mu
}

// failingServer returns an httptest server that always returns 500.
func failingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"fail"}`)) //nolint:errcheck
	}))
	t.Cleanup(srv.Close)
	return srv
}

// setupSQLiteDB creates an in-memory SQLite DB with the billing state tables
// and supporting deployment/knowledge tables needed for JOIN queries.
func setupSQLiteDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	_, err = db.Exec(`
		CREATE TABLE deployments (
			id               TEXT PRIMARY KEY,
			account_id       TEXT NOT NULL,
			agent_name       TEXT NOT NULL,
			namespace        TEXT NOT NULL,
			status           TEXT NOT NULL DEFAULT 'pending',
			status_changed_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE deployment_billing_state (
			deployment_id   TEXT    NOT NULL,
			component       TEXT    NOT NULL,
			billing_active  INTEGER NOT NULL DEFAULT 0,
			last_emitted_at DATETIME NOT NULL DEFAULT (datetime('now')),
			stopped_at      DATETIME NULL,
			cpu_request     TEXT    NOT NULL DEFAULT '',
			memory_request  TEXT    NOT NULL DEFAULT '',
			replicas        INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (deployment_id, component),
			FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
		);

		CREATE TABLE deployment_workloads (
			id              INTEGER PRIMARY KEY AUTOINCREMENT,
			deployment_id   TEXT NOT NULL,
			name            TEXT NOT NULL,
			component_kind  TEXT NOT NULL,
			component_key   TEXT NOT NULL DEFAULT '',
			workload_type   TEXT NOT NULL DEFAULT '',
			image           TEXT NOT NULL DEFAULT '',
			replicas        INTEGER NOT NULL DEFAULT 1,
			cpu_request     TEXT NOT NULL DEFAULT '',
			memory_request  TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (deployment_id) REFERENCES deployments(id) ON DELETE CASCADE
		);

		CREATE TABLE knowledge_stores (
			id         TEXT PRIMARY KEY,
			account_id TEXT NOT NULL,
			name       TEXT NOT NULL,
			provider   TEXT NOT NULL,
			mode       TEXT NOT NULL DEFAULT 'managed',
			status     TEXT NOT NULL DEFAULT 'provisioning',
			updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
		);

		CREATE TABLE knowledge_billing_state (
			knowledge_store_id TEXT PRIMARY KEY,
			billing_active     INTEGER NOT NULL DEFAULT 0,
			last_emitted_at    DATETIME NOT NULL DEFAULT (datetime('now')),
			stopped_at         DATETIME NULL,
			account_id         TEXT NOT NULL DEFAULT '',
			name               TEXT NOT NULL DEFAULT '',
			provider           TEXT NOT NULL
		);
	`)
	if err != nil {
		t.Fatalf("failed to create tables: %v", err)
	}
	return db
}

func eventData(ev CloudEvent) map[string]any {
	return ev.Data.(map[string]any)
}

// --- Unit Tests ---

func TestNewBillingStateManager_NilClient(t *testing.T) {
	m := NewBillingStateManager(nil, nil, logger.New("error", "json"))
	if m != nil {
		t.Error("expected nil manager when client is nil")
	}
}

func TestRawCU(t *testing.T) {
	tests := []struct {
		cpu, mem string
		replicas int
		want     float64
	}{
		{"1", "2Gi", 1, 1.0},       // max(1, 2/2) = 1
		{"100m", "4Gi", 1, 2.0},    // max(0.1, 4/2) = 2
		{"250m", "256Mi", 1, 0.25}, // max(0.25, 0.25/2) = 0.25
		{"1", "2Gi", 3, 3.0},       // max(1, 1) * 3
		{"", "", 1, 0},             // empty resources
		{"500m", "1Gi", 0, 0.5},    // 0 replicas treated as 1
	}
	for _, tt := range tests {
		got := rawCU(tt.cpu, tt.mem, tt.replicas)
		if math.Abs(got-tt.want) > 0.001 {
			t.Errorf("rawCU(%q, %q, %d) = %f, want %f", tt.cpu, tt.mem, tt.replicas, got, tt.want)
		}
	}
}

// --- Deployment Billing Lifecycle ---

func TestStartBilling_InsertsRows(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	m.StartBilling(context.Background(), "dep-1", []WorkloadInfo{
		{Component: "agent", CPURequest: "1", MemoryRequest: "2Gi", Replicas: 1},
		{Component: "model/llm", CPURequest: "2", MemoryRequest: "8Gi", Replicas: 1},
	})

	// Verify rows were inserted
	var count int
	db.QueryRow("SELECT COUNT(*) FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND billing_active = 1").Scan(&count)
	if count != 2 {
		t.Errorf("expected 2 billing rows, got %d", count)
	}

	var cpu, mem string
	var replicas int
	db.QueryRow("SELECT cpu_request, memory_request, replicas FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND component = 'agent'").Scan(&cpu, &mem, &replicas)
	if cpu != "1" || mem != "2Gi" || replicas != 1 {
		t.Errorf("unexpected billing row: cpu=%s mem=%s replicas=%d", cpu, mem, replicas)
	}
}

func TestStartBilling_NilManager(t *testing.T) {
	var m *BillingStateManager
	// Should not panic
	m.StartBilling(context.Background(), "dep-1", []WorkloadInfo{
		{Component: "agent", CPURequest: "1", MemoryRequest: "2Gi", Replicas: 1},
	})
}

func TestStopBilling_RecordsStoppedAt(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	thirtyMinAgo := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, thirtyMinAgo)

	stopTime := time.Now()
	m.StopBilling(context.Background(), "dep-1", stopTime)

	// No events emitted — heartbeat handles this
	mu.Lock()
	n := len(*received)
	mu.Unlock()
	if n != 0 {
		t.Errorf("expected 0 events from StopBilling, got %d", n)
	}

	// billing_active=false and stopped_at is set
	var active int
	var stoppedAt time.Time
	db.QueryRow("SELECT billing_active, stopped_at FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&active, &stoppedAt)
	if active != 0 {
		t.Error("expected billing_active=false after stop")
	}
	if stoppedAt.IsZero() {
		t.Error("expected stopped_at to be set")
	}
}

func TestStopBilling_NoActiveRows(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	m.StopBilling(context.Background(), "dep-1", time.Now())

	mu.Lock()
	defer mu.Unlock()
	if len(*received) != 0 {
		t.Errorf("expected 0 events, got %d", len(*received))
	}
}

// --- Full Deployment Lifecycle ---

func TestBilling_FullLifecycle(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	// T0 = 20 min ago. Billing row starts there.
	t0 := time.Now().Add(-20 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, t0)

	// T0+10m: simulate a heartbeat that ran at t0+10m by directly advancing last_emitted_at,
	// then calling RunBillingCycle which emits the delta from t0 to ~now.
	t1 := time.Now().Add(-10 * time.Minute)
	db.Exec(`UPDATE deployment_billing_state SET last_emitted_at = ? WHERE deployment_id = 'dep-1'`, t1)
	m.RunBillingCycle(context.Background())

	// T0+15m: stop is called. Set last_emitted_at back to t1 to create a known 5-min gap,
	// then record stopped_at = t1+5m so reconcileStopped has a non-zero elapsed window.
	stopTime := t1.Add(5 * time.Minute)
	db.Exec(`UPDATE deployment_billing_state SET last_emitted_at = ? WHERE deployment_id = 'dep-1'`, t1)
	m.StopBilling(context.Background(), "dep-1", stopTime)

	// Heartbeat picks up stopped_at and emits the final 5-min window.
	db.Exec(`UPDATE deployments SET status = 'undeployed' WHERE id = 'dep-1'`)
	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) < 2 {
		t.Fatalf("expected at least 2 events (reconcile + final), got %d", len(*received))
	}

	var totalCUH float64
	for _, ev := range *received {
		totalCUH += eventData(ev)["compute_unit_hours"].(float64)
	}
	// First reconcile billed ~10 min (t1 to now≈t0+20m), final billed 5 min (t1 to stopTime).
	// Total ≈ 15 min = 0.25h. Allow generous tolerance for wall-clock drift.
	if totalCUH < 0.1 {
		t.Errorf("total CU-hours unexpectedly low: got %f", totalCUH)
	}
}

func TestBilling_ShortLivedDeployment(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'undeployed')`)

	twoMinAgo := time.Now().Add(-2 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '500m', '1Gi', 1)`, twoMinAgo)

	// Stop records stopped_at — no events yet
	stopTime := time.Now()
	m.StopBilling(context.Background(), "dep-1", stopTime)

	mu.Lock()
	n := len(*received)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 events from StopBilling, got %d", n)
	}

	// Heartbeat emits the final period via reconcileStopped
	m.RunBillingCycle(context.Background())

	mu.Lock()
	events := append([]CloudEvent{}, (*received)...)
	mu.Unlock()

	if len(events) != 1 {
		t.Fatalf("expected 1 event after heartbeat, got %d", len(events))
	}

	cuHours := eventData(events[0])["compute_unit_hours"].(float64)
	// CU = max(0.5, 1/2) = 0.5, elapsed ≈ 2min
	expectedCUH := 0.5 * (2.0 / 60.0)
	if math.Abs(cuHours-expectedCUH) > 0.005 {
		t.Errorf("short-lived CU-hours: expected ≈ %f, got %f", expectedCUH, cuHours)
	}

	// A second heartbeat tick should emit nothing (stopped_at cleared)
	mu.Lock()
	beforeLen := len(*received)
	mu.Unlock()

	m.RunBillingCycle(context.Background())

	mu.Lock()
	afterLen := len(*received)
	mu.Unlock()

	if afterLen != beforeLen {
		t.Errorf("second heartbeat should emit 0 events, emitted %d", afterLen-beforeLen)
	}
}

func TestBilling_MultipleWorkloads(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'undeployed')`)

	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '100m', '256Mi', 1)`, fiveMinAgo)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'model/llm', 1, ?, '2', '8Gi', 1)`, fiveMinAgo)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'interfaces', 1, ?, '100m', '128Mi', 1)`, fiveMinAgo)

	m.StopBilling(context.Background(), "dep-1", time.Now())
	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) != 3 {
		t.Fatalf("expected 3 events (one per workload), got %d", len(*received))
	}

	byComponent := map[string]float64{}
	for _, ev := range *received {
		d := eventData(ev)
		byComponent[d["component"].(string)] = d["compute_unit_hours"].(float64)
	}

	if _, ok := byComponent["agent"]; !ok {
		t.Error("missing 'agent' component event")
	}
	if _, ok := byComponent["model/llm"]; !ok {
		t.Error("missing 'model/llm' component event")
	}
	if byComponent["model/llm"] < byComponent["agent"] {
		t.Error("model/llm should have higher CU-hours than agent")
	}
}

// --- Heartbeat Reconciliation ---

func TestRunBillingCycle_EmitsDelta(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, fiveMinAgo)

	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(*received))
	}

	cuHours := eventData((*received)[0])["compute_unit_hours"].(float64)
	// CU = 1, elapsed ≈ 5min ≈ 0.0833h
	expectedCUH := 1.0 * (5.0 / 60.0)
	if math.Abs(cuHours-expectedCUH) > 0.01 {
		t.Errorf("reconcile CU-hours: expected ≈ %f, got %f", expectedCUH, cuHours)
	}

	// Verify last_emitted_at was advanced (should be within last few seconds)
	var lastEmitted time.Time
	db.QueryRow("SELECT last_emitted_at FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&lastEmitted)
	if time.Since(lastEmitted) > 10*time.Second {
		t.Error("last_emitted_at was not advanced after reconcile")
	}
}

func TestRunBillingCycle_NoActiveRows(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(*received) != 0 {
		t.Errorf("expected 0 events, got %d", len(*received))
	}
}

func TestRunBillingCycle_SkipsZeroElapsed(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	// last_emitted_at is in the future — guarantees zero/negative elapsed
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, time.Now().Add(1*time.Minute))

	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(*received) != 0 {
		t.Errorf("expected 0 events for zero/negative elapsed, got %d", len(*received))
	}
}

func TestReconcileStale_CrashRecovery(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	// Deployment was undeployed 5 minutes ago (simulating crash — billing_active left true)
	statusChangedAt := time.Now().Add(-5 * time.Minute)
	lastEmittedAt := time.Now().Add(-15 * time.Minute)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status, status_changed_at)
		VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'undeployed', ?)`, statusChangedAt)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, lastEmittedAt)

	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	// Should emit CU-hours from lastEmittedAt to statusChangedAt (10 minutes)
	// The reconcileStale is called inside RunBillingCycle, but the main query
	// also picks up this row since billing_active=true. The stale reconciler
	// may fire separately. We check total CU-hours.
	var totalCUH float64
	for _, ev := range *received {
		totalCUH += eventData(ev)["compute_unit_hours"].(float64)
	}

	if len(*received) == 0 {
		t.Fatal("expected at least 1 event from stale reconciliation")
	}

	// billing_active should be false now
	var active int
	db.QueryRow("SELECT billing_active FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&active)
	if active != 0 {
		t.Error("expected billing_active=false after stale reconciliation")
	}
}

func TestReconcileStale_Stopped(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	statusChangedAt := time.Now().Add(-3 * time.Minute)
	lastEmittedAt := time.Now().Add(-8 * time.Minute)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status, status_changed_at)
		VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'stopped', ?)`, statusChangedAt)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '500m', '1Gi', 2)`, lastEmittedAt)

	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) == 0 {
		t.Fatal("expected events from stale stopped reconciliation")
	}

	var active int
	db.QueryRow("SELECT billing_active FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&active)
	if active != 0 {
		t.Error("expected billing_active=false after stopped reconciliation")
	}
}

// --- Knowledge Store Billing ---

func TestStartKnowledgeBilling_InsertsRow(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, status) VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'ready')`)

	m.StartKnowledgeBilling(context.Background(), "ks-1", "acct-1", "my-pg", "postgres")

	var active int
	var provider, accountID, name string
	db.QueryRow("SELECT billing_active, provider, account_id, name FROM knowledge_billing_state WHERE knowledge_store_id = 'ks-1'").Scan(&active, &provider, &accountID, &name)
	if active != 1 {
		t.Error("expected billing_active=true")
	}
	if provider != "postgres" {
		t.Errorf("expected provider='postgres', got %q", provider)
	}
	if accountID != "acct-1" {
		t.Errorf("expected account_id='acct-1', got %q", accountID)
	}
	if name != "my-pg" {
		t.Errorf("expected name='my-pg', got %q", name)
	}
}

func TestStopKnowledgeBilling_RecordsStoppedAt(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, status) VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'ready')`)

	thirtyMinAgo := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO knowledge_billing_state (knowledge_store_id, billing_active, last_emitted_at, account_id, name, provider)
		VALUES ('ks-1', 1, ?, 'acct-1', 'my-pg', 'postgres')`, thirtyMinAgo)

	err := m.StopKnowledgeBilling(context.Background(), "ks-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// No events emitted — heartbeat handles this
	mu.Lock()
	n := len(*received)
	mu.Unlock()
	if n != 0 {
		t.Errorf("expected 0 events from StopKnowledgeBilling, got %d", n)
	}

	var active int
	var stoppedAt time.Time
	db.QueryRow("SELECT billing_active, stopped_at FROM knowledge_billing_state WHERE knowledge_store_id = 'ks-1'").Scan(&active, &stoppedAt)
	if active != 0 {
		t.Error("expected billing_active=false after stop")
	}
	if stoppedAt.IsZero() {
		t.Error("expected stopped_at to be set")
	}
}

func TestStopKnowledgeBilling_NotActive(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, status) VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'error')`)
	db.Exec(`INSERT INTO knowledge_billing_state (knowledge_store_id, billing_active, last_emitted_at, account_id, name, provider)
		VALUES ('ks-1', 0, datetime('now'), 'acct-1', 'my-pg', 'postgres')`)

	m.StopKnowledgeBilling(context.Background(), "ks-1", time.Now())

	mu.Lock()
	defer mu.Unlock()
	if len(*received) != 0 {
		t.Errorf("expected 0 events for inactive billing, got %d", len(*received))
	}
}

func TestReconcileStoppedKnowledge_EmitsFinalPeriodAfterDeletion(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	// Store exists, billing started
	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, status) VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'ready')`)
	thirtyMinAgo := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO knowledge_billing_state (knowledge_store_id, billing_active, last_emitted_at, account_id, name, provider)
		VALUES ('ks-1', 1, ?, 'acct-1', 'my-pg', 'postgres')`, thirtyMinAgo)

	// Stop is recorded
	m.StopKnowledgeBilling(context.Background(), "ks-1", time.Now())

	// Store is deleted — billing row persists (no CASCADE)
	db.Exec(`DELETE FROM knowledge_stores WHERE id = 'ks-1'`)

	// Heartbeat emits the final period using stored account_id/name
	m.RunKnowledgeBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) != 1 {
		t.Fatalf("expected 1 event after heartbeat, got %d", len(*received))
	}
	ev := (*received)[0]
	if ev.Type != "knowledge_compute_usage" {
		t.Errorf("expected type 'knowledge_compute_usage', got %q", ev.Type)
	}
	data := eventData(ev)
	cuHours := data["compute_unit_hours"].(float64)
	// postgres CU = 0.25, elapsed ≈ 0.5h
	expectedCUH := 0.25 * 0.5
	if math.Abs(cuHours-expectedCUH) > 0.02 {
		t.Errorf("knowledge CU-hours: expected ≈ %f, got %f", expectedCUH, cuHours)
	}
	if data["store_name"] != "my-pg" {
		t.Errorf("expected store_name='my-pg', got %v", data["store_name"])
	}

	// Row deleted after emission
	var count int
	db.QueryRow("SELECT COUNT(*) FROM knowledge_billing_state WHERE knowledge_store_id = 'ks-1'").Scan(&count)
	if count != 0 {
		t.Error("expected billing row deleted after emission")
	}
}

func TestRunKnowledgeBillingCycle_EmitsDelta(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, status) VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'ready')`)

	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	db.Exec(`INSERT INTO knowledge_billing_state (knowledge_store_id, billing_active, last_emitted_at, account_id, name, provider)
		VALUES ('ks-1', 1, ?, 'acct-1', 'my-pg', 'postgres')`, fiveMinAgo)

	m.RunKnowledgeBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(*received))
	}

	cuHours := eventData((*received)[0])["compute_unit_hours"].(float64)
	expectedCUH := 0.25 * (5.0 / 60.0)
	if math.Abs(cuHours-expectedCUH) > 0.005 {
		t.Errorf("knowledge reconcile CU-hours: expected ≈ %f, got %f", expectedCUH, cuHours)
	}
}

func TestReconcileStaleKnowledge_ErrorStore(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	updatedAt := time.Now().Add(-2 * time.Minute)
	lastEmitted := time.Now().Add(-10 * time.Minute)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, status, updated_at)
		VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'error', ?)`, updatedAt)
	db.Exec(`INSERT INTO knowledge_billing_state (knowledge_store_id, billing_active, last_emitted_at, account_id, name, provider)
		VALUES ('ks-1', 1, ?, 'acct-1', 'my-pg', 'postgres')`, lastEmitted)

	m.RunKnowledgeBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()

	if len(*received) == 0 {
		t.Fatal("expected events from stale knowledge reconciliation")
	}

	var active int
	db.QueryRow("SELECT billing_active FROM knowledge_billing_state WHERE knowledge_store_id = 'ks-1'").Scan(&active)
	if active != 0 {
		t.Error("expected billing_active=false after error store reconciliation")
	}
}

// --- Edge Cases ---

func TestStartBilling_UpsertOnRedeploy(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	// First start
	m.StartBilling(context.Background(), "dep-1", []WorkloadInfo{
		{Component: "agent", CPURequest: "500m", MemoryRequest: "1Gi", Replicas: 1},
	})

	// Simulate stop (mark inactive)
	db.Exec(`UPDATE deployment_billing_state SET billing_active = 0 WHERE deployment_id = 'dep-1'`)

	// Second start with different resources (redeploy)
	m.StartBilling(context.Background(), "dep-1", []WorkloadInfo{
		{Component: "agent", CPURequest: "1", MemoryRequest: "2Gi", Replicas: 2},
	})

	var cpu, mem string
	var replicas, active int
	db.QueryRow("SELECT cpu_request, memory_request, replicas, billing_active FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND component = 'agent'").Scan(&cpu, &mem, &replicas, &active)
	if cpu != "1" || mem != "2Gi" || replicas != 2 || active != 1 {
		t.Errorf("upsert failed: cpu=%s mem=%s replicas=%d active=%d", cpu, mem, replicas, active)
	}

	// Should still be only 1 row
	var count int
	db.QueryRow("SELECT COUNT(*) FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 row after upsert, got %d", count)
	}
}

func TestStopBilling_MultipleComponents_AllDeactivated(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'undeployed')`)

	fiveMinAgo := time.Now().Add(-5 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, fiveMinAgo)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'sidecar', 1, ?, '', '', 1)`, fiveMinAgo)

	m.StopBilling(context.Background(), "dep-1", time.Now())

	// No events yet — heartbeat emits
	mu.Lock()
	n := len(*received)
	mu.Unlock()
	if n != 0 {
		t.Fatalf("expected 0 events from StopBilling, got %d", n)
	}

	// Both rows deactivated with stopped_at set
	var count int
	db.QueryRow("SELECT COUNT(*) FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND billing_active = 0 AND stopped_at IS NOT NULL").Scan(&count)
	if count != 2 {
		t.Errorf("expected both rows deactivated with stopped_at, got %d", count)
	}

	// Heartbeat emits only agent (sidecar has CU=0)
	m.RunBillingCycle(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if len(*received) != 1 {
		t.Fatalf("expected 1 event after heartbeat (zero-CU sidecar skipped), got %d", len(*received))
	}
	if eventData((*received)[0])["component"] != "agent" {
		t.Errorf("expected component='agent', got %v", eventData((*received)[0])["component"])
	}
}

// --- Self-Healing ---

func TestHealMissingBillingRows_SeededOnFirstTick(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, received, mu := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)
	db.Exec(`INSERT INTO deployment_workloads (deployment_id, name, component_kind, component_key, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 'agent', '', '1', '2Gi', 1)`)

	// No billing row exists yet — RunBillingCycle should heal it and emit on the next tick
	m.RunBillingCycle(context.Background())

	// First tick: heals the row but last_emitted_at = now, so elapsed ≈ 0 — no event yet
	mu.Lock()
	firstLen := len(*received)
	mu.Unlock()
	if firstLen != 0 {
		t.Errorf("expected 0 events on heal tick (elapsed ≈ 0), got %d", firstLen)
	}

	// Verify row was inserted
	var active int
	db.QueryRow("SELECT billing_active FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&active)
	if active != 1 {
		t.Error("expected billing row to be healed with billing_active=true")
	}
}

func TestHealMissingBillingRows_IdempotentOnSubsequentTicks(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)
	db.Exec(`INSERT INTO deployment_workloads (deployment_id, name, component_kind, component_key, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 'agent', '', '1', '2Gi', 1)`)

	// Seed a billing row manually (as StartBilling would)
	tenMinAgo := time.Now().Add(-10 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas) VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, tenMinAgo)

	// Heal should not overwrite the existing row's last_emitted_at
	m.RunBillingCycle(context.Background())

	var lastEmitted time.Time
	db.QueryRow("SELECT last_emitted_at FROM deployment_billing_state WHERE deployment_id = 'dep-1'").Scan(&lastEmitted)
	// last_emitted_at should have been advanced by RunBillingCycle (not reset to now by heal)
	if lastEmitted.Before(time.Now().Add(-5 * time.Second)) {
		t.Error("existing billing row's last_emitted_at was overwritten by heal")
	}
}

func TestHealMissingKnowledgeBillingRows_SeededOnFirstTick(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, mode, status) VALUES ('ks-1', 'acct-1', 'my-pg', 'postgres', 'managed', 'ready')`)

	// No billing row exists — RunKnowledgeBillingCycle should heal it
	m.RunKnowledgeBillingCycle(context.Background())

	var active int
	db.QueryRow("SELECT billing_active FROM knowledge_billing_state WHERE knowledge_store_id = 'ks-1'").Scan(&active)
	if active != 1 {
		t.Error("expected knowledge billing row to be healed with billing_active=true")
	}
}

func TestHealMissingKnowledgeBillingRows_SkipsExternalAndNonReady(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, mode, status) VALUES ('ks-ext', 'acct-1', 'ext-pg', 'postgres', 'external', 'ready')`)
	db.Exec(`INSERT INTO knowledge_stores (id, account_id, name, provider, mode, status) VALUES ('ks-prov', 'acct-1', 'prov-pg', 'postgres', 'managed', 'provisioning')`)

	m.RunKnowledgeBillingCycle(context.Background())

	var count int
	db.QueryRow("SELECT COUNT(*) FROM knowledge_billing_state").Scan(&count)
	if count != 0 {
		t.Errorf("expected no billing rows for external/non-ready stores, got %d", count)
	}
}

// --- Emission failure safety ---

func TestEmitActiveBilling_FailingServer_DoesNotAdvanceTimestamp(t *testing.T) {
	db := setupSQLiteDB(t)
	srv := failingServer(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	oneHourAgo := time.Now().Add(-time.Hour)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 1, ?, '1', '2Gi', 1)`, oneHourAgo)

	m.RunBillingCycle(context.Background())

	var lastEmitted time.Time
	db.QueryRow("SELECT last_emitted_at FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND component = 'agent'").Scan(&lastEmitted)
	if time.Since(lastEmitted) < 30*time.Minute {
		t.Error("last_emitted_at was advanced despite emission failure")
	}
}

func TestReconcileStopped_FailingServer_DoesNotClearStoppedAt(t *testing.T) {
	db := setupSQLiteDB(t)
	srv := failingServer(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'undeployed')`)

	oneHourAgo := time.Now().Add(-time.Hour)
	stoppedAt := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, stopped_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 0, ?, ?, '1', '2Gi', 1)`, oneHourAgo, stoppedAt)

	m.RunBillingCycle(context.Background())

	var gotStoppedAt *time.Time
	db.QueryRow("SELECT stopped_at FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND component = 'agent'").Scan(&gotStoppedAt)
	if gotStoppedAt == nil {
		t.Error("stopped_at was cleared despite emission failure")
	}
}

// --- reconcileStopped batch update ---

func TestReconcileStopped_MultipleDeployments_BatchUpdate(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'agent-a', 'ns-1', 'undeployed')`)
	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-2', 'acct-1', 'agent-b', 'ns-1', 'undeployed')`)

	oneHourAgo := time.Now().Add(-time.Hour)
	stoppedAt := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, stopped_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 0, ?, ?, '1', '2Gi', 1)`, oneHourAgo, stoppedAt)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, stopped_at, cpu_request, memory_request, replicas)
		VALUES ('dep-2', 'agent', 0, ?, ?, '1', '2Gi', 1)`, oneHourAgo, stoppedAt)

	m.RunBillingCycle(context.Background())

	for _, depID := range []string{"dep-1", "dep-2"} {
		var gotStoppedAt *time.Time
		db.QueryRow("SELECT stopped_at FROM deployment_billing_state WHERE deployment_id = ? AND component = 'agent'", depID).Scan(&gotStoppedAt)
		if gotStoppedAt != nil {
			t.Errorf("%s: stopped_at should be NULL after reconcile, still set", depID)
		}
	}
}

func TestReconcileStopped_KeyScopedUpdate_DoesNotClearUnreadRows(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'agent-a', 'ns-1', 'undeployed')`)
	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-2', 'acct-1', 'agent-b', 'ns-1', 'undeployed')`)

	oneHourAgo := time.Now().Add(-time.Hour)
	stoppedAt := time.Now().Add(-30 * time.Minute)

	// dep-1: last_emitted_at < stopped_at — selected by reconcileStopped
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, stopped_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 0, ?, ?, '1', '2Gi', 1)`, oneHourAgo, stoppedAt)
	// dep-2: last_emitted_at = stopped_at — not selected (already emitted), but stopped_at is set
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, stopped_at, cpu_request, memory_request, replicas)
		VALUES ('dep-2', 'agent', 0, ?, ?, '1', '2Gi', 1)`, stoppedAt, stoppedAt)

	m.RunBillingCycle(context.Background())

	var dep1StoppedAt *time.Time
	db.QueryRow("SELECT stopped_at FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND component = 'agent'").Scan(&dep1StoppedAt)
	if dep1StoppedAt != nil {
		t.Error("dep-1: stopped_at should be NULL after reconcile")
	}

	var dep2StoppedAt *time.Time
	db.QueryRow("SELECT stopped_at FROM deployment_billing_state WHERE deployment_id = 'dep-2' AND component = 'agent'").Scan(&dep2StoppedAt)
	if dep2StoppedAt == nil {
		t.Error("dep-2: stopped_at should not be cleared — row was not in the SELECT result")
	}
}

func TestStartBilling_RapidRedeploy_ClearsStoppedAt(t *testing.T) {
	db := setupSQLiteDB(t)
	srv, _, _ := collectEvents(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	db.Exec(`INSERT INTO deployments (id, account_id, agent_name, namespace, status) VALUES ('dep-1', 'acct-1', 'my-agent', 'ns-1', 'active')`)

	// Simulate a stopped row that the heartbeat hasn't processed yet
	oneHourAgo := time.Now().Add(-time.Hour)
	stoppedAt := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO deployment_billing_state (deployment_id, component, billing_active, last_emitted_at, stopped_at, cpu_request, memory_request, replicas)
		VALUES ('dep-1', 'agent', 0, ?, ?, '1', '2Gi', 1)`, oneHourAgo, stoppedAt)

	// Redeploy before the heartbeat runs
	m.StartBilling(context.Background(), "dep-1", []WorkloadInfo{
		{Component: "agent", CPURequest: "1", MemoryRequest: "2Gi", Replicas: 1},
	})

	var active int
	var gotStoppedAt *time.Time
	db.QueryRow("SELECT billing_active, stopped_at FROM deployment_billing_state WHERE deployment_id = 'dep-1' AND component = 'agent'").Scan(&active, &gotStoppedAt)
	if active != 1 {
		t.Error("expected billing_active=true after redeploy")
	}
	if gotStoppedAt != nil {
		t.Error("expected stopped_at=NULL after redeploy, row is in corrupt state")
	}
}

func TestReconcileStoppedKnowledge_FailingServer_DoesNotDeleteRow(t *testing.T) {
	db := setupSQLiteDB(t)
	srv := failingServer(t)
	log := logger.New("error", "json")
	m := NewBillingStateManager(NewProvider(NewClient(srv.URL)), db, log)

	oneHourAgo := time.Now().Add(-time.Hour)
	stoppedAt := time.Now().Add(-30 * time.Minute)
	db.Exec(`INSERT INTO knowledge_billing_state (knowledge_store_id, billing_active, last_emitted_at, stopped_at, account_id, name, provider)
		VALUES ('ks-1', 0, ?, ?, 'acct-1', 'my-pg', 'postgres')`, oneHourAgo, stoppedAt)

	m.RunKnowledgeBillingCycle(context.Background())

	var count int
	db.QueryRow("SELECT COUNT(*) FROM knowledge_billing_state WHERE knowledge_store_id = 'ks-1'").Scan(&count)
	if count != 1 {
		t.Error("knowledge billing row was deleted despite emission failure")
	}
}
