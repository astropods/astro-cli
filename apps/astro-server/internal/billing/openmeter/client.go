package openmeter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// DeleteCustomer deletes a customer in OpenMeter. Treats 404 as success (already gone).
func (c *Client) DeleteCustomer(ctx context.Context, customerID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/api/v1/customers/"+customerID, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config
	if err != nil {
		return fmt.Errorf("delete customer request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return nil // already gone
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete customer: status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// UpdateCustomerName updates the display name of an OpenMeter customer.
func (c *Client) UpdateCustomerName(ctx context.Context, customerID, name string) error {
	body, err := json.Marshal(map[string]string{"name": name})
	if err != nil {
		return fmt.Errorf("marshal customer: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/api/v1/customers/"+customerID, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req) //nolint:gosec
	if err != nil {
		return fmt.Errorf("update customer request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("update customer: not found")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("update customer: status %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
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
		return fmt.Errorf("ingest events: status %d: %s (url: %s)", resp.StatusCode, string(respBody), c.baseURL+"/api/v1/events")
	}

	return nil
}

// Meter represents an OpenMeter meter definition.
type Meter struct {
	Slug string `json:"slug"`
}

// RequiredMeters is the set of meter slugs that must exist in OpenMeter. Only
// metered-consumption meters remain; resource counts moved to the quota DB and
// are no longer emitted as meter events.
var RequiredMeters = []string{
	"compute",
	"knowledge_storage",
	"knowledge_compute",
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

// MeterQueryParams controls a meter value query.
type MeterQueryParams struct {
	Subject       string
	From          time.Time
	To            time.Time
	GroupBy       []string
	FilterGroupBy map[string]string
}

// MeterQueryRow is a single row from a meter value query response.
type MeterQueryRow struct {
	Subject     string            `json:"subject"`
	Value       float64           `json:"value"`
	GroupBy     map[string]string `json:"groupBy"`
	WindowStart string            `json:"windowStart"`
	WindowEnd   string            `json:"windowEnd"`
}

// MeterQueryResponse is the response from GET /api/v1/meters/{slug}/query.
type MeterQueryResponse struct {
	Data []MeterQueryRow `json:"data"`
	From string          `json:"from"`
	To   string          `json:"to"`
}

// QueryMeter queries aggregated meter values for a given slug and parameters.
func (c *Client) QueryMeter(ctx context.Context, slug string, params MeterQueryParams) (*MeterQueryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/meters/"+url.PathEscape(slug)+"/query", nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}

	q := req.URL.Query()
	if params.Subject != "" {
		q.Set("subject", params.Subject)
	}
	if !params.From.IsZero() {
		q.Set("from", params.From.UTC().Format(time.RFC3339))
	}
	if !params.To.IsZero() {
		q.Set("to", params.To.UTC().Format(time.RFC3339))
	}
	for _, g := range params.GroupBy {
		q.Add("groupBy", g)
	}
	for k, v := range params.FilterGroupBy {
		q.Set("filterGroupBy["+k+"]", v)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := c.httpClient.Do(req) //nolint:gosec // base URL is from trusted server config (OPENMETER_URL)
	if err != nil {
		return nil, fmt.Errorf("query meter request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("query meter: status %d: %s", resp.StatusCode, string(respBody))
	}

	var result MeterQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode meter query response: %w", err)
	}

	return &result, nil
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
