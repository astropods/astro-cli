package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseBlueprintListFilters_DefaultSort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agents/acct?q=bot", nil)

	f, err := ParseBlueprintListFilters(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Query != "bot" || f.Sort != "name" {
		t.Fatalf("got q=%q sort=%q", f.Query, f.Sort)
	}
	if f.Limit != defaultBlueprintListLimit || f.Offset != 0 {
		t.Fatalf("got limit=%d offset=%d", f.Limit, f.Offset)
	}
}

func TestParseBlueprintListFilters_InvalidSort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agents/acct?sort=invalid", nil)

	_, err := ParseBlueprintListFilters(c)
	if err == nil {
		t.Fatal("expected error for invalid sort")
	}
}

func TestParseBlueprintListFilters_Pagination(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agents/acct?limit=10&offset=20", nil)

	f, err := ParseBlueprintListFilters(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Limit != 10 || f.Offset != 20 {
		t.Fatalf("got limit=%d offset=%d", f.Limit, f.Offset)
	}
}

func TestParseBlueprintListFilters_LimitCapped(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agents/acct?limit=500", nil)

	f, err := ParseBlueprintListFilters(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Limit != maxBlueprintListLimit {
		t.Fatalf("got limit=%d want %d", f.Limit, maxBlueprintListLimit)
	}
}

func TestParseBlueprintListFilters_OffsetTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/agents/acct?offset=10001", nil)

	_, err := ParseBlueprintListFilters(c)
	if err == nil {
		t.Fatal("expected error for offset above max")
	}
}
