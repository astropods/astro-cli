package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/chatstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

func TestSelectLongerHydratedThread_PrefersPostgresWhenMoreComplete(t *testing.T) {
	t.Parallel()

	langfuse := []ChatMessageResponse{
		{ID: "t1-u", Role: "user", Content: "hello"},
	}
	postgres := []ChatMessageResponse{
		{ID: "m1", Role: "user", Content: "hello"},
		{ID: "m2", Role: "assistant", Content: "hi"},
		{ID: "m3", Role: "user", Content: "again"},
		{ID: "m4", Role: "assistant", Content: "sure"},
	}

	got := selectLongerHydratedThread(langfuse, postgres)
	if len(got) != 4 {
		t.Fatalf("expected 4 postgres messages when Langfuse is partial, got %d", len(got))
	}
	if got[0].ID != "m1" || got[3].ID != "m4" {
		t.Fatalf("unexpected merge result: %+v", got)
	}
}

func TestSelectLongerHydratedThread_PrefersLangfuseWhenComplete(t *testing.T) {
	t.Parallel()

	langfuse := []ChatMessageResponse{
		{ID: "t1-u", Role: "user", Content: "a"},
		{ID: "t1-a", Role: "assistant", Content: "b"},
	}
	postgres := []ChatMessageResponse{
		{ID: "m1", Role: "user", Content: "a"},
	}

	got := selectLongerHydratedThread(langfuse, postgres)
	if len(got) != 2 {
		t.Fatalf("expected Langfuse thread when it is the superset, got %d", len(got))
	}
}

func TestHydrateFromChatStore_ReturnsPersistedMessages(t *testing.T) {
	t.Parallel()

	chatDB, chatMock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	chatStore := chatstore.NewStore(chatDB)
	log := logger.New("error", "json")

	chatMock.ExpectQuery("SELECT id::text, role, content, seq").
		WithArgs("dep-1", "conv-1").
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "content", "seq"}).
			AddRow("msg-1", "user", "hello", 1).
			AddRow("msg-2", "assistant", "hi there", 2))

	msgs := hydrateFromChatStore(log, chatStore, "dep-1", "conv-1")
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "user" || msgs[0].Content != "hello" {
		t.Fatalf("unexpected first message: %+v", msgs[0])
	}
	if msgs[1].Role != "assistant" || msgs[1].Content != "hi there" {
		t.Fatalf("unexpected second message: %+v", msgs[1])
	}
	if err := chatMock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestParseConversationPage(t *testing.T) {
	t.Parallel()
	gin.SetMode(gin.TestMode)

	t.Run("full thread by default", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
		page, err := parseConversationPage(c)
		if err != nil {
			t.Fatalf("parseConversationPage: %v", err)
		}
		if page.Limit != 0 || page.BeforeSeq != 0 {
			t.Fatalf("expected empty page, got %+v", page)
		}
	})

	t.Run("tail page", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/?limit=50", nil)
		page, err := parseConversationPage(c)
		if err != nil {
			t.Fatalf("parseConversationPage: %v", err)
		}
		if page.Limit != 50 || page.BeforeSeq != 0 {
			t.Fatalf("unexpected page: %+v", page)
		}
	})

	t.Run("older page", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/?limit=25&before_seq=40", nil)
		page, err := parseConversationPage(c)
		if err != nil {
			t.Fatalf("parseConversationPage: %v", err)
		}
		if page.Limit != 25 || page.BeforeSeq != 40 {
			t.Fatalf("unexpected page: %+v", page)
		}
	})

	t.Run("rejects invalid limit", func(t *testing.T) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/?limit=0", nil)
		_, err := parseConversationPage(c)
		if err == nil {
			t.Fatal("expected invalid limit error")
		}
	})
}

func TestPaginateConversationMessages_TailPage(t *testing.T) {
	t.Parallel()

	all := make([]ChatMessageResponse, 120)
	for i := range all {
		all[i] = ChatMessageResponse{ID: fmt.Sprintf("m%d", i+1), Role: "user", Content: "x"}
	}

	msgs, hasMore, oldestSeq := paginateConversationMessages(all, chatConversationPage{Limit: 100}, false)
	if len(msgs) != 100 {
		t.Fatalf("expected 100 messages, got %d", len(msgs))
	}
	if !hasMore {
		t.Fatal("expected has_more")
	}
	if oldestSeq != 21 {
		t.Fatalf("oldest_seq = %d, want 21", oldestSeq)
	}
	if msgs[0].ID != "m21" {
		t.Fatalf("first message = %q, want m21", msgs[0].ID)
	}
}

func TestPaginateConversationMessages_OlderPage(t *testing.T) {
	t.Parallel()

	all := []ChatMessageResponse{
		{ID: "m1", Role: "user", Content: "a"},
		{ID: "m2", Role: "assistant", Content: "b"},
		{ID: "m3", Role: "user", Content: "c"},
		{ID: "m4", Role: "assistant", Content: "d"},
	}

	msgs, hasMore, oldestSeq := paginateConversationMessages(
		all,
		chatConversationPage{Limit: 2, BeforeSeq: 3},
		false,
	)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if hasMore {
		t.Fatal("expected no further pages")
	}
	if oldestSeq != 1 {
		t.Fatalf("oldest_seq = %d, want 1", oldestSeq)
	}
	if msgs[0].ID != "m1" || msgs[1].ID != "m2" {
		t.Fatalf("unexpected page: %+v", msgs)
	}
}

func TestIsAbortMarker(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   map[string]any
		want bool
	}{
		{
			name: "exact marker matches",
			in:   map[string]any{"status": "aborted", "reason": "abort"},
			want: true,
		},
		{
			name: "case-insensitive match",
			in:   map[string]any{"status": "Aborted", "reason": "ABORT"},
			want: true,
		},
		{
			name: "extra fields still match",
			in:   map[string]any{"status": "aborted", "reason": "abort", "note": "user stopped"},
			want: true,
		},
		{
			name: "status only does NOT match (tightened)",
			in:   map[string]any{"status": "aborted"},
			want: false,
		},
		{
			name: "reason only does NOT match (tightened)",
			in:   map[string]any{"reason": "abort"},
			want: false,
		},
		{
			name: "legit message with an aborted status field is not a marker",
			in:   map[string]any{"status": "aborted", "reason": "policy", "content": "here is the answer"},
			want: false,
		},
		{
			name: "unrelated object",
			in:   map[string]any{"content": "hello", "role": "assistant"},
			want: false,
		},
		{
			name: "non-string status field",
			in:   map[string]any{"status": 1, "reason": "abort"},
			want: false,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := isAbortMarker(tc.in); got != tc.want {
				t.Fatalf("isAbortMarker(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestTraceContentText_MarkerYieldsEmpty(t *testing.T) {
	t.Parallel()

	if got := traceContentText(map[string]any{"status": "aborted", "reason": "abort"}); got != "" {
		t.Fatalf("traceContentText(marker) = %q, want empty so the persisted partial wins", got)
	}

	// A near-marker that carries real content must NOT be dropped now that the
	// matcher requires both fields.
	got := traceContentText(map[string]any{"status": "aborted", "content": "partial answer"})
	if got != "partial answer" {
		t.Fatalf("traceContentText(status-only + content) = %q, want %q", got, "partial answer")
	}
}
