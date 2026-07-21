package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/insightscache"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

type insightsRangeSpec struct {
	key  string
	days int
}

var insightsRangeSpecs = []insightsRangeSpec{
	{key: "7d", days: 7},
	{key: "14d", days: 14},
	{key: "30d", days: 30},
	{key: "90d", days: 90},
}

// widestInsightsRange is the account-wide proxy for the (range-independent)
// tables: the widest window we compute (max by days, order-independent).
func widestInsightsRange() insightsRangeSpec {
	widest := insightsRangeSpecs[0]
	for _, s := range insightsRangeSpecs[1:] {
		if s.days > widest.days {
			widest = s
		}
	}
	return widest
}

const (
	defaultInsightsTableLimit = 25
	maxInsightsTableLimit     = 5000
)

var (
	insightsAgentSortKeys = map[string]struct{}{
		"cost_usd":         {},
		"requests":         {},
		"cost_per_request": {},
		"tok_per_request":  {},
		"p95_latency_ms":   {},
	}
	insightsPeopleSortKeys = map[string]struct{}{
		"cost_usd":  {},
		"requests":  {},
		"tokens":    {},
		"last_seen": {},
	}
)

type insightsTableParams struct {
	Limit     int
	Offset    int
	Sort      string
	Direction string
}

type insightsRequestParams struct {
	Query       string
	Agents      insightsTableParams
	People      insightsTableParams
	SkipRanges  bool
	HideSources map[string]bool // source keys (and "agents") excluded from the fold-in
	// RestrictDevtoolToKey gates the per-developer dev-tool breakdown in the
	// People view: empty = all developers (admins); otherwise only this identity
	// key's own dev-tool spend is folded in (members see only themselves). Set
	// from the caller's role, not a query param.
	RestrictDevtoolToKey string
}

type insightsMemberProfile struct {
	userID      string
	username    string
	displayName string
}

type insightTotals struct {
	cost     float64
	requests int
	tokens   int
}

// GetAccountInsights returns the complete server-owned view model for the
// Insights page. The lower-level observability endpoints remain reusable API
// primitives; this endpoint owns page semantics.
// GET /api/v1/accounts/:account/insights
func GetAccountInsights(
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
	cache k8scache.Cache,
	promClient *promquery.Client,
	memberEmails *memberemails.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(c.Param("account"))
		if err != nil || acct == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		isMember, err := accountStore.IsMember(acct.ID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}

		// Per-developer dev-tool spend in the People view is admin-only; members
		// see only their own row. Aggregate spend stays visible to all members.
		params := parseInsightsRequestParams(c)
		if !middleware.HasAccountPermission(c, accountStore, acct, user, "org:admin") {
			params.RestrictDevtoolToKey = "member:" + user.ID
		}

		resp, err := ComputeInsightsWithParams(c.Request.Context(), log, cfg, accountStore, deploymentStore, langfuseStore, slackStore, cache, promClient, memberEmails, acct, time.Now().UTC(), params)
		if err != nil {
			log.Error("Failed to compute insights view model", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute insights"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

func ComputeInsights(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
	cache k8scache.Cache,
	promClient *promquery.Client,
	memberEmails *memberemails.Store,
	acct *account.Account,
	now time.Time,
) (InsightsResponse, error) {
	return ComputeInsightsWithParams(ctx, log, cfg, accountStore, deploymentStore, langfuseStore, slackStore, cache, promClient, memberEmails, acct, now, defaultInsightsRequestParams())
}

func ComputeInsightsWithParams(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	accountStore *account.AccountStore,
	deploymentStore *deploymentstore.Store,
	langfuseStore *langfuse.Store,
	slackStore *slackidentity.Store,
	cache k8scache.Cache,
	promClient *promquery.Client,
	memberEmails *memberemails.Store,
	acct *account.Account,
	now time.Time,
	params insightsRequestParams,
) (InsightsResponse, error) {
	// Normalize exactly once at the compute boundary. Builders and paginators
	// receive bounded, whitelisted params and should not re-normalize them.
	params = normalizeInsightsRequestParams(params)

	var summary AccountObservabilitySummaryResponse
	var deployments AccountDeploymentsSummaryResponse
	var users AccountUsersSummaryResponse
	var members map[string]insightsMemberProfile
	var devtoolRanges map[string]DevtoolRange

	g, gCtx := errgroup.WithContext(ctx)
	g.Go(func() error {
		// Best-effort: never fails the page. Nil prom client → no dev-tool data.
		devtoolRanges = computeDevtoolForInsights(gCtx, log, promClient, memberEmails, acct.ID, params.SkipRanges)
		return nil
	})
	g.Go(func() error {
		resp, err := insightsAccountSummary(gCtx, log, cfg, langfuseStore, deploymentStore, accountStore, slackStore, cache, acct)
		if err != nil {
			return err
		}
		summary = resp
		return nil
	})
	g.Go(func() error {
		resp, err := insightsDeploymentsSummary(gCtx, log, cfg, langfuseStore, deploymentStore, accountStore, slackStore, cache, acct)
		if err != nil {
			return err
		}
		deployments = resp
		return nil
	})
	g.Go(func() error {
		resp, err := insightsUsersSummary(gCtx, log, cfg, langfuseStore, deploymentStore, accountStore, slackStore, cache, acct)
		if err != nil {
			return err
		}
		users = resp
		return nil
	})
	g.Go(func() error {
		resp, err := insightsMemberProfiles(log, accountStore, acct.ID)
		if err != nil {
			return err
		}
		members = resp
		return nil
	})
	if err := g.Wait(); err != nil {
		return InsightsResponse{}, err
	}

	resp := buildInsightsViewWithParams(acct.Name, summary, deployments, users, members, devtoolRanges, now, params)
	resp.MetricsUnavailable = summary.MetricsUnavailable || deployments.MetricsUnavailable || users.MetricsUnavailable
	return resp, nil
}

func defaultInsightsRequestParams() insightsRequestParams {
	return insightsRequestParams{
		Agents: insightsTableParams{Limit: defaultInsightsTableLimit, Sort: "cost_usd", Direction: "desc"},
		People: insightsTableParams{Limit: defaultInsightsTableLimit, Sort: "cost_usd", Direction: "desc"},
	}
}

func parseInsightsRequestParams(c *gin.Context) insightsRequestParams {
	params := defaultInsightsRequestParams()
	params.Query = strings.TrimSpace(c.Query("q"))
	params.Agents = parseInsightsTableParams(c, "agents", params.Agents)
	params.People = parseInsightsTableParams(c, "people", params.People)
	params.SkipRanges = strings.EqualFold(c.Query("skip_ranges"), "true")
	params.HideSources = parseHideSources(c.Query("hide_sources"))
	return params
}

// parseHideSources reads the comma-separated hide_sources param into a set of
// lowercased source keys (and the "agents" pseudo-source).
func parseHideSources(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := map[string]bool{}
	for _, part := range strings.Split(raw, ",") {
		if key := strings.ToLower(strings.TrimSpace(part)); key != "" {
			out[key] = true
		}
	}
	return out
}

func parseInsightsTableParams(c *gin.Context, prefix string, defaults insightsTableParams) insightsTableParams {
	params := defaults
	if raw := c.Query(prefix + "_limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			params.Limit = parsed
		}
	}
	if raw := c.Query(prefix + "_offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			params.Offset = parsed
		}
	}
	if raw := c.Query(prefix + "_sort"); raw != "" {
		params.Sort = raw
	}
	if raw := c.Query(prefix + "_direction"); raw != "" {
		params.Direction = raw
	}
	return params
}

func normalizeInsightsRequestParams(params insightsRequestParams) insightsRequestParams {
	defaults := defaultInsightsRequestParams()
	params.Query = strings.ToLower(strings.TrimSpace(params.Query))
	params.Agents = normalizeInsightsTableParams(params.Agents, defaults.Agents, insightsAgentSortKeys)
	params.People = normalizeInsightsTableParams(params.People, defaults.People, insightsPeopleSortKeys)
	return params
}

func normalizeInsightsTableParams(params, defaults insightsTableParams, validSortKeys map[string]struct{}) insightsTableParams {
	if params.Limit <= 0 {
		params.Limit = defaults.Limit
	}
	if params.Limit > maxInsightsTableLimit {
		params.Limit = maxInsightsTableLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}
	params.Sort = strings.ToLower(strings.TrimSpace(params.Sort))
	if params.Sort == "" {
		params.Sort = defaults.Sort
	}
	if _, ok := validSortKeys[params.Sort]; !ok {
		params.Sort = defaults.Sort
	}
	params.Direction = strings.ToLower(strings.TrimSpace(params.Direction))
	if params.Direction != "asc" {
		params.Direction = "desc"
	}
	return params
}

func insightsAccountSummary(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
	cache k8scache.Cache,
	acct *account.Account,
) (AccountObservabilitySummaryResponse, error) {
	params := insightscache.Params{GroupBy: "user", IncludeArchived: false}
	if bytes, ok := insightscache.Get(ctx, cache, acct.ID, insightscache.EndpointSummary, params); ok {
		var resp AccountObservabilitySummaryResponse
		if err := json.Unmarshal(bytes, &resp); err == nil {
			ResolveAccountSummaryIdentities(log, slackStore, accountStore, &resp)
			return resp, nil
		}
		log.Warn("Failed to decode cached insights summary; recomputing", "account_id", acct.ID)
	}
	resp, err := ComputeAccountSummary(ctx, log, cfg, langfuseStore, deploymentStore, slackStore, acct, "", "", "user", false)
	if errors.Is(err, ErrAllLangfuseCallsFailed) {
		degraded := zeroAccountSummary("", "", false)
		degraded.MetricsUnavailable = true
		return degraded, nil
	}
	if err == nil {
		ResolveAccountSummaryIdentities(log, slackStore, accountStore, &resp)
	}
	return resp, err
}

func insightsDeploymentsSummary(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
	cache k8scache.Cache,
	acct *account.Account,
) (AccountDeploymentsSummaryResponse, error) {
	params := insightscache.Params{IncludeArchived: false}
	if bytes, ok := insightscache.Get(ctx, cache, acct.ID, insightscache.EndpointDeploymentsSummary, params); ok {
		var resp AccountDeploymentsSummaryResponse
		if err := json.Unmarshal(bytes, &resp); err == nil {
			ResolveDeploymentsSummaryIdentities(log, slackStore, accountStore, &resp)
			return resp, nil
		}
		log.Warn("Failed to decode cached insights deployments summary; recomputing", "account_id", acct.ID)
	}
	resp, err := ComputeDeploymentsSummary(ctx, log, cfg, langfuseStore, deploymentStore, slackStore, acct, "", "", false)
	if errors.Is(err, ErrAllLangfuseCallsFailed) {
		degraded := zeroDeploymentEntries("", "")
		degraded.MetricsUnavailable = true
		return degraded, nil
	}
	if err == nil {
		ResolveDeploymentsSummaryIdentities(log, slackStore, accountStore, &resp)
	}
	return resp, err
}

func insightsUsersSummary(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	deploymentStore *deploymentstore.Store,
	accountStore *account.AccountStore,
	slackStore *slackidentity.Store,
	cache k8scache.Cache,
	acct *account.Account,
) (AccountUsersSummaryResponse, error) {
	if bytes, ok := insightscache.Get(ctx, cache, acct.ID, insightscache.EndpointUsersSummary, insightscache.Params{}); ok {
		var resp AccountUsersSummaryResponse
		if err := json.Unmarshal(bytes, &resp); err == nil {
			ResolveUsersSummaryIdentities(log, slackStore, accountStore, &resp)
			return resp, nil
		}
		log.Warn("Failed to decode cached insights users summary; recomputing", "account_id", acct.ID)
	}
	resp, err := ComputeUsersSummary(ctx, log, cfg, langfuseStore, deploymentStore, accountStore, slackStore, acct, "", "")
	if errors.Is(err, ErrAllLangfuseCallsFailed) {
		return AccountUsersSummaryResponse{
			Users:              []UserSummaryEntry{},
			Period:             buildPeriod("", ""),
			MetricsUnavailable: true,
		}, nil
	}
	if err == nil {
		ResolveUsersSummaryIdentities(log, slackStore, accountStore, &resp)
	}
	return resp, err
}

func insightsMemberProfiles(log *logger.Logger, accountStore *account.AccountStore, accountID string) (map[string]insightsMemberProfile, error) {
	members, err := accountStore.GetMembersForAccount(accountID)
	if err != nil {
		return nil, fmt.Errorf("list account members: %w", err)
	}
	userIDs := make([]string, 0, len(members))
	for _, m := range members {
		userIDs = append(userIDs, m.UserID)
	}
	profiles, err := accountStore.GetPersonalProfiles(userIDs)
	if err != nil {
		log.Warn("Failed to fetch member profiles for insights", "error", err, "account_id", accountID)
		profiles = map[string]account.PersonalProfile{}
	}
	out := make(map[string]insightsMemberProfile, len(members))
	for _, m := range members {
		p := profiles[m.UserID]
		out[m.UserID] = insightsMemberProfile{
			userID:      m.UserID,
			username:    p.Name,
			displayName: p.DisplayName,
		}
	}
	return out, nil
}

func buildInsightsView(
	accountName string,
	summary AccountObservabilitySummaryResponse,
	deployments AccountDeploymentsSummaryResponse,
	users AccountUsersSummaryResponse,
	members map[string]insightsMemberProfile,
	now time.Time,
) InsightsResponse {
	return buildInsightsViewWithParams(accountName, summary, deployments, users, members, nil, now, normalizeInsightsRequestParams(defaultInsightsRequestParams()))
}

func buildInsightsViewWithParams(
	accountName string,
	summary AccountObservabilitySummaryResponse,
	deployments AccountDeploymentsSummaryResponse,
	users AccountUsersSummaryResponse,
	members map[string]insightsMemberProfile,
	devtoolRanges map[string]DevtoolRange,
	now time.Time,
	params insightsRequestParams,
) InsightsResponse {
	depRows := deployments.Deployments
	userRows := users.Users
	identityByUser := insightsIdentityLookup(userRows)
	hidden := params.HideSources
	agentsHidden := hidden["agents"]
	tableSources := devtoolRanges[widestInsightsRange().key].Sources

	ranges := map[string]InsightsRange{}
	if !params.SkipRanges {
		ranges = make(map[string]InsightsRange, len(insightsRangeSpecs))
		for _, spec := range insightsRangeSpecs {
			period, fromDate, toDate := insightsPeriod(now, spec.days)
			sliced := sliceInsightsDeployments(depRows, fromDate, toDate)
			priorFrom, priorTo := priorWindow(fromDate, toDate)
			prior := sumDeploymentWindow(depRows, priorFrom, priorTo)
			r := InsightsRange{
				Days:             spec.days,
				Period:           period,
				StatCards:        buildInsightsStatCards(sliced, prior),
				AgentSpendChart:  buildInsightsAgentSpendChart(sliced, period),
				PeopleSpendChart: buildInsightsPeopleSpendChart(summary, period),
				SeriesLabels:     buildInsightsSeriesLabels(sliced),
			}
			ranges[spec.key] = foldDevtoolRange(r, devtoolRanges[spec.key].Sources, hidden, agentsHidden)
		}
	}

	// Tables intentionally use the full account aggregates, not the active chart
	// range. Dev-tool sources fold in before sort/paginate/percentage so they run
	// through the one table pipeline; their contribution reflects the widest range.
	agentRows, agentTotal := buildInsightsAgentRows(accountName, depRows, members, identityByUser)
	agentRows, agentTotal = foldDevtoolAgentRows(agentRows, agentTotal, tableSources, hidden, agentsHidden)
	peopleRows, peopleTotal := buildInsightsPeopleRows(accountName, userRows, depRows, members)
	peopleRows, peopleTotal = foldDevtoolPeopleRows(peopleRows, peopleTotal, tableSources, hidden, agentsHidden, members, params.RestrictDevtoolToKey)

	return InsightsResponse{
		Ranges: ranges,
		Tables: InsightsTables{
			Agents: paginateInsightsAgentsTable(agentRows, agentTotal, params.Query, params.Agents),
			People: paginateInsightsPeopleTable(peopleRows, peopleTotal, params.Query, params.People),
		},
		DevtoolSources: devtoolSourceRefs(tableSources),
	}
}

func insightsPeriod(now time.Time, days int) (AccountSummaryPeriod, string, string) {
	now = now.UTC()
	to := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, int(time.Millisecond*999), time.UTC)
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	return AccountSummaryPeriod{
		Start: from.Format(time.RFC3339Nano),
		End:   to.Format(time.RFC3339Nano),
		Days:  days,
	}, from.Format("2006-01-02"), to.Format("2006-01-02")
}

func priorWindow(fromDate, toDate string) (string, string) {
	from, err1 := time.Parse("2006-01-02", fromDate)
	to, err2 := time.Parse("2006-01-02", toDate)
	if err1 != nil || err2 != nil || from.After(to) {
		return "", ""
	}
	days := int(to.Sub(from).Hours()/24) + 1
	priorTo := from.AddDate(0, 0, -1)
	priorFrom := priorTo.AddDate(0, 0, -(days - 1))
	return priorFrom.Format("2006-01-02"), priorTo.Format("2006-01-02")
}

func sliceInsightsDeployments(rows []DeploymentSummaryEntry, fromDate, toDate string) []DeploymentSummaryEntry {
	out := make([]DeploymentSummaryEntry, 0, len(rows))
	for _, row := range rows {
		sliced := row
		sliced.CostOverTime = filterDailyCost(row.CostOverTime, fromDate, toDate)
		sliced.RequestsOverTime = filterDailyRequests(row.RequestsOverTime, fromDate, toDate)
		sliced.TokensOverTime = filterDailyTokens(row.TokensOverTime, fromDate, toDate)

		var cost float64
		for _, d := range sliced.CostOverTime {
			cost += d.CostUSD
		}
		var requests int
		for _, d := range sliced.RequestsOverTime {
			requests += d.Requests
		}
		var inputTokens, outputTokens, totalTokens int
		for _, d := range sliced.TokensOverTime {
			inputTokens += d.InputTokens
			outputTokens += d.OutputTokens
			totalTokens += d.TotalTokens
		}
		sliced.CostUSD = math.Round(cost*10000) / 10000
		sliced.Requests = requests
		sliced.InputTokens = inputTokens
		sliced.OutputTokens = outputTokens
		sliced.TotalTokens = totalTokens
		sliced.CostPerRequest = 0
		sliced.TokPerRequest = 0
		if requests > 0 {
			sliced.CostPerRequest = math.Round(cost/float64(requests)*10000) / 10000
			sliced.TokPerRequest = math.Round(float64(totalTokens)/float64(requests)*10) / 10
		}
		if sliced.IsArchived && sliced.Requests == 0 && sliced.CostUSD == 0 {
			continue
		}
		out = append(out, sliced)
	}
	return out
}

func filterDailyCost(rows []DeploymentDailyCost, fromDate, toDate string) []DeploymentDailyCost {
	out := make([]DeploymentDailyCost, 0, len(rows))
	for _, row := range rows {
		if row.Date >= fromDate && row.Date <= toDate {
			out = append(out, row)
		}
	}
	return out
}

func filterDailyRequests(rows []DeploymentDailyRequests, fromDate, toDate string) []DeploymentDailyRequests {
	out := make([]DeploymentDailyRequests, 0, len(rows))
	for _, row := range rows {
		if row.Date >= fromDate && row.Date <= toDate {
			out = append(out, row)
		}
	}
	return out
}

func filterDailyTokens(rows []DeploymentDailyTokens, fromDate, toDate string) []DeploymentDailyTokens {
	out := make([]DeploymentDailyTokens, 0, len(rows))
	for _, row := range rows {
		if row.Date >= fromDate && row.Date <= toDate {
			out = append(out, row)
		}
	}
	return out
}

func sumDeploymentWindow(rows []DeploymentSummaryEntry, fromDate, toDate string) insightTotals {
	if fromDate == "" || toDate == "" {
		return insightTotals{}
	}
	var out insightTotals
	for _, row := range rows {
		for _, d := range row.CostOverTime {
			if d.Date >= fromDate && d.Date <= toDate {
				out.cost += d.CostUSD
			}
		}
		for _, d := range row.RequestsOverTime {
			if d.Date >= fromDate && d.Date <= toDate {
				out.requests += d.Requests
			}
		}
		for _, d := range row.TokensOverTime {
			if d.Date >= fromDate && d.Date <= toDate {
				out.tokens += d.TotalTokens
			}
		}
	}
	return out
}

func buildInsightsStatCards(rows []DeploymentSummaryEntry, prior insightTotals) InsightsStatCards {
	var totals insightTotals
	for _, row := range rows {
		totals.cost += row.CostUSD
		totals.requests += row.Requests
		totals.tokens += row.TotalTokens
	}
	return InsightsStatCards{
		Totals: AccountSummaryTotals{
			CostUSD:      math.Round(totals.cost*100) / 100,
			Requests:     totals.requests,
			TotalTokens:  totals.tokens,
			ActiveAgents: len(rows),
		},
		Change: insightsChange(totals, prior),
	}
}

func insightsChange(current, prior insightTotals) *AccountSummaryChange {
	return &AccountSummaryChange{
		CostPct:     pctChange(current.cost, prior.cost),
		RequestsPct: pctChange(float64(current.requests), float64(prior.requests)),
		TokensPct:   pctChange(float64(current.tokens), float64(prior.tokens)),
	}
}

func buildInsightsAgentSpendChart(rows []DeploymentSummaryEntry, period AccountSummaryPeriod) []AccountCostOverTimeEntry {
	sorted := append([]DeploymentSummaryEntry(nil), rows...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].CostUSD > sorted[j].CostUSD })
	if len(sorted) > 5 {
		sorted = sorted[:5]
	}
	dates := enumerateInsightDates(period)
	costByDeployment := make(map[string]map[string]float64, len(sorted))
	for _, row := range sorted {
		byDate := make(map[string]float64, len(row.CostOverTime))
		for _, d := range row.CostOverTime {
			byDate[d.Date] = d.CostUSD
		}
		costByDeployment[row.DeploymentID] = byDate
	}
	out := make([]AccountCostOverTimeEntry, 0, len(dates))
	for _, date := range dates {
		models := make([]AccountModelCost, 0, len(sorted))
		for _, row := range sorted {
			models = append(models, AccountModelCost{
				Model:   row.DeploymentID,
				CostUSD: math.Round(costByDeployment[row.DeploymentID][date]*10000) / 10000,
			})
		}
		out = append(out, AccountCostOverTimeEntry{Date: date, Models: models})
	}
	return out
}

func buildInsightsPeopleSpendChart(summary AccountObservabilitySummaryResponse, period AccountSummaryPeriod) []InsightsPeopleSpendPoint {
	type acc struct {
		cost  float64
		users map[string]struct{}
	}
	byDate := map[string]acc{}
	fromDate, toDate := insightPeriodDates(period)
	for _, row := range summary.CostOverTimeByUser {
		date := row.Date
		if len(date) >= 10 {
			date = date[:10]
		}
		if fromDate != "" && (date < fromDate || date > toDate) {
			continue
		}
		a := byDate[date]
		if a.users == nil {
			a.users = map[string]struct{}{}
		}
		for _, u := range row.Users {
			a.cost += u.CostUSD
			if u.CostUSD > 0 {
				a.users[u.UserID] = struct{}{}
			}
		}
		byDate[date] = a
	}
	dates := enumerateInsightDates(period)
	out := make([]InsightsPeopleSpendPoint, 0, len(dates))
	for _, date := range dates {
		a := byDate[date]
		out = append(out, InsightsPeopleSpendPoint{
			Date:  date,
			Users: len(a.users),
			Cost:  math.Round(a.cost*10000) / 10000,
		})
	}
	return out
}

func buildInsightsSeriesLabels(rows []DeploymentSummaryEntry) map[string]string {
	counts := make(map[string]int, len(rows))
	for _, row := range rows {
		base := row.DisplayName
		if base == "" {
			base = row.AgentName
		}
		counts[base]++
	}
	labels := make(map[string]string, len(rows))
	for _, row := range rows {
		base := row.DisplayName
		if base == "" {
			base = row.AgentName
		}
		if counts[base] > 1 && row.Namespace != "" {
			labels[row.DeploymentID] = base + " (" + row.Namespace + ")"
		} else {
			labels[row.DeploymentID] = base
		}
	}
	return labels
}

func buildInsightsAgentRows(accountName string, deployments []DeploymentSummaryEntry, members map[string]insightsMemberProfile, identityByUser map[string]UserIdentity) ([]InsightsAgentRow, float64) {
	totalCost := 0.0
	for _, row := range deployments {
		totalCost += row.CostUSD
	}
	rows := make([]InsightsAgentRow, 0, len(deployments))
	for _, dep := range deployments {
		label := dep.DisplayName
		if label == "" {
			label = dep.AgentName
		}
		usedByRefs := dep.UsersUsedDetails
		if len(usedByRefs) == 0 {
			usedByRefs = make([]UserIdentity, 0, len(dep.UsersUsed))
			for _, uid := range dep.UsersUsed {
				if ref, ok := identityByUser[uid]; ok {
					usedByRefs = append(usedByRefs, ref)
				} else {
					usedByRefs = append(usedByRefs, UserIdentity{
						UserID:      uid,
						UserDetails: UserDetails{Kind: classifyUserID(uid)},
					})
				}
			}
		}
		usedBy := make([]InsightsIdentityRef, 0, len(usedByRefs))
		for _, ref := range usedByRefs {
			usedBy = append(usedBy, insightUserIdentity(ref, members))
		}
		searchParts := []string{label, dep.AgentName, dep.Namespace}
		for _, identity := range usedBy {
			searchParts = append(searchParts, insightIdentitySearchParts(identity)...)
		}
		rows = append(rows, InsightsAgentRow{
			Key:        dep.DeploymentID,
			SearchText: strings.ToLower(strings.Join(searchParts, " ")),
			Identity: InsightsIdentityRef{
				Kind:          "agent",
				ID:            dep.DeploymentID,
				Label:         label,
				Href:          insightDeploymentHref(accountName, dep.DeploymentID),
				AvatarAccount: accountName,
				AvatarName:    dep.AgentName,
			},
			UsedBy: usedBy,
			Metrics: InsightsAgentMetrics{
				Requests:       dep.Requests,
				CostUSD:        dep.CostUSD,
				CostPct:        insightPct(dep.CostUSD, totalCost),
				CostPerRequest: dep.CostPerRequest,
				TokPerRequest:  dep.TokPerRequest,
				P95LatencyMs:   dep.P95LatencyMs,
			},
			NotInstrumented: dep.Requests == 0,
		})
	}
	return rows, math.Round(totalCost*10000) / 10000
}

func buildInsightsPeopleRows(accountName string, users []UserSummaryEntry, deployments []DeploymentSummaryEntry, members map[string]insightsMemberProfile) ([]InsightsPersonRow, float64) {
	depByID := make(map[string]DeploymentSummaryEntry, len(deployments))
	for _, dep := range deployments {
		depByID[dep.DeploymentID] = dep
	}
	totalCost := 0.0
	for _, u := range users {
		totalCost += u.CostUSD
	}

	rows := make([]InsightsPersonRow, 0, len(users))
	var system *UserSummaryEntry
	for _, u := range users {
		if u.UserID == "" {
			if system == nil {
				copy := u
				system = &copy
			} else {
				system.Requests += u.Requests
				system.CostUSD += u.CostUSD
				system.Tokens += u.Tokens
				if u.LastSeen > system.LastSeen {
					system.LastSeen = u.LastSeen
				}
				system.AgentsUsed = append(system.AgentsUsed, u.AgentsUsed...)
			}
			continue
		}
		row := insightPersonRow(accountName, u, totalCost, depByID, members)
		rows = append(rows, row)
	}
	if system != nil {
		rows = append(rows, insightPersonRow(accountName, *system, totalCost, depByID, members))
	}
	return rows, math.Round(totalCost*10000) / 10000
}

func paginateInsightsAgentsTable(rows []InsightsAgentRow, totalCost float64, query string, params insightsTableParams) InsightsAgentsTable {
	totalCount := len(rows)
	filtered := filterInsightsAgentRows(rows, query)
	sortInsightsAgentRows(filtered, params)
	page, pagination := paginateInsightsRows(filtered, params, totalCount)
	return InsightsAgentsTable{
		Rows:       page,
		TotalCost:  totalCost,
		Count:      totalCount,
		Pagination: pagination,
	}
}

func paginateInsightsPeopleTable(rows []InsightsPersonRow, totalCost float64, query string, params insightsTableParams) InsightsPeopleTable {
	totalCount := len(rows)
	missingSlackDetailsCount := countMissingSlackDetailsRows(rows)
	filtered := filterInsightsPeopleRows(rows, query)
	sortInsightsPeopleRows(filtered, params)
	page, pagination := paginateInsightsRows(filtered, params, totalCount)
	return InsightsPeopleTable{
		Rows:                     page,
		TotalCost:                totalCost,
		Count:                    totalCount,
		MissingSlackDetailsCount: missingSlackDetailsCount,
		Pagination:               pagination,
	}
}

func filterInsightsAgentRows(rows []InsightsAgentRow, query string) []InsightsAgentRow {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]InsightsAgentRow(nil), rows...)
	}
	out := make([]InsightsAgentRow, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(row.SearchText, query) {
			out = append(out, row)
		}
	}
	return out
}

func filterInsightsPeopleRows(rows []InsightsPersonRow, query string) []InsightsPersonRow {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return append([]InsightsPersonRow(nil), rows...)
	}
	out := make([]InsightsPersonRow, 0, len(rows))
	for _, row := range rows {
		if strings.Contains(row.SearchText, query) {
			out = append(out, row)
		}
	}
	return out
}

func sortInsightsAgentRows(rows []InsightsAgentRow, params insightsTableParams) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := insightAgentSortValue(rows[i], params.Sort)
		right := insightAgentSortValue(rows[j], params.Sort)
		if left == right {
			return rows[i].Key < rows[j].Key
		}
		if params.Direction == "asc" {
			return left < right
		}
		return left > right
	})
}

func sortInsightsPeopleRows(rows []InsightsPersonRow, params insightsTableParams) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Identity.Kind == "system" && rows[j].Identity.Kind != "system" {
			return false
		}
		if rows[j].Identity.Kind == "system" && rows[i].Identity.Kind != "system" {
			return true
		}
		left := insightPeopleSortValue(rows[i], params.Sort)
		right := insightPeopleSortValue(rows[j], params.Sort)
		if left == right {
			return rows[i].Key < rows[j].Key
		}
		if params.Direction == "asc" {
			return left < right
		}
		return left > right
	})
}

func insightAgentSortValue(row InsightsAgentRow, key string) float64 {
	switch key {
	case "requests":
		return float64(row.Metrics.Requests)
	case "cost_per_request":
		return row.Metrics.CostPerRequest
	case "tok_per_request":
		return row.Metrics.TokPerRequest
	case "p95_latency_ms":
		return float64(row.Metrics.P95LatencyMs)
	default:
		return row.Metrics.CostUSD
	}
}

func insightPeopleSortValue(row InsightsPersonRow, key string) float64 {
	switch key {
	case "requests":
		return float64(row.Metrics.Requests)
	case "tokens":
		return float64(row.Metrics.Tokens)
	case "last_seen":
		if row.Metrics.LastSeen == "" {
			return 0
		}
		if parsed, err := time.Parse(time.RFC3339, row.Metrics.LastSeen); err == nil {
			return float64(parsed.Unix())
		}
		if parsed, err := time.Parse("2006-01-02", row.Metrics.LastSeen); err == nil {
			return float64(parsed.Unix())
		}
		return 0
	default:
		return row.Metrics.CostUSD
	}
}

func paginateInsightsRows[Row any](rows []Row, params insightsTableParams, totalCount int) ([]Row, InsightsTablePagination) {
	filteredCount := len(rows)
	offset := params.Offset
	if offset > filteredCount {
		offset = filteredCount
	}
	end := offset + params.Limit
	if end > filteredCount {
		end = filteredCount
	}
	page := append([]Row(nil), rows[offset:end]...)
	return page, InsightsTablePagination{
		Limit:         params.Limit,
		Offset:        offset,
		TotalCount:    totalCount,
		FilteredCount: filteredCount,
		HasMore:       end < filteredCount,
	}
}

func countMissingSlackDetailsRows(rows []InsightsPersonRow) int {
	missing := 0
	for _, row := range rows {
		identity := row.Identity
		if identity.Kind != "slack" {
			continue
		}
		details := identity.UserDetails
		if details != nil && details.TeamID != "" && (details.DisplayName != "" || details.Username != "" || details.AvatarURL != "") {
			continue
		}
		missing++
	}
	return missing
}

func insightPersonRow(
	accountName string,
	user UserSummaryEntry,
	totalCost float64,
	depByID map[string]DeploymentSummaryEntry,
	members map[string]insightsMemberProfile,
) InsightsPersonRow {
	identity := insightUserIdentity(user.UserIdentity, members)
	agents := insightAgentChips(user.AgentsUsed, depByID)
	searchParts := insightIdentitySearchParts(identity)
	for _, agent := range agents {
		searchParts = append(searchParts, agent.Label, agent.AvatarName)
	}
	return InsightsPersonRow{
		Key:        insightIdentityRowKey(identity),
		SearchText: strings.ToLower(strings.Join(searchParts, " ")),
		Identity:   identity,
		AgentsUsed: agents,
		Metrics: InsightsPersonMetrics{
			Requests: user.Requests,
			CostUSD:  user.CostUSD,
			CostPct:  insightPct(user.CostUSD, totalCost),
			Tokens:   user.Tokens,
			LastSeen: user.LastSeen,
		},
	}
}

func insightUserIdentity(ref UserIdentity, members map[string]insightsMemberProfile) InsightsIdentityRef {
	uid := ref.UserID
	details := ref.UserDetails
	if details.Kind == "" {
		details.Kind = classifyUserID(uid)
	}
	detailsRef := &details
	if uid == "" {
		return InsightsIdentityRef{
			Kind:    "system",
			ID:      "__system_spend__",
			Label:   "System spend",
			Tooltip: "Traces not associated with any user — typically background jobs, system tasks, or SDK calls that didn't forward a user identifier.",
		}
	}
	if member, ok := members[uid]; ok {
		label := member.displayName
		if label == "" {
			label = member.username
		}
		if label == "" {
			label = uid
		}
		identity := InsightsIdentityRef{
			Kind:         "member",
			ID:           uid,
			UserID:       uid,
			UserDetails:  detailsRef,
			Label:        label,
			AvatarHandle: member.username,
		}
		if member.username != "" {
			identity.Href = "/" + member.username
		}
		return identity
	}
	if details.Kind == UserDetailsKindAstro {
		label := details.DisplayName
		if label == "" {
			label = details.Username
		}
		if label == "" {
			label = uid
		}
		identity := InsightsIdentityRef{
			Kind:         "member",
			ID:           uid,
			UserID:       uid,
			UserDetails:  detailsRef,
			Label:        label,
			AvatarHandle: details.Username,
		}
		if details.Username != "" {
			identity.Href = "/" + details.Username
		}
		return identity
	}
	if details.Kind == UserDetailsKindSlack {
		teamID := details.TeamID
		label := slackDisplayNameFromProfile(details.DisplayName, details.Username)
		if label == "" {
			label = "Slack user - " + uid
		}
		identity := InsightsIdentityRef{
			Kind:        "slack",
			ID:          uid,
			UserID:      uid,
			UserDetails: detailsRef,
			Label:       label,
			Tooltip:     "Slack User",
		}
		if teamID != "" {
			identity.IdentityKey = "slack:" + teamID + ":" + uid
			identity.Href = "slack://user?team=" + teamID + "&id=" + uid
		}
		return identity
	}
	return InsightsIdentityRef{
		Kind:        "unidentified",
		ID:          uid,
		UserID:      uid,
		UserDetails: detailsRef,
		Label:       uid,
	}
}

func insightsIdentityLookup(users []UserSummaryEntry) map[string]UserIdentity {
	out := make(map[string]UserIdentity, len(users))
	for _, user := range users {
		uid := user.UserID
		if uid == "" {
			continue
		}
		out[uid] = user.UserIdentity
	}
	return out
}

func insightIdentitySearchParts(identity InsightsIdentityRef) []string {
	var displayName, username string
	if identity.UserDetails != nil {
		displayName = identity.UserDetails.DisplayName
		username = identity.UserDetails.Username
	}
	return []string{
		identity.Label,
		identity.ID,
		identity.UserID,
		displayName,
		username,
	}
}

func insightIdentityRowKey(identity InsightsIdentityRef) string {
	if identity.IdentityKey != "" {
		return identity.Kind + ":" + identity.IdentityKey
	}
	return identity.Kind + ":" + identity.ID
}

func insightAgentChips(agents []UserAgentRef, depByID map[string]DeploymentSummaryEntry) []InsightsAgentChip {
	out := make([]InsightsAgentChip, 0, len(agents))
	for _, agent := range agents {
		dep, ok := depByID[agent.DeploymentID]
		label := agent.Name
		if ok {
			label = dep.DisplayName
			if label == "" {
				label = dep.AgentName
			}
		}
		href := insightDeploymentHref(agent.Account, agent.DeploymentID)
		if !ok {
			href = "/" + agent.Account + "/" + agent.Name
		}
		out = append(out, InsightsAgentChip{
			Key:           agent.DeploymentID,
			Label:         label,
			Href:          href,
			AvatarAccount: agent.Account,
			AvatarName:    agent.Name,
			IsDeleted:     !ok,
		})
	}
	return out
}

func insightDeploymentHref(accountName, deploymentID string) string {
	return "/" + accountName + "/agents/" + deploymentID + "/monitor"
}

func insightPct(value, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return math.Round(value/total*1000) / 10
}

func enumerateInsightDates(period AccountSummaryPeriod) []string {
	fromDate, toDate := insightPeriodDates(period)
	if fromDate == "" || toDate == "" || fromDate > toDate {
		return []string{}
	}
	from, err1 := time.Parse("2006-01-02", fromDate)
	to, err2 := time.Parse("2006-01-02", toDate)
	if err1 != nil || err2 != nil {
		return []string{}
	}
	out := make([]string, 0, period.Days)
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		out = append(out, d.Format("2006-01-02"))
	}
	return out
}

func insightPeriodDates(period AccountSummaryPeriod) (string, string) {
	from := period.Start
	to := period.End
	if len(from) >= 10 {
		from = from[:10]
	}
	if len(to) >= 10 {
		to = to[:10]
	}
	return from, to
}
