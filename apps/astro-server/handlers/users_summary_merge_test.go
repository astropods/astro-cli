package handlers

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// ── mergeLinkedSlackRows ────────────────────────────────────────────────────

// Linked Slack user (Bob) has pre-link bare-id history AND a post-link
// WorkOS-keyed row from new activity. The merge rolls bare metrics into
// the WorkOS row so Insights shows one row per human, not a split.
// Number accuracy: cost/requests/tokens sum exactly; last_seen takes max;
// agents_used unions without duplicates.
func TestMergeLinkedSlackRows_LinkedUser_MergesIntoExistingWorkOSRow(t *testing.T) {
	rows := []UserSummaryEntry{
		{
			UserIdentityRef: UserIdentityRef{UserID: "U07BOBBOB1"},
			Requests:        10, CostUSD: 5.0, Tokens: 1000,
			LastSeen: "2026-04-01T00:00:00Z",
			AgentsUsed: []UserAgentRef{
				{DeploymentID: "dep-old-bot", Name: "old-bot", Account: "postman"},
			},
		},
		{
			UserIdentityRef: UserIdentityRef{UserID: "user_01HXX_bob"},
			Requests:        3, CostUSD: 2.5, Tokens: 300,
			LastSeen: "2026-06-01T12:00:00Z",
			AgentsUsed: []UserAgentRef{
				{DeploymentID: "dep-new-bot", Name: "new-bot", Account: "postman"},
			},
		},
	}
	entries := map[string]slackidentity.DirectoryEntry{
		"U07BOBBOB1": {TeamID: "T07POSTMAN", WorkOSUserID: "user_01HXX_bob"},
	}

	out := mergeLinkedSlackRows(rows, entries)

	if len(out) != 1 {
		t.Fatalf("expected 1 row after merge, got %d: %+v", len(out), out)
	}
	bob := out[0]
	if bob.UserID != "user_01HXX_bob" {
		t.Errorf("merged row should be keyed by WorkOS id, got %q", bob.UserID)
	}
	if bob.Requests != 13 || bob.CostUSD != 7.5 || bob.Tokens != 1300 {
		t.Errorf("metrics did not sum correctly: %+v", bob)
	}
	if bob.LastSeen != "2026-06-01T12:00:00Z" {
		t.Errorf("last_seen should be the later one, got %q", bob.LastSeen)
	}
	if len(bob.AgentsUsed) != 2 {
		t.Errorf("agents_used should union (no dup), got %+v", bob.AgentsUsed)
	}
	if bob.SlackTeamID != "" {
		t.Errorf("merged row is a WorkOS row; slack_team_id must be empty, got %q", bob.SlackTeamID)
	}
}

// Same merge but with the WorkOS row appearing BEFORE the bare-Slack row
// in input. An earlier version of mergeLinkedSlackRows tracked indices
// into the input slice and wrote merges back to it — but once a row had
// already been copied into the output slice, those writes were lost (the
// output kept the pre-merge cost). Pin the fix so future refactors can't
// regress order-dependence.
func TestMergeLinkedSlackRows_LinkedUser_WorkOSRowAppearsBeforeBareSlack(t *testing.T) {
	rows := []UserSummaryEntry{
		// WorkOS row first — the bug case.
		{
			UserIdentityRef: UserIdentityRef{UserID: "user_01HXX_bob"},
			Requests:        3, CostUSD: 2.5, Tokens: 300,
			LastSeen: "2026-06-01T12:00:00Z",
			AgentsUsed: []UserAgentRef{
				{DeploymentID: "dep-new-bot", Name: "new-bot", Account: "postman"},
			},
		},
		{
			UserIdentityRef: UserIdentityRef{UserID: "U07BOBBOB1"},
			Requests:        10, CostUSD: 5.0, Tokens: 1000,
			LastSeen: "2026-04-01T00:00:00Z",
			AgentsUsed: []UserAgentRef{
				{DeploymentID: "dep-old-bot", Name: "old-bot", Account: "postman"},
			},
		},
	}
	entries := map[string]slackidentity.DirectoryEntry{
		"U07BOBBOB1": {TeamID: "T07POSTMAN", WorkOSUserID: "user_01HXX_bob"},
	}

	out := mergeLinkedSlackRows(rows, entries)

	if len(out) != 1 {
		t.Fatalf("expected 1 merged row regardless of input order, got %d: %+v", len(out), out)
	}
	bob := out[0]
	if bob.UserID != "user_01HXX_bob" {
		t.Errorf("merged row should be keyed by WorkOS id, got %q", bob.UserID)
	}
	if bob.Requests != 13 || bob.CostUSD != 7.5 || bob.Tokens != 1300 {
		t.Errorf("metrics must sum exactly regardless of input order: %+v", bob)
	}
	if bob.LastSeen != "2026-06-01T12:00:00Z" {
		t.Errorf("last_seen should be the later one, got %q", bob.LastSeen)
	}
	if len(bob.AgentsUsed) != 2 {
		t.Errorf("agents_used should union (no dup), got %+v", bob.AgentsUsed)
	}
}

// Linked Slack user with no post-link activity: only the scoped Slack row is in
// Langfuse. The merge synthesizes a WorkOS-keyed row carrying the metrics so the
// next user-list render shows Bob under his Astro name.
func TestMergeLinkedSlackRows_LinkedUser_NoExistingWorkOSRow_Synthesizes(t *testing.T) {
	rows := []UserSummaryEntry{
		{
			UserIdentityRef: UserIdentityRef{UserID: "U07BOBBOB1"},
			Requests:        10, CostUSD: 5.0, Tokens: 1000,
			LastSeen: "2026-04-01T00:00:00Z",
		},
	}
	entries := map[string]slackidentity.DirectoryEntry{
		"U07BOBBOB1": {TeamID: "T07POSTMAN", WorkOSUserID: "user_01HXX_bob"},
	}

	out := mergeLinkedSlackRows(rows, entries)

	if len(out) != 1 {
		t.Fatalf("expected 1 synthesized row, got %d", len(out))
	}
	if out[0].UserID != "user_01HXX_bob" {
		t.Errorf("synthesized row should carry WorkOS id, got %q", out[0].UserID)
	}
	if out[0].CostUSD != 5.0 || out[0].Requests != 10 {
		t.Errorf("synthesized row should carry bare metrics: %+v", out[0])
	}
	if out[0].SlackTeamID != "" {
		t.Errorf("synthesized row is a WorkOS row; slack_team_id must be empty, got %q", out[0].SlackTeamID)
	}
}

// Unscoped bare Slack rows can still be enriched when the directory lookup has
// already proven there is exactly one possible workspace for that Slack user.
func TestMergeLinkedSlackRows_UnscopedObservedUnique_StampsTeamID(t *testing.T) {
	rows := []UserSummaryEntry{
		{UserIdentityRef: UserIdentityRef{UserID: "U07CAROL00"}, CostUSD: 1.5, Requests: 4, Tokens: 400},
	}
	entries := map[string]slackidentity.DirectoryEntry{
		"U07CAROL00": {
			TeamID:        "T07POSTMAN",
			WorkspaceName: "Postman",
			Profile: slackidentity.SlackProfile{
				DisplayName: "Carol Chen",
				Username:    "carol",
				AvatarURL:   "https://avatars.slack-edge.com/carol.png",
			},
		},
	}

	out := mergeLinkedSlackRows(rows, entries)

	if len(out) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out))
	}
	if out[0].IdentityKey != "slack:T07POSTMAN:U07CAROL00" {
		t.Errorf("expected Slack directory identity key, got %q", out[0].IdentityKey)
	}
	if out[0].SlackTeamID != "T07POSTMAN" || out[0].SlackWorkspaceName != "Postman" {
		t.Errorf("expected Slack workspace metadata, got %+v", out[0])
	}
	if out[0].SlackDisplayName != "Carol Chen" || out[0].SlackAvatarURL == "" {
		t.Errorf("expected Slack profile metadata, got %+v", out[0])
	}
	if out[0].CostUSD != 1.5 || out[0].Requests != 4 || out[0].Tokens != 400 {
		t.Errorf("metrics must not change for observed-only: %+v", out[0])
	}
}

// Unscoped bare Slack rows pass through unchanged when the unscoped directory
// lookup returns no entry. That includes unknown users and ambiguous users with
// multiple possible workspaces.
func TestMergeLinkedSlackRows_DirectoryMiss_PassesThrough(t *testing.T) {
	rows := []UserSummaryEntry{
		{UserIdentityRef: UserIdentityRef{UserID: "U07GHOSTLY"}, CostUSD: 2.0, Requests: 5},
	}

	out := mergeLinkedSlackRows(rows, nil)

	if len(out) != 1 || out[0].UserID != "U07GHOSTLY" {
		t.Errorf("expected pass-through, got %+v", out)
	}
	if out[0].SlackTeamID != "" {
		t.Errorf("no directory entry → no slack_team_id, got %q", out[0].SlackTeamID)
	}
}

// agents_used union must dedupe on DeploymentID — the same deployment
// appearing on both rows collapses, but two deployments of the same
// blueprint (identical Account/Name, distinct DeploymentID) stay as
// separate refs through the merge.
func TestMergeInto_AgentsUsedUnionDedupesOnDeploymentID(t *testing.T) {
	target := UserSummaryEntry{
		UserIdentityRef: UserIdentityRef{UserID: "user_alice"},
		AgentsUsed: []UserAgentRef{
			{DeploymentID: "dep-shared", Name: "shared", Account: "postman"},
			{DeploymentID: "dep-alice-only", Name: "alice-only", Account: "postman"},
		},
	}
	src := UserSummaryEntry{
		UserIdentityRef: UserIdentityRef{UserID: "U07ABCDEF"},
		AgentsUsed: []UserAgentRef{
			// Same deployment_id as target's "shared" — should NOT add.
			{DeploymentID: "dep-shared", Name: "shared", Account: "postman"},
			// Same Account+Name as target's "shared" but a different
			// deployment_id (second deployment of the same blueprint) —
			// should add, because dedup is per-deployment now.
			{DeploymentID: "dep-shared-2", Name: "shared", Account: "postman"},
			// Different Account+Name entirely — should add.
			{DeploymentID: "dep-bare-only", Name: "bare-only", Account: "postman"},
		},
	}

	mergeInto(&target, src)

	if len(target.AgentsUsed) != 4 {
		t.Fatalf("expected 4 refs (dedupe by deployment_id), got %d: %+v",
			len(target.AgentsUsed), target.AgentsUsed)
	}
	ids := map[string]bool{}
	for _, a := range target.AgentsUsed {
		ids[a.DeploymentID] = true
	}
	for _, want := range []string{"dep-shared", "dep-alice-only", "dep-shared-2", "dep-bare-only"} {
		if !ids[want] {
			t.Errorf("expected %s in merged refs, got %+v", want, target.AgentsUsed)
		}
	}
}
