package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-registry/internal/logger"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func TestLivez(t *testing.T) {
	log := logger.New("error", "text")
	probeHandler := NewProbeHandler(log, nil)

	router := gin.New()
	router.GET("/livez", probeHandler.Livez())

	req, _ := http.NewRequest(http.MethodGet, "/livez", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got '%s'", w.Body.String())
	}
}

func TestReadyz_Ready(t *testing.T) {
	log := logger.New("error", "text")
	probeHandler := NewProbeHandler(log, nil)

	router := gin.New()
	router.GET("/readyz", probeHandler.Readyz())

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	if w.Body.String() != "ok" {
		t.Errorf("expected body 'ok', got '%s'", w.Body.String())
	}
}

func TestReadyz_NotReady(t *testing.T) {
	log := logger.New("error", "text")
	probeHandler := NewProbeHandler(log, nil)
	probeHandler.SetReady(false)

	router := gin.New()
	router.GET("/readyz", probeHandler.Readyz())

	req, _ := http.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestHealthz_Healthy(t *testing.T) {
	log := logger.New("error", "text")
	probeHandler := NewProbeHandler(log, nil)

	router := gin.New()
	router.GET("/healthz", probeHandler.Healthz())

	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got '%v'", response["status"])
	}

	checks, ok := response["checks"].(map[string]interface{})
	if !ok {
		t.Fatal("expected checks in response")
	}

	if checks["process"] != "ok" {
		t.Errorf("expected process check 'ok', got '%v'", checks["process"])
	}

	if checks["ready"] != "ok" {
		t.Errorf("expected ready check 'ok', got '%v'", checks["ready"])
	}
}

func TestHealthz_NotReady(t *testing.T) {
	log := logger.New("error", "text")
	probeHandler := NewProbeHandler(log, nil)
	probeHandler.SetReady(false)

	router := gin.New()
	router.GET("/healthz", probeHandler.Healthz())

	req, _ := http.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if response["status"] != "unhealthy" {
		t.Errorf("expected status 'unhealthy', got '%v'", response["status"])
	}
}

func TestSetReady(t *testing.T) {
	log := logger.New("error", "text")
	probeHandler := NewProbeHandler(log, nil)

	// Should start ready
	if !probeHandler.ready.Load() {
		t.Error("expected probe handler to start ready")
	}

	// Set not ready
	probeHandler.SetReady(false)
	if probeHandler.ready.Load() {
		t.Error("expected probe handler to be not ready")
	}

	// Set ready again
	probeHandler.SetReady(true)
	if !probeHandler.ready.Load() {
		t.Error("expected probe handler to be ready")
	}
}
