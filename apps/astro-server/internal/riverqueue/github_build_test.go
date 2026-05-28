package riverqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
)

func TestNewGitHubBuildWorker_WiresOMClientAndDB(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	omClient := openmeter.NewClient("http://localhost:9999")
	w := NewGitHubBuildWorker(nil, nil, nil, nil, nil, logger.New("error", "json"), omClient, db, nil, nil)

	if w.omClient != omClient {
		t.Error("omClient not stored on worker")
	}
	if w.db != db {
		t.Error("db not stored on worker")
	}
}

func TestNewGitHubBuildWorker_NilOMClient(t *testing.T) {
	db, _, _ := sqlmock.New()
	defer db.Close()

	w := NewGitHubBuildWorker(nil, nil, nil, nil, nil, logger.New("error", "json"), nil, db, nil, nil)
	if w.omClient != nil {
		t.Error("expected nil omClient")
	}
}

// TestEmitFunctions_NilClientIsNoOp verifies that the emit functions used by
// GitHubBuildWorker are nil-safe. This does not test Work() end-to-end; it
// tests the nil-guard contract that Work() relies on to skip emitting when
// omClient is not configured.
func TestEmitFunctions_NilClientIsNoOp(t *testing.T) {
	var called atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, _, _ := sqlmock.New()
	defer db.Close()

	log := logger.New("error", "json")
	w := NewGitHubBuildWorker(nil, nil, nil, nil, nil, log, nil, db, nil, nil)

	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); openmeter.EmitAgentBuild(ctx, w.omClient, w.log, "acct-1", "my-agent") }()
	go func() { defer wg.Done(); openmeter.EmitActiveAgents(ctx, w.omClient, w.db, w.log, "acct-1") }()
	wg.Wait()

	if called.Load() != 0 {
		t.Errorf("expected no HTTP calls with nil omClient, got %d", called.Load())
	}
}

// TestEmitFunctions_SendsBothEventsWithCorrectSubject verifies the positive path:
// when omClient is configured, EmitAgentBuild and EmitActiveAgents each POST a
// CloudEvent to OpenMeter with the correct type and subject (account ID).
func TestEmitFunctions_SendsBothEventsWithCorrectSubject(t *testing.T) {
	type cloudEvent struct {
		Type    string `json:"type"`
		Subject string `json:"subject"`
	}

	var mu sync.Mutex
	var received []cloudEvent

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/events" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var events []cloudEvent
		if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		received = append(received, events...)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM agents`).
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	omClient := openmeter.NewClient(srv.URL)
	log := logger.New("error", "json")
	w := NewGitHubBuildWorker(nil, nil, nil, nil, nil, log, omClient, db, nil, nil)

	ctx := context.Background()
	openmeter.EmitAgentBuild(ctx, w.omClient, w.log, "acct-1", "my-agent")
	openmeter.EmitActiveAgents(ctx, w.omClient, w.db, w.log, "acct-1")

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	types := make(map[string]bool, 2)
	for _, ev := range received {
		if ev.Subject != "acct-1" {
			t.Errorf("event %q: expected subject %q, got %q", ev.Type, "acct-1", ev.Subject)
		}
		types[ev.Type] = true
	}
	if !types["agent_build"] {
		t.Error("agent_build event not received")
	}
	if !types["active_agents"] {
		t.Error("active_agents event not received")
	}
}
