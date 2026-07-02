package handlers

import (
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
				{DeploymentID: "dep-alpha", AgentName: "alpha", DisplayName: "Alpha Agent", Requests: 5, CostUSD: 5},
				{DeploymentID: "dep-beta", AgentName: "beta", DisplayName: "Beta Agent", Requests: 10, CostUSD: 10},
				{DeploymentID: "dep-gamma", AgentName: "gamma", DisplayName: "Gamma Agent", Requests: 1, CostUSD: 1},
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
