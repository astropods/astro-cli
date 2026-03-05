package openmeter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/postman/astro/apps/astro-server/internal/logger"
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
