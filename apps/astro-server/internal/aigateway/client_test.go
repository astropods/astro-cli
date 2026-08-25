package aigateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateKey_RequiresAccountID(t *testing.T) {
	// AccountID is load-bearing: it becomes the Bifrost customer_id and the VK
	// name, so per-account attribution and budget depend on it. The client
	// asserts it non-empty before serializing.
	c := NewClient("http://example", "", "")

	if _, err := c.GenerateKey(context.Background(), KeyRequest{}); err == nil {
		t.Error("expected error when AccountID empty")
	} else if !strings.Contains(err.Error(), "AccountID") {
		t.Errorf("expected AccountID error, got %v", err)
	}
}

func TestGenerateKey_ScopesBedrockAndCarriesAccountID(t *testing.T) {
	// End-to-end check of the Bifrost virtual-key create: the account-id lands
	// in the VK name (attribution), the grant is Bedrock with key_ids ["*"],
	// and the plaintext value + id are parsed back out.
	var captured bifrostVKRequest
	var authHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/governance/virtual-keys" {
			http.Error(w, "wrong route", http.StatusBadRequest)
			return
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"virtual_key": map[string]string{"id": "vk-123", "value": "sk-bf-xxx"},
		})
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "Basic dGVzdA==")
	resp, err := c.GenerateKey(context.Background(), KeyRequest{
		AccountID:  "acct-42",
		CustomerID: "cust-uuid",
		Metadata:   map[string]any{"cluster_id": "preview-a"},
	})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if authHeader != "Basic dGVzdA==" {
		t.Errorf("admin auth header on wire: got %q", authHeader)
	}
	if !strings.Contains(captured.Name, "acct-42") {
		t.Errorf("account-id must ride in VK name: got %q", captured.Name)
	}
	if captured.CustomerID != "cust-uuid" {
		t.Errorf("customer_id must be the resolved customer id, got %q", captured.CustomerID)
	}
	if len(captured.ProviderConfigs) != 1 || captured.ProviderConfigs[0].Provider != "bedrock" {
		t.Fatalf("expected one bedrock provider_config, got %+v", captured.ProviderConfigs)
	}
	if len(captured.ProviderConfigs[0].KeyIDs) != 1 || captured.ProviderConfigs[0].KeyIDs[0] != "*" {
		t.Errorf("grant must be key_ids [*], got %+v", captured.ProviderConfigs[0].KeyIDs)
	}
	if len(captured.ProviderConfigs[0].AllowedModels) != 1 || captured.ProviderConfigs[0].AllowedModels[0] != "*" {
		t.Errorf("default grant must allow all models, got %+v", captured.ProviderConfigs[0].AllowedModels)
	}
	if !captured.IsActive {
		t.Error("VK should be created active")
	}
	if resp.Key != "sk-bf-xxx" || resp.KeyID != "vk-123" {
		t.Errorf("response parse: got %+v", resp)
	}
}

func TestGenerateKey_EvalJudgeNameAndWildcardModels(t *testing.T) {
	var captured bifrostVKRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"virtual_key": map[string]string{"id": "vk-judge", "value": "sk-bf-judge"},
		})
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	_, err := c.GenerateKey(context.Background(), KeyRequest{
		AccountID:  "acct-42",
		CustomerID: "cust-42",
		Metadata:   map[string]any{"kind": "eval-judge"},
	})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if captured.Name != "eval-judge/acct-42" {
		t.Errorf("name = %q, want deterministic judge name", captured.Name)
	}
	if got := captured.ProviderConfigs[0].AllowedModels; len(got) != 1 || got[0] != "*" {
		t.Errorf("allowed_models = %+v, want wildcard model access", got)
	}
	if captured.ExpiresAt != "" {
		t.Errorf("expires_at = %q, want no expiry", captured.ExpiresAt)
	}
}

func TestVKName_DevKeysAreUniquePerMint(t *testing.T) {
	// Dev keys rotate while the predecessor is still alive upstream, and two
	// users share an account, so the VK name must be unique per mint (Bifrost
	// enforces unique names). Assert the actor rides in the name and that two
	// mints — same account, same actor — never collide.
	mk := func(actor string) string {
		return vkName(KeyRequest{
			AccountID: "acct-42",
			Metadata:  map[string]any{"kind": "dev", "actor_user_id": actor},
		})
	}
	a1, a2 := mk("user-1"), mk("user-1")
	if a1 == a2 {
		t.Errorf("successive dev mints for one actor must not collide: %q == %q", a1, a2)
	}
	for _, n := range []string{a1, a2} {
		if !strings.HasPrefix(n, "dev/acct-42/user-1/") {
			t.Errorf("dev name must carry account + actor, got %q", n)
		}
	}
	if b := mk("user-2"); strings.HasPrefix(b, "dev/acct-42/user-1/") {
		t.Errorf("distinct actors must get distinct names, got %q", b)
	}
}

func TestVKName_DeploymentKeysUnchanged(t *testing.T) {
	// Deployment keys are unique per deployment and never re-mint, so their
	// name stays stable (no nonce).
	got := vkName(KeyRequest{
		AccountID: "acct-42",
		Metadata:  map[string]any{"tags": []string{"deployment:dep-7", "agent:foo"}},
	})
	if got != "acct-42/deployment:dep-7" {
		t.Errorf("deployment VK name should be stable, got %q", got)
	}
}

func TestCreateCustomer_CreatesWithBudgetAndReturnsID(t *testing.T) {
	var captured bifrostCustomerRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/governance/customers" {
			http.Error(w, "wrong route", http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"customer": map[string]string{"id": "cust-1", "name": "acct-9"},
		})
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	id, err := c.CreateCustomer(context.Background(), "acct-9")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if id != "cust-1" {
		t.Errorf("returned id: got %q, want cust-1", id)
	}
	if captured.Name != "acct-9" {
		t.Errorf("customer name should be the account-id, got %q", captured.Name)
	}
	if len(captured.Budgets) != 1 || captured.Budgets[0].MaxLimit != CardlessBudgetUSD || captured.Budgets[0].ResetDuration != monthlyReset {
		t.Errorf("a new customer has no card, so it starts on the card-less budget, got %+v", captured.Budgets)
	}
}

func TestSetCustomerBudget_RewritesTheMonthlyLimit(t *testing.T) {
	var captured bifrostCustomerUpdate
	var path string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			http.Error(w, "wrong method", http.StatusBadRequest)
			return
		}
		path = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	if err := c.SetCustomerBudget(context.Background(), "cust-1", CardedBudgetUSD); err != nil {
		t.Fatalf("SetCustomerBudget: %v", err)
	}
	if path != "/api/governance/customers/cust-1" {
		t.Errorf("path: got %q", path)
	}
	// The window is what the gateway matches the existing budget on, so a
	// mismatch here would add a second budget instead of raising the limit.
	if len(captured.Budgets) != 1 || captured.Budgets[0].MaxLimit != CardedBudgetUSD || captured.Budgets[0].ResetDuration != monthlyReset {
		t.Errorf("budgets: got %+v", captured.Budgets)
	}
}

func TestSetCustomerBudget_FloorsAtTheSmallestLimitTheGatewayAccepts(t *testing.T) {
	var captured bifrostCustomerUpdate
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&captured)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	if err := c.SetCustomerBudget(context.Background(), "cust-1", 0); err != nil {
		t.Fatalf("SetCustomerBudget: %v", err)
	}
	// A zero limit is rejected by the gateway, and a rejected update would leave
	// the previous limit in force.
	if len(captured.Budgets) != 1 || captured.Budgets[0].MaxLimit != 0.01 {
		t.Errorf("budgets: got %+v", captured.Budgets)
	}
}

func TestCreateCustomer_ConflictLooksUpExisting(t *testing.T) {
	// A 409 (name already exists) falls back to listing and returning the id.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/governance/customers":
			http.Error(w, "A customer with this name already exists", http.StatusConflict)
		case r.Method == http.MethodGet && r.URL.Path == "/api/governance/customers":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"customers": []map[string]string{{"id": "cust-existing", "name": "acct-9"}},
			})
		default:
			http.Error(w, "wrong route", http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	id, err := c.CreateCustomer(context.Background(), "acct-9")
	if err != nil {
		t.Fatalf("CreateCustomer: %v", err)
	}
	if id != "cust-existing" {
		t.Errorf("conflict path should return existing id, got %q", id)
	}
}

func TestDeleteKey_TreatsNotFoundAsSuccess(t *testing.T) {
	// Retries on transient failures shouldn't fail on the second pass when
	// the upstream already deleted the key.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	if err := c.DeleteKey(context.Background(), "vk-gone"); err != nil {
		t.Errorf("404 should be treated as success, got %v", err)
	}
}

func TestDeleteKey_PropagatesOtherErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"server error"}`, http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient("https://aig.example", srv.URL, "")
	err := c.DeleteKey(context.Background(), "vk-1")
	if err == nil {
		t.Fatal("expected error from 500")
	}
	var he *httpError
	if !errors.As(err, &he) || he.Status != http.StatusInternalServerError {
		t.Errorf("expected 500 httpError, got %v", err)
	}
}
