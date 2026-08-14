package handlers

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

// recordingWriter records the order controls are written in and can fail one.
type recordingWriter struct {
	order  []billing.SpendThresholdKind
	failOn billing.SpendThresholdKind
}

var errWriteFailed = errors.New("provider unavailable")

func (r *recordingWriter) write(kind billing.SpendThresholdKind) error {
	r.order = append(r.order, kind)
	if kind == r.failOn {
		return errWriteFailed
	}
	return nil
}

func (r *recordingWriter) SetCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind, _ float64) error {
	return r.write(kind)
}

func (r *recordingWriter) ClearCustomerSpendThreshold(_ context.Context, _ string, kind billing.SpendThresholdKind) error {
	return r.write(kind)
}

func amount(f float64) *float64 { return &f }

// Changing a threshold archives the old alert before creating its replacement,
// so a failure can leave that control unset. The limit is written first, which
// means a failure on the warning leaves the cap intact. A map's random order
// would make which one survives a coin flip.
func TestWriteSpendThresholds_WritesTheLimitFirst(t *testing.T) {
	w := &recordingWriter{}
	applied, failed, err := writeSpendThresholds(context.Background(), w, "cust_1", amount(2500), amount(5000))
	if err != nil {
		t.Fatalf("writeSpendThresholds: %v", err)
	}
	if failed != "" {
		t.Errorf("failed = %q, want empty on success", failed)
	}
	if len(w.order) != 2 || w.order[0] != billing.SpendThresholdLimit {
		t.Fatalf("write order = %v, want the limit first", w.order)
	}
	if !slices.Contains(applied, string(billing.SpendThresholdLimit)) ||
		!slices.Contains(applied, string(billing.SpendThresholdWarning)) {
		t.Errorf("applied = %v, want both", applied)
	}
}

// A partial save has to name what it left behind. Reporting a bare failure over
// an account whose limit is now unset reads as "nothing happened".
func TestWriteSpendThresholds_ReportsWhatSurvivedAFailure(t *testing.T) {
	w := &recordingWriter{failOn: billing.SpendThresholdWarning}
	applied, failed, err := writeSpendThresholds(context.Background(), w, "cust_1", amount(2500), amount(5000))
	if !errors.Is(err, errWriteFailed) {
		t.Fatalf("err = %v, want the write failure", err)
	}
	if failed != billing.SpendThresholdWarning {
		t.Errorf("failed = %q, want the warning", failed)
	}
	// The protective control was written before the failure, so it stands.
	if !slices.Contains(applied, string(billing.SpendThresholdLimit)) {
		t.Errorf("applied = %v, want the limit: it was written before the failure", applied)
	}
	if slices.Contains(applied, string(billing.SpendThresholdWarning)) {
		t.Errorf("applied = %v, reports a control that failed", applied)
	}
}

// A failure on the limit must not claim the warning was applied, because the
// warning never ran.
func TestWriteSpendThresholds_LimitFailureStopsBeforeTheWarning(t *testing.T) {
	w := &recordingWriter{failOn: billing.SpendThresholdLimit}
	applied, failed, err := writeSpendThresholds(context.Background(), w, "cust_1", amount(2500), amount(5000))
	if err == nil {
		t.Fatal("want the write failure")
	}
	if failed != billing.SpendThresholdLimit {
		t.Errorf("failed = %q, want the limit", failed)
	}
	if len(applied) != 0 {
		t.Errorf("applied = %v, want none", applied)
	}
	if len(w.order) != 1 {
		t.Errorf("wrote %v after the limit failed", w.order)
	}
}
