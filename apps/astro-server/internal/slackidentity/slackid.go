package slackidentity

// IsBareSlackUserID returns true when s matches Slack's "U" + 8..11 uppercase
// alphanumerics shape — the format every historical Langfuse trace already
// carries for an unlinked Slack sender. Single source of truth shared by the
// observability handler (which uses it to decide which userIDs need a
// directory join) and the directory-backfill River worker (which uses it to
// filter Langfuse's distinct-userId list before upserting).
//
// Mirrors astro-client's SLACK_BARE_RE — keep all three in sync so the join
// fires for exactly the IDs the frontend will render as Slack rows.
func IsBareSlackUserID(s string) bool {
	if len(s) < 9 || len(s) > 12 || s[0] != 'U' {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}
