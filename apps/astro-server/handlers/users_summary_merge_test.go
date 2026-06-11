package handlers

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
)

// ── traceUserDetailsFromDirectory ──────────────────────────────────────────
//
// Trace endpoints (GetLangfuseTraces, GetLangfuseTraceDetail) aren't cached
// and resolve Slack identity inline — the per-request directory lookup is
// cheap and gives the trace a Slack-faced identity even for linked users
// (because the trace represents a Slack message, not Astro activity).

func TestTraceUserDetailsFromDirectory_SlackUserStampsProfile(t *testing.T) {
	entries := slackDirectoryEntries{
		"U07CAROL00": {
			TeamID: "T07POSTMAN",
			Profile: slackidentity.SlackProfile{
				DisplayName: "Carol Chen",
				Username:    "carol",
				AvatarURL:   "https://avatars.slack-edge.com/carol.png",
			},
		},
	}

	got := traceUserDetailsFromDirectory("U07CAROL00", entries)

	if got == nil || got.Kind != UserDetailsKindSlack {
		t.Fatalf("expected kind=slack, got %+v", got)
	}
	if got.TeamID != "T07POSTMAN" || got.DisplayName != "Carol Chen" || got.AvatarURL == "" {
		t.Errorf("expected Slack profile fields, got %+v", got)
	}
}

// A linked Slack user (the directory entry carries WorkOSUserID) STILL
// surfaces in trace output as a Slack identity — the trace itself
// originated from a Slack message, so showing the Slack avatar / name
// preserves the channel-of-origin signal.
func TestTraceUserDetailsFromDirectory_LinkedSlackStillShowsSlack(t *testing.T) {
	entries := slackDirectoryEntries{
		"U07BOBBOB1": {
			TeamID:       "T07POSTMAN",
			WorkOSUserID: "user_01HXX_bob",
			Profile: slackidentity.SlackProfile{
				DisplayName: "Bob Smith",
				Username:    "bob",
				AvatarURL:   "https://avatars.slack-edge.com/bob.png",
			},
		},
	}

	got := traceUserDetailsFromDirectory("U07BOBBOB1", entries)

	if got == nil || got.Kind != UserDetailsKindSlack {
		t.Fatalf("expected kind=slack for linked-Slack trace, got %+v", got)
	}
	if got.DisplayName != "Bob Smith" {
		t.Errorf("expected Slack display fields, got %+v", got)
	}
}

func TestTraceUserDetailsFromDirectory_UnknownSlackUserStaysBareSlack(t *testing.T) {
	got := traceUserDetailsFromDirectory("U07GHOSTLY", nil)

	if got == nil || got.Kind != UserDetailsKindSlack {
		t.Fatalf("expected kind=slack with empty fields, got %+v", got)
	}
	if got.DisplayName != "" || got.AvatarURL != "" || got.TeamID != "" {
		t.Errorf("directory miss should leave Slack fields empty, got %+v", got)
	}
}

func TestTraceUserDetailsFromDirectory_AstroUserClassifiesAstro(t *testing.T) {
	got := traceUserDetailsFromDirectory("user_01HXX_bob", nil)

	if got == nil || got.Kind != UserDetailsKindAstro {
		t.Fatalf("expected kind=astro, got %+v", got)
	}
}

func TestTraceUserDetailsFromDirectory_OpaqueIDClassifiesUnknown(t *testing.T) {
	got := traceUserDetailsFromDirectory("anon-session-7f3", nil)

	if got == nil || got.Kind != UserDetailsKindUnknown {
		t.Fatalf("expected kind=unknown, got %+v", got)
	}
}
