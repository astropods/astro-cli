package handlers

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/promquery"
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

// devtoolAdapter maps one coding tool's OTLP metric names to the normalized
// cost/token series. Register a tool by adding an entry below.
type devtoolAdapter struct {
	Key         string // astro.source value
	Label       string
	Icon        string // integration-icon key → themed logo (e.g. "anthropic")
	CostMetric  string
	TokenMetric string
}

var devtoolAdapters = []devtoolAdapter{
	{Key: "claude-code", Label: "Claude Code", Icon: "anthropic", CostMetric: "claude_code.cost.usage", TokenMetric: "claude_code.token.usage"},
}

// devtoolMatcher builds the account-scoped VM selector for one metric. VM's
// default OTLP ingestion preserves dots (confirmed empirically), so we select
// with quoted identifiers; if prod enables usePrometheusNaming, names and labels
// become underscored — switch here.
func devtoolMatcher(metric, source, accountID string) string {
	return fmt.Sprintf(`{__name__=%q, "astro.source"=%q, "astro.account_id"=%q}`, metric, source, accountID)
}

// computeDevtoolForInsights computes dev-tool usage for the ranges the Insights
// page folds in, attributing per-developer spend via the local member-email
// mirror (email → user_id, one indexed lookup — no per-request WorkOS calls).
// Best-effort: a nil metrics client (or any query failure) yields no data and
// Insights renders unchanged. On a table-only refresh (skip_ranges) just the
// widest range — the tables' window — is computed.
func computeDevtoolForInsights(ctx context.Context, log *logger.Logger, pc *promquery.Client, memberEmails *memberemails.Store, accountID string, skipRanges bool) map[string]DevtoolRange {
	if pc == nil {
		return nil
	}
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
	return computeDevtoolInsights(ctx, log, pc, accountID, emailToUserID, specs).Ranges
}

// computeDevtoolInsights builds each requested range's per-source block.
func computeDevtoolInsights(ctx context.Context, log *logger.Logger, pc *promquery.Client, accountID string, emailToUserID map[string]string, specs []insightsRangeSpec) DevtoolInsightsResponse {
	resp := DevtoolInsightsResponse{Ranges: map[string]DevtoolRange{}}
	if pc == nil {
		return resp // no metrics backend — graceful empty
	}
	now := time.Now()
	for _, spec := range specs {
		sources := map[string]DevtoolSource{}
		for _, ad := range devtoolAdapters {
			if src, ok := computeDevtoolSource(ctx, log, pc, ad, accountID, spec.days, now, emailToUserID); ok {
				sources[ad.Key] = src
			}
		}
		if len(sources) > 0 {
			resp.Ranges[spec.key] = DevtoolRange{Sources: sources}
		}
	}
	return resp
}

// computeDevtoolSource queries cost and tokens for one source/range; ok=false omits it.
func computeDevtoolSource(ctx context.Context, log *logger.Logger, pc *promquery.Client, ad devtoolAdapter, accountID string, days int, now time.Time, emailToUserID map[string]string) (DevtoolSource, bool) {
	start := now.AddDate(0, 0, -days)

	// cost is a cumulative Sum → increase() per day gives the daily delta.
	costQ := fmt.Sprintf("sum(increase(%s[1d]))", devtoolMatcher(ad.CostMetric, ad.Key, accountID))
	costSeries, err := pc.QueryRange(ctx, costQ, start, now, 24*time.Hour)
	if err != nil {
		log.Warn("devtool cost query failed", "source", ad.Key, "days", days, "error", err)
		return DevtoolSource{}, false
	}

	spend := make([]DevtoolSpendPoint, 0, days)
	var totalCost float64
	if len(costSeries) > 0 {
		for _, p := range costSeries[0].Points {
			if p.Value <= 0 {
				continue
			}
			spend = append(spend, DevtoolSpendPoint{Date: p.Timestamp.UTC().Format("2006-01-02"), CostUSD: round2(p.Value)})
			totalCost += p.Value
		}
	}

	tokenQ := fmt.Sprintf("sum(increase(%s[1d]))", devtoolMatcher(ad.TokenMetric, ad.Key, accountID))
	var totalTokens int
	if tokSeries, err := pc.QueryRange(ctx, tokenQ, start, now, 24*time.Hour); err != nil {
		log.Warn("devtool token query failed", "source", ad.Key, "days", days, "error", err)
	} else if len(tokSeries) > 0 {
		for _, p := range tokSeries[0].Points {
			if p.Value > 0 {
				totalTokens += int(p.Value + 0.5)
			}
		}
	}

	if len(spend) == 0 && totalTokens == 0 {
		return DevtoolSource{}, false // nothing to show for this source/range
	}

	totals := DevtoolTotals{CostUSD: round2(totalCost), Requests: 0, TotalTokens: totalTokens}
	return DevtoolSource{
		Label:      ad.Label,
		SpendByDay: spend,
		Totals:     totals,
		ByUser:     computeDevtoolByUser(ctx, log, pc, ad, accountID, days, emailToUserID),
		AgentRow:   devtoolAgentRow(ad, totals),
	}, true
}

// computeDevtoolByUser attributes a source's spend to developers by user.email.
// Best-effort: a query error yields no breakdown.
func computeDevtoolByUser(ctx context.Context, log *logger.Logger, pc *promquery.Client, ad devtoolAdapter, accountID string, days int, emailToUserID map[string]string) []DevtoolUserSpend {
	window := fmt.Sprintf("%dd", days)
	costQ := fmt.Sprintf(`sum by ("user.email") (increase(%s[%s]))`, devtoolMatcher(ad.CostMetric, ad.Key, accountID), window)
	costSamples, err := pc.Query(ctx, costQ)
	if err != nil {
		log.Warn("devtool per-user cost query failed", "source", ad.Key, "days", days, "error", err)
		return nil
	}

	tokQ := fmt.Sprintf(`sum by ("user.email") (increase(%s[%s]))`, devtoolMatcher(ad.TokenMetric, ad.Key, accountID), window)
	tokByEmail := map[string]int{}
	if tokSamples, err := pc.Query(ctx, tokQ); err == nil {
		for _, s := range tokSamples {
			if email := s.Labels["user.email"]; email != "" && s.Value > 0 {
				tokByEmail[email] = int(s.Value + 0.5)
			}
		}
	}

	out := make([]DevtoolUserSpend, 0, len(costSamples))
	for _, s := range costSamples {
		email := s.Labels["user.email"]
		if email == "" || s.Value <= 0 {
			continue
		}
		spend := DevtoolUserSpend{UserEmail: email, CostUSD: round2(s.Value), TotalTokens: tokByEmail[email]}
		if uid := emailToUserID[strings.ToLower(email)]; uid != "" {
			spend.UserID = uid
			spend.IdentityKey = "member:" + uid
		}
		out = append(out, spend)
	}
	return out
}

// devtoolAgentRow is the synthetic agents-table row for one dev-tool source.
func devtoolAgentRow(ad devtoolAdapter, totals DevtoolTotals) InsightsAgentRow {
	return InsightsAgentRow{
		Key:        ad.Key,
		SearchText: strings.ToLower(ad.Label),
		Identity: InsightsIdentityRef{
			Kind:    "system",
			Label:   ad.Label,
			Icon:    ad.Icon,
			Tooltip: "Aggregated local dev-tool usage (" + ad.Label + ") across developers, not a deployed agent.",
		},
		UsedBy: []InsightsIdentityRef{},
		Metrics: InsightsAgentMetrics{
			// No request-count metric is emitted, so request-derived fields stay 0;
			// the client rescales cost_pct against the combined total.
			Requests:       0,
			CostUSD:        totals.CostUSD,
			CostPct:        0,
			CostPerRequest: 0,
			TokPerRequest:  0,
			P95LatencyMs:   0,
		},
	}
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
