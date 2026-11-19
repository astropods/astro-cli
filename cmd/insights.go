package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/astropods/astro-cli/internal/auth"
	"github.com/astropods/astro-cli/internal/buildinfo"
	"github.com/astropods/astro-cli/internal/theme"
)

// insightsServerURLOverride is set in tests to redirect API calls to a test server.
var insightsServerURLOverride string

func insightsBaseURL() string {
	if insightsServerURLOverride != "" {
		return strings.TrimSuffix(insightsServerURLOverride, "/")
	}
	return strings.TrimSuffix(buildinfo.DefaultServerURL, "/")
}

// The ranges the Insights page offers. The server serves no others.
var insightsDays = []int{7, 14, 30, 90}

const (
	insightsDefaultDays  = 30
	insightsDefaultLimit = 25
	insightsMaxLimit     = 100
)

var insightsCmd = &cobra.Command{
	Use:   "insights",
	Short: "Read the account's AI spend, as on the Insights page",
	Long: fmt.Sprintf(`Read the account's AI spend and token use, as on the Insights page.

Every command covers the last --days complete days (7, 14, 30, or 90) and reads
only. '%s mcp' serves each one as a tool.`, buildinfo.BinaryName),
}

var insightsSummaryCmd = &cobra.Command{
	Use:   "summary",
	Short: "Show total spend, requests, and tokens, with the change from the period before",
	Args:  cobra.NoArgs,
	RunE:  runInsightsSummary,
}

var insightsDailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "Show each day's spend for the five agents that spent the most, as on the Insights chart",
	Args:  cobra.NoArgs,
	RunE:  runInsightsDaily,
}

var insightsAgentsCmd = &cobra.Command{
	Use:   "agents",
	Short: "List agents and dev tools by spend",
	Args:  cobra.NoArgs,
	RunE:  runInsightsAgents,
}

var insightsPeopleCmd = &cobra.Command{
	Use:   "people",
	Short: "List people by spend",
	Args:  cobra.NoArgs,
	RunE:  runInsightsPeople,
}

func init() {
	rootCmd.AddCommand(insightsCmd)
	for _, c := range []*cobra.Command{insightsSummaryCmd, insightsDailyCmd, insightsAgentsCmd, insightsPeopleCmd} {
		insightsCmd.AddCommand(c)
		c.Flags().Int("days", insightsDefaultDays, "Complete days to cover: 7, 14, 30, or 90")
		c.Flags().Bool("json", false, "Print raw JSON output")
	}
	for _, c := range []*cobra.Command{insightsAgentsCmd, insightsPeopleCmd} {
		c.Flags().Int("limit", insightsDefaultLimit, fmt.Sprintf("Rows to show, at most %d", insightsMaxLimit))
		c.Flags().Int("offset", 0, "Rows to skip, to read the next page")
	}
}

type insightsPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
	Days  int    `json:"days"`
}

type insightsTotals struct {
	CostUSD      float64 `json:"cost_usd"`
	Requests     int     `json:"requests"`
	TotalTokens  int     `json:"total_tokens"`
	ActiveAgents int     `json:"active_agents"`
}

type insightsChange struct {
	CostPct     *float64 `json:"cost_pct"`
	RequestsPct *float64 `json:"requests_pct"`
	TokensPct   *float64 `json:"tokens_pct"`
}

type insightsSeriesPoint struct {
	Model   string  `json:"model"`
	CostUSD float64 `json:"cost_usd"`
}

type insightsRange struct {
	Period    insightsPeriod `json:"period"`
	StatCards struct {
		Totals insightsTotals  `json:"totals"`
		Change *insightsChange `json:"change"`
	} `json:"stat_cards"`
	AgentSpendChart []struct {
		Date   string                `json:"date"`
		Models []insightsSeriesPoint `json:"models"`
	} `json:"agent_spend_chart"`
	SeriesLabels map[string]string `json:"series_labels"`
}

type insightsIdentity struct {
	Kind      string `json:"kind"`
	ID        string `json:"id"`
	Label     string `json:"label"`
	IsDeleted bool   `json:"is_deleted"`
}

type insightsPagination struct {
	Offset  int  `json:"offset"`
	HasMore bool `json:"has_more"`
}

type insightsResponse struct {
	Ranges map[string]insightsRange `json:"ranges"`
	AsOf   string                   `json:"as_of"`
	Tables struct {
		Agents struct {
			Rows []struct {
				Identity insightsIdentity `json:"identity"`
				Metrics  struct {
					Requests       int     `json:"requests"`
					CostUSD        float64 `json:"cost_usd"`
					CostPct        float64 `json:"cost_pct"`
					CostPerRequest float64 `json:"cost_per_request"`
					TokPerRequest  float64 `json:"tok_per_request"`
					LastSeen       string  `json:"last_seen"`
				} `json:"metrics"`
			} `json:"rows"`
			TotalCost  float64            `json:"total_cost"`
			Count      int                `json:"count"`
			Pagination insightsPagination `json:"pagination"`
		} `json:"agents"`
		People struct {
			Rows []struct {
				Identity   insightsIdentity `json:"identity"`
				AgentsUsed []struct {
					Label string `json:"label"`
				} `json:"agents_used"`
				Metrics struct {
					Requests int     `json:"requests"`
					CostUSD  float64 `json:"cost_usd"`
					CostPct  float64 `json:"cost_pct"`
					Tokens   int     `json:"tokens"`
					LastSeen string  `json:"last_seen"`
				} `json:"metrics"`
			} `json:"rows"`
			TotalCost  float64            `json:"total_cost"`
			Count      int                `json:"count"`
			Pagination insightsPagination `json:"pagination"`
		} `json:"people"`
	} `json:"tables"`
}

// insightsWindow names the account and the days a result covers.
type insightsWindow struct {
	Account string `json:"account"`
	Days    int    `json:"days"`
	From    string `json:"from"`
	To      string `json:"to"`
}

type insightsSummaryOutput struct {
	insightsWindow
	insightsTotals
	CostChangePct     *float64 `json:"cost_change_pct,omitempty"`
	RequestsChangePct *float64 `json:"requests_change_pct,omitempty"`
	TokensChangePct   *float64 `json:"tokens_change_pct,omitempty"`
}

type insightsAgentSpend struct {
	Name    string  `json:"name"`
	CostUSD float64 `json:"cost_usd"`
}

type insightsDay struct {
	Date   string               `json:"date"`
	Agents []insightsAgentSpend `json:"agents"`
}

type insightsDailyOutput struct {
	insightsWindow
	Daily []insightsDay `json:"daily"`
}

type insightsPage struct {
	TotalCostUSD float64 `json:"total_cost_usd"`
	Count        int     `json:"count"`
	Offset       int     `json:"offset"`
	HasMore      bool    `json:"has_more"`
}

type insightsAgentRow struct {
	Name           string  `json:"name"`
	Kind           string  `json:"kind"`
	ID             string  `json:"id,omitempty"`
	CostUSD        float64 `json:"cost_usd"`
	CostPct        float64 `json:"cost_pct"`
	Requests       int     `json:"requests"`
	CostPerRequest float64 `json:"cost_per_request_usd"`
	TokPerRequest  float64 `json:"tokens_per_request"`
	LastSeen       string  `json:"last_seen,omitempty"`
	Deleted        bool    `json:"deleted,omitempty"`
}

type insightsAgentsOutput struct {
	insightsWindow
	insightsPage
	Agents []insightsAgentRow `json:"agents"`
}

type insightsPersonRow struct {
	Name       string   `json:"name"`
	Kind       string   `json:"kind"`
	CostUSD    float64  `json:"cost_usd"`
	CostPct    float64  `json:"cost_pct"`
	Requests   int      `json:"requests"`
	Tokens     int      `json:"tokens"`
	LastSeen   string   `json:"last_seen,omitempty"`
	AgentsUsed []string `json:"agents_used,omitempty"`
}

type insightsPeopleOutput struct {
	insightsWindow
	insightsPage
	People []insightsPersonRow `json:"people"`
}

// fetchInsights reads the Insights page model for the active account, with the
// tables scoped to --days. table names the table whose --limit and --offset
// apply; the others keep the server's defaults. With no table, the response must
// carry the --days range.
func fetchInsights(cmd *cobra.Command, table string) (AccountToken, insightsResponse, insightsRange, insightsWindow, error) {
	var resp insightsResponse
	days, _ := cmd.Flags().GetInt("days")
	if !slices.Contains(insightsDays, days) {
		return AccountToken{}, resp, insightsRange{}, insightsWindow{}, errInsightsDays(days)
	}
	query := url.Values{"days": {strconv.Itoa(days)}}
	if table != "" {
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		if limit < 1 || limit > insightsMaxLimit {
			return AccountToken{}, resp, insightsRange{}, insightsWindow{}, errInsightsLimit(insightsMaxLimit)
		}
		if offset < 0 {
			return AccountToken{}, resp, insightsRange{}, insightsWindow{}, errInsightsOffset()
		}
		query.Set(table+"_limit", strconv.Itoa(limit))
		query.Set(table+"_offset", strconv.Itoa(offset))
	}

	at, verbose, err := cmdAuth(cmd)
	if err != nil {
		return AccountToken{}, resp, insightsRange{}, insightsWindow{}, err
	}
	u := apiPath(insightsBaseURL(), url.PathEscape(at.Account), "accounts", "insights") + "?" + query.Encode()
	status, err := apiCall(cmd.Context(), http.MethodGet, u, nil, at.Token, verbose, &resp)
	if status == http.StatusForbidden && !insightsRefusalExplained(err) {
		return at, resp, insightsRange{}, insightsWindow{}, errInsightsForbidden(at.Account)
	}
	if err != nil {
		return at, resp, insightsRange{}, insightsWindow{}, err
	}
	rng, ok := resp.Ranges[strconv.Itoa(days)+"d"]
	// summary and daily read their numbers from the range. agents and people only
	// take its dates for the header, so they go on without them.
	if !ok && table == "" {
		return at, resp, insightsRange{}, insightsWindow{}, errInsightsNoRange(days)
	}
	window := insightsWindow{
		Account: at.Account,
		Days:    days,
		From:    insightsDate(rng.Period.Start),
		To:      insightsDate(rng.Period.End),
	}
	return at, resp, rng, window, nil
}

// insightsRefusalExplained reports a 403 that explainError already words better
// than errInsightsForbidden: a coded refusal such as AUTHORIZATION_DENIED or
// BILLING_SUSPENDED, which carries the server's sentence, or any refusal of a
// machine credential.
func insightsRefusalExplained(err error) bool {
	if auth.UsesMachineCredentials() {
		return true
	}
	var apiErr *apiError
	return errors.As(err, &apiErr) && apiErr.isStructured()
}

func insightsDate(timestamp string) string {
	if len(timestamp) >= len("2006-01-02") {
		return timestamp[:len("2006-01-02")]
	}
	return timestamp
}

func insightsHeader(cmd *cobra.Command, window insightsWindow) {
	w := cmd.OutOrStdout()
	color.New(theme.PrimaryFatihAttr).Fprintf(w, "%s", window.Account)                                         //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %s%s%s\n", colorDim, msgInsightsWindow(window.Days, window.From, window.To), colorReset) //nolint:errcheck,gosec
}

func runInsightsSummary(cmd *cobra.Command, _ []string) error {
	_, _, rng, window, err := fetchInsights(cmd, "")
	if err != nil {
		return err
	}
	out := insightsSummaryOutput{insightsWindow: window, insightsTotals: rng.StatCards.Totals}
	if change := rng.StatCards.Change; change != nil {
		out.CostChangePct, out.RequestsChangePct, out.TokensChangePct = change.CostPct, change.RequestsPct, change.TokensPct
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, out)
	}
	insightsHeader(cmd, window)
	fmt.Fprintf(w, "  Spend:     %s%s\n", msgUsageDollars(out.CostUSD), msgInsightsChange(out.CostChangePct)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Requests:  %s%s\n", thousands(out.Requests), msgInsightsChange(out.RequestsChangePct))  //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Tokens:    %s%s\n", thousands(out.TotalTokens), msgInsightsChange(out.TokensChangePct)) //nolint:errcheck,gosec
	fmt.Fprintf(w, "  Agents:    %d\n", out.ActiveAgents)                                                     //nolint:errcheck,gosec
	return nil
}

func runInsightsDaily(cmd *cobra.Command, _ []string) error {
	_, _, rng, window, err := fetchInsights(cmd, "")
	if err != nil {
		return err
	}
	out := insightsDailyOutput{insightsWindow: window, Daily: make([]insightsDay, 0, len(rng.AgentSpendChart))}
	for _, point := range rng.AgentSpendChart {
		day := insightsDay{Date: point.Date, Agents: make([]insightsAgentSpend, 0, len(point.Models))}
		for _, series := range point.Models {
			name := rng.SeriesLabels[series.Model]
			if name == "" {
				name = series.Model
			}
			day.Agents = append(day.Agents, insightsAgentSpend{Name: name, CostUSD: series.CostUSD})
		}
		out.Daily = append(out.Daily, day)
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, out)
	}
	insightsHeader(cmd, window)
	if len(out.Daily) == 0 || len(out.Daily[0].Agents) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgInsightsNoSpend(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	for _, day := range out.Daily {
		var total float64
		parts := make([]string, 0, len(day.Agents))
		for _, agent := range day.Agents {
			total += agent.CostUSD
			if agent.CostUSD > 0 {
				parts = append(parts, agent.Name+" "+msgUsageDollars(agent.CostUSD))
			}
		}
		fmt.Fprintf(w, "  %s  %9s  %s%s%s\n", day.Date, msgUsageDollars(total), colorDim, strings.Join(parts, ", "), colorReset) //nolint:errcheck,gosec
	}
	fmt.Fprintf(w, "%s%s%s\n", colorDim, msgInsightsDailyTopFive(), colorReset) //nolint:errcheck,gosec
	return nil
}

func runInsightsAgents(cmd *cobra.Command, _ []string) error {
	_, resp, _, window, err := fetchInsights(cmd, "agents")
	if err != nil {
		return err
	}
	table := resp.Tables.Agents
	out := insightsAgentsOutput{
		insightsWindow: window,
		insightsPage:   insightsPage{TotalCostUSD: table.TotalCost, Count: table.Count, Offset: table.Pagination.Offset, HasMore: table.Pagination.HasMore},
		Agents:         make([]insightsAgentRow, 0, len(table.Rows)),
	}
	for _, row := range table.Rows {
		out.Agents = append(out.Agents, insightsAgentRow{
			Name:           row.Identity.Label,
			Kind:           row.Identity.Kind,
			ID:             row.Identity.ID,
			CostUSD:        row.Metrics.CostUSD,
			CostPct:        row.Metrics.CostPct,
			Requests:       row.Metrics.Requests,
			CostPerRequest: row.Metrics.CostPerRequest,
			TokPerRequest:  row.Metrics.TokPerRequest,
			LastSeen:       row.Metrics.LastSeen,
			Deleted:        row.Identity.IsDeleted,
		})
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, out)
	}
	insightsHeader(cmd, window)
	if len(out.Agents) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgInsightsNoSpend(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	for _, agent := range out.Agents {
		name := agent.Name
		if agent.Deleted {
			name += " (deleted)"
		}
		fmt.Fprintf(w, "  %-28s %10s  %5.1f%%  %s requests\n", name, msgUsageDollars(agent.CostUSD), agent.CostPct, thousands(agent.Requests)) //nolint:errcheck,gosec
	}
	insightsFooter(cmd, out.insightsPage, len(out.Agents))
	return nil
}

func runInsightsPeople(cmd *cobra.Command, _ []string) error {
	_, resp, _, window, err := fetchInsights(cmd, "people")
	if err != nil {
		return err
	}
	table := resp.Tables.People
	out := insightsPeopleOutput{
		insightsWindow: window,
		insightsPage:   insightsPage{TotalCostUSD: table.TotalCost, Count: table.Count, Offset: table.Pagination.Offset, HasMore: table.Pagination.HasMore},
		People:         make([]insightsPersonRow, 0, len(table.Rows)),
	}
	for _, row := range table.Rows {
		person := insightsPersonRow{
			Name:     row.Identity.Label,
			Kind:     row.Identity.Kind,
			CostUSD:  row.Metrics.CostUSD,
			CostPct:  row.Metrics.CostPct,
			Requests: row.Metrics.Requests,
			Tokens:   row.Metrics.Tokens,
			LastSeen: row.Metrics.LastSeen,
		}
		for _, agent := range row.AgentsUsed {
			person.AgentsUsed = append(person.AgentsUsed, agent.Label)
		}
		out.People = append(out.People, person)
	}

	w := cmd.OutOrStdout()
	if jsonOut, _ := cmd.Flags().GetBool("json"); jsonOut {
		return writeJSON(w, out)
	}
	insightsHeader(cmd, window)
	if len(out.People) == 0 {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgInsightsNoSpend(), colorReset) //nolint:errcheck,gosec
		return nil
	}
	for _, person := range out.People {
		fmt.Fprintf(w, "  %-28s %10s  %5.1f%%  %s tokens\n", person.Name, msgUsageDollars(person.CostUSD), person.CostPct, thousands(person.Tokens)) //nolint:errcheck,gosec
	}
	insightsFooter(cmd, out.insightsPage, len(out.People))
	return nil
}

func insightsFooter(cmd *cobra.Command, page insightsPage, shown int) {
	w := cmd.OutOrStdout()
	fmt.Fprintln(w)                                                               //nolint:errcheck,gosec
	fmt.Fprintf(w, "  %-28s %10s\n", "Total", msgUsageDollars(page.TotalCostUSD)) //nolint:errcheck,gosec
	if page.HasMore {
		fmt.Fprintf(w, "%s%s%s\n", colorDim, msgInsightsMore(page.Offset+shown, page.Count), colorReset) //nolint:errcheck,gosec
	}
}
