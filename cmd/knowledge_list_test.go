package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The account knowledge list returns a page envelope rather than a bare array.
// These cover the decode and the cursor walk, neither of which had coverage
// when the shape changed.

func setupKnowledgeTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	knowledgeServerURLOverride = srv.URL
	t.Cleanup(func() { knowledgeServerURLOverride = "" })
}

func knowledgePage(stores []map[string]any, nextCursor string) map[string]any {
	page := map[string]any{"limit": 50}
	if nextCursor != "" {
		page["next_cursor"] = nextCursor
	}
	return map[string]any{"stores": stores, "page": page}
}

func knowledgeStoreCell(name string) map[string]any {
	return map[string]any{
		"id": "ks-" + name, "arn": "arn:knowledge:testaccount:" + name,
		"name": name, "provider": "postgres", "mode": "external",
		"status": "ready", "created_at": "2026-09-01T10:00:00Z",
	}
}

func runKnowledgeListForTest(t *testing.T) string {
	t.Helper()
	buf := &bytes.Buffer{}
	cmd := knowledgeListCmd
	cmd.SetOut(buf)
	cmd.SetContext(context.Background())
	require.NoError(t, runKnowledgeList(cmd, nil))
	return buf.String()
}

func TestKnowledgeListDecodesThePageEnvelope(t *testing.T) {
	setupKnowledgeTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(knowledgePage(
			[]map[string]any{knowledgeStoreCell("vectors")}, ""))
	}))

	out := runKnowledgeListForTest(t)
	assert.Contains(t, out, "vectors")
	assert.Contains(t, out, "postgres")
}

// A multi-page account must print every store, not just the first page. The
// bare-array shape had no cursor, so this walk is new and untested behaviour.
func TestKnowledgeListFollowsTheCursorToTheLastPage(t *testing.T) {
	var cursors []string
	setupKnowledgeTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := r.URL.Query().Get("cursor")
		cursors = append(cursors, cursor)
		w.Header().Set("Content-Type", "application/json")
		switch cursor {
		case "":
			_ = json.NewEncoder(w).Encode(knowledgePage(
				[]map[string]any{knowledgeStoreCell("first")}, "cursor-2"))
		default:
			_ = json.NewEncoder(w).Encode(knowledgePage(
				[]map[string]any{knowledgeStoreCell("second")}, ""))
		}
	}))

	out := runKnowledgeListForTest(t)
	assert.Equal(t, []string{"", "cursor-2"}, cursors, "the cursor must be sent on the second request")
	assert.Contains(t, out, "first")
	assert.Contains(t, out, "second", "a store on a later page was dropped")
}

func TestKnowledgeListReportsAnEmptyAccount(t *testing.T) {
	setupKnowledgeTest(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(knowledgePage(nil, ""))
	}))

	// The empty notice goes to stdout directly rather than through the command
	// writer, so this asserts the call succeeds and prints no store rows.
	out := runKnowledgeListForTest(t)
	assert.NotContains(t, out, "postgres")
}
