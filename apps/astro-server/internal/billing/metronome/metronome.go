// Package metronome is the hosted billing provider backed by the Metronome API.
// It satisfies billing.BillingProvider so the rest of astro-server depends only
// on the interface.
//
// This provider is introduced "dark" (compiled and selectable via
// BILLING_PROVIDER=metronome, but not enabled in production). The seam is
// metering-only: customer lifecycle plus usage ingest. Balances, spend gating,
// credit grants, and usage/cost queries are handled out-of-band (Metronome
// dashboard), not through this provider.
package metronome

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/option"
	"github.com/Metronome-Industries/metronome-go/v3/packages/param"
	"github.com/Metronome-Industries/metronome-go/v3/shared"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// ingestBatchLimit is the max usage events per ingest request. Metronome's
// documented limit is ~100; chunk to stay under it.
const ingestBatchLimit = 100

// Config holds the settings the Metronome provider needs.
type Config struct {
	APIKey string // METRONOME_API_KEY (bearer token)

	// Provisioning. When PackageID is empty, ProvisionCustomer is a no-op.
	PackageID        string  // METRONOME_PACKAGE_ID
	CreditTypeID     string  // METRONOME_CREDIT_TYPE_ID — pricing unit
	SignupCredit     float64 // METRONOME_SIGNUP_CREDIT — in the credit type's own unit
	CreditExpiryDays int     // METRONOME_CREDIT_EXPIRY_DAYS
}

// Provider is the Metronome-backed billing provider.
type Provider struct {
	mc  metronome.Client
	cfg Config
}

// Compile-time assertions.
var (
	_ billing.BillingProvider = (*Provider)(nil)
	_ billing.Provisioner     = (*Provider)(nil)
)

// New constructs a Metronome provider. Returns nil when no API key is set so
// callers' `provider == nil` guards behave like the other backends.
func New(cfg Config) *Provider {
	if cfg.APIKey == "" {
		return nil
	}
	return &Provider{
		mc:  metronome.NewClient(option.WithBearerToken(cfg.APIKey)),
		cfg: cfg,
	}
}

// CreateCustomer creates a Metronome customer keyed on the Astro account ID as
// an ingest alias, so usage events attribute by account ID.
//
// Customer creation takes no uniqueness key, so unlike the contract and grant
// it cannot lean on a 409. It resolves the alias first instead: if the caller
// created a customer and then failed to persist the id, the retry adopts it
// rather than creating a second customer and splitting the account's usage.
func (p *Provider) CreateCustomer(ctx context.Context, a billing.Account) (string, error) {
	if existing, err := p.customerByAlias(ctx, a.ID); err != nil {
		return "", err
	} else if existing != "" {
		return existing, nil
	}
	aliases := []string{a.ID}
	if a.BifrostCustomerID != "" {
		aliases = append(aliases, a.BifrostCustomerID)
	}
	resp, err := p.mc.V1.Customers.New(ctx, metronome.V1CustomerNewParams{
		Name:          a.Name,
		IngestAliases: aliases,
	})
	if err != nil {
		return "", fmt.Errorf("metronome create customer: %w", err)
	}
	return resp.Data.ID, nil
}

// customerByAlias returns the customer carrying the ingest alias, or "".
func (p *Provider) customerByAlias(ctx context.Context, alias string) (string, error) {
	page, err := p.mc.V1.Customers.List(ctx, metronome.V1CustomerListParams{
		IngestAlias: param.NewOpt(alias),
		Limit:       param.NewOpt(int64(1)),
	})
	if err != nil {
		return "", fmt.Errorf("metronome find customer by alias: %w", err)
	}
	if len(page.Data) == 0 {
		return "", nil
	}
	return page.Data[0].ID, nil
}

// SetIngestAliases replaces the customer's ingest aliases.
func (p *Provider) SetIngestAliases(ctx context.Context, customerID string, aliases []string) error {
	err := p.mc.V1.Customers.SetIngestAliases(ctx, metronome.V1CustomerSetIngestAliasesParams{
		CustomerID:    customerID,
		IngestAliases: aliases,
	})
	if err != nil {
		return fmt.Errorf("metronome set ingest aliases: %w", err)
	}
	return nil
}

// GetIngestAliases returns the customer's current ingest aliases.
func (p *Provider) GetIngestAliases(ctx context.Context, customerID string) ([]string, error) {
	resp, err := p.mc.V1.Customers.Get(ctx, metronome.V1CustomerGetParams{CustomerID: customerID})
	if err != nil {
		return nil, fmt.Errorf("metronome get customer: %w", err)
	}
	return resp.Data.IngestAliases, nil
}

// DeleteCustomer archives the customer (Metronome has no hard delete).
func (p *Provider) DeleteCustomer(ctx context.Context, customerID string) error {
	_, err := p.mc.V1.Customers.Archive(ctx, metronome.V1CustomerArchiveParams{
		ID: shared.IDParam{ID: customerID},
	})
	if err != nil {
		return fmt.Errorf("metronome archive customer: %w", err)
	}
	return nil
}

// LinkStripeCustomer points the Metronome customer's billing config at a Stripe
// customer with automatic charging, so Metronome pushes finalized invoices to
// Stripe and charges the saved card. astro-server only vaults the card (via
// SetupIntent); Metronome does the charging. Idempotent — re-linking the same
// Stripe customer is a no-op update.
func (p *Provider) LinkStripeCustomer(ctx context.Context, metronomeCustomerID, stripeCustomerID string) error {
	err := p.mc.V1.Customers.BillingConfig.New(ctx, metronome.V1CustomerBillingConfigNewParams{
		CustomerID:                metronomeCustomerID,
		BillingProviderType:       metronome.V1CustomerBillingConfigNewParamsBillingProviderTypeStripe,
		BillingProviderCustomerID: stripeCustomerID,
		StripeCollectionMethod:    metronome.V1CustomerBillingConfigNewParamsStripeCollectionMethodChargeAutomatically,
	})
	if err != nil {
		return fmt.Errorf("metronome link stripe customer: %w", err)
	}
	return nil
}

// ProvisionCustomer puts the customer on the configured package and grants its
// signup credit. Both writes carry a uniqueness key derived from the account
// ID, so Metronome rejects a repeat with 409 and this treats that as already
// provisioned. Reports false when no package is configured.
func (p *Provider) ProvisionCustomer(ctx context.Context, customerID, accountID string) (bool, error) {
	if p.cfg.PackageID == "" {
		return false, nil
	}
	now := time.Now().UTC().Truncate(time.Hour)

	// A contract made outside this path (by hand in the dashboard) carries no
	// uniqueness key, so Contracts.New would not 409 against it. covering_date
	// filters to contracts effective now.
	existing, err := p.mc.V1.Contracts.List(ctx, metronome.V1ContractListParams{
		CustomerID:   customerID,
		CoveringDate: param.NewOpt(now),
	})
	if err != nil {
		return false, fmt.Errorf("metronome list contracts: %w", err)
	}

	contractKey := "contract:" + accountID
	switch cov, foreign := classifyCoverage(existing.Data, p.cfg.PackageID, contractKey); cov {
	case coverageOurs:
		// Already on the right package; the grant below is separately keyed.
	case coverageNone:
		// PackageID is mutually exclusive with the rest of the contract fields:
		// only customer_id, starting_at, package_id, uniqueness_key, transition,
		// and custom_fields are accepted alongside it.
		_, err = p.mc.V1.Contracts.New(ctx, metronome.V1ContractNewParams{
			CustomerID:    customerID,
			StartingAt:    now,
			PackageID:     param.NewOpt(p.cfg.PackageID),
			UniquenessKey: param.NewOpt(contractKey),
		})
		if err != nil && !isConflict(err) {
			return false, fmt.Errorf("metronome create contract: %w", err)
		}
	case coverageForeign:
		// Adding ours would stack a second contract; skipping would silently
		// bill the account on someone else's rates. Neither is ours to choose,
		// so archive the stray contract in Metronome. Not retryable, so the
		// caller cancels and the hourly sweep re-checks once rather than the
		// job burning its whole backoff schedule every tick.
		return false, fmt.Errorf("%w: customer %s is covered by a contract on package %q, want %s",
			billing.ErrProvisionBlocked, customerID, foreign, p.cfg.PackageID)
	}

	if p.cfg.SignupCredit <= 0 {
		return true, nil
	}
	expiry := now.AddDate(0, 0, p.cfg.CreditExpiryDays)
	_, err = p.mc.V1.CreditGrants.New(ctx, metronome.V1CreditGrantNewParams{
		CustomerID:    customerID,
		Name:          "Signup credit",
		ExpiresAt:     expiry,
		Priority:      1,
		GrantAmount:   metronome.V1CreditGrantNewParamsGrantAmount{Amount: p.cfg.SignupCredit, CreditTypeID: p.cfg.CreditTypeID},
		PaidAmount:    metronome.V1CreditGrantNewParamsPaidAmount{Amount: 0, CreditTypeID: p.cfg.CreditTypeID},
		UniquenessKey: param.NewOpt("signup-credit:" + accountID),
	})
	if err != nil && !isConflict(err) {
		return false, fmt.Errorf("metronome create credit grant: %w", err)
	}
	return true, nil
}

// coverage classifies the contracts already covering a customer.
type coverage int

const (
	coverageNone    coverage = iota // nothing covers the customer; create ours
	coverageOurs                    // already on our package; leave it alone
	coverageForeign                 // on someone else's package
)

// classifyCoverage scans every covering contract rather than trusting list
// order. Ours anywhere wins, since a second contract on the same package would
// double-bill; otherwise any foreign package is reported so the caller can
// refuse. foreignPkg is set only for coverageForeign.
//
// A contract is ours by package or by the uniqueness key we created it with.
// The key matters because it does not depend on the list response populating
// package_id: without it, a contract we made would read as foreign on any
// re-check — say a retry after the credit grant failed — and provisioning for
// that account would be cancelled for good.
func classifyCoverage(contracts []shared.Contract, wantPackage, wantKey string) (cov coverage, foreignPkg string) {
	for _, c := range contracts {
		if c.PackageID == wantPackage || (wantKey != "" && c.UniquenessKey == wantKey) {
			return coverageOurs, ""
		}
	}
	for _, c := range contracts {
		if c.PackageID != "" {
			return coverageForeign, c.PackageID
		}
	}
	// A packageless contract still covers the customer, so creating ours would
	// stack a second one. Treat it as foreign rather than as absent.
	if len(contracts) > 0 {
		return coverageForeign, ""
	}
	return coverageNone, ""
}

// isConflict reports a 409, which a uniqueness key returns for a repeat write.
func isConflict(err error) bool {
	var apiErr *metronome.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

// IngestUsage sends usage events, chunked to the batch limit. The event UUID is
// the transaction_id (34-day dedupe window), preserving idempotency.
func (p *Provider) IngestUsage(ctx context.Context, events []billing.UsageEvent) error {
	for start := 0; start < len(events); start += ingestBatchLimit {
		end := min(start+ingestBatchLimit, len(events))
		batch := events[start:end]
		usage := make([]metronome.V1UsageIngestParamsUsage, len(batch))
		for i, ev := range batch {
			ts := ev.Time
			if ts.IsZero() {
				ts = time.Now()
			}
			usage[i] = metronome.V1UsageIngestParamsUsage{
				TransactionID: ev.TransactionID,
				CustomerID:    ev.AccountID,
				EventType:     ev.Type,
				Timestamp:     ts.UTC().Format(time.RFC3339),
				Properties:    ev.Properties,
			}
		}
		if err := p.mc.V1.Usage.Ingest(ctx, metronome.V1UsageIngestParams{Usage: usage}); err != nil {
			return fmt.Errorf("metronome ingest usage: %w", err)
		}
	}
	return nil
}

// UsageData returns metered usage over [from, to) aggregated into daily windows,
// for all of the customer's billable metrics. The raw Metronome rows are passed
// through for the client to render.
func (p *Provider) UsageData(ctx context.Context, customerID string, from, to time.Time) (any, error) {
	rows := []metronome.V1UsageListResponse{}
	iter := p.mc.V1.Usage.ListAutoPaging(ctx, metronome.V1UsageListParams{
		StartingOn:   from,
		EndingBefore: to,
		WindowSize:   metronome.V1UsageListParamsWindowSizeDay,
		CustomerIDs:  []string{customerID},
	})
	for iter.Next() {
		rows = append(rows, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("metronome list usage: %w", err)
	}
	return rows, nil
}

// Invoices returns the customer's invoices (with line items), passed through
// as-is for the client to render.
func (p *Provider) Invoices(ctx context.Context, customerID string) (any, error) {
	invoices := []metronome.Invoice{}
	iter := p.mc.V1.Customers.Invoices.ListAutoPaging(ctx, metronome.V1CustomerInvoiceListParams{
		CustomerID: customerID,
	})
	for iter.Next() {
		invoices = append(invoices, iter.Current())
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("metronome list invoices: %w", err)
	}
	return invoices, nil
}

// InvoicePDF fetches a single invoice's PDF as a byte stream. The caller must
// close the returned reader.
func (p *Provider) InvoicePDF(ctx context.Context, customerID, invoiceID string) (io.ReadCloser, error) {
	resp, err := p.mc.V1.Customers.Invoices.GetPdf(ctx, metronome.V1CustomerInvoiceGetPdfParams{
		CustomerID: customerID,
		InvoiceID:  invoiceID,
	})
	if err != nil {
		// Draft/non-finalized invoices have no PDF yet → treat as not available.
		var apiErr *metronome.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, billing.ErrInvoiceNotAvailable
		}
		return nil, fmt.Errorf("metronome get invoice pdf: %w", err)
	}
	return resp.Body, nil
}

// Balances returns the customer's credits and commits, passed through as-is.
func (p *Provider) Balances(ctx context.Context, customerID string) (any, error) {
	credits := []shared.Credit{}
	creditIter := p.mc.V1.Customers.Credits.ListAutoPaging(ctx, metronome.V1CustomerCreditListParams{
		CustomerID: customerID,
	})
	for creditIter.Next() {
		credits = append(credits, creditIter.Current())
	}
	if err := creditIter.Err(); err != nil {
		return nil, fmt.Errorf("metronome list credits: %w", err)
	}

	commits := []shared.Commit{}
	commitIter := p.mc.V1.Customers.Commits.ListAutoPaging(ctx, metronome.V1CustomerCommitListParams{
		CustomerID: customerID,
	})
	for commitIter.Next() {
		commits = append(commits, commitIter.Current())
	}
	if err := commitIter.Err(); err != nil {
		return nil, fmt.Errorf("metronome list commits: %w", err)
	}

	return map[string]any{"credits": credits, "commits": commits}, nil
}
