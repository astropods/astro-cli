package handlers

import (
	"math"
	"sort"
	"strings"
)

// Folds dev-tool usage (Claude Code, Codex, …) into the Insights view model:
// sources become chart series, stat-card contributions, and synthetic agent /
// person rows, merged BEFORE sort/paginate/percentage so the one pipeline stays
// authoritative. `hidden` holds the source keys (and "agents") the caller
// excluded via hide_sources; `agentsHidden` drops the base agent contribution.

func round4(v float64) float64 { return math.Round(v*10000) / 10000 }

func sortedDevtoolKeys(sources map[string]DevtoolSource, hidden map[string]bool) []string {
	keys := make([]string, 0, len(sources))
	for k := range sources {
		if !hidden[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

func devtoolChips(srcKeys []string, sources map[string]DevtoolSource) []InsightsAgentChip {
	chips := make([]InsightsAgentChip, 0, len(srcKeys))
	for _, k := range srcKeys {
		chips = append(chips, InsightsAgentChip{
			Key:   "devtool:" + k,
			Label: sources[k].Label,
			Icon:  sources[k].AgentRow.Identity.Icon,
		})
	}
	return chips
}

// foldDevtoolChart adds each enabled source as a per-day series; when agents are
// hidden the base agent series are dropped, leaving only the sources.
func foldDevtoolChart(chart []AccountCostOverTimeEntry, sources map[string]DevtoolSource, hidden map[string]bool, agentsHidden bool) []AccountCostOverTimeEntry {
	keys := sortedDevtoolKeys(sources, hidden)
	if len(keys) == 0 && !agentsHidden {
		return chart
	}
	costByKeyDate := make(map[string]map[string]float64, len(keys))
	for _, k := range keys {
		byDate := make(map[string]float64, len(sources[k].SpendByDay))
		for _, p := range sources[k].SpendByDay {
			byDate[p.Date] = p.CostUSD
		}
		costByKeyDate[k] = byDate
	}
	out := make([]AccountCostOverTimeEntry, len(chart))
	for i, entry := range chart {
		models := append([]AccountModelCost(nil), entry.Models...)
		if agentsHidden {
			models = nil
		}
		for _, k := range keys {
			models = append(models, AccountModelCost{Model: k, CostUSD: round4(costByKeyDate[k][entry.Date])})
		}
		out[i] = AccountCostOverTimeEntry{Date: entry.Date, Models: models}
	}
	return out
}

func foldDevtoolStatCards(cards InsightsStatCards, sources map[string]DevtoolSource, hidden map[string]bool, agentsHidden bool) InsightsStatCards {
	keys := sortedDevtoolKeys(sources, hidden)
	if len(keys) == 0 && !agentsHidden {
		return cards
	}
	totals := cards.Totals
	if agentsHidden {
		totals.CostUSD, totals.Requests, totals.TotalTokens, totals.ActiveAgents = 0, 0, 0, 0
	}
	for _, k := range keys {
		t := sources[k].Totals
		totals.CostUSD += t.CostUSD
		totals.Requests += t.Requests
		totals.TotalTokens += t.TotalTokens
	}
	totals.CostUSD = round2(totals.CostUSD)
	cards.Totals = totals
	// The period-over-period Change is computed from agent spend only; it no
	// longer describes the folded total, so drop it rather than pair a real total
	// with a misleading delta.
	cards.Change = nil
	return cards
}

func foldDevtoolSeriesLabels(labels map[string]string, sources map[string]DevtoolSource, hidden map[string]bool, agentsHidden bool) map[string]string {
	out := make(map[string]string, len(labels)+len(sources))
	if !agentsHidden {
		for k, v := range labels {
			out[k] = v
		}
	}
	for _, k := range sortedDevtoolKeys(sources, hidden) {
		out[k] = sources[k].Label
	}
	return out
}

// foldDevtoolRange folds enabled sources into one range's chart, stat cards, and
// series labels.
func foldDevtoolRange(r InsightsRange, sources map[string]DevtoolSource, hidden map[string]bool, agentsHidden bool) InsightsRange {
	r.AgentSpendChart = foldDevtoolChart(r.AgentSpendChart, sources, hidden, agentsHidden)
	r.StatCards = foldDevtoolStatCards(r.StatCards, sources, hidden, agentsHidden)
	r.SeriesLabels = foldDevtoolSeriesLabels(r.SeriesLabels, sources, hidden, agentsHidden)
	return r
}

// foldDevtoolAgentRows appends a synthetic row per enabled source and rescales
// cost_pct over the combined total; base agent rows are dropped when hidden.
func foldDevtoolAgentRows(base []InsightsAgentRow, baseTotal float64, sources map[string]DevtoolSource, hidden map[string]bool, agentsHidden bool) ([]InsightsAgentRow, float64) {
	keys := sortedDevtoolKeys(sources, hidden)
	if len(keys) == 0 && !agentsHidden {
		return base, baseTotal
	}
	rows := append([]InsightsAgentRow(nil), base...)
	total := baseTotal
	if agentsHidden {
		rows, total = nil, 0
	}
	for _, k := range keys {
		rows = append(rows, sources[k].AgentRow)
		total += sources[k].AgentRow.Metrics.CostUSD
	}
	total = round4(total)
	for i := range rows {
		rows[i].Metrics.CostPct = insightPct(rows[i].Metrics.CostUSD, total)
	}
	return rows, total
}

// foldDevtoolPeopleRows rolls each developer's dev-tool spend into their person
// row (matched by the server-resolved identity key). A resolved member absent
// from the base people set — including every member when agents are hidden — is
// synthesized from the member profile; a developer with no Astro identity gets an
// email row. cost_pct uses the same combined denominator as the agents table.
func foldDevtoolPeopleRows(base []InsightsPersonRow, baseTotal float64, sources map[string]DevtoolSource, hidden map[string]bool, agentsHidden bool, members map[string]insightsMemberProfile, restrictToKey string) ([]InsightsPersonRow, float64) {
	keys := sortedDevtoolKeys(sources, hidden)
	if len(keys) == 0 && !agentsHidden {
		return base, baseTotal
	}
	rows := append([]InsightsPersonRow(nil), base...)
	if agentsHidden {
		rows = nil
	}
	idxByKey := make(map[string]int, len(rows))
	for i, r := range rows {
		idxByKey[r.Key] = i
	}

	// Aggregate each developer across enabled sources, keyed by identity_key when
	// the email resolved to a member, else the lowercased email; track the distinct
	// sources they used for the "agent used" chips.
	type devAgg struct {
		email       string
		identityKey string
		cost        float64
		tokens      int
		srcKeys     []string
	}
	devs := map[string]*devAgg{}
	order := []string{}
	for _, k := range keys {
		for _, u := range sources[k].ByUser {
			dk := u.IdentityKey
			if dk == "" {
				dk = "email:" + strings.ToLower(u.UserEmail)
			}
			d := devs[dk]
			if d == nil {
				d = &devAgg{email: u.UserEmail, identityKey: u.IdentityKey}
				devs[dk] = d
				order = append(order, dk)
			}
			d.cost += u.CostUSD
			d.tokens += u.TotalTokens
			if n := len(d.srcKeys); n == 0 || d.srcKeys[n-1] != k {
				d.srcKeys = append(d.srcKeys, k)
			}
		}
	}

	for _, dk := range order {
		d := devs[dk]
		// Gate: a restricted (non-admin) viewer sees only their own dev-tool spend;
		// skip every other developer's per-person contribution.
		if restrictToKey != "" && d.identityKey != restrictToKey {
			continue
		}
		chips := devtoolChips(d.srcKeys, sources)
		if i, ok := idxByKey[d.identityKey]; d.identityKey != "" && ok {
			rows[i].Metrics.CostUSD = round4(rows[i].Metrics.CostUSD + d.cost)
			rows[i].Metrics.Tokens += d.tokens
			rows[i].AgentsUsed = append(rows[i].AgentsUsed, chips...)
			continue
		}
		if d.identityKey != "" {
			// Resolved member not in the base set (no agent/trace activity, or agents
			// hidden) — synthesize their row from the member profile so their
			// dev-tool spend is still shown.
			identity := insightUserIdentity(UserIdentity{UserID: strings.TrimPrefix(d.identityKey, "member:")}, members)
			rows = append(rows, InsightsPersonRow{
				Key:        insightIdentityRowKey(identity),
				SearchText: strings.ToLower(strings.Join(insightIdentitySearchParts(identity), " ")),
				Identity:   identity,
				AgentsUsed: chips,
				Metrics:    InsightsPersonMetrics{CostUSD: round4(d.cost), Tokens: d.tokens},
			})
			continue
		}
		rows = append(rows, InsightsPersonRow{
			Key:        "devtool:" + strings.ToLower(d.email),
			SearchText: strings.ToLower(d.email),
			Identity:   InsightsIdentityRef{Kind: "unidentified", Label: d.email},
			AgentsUsed: chips,
			Metrics:    InsightsPersonMetrics{CostUSD: round4(d.cost), Tokens: d.tokens},
		})
	}

	// Denominator matches the agents table: base people total + full per-source
	// totals (not just email-attributed spend), so both tables' cost_pct share a
	// basis. Zero the base when agents are hidden.
	baseP := baseTotal
	if agentsHidden {
		baseP = 0
	}
	for _, k := range keys {
		baseP += sources[k].AgentRow.Metrics.CostUSD
	}
	total := round4(baseP)
	for i := range rows {
		rows[i].Metrics.CostPct = insightPct(rows[i].Metrics.CostUSD, total)
	}
	return rows, total
}

// devtoolSourceRefs lists every present source for the Sources filter, in a
// stable order, regardless of the hide_sources selection.
func devtoolSourceRefs(sources map[string]DevtoolSource) []DevtoolSourceRef {
	keys := sortedDevtoolKeys(sources, nil)
	refs := make([]DevtoolSourceRef, 0, len(keys))
	for _, k := range keys {
		refs = append(refs, DevtoolSourceRef{Key: k, Label: sources[k].Label, Icon: sources[k].AgentRow.Identity.Icon})
	}
	return refs
}

// devtoolFold selects which surfaces the dev-tool fold applies to.
//
// v1 folds everywhere, because dev-tool spend arrives from a separate pipeline
// (VictoriaMetrics) and is absent from the Langfuse-derived base data.
//
// v2 stores dev-tool spend in the fact table, so the ranges and the People rows
// already include it — folding those again would double-count. Only the
// synthetic agents row is missing there, because dev-tool facts carry no
// deployment id and so never become an agent row. Splitting the inputs makes
// that difference explicit instead of leaving it to a shared variable.
type devtoolFold struct {
	// Ranges folds stat cards and chart series, keyed by range.
	Ranges map[string]DevtoolRange
	// AgentRows supplies the synthetic per-source row in the agents table.
	AgentRows map[string]DevtoolSource
	// PeopleRows rolls per-developer spend into person rows.
	PeopleRows map[string]DevtoolSource
	// Present drives the Sources filter and lists every source with usage,
	// regardless of what is currently folded or hidden.
	Present map[string]DevtoolSource
}

// devtoolFoldAll folds every surface from one set of per-range sources — v1's
// behavior, where nothing in the base data includes dev-tool spend.
func devtoolFoldAll(ranges map[string]DevtoolRange) devtoolFold {
	sources := ranges[widestInsightsRange().key].Sources
	return devtoolFold{
		Ranges:     ranges,
		AgentRows:  sources,
		PeopleRows: sources,
		Present:    sources,
	}
}
