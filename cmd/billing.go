package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

// billingServerURLOverride is set in tests to redirect API calls to a test server.
var billingServerURLOverride string

func billingBaseURL() string {
	if billingServerURLOverride != "" {
		return strings.TrimSuffix(billingServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// Parity with the web UI's billing settings page:
//   - get:      plan, spend, credit, and the account's own spend controls
//   - status:   the gate, and the one action that lifts it
//   - invoices: the statements behind those numbers
//   - set:      the spend controls
//
// Every read prints dollars. The provider reports money as a decimal amount in
// the currency the response names, so nothing here converts.

var billingCmd = &cobra.Command{
	Use:   "billing",
	Short: "Inspect spend, credit, and billing status",
	Long:  "Read the active account's plan, spend, remaining credit, spend controls, and invoices.",
	Args:  cobra.NoArgs,
	RunE:  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
}

var billingGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get plan, spend, remaining credit, and spend controls",
	Args:  cobra.NoArgs,
	RunE:  runBillingGet,
}

var billingStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Get billing gating status for the active account",
	Args:  cobra.NoArgs,
	RunE:  runBillingStatus,
}

var billingInvoicesCmd = &cobra.Command{
	Use:   "invoices",
	Short: "List invoices for the active account",
	Args:  cobra.NoArgs,
	RunE:  runBillingInvoices,
}

var billingSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set the account's spend warning and limit",
	Long: `Set the account's own spend controls.

The warning notifies. The limit suspends the account's agents. Both are
measured against usage spend for the period, before credit is drawn down.

A control you do not name keeps its current value. Use --clear-warning or
--clear-limit to remove one.`,
	Args: cobra.NoArgs,
	RunE: runBillingSet,
}

func init() {
	billingCmd.AddCommand(billingGetCmd)
	billingCmd.AddCommand(billingStatusCmd)
	billingCmd.AddCommand(billingInvoicesCmd)
	billingCmd.AddCommand(billingSetCmd)

	billingGetCmd.Flags().Bool("json", false, "Print raw JSON output")
	billingStatusCmd.Flags().Bool("json", false, "Print raw JSON output")
	billingInvoicesCmd.Flags().Bool("json", false, "Print raw JSON output")
	billingInvoicesCmd.Flags().Int("limit", 12, "Number of invoices to show, newest first")

	billingSetCmd.Flags().Float64("warning", 0, "Spend amount that notifies")
	billingSetCmd.Flags().Float64("limit", 0, "Spend amount that suspends the account's agents")
	billingSetCmd.Flags().Bool("clear-warning", false, "Remove the spend warning")
	billingSetCmd.Flags().Bool("clear-limit", false, "Remove the spend limit")

	rootCmd.AddCommand(billingCmd)
}

// billingEnvelope wraps every billing read. Available is false when the account
// has no billing customer or the deployment runs without a provider, which is
// the OSS case and not an error.
type billingEnvelope struct {
	Available bool            `json:"available"`
	Data      json.RawMessage `json:"data,omitempty"`
}

type spendThreshold struct {
	Amount  float64 `json:"amount"`
	InAlarm bool    `json:"in_alarm"`
}

type billingSpend struct {
	Currency         string          `json:"currency,omitempty"`
	Plan             string          `json:"plan,omitempty"`
	CurrentSpend     float64         `json:"current_spend"`
	HasCurrentSpend  bool            `json:"has_current_spend"`
	CurrentPeriodEnd time.Time       `json:"current_period_end,omitempty"`
	UsageSpend       float64         `json:"usage_spend"`
	HasUsageSpend    bool            `json:"has_usage_spend"`
	CreditRemaining  float64         `json:"credit_remaining"`
	HasCredit        bool            `json:"has_credit"`
	Warning          *spendThreshold `json:"warning,omitempty"`
	Limit            *spendThreshold `json:"limit,omitempty"`
}

type billingStatus struct {
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	CreditsExhausted   bool   `json:"credits_exhausted"`
	HasPaymentMethod   bool   `json:"has_payment_method"`
	Enforced           bool   `json:"enforced"`
	WorkloadsSuspended bool   `json:"workloads_suspended"`
	Gated              bool   `json:"gated"`
	Action             string `json:"action,omitempty"`
}

// billingCreditType names the unit an invoice amount is denominated in.
// Invoices are a provider passthrough, unlike spend, so the amounts arrive
// unconverted and "USD (cents)" means the number is cents.
type billingCreditType struct {
	Name string `json:"name,omitempty"`
}

type billingInvoiceLineItem struct {
	Name       string             `json:"name,omitempty"`
	Quantity   float64            `json:"quantity,omitempty"`
	Total      float64            `json:"total,omitempty"`
	CreditType *billingCreditType `json:"credit_type,omitempty"`
}

type billingInvoice struct {
	ID             string                   `json:"id,omitempty"`
	Status         string                   `json:"status,omitempty"`
	Total          float64                  `json:"total,omitempty"`
	CreditType     *billingCreditType       `json:"credit_type,omitempty"`
	StartTimestamp string                   `json:"start_timestamp,omitempty"`
	EndTimestamp   string                   `json:"end_timestamp,omitempty"`
	LineItems      []billingInvoiceLineItem `json:"line_items,omitempty"`
}

func (c *billingCreditType) name() string {
	if c == nil {
		return ""
	}
	return c.Name
}

// billingRead fetches one billing endpoint for the active account. It returns
// available=false rather than an error when billing is not configured, so a
// caller can say so plainly instead of reporting a failure.
func billingRead(cmd *cobra.Command, at AccountToken, verbose bool, resource string, dest any) (bool, error) {
	u := apiPath(billingBaseURL(), at.Account, "accounts", "billing", resource)

	var env billingEnvelope
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &env); err != nil {
		return false, err
	}
	if !env.Available || len(env.Data) == 0 {
		return false, nil
	}
	if dest != nil {
		if err := json.Unmarshal(env.Data, dest); err != nil {
			return false, fmt.Errorf("failed to read billing %s: %w", resource, err)
		}
	}
	return true, nil
}

func runBillingGet(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var spend billingSpend
	available, err := billingRead(cmd, at, verbose, "spend", &spend)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if !available {
			return writeJSON(w, billingEnvelope{Available: false})
		}
		return writeJSON(w, spend)
	}
	if !available {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgBillingUnavailable(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	accent := color.New(theme.PrimaryFatihAttr)
	dim := color.New(color.Faint)
	cur := spend.Currency

	accent.Fprintf(w, "%s\n", at.Account) //nolint:errcheck,gosec
	if spend.Plan != "" {
		fmt.Fprintf(w, "  Plan:           %s\n", spend.Plan) //nolint:errcheck,gosec
	}
	if !spend.CurrentPeriodEnd.IsZero() {
		fmt.Fprintf(w, "  Period ends:    %s\n", spend.CurrentPeriodEnd.UTC().Format(time.RFC3339)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w) //nolint:errcheck,gosec

	// Usage spend is what the controls below measure, and it is the larger of
	// the two whenever credit covers part of the bill. Printing only the billed
	// amount would read as zero spend on an account burning through credit.
	if spend.HasUsageSpend {
		fmt.Fprintf(w, "  Usage spend:    %s\n", formatMoney(spend.UsageSpend, cur)) //nolint:errcheck,gosec
	}
	if spend.HasCurrentSpend {
		fmt.Fprintf(w, "  Billed spend:   %s\n", formatMoney(spend.CurrentSpend, cur)) //nolint:errcheck,gosec
	}
	if spend.HasCredit {
		fmt.Fprintf(w, "  Credit left:    %s\n", formatMoney(spend.CreditRemaining, cur)) //nolint:errcheck,gosec
	}
	fmt.Fprintln(w) //nolint:errcheck,gosec

	printSpendControl(cmd, "Warning", spend.Warning, cur)
	printSpendControl(cmd, "Limit", spend.Limit, cur)

	dim.Fprintf(w, "\nControls are measured against usage spend, before credit is drawn down.\n") //nolint:errcheck,gosec
	return nil
}

func printSpendControl(cmd *cobra.Command, label string, t *spendThreshold, currency string) {
	w := cmd.OutOrStdout()
	if t == nil {
		fmt.Fprintf(w, "  %-15s not set\n", label+":") //nolint:errcheck,gosec
		return
	}
	state := "ok"
	if t.InAlarm {
		state = "CROSSED"
	}
	fmt.Fprintf(w, "  %-15s %s (%s)\n", label+":", formatMoney(t.Amount, currency), state) //nolint:errcheck,gosec
}

func runBillingStatus(cmd *cobra.Command, _ []string) error {
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	// The gate reads a cached row rather than the provider, so it is not wrapped
	// in the availability envelope the other billing reads use.
	u := apiPath(billingBaseURL(), at.Account, "accounts", "billing", "status")
	var status billingStatus
	if _, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &status); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, status)
	}

	accent := color.New(theme.PrimaryFatihAttr)
	accent.Fprintf(w, "%s\n", at.Account) //nolint:errcheck,gosec

	state := status.Status
	if status.Gated {
		color.New(color.FgRed).Fprintf(w, "  Status:         %s\n", state) //nolint:errcheck,gosec
	} else {
		color.New(color.FgGreen).Fprintf(w, "  Status:         %s\n", state) //nolint:errcheck,gosec
	}
	if status.Reason != "" {
		fmt.Fprintf(w, "  Reason:         %s\n", status.Reason) //nolint:errcheck,gosec
	}
	if status.Action != "" {
		fmt.Fprintf(w, "  To resolve:     %s\n", status.Action) //nolint:errcheck,gosec
	}
	fmt.Fprintf(w, "  Payment method: %s\n", yesNo(status.HasPaymentMethod))   //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Credits spent:  %s\n", yesNo(status.CreditsExhausted))   //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Enforced:       %s\n", yesNo(status.Enforced))           //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Agents stopped: %s\n", yesNo(status.WorkloadsSuspended)) //nolint:errcheck,gosec
	return nil
}

func runBillingInvoices(cmd *cobra.Command, _ []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	if limit <= 0 {
		return errPositiveIntFlag("limit")
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var invoices []billingInvoice
	available, err := billingRead(cmd, at, verbose, "invoices", &invoices)
	if err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		if !available {
			return writeJSON(w, billingEnvelope{Available: false})
		}
		return writeJSON(w, invoices)
	}
	if !available {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgBillingUnavailable(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	if len(invoices) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgNoInvoices(), colorReset) //nolint:errcheck,gosec
		return nil
	}

	// Newest first: the provider's order is not guaranteed, and the invoice a
	// reader wants is almost always the open one.
	sort.SliceStable(invoices, func(i, j int) bool {
		return invoices[i].StartTimestamp > invoices[j].StartTimestamp
	})
	if len(invoices) > limit {
		invoices = invoices[:limit]
	}

	dim := color.New(color.Faint)
	for _, inv := range invoices {
		fmt.Fprintf(w, "%-10s %-12s %s\n",
			inv.Status, formatProviderAmount(inv.Total, inv.CreditType.name()), billingPeriod(inv)) //nolint:errcheck,gosec
		for _, li := range inv.LineItems {
			if li.Name == "" {
				continue
			}
			unit := li.CreditType.name()
			if unit == "" {
				unit = inv.CreditType.name()
			}
			dim.Fprintf(w, "    %-28s %12s\n", li.Name, formatProviderAmount(li.Total, unit)) //nolint:errcheck,gosec
		}
		if inv.ID != "" {
			dim.Fprintf(w, "    %s\n", inv.ID) //nolint:errcheck,gosec
		}
	}
	return nil
}

func billingPeriod(inv billingInvoice) string {
	start := shortDate(inv.StartTimestamp)
	end := shortDate(inv.EndTimestamp)
	switch {
	case start == "" && end == "":
		return ""
	case end == "":
		return start
	default:
		return start + " to " + end
	}
}

func shortDate(ts string) string {
	if ts == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t.UTC().Format("2006-01-02")
		}
	}
	return ts
}

func runBillingSet(cmd *cobra.Command, _ []string) error {
	warningSet := cmd.Flags().Changed("warning")
	limitSet := cmd.Flags().Changed("limit")
	clearWarning, _ := cmd.Flags().GetBool("clear-warning")
	clearLimit, _ := cmd.Flags().GetBool("clear-limit")

	if warningSet && clearWarning {
		return errBillingSetConflict("warning")
	}
	if limitSet && clearLimit {
		return errBillingSetConflict("limit")
	}
	// The endpoint is a PUT that replaces both controls, and a null clears one.
	// Sending nothing would therefore remove both, so refuse rather than treat
	// an empty invocation as "remove the account's spend limit".
	if !warningSet && !limitSet && !clearWarning && !clearLimit {
		return errBillingSetNoChange()
	}

	// Read first so a control the caller did not name keeps its value. Without
	// this, naming only the limit would silently drop the warning.
	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return err
	}
	var current billingSpend
	available, err := billingRead(cmd, at, verbose, "spend", &current)
	if err != nil {
		return err
	}
	if !available {
		return errBillingUnavailable()
	}

	body := struct {
		Warning *float64 `json:"warning"`
		Limit   *float64 `json:"limit"`
	}{
		Warning: existingThreshold(current.Warning),
		Limit:   existingThreshold(current.Limit),
	}
	if warningSet {
		v, _ := cmd.Flags().GetFloat64("warning")
		body.Warning = &v
	}
	if limitSet {
		v, _ := cmd.Flags().GetFloat64("limit")
		body.Limit = &v
	}
	if clearWarning {
		body.Warning = nil
	}
	if clearLimit {
		body.Limit = nil
	}

	u := apiPath(billingBaseURL(), at.Account, "accounts", "billing", "spend", "thresholds")
	if _, err := apiCall(cmd.Context(), http.MethodPut, u, body, at.Token, verbose, nil); err != nil {
		return err
	}

	w := cmd.OutOrStdout()
	color.New(color.FgGreen).Fprint(w, "✓ ")                                        //nolint:errcheck,gosec
	fmt.Fprintf(w, "%s\n", msgSpendControlsSaved())                                 //nolint:errcheck,gosec
	printSpendControl(cmd, "Warning", thresholdFor(body.Warning), current.Currency) //nolint:errcheck,gosec
	printSpendControl(cmd, "Limit", thresholdFor(body.Limit), current.Currency)     //nolint:errcheck,gosec
	return nil
}

func existingThreshold(t *spendThreshold) *float64 {
	if t == nil {
		return nil
	}
	v := t.Amount
	return &v
}

func thresholdFor(amount *float64) *spendThreshold {
	if amount == nil {
		return nil
	}
	return &spendThreshold{Amount: *amount}
}

// formatProviderAmount renders an amount in the credit type's own unit. The
// spend endpoint converts before it answers; invoices do not, so a cents credit
// type has to be scaled here or every invoice reads a hundred times too large.
func formatProviderAmount(value float64, creditType string) string {
	amount := value
	if strings.Contains(strings.ToLower(creditType), "cents") {
		amount /= 100
	}
	if creditType != "" && !strings.Contains(strings.ToLower(creditType), "usd") {
		return fmt.Sprintf("%.2f %s", amount, creditType)
	}
	return fmt.Sprintf("$%.2f", amount)
}

// formatMoney renders an amount the server already converted to the currency it
// names. The currency is appended only when it is something other than USD, so
// the common case reads as plain dollars.
func formatMoney(amount float64, currency string) string {
	s := fmt.Sprintf("$%.2f", amount)
	if c := strings.ToUpper(currency); c != "" && c != "USD" {
		return s + " " + c
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
