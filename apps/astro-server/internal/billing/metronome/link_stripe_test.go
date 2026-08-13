package metronome

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Metronome-Industries/metronome-go/v3"
	"github.com/Metronome-Industries/metronome-go/v3/option"
)

// stubMetronome records every request body by path so a test can assert both
// what was sent and what was never called.
type stubMetronome struct {
	t        *testing.T
	bodies   map[string][]map[string]any
	handlers map[string]string
}

func newStub(t *testing.T, handlers map[string]string) (*Provider, *stubMetronome) {
	t.Helper()
	s := &stubMetronome{t: t, bodies: map[string][]map[string]any{}, handlers: handlers}
	srv := httptest.NewServer(s)
	t.Cleanup(srv.Close)
	p := &Provider{
		mc: metronome.NewClient(
			option.WithBearerToken("test"),
			option.WithBaseURL(srv.URL),
			option.WithMaxRetries(0),
		),
	}
	return p, s
}

func (s *stubMetronome) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	s.bodies[path] = append(s.bodies[path], body)

	resp, ok := s.handlers[path]
	if !ok {
		s.t.Errorf("unexpected call to %s", path)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(resp))
}

func (s *stubMetronome) calls(path string) int { return len(s.bodies[path]) }

func (s *stubMetronome) firstBody(path string) map[string]any {
	s.t.Helper()
	if len(s.bodies[path]) == 0 {
		s.t.Fatalf("%s was never called", path)
	}
	return s.bodies[path][0]
}

const (
	pathGetConfigs  = "v1/getCustomerBillingProviderConfigurations"
	pathSetConfigs  = "v1/setCustomerBillingProviderConfigurations"
	pathListDeliver = "v1/listConfiguredBillingProviders"
	pathListConts   = "v2/contracts/list"
	pathEditCont    = "v2/contracts/edit"

	noConfigs   = `{"data":[]}`
	oneDelivery = `{"data":[{"billing_provider":"stripe","delivery_method":"direct_to_billing_provider",` +
		`"delivery_method_id":"dm_1","delivery_method_configuration":{"stripe_account_id":"acct_1"}}],"next_page":null}`
	configCreated  = `{"data":[{"id":"cfg_1","billing_provider":"stripe","customer_id":"cust_1"}]}`
	bareContract   = `{"data":[{"id":"con_1","billing_provider_configuration_schedule":[]}],"next_page":null}`
	routedContract = `{"data":[{"id":"con_1","billing_provider_configuration_schedule":` +
		`[{"effective_at":"2026-01-01T00:00:00Z","billing_provider_configuration":{"id":"cfg_1"}}]}],"next_page":null}`
	editAccepted = `{"data":{"id":"con_1"}}`
)

// Delivery resolves the Stripe credential through the delivery method, so a
// configuration written without one fails at send time with "No token found for
// environment type <env> and billing provider STRIPE".
func TestLinkStripeCustomer_PinsTheDeliveryMethod(t *testing.T) {
	p, s := newStub(t, map[string]string{
		pathGetConfigs:  noConfigs,
		pathListDeliver: oneDelivery,
		pathSetConfigs:  configCreated,
		pathListConts:   bareContract,
		pathEditCont:    editAccepted,
	})

	if err := p.LinkStripeCustomer(context.Background(), "cust_1", "cus_stripe"); err != nil {
		t.Fatalf("LinkStripeCustomer: %v", err)
	}

	sent, _ := s.firstBody(pathSetConfigs)["data"].([]any)
	if len(sent) != 1 {
		t.Fatalf("set configurations sent %d entries, want 1", len(sent))
	}
	entry, _ := sent[0].(map[string]any)
	if entry["delivery_method_id"] != "dm_1" {
		t.Errorf("delivery_method_id = %v, want dm_1: Metronome cannot resolve a Stripe token without it", entry["delivery_method_id"])
	}
	cfg, _ := entry["configuration"].(map[string]any)
	if cfg["stripe_customer_id"] != "cus_stripe" {
		t.Errorf("stripe_customer_id = %v, want cus_stripe", cfg["stripe_customer_id"])
	}
}

// A contract from a package carries no billing provider configuration, so its
// invoices stay inside Metronome until the link points it at one.
func TestLinkStripeCustomer_RoutesTheContract(t *testing.T) {
	p, s := newStub(t, map[string]string{
		pathGetConfigs:  noConfigs,
		pathListDeliver: oneDelivery,
		pathSetConfigs:  configCreated,
		pathListConts:   bareContract,
		pathEditCont:    editAccepted,
	})

	if err := p.LinkStripeCustomer(context.Background(), "cust_1", "cus_stripe"); err != nil {
		t.Fatalf("LinkStripeCustomer: %v", err)
	}

	body := s.firstBody(pathEditCont)
	if body["contract_id"] != "con_1" {
		t.Fatalf("edited contract %v, want con_1", body["contract_id"])
	}
	update, _ := body["add_billing_provider_configuration_update"].(map[string]any)
	cfg, _ := update["billing_provider_configuration"].(map[string]any)
	if cfg["billing_provider_configuration_id"] != "cfg_1" {
		t.Errorf("contract points at %v, want cfg_1", cfg["billing_provider_configuration_id"])
	}
	schedule, _ := update["schedule"].(map[string]any)
	if schedule["effective_at"] != "START_OF_CURRENT_PERIOD" {
		t.Errorf("effective_at = %v, want START_OF_CURRENT_PERIOD: the open invoice must route too", schedule["effective_at"])
	}
}

// Every card add re-links, so a second run must not stack a duplicate
// configuration or re-edit a contract that already names one.
func TestLinkStripeCustomer_IsIdempotent(t *testing.T) {
	existing := `{"data":[{"id":"cfg_1","billing_provider":"stripe","customer_id":"cust_1",` +
		`"configuration":{"stripe_customer_id":"cus_stripe"},"delivery_method_id":"dm_1"}]}`
	p, s := newStub(t, map[string]string{
		pathGetConfigs: existing,
		pathListConts:  routedContract,
	})

	if err := p.LinkStripeCustomer(context.Background(), "cust_1", "cus_stripe"); err != nil {
		t.Fatalf("LinkStripeCustomer: %v", err)
	}

	if n := s.calls(pathSetConfigs); n != 0 {
		t.Errorf("wrote %d configurations, want 0: the customer already had one", n)
	}
	if n := s.calls(pathEditCont); n != 0 {
		t.Errorf("edited %d contracts, want 0: the contract already routes", n)
	}
}

// With two Stripe accounts connected, which one to bill is a decision this path
// cannot make, and guessing sends the invoice to the wrong entity.
func TestLinkStripeCustomer_RefusesAmbiguousDeliveryMethod(t *testing.T) {
	two := `{"data":[` +
		`{"billing_provider":"stripe","delivery_method":"direct_to_billing_provider","delivery_method_id":"dm_1","delivery_method_configuration":{}},` +
		`{"billing_provider":"stripe","delivery_method":"direct_to_billing_provider","delivery_method_id":"dm_2","delivery_method_configuration":{}}],"next_page":null}`
	p, s := newStub(t, map[string]string{
		pathGetConfigs:  noConfigs,
		pathListDeliver: two,
	})

	err := p.LinkStripeCustomer(context.Background(), "cust_1", "cus_stripe")
	if err == nil {
		t.Fatal("linked against an ambiguous delivery method, want an error")
	}
	if !strings.Contains(err.Error(), "cannot choose one") {
		t.Errorf("error = %q, want it to name the ambiguity", err)
	}
	if n := s.calls(pathSetConfigs); n != 0 {
		t.Errorf("wrote %d configurations, want 0", n)
	}
}
