package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// InsightsRollupProducer turns one account-day of upstream telemetry into rows
// in insights_usage_daily. It lives in handlers rather than in insightsrollup
// because it reuses this package's Langfuse query helpers, dev-tool adapter
// registry, and identity classification — the same reason
// InsightsSummaryComputer is an interface injected by main.
//
// The unit of work is a single day, which is the whole point: a completed day
// is fetched once, ever, instead of being recomputed inside a 90-day window
// four times a day.
type InsightsRollupProducer struct {
	Log           *logger.Logger
	Cfg           *config.Config
	AccountStore  *account.AccountStore
	LangfuseStore *langfuse.Store
	SlackStore    *slackidentity.Store
	MemberEmails  *memberemails.Store
	Rollups       *insightsrollup.Store
}

// RollUpDay rolls every source for one account-day. It satisfies
// riverqueue.InsightsRollupProducer.
//
// Agent and dev-tool spend are rolled together because they share a day
// boundary and each is a full replace scoped to its own source, so a failure in
// one leaves the other's rows for that day intact and correct.
func (p *InsightsRollupProducer) RollUpDay(ctx context.Context, accountID string, day time.Time) error {
	acct, err := p.AccountStore.GetByID(accountID)
	if err != nil {
		return fmt.Errorf("insights rollup: load account %s: %w", accountID, err)
	}
	if acct == nil {
		// Account went away between discovery and this job. Not an error; the
		// facts are removed by the account_id FK cascade.
		return nil
	}
	if err := p.RollUpAgentsDay(ctx, acct, day); err != nil {
		return err
	}
	return p.RollUpDevtoolsDay(ctx, accountID, day)
}

// rollupQueryTimeout bounds one day's upstream fetch. Generous compared to the
// read path's 30s because nothing is waiting on it.
const rollupQueryTimeout = 60 * time.Second

// RollUpAgentsDay writes the 'usage' and 'model' grain rows for one account-day
// of agent spend, then returns. It does not touch the watermark — the caller
// owns that, because a watermark may only advance once every day behind it has
// committed.
//
// No deployment-tag filter is applied, deliberately. v1 filters to visible
// deployments and truncates at maxTagFilterValues, which silently drops spend on
// large accounts; here the tag arrives *in* the grouping key, so archived and
// deleted deployments can be stored and the read path decides what to show by
// LEFT JOINing deployments. That is what lets usage history outlive the agent
// that produced it.
func (p *InsightsRollupProducer) RollUpAgentsDay(ctx context.Context, acct *account.Account, day time.Time) error {
	creds, err := p.LangfuseStore.Get(acct.ID)
	if err != nil || creds == nil {
		// No Langfuse project for this account — nothing to roll up, and not an
		// error. Writing an empty day would be wrong: it would claim coverage.
		return nil
	}

	client := langfuse.NewClient(p.Cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)
	ctx, cancel := context.WithTimeout(ctx, rollupQueryTimeout)
	defer cancel()

	from, to := rollupDayBounds(day)

	usage, err := p.fetchUsageGrain(ctx, client, acct, day, from, to)
	if err != nil {
		return err
	}
	if err := p.Rollups.ReplaceDay(ctx, acct.ID, insightsrollup.GrainUsage, day, insightsrollup.SourceAgents, usage); err != nil {
		return err
	}

	models, err := p.fetchModelGrain(ctx, client, from, to)
	if err != nil {
		return err
	}
	return p.Rollups.ReplaceDay(ctx, acct.ID, insightsrollup.GrainModel, day, insightsrollup.SourceAgents, models)
}

// fetchUsageGrain reads the measure grain in a single query: the traces view
// grouped by [tags, userId], carrying cost, tokens and request count.
//
// Grouping by `tags` is safe to sum. Langfuse declares `tags` as a plain
// string[] dimension without `explodeArray`, so its query builder groups by the
// whole array rather than emitting arrayJoin — one row per distinct tag array,
// with each trace counted exactly once. Several arrays can carry the same
// deployment tag ([deployment:x] and [deployment:x, env:prod]); those rows are
// summed by the store's fold, which is correct.
func (p *InsightsRollupProducer) fetchUsageGrain(
	ctx context.Context,
	client *langfuse.Client,
	acct *account.Account,
	day time.Time,
	from, to string,
) ([]insightsrollup.Fact, error) {
	resp, err := client.GetMetrics(ctx, langfuse.MetricsQuery{
		View: "traces",
		Metrics: []langfuse.MetricsQueryField{
			{Measure: "totalCost", Aggregation: "sum"},
			{Measure: "totalTokens", Aggregation: "sum"},
			{Measure: "count", Aggregation: "count"},
		},
		// No TimeDimension: the query window *is* one day, so a per-bucket
		// timestamp would be redundant. last_seen resolves to the day itself,
		// which matches v1 — it already dropped to day granularity because
		// hourly buckets cost 24x for no product gain.
		Dimensions:    []langfuse.MetricsDimension{{Field: "tags"}, {Field: "userId"}},
		FromTimestamp: from,
		ToTimestamp:   to,
	})
	if err != nil {
		return nil, fmt.Errorf("insights rollup: usage grain: %w", err)
	}

	// Collapse linked Slack ids onto their WorkOS id before aggregating, exactly
	// as v1 does before writing its cache. Doing it here rather than at read
	// time keeps v2 byte-comparable with v1; a Slack account linked later is
	// picked up by the weekly reconcile re-rolling history.
	translateLinkedSlackUserIDs(p.Log, p.SlackStore, "insights-rollup", resp.Data)

	lastSeen := day.UTC()
	facts := make([]insightsrollup.Fact, 0, len(resp.Data))
	for _, row := range resp.Data {
		requests := int64(toInt(row["count_count"]))
		cost := toFloat(row["sum_totalCost"])
		tokens := int64(toInt(row["sum_totalTokens"]))
		if requests == 0 && cost == 0 && tokens == 0 {
			continue
		}

		kind, key := rollupActorFor(row["userId"])
		fact := insightsrollup.Fact{
			DeploymentID: p.deploymentIDFromTags(acct, row["tags"]),
			ActorKind:    kind,
			ActorKey:     key,
			Requests:     requests,
			TotalTokens:  tokens,
			CostUSD:      cost,
		}
		if requests > 0 {
			fact.LastSeenAt = lastSeen
		}
		facts = append(facts, fact)
	}
	return facts, nil
}

// deploymentIDFromTags extracts the deployment a tag array belongs to.
//
// The single-grain design assumes a trace carries at most one `deployment:`
// tag; if it carried two, its cost would land in both agents' rows and the
// table would over-report. That assumption is our own tagging convention rather
// than something Langfuse enforces, so it is asserted here instead of trusted.
// The first tag is still used so a violation degrades to mis-attribution rather
// than to dropped spend, but it is logged loudly enough to notice.
func (p *InsightsRollupProducer) deploymentIDFromTags(acct *account.Account, raw any) string {
	var found []string
	for _, tag := range tagStrings(raw) {
		if id := strings.TrimPrefix(tag, "deployment:"); id != tag && id != "" {
			found = append(found, id)
		}
	}
	switch len(found) {
	case 0:
		// Not an error: a trace with no deployment tag is real (SDK calls that
		// didn't tag, or spend outside a deployment). It aggregates under the
		// '' sentinel and shows up in account totals without an agent row.
		return ""
	case 1:
		return found[0]
	default:
		p.Log.Error("Insights rollup: trace carries multiple deployment tags — agent attribution is ambiguous",
			"account_id", acct.ID, "tags", strings.Join(found, ","), "using", found[0])
		return found[0]
	}
}

// fetchModelGrain reads per-model spend from the observations view, the only
// view exposing providedModelName.
//
// These rows are never summed with the 'usage' grain: observations-view cost
// does not reconcile with traces-view cost, which is already why v1's per-user
// chart reads the traces view. The grain column keeps them apart.
func (p *InsightsRollupProducer) fetchModelGrain(
	ctx context.Context,
	client *langfuse.Client,
	from, to string,
) ([]insightsrollup.Fact, error) {
	resp, err := client.GetMetrics(ctx, langfuse.MetricsQuery{
		View: "observations",
		Metrics: []langfuse.MetricsQueryField{
			{Measure: "totalCost", Aggregation: "sum"},
			{Measure: "inputTokens", Aggregation: "sum"},
			{Measure: "outputTokens", Aggregation: "sum"},
			{Measure: "count", Aggregation: "count"},
		},
		Dimensions:    []langfuse.MetricsDimension{{Field: "providedModelName"}},
		FromTimestamp: from,
		ToTimestamp:   to,
	})
	if err != nil {
		return nil, fmt.Errorf("insights rollup: model grain: %w", err)
	}

	facts := make([]insightsrollup.Fact, 0, len(resp.Data))
	for _, row := range resp.Data {
		model, _ := row["providedModelName"].(string)
		if model == "" {
			// Langfuse returns null for non-LLM observations. They carry no
			// model, and the shape CHECK forbids an empty model on this grain.
			continue
		}
		in := int64(toInt(row["sum_inputTokens"]))
		out := int64(toInt(row["sum_outputTokens"]))
		facts = append(facts, insightsrollup.Fact{
			Model:        model,
			Requests:     int64(toInt(row["count_count"])),
			InputTokens:  in,
			OutputTokens: out,
			TotalTokens:  in + out,
			CostUSD:      toFloat(row["sum_totalCost"]),
		})
	}
	return facts, nil
}

// RollUpDevtoolsDay writes 'usage' grain rows for each dev-tool source for one
// account-day: one row per identified developer, plus a source-level row
// carrying the remainder so account totals stay whole even when a developer's
// email doesn't resolve to a member.
//
// Dev-tool rows never populate deployment_id — there is no deployment — and
// their `requests` stays zero because no such metric is emitted. That zero is
// real data, which is why the read path guards per-request denominators.
func (p *InsightsRollupProducer) RollUpDevtoolsDay(ctx context.Context, accountID string, day time.Time) error {
	creds, err := p.LangfuseStore.Get(accountID)
	if err != nil || creds == nil {
		// No Langfuse project — nothing to roll up, and not an error.
		return nil
	}
	client := langfuse.NewClient(p.Cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)

	emailToUserID := map[string]string{}
	if p.MemberEmails != nil {
		if m, err := p.MemberEmails.EmailsForAccount(ctx, accountID); err == nil {
			emailToUserID = m
		} else {
			// Attribution degrades to unidentified rows rather than failing the
			// roll-up; total spend is still correct.
			p.Log.Warn("Insights rollup: member email lookup failed; dev-tool spend will be unattributed",
				"account_id", accountID, "error", err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, rollupQueryTimeout)
	defer cancel()

	for _, ad := range devtoolAdapters {
		facts, err := p.fetchDevtoolGrain(ctx, client, ad, accountID, day, emailToUserID)
		if err != nil {
			return err
		}
		if err := p.Rollups.ReplaceDay(ctx, accountID, insightsrollup.GrainUsage, day, ad.Key, facts); err != nil {
			return err
		}
	}
	return nil
}

func (p *InsightsRollupProducer) fetchDevtoolGrain(
	ctx context.Context,
	client *langfuse.Client,
	ad devtoolAdapter,
	accountID string,
	day time.Time,
	emailToUserID map[string]string,
) ([]insightsrollup.Fact, error) {
	from, to := rollupDayBounds(day)
	fromT, _ := time.Parse(time.RFC3339, from)
	toT, _ := time.Parse(time.RFC3339, to)

	usage, err := fetchDevtoolUsage(ctx, client, ad.Key, fromT, toT)
	if err != nil {
		return nil, err
	}
	total := usage.totals()
	if total.empty() {
		// Presence gate: the source wasn't used that day, so it contributes no rows.
		return nil, nil
	}
	if usage.costUnavailable() {
		p.Log.Warn("Insights rollup: dev-tool tokens reported but cost is zero; model is likely unpriced in Langfuse",
			"source", ad.Key, "account_id", accountID, "day", day.UTC().Format(time.DateOnly))
	}

	byUser := usage.byUser()
	facts := make([]insightsrollup.Fact, 0, len(byUser)+1)
	var attributed float64
	var attributedReq, attributedTok int64
	for email, b := range byUser {
		if b.empty() {
			continue
		}
		attributed += b.CostUSD
		attributedReq += int64(b.Requests)
		attributedTok += int64(b.Tokens)
		kind, key := insightsrollup.ActorKindUnidentified, email
		if userID, ok := emailToUserID[strings.ToLower(email)]; ok && userID != "" {
			// Same key space as agent spend for a member, which is exactly what
			// makes the two merge into one People row without a special case.
			kind, key = insightsrollup.ActorKindMember, userID
		}
		facts = append(facts, insightsrollup.Fact{
			ActorKind:   kind,
			ActorKey:    key,
			Requests:    int64(b.Requests),
			TotalTokens: int64(b.Tokens),
			CostUSD:     b.CostUSD,
			LastSeenAt:  day.UTC(),
		})
	}

	// Whatever the per-developer breakdown didn't account for stays on a system
	// row so the source's total is preserved. Without this, failing to attribute
	// a developer would quietly shrink account spend.
	remainder := total.CostUSD - attributed
	remReq := int64(total.Requests) - attributedReq
	remTok := int64(total.Tokens) - attributedTok
	if remainder > 0.0000005 || remReq > 0 || remTok > 0 {
		facts = append(facts, insightsrollup.Fact{
			ActorKind:   insightsrollup.ActorKindSystem,
			Requests:    max(remReq, 0),
			TotalTokens: max(remTok, 0),
			CostUSD:     max(remainder, 0),
			LastSeenAt:  day.UTC(),
		})
	}
	return facts, nil
}

// rollupActorFor maps a raw (already Slack-translated) Langfuse userId onto the
// stable identity columns.
//
// Slack actors store the bare Slack user id, not slack:<team>:<uid>. The team
// comes from the Slack directory, which is read-time enrichment — baking it in
// here would freeze a value that the directory can still learn, and the read
// path already composes the full key. Members store the WorkOS user id, which
// is the same key space dev-tool spend resolves to, and that is what lets the
// two merge into one row.
func rollupActorFor(raw any) (kind, key string) {
	userID, _ := raw.(string)
	userID = normalizeUserID(userID)
	if userID == "" {
		return insightsrollup.ActorKindSystem, ""
	}
	switch classifyUserID(userID) {
	case UserDetailsKindAstro:
		return insightsrollup.ActorKindMember, userID
	case UserDetailsKindSlack:
		return insightsrollup.ActorKindSlack, userID
	default:
		return insightsrollup.ActorKindUnidentified, userID
	}
}

// rollupDayBounds returns the [start, end) UTC instants of a day in the format
// the Langfuse metrics API expects.
func rollupDayBounds(day time.Time) (from, to string) {
	start := day.UTC().Truncate(24 * time.Hour)
	return metricsTimeRange(start.Format(time.RFC3339), start.Add(24*time.Hour).Format(time.RFC3339))
}
