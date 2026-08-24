package handlers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/langfuse"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/workclassifier"
)

// Lives in handlers to reuse the dev-tool adapter registry and member-email
// mapping, as InsightsRollupProducer does.
type ClassificationProducer struct {
	Log             *logger.Logger
	Cfg             *config.Config
	LangfuseStore   *langfuse.Store
	MemberEmails    *memberemails.Store
	Classifier      *workclassifier.Client
	Classifications *classification.Store
}

const (
	// Per-tick budgets so one backfill cannot monopolise the shared pool.
	maxDaysPerTick   = 7
	maxTracesPerTick = 20000

	tracePageSize = 100
	maxTracePages = 500

	// Terminates the backward walk for keys predating any retained telemetry.
	backfillFloorDays = 400

	classifyDayTimeout = 10 * time.Minute

	// Inference is spent before results persist, so a failing account would
	// otherwise re-spend a full day every tick.
	backoffAfterErrors = 3
	maxBackoff         = 24 * time.Hour
)

// Doubles per consecutive error up to maxBackoff, so a broken account still
// retries daily.
func backoffActive(state classification.State) bool {
	if state.ConsecutiveErrors < backoffAfterErrors || state.LastRunAt == nil {
		return false
	}
	wait := time.Hour << min(state.ConsecutiveErrors-backoffAfterErrors, 5)
	return time.Since(*state.LastRunAt) < min(wait, maxBackoff)
}

// Advances one account by at most a tick's budget. Failure is quiet and leaves
// the failed day outside the watermark, so that day retries while the days
// around it keep their progress.
func (p *ClassificationProducer) ClassifyAccount(ctx context.Context, accountID string) error {
	if p.Classifier == nil || p.Classifications == nil {
		return nil
	}
	if p.Classifier.ModelVersion() == "" {
		// Results would be rejected at write time, after inference was spent.
		p.Log.Error("classification: WORK_CLASSIFIER_VERSION unset; refusing to classify", "account_id", accountID)
		return nil
	}
	creds, err := p.LangfuseStore.Get(accountID)
	if err != nil || creds == nil {
		return nil
	}
	source := classification.SourceClaudeCode

	state, err := p.Classifications.GetState(ctx, accountID, source)
	if err != nil {
		return err
	}
	if backoffActive(state) {
		return nil
	}
	floor, err := p.Classifications.EarliestDataDay(ctx, accountID)
	if err != nil {
		return err
	}
	if floor == nil {
		// Never had an ingest key, so no dev-tool telemetry can exist.
		return nil
	}

	// After the local checks, so a backed-off or empty account costs no network.
	// Every axis, because a pass that reaches one head and not another discards
	// the inference it already paid for.
	for _, axis := range workclassifier.Axes {
		if err := p.Classifier.Ready(ctx, axis); err != nil {
			p.Log.Warn("classification: classifier not ready", "axis", axis, "account_id", accountID, "error", err)
			return nil
		}
	}

	// Account-scoped, so it is read once rather than per day of the tick.
	emailToUserID, err := p.MemberEmails.EmailsForAccount(ctx, accountID)
	if err != nil {
		p.Log.Warn("classification: member email lookup failed", "account_id", accountID, "error", err)
		emailToUserID = nil
	}

	client := langfuse.NewClient(p.Cfg.Deployment.LangfuseBaseURL, creds.PublicKey, creds.SecretKey)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	plan := planDays(state, today, *floor)

	// A day's fault is rarely the next day's, so the pass carries on and each
	// edge records only the contiguous ground behind it.
	budget := maxTracesPerTick
	covered := coverage{plan: &plan}
	var lastErr error
	for i, day := range plan.days {
		n, err := p.processDay(ctx, client, accountID, source, day, budget, emailToUserID)
		budget -= n
		// A day the tick budget capped is only partly labelled, so it counts as
		// uncovered and gets re-planned.
		exhausted := budget <= 0

		switch {
		case err != nil:
			lastErr = err
			p.Log.Warn("classification: day failed",
				"account_id", accountID, "day", day.Format(time.DateOnly), "error", err)
		case !exhausted:
			covered.record(i)
		}

		if err != nil && langfuse.IsAuthFailure(err) {
			p.Log.Error("classification: langfuse rejected the account's credentials; no day can be read until they are re-provisioned",
				"account_id", accountID, "error", err)
			break
		}
		if exhausted || ctx.Err() != nil {
			break
		}
	}

	plan.narrow(covered.forward, covered.backfill)
	if lastErr != nil {
		// Only a pass that covered nothing arms the backoff. One stuck day must
		// not throttle the days around it that do complete.
		stuck := covered.forward+covered.backfill == 0
		return p.Classifications.SetCursorsPartial(
			ctx, accountID, source, plan.through, plan.from, plan.complete, lastErr.Error(), stuck)
	}
	return p.Classifications.SetCursors(ctx, accountID, source, plan.through, plan.from, plan.complete)
}

type dayPlan struct {
	days []time.Time
	// Count of leading forward days in days; the remainder walk backward.
	forward  int
	through  *time.Time
	from     *time.Time
	complete bool
}

// coverage counts the completed days on each edge of a plan. Only a run that
// starts at its edge counts: a day completed after a gap cannot pull a cursor
// over the gap, because the cursor would then claim the day inside it.
type coverage struct {
	plan     *dayPlan
	forward  int
	backfill int
}

func (c *coverage) record(i int) {
	if i < c.plan.forward {
		if c.forward == i {
			c.forward++
		}
		return
	}
	if c.backfill == i-c.plan.forward {
		c.backfill++
	}
}

// narrow pulls each edge back to the days that actually completed. A day that
// failed, was capped, or was never reached has to stay outside the window:
// the forward walk starts at the high-water mark and the backward walk stops at
// the low one, so a day sealed inside is a day no later tick revisits.
func (pl *dayPlan) narrow(forwardDone, backfillDone int) {
	if forwardDone < pl.forward {
		pl.through = nil
		if forwardDone > 0 {
			pl.through = &pl.days[forwardDone-1]
		}
	}
	if backfillDone < len(pl.days)-pl.forward {
		pl.complete = false
		pl.from = nil
		if backfillDone > 0 {
			pl.from = &pl.days[pl.forward+backfillDone-1]
		}
	}
}

// Forward edge first, then backward, so recent days complete first. Today and
// yesterday always re-run because traces arrive late.
func planDays(state classification.State, today, floor time.Time) dayPlan {
	plan := dayPlan{complete: state.BackfillComplete}

	forwardStart := today.AddDate(0, 0, -1)
	if state.ClassifiedThrough != nil {
		if c := state.ClassifiedThrough.UTC().Truncate(24 * time.Hour); c.Before(forwardStart) {
			forwardStart = c
		}
	}
	if forwardStart.Before(floor) {
		forwardStart = floor
	}
	var lastForward time.Time
	for d := forwardStart; !d.After(today) && len(plan.days) < maxDaysPerTick; d = d.AddDate(0, 0, 1) {
		plan.days = append(plan.days, d)
		lastForward = d
	}
	plan.forward = len(plan.days)
	// Only ground actually covered: SetCursors takes GREATEST, so advancing past
	// a truncated walk would strand the skipped days.
	if !lastForward.IsZero() {
		plan.through = &lastForward
	}

	if state.BackfillComplete {
		return plan
	}

	oldest := forwardStart
	if state.BackfilledFrom != nil {
		if b := state.BackfilledFrom.UTC().Truncate(24 * time.Hour); b.Before(oldest) {
			oldest = b
		}
	}
	hardFloor := floor
	if f := today.AddDate(0, 0, -backfillFloorDays); f.After(hardFloor) {
		hardFloor = f
	}

	from := oldest
	for len(plan.days) < maxDaysPerTick {
		next := from.AddDate(0, 0, -1)
		if next.Before(hardFloor) {
			plan.complete = true
			break
		}
		plan.days = append(plan.days, next)
		from = next
	}
	plan.from = &from
	return plan
}

// processDay returns the number of prompts sent to inference.
func (p *ClassificationProducer) processDay(
	ctx context.Context,
	client *langfuse.Client,
	accountID, source string,
	day time.Time,
	budget int,
	emailToUserID map[string]string,
) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, classifyDayTimeout)
	defer cancel()

	// The pages that did arrive are still worth labelling. The day is reported
	// failed either way, so its cursor holds and the rest arrives next tick,
	// but the prompts already read are not read and inferred twice.
	traces, fetchErr := fetchDayTraces(ctx, client, source, day, budget)

	sent, err := p.classifyTraces(ctx, accountID, source, traces, budget)
	if err != nil {
		return sent, err
	}
	if fetchErr != nil {
		return sent, fetchErr
	}
	// Aggregation is a full replace, so it runs only over a complete day.
	if err := p.aggregateDay(ctx, client, accountID, source, day, emailToUserID); err != nil {
		return sent, err
	}
	return sent, nil
}

type promptTrace struct {
	id        string
	sessionID string
	prompt    string
	userEmail string
	at        time.Time
}

// The claude-code tag also matches tool/response records; user_prompt duplicates this.
var promptTraceNames = map[string]bool{
	"claude_code.interaction": true,
}

func fetchDayTraces(
	ctx context.Context,
	client *langfuse.Client,
	source string,
	day time.Time,
	budget int,
) ([]promptTrace, error) {
	start := day.UTC().Truncate(24 * time.Hour)
	from := start.Format(time.RFC3339)
	to := start.AddDate(0, 0, 1).Format(time.RFC3339)

	var out []promptTrace
	for offset := 0; ; offset += tracePageSize {
		resp, err := client.GetDevtoolTraces(ctx, source, from, to, tracePageSize, offset)
		if err != nil {
			return out, fmt.Errorf("classification: fetch traces: %w", err)
		}
		for _, tr := range resp.Data {
			if !promptTraceNames[tr.Name] {
				continue
			}
			text := promptText(tr.Input)
			if text == "" {
				// Content collection off, redacted at ingest, or not a prompt.
				continue
			}
			at, err := time.Parse(time.RFC3339, tr.Timestamp)
			if err != nil {
				at = start
			}
			out = append(out, promptTrace{
				id: tr.ID, sessionID: tr.SessionID, prompt: text, userEmail: tr.UserID, at: at,
			})
		}
		// Bounds the fetch itself, so a bad totalPages cannot page until timeout.
		if len(out) >= budget || offset/tracePageSize >= maxTracePages {
			break
		}
		if len(resp.Data) < tracePageSize || resp.Meta.Page >= resp.Meta.TotalPages {
			break
		}
	}
	return out, nil
}

// One session's prompts, newline-joined — the shape the heads were trained on.
type conversation struct {
	id     string
	traces []promptTrace
	text   string
}

// Past the predictor's own truncation point, so the rest is only request weight.
const maxConversationChars = 8000

// A prompt with no session id becomes a conversation of one. Midnight splits a session.
func groupConversations(traces []promptTrace) []conversation {
	order := make([]string, 0, len(traces))
	byID := map[string][]promptTrace{}
	for _, t := range traces {
		// Not "": that would fuse every unsessioned prompt into one conversation.
		key := t.sessionID
		if key == "" {
			key = t.id
		}
		if _, seen := byID[key]; !seen {
			order = append(order, key)
		}
		byID[key] = append(byID[key], t)
	}

	out := make([]conversation, 0, len(order))
	for _, id := range order {
		group := byID[id]
		sort.SliceStable(group, func(i, j int) bool { return group[i].at.Before(group[j].at) })
		var sb strings.Builder
		for i, t := range group {
			if sb.Len() >= maxConversationChars {
				break
			}
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString(t.prompt)
		}
		text := sb.String()
		if len(text) > maxConversationChars {
			text = text[:maxConversationChars]
		}
		out = append(out, conversation{id: id, traces: group, text: text})
	}
	return out
}

// Rows stay per prompt: the day's cost is apportioned by row count.
func (p *ClassificationProducer) classifyTraces(
	ctx context.Context,
	accountID, source string,
	traces []promptTrace,
	budget int,
) (int, error) {
	if len(traces) == 0 {
		return 0, nil
	}
	convs := groupConversations(traces)
	ids := make([]string, len(traces))
	for i, t := range traces {
		ids[i] = t.id
	}
	done, err := p.Classifications.ClassifiedAxes(
		ctx, accountID, source, p.Classifier.ModelVersion(), classification.UnitTurn, ids)
	if err != nil {
		return 0, err
	}

	// Per-axis, so a capped day covers the same prefix on every axis.
	perAxis := budget / len(workclassifier.Axes)

	// Each axis persists before the next one runs. Holding both until the end
	// meant a second-axis failure threw away inference the first axis had
	// already been billed for.
	var sent int
	for _, axis := range workclassifier.Axes {
		pending := make([]conversation, 0, len(convs))
		prompts := 0
		for _, c := range convs {
			if !conversationPending(c, done, axis) {
				continue
			}
			if prompts > 0 && prompts+len(c.traces) > perAxis {
				break
			}
			pending = append(pending, c)
			prompts += len(c.traces)
		}
		if len(pending) == 0 {
			continue
		}
		texts := make([]string, len(pending))
		for i, c := range pending {
			texts[i] = c.text
		}
		// Predictions are positional, and a partial batch returns a prefix, so
		// pending is trimmed to what came back.
		preds, err := p.Classifier.Classify(ctx, axis, texts)
		labelled := pending[:min(len(preds), len(pending))]
		for _, c := range labelled {
			sent += len(c.traces)
		}
		if saveErr := p.saveAxis(ctx, accountID, source, axis, labelled, preds); saveErr != nil {
			return sent, saveErr
		}
		if err != nil {
			return sent, err
		}
	}
	return sent, nil
}

func conversationPending(c conversation, done map[string]map[classification.Axis]bool, axis workclassifier.Axis) bool {
	for _, t := range c.traces {
		if !done[t.id][classification.Axis(axis)] {
			return true
		}
	}
	return false
}

// preds is positional against convs, and a prefix of it when a batch failed partway.
func (p *ClassificationProducer) saveAxis(
	ctx context.Context,
	accountID, source string,
	axis workclassifier.Axis,
	convs []conversation,
	preds []workclassifier.Prediction,
) error {
	if len(preds) == 0 {
		return nil
	}
	known := knownLabels(axis)
	results := make([]classification.Result, 0, len(preds))
	for i, c := range convs {
		if i >= len(preds) {
			break
		}
		pr := preds[i]
		label := pr.Label
		if !known[label] {
			// Refused conversations record the fallback; skipping re-sends them every tick.
			p.Log.Warn("classification: unusable label from classifier",
				"axis", axis, "label", pr.Label, "account_id", accountID)
			label = workclassifier.Fallback[axis]
		}
		for _, t := range c.traces {
			results = append(results, classification.Result{
				UnitKind:   classification.UnitTurn,
				UnitID:     t.id,
				Axis:       classification.Axis(axis),
				Label:      label,
				Score:      pr.Score,
				OccurredAt: t.at,
				UserEmail:  t.userEmail,
			})
		}
	}
	return p.Classifications.SaveResults(ctx, accountID, source, p.Classifier.ModelVersion(), results)
}

// Partitions the spend figure Insights reports, so segments sum to that page.
func (p *ClassificationProducer) aggregateDay(
	ctx context.Context,
	client *langfuse.Client,
	accountID, source string,
	day time.Time,
	emailToUserID map[string]string,
) error {
	counts, err := p.Classifications.CountsForDay(
		ctx, accountID, source, p.Classifier.ModelVersion(), classification.UnitTurn, day)
	if err != nil {
		return err
	}
	if len(counts) == 0 {
		return p.Classifications.ReplaceDayAggregates(ctx, accountID, day, source, nil)
	}

	// ReplaceDayAggregates is a full replace and a backfilled day is never
	// revisited, so writing on a failed lookup would zero its cost permanently.
	start := day.UTC().Truncate(24 * time.Hour)
	usage, err := fetchDevtoolUsage(ctx, client, source, start, start.AddDate(0, 0, 1))
	if err != nil {
		return fmt.Errorf("classification: devtool cost lookup: %w", err)
	}
	costByEmail := map[string]float64{}
	for email, b := range usage.byUser() {
		costByEmail[email] = b.CostUSD
	}

	facts := attributeCost(counts, costByEmail, emailToUserID)
	return p.Classifications.ReplaceDayAggregates(ctx, accountID, day, source, facts)
}

// attributeCost splits each developer's spend across their labels, per axis.
// Invariant: for any axis the attributed costs sum to the day's reported spend.
func attributeCost(
	counts []classification.LabelCount,
	costByEmail map[string]float64,
	emailToUserID map[string]string,
) []classification.DailyFact {
	totals := map[[2]string]int64{}
	for _, c := range counts {
		totals[[2]string{c.UserEmail, string(c.Axis)}] += c.Traces
	}

	facts := make([]classification.DailyFact, 0, len(counts))
	for _, c := range counts {
		total := totals[[2]string{c.UserEmail, string(c.Axis)}]
		var cost float64
		if total > 0 {
			cost = costByEmail[c.UserEmail] * (float64(c.Traces) / float64(total))
		}
		kind, key := devtoolActorFor(c.UserEmail, emailToUserID)
		facts = append(facts, classification.DailyFact{
			Axis:      c.Axis,
			Label:     c.Label,
			ActorKind: kind,
			ActorKey:  key,
			Traces:    c.Traces,
			CostUSD:   cost,
		})
	}
	return facts
}

// trace.input is typed any for agent payloads; a dev-tool prompt is always a string.
func promptText(input any) string {
	if s, ok := input.(string); ok {
		return s
	}
	return ""
}

func knownLabels(axis workclassifier.Axis) map[string]bool {
	out := make(map[string]bool, len(workclassifier.Labels[axis]))
	for _, l := range workclassifier.Labels[axis] {
		out[l] = true
	}
	return out
}
