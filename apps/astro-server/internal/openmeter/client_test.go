package openmeter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_NilWhenEmpty(t *testing.T) {
	c := NewClient("")
	if c != nil {
		t.Fatal("expected nil client for empty URL")
	}
}

func TestNewClient_NotNil(t *testing.T) {
	c := NewClient("http://localhost:8888")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
}

func TestCreateCustomer_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/customers" {
			t.Errorf("expected /api/v1/customers, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}

		body, _ := io.ReadAll(r.Body)
		var customer Customer
		if err := json.Unmarshal(body, &customer); err != nil {
			t.Fatalf("failed to unmarshal request body: %v", err)
		}
		if customer.Key != "acct-123" {
			t.Errorf("expected key 'acct-123', got %q", customer.Key)
		}
		if customer.Name != "myorg" {
			t.Errorf("expected name 'myorg', got %q", customer.Name)
		}
		if len(customer.UsageAttribution.SubjectKeys) != 1 || customer.UsageAttribution.SubjectKeys[0] != "acct-123" {
			t.Errorf("expected subject key 'acct-123', got %v", customer.UsageAttribution.SubjectKeys)
		}
		if customer.PrimaryEmail != "user@example.com" {
			t.Errorf("expected email 'user@example.com', got %q", customer.PrimaryEmail)
		}
		if customer.Metadata["type"] != "organization" {
			t.Errorf("expected metadata type 'organization', got %q", customer.Metadata["type"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]string{"id": "om-cust-456"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	id, err := c.CreateCustomer(context.Background(), "acct-123", "myorg", "organization", "user@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "om-cust-456" {
		t.Errorf("expected 'om-cust-456', got %q", id)
	}
}

func TestCreateCustomer_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.CreateCustomer(context.Background(), "acct-123", "myorg", "personal", "")
	if err == nil {
		t.Fatal("expected error on 500 response")
	}
}

func TestIngestEvents_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/events" {
			t.Errorf("expected /api/v1/events, got %s", r.URL.Path)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/cloudevents-batch+json" {
			t.Errorf("expected cloudevents-batch+json, got %s", ct)
		}

		body, _ := io.ReadAll(r.Body)
		var events []CloudEvent
		if err := json.Unmarshal(body, &events); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if len(events) != 1 {
			t.Fatalf("expected 1 event, got %d", len(events))
		}
		if events[0].Type != "agent_register" {
			t.Errorf("expected type 'agent_register', got %q", events[0].Type)
		}
		if events[0].Subject != "acct-123" {
			t.Errorf("expected subject 'acct-123', got %q", events[0].Subject)
		}

		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ev := NewCloudEvent("agent_register", "acct-123", map[string]string{"agent_name": "my-agent"})
	err := c.IngestEvents(context.Background(), []CloudEvent{ev})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIngestEvents_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ev := NewCloudEvent("agent_register", "acct-123", nil)
	err := c.IngestEvents(context.Background(), []CloudEvent{ev})
	if err == nil {
		t.Fatal("expected error on 400 response")
	}
}

func TestGetCustomerAccess_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/customers/acct-123/access" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"entitlements": map[string]any{
				"agent_deployments": map[string]any{
					"hasAccess":                 true,
					"balance":                   10.0,
					"usage":                     5.0,
					"overage":                   0.0,
					"totalAvailableGrantAmount": 10.0,
				},
			},
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	access, err := c.GetCustomerAccess(context.Background(), "acct-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	ent, ok := access.Entitlements["agent_deployments"]
	if !ok {
		t.Fatal("expected agent_deployments entitlement")
	}
	if !ent.HasAccess {
		t.Error("expected hasAccess=true")
	}
	if ent.TotalAvailableGrantAmount == nil || *ent.TotalAvailableGrantAmount != 10.0 {
		t.Errorf("expected totalAvailableGrantAmount=10, got %v", ent.TotalAvailableGrantAmount)
	}
}

func TestGetCustomerAccess_Error(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"not found"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.GetCustomerAccess(context.Background(), "acct-123")
	if err == nil {
		t.Fatal("expected error on 404 response")
	}
}

func TestNewCloudEvent_Fields(t *testing.T) {
	ev := NewCloudEvent("agent_deploy", "acct-999", map[string]string{"agent_name": "bot"})
	if ev.Source != "astro-server" {
		t.Errorf("expected source 'astro-server', got %q", ev.Source)
	}
	if ev.SpecVersion != "1.0" {
		t.Errorf("expected specversion '1.0', got %q", ev.SpecVersion)
	}
	if ev.Type != "agent_deploy" {
		t.Errorf("expected type 'agent_deploy', got %q", ev.Type)
	}
	if ev.Subject != "acct-999" {
		t.Errorf("expected subject 'acct-999', got %q", ev.Subject)
	}
	if ev.ID == "" {
		t.Error("expected non-empty ID")
	}
	if ev.Time == "" {
		t.Error("expected non-empty Time")
	}
}
