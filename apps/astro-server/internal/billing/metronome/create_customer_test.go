package metronome

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func TestCreateCustomer_ReusesAnExistingCustomerByAlias(t *testing.T) {
	var createCalled bool
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "v1/customers"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"cust_existing","created_at":"2026-01-01T00:00:00Z","custom_fields":{},"customer_config":{}}],"next_page":null}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "v1/customers"):
			createCalled = true
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	id, err := p.CreateCustomer(context.Background(), billing.Account{ID: "acct_1", Name: "Acme"})
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}
	if id != "cust_existing" {
		t.Errorf("id = %q, want the existing customer's ID", id)
	}
	if createCalled {
		t.Error("a new customer was created despite an existing one matching the alias")
	}
}

func TestCreateCustomer_CreatesWithBothAliasesWhenNoneExists(t *testing.T) {
	var gotBody string
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "v1/customers"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[],"next_page":null}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "v1/customers"):
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			gotBody = string(buf)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"cust_new","created_at":"2026-01-01T00:00:00Z","custom_fields":{},"customer_config":{}}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	id, err := p.CreateCustomer(context.Background(), billing.Account{ID: "acct_1", Name: "Acme", BifrostCustomerID: "bifrost_1"})
	if err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}
	if id != "cust_new" {
		t.Errorf("id = %q, want the newly created customer's ID", id)
	}
	if !strings.Contains(gotBody, "acct_1") || !strings.Contains(gotBody, "bifrost_1") {
		t.Errorf("create request body = %s, want both acct_1 and bifrost_1 as ingest aliases", gotBody)
	}
}

func TestCreateCustomer_OmitsBifrostAliasWhenUnset(t *testing.T) {
	var gotBody string
	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "v1/customers"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[],"next_page":null}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "v1/customers"):
			buf := make([]byte, r.ContentLength)
			_, _ = r.Body.Read(buf)
			gotBody = string(buf)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":{"id":"cust_new","created_at":"2026-01-01T00:00:00Z","custom_fields":{},"customer_config":{}}}`))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if _, err := p.CreateCustomer(context.Background(), billing.Account{ID: "acct_1", Name: "Acme"}); err != nil {
		t.Fatalf("CreateCustomer() error = %v", err)
	}
	if strings.Count(gotBody, "acct_1") != 1 {
		t.Errorf("create request body = %s, want exactly one alias (acct_1), no empty second alias", gotBody)
	}
}
