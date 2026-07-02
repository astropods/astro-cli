package handlers

import (
	"net/url"
	"testing"

	slackclient "github.com/astropods/astro/apps/astro-server/internal/slack"
)

func TestObservedUsersForSlackDirectory(t *testing.T) {
	got := observedUsersForSlackDirectory("T07XYZ", []slackclient.UserInfo{
		{
			ID:          "U07ABC",
			Name:        "jesse",
			DisplayName: "Jesse Morgan",
			RealName:    "Jessica Legal",
			AvatarURL:   "https://avatars.slack-edge.com/jesse.png",
		},
		{
			ID:        "U08BOT",
			RealName:  "Deploy Bot",
			AvatarURL: "https://avatars.slack-edge.com/bot.png",
			IsBot:     true,
			Deleted:   true,
		},
		{
			Name: "missing-id",
		},
	})

	if len(got) != 2 {
		t.Fatalf("expected 2 observed users, got %d", len(got))
	}
	if got[0].TeamID != "T07XYZ" || got[0].SlackUserID != "U07ABC" {
		t.Fatalf("first observed identity mismatch: %+v", got[0])
	}
	if got[0].Profile.DisplayName != "Jessica Legal" || got[0].Profile.Username != "jesse" {
		t.Errorf("first profile mismatch: %+v", got[0].Profile)
	}
	if got[0].Profile.AvatarURL != "https://avatars.slack-edge.com/jesse.png" {
		t.Errorf("first avatar mismatch: %+v", got[0].Profile)
	}
	if got[1].Profile.DisplayName != "Deploy Bot" || got[1].Profile.Username != "Deploy Bot" || !got[1].Profile.IsBot || !got[1].Profile.Deleted {
		t.Errorf("second profile mismatch: %+v", got[1].Profile)
	}
}

func TestSlackFrontendRedirectMergesQueryParams(t *testing.T) {
	params := url.Values{}
	params.Set("slack_connected", "true")
	params.Set("slack_team", "Postman")

	got := slackFrontendRedirect("https://app.astro.dev", "/settings/account?tab=slack", params)
	want := "https://app.astro.dev/settings/account?slack_connected=true&slack_team=Postman&tab=slack"
	if got != want {
		t.Fatalf("redirect = %q, want %q", got, want)
	}
}

func TestSafeSlackRedirectPathRejectsUnsafeTargets(t *testing.T) {
	for _, input := range []string{
		"",
		"settings/account",
		"//evil.example/settings",
		"https://evil.example/settings",
	} {
		if got := safeSlackRedirectPath(input); got != "/settings/account" {
			t.Fatalf("safeSlackRedirectPath(%q) = %q, want fallback", input, got)
		}
	}

	got := safeSlackRedirectPath("/settings/account?tab=slack#ignored")
	if got != "/settings/account?tab=slack" {
		t.Fatalf("safe path = %q, want query preserved and fragment stripped", got)
	}
}
