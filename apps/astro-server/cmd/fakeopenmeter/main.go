// fakeopenmeter is a lightweight OpenMeter stub for local development.
// It accepts all OpenMeter API calls the server makes (customers, events, meter queries)
// and returns plausible fake data so the usage page works without a real OpenMeter instance.
//
// Usage: go run ./cmd/fakeopenmeter
// Default port: 8888 (override with PORT env var)
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand/v2" //nolint:gosec // fake dev data, not security-sensitive
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// store holds ingested events in memory so meter queries can return semi-realistic values.
type store struct {
	mu     sync.Mutex
	events []cloudEvent
}

type cloudEvent struct {
	Type    string `json:"type"`
	Subject string `json:"subject"`
	Data    any    `json:"data"`
	Time    string `json:"time"`
}

var db = &store{}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8888"
	}

	mux := http.NewServeMux()

	// Customer creation — accept and return a fake ID
	mux.HandleFunc("POST /api/v1/customers", handleCreateCustomer)

	// Event ingestion — store in memory
	mux.HandleFunc("POST /api/v1/events", handleIngestEvents)

	// Meter queries — return aggregated or seed data
	mux.HandleFunc("GET /api/v1/meters/{meterSlug}/query", handleQueryMeter)

	// Entitlement checks — always grant access
	mux.HandleFunc("GET /api/v1/subjects/{subject}/entitlements/{feature}/value", handleEntitlement)

	// Catch-all for other OpenMeter endpoints we don't need
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[fakeopenmeter] %s %s (unhandled, returning 200)", r.Method, r.URL.EscapedPath()) //nolint:gosec // dev-only server, path is logged for debugging
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	})

	log.Printf("[fakeopenmeter] listening on :%s", port) //nolint:gosec // port is from env, not user input
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	name, _ := body["name"].(string)
	log.Printf("[fakeopenmeter] POST /api/v1/customers name=%q", name)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]string{
		"id": fmt.Sprintf("fake-cust-%d", time.Now().UnixMilli()),
	}); err != nil {
		log.Printf("[fakeopenmeter] encode error: %v", err)
	}
}

func handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	var events []cloudEvent
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		log.Printf("[fakeopenmeter] POST /api/v1/events decode error: %v", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	db.mu.Lock()
	db.events = append(db.events, events...)
	db.mu.Unlock()

	log.Printf("[fakeopenmeter] POST /api/v1/events ingested %d events", len(events))
	w.WriteHeader(http.StatusNoContent)
}

func handleQueryMeter(w http.ResponseWriter, r *http.Request) {
	meterSlug := r.PathValue("meterSlug")
	subject := r.URL.Query().Get("subject")

	log.Printf("[fakeopenmeter] GET /api/v1/meters/%s/query subject=%s", meterSlug, subject) //nolint:gosec // path values are from route params, not raw input

	// First check if we have real ingested events for this meter+subject
	value := aggregateEvents(meterSlug, subject)

	// If no real events, return seed data so the UI isn't empty
	if value == 0 {
		value = seedValue(meterSlug)
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"data": []map[string]any{
			{
				"value":       value,
				"windowStart": r.URL.Query().Get("from"),
				"windowEnd":   r.URL.Query().Get("to"),
				"subject":     subject,
				"groupBy":     map[string]string{},
			},
		},
	}); err != nil {
		log.Printf("[fakeopenmeter] encode error: %v", err)
	}
}

// featureLimits defines fake plan limits for each entitlement feature.
var featureLimits = map[string]int64{
	"compute_usage":      100, // 100 CU-hours/month
	"agent_builds":       50,  // 50 builds/month
	"active_deployments": 5,   // 5 concurrent deployments
	"active_agents":      10,  // 10 registered agents
}

func handleEntitlement(w http.ResponseWriter, r *http.Request) {
	subject := r.PathValue("subject")
	feature := r.PathValue("feature")

	limit, ok := featureLimits[feature]
	if !ok {
		limit = 1000
	}
	log.Printf("[fakeopenmeter] GET entitlement subject=%s feature=%s → limit=%d", subject, feature, limit) //nolint:gosec // path values are from route params

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"hasAccess": true,
		"usage":     0,
		"limit":     limit,
	}); err != nil {
		log.Printf("[fakeopenmeter] encode error: %v", err)
	}
}

// aggregateEvents sums up values from ingested events matching the meter type and subject.
func aggregateEvents(meterSlug, subject string) float64 {
	db.mu.Lock()
	defer db.mu.Unlock()

	var total float64
	for _, ev := range db.events {
		if ev.Type != meterSlug {
			continue
		}
		if subject != "" && ev.Subject != subject {
			continue
		}
		data, ok := ev.Data.(map[string]any)
		if !ok {
			total++
			continue
		}

		switch {
		case strings.Contains(meterSlug, "compute"):
			if v, ok := data["compute_unit_hours"].(float64); ok {
				total += v
			}
		case strings.Contains(meterSlug, "deployment") || strings.Contains(meterSlug, "agent"):
			if v, ok := data["count"].(float64); ok {
				total += v
			} else {
				total++
			}
		default:
			total++
		}
	}
	return total
}

// seedValue returns plausible fake values when no real events have been ingested.
func seedValue(meterSlug string) float64 {
	switch meterSlug {
	case "compute_usage":
		return math.Round((12.5+rand.Float64()*5)*100) / 100 //nolint:gosec // fake dev data
	case "agent_build":
		return float64(3 + rand.IntN(8)) //nolint:gosec // fake dev data
	case "active_deployments":
		return float64(1 + rand.IntN(4)) //nolint:gosec // fake dev data
	case "active_agents":
		return float64(2 + rand.IntN(5)) //nolint:gosec // fake dev data
	default:
		return 0
	}
}
