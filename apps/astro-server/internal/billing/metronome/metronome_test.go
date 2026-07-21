package metronome

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func TestNew_NilWithoutAPIKey(t *testing.T) {
	if p := New(Config{}); p != nil {
		t.Error("expected nil provider when API key is empty")
	}
}

func TestNew_ImplementsInterface(t *testing.T) {
	p := New(Config{APIKey: "test-key"})
	if p == nil {
		t.Fatal("expected non-nil provider")
	}
	var _ billing.BillingProvider = p
}
