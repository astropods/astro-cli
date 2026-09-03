package cmd

import (
	"encoding/json"
	"fmt"
	"strings"
)

// billingSuspendedCode is the server's code for an account the consumption gate
// refused. It arrives with 402 from astro-server and with 403 from the registry,
// so the code rather than the status is what identifies it.
const billingSuspendedCode = "BILLING_SUSPENDED"

// Billing actions the server names. Each maps to a different thing the reader
// has to do, and getting it wrong sends an account with a working card to add
// one it already has.
const (
	billingActionAddCard        = "add_card"
	billingActionUpdateCard     = "update_card"
	billingActionContactSupport = "contact_support"
)

// exitCodeBillingSuspended is the process exit code for a refused-for-billing
// command. A script that retries on failure has to be able to tell "the server
// was busy" from "this account cannot run anything until someone pays".
const exitCodeBillingSuspended = 3

// apiError is a non-2xx response from the server. The structured fields are
// present only when the server sends them; an older server, a proxy error page,
// or any non-JSON body leaves them empty and Error() falls back to the raw text.
type apiError struct {
	StatusCode int
	Code       string
	Reason     string
	Action     string
	Message    string
	Details    string
	Body       string
}

// newAPIError builds the error for a non-2xx response. It never fails: a body it
// cannot parse is carried through verbatim, because a CLI that swallowed an
// unrecognised error would leave the user with a status code and nothing else.
func newAPIError(statusCode int, body []byte) *apiError {
	e := &apiError{StatusCode: statusCode, Body: strings.TrimSpace(string(body))}
	var parsed struct {
		Error   string `json:"error"`
		Code    string `json:"code"`
		Reason  string `json:"reason"`
		Action  string `json:"action"`
		Details string `json:"details"`
	}
	if err := json.Unmarshal(body, &parsed); err == nil {
		e.Message, e.Code, e.Reason, e.Action, e.Details = parsed.Error, parsed.Code, parsed.Reason, parsed.Action, parsed.Details
	}
	return e
}

// isBillingSuspended reports whether the account was refused for billing.
func (e *apiError) isBillingSuspended() bool {
	return e != nil && e.Code == billingSuspendedCode
}

func (e *apiError) isStructured() bool {
	return e != nil && e.Code != ""
}

func (e *apiError) Error() string {
	if e.isBillingSuspended() {
		return e.billingMessage()
	}
	if e.isStructured() {
		if detail := e.detailSentence(); detail != "" {
			return detail
		}
	}
	// Uncoded responses keep the shape older commands and scripts expect.
	return fmt.Sprintf("server returned status %d: %s", e.StatusCode, e.Body)
}

// billingMessage renders the refusal as something a person can act on: what
// happened, and the one command or page that fixes it.
//
// The server supplies the sentence, so the CLI adds the next step rather than
// composing its own explanation. A build that cannot name the action still
// prints the server's own words instead of the JSON.
func (e *apiError) billingMessage() string {
	var b strings.Builder
	b.WriteString(colorRed + "Billing: this account is suspended" + colorReset)
	if detail := e.detailSentence(); detail != "" {
		b.WriteString("\n\n  " + detail)
	}
	if next := billingNextStep(e.Action); next != "" {
		b.WriteString("\n\n  " + next)
	}
	return b.String()
}

// detailSentence prefers the server's explanation, falling back to its short
// error string.
func (e *apiError) detailSentence() string {
	if e.Details != "" {
		return e.Details
	}
	return e.Message
}

// billingNextStep names where the fix happens. An unrecognised action returns
// nothing rather than guessing, because the wrong instruction is worse than the
// server's sentence on its own.
func billingNextStep(action string) string {
	switch action {
	case billingActionAddCard, billingActionUpdateCard:
		return "Manage billing at " + colorBold + "https://astropod.ai/settings/billing" + colorReset
	case billingActionContactSupport:
		return "Contact support to continue."
	default:
		return ""
	}
}
