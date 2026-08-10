package promquery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_EmptyURL(t *testing.T) {
	c := NewClient("", "")
	if c != nil {
		t.Error("expected nil client for empty URL")
	}
}

func TestNewClient_NonEmpty(t *testing.T) {
	c := NewClient("http://prometheus:9090", "")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.baseURL != "http://prometheus:9090" {
		t.Errorf("baseURL = %q, want %q", c.baseURL, "http://prometheus:9090")
	}
}

func TestNewClient_WithCluster(t *testing.T) {
	c := NewClient("http://prometheus:9090", "astro-prod")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.Cluster() != "astro-prod" {
		t.Errorf("Cluster() = %q, want %q", c.Cluster(), "astro-prod")
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

	c := NewClient(srv.URL, "")
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

	c := NewClient(srv.URL, "")
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

	c := NewClient(srv.URL, "")
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

	c := NewClient(srv.URL, "")
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

	c := NewClient(srv.URL, "")
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

	c := NewClient(srv.URL, "")
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

	c := NewClient(srv.URL, "")
	_, err := c.Query(context.Background(), "up")
	if err == nil {
		t.Fatal("expected error for non-vector result type")
	}
}

func TestQueryRange_Success(t *testing.T) {
	start := time.Unix(1700000000, 0)
	end := time.Unix(1700000600, 0)
	step := 30 * time.Second

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/query_range" {
			t.Errorf("path = %q, want /api/v1/query_range", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("query") != "up" {
			t.Errorf("query = %q, want up", q.Get("query"))
		}
		if q.Get("start") != "1700000000" {
			t.Errorf("start = %q, want 1700000000", q.Get("start"))
		}
		if q.Get("end") != "1700000600" {
			t.Errorf("end = %q, want 1700000600", q.Get("end"))
		}
		if q.Get("step") != "30s" {
			t.Errorf("step = %q, want 30s", q.Get("step"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "matrix",
				"result": [
					{
						"metric": {"agent": "acct.bot"},
						"values": [
							[1700000000, "1"],
							[1700000030, "2.5"],
							[1700000060, "3"]
						]
					},
					{
						"metric": {"agent": "acct.helper"},
						"values": [
							[1700000000, "10"]
						]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	series, err := c.QueryRange(context.Background(), "up", start, end, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(series) != 2 {
		t.Fatalf("expected 2 series, got %d", len(series))
	}
	if series[0].Labels["agent"] != "acct.bot" {
		t.Errorf("series[0] agent = %q", series[0].Labels["agent"])
	}
	if len(series[0].Points) != 3 {
		t.Fatalf("series[0] points = %d, want 3", len(series[0].Points))
	}
	if series[0].Points[1].Value != 2.5 {
		t.Errorf("series[0].Points[1].Value = %f, want 2.5", series[0].Points[1].Value)
	}
	if !series[0].Points[1].Timestamp.Equal(time.Unix(1700000030, 0)) {
		t.Errorf("series[0].Points[1].Timestamp = %v, want %v",
			series[0].Points[1].Timestamp, time.Unix(1700000030, 0))
	}
	if len(series[1].Points) != 1 || series[1].Points[0].Value != 10 {
		t.Errorf("series[1] points wrong: %+v", series[1].Points)
	}
}

func TestQueryRange_EmptyMatrix(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"resultType":"matrix","result":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	series, err := c.QueryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(series) != 0 {
		t.Errorf("expected 0 series, got %d", len(series))
	}
}

func TestQueryRange_PrometheusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"error","errorType":"bad_data","error":"parse error"}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	_, err := c.QueryRange(context.Background(), "bad{", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestQueryRange_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	_, err := c.QueryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestQueryRange_WrongResultType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"status":"success","data":{"resultType":"vector","result":[]}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	_, err := c.QueryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err == nil {
		t.Fatal("expected error for non-matrix result type")
	}
}

func TestQueryRange_SkipsMalformedPoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{
			"status": "success",
			"data": {
				"resultType": "matrix",
				"result": [
					{
						"metric": {"a": "1"},
						"values": [
							[1700000000, "42"],
							[1700000030, "not-a-number"],
							[1700000060],
							["bad-ts", "1"],
							[1700000090, 999]
						]
					}
				]
			}
		}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	series, err := c.QueryRange(context.Background(), "up", time.Unix(0, 0), time.Unix(60, 0), time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(series) != 1 {
		t.Fatalf("expected 1 series, got %d", len(series))
	}
	if len(series[0].Points) != 1 || series[0].Points[0].Value != 42 {
		t.Errorf("expected single valid point with value 42, got %+v", series[0].Points)
	}
}

// A slow upstream must not outlive the per-request cap, and the cap must be
// the caller's choice — an http.Client.Timeout would make it unraisable.
func TestQueryWithTimeout_BoundsSlowUpstream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	start := time.Now()
	_, err := NewClient(srv.URL, "").QueryWithTimeout(context.Background(), "up", 150*time.Millisecond)
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; the per-call timeout was ignored", elapsed)
	}
}

// QueryWithTimeout can only tighten the caller's deadline, never extend it.
func TestQueryWithTimeout_CallerDeadlineStillWins(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := NewClient(srv.URL, "").QueryWithTimeout(ctx, "up", time.Hour); err == nil {
		t.Fatal("expected a timeout error")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %s; the caller's shorter deadline was ignored", elapsed)
	}
}
