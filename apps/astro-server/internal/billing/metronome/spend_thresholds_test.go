package metronome

import (
	"context"
	"testing"
)

const pathCustomerAlerts = "v1/customer-alerts/list"

// The org-wide backstop comes back in the same list as the customer's own
// thresholds, and the SDK's alert type carries no customer_id, so the name is
// the only thing separating them. Reading the backstop as a customer setting
// would show every account a limit it never chose.
func TestCustomerSpendThresholds_ReadsOnlyTheCustomersOwn(t *testing.T) {
	body := `{"data":[
		{"customer_status":"ok","alert":{"id":"a1","name":"Hard spend threshold","status":"enabled","threshold":10000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}},
		{"customer_status":"ok","alert":{"id":"a2","name":"Low Remaining Contract Credit Balance Reached","status":"enabled","threshold":0,"type":"low_remaining_contract_credit_balance_reached","updated_at":"2026-08-12T00:00:00Z"}},
		{"customer_status":"in_alarm","alert":{"id":"a3","name":"astro:spend_limit","status":"enabled","threshold":5000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}},
		{"customer_status":"ok","alert":{"id":"a4","name":"astro:spend_warning","status":"enabled","threshold":2500,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}
	],"next_page":null}`
	p, _ := newStub(t, map[string]string{pathCustomerAlerts: body})

	got, err := p.CustomerSpendThresholds(context.Background(), "cust_1")
	if err != nil {
		t.Fatalf("CustomerSpendThresholds: %v", err)
	}
	if !got.HasWarning || got.Warning.Amount != 2500 {
		t.Errorf("warning = %+v, want 2500", got.Warning)
	}
	if got.Warning.InAlarm {
		t.Error("warning reported crossed: the stub says ok")
	}
	if !got.HasLimit || got.Limit.Amount != 5000 {
		t.Errorf("limit = %+v, want 5000", got.Limit)
	}
	if !got.Limit.InAlarm {
		t.Error("limit reported not crossed: the stub says in_alarm, which is what tells the owner they are over")
	}
}

// A customer that set nothing must report absence, not zero. Zero is a
// threshold someone could choose, and rendering it as one would tell an
// unconfigured account it had capped itself at nothing.
func TestCustomerSpendThresholds_AbsentIsNotZero(t *testing.T) {
	body := `{"data":[
		{"customer_status":"ok","alert":{"id":"a1","name":"Hard spend threshold","status":"enabled","threshold":10000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}
	],"next_page":null}`
	p, _ := newStub(t, map[string]string{pathCustomerAlerts: body})

	got, err := p.CustomerSpendThresholds(context.Background(), "cust_1")
	if err != nil {
		t.Fatalf("CustomerSpendThresholds: %v", err)
	}
	if got.HasWarning || got.HasLimit {
		t.Errorf("reported thresholds for a customer that set none: %+v", got)
	}
}

// A customer can legitimately cap itself at zero, which must read as a real
// setting rather than as absence.
func TestCustomerSpendThresholds_ZeroIsARealSetting(t *testing.T) {
	body := `{"data":[
		{"customer_status":"in_alarm","alert":{"id":"a3","name":"astro:spend_limit","status":"enabled","threshold":0,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}
	],"next_page":null}`
	p, _ := newStub(t, map[string]string{pathCustomerAlerts: body})

	got, err := p.CustomerSpendThresholds(context.Background(), "cust_1")
	if err != nil {
		t.Fatalf("CustomerSpendThresholds: %v", err)
	}
	if !got.HasLimit {
		t.Fatal("a zero limit read as absent")
	}
	if got.Limit.Amount != 0 {
		t.Errorf("limit = %v, want 0", got.Limit.Amount)
	}
}
