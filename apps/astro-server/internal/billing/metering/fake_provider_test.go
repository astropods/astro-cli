package metering

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// This file is a test-only fake billing.BillingProvider. The metering engine
// emits provider-agnostic billing.UsageEvent through a billing.BillingProvider;
// these helpers replay that over a CloudEvent-batch HTTP shape the metering
// tests assert on, so the test bodies (collectEvents / NewProvider(NewClient(url))
// / CloudEvent) can capture emitted events against an httptest server.

// CloudEvent mirrors the wire shape the tests capture and assert on.
type CloudEvent struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	SpecVersion string `json:"specversion"`
	Type        string `json:"type"`
	Subject     string `json:"subject"`
	Time        string `json:"time"`
	Data        any    `json:"data"`
}

// fakeClient posts CloudEvents to a test httptest server.
type fakeClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs the test client pointed at an httptest server URL.
func NewClient(baseURL string) *fakeClient {
	return &fakeClient{baseURL: baseURL, httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// fakeProvider adapts fakeClient to billing.BillingProvider. Only IngestUsage is
// meaningful; it maps each billing.UsageEvent to a CloudEvent for capture.
type fakeProvider struct{ client *fakeClient }

// NewProvider wraps the test client as a billing.BillingProvider.
func NewProvider(c *fakeClient) billing.BillingProvider { return &fakeProvider{client: c} }

func (p *fakeProvider) IngestUsage(ctx context.Context, events []billing.UsageEvent) error {
	ce := make([]CloudEvent, len(events))
	for i, ev := range events {
		id := ev.TransactionID
		if id == "" {
			id = uuid.New().String()
		}
		ce[i] = CloudEvent{
			ID:          id,
			Source:      "astro-server",
			SpecVersion: "1.0",
			Type:        ev.Type,
			Subject:     ev.AccountID,
			Time:        ev.Time.UTC().Format(time.RFC3339),
			Data:        ev.Properties,
		}
	}
	body, err := json.Marshal(ce)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.client.baseURL+"/api/v1/events", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/cloudevents-batch+json")
	resp, err := p.client.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ingest events: status %d", resp.StatusCode)
	}
	return nil
}

func (p *fakeProvider) CreateCustomer(context.Context, billing.Account) (string, error) {
	return "", nil
}
func (p *fakeProvider) DeleteCustomer(context.Context, string) error { return nil }
func (p *fakeProvider) SetIngestAliases(context.Context, string, []string) error {
	return nil
}
func (p *fakeProvider) GetIngestAliases(context.Context, string) ([]string, error) {
	return nil, nil
}
func (p *fakeProvider) UsageData(context.Context, string, time.Time, time.Time) (any, error) {
	return nil, billing.ErrBillingUnavailable
}
func (p *fakeProvider) DailySpend(context.Context, string, time.Time, time.Time) (any, error) {
	return nil, billing.ErrBillingUnavailable
}
func (p *fakeProvider) Invoices(context.Context, string) (any, error) {
	return nil, billing.ErrBillingUnavailable
}
func (p *fakeProvider) InvoicePDF(context.Context, string, string) (io.ReadCloser, error) {
	return nil, billing.ErrBillingUnavailable
}
func (p *fakeProvider) Balances(context.Context, string) (any, error) {
	return nil, billing.ErrBillingUnavailable
}
