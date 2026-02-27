package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAstroAISpecSchema(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/schema/package.json", AstroAISpecSchema())

	req := httptest.NewRequest(http.MethodGet, "/schema/package.json", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	ct := rec.Header().Get("Content-Type")
	if ct != "application/schema+json" {
		t.Errorf("Content-Type = %q, want application/schema+json", ct)
	}

	var schema map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &schema); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if schema["type"] != "object" {
		t.Errorf("schema.type = %v, want object", schema["type"])
	}

	props, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema.properties missing or not an object")
	}

	for _, key := range []string{"spec", "name", "agent", "models", "knowledge", "integrations"} {
		if _, ok := props[key]; !ok {
			t.Errorf("schema.properties missing key %q", key)
		}
	}

	if _, hasTools := props["tools"]; hasTools {
		t.Error("schema.properties should not contain deprecated key \"tools\"")
	}
}
