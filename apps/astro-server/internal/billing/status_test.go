package billing

import (
	"testing"
	"time"
)

func TestComputeStatus(t *testing.T) {
	grace := 7 * 24 * time.Hour
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	within := now.Add(-2 * 24 * time.Hour)  // 2 days ago, inside grace
	expired := now.Add(-8 * 24 * time.Hour) // 8 days ago, past grace

	ptr := func(t time.Time) *time.Time { return &t }

	cases := []struct {
		name       string
		sig        signals
		wantStatus Status
		wantReason string
	}{
		{"clean", signals{}, StatusActive, ""},
		{"dunning within grace", signals{dunningSince: ptr(within)}, StatusPastDue, ReasonDunning},
		{"dunning past grace", signals{dunningSince: ptr(expired)}, StatusSuspended, ReasonPaymentFailed},
		// The boundary decides a whole day of service either way. The comparison
		// is strictly greater, so the grace is inclusive of its final instant.
		{"dunning exactly at grace", signals{dunningSince: ptr(now.Add(-grace))}, StatusPastDue, ReasonDunning},
		{"dunning one second past grace", signals{dunningSince: ptr(now.Add(-grace - time.Second))}, StatusSuspended, ReasonPaymentFailed},
		{"dunning one second inside grace", signals{dunningSince: ptr(now.Add(-grace + time.Second))}, StatusPastDue, ReasonDunning},
		{"alert overrides clean", signals{alertActive: true}, StatusSuspended, ReasonBalanceAlert},
		{"alert overrides dunning", signals{dunningSince: ptr(within), alertActive: true}, StatusSuspended, ReasonBalanceAlert},
		{"force suspend overrides clean", signals{forceSuspended: true}, StatusSuspended, ReasonUncollectible},
		{"force suspend overrides alert", signals{dunningSince: ptr(within), alertActive: true, forceSuspended: true}, StatusSuspended, ReasonUncollectible},

		// Credit exhaustion gates the free tier only. A card is what turns the
		// account into pay-as-you-go, so the same latch stops mattering.
		{"credits exhausted, no card", signals{creditsExhausted: true}, StatusSuspended, ReasonCreditsExhausted},
		{"credits exhausted, card on file", signals{creditsExhausted: true, hasPaymentMethod: true}, StatusActive, ""},
		{"card alone changes nothing", signals{hasPaymentMethod: true}, StatusActive, ""},
		// A paying customer still gates on payment collection, just not on balance.
		{"card on file, payment failed past grace", signals{creditsExhausted: true, hasPaymentMethod: true, dunningSince: ptr(expired)}, StatusSuspended, ReasonPaymentFailed},
		{"card on file, dunning within grace", signals{creditsExhausted: true, hasPaymentMethod: true, dunningSince: ptr(within)}, StatusPastDue, ReasonDunning},
		// Exhaustion outranks dunning: "add a card" is the actionable message.
		{"exhausted outranks dunning", signals{creditsExhausted: true, dunningSince: ptr(within)}, StatusSuspended, ReasonCreditsExhausted},
		{"hard alert outranks exhaustion", signals{creditsExhausted: true, hasPaymentMethod: true, alertActive: true}, StatusSuspended, ReasonBalanceAlert},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotReason := computeStatus(tc.sig, grace, now)
			if gotStatus != tc.wantStatus || gotReason != tc.wantReason {
				t.Fatalf("computeStatus = (%s, %q), want (%s, %q)", gotStatus, gotReason, tc.wantStatus, tc.wantReason)
			}
		})
	}
}

// anyFlagSet drives whether Recompute may skip writing a row at all, so a new
// flag that gates must be counted and has_payment_method must not be.
func TestSignalsAnyFlagSet(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name string
		sig  signals
		want bool
	}{
		{"empty", signals{}, false},
		{"card only", signals{hasPaymentMethod: true}, false},
		{"dunning", signals{dunningSince: &now}, true},
		{"alert", signals{alertActive: true}, true},
		{"force", signals{forceSuspended: true}, true},
		{"exhausted", signals{creditsExhausted: true}, true},
		{"exhausted with card", signals{creditsExhausted: true, hasPaymentMethod: true}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.sig.anyFlagSet(); got != tc.want {
				t.Fatalf("anyFlagSet() = %v, want %v", got, tc.want)
			}
		})
	}
}
