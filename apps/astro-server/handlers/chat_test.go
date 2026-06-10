package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/chatstore"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func TestValidateChatMessage_RejectsEmptyContent(t *testing.T) {
	t.Parallel()

	err := validateChatMessage(ChatMessageResponse{
		ID:      uuid.NewString(),
		Role:    "user",
		Content: "   ",
	})
	if err == nil {
		t.Fatal("expected empty content error")
	}
	inv, ok := chatInvalidFromErr(err)
	if !ok || inv.Error() != "message content is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeChatMessage_TrimsRoleAndCanonicalizesID(t *testing.T) {
	t.Parallel()

	id := uuid.NewString()
	got := normalizeChatMessage(ChatMessageResponse{
		ID:      " " + id + " ",
		Role:    " user ",
		Content: "hello",
	})
	if got.ID != id {
		t.Fatalf("expected canonical id %s, got %s", id, got.ID)
	}
	if got.Role != "user" {
		t.Fatalf("expected trimmed role user, got %q", got.Role)
	}
	if got.Content != "hello" {
		t.Fatalf("expected content preserved, got %q", got.Content)
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

func TestValidateChatMessage_RejectsOversizeContent(t *testing.T) {
	t.Parallel()

	content := strings.Repeat("x", chatstore.MaxMessageContentRunes+1)
	err := validateChatMessage(ChatMessageResponse{
		ID:      uuid.NewString(),
		Role:    "assistant",
		Content: content,
	})
	if err == nil {
		t.Fatal("expected oversize content error")
	}
}
