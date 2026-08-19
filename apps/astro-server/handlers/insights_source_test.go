package handlers

import (
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/workclassifier"
)

func aggRow(day, axis, label string, traces int64, cost float64) classification.AggRow {
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		panic(err)
	}
	return classification.AggRow{
		Day:       d,
		Axis:      classification.Axis(axis),
		Label:     label,
		ActorKind: "member",
		ActorKey:  "user_1",
		Traces:    traces,
		CostUSD:   cost,
	}
}

func labelByKey(axis InsightsSourceAxis, key string) (InsightsSourceLabel, bool) {
	for _, l := range axis.Labels {
		if l.Key == key {
			return l, true
		}
	}
	return InsightsSourceLabel{}, false
}

func TestBuildSourceAxisTotalsAndShares(t *testing.T) {
	rows := []classification.AggRow{
		aggRow("2026-08-10", "purpose", "work", 8, 4.00),
		aggRow("2026-08-11", "purpose", "work", 4, 2.00),
		aggRow("2026-08-11", "purpose", "personal", 2, 2.00),
	}
	axis := buildSourceAxis(rows, classification.AxisPurpose, parseInsightsDate("2026-08-01"))

	if axis.Totals.Traces != 14 || axis.Totals.CostUSD != 8.00 {
		t.Errorf("totals = %+v, want 14 traces / 8.00", axis.Totals)
	}
	work, ok := labelByKey(axis, "work")
	if !ok {
		t.Fatal("work label missing")
	}
	if work.Traces != 12 || work.CostUSD != 6.00 || work.CostPct != 75 {
		t.Errorf("work = %+v, want 12 traces / 6.00 / 75%%", work)
	}
	if work.Label != "Work" {
		t.Errorf("display label = %q, want %q", work.Label, "Work")
	}
}

// One fetch at the widest range serves every narrower one, so the window cut
// happens here rather than in a second query.
func TestBuildSourceAxisSlicesWindow(t *testing.T) {
	rows := []classification.AggRow{
		aggRow("2026-08-01", "purpose", "work", 10, 10.00),
		aggRow("2026-08-10", "purpose", "work", 3, 3.00),
		aggRow("2026-08-11", "purpose", "work", 2, 2.00),
	}
	axis := buildSourceAxis(rows, classification.AxisPurpose, parseInsightsDate("2026-08-10"))
	if axis.Totals.CostUSD != 5.00 {
		t.Errorf("windowed cost = %v, want 5.00", axis.Totals.CostUSD)
	}
	if all := buildSourceAxis(rows, classification.AxisPurpose, parseInsightsDate("2026-08-01")); all.Totals.CostUSD != 15.00 {
		t.Errorf("slicing mutated the source rows: %v, want 15.00", all.Totals.CostUSD)
	}
}

// Topic has 15 labels; the tail must fold rather than drop, or the segments
// stop summing to the axis total.
func TestBuildSourceAxisFoldsTailIntoRemainder(t *testing.T) {
	var rows []classification.AggRow
	var wantCost float64
	var wantTraces int64
	for i, label := range []string{
		"software-engineering", "data-analytics", "product", "design",
		"marketing", "sales", "customer-support", "operations-it",
		"hr-recruiting", "finance-legal", "research-learning",
	} {
		cost := float64(100 - i)
		rows = append(rows, aggRow("2026-08-11", "topic", label, 1, cost))
		wantCost += cost
		wantTraces++
	}
	axis := buildSourceAxis(rows, classification.AxisTopic, parseInsightsDate("2026-08-01"))

	if len(axis.Labels) != maxSourceLabels {
		t.Fatalf("labels = %d, want %d", len(axis.Labels), maxSourceLabels)
	}
	var gotCost float64
	var gotTraces int64
	for _, l := range axis.Labels {
		gotCost += l.CostUSD
		gotTraces += l.Traces
	}
	if gotCost != wantCost || gotTraces != wantTraces {
		t.Errorf("folded labels = %v / %d traces, want %v / %d — the tail was dropped",
			gotCost, gotTraces, wantCost, wantTraces)
	}
	rem, ok := labelByKey(axis, "other")
	if !ok {
		t.Fatal("remainder label missing")
	}
	if !rem.Aggregated {
		t.Error("remainder must be marked aggregated")
	}
}

// The chart and the table have to collapse the same set, or a segment appears
// in one and not the other.
func TestBuildSourceSeriesFoldsSameTailAsLabels(t *testing.T) {
	var rows []classification.AggRow
	for i, label := range workclassifier15() {
		rows = append(rows, aggRow("2026-08-11", "topic", label, 1, float64(100-i)))
	}
	axis := buildSourceAxis(rows, classification.AxisTopic, parseInsightsDate("2026-08-01"))
	if len(axis.Series) != 1 {
		t.Fatalf("series points = %d, want 1", len(axis.Series))
	}

	kept := map[string]bool{}
	for _, l := range axis.Labels {
		kept[l.Key] = true
	}
	var seriesCost float64
	for key, cost := range axis.Series[0].CostUSD {
		if !kept[key] {
			t.Errorf("series carries %q, which the table folded away", key)
		}
		seriesCost += cost
	}
	if seriesCost != axis.Totals.CostUSD {
		t.Errorf("series total = %v, axis total = %v — they must agree", seriesCost, axis.Totals.CostUSD)
	}
}

func workclassifier15() []string {
	return []string{
		"software-engineering", "data-analytics", "product", "design",
		"marketing", "sales", "customer-support", "operations-it",
		"hr-recruiting", "finance-legal", "research-learning",
		"creative-writing", "general-knowledge", "personal-life", "other",
	}
}

// Segments keep their position as their share moves between ranges.
func TestBuildSourceAxisOrdersByDeclaredStackingOrder(t *testing.T) {
	rows := []classification.AggRow{
		aggRow("2026-08-11", "purpose", "ambiguous", 1, 1.00),
		aggRow("2026-08-11", "purpose", "work", 1, 50.00),
		aggRow("2026-08-11", "purpose", "personal", 1, 5.00),
	}
	axis := buildSourceAxis(rows, classification.AxisPurpose, parseInsightsDate("2026-08-01"))
	var got []string
	for _, l := range axis.Labels {
		got = append(got, l.Key)
	}
	want := []string{"work", "personal", "ambiguous"}
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestBuildSourceAxesOmitsAxisWithNoData(t *testing.T) {
	rows := []classification.AggRow{aggRow("2026-08-11", "purpose", "work", 1, 1.00)}
	axes := buildSourceAxes(rows, parseInsightsDate("2026-08-01"))
	if _, ok := axes["purpose"]; !ok {
		t.Error("purpose axis should be present")
	}
	if _, ok := axes["topic"]; ok {
		t.Error("topic axis has no rows and must be omitted, not returned empty")
	}
}

// Actors are only "member" when the dev-tool email is a registered account
// member email, which it often is not. Gating the page on a per-member match
// therefore blanked it for everyone, and the empty result then read as
// "prompt collection is off" — pointing the user at an unrelated setting.
func TestBuildSourceAxesIgnoresActorIdentity(t *testing.T) {
	rows := []classification.AggRow{
		aggRow("2026-08-11", "purpose", "work", 3, 3.00),
		aggRow("2026-08-11", "purpose", "personal", 1, 1.00),
	}
	for i := range rows {
		rows[i].ActorKind = "unidentified"
		rows[i].ActorKey = "someone@example.com"
	}
	axes := buildSourceAxes(rows, parseInsightsDate("2026-08-01"))
	if got := axes["purpose"].Totals.Traces; got != 4 {
		t.Errorf("traces = %d, want 4 — unidentified actors must still aggregate", got)
	}
}

func actorRow(day, axis, label, kind, key string, traces int64, cost float64) classification.AggRow {
	r := aggRow(day, axis, label, traces, cost)
	r.ActorKind, r.ActorKey = kind, key
	return r
}

func personByKey(p InsightsSourcePeople, key string) (InsightsSourcePersonRow, bool) {
	for _, r := range p.Rows {
		if r.Key == key {
			return r, true
		}
	}
	return InsightsSourcePersonRow{}, false
}

var peopleRows = []classification.AggRow{
	// Busy developer: mostly work.
	actorRow("2026-08-11", "purpose", "work", "member", "user_busy", 90, 9.00),
	actorRow("2026-08-11", "purpose", "personal", "member", "user_busy", 10, 1.00),
	// Quiet developer: mostly personal. Fewer personal prompts than the busy
	// one in absolute terms, but a far higher share of their own.
	actorRow("2026-08-11", "purpose", "work", "member", "user_quiet", 2, 0.20),
	actorRow("2026-08-11", "purpose", "personal", "member", "user_quiet", 8, 0.80),
}

// Ranking by raw count finds whoever uses the tool most. The share of a
// person's own prompts is what separates unusual from merely busy, so both
// numbers have to be present and correct.
func TestBuildSourcePeopleReportsShareOfOwnPrompts(t *testing.T) {
	people := buildSourcePeople(peopleRows, parseInsightsDate("2026-08-01"), sourceViewer{}, "")

	busy, ok := personByKey(people, "user_busy")
	if !ok {
		t.Fatal("busy developer missing")
	}
	if busy.Traces != 100 {
		t.Errorf("busy traces = %d, want 100 (one axis, not summed across axes)", busy.Traces)
	}
	quiet, _ := personByKey(people, "user_quiet")

	pct := func(row InsightsSourcePersonRow, label string) float64 {
		for _, l := range row.Axes["purpose"] {
			if l.Key == label {
				return l.TracesPct
			}
		}
		return -1
	}
	if got := pct(busy, "personal"); got != 10 {
		t.Errorf("busy personal share = %v, want 10", got)
	}
	if got := pct(quiet, "personal"); got != 80 {
		t.Errorf("quiet personal share = %v, want 80", got)
	}
	// Absolute count still favours the busy developer — which is exactly why
	// the share column has to exist alongside it.
	if busy.Traces <= quiet.Traces {
		t.Error("expected the busy developer to lead on raw volume")
	}
}

// Every prompt carries a label on each axis, so adding the axes together counts
// it twice.
func TestBuildSourcePeopleDoesNotDoubleCountAcrossAxes(t *testing.T) {
	rows := append([]classification.AggRow{}, peopleRows...)
	rows = append(rows,
		actorRow("2026-08-11", "topic", "software-engineering", "member", "user_busy", 100, 10.00))

	people := buildSourcePeople(rows, parseInsightsDate("2026-08-01"), sourceViewer{}, "")
	busy, _ := personByKey(people, "user_busy")
	if busy.Traces != 100 {
		t.Errorf("traces = %d, want 100 — topic rows describe the same prompts", busy.Traces)
	}
	if busy.CostUSD != 10.00 {
		t.Errorf("cost = %v, want 10.00", busy.CostUSD)
	}
}

// A member sees their own row and nobody else's, while the charts above stay
// account-wide.
func TestBuildSourcePeopleRestrictsToViewer(t *testing.T) {
	people := buildSourcePeople(peopleRows, parseInsightsDate("2026-08-01"),
		sourceViewer{Restricted: true, ActorKey: "user_quiet"}, "")

	if len(people.Rows) != 1 || people.Rows[0].Key != "user_quiet" {
		t.Fatalf("rows = %+v, want only the viewer", people.Rows)
	}
	if !people.RestrictedToSelf {
		t.Error("restriction must be reported so the page can say so")
	}
	if people.ViewerUnresolved {
		t.Error("a viewer with a matching actor is resolved")
	}
}

// A restricted viewer whose dev-tool address is not linked matches no row. That
// is not "no prompts" — the page must be able to tell the reader why.
func TestBuildSourcePeopleFlagsUnresolvedViewer(t *testing.T) {
	people := buildSourcePeople(peopleRows, parseInsightsDate("2026-08-01"),
		sourceViewer{Restricted: true}, "")

	if !people.ViewerUnresolved {
		t.Error("an unlinked viewer must be flagged, not shown an empty table")
	}
	if len(people.Rows) != 0 {
		t.Errorf("rows = %+v, want none", people.Rows)
	}
}

// A day drill answers "who was behind the spike", so it must narrow the named
// breakdown to that day and nothing else.
func TestBuildSourcePeopleScopesToASingleDay(t *testing.T) {
	rows := []classification.AggRow{
		actorRow("2026-08-11", "purpose", "personal", "member", "user_a", 9, 0.90),
		actorRow("2026-08-12", "purpose", "personal", "member", "user_b", 4, 0.40),
	}
	all := buildSourcePeople(rows, parseInsightsDate("2026-08-01"), sourceViewer{}, "")
	if len(all.Rows) != 2 {
		t.Fatalf("unscoped rows = %d, want 2", len(all.Rows))
	}
	day := buildSourcePeople(rows, parseInsightsDate("2026-08-01"), sourceViewer{}, "2026-08-12")
	if len(day.Rows) != 1 || day.Rows[0].Key != "user_b" {
		t.Fatalf("day-scoped rows = %+v, want only user_b", day.Rows)
	}
	if day.Rows[0].Traces != 4 {
		t.Errorf("traces = %d, want 4 — only that day's prompts", day.Rows[0].Traces)
	}
}

// A day outside the selected range yields nothing, which is correct: the table
// cannot show prompts the charts above are not covering.
func TestBuildSourcePeopleDayOutsideWindowIsEmpty(t *testing.T) {
	rows := []classification.AggRow{
		actorRow("2026-08-01", "purpose", "work", "member", "user_a", 5, 0.50),
	}
	got := buildSourcePeople(rows, parseInsightsDate("2026-08-10"), sourceViewer{}, "2026-08-01")
	if len(got.Rows) != 0 {
		t.Errorf("rows = %+v, want none", got.Rows)
	}
}

// An address with no account member behind it still carries prompts, so it must
// appear — but must not be dressed up as a member.
func TestBuildSourcePeopleKeepsUnidentifiedActors(t *testing.T) {
	rows := []classification.AggRow{
		actorRow("2026-08-11", "purpose", "work", "unidentified", "dev@example.com", 5, 1.00),
	}
	people := buildSourcePeople(rows, parseInsightsDate("2026-08-01"), sourceViewer{}, "")
	if len(people.Rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(people.Rows))
	}
	id := people.Rows[0].Identity
	if id.Kind != "unidentified" || id.Label != "dev@example.com" {
		t.Errorf("identity = %+v, want an unidentified row labelled by address", id)
	}
}

// The client indexes a palette by this number and clamps to its last entry, so
// an unknown label has to land past every declared slot. Returning a wrapped or
// hashed index would paint it as a real category.
func TestLabelColorIndexPutsUnknownLabelsPastTheDeclaredSet(t *testing.T) {
	for _, axis := range workclassifier.Axes {
		declared := workclassifier.Labels[axis]
		for i, label := range declared {
			if got := labelColorIndex(classification.Axis(axis), label); got != i {
				t.Errorf("%s/%s = %d, want its declared slot %d", axis, label, got, i)
			}
		}
		if got := labelColorIndex(classification.Axis(axis), "a-label-from-a-later-retrain"); got != len(declared) {
			t.Errorf("%s unknown = %d, want the overflow slot %d", axis, got, len(declared))
		}
	}
}

// An unknown label must still render — the alternative is a segment vanishing
// from the chart after a retrain adds a class.
func TestDisplayLabelFallsBackForUnknownKey(t *testing.T) {
	if got := displayLabel("brand-new-class"); got != "Brand new class" {
		t.Errorf("displayLabel = %q, want %q", got, "Brand new class")
	}
}
