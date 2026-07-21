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
		name         string
		dunningSince *time.Time
		alertActive  bool
		wantStatus   Status
		wantReason   string
	}{
		{"clean", nil, false, StatusActive, ""},
		{"dunning within grace", ptr(within), false, StatusPastDue, ReasonDunning},
		{"dunning past grace", ptr(expired), false, StatusSuspended, ReasonPaymentFailed},
		{"alert overrides clean", nil, true, StatusSuspended, ReasonBalanceAlert},
		{"alert overrides dunning", ptr(within), true, StatusSuspended, ReasonBalanceAlert},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotStatus, gotReason := computeStatus(tc.dunningSince, tc.alertActive, grace, now)
			if gotStatus != tc.wantStatus || gotReason != tc.wantReason {
				t.Fatalf("computeStatus = (%s, %q), want (%s, %q)", gotStatus, gotReason, tc.wantStatus, tc.wantReason)
			}
		})
	}
}
