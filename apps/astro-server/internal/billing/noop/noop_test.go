package noop

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func TestNoop_DiscardsUsage(t *testing.T) {
	p := New()
	ctx := context.Background()

	if id, err := p.CreateCustomer(ctx, billing.Account{ID: "a"}); err != nil || id != "" {
		t.Errorf("CreateCustomer: want (\"\", nil), got (%q, %v)", id, err)
	}
	if err := p.DeleteCustomer(ctx, "cust"); err != nil {
		t.Errorf("DeleteCustomer: unexpected error %v", err)
	}
	if err := p.IngestUsage(ctx, []billing.UsageEvent{{Type: "deployment_compute_usage"}}); err != nil {
		t.Errorf("IngestUsage: unexpected error %v", err)
	}
}
