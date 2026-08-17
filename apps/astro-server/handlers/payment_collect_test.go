package handlers

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// A saved card does not pay what the account already owes, and dunning clears
// only on payment. Without a charge attempt the owner fixes their card and stays
// gated until the provider's own retry schedule comes around.
func TestCollectAfterCard(t *testing.T) {
	cases := []struct {
		name   string
		status billing.Status
		sig    billing.Signal
		want   bool
	}{
		{"suspended for a failed payment", billing.StatusSuspended, billing.SignalCardAdded, true},
		{"past due inside the grace window", billing.StatusPastDue, billing.SignalCardAdded, true},
		// No collection flag is raised, so there is nothing owed to chase. The
		// credits-exhausted account that just added a card lands here.
		{"active", billing.StatusActive, billing.SignalCardAdded, false},
		// Removing a card leaves no card to charge, and the account is on its way
		// to the free-tier floor rather than paying anything.
		{"card removed while suspended", billing.StatusSuspended, billing.SignalCardRemoved, false},
		{"card removed while active", billing.StatusActive, billing.SignalCardRemoved, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := collectAfterCard(tc.status, tc.sig); got != tc.want {
				t.Errorf("collectAfterCard(%q, %q) = %v, want %v", tc.status, tc.sig, got, tc.want)
			}
		})
	}
}
