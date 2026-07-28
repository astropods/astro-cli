package notify

import (
	"context"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/novu"
)

// Recipient is a resolved audience member: a stable subscriber identity plus
// the email the email channel needs.
type Recipient struct {
	UserID string
	Email  string
}

// Provider delivers a resolved event to the notification backend.
type Provider interface {
	// Trigger fires one workflow to the given recipients. transactionID, when
	// non-empty, dedupes retries/redeliveries at the provider.
	Trigger(ctx context.Context, workflowID string, recipients []Recipient, payload map[string]any, transactionID string) error
}

// novuProvider adapts the Novu client to Provider.
type novuProvider struct {
	client *novu.Client
}

// NewNovuProvider wraps a configured Novu client.
func NewNovuProvider(client *novu.Client) Provider { return &novuProvider{client: client} }

func (p *novuProvider) Trigger(ctx context.Context, workflowID string, recipients []Recipient, payload map[string]any, transactionID string) error {
	subs := make([]novu.Subscriber, 0, len(recipients))
	for _, r := range recipients {
		subs = append(subs, novu.Subscriber{SubscriberID: r.UserID, Email: r.Email})
	}
	return p.client.Trigger(ctx, novu.TriggerRequest{
		WorkflowID:    workflowID,
		To:            subs,
		Payload:       payload,
		TransactionID: transactionID,
	})
}

// noopProvider logs and drops. Selected when Novu is unconfigured (OSS/local
// without NOVU_API_URL), so the seam and worker run without a backend.
type noopProvider struct {
	log *logger.Logger
}

// NewNoopProvider returns a Provider that logs the intended send and discards it.
func NewNoopProvider(log *logger.Logger) Provider { return &noopProvider{log: log} }

func (p *noopProvider) Trigger(_ context.Context, workflowID string, recipients []Recipient, _ map[string]any, transactionID string) error {
	if p.log != nil {
		p.log.Info("notify: no-op provider dropping trigger",
			"workflow", workflowID, "recipients", len(recipients), "transaction_id", transactionID)
	}
	return nil
}
