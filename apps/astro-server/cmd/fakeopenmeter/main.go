// fakeopenmeter is a lightweight OpenMeter stub for local development.
// It accepts all OpenMeter API calls the server makes (customers, events, meter queries,
// entitlements, subscriptions) and returns responses that strictly follow the OpenMeter
// OpenAPI spec (apps/astro-queen/openapi.json).
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

	// Customer creation — accept and return a full Customer object (201)
	mux.HandleFunc("POST /api/v1/customers", handleCreateCustomer)

	// Event ingestion — store in memory, return 204
	mux.HandleFunc("POST /api/v1/events", handleIngestEvents)

	// List meters — return bare array of Meter objects
	mux.HandleFunc("GET /api/v1/meters", handleListMeters)

	// Meter queries — return MeterQueryResult with envelope fields
	mux.HandleFunc("GET /api/v1/meters/{meterSlug}/query", handleQueryMeter)

	// Entitlement checks — return EntitlementValue per spec
	mux.HandleFunc("GET /api/v1/subjects/{subject}/entitlements/{feature}/value", handleEntitlement)

	// Subscription creation — return Subscription object (201)
	mux.HandleFunc("POST /api/v1/subscriptions", handleCreateSubscription)

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

// handleCreateCustomer returns a full Customer object per the OpenAPI spec (201 Created).
func handleCreateCustomer(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	name, _ := body["name"].(string)
	key, _ := body["key"].(string)
	log.Printf("[fakeopenmeter] POST /api/v1/customers name=%q key=%q", name, key)

	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("01J%013d", time.Now().UnixMilli()) // ULID-ish

	// Echo back request fields and add server-generated readOnly fields.
	resp := map[string]any{
		"id":               id,
		"name":             name,
		"key":              key,
		"usageAttribution": body["usageAttribution"],
		"primaryEmail":     body["primaryEmail"],
		"currency":         body["currency"],
		"metadata":         body["metadata"],
		"createdAt":        now,
		"updatedAt":        now,
		"deletedAt":        nil,
		"annotations":      map[string]string{},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[fakeopenmeter] encode error: %v", err)
	}
}

// handleIngestEvents accepts CloudEvents batch and returns 204 No Content per spec.
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

// requiredMeters is the set of meters the fake server advertises.
var requiredMeters = []map[string]any{
	{"slug": "compute", "id": "01METER0COMPUTE0000", "name": "Compute Usage", "aggregation": "SUM", "eventType": "compute", "valueProperty": "$.compute_unit_hours", "groupBy": map[string]string{}, "metadata": map[string]string{}},
	{"slug": "agents", "id": "01METER0AGENTS00000", "name": "Active Agents", "aggregation": "COUNT", "eventType": "agents", "groupBy": map[string]string{}, "metadata": map[string]string{}},
	{"slug": "agent_builds", "id": "01METER0BUILDS00000", "name": "Agent Builds", "aggregation": "COUNT", "eventType": "agent_builds", "groupBy": map[string]string{}, "metadata": map[string]string{}},
	{"slug": "agent_deployments", "id": "01METER0DEPLOYS0000", "name": "Agent Deployments", "aggregation": "COUNT", "eventType": "agent_deployments", "groupBy": map[string]string{}, "metadata": map[string]string{}},
	{"slug": "members", "id": "01METER0MEMBERS0000", "name": "Team Members", "aggregation": "COUNT", "eventType": "members", "groupBy": map[string]string{}, "metadata": map[string]string{}},
}

// handleListMeters returns a bare JSON array of Meter objects per the OpenAPI spec.
func handleListMeters(w http.ResponseWriter, r *http.Request) {
	log.Printf("[fakeopenmeter] GET /api/v1/meters")

	now := time.Now().UTC().Format(time.RFC3339)
	meters := make([]map[string]any, len(requiredMeters))
	for i, m := range requiredMeters {
		meter := make(map[string]any, len(m)+3)
		for k, v := range m {
			meter[k] = v
		}
		meter["createdAt"] = now
		meter["updatedAt"] = now
		meter["deletedAt"] = nil
		meters[i] = meter
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(meters); err != nil {
		log.Printf("[fakeopenmeter] encode error: %v", err)
	}
}

// handleQueryMeter returns a MeterQueryResult with the full envelope per spec:
// { from, to, windowSize, data: [{ value, windowStart, windowEnd, subject, groupBy }] }
func handleQueryMeter(w http.ResponseWriter, r *http.Request) {
	meterSlug := r.PathValue("meterSlug")
	subject := r.URL.Query().Get("subject")
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	windowSize := r.URL.Query().Get("windowSize")

	log.Printf("[fakeopenmeter] GET /api/v1/meters/%s/query subject=%s", meterSlug, subject) //nolint:gosec // path values are from route params, not raw input

	// First check if we have real ingested events for this meter+subject
	value := aggregateEvents(meterSlug, subject)

	// If no real events, return seed data so the UI isn't empty
	if value == 0 {
		value = seedValue(meterSlug)
	}

	resp := map[string]any{
		"from":       from,
		"to":         to,
		"windowSize": windowSize,
		"data": []map[string]any{
			{
				"value":       value,
				"windowStart": from,
				"windowEnd":   to,
				"subject":     subject,
				"groupBy":     map[string]string{},
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
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

// handleEntitlement returns an EntitlementValue per spec with fields:
// { hasAccess, balance, usage, overage }
func handleEntitlement(w http.ResponseWriter, r *http.Request) {
	subject := r.PathValue("subject")
	feature := r.PathValue("feature")

	limit, ok := featureLimits[feature]
	if !ok {
		limit = 1000
	}
	log.Printf("[fakeopenmeter] GET entitlement subject=%s feature=%s → limit=%d", subject, feature, limit) //nolint:gosec // path values are from route params

	resp := map[string]any{
		"hasAccess":                 true,
		"balance":                   float64(limit),
		"usage":                     float64(0),
		"overage":                   float64(0),
		"totalAvailableGrantAmount": float64(limit),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("[fakeopenmeter] encode error: %v", err)
	}
}

// handleCreateSubscription returns a Subscription object per spec (201 Created).
func handleCreateSubscription(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)

	customerID, _ := body["customerId"].(string)
	plan, _ := body["plan"].(map[string]any)
	planKey := ""
	if plan != nil {
		planKey, _ = plan["key"].(string)
	}
	log.Printf("[fakeopenmeter] POST /api/v1/subscriptions customer=%s plan=%s", customerID, planKey)

	now := time.Now().UTC().Format(time.RFC3339)
	id := fmt.Sprintf("01S%013d", time.Now().UnixMilli()) // ULID-ish

	resp := map[string]any{
		"id":             id,
		"customerId":     customerID,
		"plan":           body["plan"],
		"status":         "active",
		"currency":       "USD",
		"billingCadence": "P1M",
		"activeFrom":     now,
		"activeTo":       nil,
		"metadata":       map[string]string{},
		"createdAt":      now,
		"updatedAt":      now,
		"deletedAt":      nil,
		"annotations":    map[string]string{},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
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
