package handlers

import "testing"

func mkDevtoolSource(key, label, icon string, total float64, byUser []DevtoolUserSpend) DevtoolSource {
	totals := DevtoolTotals{CostUSD: total}
	return DevtoolSource{
		Label:    label,
		Totals:   totals,
		ByUser:   byUser,
		AgentRow: devtoolAgentRow(devtoolAdapter{Key: key, Label: label, Icon: icon}, totals),
	}
}

func findPersonRow(rows []InsightsPersonRow, key string) *InsightsPersonRow {
	for i := range rows {
		if rows[i].Key == key {
			return &rows[i]
		}
	}
	return nil
}

func TestFoldDevtoolAgentRows(t *testing.T) {
	base := []InsightsAgentRow{{Key: "dep1", Metrics: InsightsAgentMetrics{CostUSD: 10}}}
	sources := map[string]DevtoolSource{"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 5, nil)}

	rows, total := foldDevtoolAgentRows(base, 10, sources, nil, false)
	if total != 15 {
		t.Fatalf("combined total = %v, want 15", total)
	}
	src := findAgentRow(rows, "claude-code")
	if src == nil {
		t.Fatal("missing synthetic claude-code agent row")
	}
	if src.Identity.Icon != "anthropic" {
		t.Fatalf("source row icon = %q, want anthropic (icon must be wired from the adapter)", src.Identity.Icon)
	}
	if src.Metrics.CostPct == 0 {
		t.Fatal("source row cost_pct not rescaled over combined total")
	}

	// Early return: no enabled sources and agents shown → base untouched.
	got, gotTotal := foldDevtoolAgentRows(base, 10, map[string]DevtoolSource{}, nil, false)
	if gotTotal != 10 || len(got) != 1 {
		t.Fatalf("no-source fold should return base unchanged, got %d rows total %v", len(got), gotTotal)
	}
}

func findAgentRow(rows []InsightsAgentRow, key string) *InsightsAgentRow {
	for i := range rows {
		if rows[i].Key == key {
			return &rows[i]
		}
	}
	return nil
}

func TestFoldDevtoolPeopleRows(t *testing.T) {
	members := map[string]insightsMemberProfile{
		"u1": {userID: "u1", username: "alice", displayName: "Alice"},
		"u2": {userID: "u2", username: "bob", displayName: "Bob"},
	}
	base := []InsightsPersonRow{
		{Key: "member:u1", Identity: InsightsIdentityRef{Kind: "member", ID: "u1", Label: "Alice"}, Metrics: InsightsPersonMetrics{CostUSD: 10}},
	}
	sources := map[string]DevtoolSource{
		"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 6, []DevtoolUserSpend{
			{UserEmail: "alice@x.com", CostUSD: 4, TotalTokens: 100, IdentityKey: "member:u1"}, // on base → merge
			{UserEmail: "bob@x.com", CostUSD: 1, TotalTokens: 50, IdentityKey: "member:u2"},    // resolved, off base → synthesize
			{UserEmail: "ext@x.com", CostUSD: 1, TotalTokens: 10},                              // unresolved → email row
		}),
	}

	rows, total := foldDevtoolPeopleRows(base, 10, sources, nil, false, members, "")
	// Denominator = base people total (10) + full source total (6).
	if total != 16 {
		t.Fatalf("combined total = %v, want 16 (base + full source total)", total)
	}
	alice := findPersonRow(rows, "member:u1")
	if alice == nil || alice.Metrics.CostUSD != 14 {
		t.Fatalf("alice row = %+v, want merged cost 14", alice)
	}
	if len(alice.AgentsUsed) != 1 || alice.AgentsUsed[0].Icon != "anthropic" {
		t.Fatalf("alice agents_used = %+v, want one chip with anthropic icon", alice.AgentsUsed)
	}
	bob := findPersonRow(rows, "member:u2")
	if bob == nil || bob.Metrics.CostUSD != 1 || bob.Identity.Label != "Bob" {
		t.Fatalf("bob row = %+v, want synthesized member row cost 1 label Bob", bob)
	}
	if ext := findPersonRow(rows, "devtool:ext@x.com"); ext == nil || ext.Metrics.CostUSD != 1 {
		t.Fatalf("missing unresolved email row for ext@x.com: %+v", ext)
	}
}

func TestFoldDevtoolPeopleRowsRestrictedToViewer(t *testing.T) {
	members := map[string]insightsMemberProfile{
		"u1": {userID: "u1", username: "alice", displayName: "Alice"},
		"u2": {userID: "u2", username: "bob", displayName: "Bob"},
	}
	base := []InsightsPersonRow{
		{Key: "member:u1", Identity: InsightsIdentityRef{Kind: "member", ID: "u1", Label: "Alice"}, Metrics: InsightsPersonMetrics{CostUSD: 10}},
	}
	sources := map[string]DevtoolSource{
		"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 6, []DevtoolUserSpend{
			{UserEmail: "alice@x.com", CostUSD: 4, IdentityKey: "member:u1"},
			{UserEmail: "bob@x.com", CostUSD: 1, IdentityKey: "member:u2"},
			{UserEmail: "ext@x.com", CostUSD: 1},
		}),
	}

	// Non-admin viewer (u1): only their own dev-tool spend is folded in.
	rows, total := foldDevtoolPeopleRows(base, 10, sources, nil, false, members, "member:u1")
	// The denominator is the full account total regardless of the gate.
	if total != 16 {
		t.Fatalf("combined total = %v, want 16", total)
	}
	alice := findPersonRow(rows, "member:u1")
	if alice == nil || alice.Metrics.CostUSD != 14 || len(alice.AgentsUsed) != 1 {
		t.Fatalf("viewer's own row should include their dev-tool spend + chip, got %+v", alice)
	}
	if bob := findPersonRow(rows, "member:u2"); bob != nil {
		t.Fatalf("another member's dev-tool spend must be gated, got %+v", bob)
	}
	if ext := findPersonRow(rows, "devtool:ext@x.com"); ext != nil {
		t.Fatalf("another developer's email row must be gated, got %+v", ext)
	}
}

func TestFoldDevtoolPeopleRowsAgentsHidden(t *testing.T) {
	members := map[string]insightsMemberProfile{"u1": {userID: "u1", username: "alice", displayName: "Alice"}}
	base := []InsightsPersonRow{
		{Key: "member:u1", Identity: InsightsIdentityRef{Kind: "member", ID: "u1", Label: "Alice"}, Metrics: InsightsPersonMetrics{CostUSD: 10}},
	}
	sources := map[string]DevtoolSource{
		"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 4, []DevtoolUserSpend{
			{UserEmail: "alice@x.com", CostUSD: 4, IdentityKey: "member:u1"},
		}),
	}

	rows, total := foldDevtoolPeopleRows(base, 10, sources, nil, true, members, "")
	// agents hidden: base dropped, so alice must still appear (synthesized) with
	// ONLY her dev-tool spend (no base $10 leakage), and total excludes base.
	if total != 4 {
		t.Fatalf("agents-hidden total = %v, want 4", total)
	}
	alice := findPersonRow(rows, "member:u1")
	if alice == nil {
		t.Fatal("resolved member vanished when agents hidden")
	}
	if alice.Metrics.CostUSD != 4 {
		t.Fatalf("alice cost = %v, want 4 (dev-tool only, no base leakage)", alice.Metrics.CostUSD)
	}
}

func TestFoldDevtoolPeopleRowsDedupChips(t *testing.T) {
	members := map[string]insightsMemberProfile{"u1": {userID: "u1", username: "alice", displayName: "Alice"}}
	base := []InsightsPersonRow{
		{Key: "member:u1", Identity: InsightsIdentityRef{Kind: "member", ID: "u1", Label: "Alice"}, Metrics: InsightsPersonMetrics{CostUSD: 10}},
	}
	// Two emails resolving to the same member for the same source.
	sources := map[string]DevtoolSource{
		"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 8, []DevtoolUserSpend{
			{UserEmail: "alice@x.com", CostUSD: 4, IdentityKey: "member:u1"},
			{UserEmail: "alice.alt@x.com", CostUSD: 4, IdentityKey: "member:u1"},
		}),
	}
	rows, _ := foldDevtoolPeopleRows(base, 10, sources, nil, false, members, "")
	alice := findPersonRow(rows, "member:u1")
	if alice == nil || len(alice.AgentsUsed) != 1 {
		t.Fatalf("expected exactly one deduped chip, got %+v", alice)
	}
}

func TestFoldDevtoolStatCards(t *testing.T) {
	cards := InsightsStatCards{
		Totals: AccountSummaryTotals{CostUSD: 10, Requests: 2, TotalTokens: 100},
		Change: &AccountSummaryChange{},
	}
	sources := map[string]DevtoolSource{"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 5, nil)}

	out := foldDevtoolStatCards(cards, sources, nil, false)
	if out.Totals.CostUSD != 15 {
		t.Fatalf("folded cost = %v, want 15", out.Totals.CostUSD)
	}
	if out.Change != nil {
		t.Fatal("Change must be dropped when folding (agent-only delta no longer describes the total)")
	}

	// Early return: no sources, agents shown → cards untouched (Change preserved).
	got := foldDevtoolStatCards(cards, map[string]DevtoolSource{}, nil, false)
	if got.Change == nil || got.Totals.CostUSD != 10 {
		t.Fatal("no-source fold should leave stat cards untouched")
	}
}

func TestDevtoolSourceRefsCarryIcon(t *testing.T) {
	sources := map[string]DevtoolSource{"claude-code": mkDevtoolSource("claude-code", "Claude Code", "anthropic", 5, nil)}
	refs := devtoolSourceRefs(sources)
	if len(refs) != 1 || refs[0].Icon != "anthropic" || refs[0].Label != "Claude Code" {
		t.Fatalf("devtool source refs = %+v, want one ref with anthropic icon", refs)
	}
}
