package metronome

import (
	"context"
	"strconv"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/billing"
)

const pathBillableMetrics = "v1/billable-metrics"

const billableMetricsBody = `{"data":[
	{"id":"bm_gateway","name":"gateway","event_type_filter":{"in_values":["ai_gateway_llm_usage"]}},
	{"id":"bm_compute","name":"compute","event_type_filter":{"in_values":["deployment_compute_usage"]}}
],"next_page":null}`

func alertRow(id, name string, threshold int, status string) string {
	return `{"customer_status":"` + status + `","alert":{"id":"` + id + `","name":"` + name +
		`","status":"enabled","threshold":` + strconv.Itoa(threshold) + `,"type":"usage_threshold_reached","updated_at":"2026-08-24T00:00:00Z"}}`
}

func alertList(rows ...string) string {
	body := `{"data":[`
	for i, r := range rows {
		if i > 0 {
			body += ","
		}
		body += r
	}
	return body + `],"next_page":null}`
}

// The two metrics bill in different units, so folding them together suspends
// an account on the wrong number.
func TestCustomerUsageThresholds_SeparatesTheMetrics(t *testing.T) {
	body := alertList(
		alertRow("a1", billing.UsageAlertName(billing.UsageMetricGateway, billing.SpendThresholdWarning), 50, "ok"),
		alertRow("a2", billing.UsageAlertName(billing.UsageMetricGateway, billing.SpendThresholdLimit), 100, "in_alarm"),
		alertRow("a3", billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit), 700, "ok"),
		alertRow("a4", "astro:spend_limit", 9000, "ok"),
	)
	p, _ := newStub(t, map[string]string{pathCustomerAlerts: body})

	got, err := p.CustomerUsageThresholds(context.Background(), "cust_1")
	if err != nil {
		t.Fatalf("CustomerUsageThresholds: %v", err)
	}

	gw := got[billing.UsageMetricGateway]
	if !gw.HasWarning || gw.Warning.Amount != 50 {
		t.Errorf("gateway warning = %+v, want 50", gw.Warning)
	}
	if !gw.HasLimit || gw.Limit.Amount != 100 {
		t.Errorf("gateway limit = %+v, want 100", gw.Limit)
	}
	if !gw.Limit.InAlarm {
		t.Error("gateway limit not in alarm; the stub says in_alarm, which is what gates the account")
	}
	cu := got[billing.UsageMetricCompute]
	if !cu.HasLimit || cu.Limit.Amount != 700 {
		t.Errorf("compute limit = %+v, want 700", cu.Limit)
	}
	if cu.HasWarning {
		t.Errorf("compute reported a warning it never set: %+v", cu.Warning)
	}
}

// Reading a spend alert as a usage cap reports a dollar figure against
// whichever metric happens to match.
func TestCustomerUsageThresholds_IgnoresNonUsageAlerts(t *testing.T) {
	body := alertList(
		alertRow("a1", "astro:spend_warning", 2500, "ok"),
		alertRow("a2", "Low Remaining Contract Credit Balance Reached", 0, "ok"),
	)
	p, _ := newStub(t, map[string]string{pathCustomerAlerts: body})

	got, err := p.CustomerUsageThresholds(context.Background(), "cust_1")
	if err != nil {
		t.Fatalf("CustomerUsageThresholds: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("thresholds = %v, want none from spend and credit alerts", got)
	}
}

// A rewrite recreates the alert and clears its alarm, so an account over its
// cap would ungate every time anything re-applied the same setting.
func TestSetCustomerUsageThreshold_UnchangedAmountWritesNothing(t *testing.T) {
	name := billing.UsageAlertName(billing.UsageMetricGateway, billing.SpendThresholdLimit)
	p, s := newStub(t, map[string]string{
		pathBillableMetrics: billableMetricsBody,
		pathCustomerAlerts:  alertList(alertRow("al_1", name, 100, "in_alarm")),
	})

	if err := p.SetCustomerUsageThreshold(context.Background(), "cust_1",
		billing.UsageMetricGateway, billing.SpendThresholdLimit, 100); err != nil {
		t.Fatalf("SetCustomerUsageThreshold: %v", err)
	}

	if n := s.calls(pathAlertArchive); n != 0 {
		t.Errorf("archive calls = %d, want 0 for an unchanged threshold", n)
	}
	if n := s.calls(pathAlertCreate); n != 0 {
		t.Errorf("create calls = %d, want 0 for an unchanged threshold", n)
	}
}

// Metronome cannot update a threshold, so a change is archive then create.
// Without the archive the replaced cap stays live and keeps gating.
func TestSetCustomerUsageThreshold_ChangeArchivesThenCreatesAndResets(t *testing.T) {
	name := billing.UsageAlertName(billing.UsageMetricGateway, billing.SpendThresholdLimit)
	p, s := newStub(t, map[string]string{
		pathBillableMetrics: billableMetricsBody,
		pathCustomerAlerts:  alertList(alertRow("al_old", name, 100, "ok")),
		pathAlertArchive:    `{"data":{"id":"al_old"}}`,
		pathAlertCreate:     `{"data":{"id":"al_new"}}`,
		pathAlertReset:      `{}`,
	})

	if err := p.SetCustomerUsageThreshold(context.Background(), "cust_1",
		billing.UsageMetricGateway, billing.SpendThresholdLimit, 250); err != nil {
		t.Fatalf("SetCustomerUsageThreshold: %v", err)
	}

	if n := s.calls(pathAlertArchive); n != 1 {
		t.Fatalf("archive calls = %d, want 1", n)
	}
	if got := s.firstBody(pathAlertArchive)["id"]; got != "al_old" {
		t.Errorf("archived id = %v, want al_old", got)
	}

	created := s.firstBody(pathAlertCreate)
	if got := created["threshold"]; got != float64(250) {
		t.Errorf("created threshold = %v, want 250", got)
	}
	if got := created["billable_metric_id"]; got != "bm_gateway" {
		t.Errorf("billable_metric_id = %v, want bm_gateway; the cap must count the metric it names", got)
	}
	if got := created["name"]; got != name {
		t.Errorf("created name = %v, want %v", got, name)
	}
	if got := created["uniqueness_key"]; got != name+":cust_1" {
		t.Errorf("uniqueness_key = %v, want it scoped to the customer", got)
	}

	// A fresh alert starts unevaluated, so an account already over the new cap
	// stays ungated until its next usage event.
	if n := s.calls(pathAlertReset); n != 1 {
		t.Errorf("reset calls = %d, want 1", n)
	}
}

func TestSetCustomerUsageThreshold_FirstCapOnlyCreates(t *testing.T) {
	p, s := newStub(t, map[string]string{
		pathBillableMetrics: billableMetricsBody,
		pathCustomerAlerts:  alertList(),
		pathAlertCreate:     `{"data":{"id":"al_new"}}`,
		pathAlertReset:      `{}`,
	})

	if err := p.SetCustomerUsageThreshold(context.Background(), "cust_1",
		billing.UsageMetricCompute, billing.SpendThresholdWarning, 40); err != nil {
		t.Fatalf("SetCustomerUsageThreshold: %v", err)
	}

	if n := s.calls(pathAlertArchive); n != 0 {
		t.Errorf("archive calls = %d, want 0 when no cap existed", n)
	}
	if got := s.firstBody(pathAlertCreate)["billable_metric_id"]; got != "bm_compute" {
		t.Errorf("billable_metric_id = %v, want bm_compute", got)
	}
}

// A live alert keeps gating an account that just removed its own limit.
func TestClearCustomerUsageThreshold_ArchivesTheAlert(t *testing.T) {
	name := billing.UsageAlertName(billing.UsageMetricCompute, billing.SpendThresholdLimit)
	p, s := newStub(t, map[string]string{
		pathCustomerAlerts: alertList(alertRow("al_live", name, 700, "in_alarm")),
		pathAlertArchive:   `{"data":{"id":"al_live"}}`,
	})

	if err := p.ClearCustomerUsageThreshold(context.Background(), "cust_1",
		billing.UsageMetricCompute, billing.SpendThresholdLimit); err != nil {
		t.Fatalf("ClearCustomerUsageThreshold: %v", err)
	}

	if got := s.firstBody(pathAlertArchive)["id"]; got != "al_live" {
		t.Errorf("archived id = %v, want al_live", got)
	}
}

// The lookup must not fall through to whatever alert is first in the list.
func TestClearCustomerUsageThreshold_AbsentCapArchivesNothing(t *testing.T) {
	p, s := newStub(t, map[string]string{
		pathCustomerAlerts: alertList(alertRow("al_other", "astro:spend_limit", 9000, "ok")),
	})

	if err := p.ClearCustomerUsageThreshold(context.Background(), "cust_1",
		billing.UsageMetricCompute, billing.SpendThresholdLimit); err != nil {
		t.Fatalf("ClearCustomerUsageThreshold: %v", err)
	}

	if n := s.calls(pathAlertArchive); n != 0 {
		t.Errorf("archive calls = %d, want 0", n)
	}
}
