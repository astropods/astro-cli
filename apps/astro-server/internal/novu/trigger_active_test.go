package novu

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func triggerStatusError(t *testing.T, status string) error {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"acknowledged":true,"status":"` + status + `"}}`))
	}))
	defer srv.Close()

	err := NewClient(srv.URL, "key").Trigger(context.Background(), TriggerRequest{
		WorkflowID: "billing.spend_warning",
		To:         []Subscriber{{SubscriberID: "user_1"}},
	})
	if err == nil {
		t.Fatalf("status %q produced no error", status)
	}
	if !strings.Contains(err.Error(), status) {
		t.Errorf("error %q does not name the status, which is all the log has", err)
	}
	return err
}

// These statuses mean the trigger was accepted and delivered to nobody: the
// workflow is off, it defines no live steps, the environment has no provider for
// its channels. All answer 201, so checking the HTTP code reports success for a
// notification that was never sent, which is quieter than a missing workflow's
// 422: the job completes and nothing is logged.
func TestTriggerRejectsTheConfigurationStatuses(t *testing.T) {
	for _, status := range []string{
		"trigger_not_active",
		"no_workflow_active_steps_defined",
		"no_workflow_steps_defined",
		"subscriber_id_missing",
	} {
		t.Run(status, func(t *testing.T) {
			if err := triggerStatusError(t, status); !errors.Is(err, ErrNotDelivered) {
				t.Fatalf("Trigger error = %v, want ErrNotDelivered", err)
			}
		})
	}
}

// A status the caller cannot prove is permanent has to retry. ErrNotDelivered
// cancels the job, and a spend threshold crosses once, so classifying a
// recoverable provider failure as permanent loses the only notification the
// owner was going to get.
func TestTriggerRetriesAnAmbiguousStatus(t *testing.T) {
	for _, status := range []string{
		// Novu's catch-all, covering provider and internal failures alike.
		"error",
		// A status Novu has not shipped yet. Retrying an unknown outcome still
		// names it in the log, and it does not throw the notification away.
		"some_future_status",
	} {
		t.Run(status, func(t *testing.T) {
			if err := triggerStatusError(t, status); errors.Is(err, ErrNotDelivered) {
				t.Fatalf("status %q was classified permanent", status)
			}
		})
	}
}

func TestTriggerAcceptsAProcessedWorkflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"acknowledged":true,"status":"processed","transactionId":"t1"}}`))
	}))
	defer srv.Close()

	if err := NewClient(srv.URL, "key").Trigger(context.Background(), TriggerRequest{
		WorkflowID: "billing.spend_warning",
		To:         []Subscriber{{SubscriberID: "user_1"}},
	}); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
}

// Reading the body must not turn a successful send into a retry. Before this
// check existed the body was discarded, so any 2xx was delivered; a shape change
// at the provider would otherwise resend every notification in the queue.
func TestTriggerToleratesABodyItCannotRead(t *testing.T) {
	cases := []struct {
		name, body string
	}{
		{"no status field", `{"data":{"acknowledged":true}}`},
		{"empty body", ``},
		{"whitespace only", "  \n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			if err := NewClient(srv.URL, "key").Trigger(context.Background(), TriggerRequest{
				WorkflowID: "billing.recovered",
				To:         []Subscriber{{SubscriberID: "user_1"}},
			}); err != nil {
				t.Fatalf("Trigger: %v", err)
			}
		})
	}
}
