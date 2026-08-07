package handlers

import (
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
	"github.com/astropods/astro/apps/astro-server/internal/billing/noop"
)

// Signup enqueues billing.provision, but the worker is only registered for a
// provisioning backend. The noop provider is non-nil, so a nil check would
// insert jobs no worker can run on every OSS signup. Only the Provisioner
// assertion separates them.
func TestNoopProviderIsNotAProvisioner(t *testing.T) {
	var p billing.BillingProvider = noop.New()
	if p == nil {
		t.Fatal("noop provider is nil; the signup gate would already be closed by a nil check")
	}
	if _, ok := p.(billing.Provisioner); ok {
		t.Error("noop implements Provisioner, so signup would enqueue jobs with no registered worker")
	}
}
