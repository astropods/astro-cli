package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// windowRouter exposes parseBillingWindow's own decision directly, without a
// provider behind it: every case here is decided before a provider would be
// reached.
func windowRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/window", func(c *gin.Context) {
		from, to, ok := parseBillingWindow(c)
		if !ok {
			return
		}
		c.JSON(http.StatusOK, gin.H{"from": from, "to": to})
	})
	return r
}

func getWindow(t *testing.T, r *gin.Engine, query string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/window"+query, nil)
	r.ServeHTTP(rec, req)
	return rec
}

// A window right at the bound is a real, if unusual, report: a full year of
// daily spend. It must not be caught by the same check that refuses one day
// past it.
func TestParseBillingWindow_TheBoundItselfIsAllowed(t *testing.T) {
	r := windowRouter(t)
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, maxBillingWindowDays)
	query := "?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)

	rec := getWindow(t, r, query)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}

// One day past the bound is exactly what pages the breakdown endpoint
// serially until the request's own deadline cuts it off with an opaque 502.
// A 400 naming the bound is the whole point of the check.
func TestParseBillingWindow_PastTheBoundIsRefusedWithA400(t *testing.T) {
	r := windowRouter(t)
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 0, maxBillingWindowDays+1)
	query := "?from=" + from.Format(time.RFC3339) + "&to=" + to.Format(time.RFC3339)

	rec := getWindow(t, r, query)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	var got struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if want := strconv.Itoa(maxBillingWindowDays); !strings.Contains(got.Error, want) {
		t.Errorf("error = %q, want the bound named", got.Error)
	}
}

// With no query params at all, the default is the current calendar month,
// nowhere near the bound; the check must not fire on the common case.
func TestParseBillingWindow_DefaultWindowIsAllowed(t *testing.T) {
	r := windowRouter(t)
	rec := getWindow(t, r, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
}
