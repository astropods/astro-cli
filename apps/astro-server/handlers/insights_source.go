package handlers

import (
	"context"
	"net/http"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/classification"
	"github.com/astropods/astro/apps/astro-server/internal/experiment"
	"github.com/astropods/astro/apps/astro-server/internal/insightsrollup"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/workclassifier"
)

// Topic has 15 labels, more than a palette reads; the tail folds into a
// remainder rather than being dropped.
const maxSourceLabels = 8

var axisLabels = map[string]string{
	"purpose": "Work vs personal",
	"topic":   "Topic",
}

// labelRank is each axis's declared label order, for colour slots and stacking.
var labelRank = func() map[classification.Axis]map[string]int {
	out := map[classification.Axis]map[string]int{}
	for axis, labels := range workclassifier.Labels {
		rank := make(map[string]int, len(labels))
		for i, l := range labels {
			rank[l] = i
		}
		out[classification.Axis(axis)] = rank
	}
	return out
}()

// A map, not a derivation: "operations-it" is "Operations & IT".
var labelDisplay = map[string]string{
	"work":      "Work",
	"personal":  "Personal",
	"ambiguous": "Unclear",
	"other":     "Other",

	"software-engineering": "Software engineering",
	"data-analytics":       "Data & analytics",
	"product":              "Product",
	"design":               "Design",
	"marketing":            "Marketing",
	"sales":                "Sales",
	"customer-support":     "Customer support",
	"operations-it":        "Operations & IT",
	"hr-recruiting":        "HR & recruiting",
	"finance-legal":        "Finance & legal",
	"research-learning":    "Research & learning",
	"creative-writing":     "Creative writing",
	"general-knowledge":    "General knowledge",
	"personal-life":        "Personal life",
}

func displayLabel(key string) string {
	if l, ok := labelDisplay[key]; ok {
		return l
	}
	if key == "" {
		return ""
	}
	return strings.ToUpper(key[:1]) + strings.ReplaceAll(key[1:], "-", " ")
}

// GetAccountInsightsSource returns the detail view for one dev-tool source:
// what the account used the tool for, by classified prompt.
// GET /api/v1/accounts/:account/insights/sources/:source
func GetAccountInsightsSource(
	log *logger.Logger,
	accountStore *account.AccountStore,
	classifications *classification.Store,
	orgRoles orgRoleLookup,
	classificationGate *experiment.Gate,
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
		ad, ok := devtoolAdapterByKey(c.Param("source"))
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "unknown source"})
			return
		}
		// Not found rather than forbidden: to an account without the experiment
		// the page does not exist, and saying "forbidden" would advertise it.
		on, gerr := classificationGate.Enabled(c.Request.Context(), acct.ID)
		if gerr != nil {
			// A read failure is not an absent page; the client turns 404 into a
			// route-level not-found the reader cannot retry out of.
			log.Error("insights source: experiment check failed", "error", gerr, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read experiment"})
			return
		}
		if !on {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// The whole page is scoped, not just the named breakdown: charts built
		// from everyone's prompts would report the account's work/personal split
		// to a reader who may not see who is behind it.
		viewer := sourceViewer{}
		if !insightsSeesEveryone(c, accountStore, orgRoles, acct, user) {
			viewer.Restricted = true
			viewer.ActorKey = user.ID
		}
		if members, merr := insightsMemberProfiles(log, accountStore, acct.ID); merr == nil {
			viewer.Members = members
		} else {
			log.Warn("source insights: member profiles unavailable", "account_id", acct.ID, "error", merr)
		}

		// Dropped rather than guessed at: a bad value would widen the drill.
		day := c.Query("day")
		if day != "" {
			if _, perr := time.Parse(time.DateOnly, day); perr != nil {
				day = ""
			}
		}

		resp, err := computeInsightsSource(c.Request.Context(), classifications, acct.ID, ad, viewer, day, time.Now().UTC())
		if err != nil {
			log.Error("insights source: compute source insights failed", "error", err, "account_id", acct.ID, "source", ad.Key)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to compute source insights"})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// One fetch at the widest range feeds every range.
func computeInsightsSource(
	ctx context.Context,
	classifications *classification.Store,
	accountID string,
	ad devtoolAdapter,
	viewer sourceViewer,
	day string,
	now time.Time,
) (InsightsSourceResponse, error) {
	resp := InsightsSourceResponse{
		// Conversion, so a field added to the adapter is not silently dropped.
		Source: DevtoolSourceRef(ad),
		Ranges: map[string]InsightsSourceRange{},
		Axes:   sourceAxisRefs(),
	}
	if classifications == nil {
		return resp, nil
	}

	state, err := classifications.GetState(ctx, accountID, ad.Key)
	if err != nil {
		return resp, err
	}
	resp.Coverage.BackfillComplete = state.BackfillComplete
	resp.Coverage.ClassifiedFrom = formatDayPtr(state.BackfilledFrom)
	resp.Coverage.ClassifiedThrough = formatDayPtr(state.ClassifiedThrough)

	// Last complete day: a window ending today carries a bar that cannot fill.
	asOf := now.UTC().Truncate(24*time.Hour).AddDate(0, 0, -1)
	widest := widestInsightsRange()
	rows, err := classifications.Aggregates(
		ctx, accountID, ad.Key, asOf.AddDate(0, 0, -(widest.days-1)), asOf, viewer.ActorKey)
	if err != nil {
		return resp, err
	}
	// Account-scoped on purpose: the reader's own rows are restricted below, and
	// reporting their emptiness as "collection is off" sends them to a setting in
	// a console Astro does not control.
	from := asOf.AddDate(0, 0, -(widest.days - 1))
	accountHasContent, err := classifications.HasAggregates(ctx, accountID, ad.Key, from, asOf)
	if err != nil {
		return resp, err
	}
	resp.Coverage.ContentAvailable = accountHasContent

	// Nothing of the reader's own where the account has plenty: their dev-tool
	// address is not attributed to them, which is not the same as no prompts.
	viewer.Unresolved = viewer.Restricted && accountHasContent && len(rows) == 0

	// Presence, not a total — rows repeat per axis.
	var anyTraces bool
	var anyCost bool
	for _, r := range rows {
		anyTraces = anyTraces || r.Traces > 0
		anyCost = anyCost || r.CostUSD > 0
	}
	resp.Coverage.CostUnavailable = anyTraces && !anyCost

	for _, spec := range insightsRangeSpecs {
		period, fromDate, _ := insightsPeriod(asOf, spec.days)
		from := parseInsightsDate(fromDate)
		resp.Ranges[spec.key] = InsightsSourceRange{
			Days:   spec.days,
			Period: period,
			Axes:   buildSourceAxes(rows, from),
			People: buildSourcePeople(rows, from, viewer, day),
		}
	}
	return resp, nil
}

// Ranked by prompts, so the cap drops the quietest, and TotalCount reports it.
const maxSourcePeople = 100

type sourceViewer struct {
	Restricted bool
	ActorKey   string
	// Unresolved reports a restricted reader that nothing attributes to, on an
	// account that does have classified prompts.
	Unresolved bool
	Members    map[string]insightsMemberProfile
}

// Folds the rows the charts already fetched into one row per developer. They
// arrive scoped to the reader, so nothing is filtered here.
func buildSourcePeople(
	rows []classification.AggRow,
	from time.Time,
	viewer sourceViewer,
	day string,
) InsightsSourcePeople {
	out := InsightsSourcePeople{
		Rows:             []InsightsSourcePersonRow{},
		RestrictedToSelf: viewer.Restricted,
		ViewerUnresolved: viewer.Unresolved,
	}
	if out.ViewerUnresolved {
		return out
	}

	type person struct {
		kind    string
		key     string
		traces  int64
		cost    float64
		byAxis  map[classification.Axis]map[string]*sourceCell
		perAxis map[classification.Axis]int64
	}
	people := map[string]*person{}

	for _, r := range rows {
		if r.Day.UTC().Before(from) {
			continue
		}
		// The day narrows this breakdown only; the charts keep the range.
		if day != "" && r.Day.UTC().Format(time.DateOnly) != day {
			continue
		}
		p := people[r.ActorKey]
		if p == nil {
			p = &person{
				kind:    r.ActorKind,
				key:     r.ActorKey,
				byAxis:  map[classification.Axis]map[string]*sourceCell{},
				perAxis: map[classification.Axis]int64{},
			}
			people[r.ActorKey] = p
		}
		if p.byAxis[r.Axis] == nil {
			p.byAxis[r.Axis] = map[string]*sourceCell{}
		}
		if p.byAxis[r.Axis][r.Label] == nil {
			p.byAxis[r.Axis][r.Label] = &sourceCell{}
		}
		p.byAxis[r.Axis][r.Label].traces += r.Traces
		p.byAxis[r.Axis][r.Label].cost += r.CostUSD
		p.perAxis[r.Axis] += r.Traces
	}

	// One axis, not a sum: every prompt is labelled on each, so summing doubles.
	primary := classification.Axis(workclassifier.Axes[0])
	for _, p := range people {
		p.traces = p.perAxis[primary]
		for _, c := range p.byAxis[primary] {
			p.cost += c.cost
		}
	}

	ranked := make([]*person, 0, len(people))
	for _, p := range people {
		ranked = append(ranked, p)
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].traces != ranked[j].traces {
			return ranked[i].traces > ranked[j].traces
		}
		return ranked[i].key < ranked[j].key
	})
	out.TotalCount = len(ranked)
	if len(ranked) > maxSourcePeople {
		ranked = ranked[:maxSourcePeople]
	}

	for _, p := range ranked {
		row := InsightsSourcePersonRow{
			Key:      p.key,
			Identity: sourcePersonIdentity(p.kind, p.key, viewer.Members),
			Traces:   p.traces,
			CostUSD:  p.cost,
			Axes:     map[string][]InsightsSourceLabel{},
		}
		for _, a := range workclassifier.Axes {
			axis := classification.Axis(a)
			cells := p.byAxis[axis]
			if len(cells) == 0 {
				continue
			}
			total := p.perAxis[axis]
			labels := make([]InsightsSourceLabel, 0, len(cells))
			for key, c := range cells {
				labels = append(labels, InsightsSourceLabel{
					Key:        key,
					Label:      displayLabel(key),
					ColorIndex: labelColorIndex(axis, key),
					Traces:     c.traces,
					TracesPct:  insightPct(float64(c.traces), float64(total)),
					CostUSD:    c.cost,
				})
			}
			sort.SliceStable(labels, func(i, j int) bool {
				return labels[i].ColorIndex < labels[j].ColorIndex
			})
			row.Axes[string(a)] = labels
		}
		out.Rows = append(out.Rows, row)
	}
	return out
}

// Matches the Insights People table, and leaves an unresolved address bare
// rather than dressing it as a member.
func sourcePersonIdentity(kind, key string, members map[string]insightsMemberProfile) InsightsIdentityRef {
	if kind == insightsrollup.ActorKindMember {
		return insightUserIdentity(UserIdentity{
			UserID:      key,
			UserDetails: UserDetails{Kind: UserDetailsKindAstro},
		}, members)
	}
	if key == "" {
		return InsightsIdentityRef{Kind: "system", Label: "Unattributed"}
	}
	return InsightsIdentityRef{
		Kind:    "unidentified",
		Label:   key,
		Tooltip: "This address is not linked to an account member, so these prompts cannot be attributed to a person.",
	}
}

func sourceAxisRefs() []InsightsSourceAxisRef {
	refs := make([]InsightsSourceAxisRef, 0, len(workclassifier.Axes))
	for _, a := range workclassifier.Axes {
		key := string(a)
		label := axisLabels[key]
		if label == "" {
			label = displayLabel(key)
		}
		refs = append(refs, InsightsSourceAxisRef{Key: key, Label: label})
	}
	return refs
}

// Skipping rows older than from is how one fetch serves every range.
func buildSourceAxes(rows []classification.AggRow, from time.Time) map[string]InsightsSourceAxis {
	out := map[string]InsightsSourceAxis{}
	for _, a := range workclassifier.Axes {
		axis := buildSourceAxis(rows, classification.Axis(a), from)
		if axis.Totals.Traces == 0 {
			continue
		}
		out[string(a)] = axis
	}
	return out
}

type sourceCell struct {
	traces int64
	cost   float64
}

func buildSourceAxis(rows []classification.AggRow, axis classification.Axis, from time.Time) InsightsSourceAxis {
	byLabel := map[string]*sourceCell{}
	byDay := map[string]map[string]*sourceCell{}
	var totals InsightsSourceTotals

	for _, r := range rows {
		if r.Axis != axis || r.Day.UTC().Before(from) {
			continue
		}
		if byLabel[r.Label] == nil {
			byLabel[r.Label] = &sourceCell{}
		}
		byLabel[r.Label].traces += r.Traces
		byLabel[r.Label].cost += r.CostUSD

		day := r.Day.UTC().Format(time.DateOnly)
		if byDay[day] == nil {
			byDay[day] = map[string]*sourceCell{}
		}
		if byDay[day][r.Label] == nil {
			byDay[day][r.Label] = &sourceCell{}
		}
		byDay[day][r.Label].traces += r.Traces
		byDay[day][r.Label].cost += r.CostUSD

		totals.Traces += r.Traces
		totals.CostUSD += r.CostUSD
	}
	if totals.Traces == 0 {
		return InsightsSourceAxis{}
	}

	kept, remainder := topSourceLabels(byLabel, axis)
	labels := make([]InsightsSourceLabel, 0, len(kept))
	for _, key := range kept {
		c := byLabel[key]
		labels = append(labels, InsightsSourceLabel{
			Key:        key,
			Label:      displayLabel(key),
			ColorIndex: labelColorIndex(axis, key),
			Traces:     c.traces,
			TracesPct:  insightPct(float64(c.traces), float64(totals.Traces)),
			CostUSD:    c.cost,
			CostPct:    insightPct(c.cost, totals.CostUSD),
			Aggregated: key == remainder && remainder != "",
		})
	}

	return InsightsSourceAxis{
		Labels: labels,
		Series: buildSourceSeries(byDay, kept, remainder),
		Totals: totals,
	}
}

// Rewrites byLabel in place so the series and the table collapse the same set.
// Returns the kept keys in stacking order and which absorbed the tail.
func topSourceLabels(byLabel map[string]*sourceCell, axis classification.Axis) (kept []string, remainder string) {
	keys := make([]string, 0, len(byLabel))
	for k := range byLabel {
		keys = append(keys, k)
	}
	// Key ascending on ties, so equal-cost labels order stably.
	sort.Slice(keys, func(i, j int) bool {
		if byLabel[keys[i]].cost != byLabel[keys[j]].cost {
			return byLabel[keys[i]].cost > byLabel[keys[j]].cost
		}
		if byLabel[keys[i]].traces != byLabel[keys[j]].traces {
			return byLabel[keys[i]].traces > byLabel[keys[j]].traces
		}
		return keys[i] < keys[j]
	})
	if len(keys) <= maxSourceLabels {
		return orderByAxis(keys, axis), ""
	}

	remainder = workclassifier.Fallback[workclassifier.Axis(axis)]
	if remainder == "" {
		remainder = "other"
	}
	kept = keys[:maxSourceLabels]
	// The remainder must be kept, or the tail it absorbs is never rendered.
	if !slices.Contains(kept, remainder) {
		kept = append(kept[:maxSourceLabels-1:maxSourceLabels-1], remainder)
		if byLabel[remainder] == nil {
			byLabel[remainder] = &sourceCell{}
		}
	}
	for _, k := range keys {
		if slices.Contains(kept, k) {
			continue
		}
		byLabel[remainder].traces += byLabel[k].traces
		byLabel[remainder].cost += byLabel[k].cost
		delete(byLabel, k)
	}
	return orderByAxis(kept, axis), remainder
}

// A label's slot in its axis's declared space, so the fold reordering a
// response cannot recolour the chart. Unknown labels land after the declared
// set rather than colliding with slot zero.
func labelColorIndex(axis classification.Axis, key string) int {
	declared := workclassifier.Labels[workclassifier.Axis(axis)]
	if i, ok := labelRank[axis][key]; ok {
		return i
	}
	// One shared overflow slot past the declared set. Spreading unknown labels
	// over several would need the client to size its palette to match; sharing
	// one keeps the contract to "declared slot, or overflow".
	return len(declared)
}

// Declared stacking order, so a segment holds its position across ranges.
func orderByAxis(keys []string, axis classification.Axis) []string {
	rank := labelRank[axis]
	out := append([]string(nil), keys...)
	sort.SliceStable(out, func(i, j int) bool {
		ri, oki := rank[out[i]]
		rj, okj := rank[out[j]]
		if oki != okj {
			// Unknown labels sort last rather than colliding at rank zero.
			return oki
		}
		if !oki {
			return out[i] < out[j]
		}
		return ri < rj
	})
	return out
}

// buildSourceSeries emits an ascending point per day that has classified
// prompts. Days with none are absent rather than zero-filled, so the chart can
// tell "nobody used it" from "used it, spent nothing".
func buildSourceSeries(
	byDay map[string]map[string]*sourceCell,
	kept []string,
	remainder string,
) []InsightsSourceSeriesPoint {
	days := make([]string, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Strings(days)

	out := make([]InsightsSourceSeriesPoint, 0, len(days))
	for _, day := range days {
		point := InsightsSourceSeriesPoint{
			Date:    day,
			Traces:  map[string]int64{},
			CostUSD: map[string]float64{},
		}
		for label, c := range byDay[day] {
			key := label
			if !slices.Contains(kept, label) {
				// Same fold the table applied, so segments sum to the row.
				if remainder == "" {
					continue
				}
				key = remainder
			}
			point.Traces[key] += c.traces
			point.CostUSD[key] += c.cost
		}
		out = append(out, point)
	}
	return out
}

func formatDayPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.DateOnly)
}
