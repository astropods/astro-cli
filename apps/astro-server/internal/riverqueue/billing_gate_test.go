package riverqueue

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/notify"
)

// observeQueue has no River client on purpose: any path that reaches an insert
// panics, which is what proves the gate ran before it.
func observeQueue(enforce bool) *Queue {
	return &Queue{log: logger.New("error", "json"), billingEnforce: enforce}
}

func TestBillingActs(t *testing.T) {
	if observeQueue(false).billingActs("suspend", "acct_1") {
		t.Error("observe mode reported that billing may act")
	}
	if !observeQueue(true).billingActs("suspend", "acct_1") {
		t.Error("enforce mode reported that billing may not act")
	}
}

// The whole point of the flag is that suspension does not reach the queue in
// observe mode. A nil client means a regression that drops the gate panics
// here rather than silently scaling someone's deployments to zero.
func TestInsertBillingSuspend_ObserveModeNeverEnqueues(t *testing.T) {
	if err := observeQueue(false).InsertBillingSuspend(context.Background(), "acct_1"); err != nil {
		t.Fatalf("InsertBillingSuspend in observe mode: %v", err)
	}
}

func TestEmitBillingNotify_ObserveModeNeverEnqueues(t *testing.T) {
	ev := notify.BillingSuspended("acct_1", "")
	if err := observeQueue(false).EmitBillingNotify(context.Background(), ev); err != nil {
		t.Fatalf("EmitBillingNotify in observe mode: %v", err)
	}
}

// Resume is remediation, not enforcement. Gating it with suspend would mean
// turning the flag off after a real suspension could never undo it, so it must
// reach the client even in observe mode. Reaching a nil client panics today and
// could return an error after a River upgrade; either proves the call was
// attempted, and only a silent nil would mean the gate crept back in.
func TestInsertBillingResume_IsNotGated(t *testing.T) {
	reached := false
	func() {
		defer func() {
			if recover() != nil {
				reached = true
			}
		}()
		if err := observeQueue(false).InsertBillingResume(context.Background(), "acct_1"); err != nil {
			reached = true
		}
	}()
	if !reached {
		t.Error("resume never reached the client; a suspended account could not be restored")
	}
}
