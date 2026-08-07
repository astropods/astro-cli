package riverqueue

import (
	"context"
	"errors"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/payment"
)

type fakeCards struct {
	card *payment.Card
	err  error
}

func (f fakeCards) DefaultCard(context.Context, string) (*payment.Card, error) {
	return f.card, f.err
}

// Replacing a card detaches the old one, and the detached event must not clear
// has_payment_method while a card is still on file — that would suspend a
// paying account.
func TestResolveCardSignal(t *testing.T) {
	cases := []struct {
		name  string
		cards fakeCards
		want  billing.Signal
	}{
		{"card still on file after a detach", fakeCards{card: &payment.Card{Last4: "4242"}}, billing.SignalCardAdded},
		{"no cards left", fakeCards{}, billing.SignalCardRemoved},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveCardSignal(context.Background(), tc.cards, "cus_1")
			if err != nil {
				t.Fatalf("resolveCardSignal: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A Stripe read failure must surface so River retries. Defaulting to "removed"
// on a transient error would suspend a paying account.
func TestResolveCardSignal_ErrorsRatherThanAssumingRemoval(t *testing.T) {
	_, err := resolveCardSignal(context.Background(), fakeCards{err: errors.New("stripe down")}, "cus_1")
	if err == nil {
		t.Fatal("expected an error so the job retries, got nil")
	}
}
