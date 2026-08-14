package metronome

import (
	"context"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
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

// An alert that is neither of the customer's own gates the same latch, so whether
// it is over has to reach the caller. The only other coverage uses a fake reader,
// which never runs this parse: a renamed alert type would go unnoticed.
func TestCustomerSpendThresholds_ReportsAnOperatorAlertInAlarm(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{
			name: "an operator spend alert over its threshold",
			body: `{"data":[
				{"customer_status":"in_alarm","alert":{"id":"a1","name":"Hard spend threshold","status":"enabled","threshold":10000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}
			],"next_page":null}`,
			want: true,
		},
		{
			name: "the same alert under its threshold",
			body: `{"data":[
				{"customer_status":"ok","alert":{"id":"a1","name":"Hard spend threshold","status":"enabled","threshold":10000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}
			],"next_page":null}`,
			want: false,
		},
		{
			// A credit alert gates through its own latch, not this one, so an
			// exhausted account must not read as over its spend.
			name: "a credit alert in alarm is not a spend alert",
			body: `{"data":[
				{"customer_status":"in_alarm","alert":{"id":"a2","name":"Low Remaining Contract Credit Balance Reached","status":"enabled","threshold":0,"type":"low_remaining_contract_credit_balance_reached","updated_at":"2026-08-12T00:00:00Z"}}
			],"next_page":null}`,
			want: false,
		},
		{
			// The customer's own limit is reported by name. Counting it here too
			// would make it hold its own latch after it resolved.
			name: "the customer's own limit is not an operator alert",
			body: `{"data":[
				{"customer_status":"in_alarm","alert":{"id":"a3","name":"astro:spend_limit","status":"enabled","threshold":5000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}
			],"next_page":null}`,
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := newStub(t, map[string]string{pathCustomerAlerts: tc.body})
			got, err := p.CustomerSpendThresholds(context.Background(), "cust_1")
			if err != nil {
				t.Fatalf("CustomerSpendThresholds: %v", err)
			}
			if got.OperatorSpendInAlarm != tc.want {
				t.Errorf("OperatorSpendInAlarm = %v, want %v", got.OperatorSpendInAlarm, tc.want)
			}
		})
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

const (
	pathAlertCreate  = "v1/alerts/create"
	pathAlertArchive = "v1/alerts/archive"
	pathAlertReset   = "v1/customer-alerts/reset"
)

const noCustomerAlerts = `{"data":[],"next_page":null}`

// Metronome has no edit, so changing a threshold is archive then create. The
// archive has to release the uniqueness key, or the replacement collides with
// the alert it replaces and the customer's new number never takes effect.
func TestSetCustomerSpendThreshold_ChangeReleasesTheUniquenessKey(t *testing.T) {
	existing := `{"data":[{"customer_status":"ok","alert":{"id":"a_old","name":"astro:spend_limit","status":"enabled","threshold":5000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}],"next_page":null}`
	p, s := newStub(t, map[string]string{
		pathCustomerAlerts: existing,
		pathAlertArchive:   `{"data":{"id":"a_old"}}`,
		pathAlertCreate:    `{"data":{"id":"a_new"}}`,
		pathAlertReset:     `{}`,
	})

	if err := p.SetCustomerSpendThreshold(context.Background(), "cust_1", billing.SpendThresholdLimit, 9000); err != nil {
		t.Fatalf("SetCustomerSpendThreshold: %v", err)
	}

	archive := s.firstBody(pathAlertArchive)
	if archive["id"] != "a_old" {
		t.Errorf("archived %v, want a_old", archive["id"])
	}
	if archive["release_uniqueness_key"] != true {
		t.Errorf("release_uniqueness_key = %v, want true: the replacement collides without it", archive["release_uniqueness_key"])
	}
	create := s.firstBody(pathAlertCreate)
	if create["threshold"] != float64(9000) {
		t.Errorf("threshold = %v, want 9000", create["threshold"])
	}
	if create["uniqueness_key"] != "astro:spend_limit:cust_1" {
		t.Errorf("uniqueness_key = %v, want it scoped to the customer", create["uniqueness_key"])
	}
	if create["customer_id"] != "cust_1" {
		t.Errorf("customer_id = %v: an alert without one applies to every customer", create["customer_id"])
	}
}

// Evaluation is otherwise deferred, so an owner who raises a limit above current
// spend would stay suspended until Metronome next evaluated on its own.
func TestSetCustomerSpendThreshold_ResetsAfterWriting(t *testing.T) {
	p, s := newStub(t, map[string]string{
		pathCustomerAlerts: noCustomerAlerts,
		pathAlertCreate:    `{"data":{"id":"a_new"}}`,
		pathAlertReset:     `{}`,
	})

	if err := p.SetCustomerSpendThreshold(context.Background(), "cust_1", billing.SpendThresholdLimit, 9000); err != nil {
		t.Fatalf("SetCustomerSpendThreshold: %v", err)
	}
	reset := s.firstBody(pathAlertReset)
	if reset["alert_id"] != "a_new" || reset["customer_id"] != "cust_1" {
		t.Errorf("reset = %v, want the new alert for this customer", reset)
	}
}

// Every card add and every settings save re-sends the same numbers. Rewriting
// an unchanged threshold would archive a live alert and briefly leave the
// account uncapped.
func TestSetCustomerSpendThreshold_UnchangedIsANoop(t *testing.T) {
	existing := `{"data":[{"customer_status":"ok","alert":{"id":"a_old","name":"astro:spend_limit","status":"enabled","threshold":5000,"type":"spend_threshold_reached","updated_at":"2026-08-12T00:00:00Z"}}],"next_page":null}`
	p, s := newStub(t, map[string]string{pathCustomerAlerts: existing})

	if err := p.SetCustomerSpendThreshold(context.Background(), "cust_1", billing.SpendThresholdLimit, 5000); err != nil {
		t.Fatalf("SetCustomerSpendThreshold: %v", err)
	}
	if n := s.calls(pathAlertArchive) + s.calls(pathAlertCreate); n != 0 {
		t.Errorf("rewrote an unchanged threshold in %d calls", n)
	}
}

// Clearing a control the customer never set is what a settings form sends when
// the field was left empty, so it cannot be an error.
func TestClearCustomerSpendThreshold_AbsentIsNotAnError(t *testing.T) {
	p, s := newStub(t, map[string]string{pathCustomerAlerts: noCustomerAlerts})

	if err := p.ClearCustomerSpendThreshold(context.Background(), "cust_1", billing.SpendThresholdWarning); err != nil {
		t.Fatalf("ClearCustomerSpendThreshold: %v", err)
	}
	if n := s.calls(pathAlertArchive); n != 0 {
		t.Errorf("archived %d alerts for a customer that had none", n)
	}
}
