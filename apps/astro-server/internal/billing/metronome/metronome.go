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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
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

// billingProviderStripe is Metronome's enum value for Stripe.
const billingProviderStripe = "stripe"

// Config holds the settings the Metronome provider needs.
type Config struct {
	APIKey string // METRONOME_API_KEY (bearer token)

	// Provisioning. When PackageID is empty, ProvisionCustomer is a no-op.
	//
	// Both packages share a rate card and a statement schedule and differ only
	// in the terms attached. Keeping them as packages rather than building a
	// bare rate-card contract is what makes the terms identical: contract
	// creation accepts no statement schedule, so a rate-card contract would bill
	// on a different period.
	PackageID         string // METRONOME_PACKAGE_ID
	PackageIDNoCredit string // METRONOME_PACKAGE_ID_NO_CREDIT
}

// Provider is the Metronome-backed billing provider.
type Provider struct {
	mc      metronome.Client
	cfg     Config
	metrics billableMetricIDs
}

// Compile-time assertions.
var (
	_ billing.BillingProvider      = (*Provider)(nil)
	_ billing.Provisioner          = (*Provider)(nil)
	_ billing.ContractInspector    = (*Provider)(nil)
	_ billing.PlanReporter         = (*Provider)(nil)
	_ billing.UsageThresholdReader = (*Provider)(nil)
	_ billing.UsageThresholdWriter = (*Provider)(nil)
	_ billing.SpendReporter        = (*Provider)(nil)
	_ billing.SpendThresholdReader = (*Provider)(nil)
	_ billing.SpendThresholdWriter = (*Provider)(nil)
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

// LinkStripeCustomer routes the customer's invoices to Stripe, where the vaulted
// card is charged. Both writes are required, and both are idempotent. The
// configuration names the delivery method that resolves the Stripe credential at
// send time. A configuration without one fails delivery with "No token found for
// environment type <env> and billing provider STRIPE". The contract must then
// reference that configuration. A contract provisioned from a package carries
// none, and keeps its invoices inside Metronome.
func (p *Provider) LinkStripeCustomer(ctx context.Context, metronomeCustomerID, stripeCustomerID string) error {
	configID, err := p.stripeConfiguration(ctx, metronomeCustomerID, stripeCustomerID)
	if err != nil {
		return err
	}
	return p.attachConfigurationToContracts(ctx, metronomeCustomerID, configID)
}

// stripeConfiguration returns the id of the customer's Stripe billing provider
// configuration, creating it when absent.
func (p *Provider) stripeConfiguration(ctx context.Context, metronomeCustomerID, stripeCustomerID string) (string, error) {
	existing, err := p.mc.V1.Customers.GetBillingConfigurations(ctx, metronome.V1CustomerGetBillingConfigurationsParams{
		CustomerID: metronomeCustomerID,
	})
	if err != nil {
		return "", fmt.Errorf("metronome get billing configurations: %w", err)
	}
	for _, cfg := range existing.Data {
		if cfg.BillingProvider == billingProviderStripe && cfg.Configuration["stripe_customer_id"] == stripeCustomerID {
			return cfg.ID, nil
		}
	}

	deliveryMethodID, err := p.stripeDeliveryMethodID(ctx)
	if err != nil {
		return "", err
	}
	created, err := p.mc.V1.Customers.SetBillingConfigurations(ctx, metronome.V1CustomerSetBillingConfigurationsParams{
		Data: []metronome.V1CustomerSetBillingConfigurationsParamsData{{
			CustomerID:       metronomeCustomerID,
			BillingProvider:  billingProviderStripe,
			DeliveryMethodID: param.NewOpt(deliveryMethodID),
			Configuration: map[string]any{
				"stripe_customer_id":       stripeCustomerID,
				"stripe_collection_method": "charge_automatically",
			},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("metronome set billing configuration: %w", err)
	}
	if len(created.Data) == 0 {
		return "", fmt.Errorf("metronome set billing configuration: empty response for customer %s", metronomeCustomerID)
	}
	return created.Data[0].ID, nil
}

// stripeDeliveryMethodID returns the environment's single Stripe delivery
// method. More than one means multi-entity billing, where which Stripe account
// to bill is a decision this path cannot make.
func (p *Provider) stripeDeliveryMethodID(ctx context.Context) (string, error) {
	providers, err := p.mc.V1.Settings.BillingProviders.List(ctx, metronome.V1SettingBillingProviderListParams{})
	if err != nil {
		return "", fmt.Errorf("metronome list billing providers: %w", err)
	}
	var ids []string
	for _, prov := range providers.Data {
		if prov.BillingProvider == billingProviderStripe {
			ids = append(ids, prov.DeliveryMethodID)
		}
	}
	switch len(ids) {
	case 1:
		return ids[0], nil
	case 0:
		return "", errors.New("metronome: no Stripe billing provider configured in this environment")
	default:
		return "", fmt.Errorf("metronome: %d Stripe billing providers configured, cannot choose one", len(ids))
	}
}

// attachConfigurationToContracts points the customer's covering contracts at the
// billing provider configuration. START_OF_CURRENT_PERIOD routes the open
// invoice rather than waiting a month for the next one.
func (p *Provider) attachConfigurationToContracts(ctx context.Context, metronomeCustomerID, configID string) error {
	contracts, err := p.coveringContracts(ctx, metronomeCustomerID, time.Now().UTC())
	if err != nil {
		return err
	}
	for _, contract := range contracts {
		if len(contract.BillingProviderConfigurationSchedule) > 0 {
			continue
		}
		_, err := p.mc.V2.Contracts.Edit(ctx, metronome.V2ContractEditParams{
			CustomerID: metronomeCustomerID,
			ContractID: contract.ID,
			AddBillingProviderConfigurationUpdate: metronome.V2ContractEditParamsAddBillingProviderConfigurationUpdate{
				BillingProviderConfiguration: metronome.V2ContractEditParamsAddBillingProviderConfigurationUpdateBillingProviderConfiguration{
					BillingProviderConfigurationID: param.NewOpt(configID),
				},
				Schedule: metronome.V2ContractEditParamsAddBillingProviderConfigurationUpdateSchedule{
					EffectiveAt: "START_OF_CURRENT_PERIOD",
				},
			},
		})
		if err != nil {
			return fmt.Errorf("metronome attach billing configuration to contract %s: %w", contract.ID, err)
		}
	}
	return nil
}

// ProvisionCustomer puts the customer on the configured package. The package
// carries the signup credit, so Metronome attaches it to the contract and no
// second write is needed. Reports false when no package is configured.
func (p *Provider) ProvisionCustomer(ctx context.Context, customerID, accountID string, plan billing.Plan) (bool, error) {
	packageID, err := p.provisionPackage(plan)
	if err != nil {
		return false, err
	}
	if packageID == "" {
		return false, nil
	}
	now := time.Now().UTC().Truncate(time.Hour)

	existing, cerr := p.coveringContracts(ctx, customerID, now)
	if cerr != nil {
		return false, cerr
	}
	// Any contract effective now already bills the customer, so a second would
	// bill them twice. The uniqueness key does not 409 against a contract made
	// by hand, which is why the list happens at all.
	if len(existing) > 0 {
		return true, nil
	}

	// PackageID is mutually exclusive with the rest of the contract fields:
	// only customer_id, starting_at, package_id, uniqueness_key, transition,
	// and custom_fields are accepted alongside it.
	_, err = p.mc.V1.Contracts.New(ctx, metronome.V1ContractNewParams{
		CustomerID:    customerID,
		StartingAt:    now,
		PackageID:     param.NewOpt(packageID),
		UniquenessKey: param.NewOpt(contractKey(accountID)),
	})
	if err != nil && !isConflict(err) {
		return false, fmt.Errorf("metronome create contract: %w", err)
	}
	return true, nil
}

// provisionPackage maps the plan to its package. A missing package is a
// configuration error rather than a reason to fall back: falling back would
// silently restore a grant, or silently bill an internal account, and the sweep
// re-runs the job once the configuration lands.
func (p *Provider) provisionPackage(plan billing.Plan) (string, error) {
	if p.cfg.PackageID == "" {
		return "", nil
	}
	switch plan {
	case billing.PlanCredit:
		return p.cfg.PackageID, nil
	default:
		if p.cfg.PackageIDNoCredit == "" {
			return "", errors.New("metronome: METRONOME_PACKAGE_ID_NO_CREDIT is unset, so an account that has already had its signup credit cannot be provisioned")
		}
		return p.cfg.PackageIDNoCredit, nil
	}
}

// coveringContracts lists the contracts effective at `at`. A contract made
// outside this path (by hand in the dashboard) carries no uniqueness key, so
// Contracts.New would not 409 against it and it has to be read first.
func (p *Provider) coveringContracts(ctx context.Context, customerID string, at time.Time) ([]shared.ContractV2, error) {
	page, err := p.mc.V2.Contracts.List(ctx, metronome.V2ContractListParams{
		CustomerID:   customerID,
		CoveringDate: param.NewOpt(at),
	})
	if err != nil {
		return nil, fmt.Errorf("metronome list contracts: %w", err)
	}
	return page.Data, nil
}

func (p *Provider) CustomerPlan(ctx context.Context, customerID string) (billing.Plan, bool, error) {
	contracts, err := p.coveringContracts(ctx, customerID, time.Now().UTC().Truncate(time.Hour))
	if err != nil {
		return "", false, err
	}
	for _, c := range contracts {
		if plan, ok := p.planForPackage(contractPackageID(c)); ok {
			return plan, true, nil
		}
	}
	return "", len(contracts) > 0, nil
}

func (p *Provider) planForPackage(packageID string) (billing.Plan, bool) {
	if packageID == "" {
		return "", false
	}
	switch packageID {
	case p.cfg.PackageID:
		return billing.PlanCredit, true
	case p.cfg.PackageIDNoCredit:
		return billing.PlanNoCredit, true
	}
	return "", false
}

func contractPackageID(c shared.ContractV2) string {
	var body struct {
		PackageID string `json:"package_id"`
	}
	if err := json.Unmarshal([]byte(c.RawJSON()), &body); err != nil {
		return ""
	}
	return body.PackageID
}

// ContractCoverage reports the verdict ProvisionCustomer acts on, for the admin
// view. Read-only: it never creates a contract.
func (p *Provider) ContractCoverage(ctx context.Context, customerID string) (billing.Coverage, error) {
	contracts, err := p.coveringContracts(ctx, customerID, time.Now().UTC().Truncate(time.Hour))
	if err != nil {
		return billing.Coverage{}, err
	}

	return coverageFrom(contracts), nil
}

// coverageFrom maps the contract list to the verdict. Covered is by existence:
// with one package, who created a contract changes nothing and is not reported.
func coverageFrom(contracts []shared.ContractV2) billing.Coverage {
	out := billing.Coverage{
		State:     billing.CoverageNone,
		Contracts: make([]billing.Contract, 0, len(contracts)),
	}
	if len(contracts) > 0 {
		out.State = billing.CoverageCovered
	}
	for _, c := range contracts {
		out.Contracts = append(out.Contracts, billing.Contract{
			ID:           c.ID,
			Name:         c.Name,
			RateCardID:   c.RateCardID,
			StartingAt:   c.StartingAt,
			EndingBefore: c.EndingBefore,
		})
	}
	return out
}

// contractKey is the uniqueness key every contract provisioning creates carries,
// derived here so the provisioning path and the admin check cannot disagree.
func contractKey(accountID string) string { return "contract:" + accountID }

// isConflict reports a 409, which a uniqueness key returns for a repeat write.
func isConflict(err error) bool {
	var apiErr *metronome.Error
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict
}

// IngestUsage sends usage events, chunked to the batch limit. The caller's
// TransactionID becomes the transaction_id, which Metronome dedupes on for 34
// days, so the ID has to name the span rather than the send.
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

// DailySpendPoint is one day's rated spend, summed across every usage-type
// line item in that day's invoice breakdown. Day is the breakdown's own
// window start, not derived from anything inside it. ByProduct breaks the
// same total down by line item name (for example "Compute Units" against
// "LLM Usage"), so a caller that needs the Compute/Models split reads it
// straight from the rated breakdown instead of approximating it against a
// raw usage quantity.
type DailySpendPoint struct {
	Day       time.Time          `json:"day"`
	Amount    float64            `json:"amount"`
	ByProduct map[string]float64 `json:"by_product,omitempty"`
}

// invoiceStatusVoid marks an invoice that has been superseded. Its breakdown
// still carries a full day's line items, so counting it alongside its
// replacement would double that day's total.
const invoiceStatusVoid = "VOID"

// DailySpend returns the account's rated spend per calendar day over
// [from, to), read from Metronome's invoice breakdown rather than the raw
// usage list: usage-list never rates a quantity metric like Compute Units,
// at any window size, so it can't answer "how many dollars did the account
// spend on Tuesday" the way a breakdown of the invoice itself can, since
// each breakdown window carries that window's own line items with real
// dollar totals. Reuses usageSpend's usage-vs-credit-drawdown split, so a
// day's figure means the same thing here as it does in the period total.
//
// A day can be covered by more than one invoice: a finalized one for a past
// period abutting a draft one for the current period, or a correction that
// supersedes an earlier invoice. A voided invoice is dropped outright, and
// when two non-void invoices still cover the same day, the one issued last
// wins rather than being summed with the other, which would double the day.
func (p *Provider) DailySpend(ctx context.Context, customerID string, from, to time.Time) (any, error) {
	type dayAgg struct {
		amount    float64
		byProduct map[string]float64
		issuedAt  time.Time
	}
	byDay := map[time.Time]dayAgg{}

	iter := p.mc.V1.Customers.Invoices.ListBreakdownsAutoPaging(ctx, metronome.V1CustomerInvoiceListBreakdownsParams{
		CustomerID:   customerID,
		StartingOn:   from,
		EndingBefore: to,
		WindowSize:   metronome.V1CustomerInvoiceListBreakdownsParamsWindowSizeDay,
	})
	for iter.Next() {
		bd := iter.Current()
		if bd.Status == invoiceStatusVoid {
			continue
		}
		day := bd.BreakdownStartTimestamp
		if existing, ok := byDay[day]; ok && !bd.IssuedAt.After(existing.issuedAt) {
			continue
		}
		amount, _ := usageSpend(&bd.Invoice)
		byDay[day] = dayAgg{amount: amount, byProduct: usageSpendByProduct(&bd.Invoice), issuedAt: bd.IssuedAt}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("metronome list invoice breakdowns: %w", err)
	}

	days := make([]time.Time, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })

	points := make([]DailySpendPoint, 0, len(days))
	for _, day := range days {
		agg := byDay[day]
		points = append(points, DailySpendPoint{Day: day, Amount: agg.amount, ByProduct: agg.byProduct})
	}
	return points, nil
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
		// No stored PDF: a draft, or a finalized invoice Metronome never rendered.
		var apiErr *metronome.Error
		if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound {
			return nil, billing.ErrInvoiceNotAvailable
		}
		return nil, fmt.Errorf("metronome get invoice pdf: %w", err)
	}
	return resp.Body, nil
}
