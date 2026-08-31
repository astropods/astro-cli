package payment

import (
	"testing"
	"time"
)

func TestCardExpired(t *testing.T) {
	tests := []struct {
		name string
		card *Card
		now  time.Time
		want bool
	}{
		{
			name: "last day of the expiry month is still valid",
			card: &Card{ExpMonth: 8, ExpYear: 2026},
			now:  time.Date(2026, 8, 31, 23, 59, 59, 0, time.UTC),
		},
		{
			name: "first instant of the next month is expired",
			card: &Card{ExpMonth: 8, ExpYear: 2026},
			now:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "december rolls into the next year",
			card: &Card{ExpMonth: 12, ExpYear: 2026},
			now:  time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "december is valid to its end",
			card: &Card{ExpMonth: 12, ExpYear: 2026},
			now:  time.Date(2026, 12, 31, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "no expiry is not expired",
			card: &Card{},
			now:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "no card at all counts as expired",
			card: nil,
			now:  time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
			want: true,
		},
		{
			name: "a zone behind UTC does not extend the card",
			card: &Card{ExpMonth: 8, ExpYear: 2026},
			now:  time.Date(2026, 8, 31, 20, 0, 0, 0, time.FixedZone("UTC-8", -8*60*60)),
			want: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.card.Expired(tc.now); got != tc.want {
				t.Errorf("Expired(%s) = %v, want %v", tc.now.Format(time.RFC3339), got, tc.want)
			}
		})
	}
}
