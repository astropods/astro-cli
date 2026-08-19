package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBillingTest(t *testing.T, handler http.Handler) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, accountTestCreds("testaccount"))

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	billingServerURLOverride = srv.URL
	t.Cleanup(func() { billingServerURLOverride = "" })
}

func spendPayload(data any) map[string]any {
	return map[string]any{"available": true, "data": data}
}

func TestBillingGet(t *testing.T) {
	full := map[string]any{
		"currency":           "USD",
		"plan":               "credit",
		"current_spend":      0.0,
		"has_current_spend":  true,
		"current_period_end": "2026-09-01T00:00:00Z",
		"usage_spend":        6.57,
		"has_usage_spend":    true,
		"credit_remaining":   3.43,
		"has_credit":         true,
		"warning":            map[string]any{"amount": 8.0, "in_alarm": false},
		"limit":              map[string]any{"amount": 20.0, "in_alarm": true},
	}

	cases := []struct {
		name       string
		body       any
		statusCode int
		jsonOutput bool
		wantErr    bool
		wantOut    string
	}{
		{name: "shows plan", body: spendPayload(full), statusCode: http.StatusOK, wantOut: "credit"},
		// Usage spend is the number the controls measure. An account on credit
		// reads zero billed spend while usage climbs, so hiding it would report
		// a burning account as idle.
		{name: "shows usage spend", body: spendPayload(full), statusCode: http.StatusOK, wantOut: "$6.57"},
		{name: "shows billed spend", body: spendPayload(full), statusCode: http.StatusOK, wantOut: "$0.00"},
		{name: "shows credit left", body: spendPayload(full), statusCode: http.StatusOK, wantOut: "$3.43"},
		{name: "shows an uncrossed warning", body: spendPayload(full), statusCode: http.StatusOK, wantOut: "$8.00 (ok)"},
		{name: "shows a crossed limit", body: spendPayload(full), statusCode: http.StatusOK, wantOut: "$20.00 (CROSSED)"},
		{name: "shows a control that is not set", body: spendPayload(map[string]any{
			"currency": "USD", "usage_spend": 1.0, "has_usage_spend": true,
		}), statusCode: http.StatusOK, wantOut: "not set"},
		{name: "billing not configured", body: map[string]any{"available": false},
			statusCode: http.StatusOK, wantOut: msgBillingUnavailable()},
		{name: "json output", body: spendPayload(full), statusCode: http.StatusOK,
			jsonOutput: true, wantOut: `"usage_spend": 6.57`},
		{name: "server error", body: map[string]any{"error": "boom"},
			statusCode: http.StatusInternalServerError, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBillingTest(t, jsonHandler(tc.statusCode, tc.body))
			if tc.jsonOutput {
				require.NoError(t, billingGetCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { _ = billingGetCmd.Flags().Set("json", "false") })
			}
			buf := &bytes.Buffer{}
			billingGetCmd.SetOut(buf)
			billingGetCmd.SetContext(context.Background())

			err := runBillingGet(billingGetCmd, nil)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func TestBillingStatus(t *testing.T) {
	gated := map[string]any{
		"status":              "suspended",
		"reason":              "credits_exhausted",
		"credits_exhausted":   true,
		"has_payment_method":  false,
		"enforced":            true,
		"workloads_suspended": true,
		"gated":               true,
		"action":              "add_card",
	}

	cases := []struct {
		name       string
		body       any
		jsonOutput bool
		wantOut    string
	}{
		{name: "shows the status", body: gated, wantOut: "suspended"},
		{name: "shows the reason", body: gated, wantOut: "credits_exhausted"},
		{name: "names the resolving action", body: gated, wantOut: "add_card"},
		{name: "reports stopped agents", body: gated, wantOut: "Agents stopped: yes"},
		{name: "active account", body: map[string]any{"status": "active", "gated": false}, wantOut: "active"},
		{name: "json output", body: gated, jsonOutput: true, wantOut: `"action": "add_card"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			setupBillingTest(t, jsonHandler(http.StatusOK, tc.body))
			if tc.jsonOutput {
				require.NoError(t, billingStatusCmd.Flags().Set("json", "true"))
				t.Cleanup(func() { _ = billingStatusCmd.Flags().Set("json", "false") })
			}
			buf := &bytes.Buffer{}
			billingStatusCmd.SetOut(buf)
			billingStatusCmd.SetContext(context.Background())

			require.NoError(t, runBillingStatus(billingStatusCmd, nil))
			assert.Contains(t, buf.String(), tc.wantOut)
		})
	}
}

func TestBillingInvoices(t *testing.T) {
	invoices := []any{
		map[string]any{
			"id": "inv-old", "status": "FINALIZED", "total": 1250.0,
			"credit_type":     map[string]any{"name": "USD (cents)"},
			"start_timestamp": "2026-07-01T00:00:00Z", "end_timestamp": "2026-08-01T00:00:00Z",
		},
		map[string]any{
			"id": "inv-new", "status": "DRAFT", "total": 657.0,
			"credit_type":     map[string]any{"name": "USD (cents)"},
			"start_timestamp": "2026-08-01T00:00:00Z", "end_timestamp": "2026-09-01T00:00:00Z",
			"line_items": []any{map[string]any{"name": "Compute Units", "total": 644.0}},
		},
	}

	t.Run("lists newest first", func(t *testing.T) {
		setupBillingTest(t, jsonHandler(http.StatusOK, spendPayload(invoices)))
		buf := &bytes.Buffer{}
		billingInvoicesCmd.SetOut(buf)
		billingInvoicesCmd.SetContext(context.Background())

		require.NoError(t, runBillingInvoices(billingInvoicesCmd, nil))
		out := buf.String()
		assert.Less(t, strings.Index(out, "inv-new"), strings.Index(out, "inv-old"))
	})

	// Invoices are a provider passthrough, so a cents credit type arrives
	// unconverted. Printing it raw would read a hundred times too large.
	t.Run("scales a cents credit type to dollars", func(t *testing.T) {
		setupBillingTest(t, jsonHandler(http.StatusOK, spendPayload(invoices)))
		buf := &bytes.Buffer{}
		billingInvoicesCmd.SetOut(buf)
		billingInvoicesCmd.SetContext(context.Background())

		require.NoError(t, runBillingInvoices(billingInvoicesCmd, nil))
		out := buf.String()
		assert.Contains(t, out, "$6.57")
		assert.Contains(t, out, "$12.50")
		assert.Contains(t, out, "$6.44")
		assert.NotContains(t, out, "$657.00")
	})

	t.Run("shows line items", func(t *testing.T) {
		setupBillingTest(t, jsonHandler(http.StatusOK, spendPayload(invoices)))
		buf := &bytes.Buffer{}
		billingInvoicesCmd.SetOut(buf)
		billingInvoicesCmd.SetContext(context.Background())

		require.NoError(t, runBillingInvoices(billingInvoicesCmd, nil))
		assert.Contains(t, buf.String(), "Compute Units")
	})

	t.Run("no invoices", func(t *testing.T) {
		setupBillingTest(t, jsonHandler(http.StatusOK, spendPayload([]any{})))
		buf := &bytes.Buffer{}
		billingInvoicesCmd.SetOut(buf)
		billingInvoicesCmd.SetContext(context.Background())

		require.NoError(t, runBillingInvoices(billingInvoicesCmd, nil))
		assert.Contains(t, buf.String(), msgNoInvoices())
	})

	t.Run("rejects a non-positive limit", func(t *testing.T) {
		setupBillingTest(t, jsonHandler(http.StatusOK, spendPayload(invoices)))
		require.NoError(t, billingInvoicesCmd.Flags().Set("limit", "0"))
		t.Cleanup(func() { _ = billingInvoicesCmd.Flags().Set("limit", "12") })

		err := runBillingInvoices(billingInvoicesCmd, nil)
		require.Error(t, err)
		assert.Equal(t, errPositiveIntFlag("limit").Error(), err.Error())
	})
}

// The threshold endpoint is a PUT that replaces both controls, and a null
// clears one. These cases pin the read-modify-write that keeps a control the
// caller did not name, which a straight passthrough would silently remove.
func TestBillingSetPreservesTheUnnamedControl(t *testing.T) {
	current := map[string]any{
		"currency": "USD",
		"warning":  map[string]any{"amount": 8.0, "in_alarm": false},
		"limit":    map[string]any{"amount": 20.0, "in_alarm": false},
	}

	cases := []struct {
		name        string
		flags       map[string]string
		wantWarning *float64
		wantLimit   *float64
	}{
		{
			name:        "setting only the limit keeps the warning",
			flags:       map[string]string{"limit": "50"},
			wantWarning: ptrFloat(8),
			wantLimit:   ptrFloat(50),
		},
		{
			name:        "setting only the warning keeps the limit",
			flags:       map[string]string{"warning": "5"},
			wantWarning: ptrFloat(5),
			wantLimit:   ptrFloat(20),
		},
		{
			name:        "clearing the limit keeps the warning",
			flags:       map[string]string{"clear-limit": "true"},
			wantWarning: ptrFloat(8),
			wantLimit:   nil,
		},
		{
			name:        "setting both replaces both",
			flags:       map[string]string{"warning": "1", "limit": "2"},
			wantWarning: ptrFloat(1),
			wantLimit:   ptrFloat(2),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sent struct {
				Warning *float64 `json:"warning"`
				Limit   *float64 `json:"limit"`
			}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					require.NoError(t, json.NewDecoder(r.Body).Decode(&sent))
					jsonHandler(http.StatusOK, map[string]any{"available": true})(w, r)
					return
				}
				jsonHandler(http.StatusOK, spendPayload(current))(w, r)
			})
			setupBillingTest(t, handler)

			for name, value := range tc.flags {
				require.NoError(t, billingSetCmd.Flags().Set(name, value))
			}
			t.Cleanup(func() { resetBillingSetFlags(t) })

			buf := &bytes.Buffer{}
			billingSetCmd.SetOut(buf)
			billingSetCmd.SetContext(context.Background())

			require.NoError(t, runBillingSet(billingSetCmd, nil))
			assert.Equal(t, tc.wantWarning, sent.Warning)
			assert.Equal(t, tc.wantLimit, sent.Limit)
			assert.Contains(t, buf.String(), msgSpendControlsSaved())
		})
	}
}

// An empty invocation would send two nulls, which removes the account's spend
// limit. Refusing is the difference between a no-op and an uncapped account.
func TestBillingSetRefusesAnEmptyInvocation(t *testing.T) {
	calls := 0
	setupBillingTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		jsonHandler(http.StatusOK, map[string]any{"available": true})(w, r)
	}))
	billingSetCmd.SetOut(&bytes.Buffer{})
	billingSetCmd.SetContext(context.Background())

	err := runBillingSet(billingSetCmd, nil)
	require.Error(t, err)
	assert.Equal(t, errBillingSetNoChange().Error(), err.Error())
	assert.Zero(t, calls, "the command must not reach the API")
}

func TestBillingSetRejectsConflictingFlags(t *testing.T) {
	for _, name := range []string{"warning", "limit"} {
		t.Run(name, func(t *testing.T) {
			setupBillingTest(t, jsonHandler(http.StatusOK, map[string]any{"available": true}))
			require.NoError(t, billingSetCmd.Flags().Set(name, "5"))
			require.NoError(t, billingSetCmd.Flags().Set("clear-"+name, "true"))
			t.Cleanup(func() { resetBillingSetFlags(t) })

			billingSetCmd.SetOut(&bytes.Buffer{})
			billingSetCmd.SetContext(context.Background())

			err := runBillingSet(billingSetCmd, nil)
			require.Error(t, err)
			assert.Equal(t, errBillingSetConflict(name).Error(), err.Error())
		})
	}
}

func TestBillingSetReportsUnconfiguredBilling(t *testing.T) {
	setupBillingTest(t, jsonHandler(http.StatusOK, map[string]any{"available": false}))
	require.NoError(t, billingSetCmd.Flags().Set("limit", "10"))
	t.Cleanup(func() { resetBillingSetFlags(t) })

	billingSetCmd.SetOut(&bytes.Buffer{})
	billingSetCmd.SetContext(context.Background())

	err := runBillingSet(billingSetCmd, nil)
	require.Error(t, err)
	assert.Equal(t, errBillingUnavailable().Error(), err.Error())
}

func TestFormatProviderAmount(t *testing.T) {
	cases := []struct {
		value      float64
		creditType string
		want       string
	}{
		{657, "USD (cents)", "$6.57"},
		{6.57, "USD", "$6.57"},
		{6.57, "", "$6.57"},
		{3, "tokens", "3.00 tokens"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, formatProviderAmount(tc.value, tc.creditType))
	}
}

func TestFormatMoney(t *testing.T) {
	cases := []struct {
		amount   float64
		currency string
		want     string
	}{
		{6.567, "USD", "$6.57"},
		{0, "", "$0.00"},
		{12.5, "EUR", "$12.50 EUR"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, formatMoney(tc.amount, tc.currency))
	}
}

// resetBillingSetFlags clears both the value and the Changed bit. Cobra flags
// are package-level, and runBillingSet branches on Changed, so a value reset
// alone would leak "the caller named this flag" into the next test.
func resetBillingSetFlags(t *testing.T) {
	t.Helper()
	for name, value := range map[string]string{
		"warning": "0", "limit": "0", "clear-warning": "false", "clear-limit": "false",
	} {
		require.NoError(t, billingSetCmd.Flags().Set(name, value))
		billingSetCmd.Flags().Lookup(name).Changed = false
	}
}

func ptrFloat(v float64) *float64 { return &v }
