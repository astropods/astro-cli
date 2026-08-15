package handlers

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
)

// DevtoolInsightsResponse is the per-range, per-source dev-tool usage block the
// client folds into the agent surfaces. Sources are keyed by the astro.source
// label; add a tool via the adapter registry below.
type DevtoolInsightsResponse struct {
	Ranges map[string]DevtoolRange `json:"ranges"`
}

type DevtoolRange struct {
	Sources map[string]DevtoolSource `json:"sources"`
}

type DevtoolSource struct {
	Label      string              `json:"label"`
	SpendByDay []DevtoolSpendPoint `json:"spend_by_day"`
	Totals     DevtoolTotals       `json:"totals"`
	ByUser     []DevtoolUserSpend  `json:"by_user"` // per-developer spend, keyed by user.email
	AgentRow   InsightsAgentRow    `json:"agent_row"`
	// Tokens were reported but priced at zero — the model has no Langfuse
	// definition. Distinguishes "unpriced" from "unused".
	CostUnavailable bool `json:"cost_unavailable,omitempty"`
}

type DevtoolSpendPoint struct {
	// YYYY-MM-DD, matches agent_spend_chart so the client can merge by date.
	Date    string  `json:"date"`
	CostUSD float64 `json:"cost_usd"`
}

type DevtoolTotals struct {
	CostUSD     float64 `json:"cost_usd"`
	Requests    int     `json:"requests"`
	TotalTokens int     `json:"total_tokens"`
}

// DevtoolUserSpend is one developer's spend, attributed by the user.email label.
type DevtoolUserSpend struct {
	UserEmail   string  `json:"user_email"`
	CostUSD     float64 `json:"cost_usd"`
	TotalTokens int     `json:"total_tokens"`
	// Set when the email resolves to a member; IdentityKey ("member:"+user_id)
	// matches the member's People-row key so the client merges the spend in.
	UserID      string `json:"user_id,omitempty"`
	IdentityKey string `json:"identity_key,omitempty"`
}

// devtoolAdapter registers one coding tool. Key is both the astro.source value
// and the Langfuse trace tag.
type devtoolAdapter struct {
	Key   string
	Label string
	Icon  string // integration-icon key → themed logo (e.g. "anthropic")
}

var devtoolAdapters = []devtoolAdapter{
	{Key: "claude-code", Label: "Claude Code", Icon: "anthropic"},
}

// computeDevtoolForInsights computes dev-tool usage for the ranges the Insights
// page folds in. Best-effort: any failure yields no data and Insights renders
// unchanged.
//
// One Langfuse query per source covers the widest range; narrower ranges are
// sliced from the same day cells rather than re-queried.
func computeDevtoolForInsights(
	ctx context.Context,
	log *logger.Logger,
	cfg *config.Config,
	langfuseStore *langfuse.Store,
	memberEmails *memberemails.Store,
	accountID string,
	skipRanges bool,
) map[string]DevtoolRange {
	if langfuseStore == nil {
		return nil
	}
	creds, err := langfuseStore.Get(accountID)
	if err != nil || creds == nil {
		return nil
	}
	client := langfuse.NewClient(cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	emailToUserID, err := memberEmails.EmailsForAccount(ctx, accountID)
	if err != nil {
		log.Warn("devtool: member email lookup failed", "account_id", accountID, "error", err)
		emailToUserID = nil
	}
	specs := insightsRangeSpecs
	if skipRanges {
		// Tables read their dev-tool sources from the widest range; compute the
		// same one here (by max days, not slice position) so they can't diverge.
		specs = []insightsRangeSpec{widestInsightsRange()}
	}
	return computeDevtoolInsights(ctx, log, client, accountID, emailToUserID, specs).Ranges
}

// computeDevtoolInsights builds each requested range's per-source block.
func computeDevtoolInsights(
	ctx context.Context,
	log *logger.Logger,
	client *langfuse.Client,
	accountID string,
	emailToUserID map[string]string,
	specs []insightsRangeSpec,
) DevtoolInsightsResponse {
	resp := DevtoolInsightsResponse{Ranges: map[string]DevtoolRange{}}
	if client == nil || len(specs) == 0 {
		return resp
	}
	now := time.Now()
	widest := widestInsightsRange().days

	for _, ad := range devtoolAdapters {
		usage, err := fetchDevtoolUsage(ctx, client, ad.Key, now.AddDate(0, 0, -widest), now)
		if err != nil {
			log.Warn("devtool: usage query failed", "source", ad.Key, "account_id", accountID, "error", err)
			continue
		}
		if usage.costUnavailable() {
			log.Warn("devtool: tokens reported but cost is zero; model is likely unpriced in Langfuse",
				"source", ad.Key, "account_id", accountID)
		}
		for _, spec := range specs {
			src, ok := devtoolSourceFor(ad, usage.since(now.AddDate(0, 0, -spec.days)), emailToUserID)
			if !ok {
				continue
			}
			r := resp.Ranges[spec.key]
			if r.Sources == nil {
				r.Sources = map[string]DevtoolSource{}
			}
			r.Sources[ad.Key] = src
			resp.Ranges[spec.key] = r
		}
	}
	return resp
}

// devtoolSourceFor folds one window of usage into the per-source block;
// ok=false omits the source from that range.
func devtoolSourceFor(ad devtoolAdapter, usage devtoolUsage, emailToUserID map[string]string) (DevtoolSource, bool) {
	total := usage.totals()
	if total.empty() {
		return DevtoolSource{}, false
	}

	days := usage.byDay()
	spend := make([]DevtoolSpendPoint, 0, len(days))
	for date, b := range days {
		if b.CostUSD <= 0 {
			continue
		}
		spend = append(spend, DevtoolSpendPoint{Date: date, CostUSD: round2(b.CostUSD)})
	}
	sort.Slice(spend, func(i, j int) bool { return spend[i].Date < spend[j].Date })

	byUser := usage.byUser()
	users := make([]DevtoolUserSpend, 0, len(byUser))
	for email, b := range byUser {
		u := DevtoolUserSpend{UserEmail: email, CostUSD: round2(b.CostUSD), TotalTokens: b.Tokens}
		if uid := emailToUserID[strings.ToLower(email)]; uid != "" {
			u.UserID = uid
			u.IdentityKey = "member:" + uid
		}
		users = append(users, u)
	}
	sort.Slice(users, func(i, j int) bool { return users[i].UserEmail < users[j].UserEmail })

	totals := DevtoolTotals{
		CostUSD:     round2(total.CostUSD),
		Requests:    total.Requests,
		TotalTokens: total.Tokens,
	}
	agentRow := devtoolAgentRow(ad, totals)
	for _, point := range spend {
		if point.Date > agentRow.Metrics.LastSeen {
			agentRow.Metrics.LastSeen = point.Date
		}
	}
	return DevtoolSource{
		Label:           ad.Label,
		SpendByDay:      spend,
		Totals:          totals,
		ByUser:          users,
		AgentRow:        agentRow,
		CostUnavailable: usage.costUnavailable(),
	}, true
}

// devtoolAgentRow is the synthetic agents-table row for one dev-tool source.
// devtoolIdentity is the row identity for a dev-tool source: system-kind, brand
// icon, and not clickable, because it aggregates local usage across developers
// rather than pointing at a deployed agent.
func devtoolIdentity(ad devtoolAdapter) InsightsIdentityRef {
	return InsightsIdentityRef{
		Kind:    "system",
		Label:   ad.Label,
		Icon:    ad.Icon,
		Tooltip: "Aggregated local dev-tool usage (" + ad.Label + ") across developers, not a deployed agent.",
	}
}

// devtoolAdapterByKey resolves a registered source key.
func devtoolAdapterByKey(key string) (devtoolAdapter, bool) {
	for _, ad := range devtoolAdapters {
		if ad.Key == key {
			return ad, true
		}
	}
	return devtoolAdapter{}, false
}

func devtoolAgentRow(ad devtoolAdapter, totals DevtoolTotals) InsightsAgentRow {
	return InsightsAgentRow{
		Key:        ad.Key,
		SearchText: strings.ToLower(ad.Label),
		Identity:   devtoolIdentity(ad),
		UsedBy:     []InsightsIdentityRef{},
		Metrics: InsightsAgentMetrics{
			Requests:       totals.Requests,
			CostUSD:        totals.CostUSD,
			CostPct:        0, // client rescales against the combined total
			CostPerRequest: perRequest(totals.CostUSD, totals.Requests),
			TokPerRequest:  perRequest(float64(totals.TotalTokens), totals.Requests),
			P95LatencyMs:   0,
		},
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func perRequest(total float64, requests int) float64 {
	if requests <= 0 {
		return 0
	}
	return round2(total / float64(requests))
}
