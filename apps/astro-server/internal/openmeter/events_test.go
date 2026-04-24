package openmeter

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
)

func TestEmitAgentBuild_NilClient(t *testing.T) {
	// Should not panic
	log := logger.New("error", "json")
	EmitAgentBuild(context.Background(), nil, log, "acct-1", "my-agent")
}

func TestEmitAgentBuild_Success(t *testing.T) {
	var received atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		if len(events) != 1 {
			t.Errorf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != "agent_build" {
			t.Errorf("expected type 'agent_register', got %q", events[0].Type)
		}
		if events[0].Subject != "acct-1" {
			t.Errorf("expected subject 'acct-1', got %q", events[0].Subject)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	log := logger.New("error", "json")
	client := NewClient(srv.URL)
	EmitAgentBuild(context.Background(), client, log, "acct-1", "my-agent")

	if received.Load() != 1 {
		t.Errorf("expected 1 API call, got %d", received.Load())
	}
}

func TestEmitActiveKnowledgeStores_NilClient(t *testing.T) {
	log := logger.New("error", "json")
	EmitActiveKnowledgeStores(context.Background(), nil, nil, log, "acct-1")
}

func TestEmitActiveKnowledgeStores_Success(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT COUNT.+ FROM knowledge_stores WHERE account_id").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(3))

	log := logger.New("error", "json")
	client := NewClient(srv.URL)
	EmitActiveKnowledgeStores(context.Background(), client, db, log, "acct-1")

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "active_knowledge_stores" {
		t.Errorf("expected type 'active_knowledge_stores', got %q", received[0].Type)
	}
	if received[0].Subject != "acct-1" {
		t.Errorf("expected subject 'acct-1', got %q", received[0].Subject)
	}
	data := received[0].Data.(map[string]any)
	if count := data["count"].(float64); count != 3 {
		t.Errorf("expected count=3, got %v", count)
	}
}

func TestEmitKnowledgeStorage_NilClient(t *testing.T) {
	log := logger.New("error", "json")
	EmitKnowledgeStorage(context.Background(), nil, nil, log, "acct-1")
}

func TestEmitKnowledgeStorage_Success(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT name, provider, storage FROM knowledge_stores WHERE account_id").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"name", "provider", "storage"}).
			AddRow("my-pg", "postgres", "10Gi").
			AddRow("my-redis", "redis", "1Gi"))

	log := logger.New("error", "json")
	client := NewClient(srv.URL)
	EmitKnowledgeStorage(context.Background(), client, db, log, "acct-1")

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}

	for _, ev := range received {
		if ev.Type != "knowledge_storage_provisioned" {
			t.Errorf("expected type 'knowledge_storage_provisioned', got %q", ev.Type)
		}
		if ev.Subject != "acct-1" {
			t.Errorf("expected subject 'acct-1', got %q", ev.Subject)
		}
	}

	// First event: 10Gi = 10 GB
	data0 := received[0].Data.(map[string]any)
	if gb := data0["storage_gb"].(float64); math.Abs(gb-10) > 0.01 {
		t.Errorf("expected storage_gb=10, got %v", gb)
	}
	if data0["store_name"] != "my-pg" {
		t.Errorf("expected store_name='my-pg', got %v", data0["store_name"])
	}
	if data0["provider"] != "postgres" {
		t.Errorf("expected provider='postgres', got %v", data0["provider"])
	}

	// Second event: 1Gi = 1 GB
	data1 := received[1].Data.(map[string]any)
	if gb := data1["storage_gb"].(float64); math.Abs(gb-1) > 0.01 {
		t.Errorf("expected storage_gb=1, got %v", gb)
	}
}

func TestEmitActiveKnowledgeEndpoints_NilClient(t *testing.T) {
	log := logger.New("error", "json")
	EmitActiveKnowledgeEndpoints(context.Background(), nil, nil, log, "acct-1")
}

func TestEmitActiveKnowledgeEndpoints_Success(t *testing.T) {
	var received []CloudEvent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var events []CloudEvent
		_ = json.NewDecoder(r.Body).Decode(&events)
		received = append(received, events...)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	db, mock, _ := sqlmock.New()
	defer db.Close()
	mock.ExpectQuery("SELECT COUNT.+ FROM knowledge_store_endpoints kse").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))

	log := logger.New("error", "json")
	client := NewClient(srv.URL)
	EmitActiveKnowledgeEndpoints(context.Background(), client, db, log, "acct-1")

	if len(received) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received))
	}
	if received[0].Type != "active_knowledge_endpoints" {
		t.Errorf("expected type 'active_knowledge_endpoints', got %q", received[0].Type)
	}
	data := received[0].Data.(map[string]any)
	if count := data["count"].(float64); count != 2 {
		t.Errorf("expected count=2, got %v", count)
	}
}

func TestKnowledgeCU(t *testing.T) {
	tests := []struct {
		provider string
		wantCU   float64
	}{
		{"postgres", 0.25}, // max(0.25, 0.25/2) = 0.25
		{"redis", 0.05},    // max(0.05, 0.0625/2) = 0.05
		{"qdrant", 0.25},   // max(0.25, 0.5/2) = 0.25
		{"neo4j", 0.5},     // max(0.5, 0.5/2) = 0.5
		{"unknown", 0.1},   // max(0.1, 0.125/2) = 0.1
	}
	for _, tt := range tests {
		got := knowledgeCU(tt.provider)
		if math.Abs(got-tt.wantCU) > 0.001 {
			t.Errorf("knowledgeCU(%q) = %f, want %f", tt.provider, got, tt.wantCU)
		}
	}
}

func TestStorageToGB(t *testing.T) {
	tests := []struct {
		input string
		want  float64
	}{
		{"10Gi", 10},
		{"1Gi", 1},
		{"500Mi", 500.0 / 1024},
		{"256Mi", 0.25},
		{"50Gi", 50},
		{"1Ti", 1024},
	}
	for _, tt := range tests {
		got := storageToGB(tt.input)
		if math.Abs(got-tt.want) > 0.01 {
			t.Errorf("storageToGB(%q) = %f, want %f", tt.input, got, tt.want)
		}
	}
}
