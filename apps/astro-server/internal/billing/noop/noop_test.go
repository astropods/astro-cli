package noop

import (
	"context"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

func TestNoop_AllowsAndDiscards(t *testing.T) {
	p := New()
	ctx := context.Background()

	if id, err := p.CreateCustomer(ctx, billing.Account{ID: "a"}); err != nil || id != "" {
		t.Errorf("CreateCustomer: want (\"\", nil), got (%q, %v)", id, err)
	}
	if err := p.IngestUsage(ctx, []billing.UsageEvent{{Type: "compute_usage"}}); err != nil {
		t.Errorf("IngestUsage: unexpected error %v", err)
	}
	bal, err := p.CheckBalance(ctx, "cust")
	if err != nil || !bal.Allow {
		t.Errorf("CheckBalance: want allow, got %+v err=%v", bal, err)
	}
	if _, err := p.GetUsage(ctx, "cust", time.Time{}, time.Now()); err != nil {
		t.Errorf("GetUsage: unexpected error %v", err)
	}
}

// TestNoop_NotHostedBilling asserts the no-op provider is not a HostedBilling,
// so the `, ok` type-assertion in callers cleanly skips hosted-only operations.
func TestNoop_NotHostedBilling(t *testing.T) {
	var p billing.BillingProvider = New()
	if _, ok := p.(billing.HostedBilling); ok {
		t.Error("noop provider must not implement HostedBilling")
	}
}
