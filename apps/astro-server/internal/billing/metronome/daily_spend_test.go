package metronome

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// The daily chart needs a real dollar figure per day, which the raw usage
// list can never give a quantity metric like Compute Units. DailySpend reads
// the invoice breakdown instead, so this pins the two things that make that
// safe: each day keys off the breakdown's own window, and a credit-drawdown
// line item is excluded rather than subtracted, matching usageSpend.
func TestDailySpend_SumsUsageLineItemsPerDayExcludingCredit(t *testing.T) {
	breakdowns := `{"data":[` +
		`{"breakdown_start_timestamp":"2026-08-11T00:00:00Z","breakdown_end_timestamp":"2026-08-12T00:00:00Z",` +
		`"id":"inv_1","status":"DRAFT","type":"USAGE","total":2500,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[` +
		`{"name":"Compute","type":"usage","total":3000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}},` +
		`{"name":"Credit","type":"applied_commit_or_credit","total":-500,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}` +
		`]},` +
		`{"breakdown_start_timestamp":"2026-08-12T00:00:00Z","breakdown_end_timestamp":"2026-08-13T00:00:00Z",` +
		`"id":"inv_1","status":"DRAFT","type":"USAGE","total":1000,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[` +
		`{"name":"Compute","type":"usage","total":1000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}` +
		`]}` +
		`],"next_page":null}`

	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(breakdowns))
	})

	result, err := p.DailySpend(context.Background(), "cust_1",
		time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	points, ok := result.([]DailySpendPoint)
	if !ok {
		t.Fatalf("result type = %T, want []DailySpendPoint", result)
	}
	if len(points) != 2 {
		t.Fatalf("got %d points, want 2", len(points))
	}

	wantDay1 := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	if !points[0].Day.Equal(wantDay1) {
		t.Errorf("points[0].Day = %v, want %v", points[0].Day, wantDay1)
	}
	// $30 of usage, unaffected by the -$5 credit line: usageSpend excludes
	// applied_commit_or_credit rather than netting it against usage.
	if points[0].Amount != 30 {
		t.Errorf("points[0].Amount = %v, want 30", points[0].Amount)
	}

	wantDay2 := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	if !points[1].Day.Equal(wantDay2) {
		t.Errorf("points[1].Day = %v, want %v", points[1].Day, wantDay2)
	}
	if points[1].Amount != 10 {
		t.Errorf("points[1].Amount = %v, want 10", points[1].Amount)
	}
}

// ByProduct is what lets a caller split Compute from Models without
// approximating it against a raw usage quantity, so it has to carry the
// same dollar figures usageSpend already sums into Amount.
func TestDailySpend_BreaksAmountDownByProduct(t *testing.T) {
	breakdowns := `{"data":[` +
		`{"breakdown_start_timestamp":"2026-08-11T00:00:00Z","breakdown_end_timestamp":"2026-08-12T00:00:00Z",` +
		`"id":"inv_1","status":"DRAFT","type":"USAGE","total":4000,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[` +
		`{"name":"Compute Units","type":"usage","total":3000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}},` +
		`{"name":"LLM Usage","type":"usage","total":1000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}` +
		`]}` +
		`],"next_page":null}`

	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(breakdowns))
	})

	result, err := p.DailySpend(context.Background(), "cust_1",
		time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	points := result.([]DailySpendPoint)
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1", len(points))
	}
	if points[0].Amount != 40 {
		t.Fatalf("Amount = %v, want 40", points[0].Amount)
	}
	if got := points[0].ByProduct["Compute Units"]; got != 30 {
		t.Errorf("ByProduct[Compute Units] = %v, want 30", got)
	}
	if got := points[0].ByProduct["LLM Usage"]; got != 10 {
		t.Errorf("ByProduct[LLM Usage] = %v, want 10", got)
	}
}

// A voided invoice has been replaced; without this its breakdown still
// carries a full day's line items and would double the day's total
// alongside its replacement.
func TestDailySpend_DropsVoidedInvoices(t *testing.T) {
	breakdowns := `{"data":[` +
		`{"breakdown_start_timestamp":"2026-08-11T00:00:00Z","breakdown_end_timestamp":"2026-08-12T00:00:00Z",` +
		`"id":"inv_voided","status":"VOID","type":"USAGE","total":3000,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[{"name":"Compute","type":"usage","total":3000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}]},` +
		`{"breakdown_start_timestamp":"2026-08-11T00:00:00Z","breakdown_end_timestamp":"2026-08-12T00:00:00Z",` +
		`"id":"inv_replacement","status":"DRAFT","type":"USAGE","total":2000,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[{"name":"Compute","type":"usage","total":2000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}]}` +
		`],"next_page":null}`

	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(breakdowns))
	})

	result, err := p.DailySpend(context.Background(), "cust_1",
		time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	points := result.([]DailySpendPoint)
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1: a voided invoice's breakdown should not add its own point", len(points))
	}
	if points[0].Amount != 20 {
		t.Errorf("Amount = %v, want 20 from the replacement invoice alone", points[0].Amount)
	}
}

// A correction can leave two non-void invoices both covering the same day.
// Summing both would double the day; the one issued last is the one that
// actually stands.
func TestDailySpend_TwoNonVoidInvoicesForTheSameDayKeepsTheLatestIssued(t *testing.T) {
	breakdowns := `{"data":[` +
		`{"breakdown_start_timestamp":"2026-08-11T00:00:00Z","breakdown_end_timestamp":"2026-08-12T00:00:00Z",` +
		`"id":"inv_original","status":"FINALIZED","issued_at":"2026-08-12T01:00:00Z","type":"USAGE","total":3000,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[{"name":"Compute","type":"usage","total":3000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}]},` +
		`{"breakdown_start_timestamp":"2026-08-11T00:00:00Z","breakdown_end_timestamp":"2026-08-12T00:00:00Z",` +
		`"id":"inv_correction","status":"FINALIZED","issued_at":"2026-08-20T01:00:00Z","type":"USAGE","total":2000,` +
		`"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"},` +
		`"line_items":[{"name":"Compute","type":"usage","total":2000,"credit_type":{"id":"2714e483-4ff1-48e4-9e25-ac732e8f24f2","name":"USD (cents)"}}]}` +
		`],"next_page":null}`

	p := spendProvider(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(breakdowns))
	})

	result, err := p.DailySpend(context.Background(), "cust_1",
		time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	points := result.([]DailySpendPoint)
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1: two invoices for one day should fold into one point", len(points))
	}
	if points[0].Amount != 20 {
		t.Errorf("Amount = %v, want 20 from the later-issued correction, not 30 from summing both", points[0].Amount)
	}
}
