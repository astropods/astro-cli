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
		name           string
		dunningSince   *time.Time
		alertActive    bool
		forceSuspended bool
		wantStatus     Status
		wantReason     string
	}{
		{"clean", nil, false, false, StatusActive, ""},
		{"dunning within grace", ptr(within), false, false, StatusPastDue, ReasonDunning},
		{"dunning past grace", ptr(expired), false, false, StatusSuspended, ReasonPaymentFailed},
		{"alert overrides clean", nil, true, false, StatusSuspended, ReasonBalanceAlert},
		{"alert overrides dunning", ptr(within), true, false, StatusSuspended, ReasonBalanceAlert},
		{"force suspend overrides clean", nil, false, true, StatusSuspended, ReasonUncollectible},
		{"force suspend overrides alert", ptr(within), true, true, StatusSuspended, ReasonUncollectible},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotReason := computeStatus(tc.dunningSince, tc.alertActive, tc.forceSuspended, grace, now)
			if gotStatus != tc.wantStatus || gotReason != tc.wantReason {
				t.Fatalf("computeStatus = (%s, %q), want (%s, %q)", gotStatus, gotReason, tc.wantStatus, tc.wantReason)
			}
		})
	}
}
