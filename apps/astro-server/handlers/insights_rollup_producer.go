package handlers

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
// registry, and identity classification. main injects it through an interface so
// riverqueue never imports handlers.
//
// The unit of storage is a single day, which is the whole point: a completed
// day is fetched once, ever, rather than re-aggregated inside a 90-day window
// on every refresh. The unit of *fetch* is a window of days, because Langfuse
// buckets a range query by day and one request answers all of them.
type InsightsRollupProducer struct {
	Log           *logger.Logger
	Cfg           *config.Config
	AccountStore  *account.AccountStore
	LangfuseStore *langfuse.Store
	SlackStore    *slackidentity.Store
	MemberEmails  *memberemails.Store
	Rollups       *insightsrollup.Store
}

// RollUpRange rolls every source for a consecutive run of account-days. It
// satisfies riverqueue.InsightsRollupProducer.
//
// The window is the unit of *fetch*, not of storage: Langfuse buckets a range
// query by day, so one request answers every day in it and the day stays the
// unit written. Asking per day instead costs a round trip per day, which is
// what made a 90-day backfill minutes long.
//
// Days are passed explicitly rather than as bounds because every one of them
// must be replaced, including the days the response has no rows for. Writing
// only the days that came back would leave stale facts on a day whose spend
// went to zero.
//
// Agent and dev-tool spend are rolled together because they share a day
// boundary and each is a full replace scoped to its own source, so a failure in
// one leaves the other's rows for those days intact and correct.
func (p *InsightsRollupProducer) RollUpRange(ctx context.Context, accountID string, days []time.Time) error {
	if len(days) == 0 {
		return nil
	}
	acct, err := p.AccountStore.GetByID(accountID)
	if errors.Is(err, account.ErrAccountNotFound) {
		// Deleted between discovery and this job. Reported as ErrAccountGone so
		// the worker stops the whole account rather than retrying a lookup that
		// can only keep failing.
		return fmt.Errorf("%w: %s", insightsrollup.ErrAccountGone, accountID)
	}
	if err != nil {
		return fmt.Errorf("insights rollup: load account %s: %w", accountID, err)
	}
	agentsStart := time.Now()
	if err := p.RollUpAgentsRange(ctx, acct, days); err != nil {
		return err
	}
	agentsDuration := time.Since(agentsStart)

	devtoolsStart := time.Now()
	err = p.RollUpDevtoolsRange(ctx, accountID, days)
	p.Log.Debug("insights rollup: window sources rolled",
		"account_id", accountID,
		"first_day", days[0].UTC().Format(time.DateOnly),
		"last_day", days[len(days)-1].UTC().Format(time.DateOnly),
		"day_count", len(days),
		"agents_duration", agentsDuration.Round(time.Millisecond).String(),
		"devtools_duration", time.Since(devtoolsStart).Round(time.Millisecond).String())
	return err
}

// rollupQueryTimeout bounds one window's upstream fetch. Generous compared to
// the read path's 30s because nothing is waiting on it.
const rollupQueryTimeout = 60 * time.Second

// timedMetrics reports what one upstream query cost. A roll-up is almost
// entirely these calls, so their count and latency are what explain a run that
// runs out of time.
func (p *InsightsRollupProducer) timedMetrics(
	ctx context.Context,
	client *langfuse.Client,
	grain string,
	q langfuse.MetricsQuery,
) (*langfuse.MetricsResponse, error) {
	start := time.Now()
	resp, err := client.GetMetrics(ctx, q)
	rows := 0
	if resp != nil {
		rows = len(resp.Data)
	}
	p.Log.Debug("insights rollup: langfuse query",
		"grain", grain,
		"view", q.View,
		"from", q.FromTimestamp,
		"rows", rows,
		"duration", time.Since(start).Round(time.Millisecond).String(),
		"failed", err != nil)
	return resp, err
}

// RollUpAgentsRange writes the 'usage' and 'model' grain rows for a window of
// agent spend, then returns. It does not touch the watermark — the caller owns
// that, because a watermark may only advance once every day behind it has
// committed.
//
// No deployment-tag filter is applied, deliberately. v1 filters to visible
// deployments and truncates at maxTagFilterValues, which silently drops spend on
// large accounts; here the tag arrives *in* the grouping key, so archived and
// deleted deployments can be stored and the read path decides what to show by
// LEFT JOINing deployments. That is what lets usage history outlive the agent
// that produced it.
func (p *InsightsRollupProducer) RollUpAgentsRange(ctx context.Context, acct *account.Account, days []time.Time) error {
	creds, err := p.LangfuseStore.Get(acct.ID)
	if err != nil || creds == nil {
		// No Langfuse project for this account — nothing to roll up, and not an
		// error. Writing empty days would be wrong: it would claim coverage.
		return nil
	}

	client := langfuse.NewClient(p.Cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)
	ctx, cancel := context.WithTimeout(ctx, rollupQueryTimeout)
	defer cancel()

	from, to := rollupRangeBounds(days)

	usage, err := p.fetchUsageGrain(ctx, client, acct, from, to)
	if err != nil {
		return err
	}
	models, err := p.fetchModelGrain(ctx, client, from, to)
	if err != nil {
		return err
	}

	for _, day := range days {
		d := day.UTC().Format(time.DateOnly)
		if err := p.Rollups.ReplaceDay(ctx, acct.ID, insightsrollup.GrainUsage, day, insightsrollup.SourceAgents, usage[d]); err != nil {
			return err
		}
		if err := p.Rollups.ReplaceDay(ctx, acct.ID, insightsrollup.GrainModel, day, insightsrollup.SourceAgents, models[d]); err != nil {
			return err
		}
	}
	return nil
}

func (p *InsightsRollupProducer) fetchUsageGrain(
	ctx context.Context,
	client *langfuse.Client,
	acct *account.Account,
	from, to string,
) (map[string][]insightsrollup.Fact, error) {
	// A day time-dimension makes each row one (day, tags, userId) cell, so one
	// request covers the whole window. Day is the finest bucket worth asking
	// for: last_seen resolves to the day itself, matching v1, which already
	// dropped to day granularity because hourly buckets cost 24x for no product
	// gain.
	dims := []langfuse.MetricsDimension{{Field: "tags"}, {Field: "userId"}}
	byDay := &langfuse.TimeDimension{Granularity: "day"}

	countResp, err := p.timedMetrics(ctx, client, "usage:count", langfuse.MetricsQuery{
		View:          "traces",
		Metrics:       []langfuse.MetricsQueryField{{Measure: "count", Aggregation: "count"}},
		Dimensions:    dims,
		TimeDimension: byDay,
		FromTimestamp: from,
		ToTimestamp:   to,
	})
	if err != nil {
		return nil, fmt.Errorf("insights rollup: usage grain (count): %w", err)
	}
	usageResp, err := p.timedMetrics(ctx, client, "usage:cost_tokens", langfuse.MetricsQuery{
		View: "traces",
		Metrics: []langfuse.MetricsQueryField{
			{Measure: "totalCost", Aggregation: "sum"},
			{Measure: "totalTokens", Aggregation: "sum"},
		},
		Dimensions:    dims,
		TimeDimension: byDay,
		FromTimestamp: from,
		ToTimestamp:   to,
	})
	if err != nil {
		return nil, fmt.Errorf("insights rollup: usage grain (cost/tokens): %w", err)
	}

	translateLinkedSlackUserIDs(p.Log, p.SlackStore, "insights-rollup", countResp.Data, usageResp.Data)

	usageByGroup := make(map[string]map[string]any, len(usageResp.Data))
	for _, row := range usageResp.Data {
		usageByGroup[usageGroupKey(row)] = row
	}

	byDate := make(map[string][]insightsrollup.Fact)
	seen := make(map[string]bool, len(countResp.Data))
	for _, row := range countResp.Data {
		group := usageGroupKey(row)
		seen[group] = true
		if devtoolTagged(row["tags"]) {
			continue
		}

		requests := int64(toInt(row["count_count"]))
		var cost float64
		var tokens int64
		if u, ok := usageByGroup[group]; ok {
			cost = toFloat(u["sum_totalCost"])
			tokens = int64(toInt(u["sum_totalTokens"]))
		}
		if requests == 0 && cost == 0 && tokens == 0 {
			continue
		}

		date := dateFromTimeDim(row[langfuseTimeDimensionKey])
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
			fact.LastSeenAt = dayInstant(date)
		}
		byDate[date] = append(byDate[date], fact)
	}

	for _, row := range usageResp.Data {
		group := usageGroupKey(row)
		if seen[group] || devtoolTagged(row["tags"]) {
			continue
		}
		cost := toFloat(row["sum_totalCost"])
		tokens := int64(toInt(row["sum_totalTokens"]))
		if cost == 0 && tokens == 0 {
			continue
		}
		date := dateFromTimeDim(row[langfuseTimeDimensionKey])
		kind, key := rollupActorFor(row["userId"])
		byDate[date] = append(byDate[date], insightsrollup.Fact{
			DeploymentID: p.deploymentIDFromTags(acct, row["tags"]),
			ActorKind:    kind,
			ActorKey:     key,
			TotalTokens:  tokens,
			CostUSD:      cost,
		})
	}
	return byDate, nil
}

// usageGroupKey identifies a [day, tags, userId] group so rows from the count
// and cost/tokens queries in fetchUsageGrain can be joined back together. The
// day is part of the key because a range query returns one row per day per
// group; without it the two responses would join across days and every group
// would collapse onto whichever day happened to be last.
//
// Tags are sorted before joining since group identity shouldn't depend on the
// order Langfuse happened to return them in.
func usageGroupKey(row map[string]any) string {
	tags := append([]string{}, tagStrings(row["tags"])...)
	sort.Strings(tags)
	userID, _ := row["userId"].(string)
	return dateFromTimeDim(row[langfuseTimeDimensionKey]) + "\x1e" +
		strings.Join(tags, "\x1f") + "\x1e" + userID
}

// dayInstant parses a YYYY-MM-DD bucket back to the UTC midnight the fact table
// stores. An unparseable bucket yields the zero time, which ReplaceDay writes
// as a NULL last_seen rather than a wrong date.
func dayInstant(date string) time.Time {
	t, err := time.ParseInLocation(time.DateOnly, date, time.UTC)
	if err != nil {
		return time.Time{}
	}
	return t
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
		p.Log.Error("insights rollup: trace carries multiple deployment tags, agent attribution is ambiguous",
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
) (map[string][]insightsrollup.Fact, error) {
	resp, err := p.timedMetrics(ctx, client, "model", langfuse.MetricsQuery{
		View: "observations",
		Metrics: []langfuse.MetricsQueryField{
			{Measure: "totalCost", Aggregation: "sum"},
			{Measure: "inputTokens", Aggregation: "sum"},
			{Measure: "outputTokens", Aggregation: "sum"},
			{Measure: "count", Aggregation: "count"},
		},
		Dimensions:    []langfuse.MetricsDimension{{Field: "providedModelName"}},
		TimeDimension: &langfuse.TimeDimension{Granularity: "day"},
		FromTimestamp: from,
		ToTimestamp:   to,
	})
	if err != nil {
		return nil, fmt.Errorf("insights rollup: model grain: %w", err)
	}

	byDate := make(map[string][]insightsrollup.Fact)
	for _, row := range resp.Data {
		model, _ := row["providedModelName"].(string)
		if model == "" {
			// Langfuse returns null for non-LLM observations. They carry no
			// model, and the shape CHECK forbids an empty model on this grain.
			continue
		}
		in := int64(toInt(row["sum_inputTokens"]))
		out := int64(toInt(row["sum_outputTokens"]))
		date := dateFromTimeDim(row[langfuseTimeDimensionKey])
		byDate[date] = append(byDate[date], insightsrollup.Fact{
			Model:        model,
			Requests:     int64(toInt(row["count_count"])),
			InputTokens:  in,
			OutputTokens: out,
			TotalTokens:  in + out,
			CostUSD:      toFloat(row["sum_totalCost"]),
		})
	}
	return byDate, nil
}

// RollUpDevtoolsDay writes 'usage' grain rows for each dev-tool source for one
// account-day: one row per identified developer, plus a source-level row
// carrying the remainder so account totals stay whole even when a developer's
// email doesn't resolve to a member.
//
// Dev-tool rows never populate deployment_id — there is no deployment — and
// their `requests` stays zero because no such metric is emitted. That zero is
// real data, which is why the read path guards per-request denominators.
func (p *InsightsRollupProducer) RollUpDevtoolsRange(ctx context.Context, accountID string, days []time.Time) error {
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
			p.Log.Warn("insights rollup: member email lookup failed, dev-tool spend will be unattributed",
				"account_id", accountID, "error", err)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, rollupQueryTimeout)
	defer cancel()

	windowStart := days[0].UTC().Truncate(24 * time.Hour)
	windowEnd := days[len(days)-1].UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)

	for _, ad := range devtoolAdapters {
		start := time.Now()
		usage, err := fetchDevtoolUsage(ctx, client, ad.Key, windowStart, windowEnd)
		p.Log.Debug("insights rollup: devtool source fetched",
			"account_id", accountID,
			"source", ad.Key,
			"first_day", windowStart.Format(time.DateOnly),
			"day_count", len(days),
			"cells", len(usage.Cells),
			"duration", time.Since(start).Round(time.Millisecond).String(),
			"failed", err != nil)
		if err != nil {
			return err
		}
		byDate := usage.byDate()
		for _, day := range days {
			d := day.UTC().Format(time.DateOnly)
			facts := p.devtoolFactsFor(byDate[d], ad, accountID, d, emailToUserID)
			if err := p.Rollups.ReplaceDay(ctx, accountID, insightsrollup.GrainUsage, day, ad.Key, facts); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *InsightsRollupProducer) devtoolFactsFor(
	usage devtoolUsage,
	ad devtoolAdapter,
	accountID string,
	date string,
	emailToUserID map[string]string,
) []insightsrollup.Fact {
	total := usage.totals()
	if total.empty() {
		// Presence gate: the source wasn't used that day, so it contributes no rows.
		return nil
	}
	if usage.costUnavailable() {
		p.Log.Warn("insights rollup: dev-tool tokens reported but cost is zero, model is likely unpriced upstream",
			"source", ad.Key, "account_id", accountID, "day", date)
	}
	day := dayInstant(date)

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
		kind, key := devtoolActorFor(email, emailToUserID)
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
	return facts
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

// rollupRangeBounds returns the [start, end) UTC instants spanning a window of
// days, in the format the Langfuse metrics API expects. Days are consecutive
// and oldest-first, so the bounds are the first day's midnight and the last
// day's midnight plus one.
func rollupRangeBounds(days []time.Time) (from, to string) {
	start := days[0].UTC().Truncate(24 * time.Hour)
	end := days[len(days)-1].UTC().Truncate(24*time.Hour).AddDate(0, 0, 1)
	return metricsTimeRange(start.Format(time.RFC3339), end.Format(time.RFC3339))
}
