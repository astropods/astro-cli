package promquery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient_EmptyURL(t *testing.T) {
	c := NewClient("")
	if c != nil {
		t.Error("expected nil client for empty URL")
	}
}

func TestNewClient_NonEmpty(t *testing.T) {
	c := NewClient("http://prometheus:9090")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.baseURL != "http://prometheus:9090" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://prometheus:9090")
	}
}

func TestQuery_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query" {
			t.Errorf("path = %q, want /api/v1/query", r.URL.Path)
		}
		if q := r.URL.Query().Get("query"); q != "up" {
			t.Errorf("query param = %q, want %q", q, "up")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{
						"metric": {"account_id": "acct-1", "agent_name": "bot"},
						"value": [1234567890, "42.5"]
					},
					{
						"metric": {"account_id": "acct-2", "agent_name": "helper"},
						"value": [1234567890, "100"]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	samples, err := c.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	if samples[0].Labels["account_id"] != "acct-1" {
		t.Errorf("sample[0] account_id = %q", samples[0].Labels["account_id"])
	}
	if samples[0].Value != 42.5 {
		t.Errorf("sample[0] value = %f, want 42.5", samples[0].Value)
	}
	if samples[1].Labels["agent_name"] != "helper" {
		t.Errorf("sample[1] agent_name = %q", samples[1].Labels["agent_name"])
	}
	if samples[1].Value != 100 {
		t.Errorf("sample[1] value = %f, want 100", samples[1].Value)
	}
}

func TestQuery_EmptyVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	samples, err := c.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(samples) != 0 {
		t.Errorf("expected 0 samples, got %d", len(samples))
	}
}

func TestQuery_PrometheusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Query(context.Background(), "bad{")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestQuery_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestQuery_MalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestQuery_SkipsMalformedValues(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "vector",
				"result": [
					{"metric": {"a": "1"}, "value": [1234567890, "42"]},
					{"metric": {"a": "2"}, "value": [1234567890, "not-a-number"]},
					{"metric": {"a": "3"}, "value": [1234567890]},
					{"metric": {"a": "4"}, "value": [1234567890, 999]}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	samples, err := c.Query(context.Background(), "up")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only the first result should parse; the rest are skipped:
	// "not-a-number" fails ParseFloat, short array is skipped, 999 (non-string) is skipped
	if len(samples) != 1 {
		t.Fatalf("expected 1 valid sample, got %d", len(samples))
	}
	if samples[0].Value != 42 {
		t.Errorf("sample value = %f, want 42", samples[0].Value)
	}
}

func TestQuery_WrongResultType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for non-vector result type")
	}
}
