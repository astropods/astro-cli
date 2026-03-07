package openapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type testInput struct {
	Name  string `json:"name" binding:"required"`
	Email string `json:"email"`
}

type testOutput struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func TestSpecGeneratesValidOpenAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()

	api := New("Test API", "0.1.0", "A test")

	g := router.Group("/api/v1")
	{
		api.GET(g, "/items", "List items", func(c *gin.Context) {},
			Tags("Items"),
			Response(200, &[]testOutput{}),
		)

		api.GET(g, "/items/:id", "Get an item", func(c *gin.Context) {},
			Tags("Items"),
			PathParam("id", "Item ID"),
			Response(200, &testOutput{}),
			Response(404, nil),
		)

		api.POST(g, "/items", "Create an item", func(c *gin.Context) {},
			Tags("Items"),
			BearerAuth(),
			Body(&testInput{}),
			Response(201, &testOutput{}),
		)

		api.GET(g, "/search", "Search items", func(c *gin.Context) {},
			Tags("Items"),
			QueryParam("q", "Search query", true),
			QueryParam("limit", "Max results", false),
			Response(200, &[]testOutput{}),
		)

		api.DELETE(g, "/items/:id", "Delete an item", func(c *gin.Context) {},
			Tags("Items"),
			BearerAuth(),
			PathParam("id", "Item ID"),
			Response(204, nil),
		)
	}

	// Serve and fetch the spec
	router.GET("/openapi.json", api.JSON())
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/openapi.json", nil)
	router.ServeHTTP(w, req)

	if w.Code != 200 {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	// Basic structure checks
	if doc["openapi"] != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %v", doc["openapi"])
	}

	info := doc["info"].(map[string]any)
	if info["title"] != "Test API" {
		t.Errorf("expected title 'Test API', got %v", info["title"])
	}

	paths := doc["paths"].(map[string]any)

	// Check path param conversion: :id -> {id}
	if _, ok := paths["/api/v1/items/{id}"]; !ok {
		t.Errorf("expected path /api/v1/items/{id}, got paths: %v", keysOf(paths))
	}

	// Check that all 4 paths are present
	expectedPaths := []string{"/api/v1/items", "/api/v1/items/{id}", "/api/v1/search"}
	for _, p := range expectedPaths {
		if _, ok := paths[p]; !ok {
			t.Errorf("missing path %s", p)
		}
	}

	// Check GET /api/v1/items has the Items tag
	itemsPath := paths["/api/v1/items"].(map[string]any)
	getOp := itemsPath["get"].(map[string]any)
	tags := getOp["tags"].([]any)
	if len(tags) == 0 || tags[0] != "Items" {
		t.Errorf("expected tag 'Items', got %v", tags)
	}

	// Check POST /api/v1/items has security
	postOp := itemsPath["post"].(map[string]any)
	if postOp["security"] == nil {
		t.Error("expected security on POST /api/v1/items")
	}

	// Check POST has request body
	if postOp["requestBody"] == nil {
		t.Error("expected requestBody on POST /api/v1/items")
	}

	// Check GET /api/v1/search has query params
	searchPath := paths["/api/v1/search"].(map[string]any)
	searchGet := searchPath["get"].(map[string]any)
	params := searchGet["parameters"].([]any)
	if len(params) != 2 {
		t.Errorf("expected 2 query params, got %d", len(params))
	}

	// Check bearer auth security scheme exists
	components := doc["components"].(map[string]any)
	schemes := components["securitySchemes"].(map[string]any)
	if _, ok := schemes["bearerAuth"]; !ok {
		t.Error("expected bearerAuth security scheme")
	}
}

func keysOf(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
