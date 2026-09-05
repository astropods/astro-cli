package cmd

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func suspendedBody(reason, action, details string) []byte {
	return fmt.Appendf(nil,
		`{"error":"Billing suspended","code":"BILLING_SUSPENDED","reason":%q,"action":%q,"details":%q}`,
		reason, action, details)
}

// The refusal has to read as an instruction. Dumping the JSON leaves the reader
// to find the one sentence that matters inside a payload written for a program.
func TestAPIError_BillingRefusalNamesTheFix(t *testing.T) {
	cases := []struct {
		name, action, details string
		wantSubstrings        []string
	}{
		{
			name:    "add a card",
			action:  billingActionAddCard,
			details: "This account's free credits are used up. Add a payment method to continue.",
			wantSubstrings: []string{
				"suspended",
				"free credits are used up",
				"astropod.ai/settings/billing",
			},
		},
		{
			name:    "update the card",
			action:  billingActionUpdateCard,
			details: "A payment for this account could not be collected. Update the payment method to continue.",
			wantSubstrings: []string{
				"could not be collected",
				"astropod.ai/settings/billing",
			},
		},
		{
			name:           "contact support",
			action:         billingActionContactSupport,
			details:        "This account is suspended for a billing issue only support can resolve.",
			wantSubstrings: []string{"Contact support"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := newAPIError(http.StatusPaymentRequired, suspendedBody("payment_failed", tc.action, tc.details)).Error()
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(got, want) {
					t.Errorf("message does not contain %q:\n%s", want, got)
				}
			}
			if strings.Contains(got, "BILLING_SUSPENDED") {
				t.Errorf("message leaks the raw payload:\n%s", got)
			}
		})
	}
}

// An action the CLI does not recognise must still print the server's sentence.
// Guessing a next step would tell an account with a card to add one.
func TestAPIError_AnUnknownActionStillExplains(t *testing.T) {
	got := newAPIError(http.StatusPaymentRequired,
		suspendedBody("something_new", "do_a_barrel_roll", "Your account is on hold.")).Error()

	if !strings.Contains(got, "Your account is on hold.") {
		t.Errorf("the server's explanation is missing:\n%s", got)
	}
	if strings.Contains(got, "astropod.ai") || strings.Contains(got, "Contact support") {
		t.Errorf("an unknown action produced a made-up next step:\n%s", got)
	}
}

// The registry refuses a push with 403 and the server refuses a deploy with 402.
// The code identifies the refusal, so keying on the status would miss one.
func TestAPIError_TheCodeIdentifiesTheRefusal(t *testing.T) {
	for _, status := range []int{http.StatusPaymentRequired, http.StatusForbidden} {
		e := newAPIError(status, suspendedBody("credits_exhausted", billingActionAddCard, "Add a payment method."))
		if !e.isBillingSuspended() {
			t.Errorf("status %d was not recognised as a billing refusal", status)
		}
	}
}

func TestAPIError_StructuredFailureUsesTheServerExplanation(t *testing.T) {
	body := []byte(`{"error":"authorization denied","code":"AUTHORIZATION_DENIED","action":"resource:future_action","details":"The server selected this explanation."}`)
	e := newAPIError(http.StatusForbidden, body)

	require.True(t, e.isStructured())
	require.Equal(t, "The server selected this explanation.", e.Error())
}

// Every other error keeps the shape the CLI has always printed. Scripts match on
// it, and a billing change is no reason to move them.
func TestAPIError_OtherFailuresKeepTheOldText(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"json with no code", `{"error":"agent not found"}`},
		// A proxy error page or a gateway timeout is not JSON at all.
		{"html", "<html><body>502 Bad Gateway</body></html>"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newAPIError(http.StatusInternalServerError, []byte(tc.body))
			if e.isBillingSuspended() {
				t.Fatal("a non-billing error was treated as a billing refusal")
			}
			want := fmt.Sprintf("server returned status 500: %s", strings.TrimSpace(tc.body))
			if e.Error() != want {
				t.Errorf("Error() = %q, want %q", e.Error(), want)
			}
		})
	}
}

// A server that predates the billing fields sends a plain 402. It must not be
// mistaken for a suspension, and it must not lose its body.
func TestAPIError_AnOlderServerIsNotMisread(t *testing.T) {
	e := newAPIError(http.StatusPaymentRequired, []byte(`{"error":"payment required"}`))
	if e.isBillingSuspended() {
		t.Error("a 402 with no code was read as a suspension")
	}
	if !strings.Contains(e.Error(), "payment required") {
		t.Errorf("the body was lost: %q", e.Error())
	}
}

// The exit code is the whole point of the type for a script. It has to survive
// the wrapping every command does on the way back up.
func TestExitCodeFor(t *testing.T) {
	suspended := newAPIError(http.StatusPaymentRequired,
		suspendedBody("payment_failed", billingActionUpdateCard, "Update the payment method."))

	if got := exitCodeFor(suspended); got != exitCodeBillingSuspended {
		t.Errorf("exit code = %d, want %d", got, exitCodeBillingSuspended)
	}
	if got := exitCodeFor(fmt.Errorf("deploying agent: %w", suspended)); got != exitCodeBillingSuspended {
		t.Errorf("wrapped exit code = %d, want %d", got, exitCodeBillingSuspended)
	}
	if got := exitCodeFor(errors.New("something else")); got != 1 {
		t.Errorf("unrelated error exit code = %d, want 1", got)
	}
	if got := exitCodeFor(newAPIError(http.StatusInternalServerError, []byte("boom"))); got != 1 {
		t.Errorf("server error exit code = %d, want 1", got)
	}
}

// The 404 helper reads the body off the typed error rather than splitting the
// message text, so a body that happens to contain ": " no longer truncates.
func TestAPIErrorCodeAndBody_ReadsTheTypedError(t *testing.T) {
	body := `{"error_code":"build_not_found","message":"no builds found for agent: chatbot"}`
	code, got := apiErrorCodeAndBody(fmt.Errorf("wrapped: %w", newAPIError(http.StatusNotFound, []byte(body))))

	if code != errCodeBuildNotFound {
		t.Errorf("code = %q, want %q", code, errCodeBuildNotFound)
	}
	if got != body {
		t.Errorf("body = %q, want the whole body", got)
	}
}
