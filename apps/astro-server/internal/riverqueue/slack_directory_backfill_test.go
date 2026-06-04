package riverqueue

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
)

// distinctBareSlackUserIDs MUST send non-empty fromTimestamp/toTimestamp
// on its Langfuse query. The /api/public/metrics endpoint returns 400 on
// empty timestamps, which previously caused the backfill to fail
// silently per-account and write zero observed rows. This test pins the
// fix so a future refactor that drops the date range will fail loudly.
func TestDistinctBareSlackUserIDs_SendsNonEmptyTimestamps(t *testing.T) {
	var capturedQuery langfuse.MetricsQuery

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("query")
		if q == "" {
			t.Errorf("expected ?query= parameter")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if err := json.Unmarshal([]byte(q), &capturedQuery); err != nil {
			t.Errorf("decode query: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	if _, err := distinctBareSlackUserIDs(context.Background(), client); err != nil {
		t.Fatalf("distinctBareSlackUserIDs: %v", err)
	}

	if capturedQuery.FromTimestamp == "" {
		t.Errorf("expected non-empty fromTimestamp; Langfuse rejects empty as 400")
	}
	if capturedQuery.ToTimestamp == "" {
		t.Errorf("expected non-empty toTimestamp; Langfuse rejects empty as 400")
	}
}

// Only bare Slack user_ids flow through — non-matching IDs (workos
// "user_…", empty, dashes) are filtered out before upsert. Pins the
// regex contract on which the merge code depends.
func TestDistinctBareSlackUserIDs_FiltersToBareShape(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Mixed shapes: legit Slack IDs, workos IDs, sentinels, garbage.
		_, _ = w.Write([]byte(`{
			"data": [
				{"userId": "U07ABCDEFG", "count_count": 12},
				{"userId": "user_01HABCDEFG", "count_count": 8},
				{"userId": "U08TSB1BZ5H", "count_count": 3},
				{"userId": "-", "count_count": 1},
				{"userId": "", "count_count": 1},
				{"userId": "slackbot", "count_count": 1}
			]
		}`))
	}))
	defer srv.Close()

	client := langfuse.NewClient(srv.URL, "pk", "sk")
	got, err := distinctBareSlackUserIDs(context.Background(), client)
	if err != nil {
		t.Fatalf("distinctBareSlackUserIDs: %v", err)
	}

	want := map[string]bool{"U07ABCDEFG": true, "U08TSB1BZ5H": true}
	if len(got) != len(want) {
		t.Errorf("filter returned %d ids, want %d: %v", len(got), len(want), got)
	}
	for _, uid := range got {
		if !want[uid] {
			t.Errorf("unexpected uid in result: %q", uid)
		}
	}
}
