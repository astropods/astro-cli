package handlers

import (
	"math"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/workclassifier"
)

var (
	clsToday = time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	clsFloor = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
)

func dayStrs(days []time.Time) []string {
	out := make([]string, len(days))
	for i, d := range days {
		out[i] = d.Format(time.DateOnly)
	}
	return out
}

func ptrDay(s string) *time.Time {
	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return &d
}

// A fresh account starts at the forward edge and immediately begins walking
// backward, so the newest day is complete after the first tick.
func TestPlanDaysFirstRunStartsAtForwardEdge(t *testing.T) {
	plan := planDays(classification.State{}, clsToday, clsFloor)

	got := dayStrs(plan.days)
	if len(got) != maxDaysPerTick {
		t.Fatalf("got %d days (%v), want %d", len(got), got, maxDaysPerTick)
	}
	// Forward edge first: yesterday then today, before any backfill.
	if got[0] != "2026-08-10" || got[1] != "2026-08-11" {
		t.Errorf("forward edge = %v, want [2026-08-10 2026-08-11]", got[:2])
	}
	// Then backward from the forward edge.
	if got[2] != "2026-08-09" || got[len(got)-1] != "2026-08-05" {
		t.Errorf("backfill = %v, want 08-09 down to 08-05", got[2:])
	}
	if plan.through == nil || plan.through.Format(time.DateOnly) != "2026-08-11" {
		t.Errorf("through = %v, want today", plan.through)
	}
	if plan.from == nil || plan.from.Format(time.DateOnly) != "2026-08-05" {
		t.Errorf("from = %v, want 2026-08-05", plan.from)
	}
	if plan.complete {
		t.Error("backfill should not be complete after one tick")
	}
}

// Late-arriving traces are the norm — laptops sleep, exporters retry — so the
// forward edge must re-cover yesterday even once it is marked classified.
func TestPlanDaysAlwaysRerunsYesterdayAndToday(t *testing.T) {
	plan := planDays(classification.State{
		ClassifiedThrough: ptrDay("2026-08-11"),
		BackfilledFrom:    ptrDay("2026-01-01"),
		BackfillComplete:  true,
	}, clsToday, clsFloor)

	if got := dayStrs(plan.days); len(got) != 2 || got[0] != "2026-08-10" || got[1] != "2026-08-11" {
		t.Fatalf("got %v, want [2026-08-10 2026-08-11]", got)
	}
}

// Once the backfill is done the walk must stop, or every tick re-reads history.
func TestPlanDaysCompleteBackfillDoesNotWalkBack(t *testing.T) {
	plan := planDays(classification.State{
		ClassifiedThrough: ptrDay("2026-08-11"),
		BackfilledFrom:    ptrDay("2026-01-01"),
		BackfillComplete:  true,
	}, clsToday, clsFloor)

	if plan.from != nil {
		t.Errorf("from = %v, want nil (no backward movement)", plan.from)
	}
	if !plan.complete {
		t.Error("complete should stay true")
	}
}

// The floor is the earliest ingest key: telemetry cannot predate it, so the
// walk terminates there and marks itself done.
func TestPlanDaysStopsAtFloorAndMarksComplete(t *testing.T) {
	floor := clsToday.AddDate(0, 0, -3)
	plan := planDays(classification.State{}, clsToday, floor)

	got := dayStrs(plan.days)
	for _, d := range got {
		if d < floor.Format(time.DateOnly) {
			t.Errorf("day %s is below the floor %s", d, floor.Format(time.DateOnly))
		}
	}
	if !plan.complete {
		t.Error("reaching the floor should mark the backfill complete")
	}
}

// An account whose key predates any retained telemetry must still terminate.
func TestPlanDaysHardFloorBoundsAncientKeys(t *testing.T) {
	ancient := clsToday.AddDate(-5, 0, 0)
	state := classification.State{BackfilledFrom: ptrDay("2025-01-01")}
	plan := planDays(state, clsToday, ancient)

	limit := clsToday.AddDate(0, 0, -backfillFloorDays)
	for _, d := range plan.days {
		if d.Before(limit) {
			t.Errorf("day %s is below the hard floor %s", d.Format(time.DateOnly), limit.Format(time.DateOnly))
		}
	}
}

func TestPlanDaysNeverExceedsTickBudget(t *testing.T) {
	plan := planDays(classification.State{BackfilledFrom: ptrDay("2026-06-01")}, clsToday, clsFloor)
	if len(plan.days) > maxDaysPerTick {
		t.Fatalf("planned %d days, want at most %d", len(plan.days), maxDaysPerTick)
	}
}

// The exhausted day is only partly labelled, so the floor must stop above it —
// planDays never walks back over anything at or below backfilled_from.
func TestTruncateAtExcludesTheExhaustedDay(t *testing.T) {
	plan := dayPlan{from: ptrDay("2026-08-05"), complete: true}
	plan.truncateAt(*ptrDay("2026-08-08"))

	if got := plan.from.Format(time.DateOnly); got != "2026-08-09" {
		t.Errorf("from = %s, want 2026-08-09 (the last fully complete day)", got)
	}
	if plan.complete {
		t.Error("a truncated tick cannot claim the backfill is complete")
	}
}

// The invariant the whole cost design rests on: per axis, attributed cost sums
// to the spend Insights reports. If this drifts, the detail page contradicts the
// page that links to it.
func TestAttributeCostSumsToSpendPerAxis(t *testing.T) {
	counts := []classification.LabelCount{
		{Axis: classification.AxisPurpose, Label: "work", UserEmail: "a@x.com", Traces: 8},
		{Axis: classification.AxisPurpose, Label: "personal", UserEmail: "a@x.com", Traces: 2},
		{Axis: classification.AxisTopic, Label: "software-engineering", UserEmail: "a@x.com", Traces: 7},
		{Axis: classification.AxisTopic, Label: "marketing", UserEmail: "a@x.com", Traces: 3},
		{Axis: classification.AxisPurpose, Label: "work", UserEmail: "b@x.com", Traces: 5},
	}
	cost := map[string]float64{"a@x.com": 10.0, "b@x.com": 4.0}

	facts := attributeCost(counts, cost, nil)

	perAxis := map[classification.Axis]float64{}
	for _, f := range facts {
		perAxis[f.Axis] += f.CostUSD
	}
	if got := perAxis[classification.AxisPurpose]; math.Abs(got-14.0) > 1e-9 {
		t.Errorf("purpose total = %v, want 14.0 (both developers)", got)
	}
	// Topic only has a@x.com's prompts, so it totals that developer's spend.
	if got := perAxis[classification.AxisTopic]; math.Abs(got-10.0) > 1e-9 {
		t.Errorf("topic total = %v, want 10.0", got)
	}

	// Shares are proportional within a developer: 8/10 of $10.
	for _, f := range facts {
		if f.Axis == classification.AxisPurpose && f.Label == "work" && f.ActorKey == "a@x.com" {
			if math.Abs(f.CostUSD-8.0) > 1e-9 {
				t.Errorf("a@x.com work = %v, want 8.0", f.CostUSD)
			}
		}
	}
}

// A developer with labels but no metrics (or vice versa) must not produce NaN.
func TestAttributeCostHandlesMissingSpend(t *testing.T) {
	facts := attributeCost([]classification.LabelCount{
		{Axis: classification.AxisPurpose, Label: "work", UserEmail: "ghost@x.com", Traces: 3},
	}, map[string]float64{}, nil)

	if len(facts) != 1 {
		t.Fatalf("got %d facts, want 1", len(facts))
	}
	if facts[0].CostUSD != 0 || math.IsNaN(facts[0].CostUSD) {
		t.Errorf("cost = %v, want 0", facts[0].CostUSD)
	}
	if facts[0].Traces != 3 {
		t.Errorf("traces = %d, want 3 (label counts survive missing spend)", facts[0].Traces)
	}
}

func TestClassificationActorFor(t *testing.T) {
	members := map[string]string{"dev@x.com": "user_123"}

	// Bare id, matching insights_usage_daily — the "member:" prefix is the API
	// response key space, not the fact table's.
	if kind, key := devtoolActorFor("dev@x.com", members); kind != "member" || key != "user_123" {
		t.Errorf("member = (%q,%q), want (member, user_123)", kind, key)
	}
	// Member emails are stored lowercased; a mixed-case Langfuse userId must
	// still resolve or that developer never merges into their People row.
	if kind, key := devtoolActorFor("Dev@X.com", members); kind != "member" || key != "user_123" {
		t.Errorf("mixed-case = (%q,%q), want (member, user_123)", kind, key)
	}
	if kind, key := devtoolActorFor("stranger@x.com", members); kind != "unidentified" || key != "stranger@x.com" {
		t.Errorf("non-member = (%q,%q), want (unidentified, stranger@x.com)", kind, key)
	}
	if kind, key := devtoolActorFor("", members); kind != "unidentified" || key != "" {
		t.Errorf("empty = (%q,%q), want (unidentified, \"\")", kind, key)
	}
}

// trace.input is a plain string for dev-tool traces, but the field is typed
// `any` because agent traces carry structured payloads.
func TestPromptText(t *testing.T) {
	if got := promptText("fix the tests"); got != "fix the tests" {
		t.Errorf("string = %q", got)
	}
	if got := promptText(nil); got != "" {
		t.Errorf("nil = %q, want empty", got)
	}
	if got := promptText(map[string]any{"role": "user"}); got != `{"role":"user"}` {
		t.Errorf("object = %q", got)
	}
}

// A capped day must not be sealed behind the backfill floor while only one axis
// was labelled — planDays never revisits anything at or below backfilled_from.
func TestPlanDaysDoesNotResealATruncatedDay(t *testing.T) {
	plan := planDays(classification.State{BackfilledFrom: ptrDay("2026-08-05")}, clsToday, clsFloor)
	plan.truncateAt(*ptrDay("2026-08-07"))

	next := planDays(classification.State{
		ClassifiedThrough: plan.through,
		BackfilledFrom:    plan.from,
		BackfillComplete:  plan.complete,
	}, clsToday, clsFloor)

	for _, d := range next.days {
		if d.Format(time.DateOnly) == "2026-08-07" {
			return // the partial day is re-planned
		}
	}
	t.Errorf("2026-08-07 was truncated but never re-planned; got %v", dayStrs(next.days))
}

// After a long outage the forward edge must stay inside the tick budget, or one
// pass tries every day since and dies on the worker timeout mid-run.
func TestPlanDaysBoundsAStaleForwardEdge(t *testing.T) {
	plan := planDays(classification.State{
		ClassifiedThrough: ptrDay("2026-05-01"),
		BackfillComplete:  true,
	}, clsToday, clsFloor)

	if len(plan.days) > maxDaysPerTick {
		t.Fatalf("planned %d days from a stale watermark, want at most %d", len(plan.days), maxDaysPerTick)
	}
	// The cursor may only advance over ground actually covered. Jumping to
	// today would strand every skipped day: the backfill walks backward from
	// the forward edge and never revisits the middle.
	last := plan.days[len(plan.days)-1]
	if plan.through == nil || !plan.through.Equal(last) {
		t.Fatalf("through = %v, want %v (the last day actually planned)", plan.through, last)
	}
}

// Ticking repeatedly from a stale watermark must eventually cover every day —
// no permanent hole between the old watermark and today.
func TestPlanDaysStaleWatermarkLeavesNoGap(t *testing.T) {
	state := classification.State{
		ClassifiedThrough: ptrDay("2026-07-20"),
		BackfilledFrom:    ptrDay("2026-07-20"),
		BackfillComplete:  true,
	}
	seen := map[string]bool{}
	for tick := 0; tick < 10; tick++ {
		plan := planDays(state, clsToday, clsFloor)
		for _, d := range plan.days {
			seen[d.Format(time.DateOnly)] = true
		}
		state.ClassifiedThrough = plan.through
		if plan.from != nil {
			state.BackfilledFrom = plan.from
		}
		state.BackfillComplete = plan.complete
	}
	for d := *ptrDay("2026-07-20"); !d.After(clsToday); d = d.AddDate(0, 0, 1) {
		if !seen[d.Format(time.DateOnly)] {
			t.Errorf("%s was never planned across 10 ticks", d.Format(time.DateOnly))
		}
	}
}

// Inference is spent before results are persisted, so a deterministically
// failing account must not re-spend a full day every tick.
func TestBackoffActive(t *testing.T) {
	recent := time.Now().Add(-time.Minute)
	old := time.Now().Add(-48 * time.Hour)

	if backoffActive(classification.State{ConsecutiveErrors: 1, LastRunAt: &recent}) {
		t.Error("a single failure should retry on the next tick")
	}
	if !backoffActive(classification.State{ConsecutiveErrors: 5, LastRunAt: &recent}) {
		t.Error("a repeatedly failing account should be held off")
	}
	if backoffActive(classification.State{ConsecutiveErrors: 99, LastRunAt: &old}) {
		t.Error("backoff is capped, so a broken account still retries daily")
	}
	if backoffActive(classification.State{ConsecutiveErrors: 9}) {
		t.Error("no LastRunAt means nothing to back off from")
	}
}

// An out-of-space label would fail the whole batch at write time, re-spending
// the day's inference on the next tick.
func TestKnownLabelsGuardsTheOutputSpace(t *testing.T) {
	known := knownLabels(workclassifier.AxisPurpose)
	if !known["work"] || !known["personal"] || !known["ambiguous"] {
		t.Errorf("purpose space incomplete: %v", known)
	}
	if known["software-engineering"] {
		t.Error("topic labels must not validate against the purpose axis")
	}
}

// The unknown-label fallback has to be a real member of its own axis's space,
// or the row it writes fails the batch it was meant to rescue. purpose has no
// "other" — its undetermined member is "ambiguous".
func TestFallbackLabelIsInItsAxisSpace(t *testing.T) {
	for _, axis := range workclassifier.Axes {
		fallback, ok := workclassifier.Fallback[axis]
		if !ok {
			t.Errorf("axis %q has no fallback label", axis)
			continue
		}
		if !knownLabels(axis)[fallback] {
			t.Errorf("axis %q fallback %q is not in its own label space", axis, fallback)
		}
	}
}
