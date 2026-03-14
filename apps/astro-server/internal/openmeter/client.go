package openmeter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// Client is a typed HTTP client for the OpenMeter API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new OpenMeter client. Returns nil if baseURL is empty.
func NewClient(baseURL string) *Client {
	if baseURL == "" {
		return nil
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Customer represents an OpenMeter customer.
type Customer struct {
	ID               string            `json:"id,omitempty"`
	Name             string            `json:"name"`
	Key              string            `json:"key"`
	UsageAttribution UsageAttribution  `json:"usageAttribution"`
	PrimaryEmail     string            `json:"primaryEmail,omitempty"`
	Currency         string            `json:"currency,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// UsageAttribution controls how usage events are attributed to a customer.
type UsageAttribution struct {
	SubjectKeys []string `json:"subjectKeys"`
}

// CreateCustomer creates a customer in OpenMeter mapped to an Astro account.
// Returns the OpenMeter customer ID.
func (c *Client) CreateCustomer(ctx context.Context, accountID, accountName, accountType, ownerEmail string) (string, error) {
	customer := Customer{
		Name: accountName,
		Key:  accountID,
		UsageAttribution: UsageAttribution{
			SubjectKeys: []string{accountID},
		},
		PrimaryEmail: ownerEmail,
		Currency:     "USD",
		Metadata: map[string]string{
			"type": accountType,
		},
	}

	body, err := json.Marshal(customer)
	if err != nil {
		return "", fmt.Errorf("marshal customer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/customers", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return "", fmt.Errorf("create customer request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create customer: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode customer response: %w", err)
	}

	return result.ID, nil
}

// CloudEvent is a CloudEvents v1.0 envelope for OpenMeter event ingestion.
type CloudEvent struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	SpecVersion string `json:"specversion"`
	Type        string `json:"type"`
	Subject     string `json:"subject"`
	Time        string `json:"time"`
	Data        any    `json:"data"`
}

// NewCloudEvent creates a CloudEvent with standard fields pre-filled.
func NewCloudEvent(eventType, subject string, data any) CloudEvent {
	return CloudEvent{
		ID:          uuid.New().String(),
		Source:      "astro-server",
		SpecVersion: "1.0",
		Type:        eventType,
		Subject:     subject,
		Time:        time.Now().UTC().Format(time.RFC3339),
		Data:        data,
	}
}

// IngestEvents sends a batch of CloudEvents to OpenMeter.
func (c *Client) IngestEvents(ctx context.Context, events []CloudEvent) error {
	body, err := json.Marshal(events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/cloudevents-batch+json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return fmt.Errorf("ingest events request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ingest events: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// MeterQueryRow represents a single row in a meter query response.
type MeterQueryRow struct {
	Value       float64           `json:"value"`
	WindowStart string            `json:"windowStart"`
	WindowEnd   string            `json:"windowEnd"`
	Subject     string            `json:"subject,omitempty"`
	GroupBy     map[string]string `json:"groupBy,omitempty"`
}

// MeterQueryResult represents the response from a meter query.
type MeterQueryResult struct {
	Data []MeterQueryRow `json:"data"`
}

// QueryMeter queries a meter for a given subject over a time range.
// windowSize can be MINUTE, HOUR, DAY, or empty for total aggregation.
func (c *Client) QueryMeter(ctx context.Context, meterSlug, subject string, from, to time.Time, windowSize string) (*MeterQueryResult, error) {
	url := fmt.Sprintf("%s/api/v1/meters/%s/query?subject=%s&from=%s&to=%s",
		c.baseURL, meterSlug, subject,
		from.UTC().Format(time.RFC3339),
		to.UTC().Format(time.RFC3339),
	)
	if windowSize != "" {
		url += "&windowSize=" + windowSize
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return nil, fmt.Errorf("query meter request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query meter: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result MeterQueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode meter query response: %w", err)
	}

	return &result, nil
}

// Meter represents an OpenMeter meter definition.
type Meter struct {
	Slug string `json:"slug"`
}

// RequiredMeters is the set of meter slugs that must exist in OpenMeter.
var RequiredMeters = []string{
	"compute",
	"agents",
	"agent_builds",
	"agent_deployments",
	"members",
}

// ListMeters fetches all meters from OpenMeter.
func (c *Client) ListMeters(ctx context.Context) ([]Meter, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/meters", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return nil, fmt.Errorf("list meters request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list meters: status %d: %s", resp.StatusCode, string(respBody))
	}

	var meters []Meter
	if err := json.NewDecoder(resp.Body).Decode(&meters); err != nil {
		return nil, fmt.Errorf("decode meters response: %w", err)
	}

	return meters, nil
}

// ValidateMeters checks that all required meters exist in OpenMeter.
// Returns a list of missing meter slugs, or an error if the API call fails.
func (c *Client) ValidateMeters(ctx context.Context) (missing []string, err error) {
	meters, err := c.ListMeters(ctx)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]bool, len(meters))
	for _, m := range meters {
		existing[m.Slug] = true
	}

	for _, slug := range RequiredMeters {
		if !existing[slug] {
			missing = append(missing, slug)
		}
	}

	return missing, nil
}

// EntitlementValue represents an OpenMeter entitlement value.
// Fields match the EntitlementValue schema from the OpenAPI spec.
// Metered entitlements populate Balance/Usage/Overage/TotalAvailableGrantAmount.
// Static entitlements populate Config (a JSON string, e.g. `{"limit": 10}`).
type EntitlementValue struct {
	HasAccess                 bool     `json:"hasAccess"`
	Balance                   *float64 `json:"balance,omitempty"`
	Usage                     *float64 `json:"usage,omitempty"`
	Overage                   *float64 `json:"overage,omitempty"`
	TotalAvailableGrantAmount *float64 `json:"totalAvailableGrantAmount,omitempty"`
	Config                    *string  `json:"config,omitempty"`
}

// CustomerAccess represents the response from GET /api/v1/customers/{id}/access.
// The map key is the feature key (e.g. "compute", "agents").
type CustomerAccess struct {
	Entitlements map[string]EntitlementValue `json:"entitlements"`
}

// GetCustomerAccess fetches all entitlements for a customer in a single call.
// Uses GET /api/v1/customers/{customerIdOrKey}/access.
// The customerKey is typically the account ID.
func (c *Client) GetCustomerAccess(ctx context.Context, customerKey string) (*CustomerAccess, error) {
	url := fmt.Sprintf("%s/api/v1/customers/%s/access", c.baseURL, customerKey)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return nil, fmt.Errorf("get customer access request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get customer access: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CustomerAccess
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode customer access response: %w", err)
	}

	return &result, nil
}

// CreateGrant creates an entitlement grant for a customer's feature.
// Uses POST /api/v1/customers/{customerKey}/entitlements/{featureKey}/grants.
func (c *Client) CreateGrant(ctx context.Context, customerKey, featureKey string, amount float64) error {
	url := fmt.Sprintf("%s/api/v1/customers/%s/entitlements/%s/grants", c.baseURL, customerKey, featureKey)

	payload := map[string]any{
		"amount":      amount,
		"effectiveAt": time.Now().UTC().Format(time.RFC3339),
		"priority":    1,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal grant: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return fmt.Errorf("create grant request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create grant: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// CreateSubscription subscribes a customer to a plan by plan key.
func (c *Client) CreateSubscription(ctx context.Context, customerID, planKey string) error {
	payload := map[string]any{
		"customerId": customerID,
		"plan":       map[string]string{"key": planKey},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal subscription: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/subscriptions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return fmt.Errorf("create subscription request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create subscription: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
