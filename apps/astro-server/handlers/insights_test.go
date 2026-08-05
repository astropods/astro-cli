package handlers

import (
	"math"
	"testing"
	"time"
)

func TestBuildInsightsViewShapesServerOwnedRows(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	members := map[string]insightsMemberProfile{
		"u_alice": {
			userID:      "u_alice",
			username:    "alice",
			displayName: "Alice Chen",
		},
	}
	deployments := AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{
			{
				DeploymentID:   "dep-alpha",
				AgentName:      "alpha",
				DisplayName:    "Alpha Agent",
				Requests:       4,
				CostUSD:        4,
				CostPerRequest: 1,
				TotalTokens:    400,
				TokPerRequest:  100,
				P95LatencyMs:   250,
				LastSeen:       "2026-06-09T11:30:00Z",
				CostOverTime: []DeploymentDailyCost{
					{Date: "2026-06-01", CostUSD: 10},
					{Date: "2026-06-08", CostUSD: 1},
					{Date: "2026-06-09", CostUSD: 3},
				},
				RequestsOverTime: []DeploymentDailyRequests{
					{Date: "2026-06-01", Requests: 10},
					{Date: "2026-06-08", Requests: 1},
					{Date: "2026-06-09", Requests: 3},
				},
				TokensOverTime: []DeploymentDailyTokens{
					{Date: "2026-06-01", TotalTokens: 1000},
					{Date: "2026-06-08", TotalTokens: 100},
					{Date: "2026-06-09", TotalTokens: 300},
				},
				UsersUsed: []string{"u_alice", "U07ABCDEF"},
				UsersUsedDetails: []UserIdentity{
					{UserID: "u_alice", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
					{
						UserID: "U07ABCDEF",
						UserDetails: UserDetails{
							Kind:        UserDetailsKindSlack,
							TeamID:      "T07XYZ",
							DisplayName: "Slack Jesse",
							AvatarURL:   "https://cdn.example.com/jesse.png",
						},
					},
				},
			},
		},
	}
	users := AccountUsersSummaryResponse{
		Users: []UserSummaryEntry{
			{
				UserIdentity: UserIdentity{UserID: "u_alice", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
				Requests:     3,
				CostUSD:      3,
				Tokens:       300,
				LastSeen:     "2026-06-09T10:00:00Z",
				AgentsUsed:   []UserAgentRef{{DeploymentID: "dep-alpha", Name: "alpha", Account: "acme"}},
			},
			{
				UserIdentity: UserIdentity{
					UserID: "U07ABCDEF",
					UserDetails: UserDetails{
						Kind:        UserDetailsKindSlack,
						TeamID:      "T07XYZ",
						DisplayName: "Slack Jesse",
						AvatarURL:   "https://cdn.example.com/jesse.png",
					},
				},
				Requests:   1,
				CostUSD:    1,
				Tokens:     100,
				LastSeen:   "2026-06-09T11:00:00Z",
				AgentsUsed: []UserAgentRef{{DeploymentID: "dep-alpha", Name: "alpha", Account: "acme"}},
			},
			{
				UserIdentity: UserIdentity{UserID: ""},
				Requests:     1,
				CostUSD:      99,
				Tokens:       10,
			},
		},
	}
	summary := AccountObservabilitySummaryResponse{
		CostOverTimeByUser: []AccountCostOverTimeByUserEntry{
			{
				Date: "2026-06-08",
				Users: []AccountUserCost{
					{UserIdentity: UserIdentity{UserID: "u_alice"}, CostUSD: 1, Requests: 1, Tokens: 100},
				},
			},
			{
				Date: "2026-06-09",
				Users: []AccountUserCost{
					{UserIdentity: UserIdentity{UserID: "u_alice"}, CostUSD: 3, Requests: 3, Tokens: 300},
					{UserIdentity: UserIdentity{UserID: "U07ABCDEF"}, CostUSD: 1, Requests: 1, Tokens: 100},
				},
			},
		},
	}

	view := buildInsightsView("acme", summary, deployments, users, members, now)

	range7 := view.Ranges["7d"]
	if range7.Days != 7 {
		t.Fatalf("7d range days = %d, want 7", range7.Days)
	}
	if got, want := range7.StatCards.Totals.CostUSD, 4.0; got != want {
		t.Fatalf("7d stat cost = %v, want %v", got, want)
	}
	if got, want := range7.StatCards.Totals.Requests, 4; got != want {
		t.Fatalf("7d requests = %d, want %d", got, want)
	}
	if got, want := len(range7.PeopleSpendChart), 7; got != want {
		t.Fatalf("people chart points = %d, want %d", got, want)
	}
	lastPoint := range7.PeopleSpendChart[len(range7.PeopleSpendChart)-1]
	if got, want := lastPoint.Users, 2; got != want {
		t.Fatalf("last people point users = %d, want %d", got, want)
	}
	if got, want := lastPoint.Cost, 4.0; got != want {
		t.Fatalf("last people point cost = %v, want %v", got, want)
	}

	if got, want := view.Tables.Agents.Count, 1; got != want {
		t.Fatalf("agents count = %d, want %d", got, want)
	}
	agent := view.Tables.Agents.Rows[0]
	if got, want := agent.Identity.Href, "/acme/agents/dep-alpha/monitor"; got != want {
		t.Fatalf("agent href = %q, want %q", got, want)
	}
	if got, want := agent.Metrics.LastSeen, "2026-06-09T11:30:00Z"; got != want {
		t.Fatalf("agent last_seen = %q, want %q", got, want)
	}
	if got, want := len(agent.UsedBy), 2; got != want {
		t.Fatalf("agent used_by len = %d, want %d", got, want)
	}
	if got, want := agent.UsedBy[0].Kind, "member"; got != want {
		t.Fatalf("first used_by kind = %q, want %q", got, want)
	}
	if got, want := agent.UsedBy[1].Href, "slack://user?team=T07XYZ&id=U07ABCDEF"; got != want {
		t.Fatalf("slack used_by href = %q, want %q", got, want)
	}
	if got, want := agent.UsedBy[1].Label, "Slack Jesse"; got != want {
		t.Fatalf("slack used_by label = %q, want %q", got, want)
	}
	if got, want := agent.UsedBy[1].UserDetails.AvatarURL, "https://cdn.example.com/jesse.png"; got != want {
		t.Fatalf("slack used_by avatar = %q, want %q", got, want)
	}

	if got, want := view.Tables.People.Count, 3; got != want {
		t.Fatalf("people count = %d, want %d", got, want)
	}
	if got, want := view.Tables.People.Rows[0].Identity.Label, "System spend"; got == want {
		t.Fatalf("system spend should be pinned after identified users, got first row")
	}
	slack := view.Tables.People.Rows[1]
	if got, want := slack.Identity.Kind, "slack"; got != want {
		t.Fatalf("second people row kind = %q, want %q", got, want)
	}
	if got, want := slack.Identity.Href, "slack://user?team=T07XYZ&id=U07ABCDEF"; got != want {
		t.Fatalf("slack people href = %q, want %q", got, want)
	}
	if got, want := slack.Identity.Label, "Slack Jesse"; got != want {
		t.Fatalf("slack people label = %q, want %q", got, want)
	}
	if got, want := slack.Identity.UserDetails.TeamID, "T07XYZ"; got != want {
		t.Fatalf("slack people team = %q, want %q", got, want)
	}
	system := view.Tables.People.Rows[len(view.Tables.People.Rows)-1]
	if got, want := system.Identity.Kind, "system"; got != want {
		t.Fatalf("last people row kind = %q, want %q", got, want)
	}

	t.Run("empty deployments and users", func(t *testing.T) {
		view := buildInsightsView(
			"acme",
			AccountObservabilitySummaryResponse{},
			AccountDeploymentsSummaryResponse{},
			AccountUsersSummaryResponse{},
			map[string]insightsMemberProfile{},
			now,
		)
		if got := len(view.Tables.Agents.Rows); got != 0 {
			t.Fatalf("agent rows len = %d, want 0", got)
		}
		if got := len(view.Tables.People.Rows); got != 0 {
			t.Fatalf("people rows len = %d, want 0", got)
		}
		if got := view.Tables.Agents.TotalCost; got != 0 {
			t.Fatalf("agent total cost = %v, want 0", got)
		}
		if got := view.Tables.People.TotalCost; got != 0 {
			t.Fatalf("people total cost = %v, want 0", got)
		}
	})

	t.Run("multiple empty user rows merge into system spend", func(t *testing.T) {
		view := buildInsightsView(
			"acme",
			AccountObservabilitySummaryResponse{},
			AccountDeploymentsSummaryResponse{},
			AccountUsersSummaryResponse{
				Users: []UserSummaryEntry{
					{UserIdentity: UserIdentity{UserID: ""}, Requests: 1, CostUSD: 2, Tokens: 20},
					{UserIdentity: UserIdentity{UserID: ""}, Requests: 2, CostUSD: 3, Tokens: 30},
				},
			},
			map[string]insightsMemberProfile{},
			now,
		)
		if got := len(view.Tables.People.Rows); got != 1 {
			t.Fatalf("people rows len = %d, want 1", got)
		}
		row := view.Tables.People.Rows[0]
		if got, want := row.Identity.Kind, "system"; got != want {
			t.Fatalf("identity kind = %q, want %q", got, want)
		}
		if row.Metrics.CostUSD != 5 || row.Metrics.Requests != 3 || row.Metrics.Tokens != 50 {
			t.Fatalf("merged metrics mismatch: %+v", row.Metrics)
		}
	})

	t.Run("unidentified non-member non-slack user", func(t *testing.T) {
		view := buildInsightsView(
			"acme",
			AccountObservabilitySummaryResponse{},
			AccountDeploymentsSummaryResponse{},
			AccountUsersSummaryResponse{
				Users: []UserSummaryEntry{{
					UserIdentity: UserIdentity{UserID: "random-trace-id"},
					Requests:     1,
					CostUSD:      1,
				}},
			},
			map[string]insightsMemberProfile{},
			now,
		)
		if got := len(view.Tables.People.Rows); got != 1 {
			t.Fatalf("people rows len = %d, want 1", got)
		}
		if got, want := view.Tables.People.Rows[0].Identity.Kind, "unidentified"; got != want {
			t.Fatalf("identity kind = %q, want %q", got, want)
		}
	})

	t.Run("zero total cost produces zero cost percentage", func(t *testing.T) {
		view := buildInsightsView(
			"acme",
			AccountObservabilitySummaryResponse{},
			AccountDeploymentsSummaryResponse{
				Deployments: []DeploymentSummaryEntry{{DeploymentID: "dep-zero", AgentName: "zero"}},
			},
			AccountUsersSummaryResponse{
				Users: []UserSummaryEntry{{UserIdentity: UserIdentity{UserID: "random-trace-id"}}},
			},
			map[string]insightsMemberProfile{},
			now,
		)
		if got := view.Tables.Agents.Rows[0].Metrics.CostPct; got != 0 {
			t.Fatalf("agent cost_pct = %v, want 0", got)
		}
		if got := view.Tables.People.Rows[0].Metrics.CostPct; got != 0 {
			t.Fatalf("people cost_pct = %v, want 0", got)
		}
	})

	t.Run("table params filter sort and page rows without changing total counts", func(t *testing.T) {
		deployments := AccountDeploymentsSummaryResponse{
			Deployments: []DeploymentSummaryEntry{
				{DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent", Requests: 5, CostUSD: 5, LastSeen: "2026-06-08T10:00:00Z"},
				{DeploymentID: "dep-beta", AgentName: "beta", DisplayName: "Beta Agent", Requests: 10, CostUSD: 10, LastSeen: "2026-06-09T10:00:00Z"},
				{DeploymentID: "dep-gamma", AgentName: "gamma", DisplayName: "Gamma Agent", Requests: 1, CostUSD: 1, LastSeen: "2026-06-07T10:00:00Z"},
			},
		}
		users := AccountUsersSummaryResponse{
			Users: []UserSummaryEntry{
				{UserIdentity: UserIdentity{UserID: "u_alpha", UserDetails: UserDetails{Kind: UserDetailsKindAstro}}, Requests: 1, CostUSD: 1},
				{UserIdentity: UserIdentity{UserID: "u_beta", UserDetails: UserDetails{Kind: UserDetailsKindAstro}}, Requests: 10, CostUSD: 10},
				{UserIdentity: UserIdentity{UserID: "u_gamma", UserDetails: UserDetails{Kind: UserDetailsKindAstro}}, Requests: 5, CostUSD: 5},
			},
		}
		members := map[string]insightsMemberProfile{
			"u_alpha": {userID: "u_alpha", username: "alpha", displayName: "Alpha Person"},
			"u_beta":  {userID: "u_beta", username: "beta", displayName: "Beta Person"},
			"u_gamma": {userID: "u_gamma", username: "gamma", displayName: "Gamma Person"},
		}

		view := buildInsightsViewWithParams(
			"acme",
			AccountObservabilitySummaryResponse{},
			deployments,
			users,
			members,
			devtoolFold{},
			now,
			normalizeInsightsRequestParams(insightsRequestParams{
				Query:  "a",
				Agents: insightsTableParams{Limit: 2, Sort: "requests", Direction: "asc"},
				People: insightsTableParams{Limit: 2, Sort: "requests", Direction: "desc"},
			}),
		)

		if got, want := view.Tables.Agents.Count, 3; got != want {
			t.Fatalf("agents total count = %d, want %d", got, want)
		}
		if got, want := view.Tables.Agents.Pagination.FilteredCount, 3; got != want {
			t.Fatalf("agents filtered count = %d, want %d", got, want)
		}
		if got, want := len(view.Tables.Agents.Rows), 2; got != want {
			t.Fatalf("agent page len = %d, want %d", got, want)
		}
		if got, want := view.Tables.Agents.Rows[0].Key, "dep-gamma"; got != want {
			t.Fatalf("first agent row = %q, want %q", got, want)
		}
		if !view.Tables.Agents.Pagination.HasMore {
			t.Fatalf("agents pagination should have more rows")
		}

		lastSeenView := buildInsightsViewWithParams(
			"acme",
			AccountObservabilitySummaryResponse{},
			deployments,
			users,
			members,
			devtoolFold{},
			now,
			normalizeInsightsRequestParams(insightsRequestParams{
				Agents: insightsTableParams{Limit: 3, Sort: "last_seen", Direction: "desc"},
			}),
		)
		if got, want := lastSeenView.Tables.Agents.Rows[0].Key, "dep-beta"; got != want {
			t.Fatalf("first agent row by last_seen = %q, want %q", got, want)
		}

		if got, want := view.Tables.People.Count, 3; got != want {
			t.Fatalf("people total count = %d, want %d", got, want)
		}
		if got, want := len(view.Tables.People.Rows), 2; got != want {
			t.Fatalf("people page len = %d, want %d", got, want)
		}
		if got, want := view.Tables.People.Rows[0].Key, "member:u_beta"; got != want {
			t.Fatalf("first people row = %q, want %q", got, want)
		}
		if !view.Tables.People.Pagination.HasMore {
			t.Fatalf("people pagination should have more rows")
		}

		invalidParams := normalizeInsightsRequestParams(insightsRequestParams{
			Agents: insightsTableParams{Limit: 2, Sort: "unknown_sort", Direction: "desc"},
			People: insightsTableParams{Limit: 2, Sort: "unknown_sort", Direction: "desc"},
		})
		if got, want := invalidParams.Agents.Sort, "cost_usd"; got != want {
			t.Fatalf("agent invalid sort normalized to %q, want %q", got, want)
		}
		if got, want := invalidParams.People.Sort, "cost_usd"; got != want {
			t.Fatalf("people invalid sort normalized to %q, want %q", got, want)
		}
		invalidSortView := buildInsightsViewWithParams(
			"acme",
			AccountObservabilitySummaryResponse{},
			deployments,
			users,
			members,
			devtoolFold{},
			now,
			invalidParams,
		)
		if got, want := invalidSortView.Tables.Agents.Rows[0].Key, "dep-beta"; got != want {
			t.Fatalf("first agent row with invalid sort = %q, want default cost sort row %q", got, want)
		}
		if got, want := invalidSortView.Tables.People.Rows[0].Key, "member:u_beta"; got != want {
			t.Fatalf("first people row with invalid sort = %q, want default cost sort row %q", got, want)
		}

		skipRangesView := buildInsightsViewWithParams(
			"acme",
			AccountObservabilitySummaryResponse{},
			deployments,
			users,
			members,
			devtoolFold{},
			now,
			normalizeInsightsRequestParams(insightsRequestParams{SkipRanges: true}),
		)
		if got := len(skipRangesView.Ranges); got != 0 {
			t.Fatalf("ranges len with skip_ranges = %d, want 0", got)
		}
		if got, want := len(skipRangesView.Tables.Agents.Rows), 3; got != want {
			t.Fatalf("agent rows len with skip_ranges = %d, want %d", got, want)
		}
		if got, want := len(skipRangesView.Tables.People.Rows), 3; got != want {
			t.Fatalf("people rows len with skip_ranges = %d, want %d", got, want)
		}
	})

	t.Run("default table limit is 25 when limit is missing or invalid", func(t *testing.T) {
		defaults := normalizeInsightsRequestParams(insightsRequestParams{})
		if got, want := defaults.Agents.Limit, 25; got != want {
			t.Fatalf("default agents limit = %d, want %d", got, want)
		}
		if got, want := defaults.People.Limit, 25; got != want {
			t.Fatalf("default people limit = %d, want %d", got, want)
		}

		fallback := normalizeInsightsRequestParams(insightsRequestParams{
			Agents: insightsTableParams{Limit: 0},
			People: insightsTableParams{Limit: -3},
		})
		if got, want := fallback.Agents.Limit, 25; got != want {
			t.Fatalf("fallback agents limit = %d, want %d", got, want)
		}
		if got, want := fallback.People.Limit, 25; got != want {
			t.Fatalf("fallback people limit = %d, want %d", got, want)
		}
	})
}

func TestInsightUserIdentity_SlackOnlyTooltipAndFullName(t *testing.T) {
	slack := insightUserIdentity(UserIdentity{
		UserID: "U123",
		UserDetails: UserDetails{
			Kind:     UserDetailsKindSlack,
			TeamID:   "T123",
			Username: "christopher.patty",
		},
	}, nil)
	if slack.Label != "Christopher Patty" {
		t.Fatalf("slack label = %q, want %q", slack.Label, "Christopher Patty")
	}
	if slack.Tooltip != "Slack User" {
		t.Fatalf("slack tooltip = %q, want %q", slack.Tooltip, "Slack User")
	}

	withDisplayName := insightUserIdentity(UserIdentity{
		UserID: "U456",
		UserDetails: UserDetails{
			Kind:        UserDetailsKindSlack,
			DisplayName: "Christopher Patty",
			Username:    "christopher.patty",
		},
	}, nil)
	if withDisplayName.Label != "Christopher Patty" {
		t.Fatalf("slack display name label = %q, want %q", withDisplayName.Label, "Christopher Patty")
	}

	withStaleHandleDisplayName := insightUserIdentity(UserIdentity{
		UserID: "U457",
		UserDetails: UserDetails{
			Kind:        UserDetailsKindSlack,
			DisplayName: "christopher.patty",
			Username:    "christopher.patty",
		},
	}, nil)
	if withStaleHandleDisplayName.Label != "Christopher Patty" {
		t.Fatalf("slack stale handle display name label = %q, want %q", withStaleHandleDisplayName.Label, "Christopher Patty")
	}

	withInitialsDisplayName := insightUserIdentity(UserIdentity{
		UserID: "U789",
		UserDetails: UserDetails{
			Kind:        UserDetailsKindSlack,
			DisplayName: "will.i.am",
			Username:    "will.i.am",
		},
	}, nil)
	if withInitialsDisplayName.Label != "will.i.am" {
		t.Fatalf("slack display name with punctuation label = %q, want %q", withInitialsDisplayName.Label, "will.i.am")
	}

	withLowercaseLiteraryName := insightUserIdentity(UserIdentity{
		UserID: "U790",
		UserDetails: UserDetails{
			Kind:        UserDetailsKindSlack,
			DisplayName: "e.e.cummings",
			Username:    "e.e.cummings",
		},
	}, nil)
	if withLowercaseLiteraryName.Label != "e.e.cummings" {
		t.Fatalf("slack lowercase punctuated display name label = %q, want %q", withLowercaseLiteraryName.Label, "e.e.cummings")
	}
}

// Agent chips flag deletion so the client can label them "(deleted)". Archived
// deployments are now carried in the deployments list so their spend still
// counts toward account totals — which means absence from that list is no
// longer sufficient to detect deletion, and an archived agent's chip would
// otherwise render as live while its row in the agents table shows an archive
// marker.
func TestInsightAgentChipsFlagArchivedAndMissing(t *testing.T) {
	depByID := map[string]DeploymentSummaryEntry{
		"dep-live": {DeploymentID: "dep-live", AgentName: "live-agent", DisplayName: "Live Agent"},
		"dep-old":  {DeploymentID: "dep-old", AgentName: "old-agent", DisplayName: "Old Agent", IsArchived: true},
	}
	chips := insightAgentChips([]UserAgentRef{
		{DeploymentID: "dep-live", Name: "live-agent", Account: "acme"},
		{DeploymentID: "dep-old", Name: "old-agent", Account: "acme"},
		{DeploymentID: "dep-gone", Name: "gone-agent", Account: "acme"},
	}, depByID)

	if len(chips) != 3 {
		t.Fatalf("chips = %d, want 3", len(chips))
	}

	if chips[0].IsDeleted {
		t.Error("live chip marked deleted")
	}
	if chips[0].Href != "/acme/agents/dep-live/monitor" {
		t.Errorf("live href = %q, want the monitor page", chips[0].Href)
	}

	// Archived: present in the list, but the deployment is gone.
	if !chips[1].IsDeleted {
		t.Error("archived chip not marked deleted")
	}
	if chips[1].Label != "Old Agent" {
		t.Errorf("archived label = %q, want the retained display name", chips[1].Label)
	}

	// Absent entirely — the pre-existing case, unchanged.
	if !chips[2].IsDeleted {
		t.Error("missing chip not marked deleted")
	}

	// Neither deleted case may point at a monitor page that no longer exists,
	// and neither may have an empty href: the client reads that as "external".
	for _, i := range []int{1, 2} {
		if chips[i].Href != "/acme/"+chips[i].AvatarName {
			t.Errorf("chip %d href = %q, want the agent page", i, chips[i].Href)
		}
	}
}

// v1's fold shape: dev-tool spend arrives from a separate pipeline and is absent
// from every base surface, so all of them are folded. Kept because v1 still
// serves this way; v2 uses the single-lineage path exercised below.
func TestDevtoolFoldAppliesToEverySurface(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	deployments := AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{{
			DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent",
			Requests: 4, CostUSD: 4,
			CostOverTime: []DeploymentDailyCost{{Date: "2026-06-09", CostUSD: 4}},
		}},
	}
	// The member's dev-tool spend is already merged into their row by the facts,
	// exactly as rollupUserEntries produces it.
	users := AccountUsersSummaryResponse{
		Users: []UserSummaryEntry{{
			UserIdentity: UserIdentity{UserID: "u_alpha", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
			Requests:     4, CostUSD: 6,
		}},
	}
	members := map[string]insightsMemberProfile{
		"u_alpha": {userID: "u_alpha", username: "alpha", displayName: "Alpha Person"},
	}

	totals := DevtoolTotals{CostUSD: 2, TotalTokens: 100}
	ad := devtoolAdapters[0]
	sources := map[string]DevtoolSource{ad.Key: {
		Label:      ad.Label,
		Totals:     totals,
		SpendByDay: []DevtoolSpendPoint{{Date: "2026-06-09", CostUSD: 2}},
		AgentRow:   devtoolAgentRow(ad, totals),
		// ByUser left empty: v2 never folds the People surface.
	}}
	ranges := map[string]DevtoolRange{}
	for _, spec := range insightsRangeSpecs {
		ranges[spec.key] = DevtoolRange{Sources: sources}
	}

	view := buildInsightsViewWithParams("acme",
		AccountObservabilitySummaryResponse{},
		deployments, users, members,
		devtoolFold{Ranges: ranges, AgentRows: sources, Present: sources},
		now, normalizeInsightsRequestParams(defaultInsightsRequestParams()))

	// The synthetic source row must appear in the agents table.
	var found bool
	for _, r := range view.Tables.Agents.Rows {
		if r.Key == ad.Key {
			found = true
			if r.Metrics.CostUSD != 2 {
				t.Errorf("devtool row cost = %v, want 2", r.Metrics.CostUSD)
			}
		}
	}
	if !found {
		t.Fatalf("devtool agent row missing; rows = %d", len(view.Tables.Agents.Rows))
	}
	// Agents total includes it, because agent rows never carried it.
	if view.Tables.Agents.TotalCost != 6 {
		t.Errorf("agents total = %v, want 6 (4 agent + 2 devtool)", view.Tables.Agents.TotalCost)
	}

	// People must be untouched: the row already includes dev-tool spend, so a
	// fold here would push it to 8 and add a duplicate synthesized row.
	if len(view.Tables.People.Rows) != 1 {
		t.Fatalf("people rows = %d, want 1 (no synthesized duplicate)", len(view.Tables.People.Rows))
	}
	if got := view.Tables.People.Rows[0].Metrics.CostUSD; got != 6 {
		t.Errorf("people cost = %v, want 6 (unchanged, not double-folded)", got)
	}

	// Stat cards must include dev-tool spend. Without the range fold they show
	// only the 4 of agent spend while the table beneath totals 6 — the page
	// contradicting itself.
	if got := view.Ranges["7d"].StatCards.Totals.CostUSD; got != 6 {
		t.Errorf("7d stat card cost = %v, want 6 (4 agent + 2 devtool)", got)
	}
	// And the chart must carry a series for the source, or the spend is in the
	// total with nothing to attribute it to.
	var chartHasSource bool
	for _, entry := range view.Ranges["7d"].AgentSpendChart {
		for _, m := range entry.Models {
			if m.Model == ad.Key && m.CostUSD > 0 {
				chartHasSource = true
			}
		}
	}
	if !chartHasSource {
		t.Error("agent spend chart has no dev-tool series")
	}
	if view.Ranges["7d"].SeriesLabels[ad.Key] != ad.Label {
		t.Errorf("series label for %s = %q, want %q", ad.Key,
			view.Ranges["7d"].SeriesLabels[ad.Key], ad.Label)
	}

	// The Sources filter still lists the source so it can be toggled.
	if len(view.DevtoolSources) != 1 || view.DevtoolSources[0].Key != ad.Key {
		t.Errorf("devtool_sources = %+v, want one entry for %s", view.DevtoolSources, ad.Key)
	}

	// All fixture spend is on `now`, so every range covers it and the
	// range-scoped surfaces must agree with the account-wide tables.
	assertInsightsSurfacesAgree(t, view, 6)
}

// assertInsightsSurfacesAgree checks the one property every dev-tool regression
// on this page violated: the surfaces must not contradict each other. Stat
// cards, both table totals, and the chart series all describe the same spend, so
// any of them disagreeing means a surface either missed a contribution or
// counted one twice — which is invisible in the response and easy to reason
// wrongly about, since the surfaces derive from different lineages.
//
// Callers must place all fixture spend inside the narrowest range, so that the
// range-scoped surfaces and the account-wide tables are comparable.
func assertInsightsSurfacesAgree(t *testing.T, view InsightsResponse, want float64) {
	t.Helper()
	const tolerance = 0.0001

	if got := view.Tables.Agents.TotalCost; math.Abs(got-want) > tolerance {
		t.Errorf("agents table total = %v, want %v", got, want)
	}
	if got := view.Tables.People.TotalCost; math.Abs(got-want) > tolerance {
		t.Errorf("people table total = %v, want %v", got, want)
	}

	for _, spec := range insightsRangeSpecs {
		r, ok := view.Ranges[spec.key]
		if !ok {
			t.Fatalf("range %s missing", spec.key)
		}
		if got := r.StatCards.Totals.CostUSD; math.Abs(got-want) > 0.01 {
			t.Errorf("%s stat card cost = %v, want %v", spec.key, got, want)
		}
		var chart float64
		for _, entry := range r.AgentSpendChart {
			for _, m := range entry.Models {
				chart += m.CostUSD
			}
		}
		if math.Abs(chart-want) > tolerance {
			t.Errorf("%s chart series sum = %v, want %v", spec.key, chart, want)
		}
	}
}

// The v2 path folds nothing. Dev-tool spend is a first-class deployment entry,
// so the stat cards, chart, series labels and agents table pick it up from the
// same lineage as agent spend, and the People surfaces from the actor grain.
//
// This is the property the fold kept getting wrong: with one lineage there is no
// per-surface reasoning left to get wrong, so the invariant holds by
// construction rather than by remembering which surfaces to compensate.
func TestDevtoolAsDeploymentEntryNeedsNoFold(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	ad := devtoolAdapters[0]
	deployments := AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{
			{
				DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent",
				Requests: 4, CostUSD: 4, TotalTokens: 400,
				CostOverTime:     []DeploymentDailyCost{{Date: "2026-06-09", CostUSD: 4}},
				RequestsOverTime: []DeploymentDailyRequests{{Date: "2026-06-09", Requests: 4}},
				TokensOverTime:   []DeploymentDailyTokens{{Date: "2026-06-09", TotalTokens: 400}},
			},
			{
				// Synthetic dev-tool entry, as rollupDeploymentEntries builds it:
				// no requests, keyed by source.
				DeploymentID: ad.Key, DevtoolSourceKey: ad.Key, AgentName: ad.Label,
				CostUSD: 2, TotalTokens: 100,
				CostOverTime:   []DeploymentDailyCost{{Date: "2026-06-09", CostUSD: 2}},
				TokensOverTime: []DeploymentDailyTokens{{Date: "2026-06-09", TotalTokens: 100}},
			},
		},
	}
	users := AccountUsersSummaryResponse{
		Users: []UserSummaryEntry{{
			UserIdentity: UserIdentity{UserID: "u_alpha", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
			Requests:     4, CostUSD: 6,
		}},
	}
	members := map[string]insightsMemberProfile{
		"u_alpha": {userID: "u_alpha", username: "alpha", displayName: "Alpha Person"},
	}

	view := buildInsightsViewWithParams("acme",
		AccountObservabilitySummaryResponse{},
		deployments, users, members,
		devtoolFold{Present: map[string]DevtoolSource{ad.Key: {Label: ad.Label}}},
		now, normalizeInsightsRequestParams(defaultInsightsRequestParams()))

	assertInsightsSurfacesAgree(t, view, 6)

	var row *InsightsAgentRow
	for i := range view.Tables.Agents.Rows {
		if view.Tables.Agents.Rows[i].Key == ad.Key {
			row = &view.Tables.Agents.Rows[i]
		}
	}
	if row == nil {
		t.Fatal("dev-tool row missing from agents table")
	}
	// Rendered as an aggregated source, not as a deployed agent: no monitor link.
	if row.Identity.Kind != "system" {
		t.Errorf("identity kind = %q, want system", row.Identity.Kind)
	}
	if row.Identity.Href != "" {
		t.Errorf("identity href = %q, want empty", row.Identity.Href)
	}
	if row.Identity.Icon != ad.Icon {
		t.Errorf("identity icon = %q, want %q", row.Identity.Icon, ad.Icon)
	}
	// requests == 0 is expected for a dev-tool source, so the not-instrumented
	// warning must not fire.
	if row.NotInstrumented {
		t.Error("dev-tool row flagged not_instrumented")
	}
	// It contributes spend but is not a deployed agent.
	if got := view.Ranges["7d"].StatCards.Totals.ActiveAgents; got != 1 {
		t.Errorf("active_agents = %d, want 1 (dev-tool source excluded)", got)
	}
	if view.Ranges["7d"].SeriesLabels[ad.Key] != ad.Label {
		t.Errorf("series label = %q, want %q", view.Ranges["7d"].SeriesLabels[ad.Key], ad.Label)
	}
}

// Agent usage that never reported which agent produced it has no agent row to
// belong to. v1 hides it entirely by filtering on the deployment tag upstream,
// which understates account spend; v2 surfaces it as its own row so the totals
// stay whole and the gap is visible rather than silent.
func TestUnattributedUsageRendersAsItsOwnRow(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	deployments := AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{
			{
				DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent",
				Requests: 4, CostUSD: 4, TotalTokens: 400,
				CostOverTime:     []DeploymentDailyCost{{Date: "2026-06-09", CostUSD: 4}},
				RequestsOverTime: []DeploymentDailyRequests{{Date: "2026-06-09", Requests: 4}},
				TokensOverTime:   []DeploymentDailyTokens{{Date: "2026-06-09", TotalTokens: 400}},
			},
			{
				// As rollupDeploymentEntries builds it: real requests, no agent.
				DeploymentID: unattributedAgentKey, IsUnattributed: true,
				AgentName: "Unattributed usage",
				Requests:  3, CostUSD: 3, TotalTokens: 300,
				CostOverTime:     []DeploymentDailyCost{{Date: "2026-06-09", CostUSD: 3}},
				RequestsOverTime: []DeploymentDailyRequests{{Date: "2026-06-09", Requests: 3}},
				TokensOverTime:   []DeploymentDailyTokens{{Date: "2026-06-09", TotalTokens: 300}},
			},
		},
	}
	users := AccountUsersSummaryResponse{
		Users: []UserSummaryEntry{{
			UserIdentity: UserIdentity{UserID: "u_alpha", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
			Requests:     7, CostUSD: 7,
		}},
	}
	members := map[string]insightsMemberProfile{"u_alpha": {userID: "u_alpha", username: "alpha"}}

	view := buildInsightsViewWithParams("acme",
		AccountObservabilitySummaryResponse{},
		deployments, users, members, devtoolFold{},
		now, normalizeInsightsRequestParams(defaultInsightsRequestParams()))

	// The whole point: nothing is hidden, so every surface still agrees.
	assertInsightsSurfacesAgree(t, view, 7)

	var row *InsightsAgentRow
	for i := range view.Tables.Agents.Rows {
		if view.Tables.Agents.Rows[i].Key == unattributedAgentKey {
			row = &view.Tables.Agents.Rows[i]
		}
	}
	if row == nil {
		t.Fatal("unattributed row missing from agents table")
	}
	if row.Identity.Kind != "system" {
		t.Errorf("identity kind = %q, want system", row.Identity.Kind)
	}
	// No agent exists behind it, so there is nothing to open.
	if row.Identity.Href != "" {
		t.Errorf("identity href = %q, want empty", row.Identity.Href)
	}
	if row.Identity.Tooltip == "" {
		t.Error("unattributed row has no tooltip explaining why it exists")
	}
	// It has real requests, so it must not be mistaken for an idle agent...
	if row.NotInstrumented {
		t.Error("unattributed row flagged not_instrumented")
	}
	// ...nor counted as one.
	if got := view.Ranges["7d"].StatCards.Totals.ActiveAgents; got != 1 {
		t.Errorf("active_agents = %d, want 1 (unattributed excluded)", got)
	}
}

// Dev-tool usage has no deployment, so it can't be a UserAgentRef and Pairs
// can't see it — a person who used Claude Code showed no chip for it at all.
// The source keys ride on the entry instead, and the chips are built alongside
// the agent chips so they land in the same place v1 put them.
func TestPersonRowCarriesDevtoolChips(t *testing.T) {
	ad := devtoolAdapters[0]
	users := AccountUsersSummaryResponse{
		Users: []UserSummaryEntry{{
			UserIdentity:      UserIdentity{UserID: "u_alpha", UserDetails: UserDetails{Kind: UserDetailsKindAstro}},
			Requests:          3,
			CostUSD:           3,
			AgentsUsed:        []UserAgentRef{{DeploymentID: "dep-alpha", Name: "alpha", Account: "acme"}},
			DevtoolSourceKeys: []string{ad.Key},
		}},
	}
	deployments := []DeploymentSummaryEntry{
		{DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent"},
	}

	rows, _ := buildInsightsPeopleRows("acme", users.Users, deployments,
		map[string]insightsMemberProfile{"u_alpha": {userID: "u_alpha", username: "alpha"}})

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	// The agent chip stays first; the dev-tool chip is appended after it.
	if len(rows[0].AgentsUsed) != 2 {
		t.Fatalf("chips = %d, want 2: %+v", len(rows[0].AgentsUsed), rows[0].AgentsUsed)
	}
	chip := rows[0].AgentsUsed[1]
	if chip.Key != "devtool:"+ad.Key {
		t.Errorf("chip key = %q, want devtool:%s", chip.Key, ad.Key)
	}
	if chip.Label != ad.Label {
		t.Errorf("chip label = %q, want %q", chip.Label, ad.Label)
	}
	// The brand icon is what the client renders as the logo.
	if chip.Icon != ad.Icon {
		t.Errorf("chip icon = %q, want %q", chip.Icon, ad.Icon)
	}
	// No Href: that is what makes the client tag it "External" instead of linking
	// to a deployment that doesn't exist.
	if chip.Href != "" {
		t.Errorf("chip href = %q, want empty", chip.Href)
	}
}

// The Sources filter renders a brand logo per source, and devtoolSourceRefs
// reads that icon off AgentRow.Identity — so a source map built without the row
// yields a filter with no icons.
func TestPresentDevtoolSourcesCarryIcon(t *testing.T) {
	ad := devtoolAdapters[0]
	refs := devtoolSourceRefs(map[string]DevtoolSource{ad.Key: {
		Label:    ad.Label,
		AgentRow: InsightsAgentRow{Identity: devtoolIdentity(ad)},
	}})
	if len(refs) != 1 {
		t.Fatalf("refs = %d, want 1", len(refs))
	}
	if refs[0].Icon != ad.Icon {
		t.Errorf("icon = %q, want %q", refs[0].Icon, ad.Icon)
	}
	if refs[0].Label != ad.Label {
		t.Errorf("label = %q, want %q", refs[0].Label, ad.Label)
	}
}

// Several deployments of one agent share a display name, so the chart legend
// needs a discriminator. It used to be the namespace ("astro-y3nso9vce-0"),
// which reads as infra noise and can't be matched against anything else on the
// page — the deployment id can, since it keys the table rows and their links.
func TestSeriesLabelsDisambiguateByDeploymentID(t *testing.T) {
	labels := buildInsightsSeriesLabels([]DeploymentSummaryEntry{
		{DeploymentID: "y3n-so9-vce", DisplayName: "Sasbot", Namespace: "astro-y3nso9vce-0"},
		{DeploymentID: "7bj-k89-g6q", DisplayName: "Sasbot", Namespace: "astro-7bjk89g6q-0"},
		{DeploymentID: "sol-o11-abc", DisplayName: "Solo Agent", Namespace: "astro-solo11abc-0"},
	})

	if got, want := labels["y3n-so9-vce"], "Sasbot (y3n-so9-vce)"; got != want {
		t.Errorf("duplicate label = %q, want %q", got, want)
	}
	if got, want := labels["7bj-k89-g6q"], "Sasbot (7bj-k89-g6q)"; got != want {
		t.Errorf("duplicate label = %q, want %q", got, want)
	}
	// A unique name needs no discriminator, so it stays clean.
	if got, want := labels["sol-o11-abc"], "Solo Agent"; got != want {
		t.Errorf("unique label = %q, want %q", got, want)
	}
}

// A deployment with no name at all still needs a distinguishable legend entry,
// or several of them collapse to the same blank string.
func TestSeriesLabelsDisambiguateUnnamedDeployments(t *testing.T) {
	labels := buildInsightsSeriesLabels([]DeploymentSummaryEntry{
		{DeploymentID: "aaa-bbb-ccc"},
		{DeploymentID: "ddd-eee-fff"},
	})
	if labels["aaa-bbb-ccc"] == labels["ddd-eee-fff"] {
		t.Errorf("unnamed deployments share label %q", labels["aaa-bbb-ccc"])
	}
}

// The range chip used to change the charts and leave the tables alone, which is
// the page's oldest confusion: cards said one thing, the table beneath said
// another. TableDays scopes the agents table to the same window.
func TestTableDaysScopesTheAgentsTable(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	// Spend on two days: one inside a 7-day window, one only inside 90.
	deployments := AccountDeploymentsSummaryResponse{
		Deployments: []DeploymentSummaryEntry{{
			DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent",
			// Entry totals must match the daily series: the unscoped path reads
			// these fields directly, while the scoped path recomputes them from the
			// series, so a fixture that disagrees with itself tests nothing.
			Requests: 13, CostUSD: 13,
			CostOverTime: []DeploymentDailyCost{
				{Date: "2026-04-01", CostUSD: 10},
				{Date: "2026-06-09", CostUSD: 3},
			},
			RequestsOverTime: []DeploymentDailyRequests{
				{Date: "2026-04-01", Requests: 10},
				{Date: "2026-06-09", Requests: 3},
			},
		}},
	}

	scoped := func(days int) float64 {
		p := normalizeInsightsRequestParams(defaultInsightsRequestParams())
		p.TableDays = days
		v := buildInsightsViewWithParams("acme",
			AccountObservabilitySummaryResponse{},
			deployments, AccountUsersSummaryResponse{},
			map[string]insightsMemberProfile{}, devtoolFold{}, now, p)
		return v.Tables.Agents.TotalCost
	}

	// 7d covers only the recent day.
	if got := scoped(7); got != 3 {
		t.Errorf("7d table total = %v, want 3", got)
	}
	// 90d covers both.
	if got := scoped(90); got != 13 {
		t.Errorf("90d table total = %v, want 13", got)
	}
	// Zero means account-wide, which is what v1 must keep using.
	if got := scoped(0); got != 13 {
		t.Errorf("unscoped table total = %v, want 13", got)
	}

	// The table now agrees with the stat cards for the same range, which is the
	// whole point of the change.
	p := normalizeInsightsRequestParams(defaultInsightsRequestParams())
	p.TableDays = 7
	v := buildInsightsViewWithParams("acme",
		AccountObservabilitySummaryResponse{},
		deployments, AccountUsersSummaryResponse{},
		map[string]insightsMemberProfile{}, devtoolFold{}, now, p)
	if card, table := v.Ranges["7d"].StatCards.Totals.CostUSD, v.Tables.Agents.TotalCost; card != table {
		t.Errorf("7d stat card %v disagrees with table %v", card, table)
	}
}
