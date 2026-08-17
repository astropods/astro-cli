package notify

import "testing"

// The provider stores minor units. Sending the integer would render cents as
// dollars, so a $25 threshold would read as $2500 in the message.
func TestMoneyFormatsMinorUnits(t *testing.T) {
	cases := []struct {
		cents int64
		want  string
	}{
		{2500, "$25.00"},
		{2599, "$25.99"},
		{5, "$0.05"},
		{100000, "$1000.00"},
		// A customer can set a threshold of zero, and the provider reports it as a
		// real limit. Blanking it would leave the message naming no number.
		{0, "$0.00"},
	}
	for _, tc := range cases {
		if got := money(tc.cents); got != tc.want {
			t.Errorf("money(%d) = %q, want %q", tc.cents, got, tc.want)
		}
	}
}

// The warning and the limit are separate types because the limit's message says
// the account stopped. One type for both would tell an owner still under their
// limit that their agents are down.
func TestSpendWarningAndLimitAreDistinctTypes(t *testing.T) {
	warning := BillingSpendWarning("acct_1", "acme", 8000, 8100)
	limit := BillingSpendThreshold("acct_1", "acme", 10000, 10100)

	if warning.Type == limit.Type {
		t.Fatalf("both events use %q", warning.Type)
	}
	if warning.Type != TypeBillingSpendWarning {
		t.Errorf("warning type = %q", warning.Type)
	}
	for _, ev := range []Event{warning, limit} {
		if ev.Audience != AudienceManagers {
			t.Errorf("%s audience = %q, want managers", ev.Type, ev.Audience)
		}
		if ev.Payload[PayloadCTAURL] != "/settings/billing" {
			t.Errorf("%s ctaUrl = %v", ev.Type, ev.Payload[PayloadCTAURL])
		}
	}
	if warning.Payload[PayloadThreshold] != "$80.00" || warning.Payload[PayloadSpent] != "$81.00" {
		t.Errorf("warning amounts = %v / %v", warning.Payload[PayloadThreshold], warning.Payload[PayloadSpent])
	}
}

// The payload contract is what a template author builds against, so a key the
// builder sends but the contract omits is a variable nobody knows exists.
func TestSpendPayloadContractMatchesTheBuilders(t *testing.T) {
	built := map[Type]Event{
		TypeBillingSpendWarning:   BillingSpendWarning("acct_1", "acme", 8000, 8100),
		TypeBillingSpendThreshold: BillingSpendThreshold("acct_1", "acme", 10000, 10100),
	}
	for typ, ev := range built {
		declared := map[string]bool{}
		for _, key := range PayloadProperties(typ) {
			declared[key] = true
		}
		for key := range ev.Payload {
			if !declared[key] {
				t.Errorf("%s sends %q, which the payload contract omits", typ, key)
			}
		}
		for key := range declared {
			if key == PayloadTimestamp {
				continue // stamped at delivery, not by the builder
			}
			if _, ok := ev.Payload[key]; !ok {
				t.Errorf("%s declares %q but the builder never sets it", typ, key)
			}
		}
	}
}
