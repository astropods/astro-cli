package cmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/astropods/astro-cli/internal/auth"
)

// cliError is a failure the CLI explains in three parts: what failed, why, and
// what to do next.
type cliError struct {
	headline string
	reason   string
	fix      string
	cause    error
}

func (e *cliError) Error() string {
	parts := []string{e.headline + "."}
	for _, s := range []string{e.reason, e.fix} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

func (e *cliError) Unwrap() error { return e.cause }

func newCLIError(headline, reason, fix string, cause error) *cliError {
	return &cliError{headline: headline, reason: reason, fix: fix, cause: cause}
}

func printExecutionError(w io.Writer, err error) {
	e := explainError(err)
	fmt.Fprintf(w, "%s✗ %s%s\n", colorRed, e.headline, colorReset) //nolint:errcheck
	if e.reason != "" {
		fmt.Fprintf(w, "  %s\n", e.reason) //nolint:errcheck
	}
	if e.fix != "" {
		fmt.Fprintf(w, "  %s→ %s%s\n", colorBold, e.fix, colorReset) //nolint:errcheck
	}
	if e.cause != nil {
		fmt.Fprintf(w, "  %sCause: %s%s\n", colorDim, e.cause, colorReset) //nolint:errcheck
	}
}

// explainError returns err as a cliError. An error with no explanation keeps its
// own text as the headline.
func explainError(err error) *cliError {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce
	}
	if errors.Is(err, errDirtyWorkingTree) {
		return newCLIError(msgDirtyPushBlockedHeadline(), "", msgDirtyPushBlockedRemediation(), nil)
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		if apiErr.isBillingSuspended() {
			return newCLIError(msgBillingSuspendedHeadline(), sentence(apiErr.detailSentence()), billingNextStep(apiErr.Action), err)
		}
		return explainAPIError(err, apiErr)
	}
	if unreachable := asUnreachable(err); unreachable != nil {
		return errServerUnreachable(unreachable)
	}
	return newCLIError(strings.TrimRight(err.Error(), "."), "", "", nil)
}

func explainAPIError(err error, apiErr *apiError) *cliError {
	reason := sentence(apiErr.detailSentence())
	headline := msgRequestFailedHeadline(apiErr.StatusCode)
	refused := apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden
	switch {
	case refused && auth.UsesMachineCredentials() && apiErr.isAuthorizationDenied():
		headline = msgAppLacksScopeHeadline()
	case refused && auth.UsesMachineCredentials():
		headline = msgAppCommandUnsupportedHeadline()
	default:
		if context := errorContext(err, apiErr); context != "" {
			headline = capitalize(context)
		}
	}
	var cause error
	if reason == "" {
		cause = err
	}
	return newCLIError(headline, reason, apiErrorFix(apiErr), cause)
}

func apiErrorFix(apiErr *apiError) string {
	machine := auth.UsesMachineCredentials()
	switch {
	case apiErr.isAuthorizationDenied() && machine && apiErr.deniedAction() != "":
		return msgFixAddAppScope(apiErr.deniedAction())
	case apiErr.isAuthorizationDenied():
		return msgFixAskForPermission()
	case machine && (apiErr.StatusCode == http.StatusUnauthorized || apiErr.StatusCode == http.StatusForbidden):
		return msgMachineCommandUnsupported()
	case apiErr.StatusCode == http.StatusUnauthorized:
		return msgFixSignIn()
	case apiErr.StatusCode >= http.StatusInternalServerError:
		return msgFixRetryLater()
	default:
		return ""
	}
}

// errorContext is the text a caller wrapped around apiErr, such as
// "registration failed", or "" when err is apiErr itself.
func errorContext(err error, apiErr *apiError) string {
	full, inner := err.Error(), apiErr.Error()
	if full == inner || !strings.HasSuffix(full, inner) {
		return ""
	}
	return strings.TrimRight(strings.TrimSuffix(full, inner), ": \n")
}

// asUnreachable returns the request error in err when the CLI never got an
// answer from the server.
func asUnreachable(err error) *url.Error {
	var urlErr *url.Error
	if !errors.As(err, &urlErr) {
		return nil
	}
	var opErr *net.OpError
	var dnsErr *net.DNSError
	if urlErr.Timeout() || errors.As(urlErr, &opErr) || errors.As(urlErr, &dnsErr) {
		return urlErr
	}
	return nil
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	s = capitalize(s)
	if !strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "?") && !strings.HasSuffix(s, "!") {
		s += "."
	}
	return s
}
