package cmd

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestPollingGETRecoversFromTransientResponses(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTooManyRequests} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodGet, r.Method)
				require.Equal(t, "Bearer token", r.Header.Get("Authorization"))
				calls++
				if calls == 1 {
					w.Header().Set("Retry-After", "0")
					w.WriteHeader(status)
					return
				}
				_, _ = io.WriteString(w, `{"build":"one"}`)
			}))
			t.Cleanup(server.Close)
			var result struct {
				Build string `json:"build"`
			}
			got, err := pollingGET(context.Background(), server.URL, "token", false, &result)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, got)
			require.Equal(t, "one", result.Build)
			require.Equal(t, 2, calls)
		})
	}
}

func TestPollingGETBoundsRetriesAndPreservesTheLastError(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"error":"attempt %d","code":"PROXY_FAILURE"}`, calls)
	}))
	t.Cleanup(server.Close)
	status, err := pollingGET(context.Background(), server.URL, "", false, nil)
	var apiErr *apiError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, http.StatusBadGateway, status)
	require.Equal(t, "attempt 4", apiErr.Message)
	require.Equal(t, "PROXY_FAILURE", apiErr.Code)
	require.Equal(t, 4, calls)
}

func TestPollingGETDoesNotRetryPermanentResponses(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 412, 501, 200} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(status)
				_, _ = io.WriteString(w, "invalid JSON")
			}))
			t.Cleanup(server.Close)
			var result map[string]any
			got, err := pollingGET(context.Background(), server.URL, "", false, &result)
			require.Error(t, err)
			require.Equal(t, status, got)
			require.Equal(t, 1, calls)
		})
	}
}

func TestPollingGETHonorsRetryAfterAndCancellation(t *testing.T) {
	for _, mode := range []string{"deadline", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 650*time.Millisecond)
			defer cancel()
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.Header().Set("Retry-After", "60")
				w.WriteHeader(http.StatusTooManyRequests)
				if mode == "canceled" {
					cancel()
				}
			}))
			t.Cleanup(server.Close)
			_, err := pollingGET(ctx, server.URL, "", false, nil)
			require.ErrorIs(t, err, ctx.Err())
			require.Equal(t, 1, calls)
		})
	}
}

func TestRetryAfterDelayAcceptsSecondsAndHTTPDates(t *testing.T) {
	now := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		value string
		want  time.Duration
	}{
		{"2", 2 * time.Second}, {now.Add(time.Minute).Format(http.TimeFormat), time.Minute},
		{now.Add(-time.Minute).Format(http.TimeFormat), 0}, {"-1", 0}, {"invalid", 0},
	} {
		t.Run(tc.value, func(t *testing.T) { require.Equal(t, tc.want, retryAfterDelay(tc.value, now)) })
	}
}

func TestRetryableReadClassifiesTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{"reset", &url.Error{Err: &net.OpError{Err: syscall.ECONNRESET}}, true},
		{"refused", syscall.ECONNREFUSED, true},
		{"EOF", io.EOF, true},
		{"truncated body", io.ErrUnexpectedEOF, true},
		{"timeout", &url.Error{Err: context.DeadlineExceeded}, true},
		{"canceled", context.Canceled, false},
		{"certificate", &url.Error{Err: x509.UnknownAuthorityError{}}, false},
		{"unknown", errors.New("permanent failure"), false},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.want, retryableRead(-1, tc.err)) })
	}
}

func TestPollingGETRecoversFromTruncatedResponse(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Length", "100")
			_, _ = io.WriteString(w, `{"build":`)
			return
		}
		_, _ = io.WriteString(w, `{"build":"one"}`)
	}))
	t.Cleanup(server.Close)
	var result map[string]string
	_, err := pollingGET(context.Background(), server.URL, "", false, &result)
	require.NoError(t, err)
	require.Equal(t, "one", result["build"])
	require.Equal(t, 2, calls)
}

func TestDeployDoesNotRetryFailedWrites(t *testing.T) {
	for _, path := range []string{"/api/v1/agents/testaccount/dummy/deployment-template", "/api/v1/deploy"} {
		t.Run(path, func(t *testing.T) {
			calls := 0
			setupAgentRedeployTest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, http.MethodPost, r.Method)
				if r.URL.Path == path {
					calls++
					w.WriteHeader(http.StatusServiceUnavailable)
					return
				}
				_ = json.NewEncoder(w).Encode(deployTemplateResponse{Template: json.RawMessage(`{"spec":"deployment/v1"}`), Validation: deployTemplateValidation{Valid: true}, Signature: "signature"})
			}))
			cmd := &cobra.Command{}
			registerDeployCommonFlags(cmd)
			cmd.SetContext(context.Background())
			cmd.SetOut(io.Discard)
			err := runDeployWithRequest(cmd, AccountToken{Account: "testaccount"}, false, deployOptions{TemplateAccount: "testaccount", Blueprint: "dummy"})
			var apiErr *apiError
			require.ErrorAs(t, err, &apiErr)
			require.Equal(t, http.StatusServiceUnavailable, apiErr.StatusCode)
			require.Equal(t, 1, calls)
		})
	}
}
