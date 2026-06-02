package slackidentity

import "testing"

// IsBareSlackUserID must match the same shape astro-client's SLACK_BARE_RE
// accepts (`U` + 8..11 uppercase alphanumerics). Drift here would either
// (a) trigger the Insights directory join for ids the frontend won't
// render as Slack rows, or (b) skip the join for ids the frontend WILL
// render as Slack rows — both manifest as missing deep links.
func TestIsBareSlackUserID(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"U07ABCDEF", true},          // 9 chars (lower bound)
		{"U01234567890", true},       // 12 chars (upper bound)
		{"U07A1B2C3", true},          // 9 chars, mixed alphanumeric
		{"U07ABC", false},            // 6 chars (too short)
		{"U07ABCD", false},           // 7 chars (too short)
		{"U0123456789012", false},    // 14 chars (too long)
		{"user_01HXX", false},        // WorkOS id
		{"u01abcdef", false},         // lowercase
		{"slack:T:U07ABCDEF", false}, // namespaced form (no longer emitted)
		{"", false},
	}
	for _, tc := range cases {
		if got := IsBareSlackUserID(tc.s); got != tc.want {
			t.Errorf("IsBareSlackUserID(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
