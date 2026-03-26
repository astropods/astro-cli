//go:build integration

package e2e

import (
	"strings"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/deployid"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	spec "github.com/astropods/astro/packages/astro-spec"
	_ "github.com/lib/pq"
)

func createVarsTestAccount(t *testing.T, store *account.AccountStore) *account.Account {
	t.Helper()
	acct, err := store.Create("vars-e2e-"+deployid.New(), "personal", "user-vars-test", "")
	if err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	return acct
}

// TestAccountVars_CRUD tests create, list, get, update, and delete operations.
func TestAccountVars_CRUD(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	// List empty
	vars, err := varsStore.List(acct.ID)
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(vars) != 0 {
		t.Fatalf("expected 0 variables, got %d", len(vars))
	}

	// Create plaintext variable
	plainVar := &accountvars.AccountVariable{
		AccountID:   acct.ID,
		Name:        "LOG_LEVEL",
		Value:       "debug",
		Secret:      false,
		Description: "logging level",
	}
	if err := varsStore.Save(plainVar); err != nil {
		t.Fatalf("Save plaintext: %v", err)
	}

	// Create secret variable (no KMS in tests — stores plaintext with secret flag)
	secretVar := &accountvars.AccountVariable{
		AccountID:   acct.ID,
		Name:        "API_KEY",
		Value:       "sk-test-123",
		Secret:      true,
		Nonce:       nil, // no encryption in tests
		Description: "api key for external service",
	}
	if err := varsStore.Save(secretVar); err != nil {
		t.Fatalf("Save secret: %v", err)
	}

	// List — should return 2 variables ordered by name
	vars, err = varsStore.List(acct.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(vars))
	}
	if vars[0].Name != "API_KEY" || vars[1].Name != "LOG_LEVEL" {
		t.Errorf("unexpected order: %v, %v", vars[0].Name, vars[1].Name)
	}
	if !vars[0].Secret {
		t.Error("API_KEY should be marked as secret")
	}
	if vars[1].Secret {
		t.Error("LOG_LEVEL should not be marked as secret")
	}

	// Get single
	got, err := varsStore.Get(acct.ID, "LOG_LEVEL")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("Get returned nil for existing variable")
	}
	if got.Value != "debug" {
		t.Errorf("expected value 'debug', got %q", got.Value)
	}
	if got.Description != "logging level" {
		t.Errorf("expected description 'logging level', got %q", got.Description)
	}

	// Get nonexistent
	got, err = varsStore.Get(acct.ID, "NONEXISTENT")
	if err != nil {
		t.Fatalf("Get nonexistent: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent variable")
	}

	// Update (upsert)
	plainVar.Value = "info"
	plainVar.Description = "updated logging level"
	if err := varsStore.Save(plainVar); err != nil {
		t.Fatalf("Save update: %v", err)
	}
	got, err = varsStore.Get(acct.ID, "LOG_LEVEL")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.Value != "info" {
		t.Errorf("expected updated value 'info', got %q", got.Value)
	}
	if got.Description != "updated logging level" {
		t.Errorf("expected updated description, got %q", got.Description)
	}

	// Delete
	if err := varsStore.Delete(acct.ID, "LOG_LEVEL"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	vars, err = varsStore.List(acct.ID)
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(vars) != 1 {
		t.Fatalf("expected 1 variable after delete, got %d", len(vars))
	}
	if vars[0].Name != "API_KEY" {
		t.Errorf("expected remaining variable API_KEY, got %q", vars[0].Name)
	}

	// Delete nonexistent
	if err := varsStore.Delete(acct.ID, "NONEXISTENT"); err == nil {
		t.Error("expected error when deleting nonexistent variable")
	}
}

// TestAccountVars_GetByNames tests batch fetching of variables by name.
func TestAccountVars_GetByNames(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	// Create 3 variables
	for _, name := range []string{"VAR_A", "VAR_B", "VAR_C"} {
		v := &accountvars.AccountVariable{
			AccountID: acct.ID,
			Name:      name,
			Value:     "val-" + name,
		}
		if err := varsStore.Save(v); err != nil {
			t.Fatalf("Save %s: %v", name, err)
		}
	}

	// Fetch subset
	got, err := varsStore.GetByNames(acct.ID, []string{"VAR_A", "VAR_C"})
	if err != nil {
		t.Fatalf("GetByNames: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 results, got %d", len(got))
	}

	names := map[string]bool{}
	for _, v := range got {
		names[v.Name] = true
	}
	if !names["VAR_A"] || !names["VAR_C"] {
		t.Errorf("expected VAR_A and VAR_C, got %v", names)
	}

	// Fetch with nonexistent name — returns only existing ones
	got, err = varsStore.GetByNames(acct.ID, []string{"VAR_B", "MISSING"})
	if err != nil {
		t.Fatalf("GetByNames with missing: %v", err)
	}
	if len(got) != 1 || got[0].Name != "VAR_B" {
		t.Errorf("expected only VAR_B, got %v", got)
	}

	// Empty names
	got, err = varsStore.GetByNames(acct.ID, nil)
	if err != nil {
		t.Fatalf("GetByNames nil: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 results for nil names, got %d", len(got))
	}
}

// TestAccountVars_EncryptionKey tests encryption key CRUD.
func TestAccountVars_EncryptionKey(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	// No key initially
	ek, err := varsStore.GetEncryptionKey(acct.ID)
	if err != nil {
		t.Fatalf("GetEncryptionKey empty: %v", err)
	}
	if ek != nil {
		t.Fatal("expected nil encryption key for new account")
	}

	// Save key
	fakeKey := []byte("encrypted-data-key-bytes")
	fakeARN := "arn:aws:kms:us-east-1:123456:key/test"
	if err := varsStore.SaveEncryptionKey(acct.ID, fakeKey, fakeARN); err != nil {
		t.Fatalf("SaveEncryptionKey: %v", err)
	}

	// Retrieve key
	ek, err = varsStore.GetEncryptionKey(acct.ID)
	if err != nil {
		t.Fatalf("GetEncryptionKey: %v", err)
	}
	if ek == nil {
		t.Fatal("expected encryption key")
	}
	if string(ek.EncryptedDataKey) != string(fakeKey) {
		t.Errorf("data key mismatch: got %q", ek.EncryptedDataKey)
	}
	if ek.KMSKeyARN != fakeARN {
		t.Errorf("ARN mismatch: got %q", ek.KMSKeyARN)
	}

	// Upsert key
	newKey := []byte("new-encrypted-data-key")
	if err := varsStore.SaveEncryptionKey(acct.ID, newKey, fakeARN); err != nil {
		t.Fatalf("SaveEncryptionKey upsert: %v", err)
	}
	ek, err = varsStore.GetEncryptionKey(acct.ID)
	if err != nil {
		t.Fatalf("GetEncryptionKey after upsert: %v", err)
	}
	if string(ek.EncryptedDataKey) != string(newKey) {
		t.Errorf("expected updated key, got %q", ek.EncryptedDataKey)
	}
}

// TestAccountVars_CascadeDelete verifies that deleting an account cascades to its variables and key.
func TestAccountVars_CascadeDelete(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)

	// Save key and variable
	if err := varsStore.SaveEncryptionKey(acct.ID, []byte("key"), "arn:test"); err != nil {
		t.Fatalf("SaveEncryptionKey: %v", err)
	}
	if err := varsStore.Save(&accountvars.AccountVariable{
		AccountID: acct.ID,
		Name:      "CASCADED",
		Value:     "val",
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Hard-delete account
	if _, err := db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID); err != nil {
		t.Fatalf("DELETE account: %v", err)
	}

	// Variables should be gone
	vars, err := varsStore.List(acct.ID)
	if err != nil {
		t.Fatalf("List after cascade: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected 0 variables after cascade delete, got %d", len(vars))
	}

	// Encryption key should be gone
	ek, err := varsStore.GetEncryptionKey(acct.ID)
	if err != nil {
		t.Fatalf("GetEncryptionKey after cascade: %v", err)
	}
	if ek != nil {
		t.Error("encryption key should be cascade-deleted with account")
	}
}

// TestAccountVars_UniqueConstraint verifies that duplicate names within an account
// are handled by upsert, not errored.
func TestAccountVars_UniqueConstraint(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	v := &accountvars.AccountVariable{
		AccountID:   acct.ID,
		Name:        "DUPE_TEST",
		Value:       "first",
		Description: "original",
	}
	if err := varsStore.Save(v); err != nil {
		t.Fatalf("Save first: %v", err)
	}

	// Save again with same name — should upsert
	v.Value = "second"
	v.Description = "updated"
	if err := varsStore.Save(v); err != nil {
		t.Fatalf("Save upsert: %v", err)
	}

	// Only one row
	vars, err := varsStore.List(acct.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(vars) != 1 {
		t.Fatalf("expected 1 variable, got %d", len(vars))
	}

	got, _ := varsStore.Get(acct.ID, "DUPE_TEST")
	if got.Value != "second" {
		t.Errorf("expected upserted value 'second', got %q", got.Value)
	}
}

// TestAccountVars_CrossAccountIsolation verifies that variables from one account
// are not visible to another.
func TestAccountVars_CrossAccountIsolation(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct1 := createVarsTestAccount(t, accountStore)
	acct2 := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() {
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct1.ID)
		_, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct2.ID)
	})

	// Create variable in acct1
	if err := varsStore.Save(&accountvars.AccountVariable{
		AccountID: acct1.ID,
		Name:      "ISOLATED",
		Value:     "acct1-val",
	}); err != nil {
		t.Fatalf("Save acct1: %v", err)
	}

	// Not visible from acct2
	got, err := varsStore.Get(acct2.ID, "ISOLATED")
	if err != nil {
		t.Fatalf("Get from acct2: %v", err)
	}
	if got != nil {
		t.Error("variable from acct1 should not be visible to acct2")
	}

	vars, err := varsStore.List(acct2.ID)
	if err != nil {
		t.Fatalf("List acct2: %v", err)
	}
	if len(vars) != 0 {
		t.Errorf("expected 0 variables for acct2, got %d", len(vars))
	}

	// GetByNames from acct2 returns nothing
	got2, err := varsStore.GetByNames(acct2.ID, []string{"ISOLATED"})
	if err != nil {
		t.Fatalf("GetByNames acct2: %v", err)
	}
	if len(got2) != 0 {
		t.Errorf("expected 0 results from acct2, got %d", len(got2))
	}
}

// TestAccountVars_RefResolution simulates the deploy-time variable resolution flow:
// save account variables, build a spec with ref fields, fetch and substitute.
func TestAccountVars_RefResolution(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	// Create account variables
	for _, v := range []accountvars.AccountVariable{
		{AccountID: acct.ID, Name: "API_KEY", Value: "sk-real-key", Secret: true},
		{AccountID: acct.ID, Name: "LOG_LEVEL", Value: "debug", Secret: false},
	} {
		if err := varsStore.Save(&v); err != nil {
			t.Fatalf("Save %s: %v", v.Name, err)
		}
	}

	// Build a spec with refs and literal values
	submittedSpec := &spec.AstroDeploymentSpec{
		Variables: map[string]spec.Variable{
			"API_KEY":   {Ref: "API_KEY", Secret: true, Targets: []string{"agent"}},
			"LOG_LEVEL": {Ref: "LOG_LEVEL", Targets: []string{"agent"}},
			"INLINE":    {Value: "literal-value", Targets: []string{"agent"}, Optional: true},
		},
	}

	// Simulate resolution: fetch account vars matching refs, substitute values
	refs := make(map[string]string)
	for key, v := range submittedSpec.Variables {
		if v.Ref != "" {
			refs[key] = v.Ref
		}
	}

	names := make([]string, 0, len(refs))
	for _, name := range refs {
		names = append(names, name)
	}

	acctVars, err := varsStore.GetByNames(acct.ID, names)
	if err != nil {
		t.Fatalf("GetByNames: %v", err)
	}

	varMap := make(map[string]*accountvars.AccountVariable, len(acctVars))
	for i := range acctVars {
		varMap[acctVars[i].Name] = &acctVars[i]
	}

	// Resolve
	for varKey, acctVarName := range refs {
		av, ok := varMap[acctVarName]
		if !ok {
			t.Fatalf("account variable %q not found", acctVarName)
		}
		v := submittedSpec.Variables[varKey]
		v.Value = av.Value // no KMS in tests — plaintext
		v.Ref = ""
		submittedSpec.Variables[varKey] = v
	}

	// Verify resolved values
	if v := submittedSpec.Variables["API_KEY"]; v.Value != "sk-real-key" || v.Ref != "" {
		t.Errorf("API_KEY: expected resolved value 'sk-real-key', got value=%q ref=%q", v.Value, v.Ref)
	}
	if v := submittedSpec.Variables["LOG_LEVEL"]; v.Value != "debug" || v.Ref != "" {
		t.Errorf("LOG_LEVEL: expected resolved value 'debug', got value=%q ref=%q", v.Value, v.Ref)
	}
	if v := submittedSpec.Variables["INLINE"]; v.Value != "literal-value" {
		t.Errorf("INLINE: expected untouched value 'literal-value', got %q", v.Value)
	}
}

// TestAccountVars_RefValidation tests that the validator correctly handles ref fields.
func TestAccountVars_RefValidation(t *testing.T) {
	// ref counts as provided — required variable with ref should pass
	t.Run("ref_satisfies_required", func(t *testing.T) {
		ds := minimalDeploySpec()
		ds.Variables = map[string]spec.Variable{
			"MY_KEY": {Ref: "MY_KEY", Targets: []string{"agent"}, Secret: true},
		}
		result, err := deployment.ValidateAndResolve(ds)
		if err != nil {
			t.Fatalf("ValidateAndResolve: %v", err)
		}
		if len(result.Errors) > 0 {
			t.Errorf("expected no errors, got: %v", result.Errors)
		}
	})

	// both value and ref set — should error
	t.Run("value_and_ref_conflict", func(t *testing.T) {
		ds := minimalDeploySpec()
		ds.Variables = map[string]spec.Variable{
			"MY_KEY": {Value: "inline", Ref: "MY_KEY", Targets: []string{"agent"}},
		}
		result, err := deployment.ValidateAndResolve(ds)
		if err != nil {
			t.Fatalf("ValidateAndResolve: %v", err)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected validation error for value+ref conflict")
		}
		found := false
		for _, e := range result.Errors {
			if strings.Contains(e, "cannot set both value and ref") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected 'cannot set both value and ref' error, got: %v", result.Errors)
		}
	})

	// empty required variable without ref — should error
	t.Run("empty_required_no_ref", func(t *testing.T) {
		ds := minimalDeploySpec()
		ds.Variables = map[string]spec.Variable{
			"MY_KEY": {Targets: []string{"agent"}, Secret: true},
		}
		result, err := deployment.ValidateAndResolve(ds)
		if err != nil {
			t.Fatalf("ValidateAndResolve: %v", err)
		}
		if len(result.Errors) == 0 {
			t.Fatal("expected validation error for empty required variable")
		}
	})
}

// TestAccountVars_RefMissing tests that resolution fails when a ref points to a nonexistent account variable.
func TestAccountVars_RefMissing(t *testing.T) {
	db := testDB(t)
	accountStore := account.NewAccountStore(db)
	varsStore := accountvars.NewStore(db)

	acct := createVarsTestAccount(t, accountStore)
	t.Cleanup(func() { _, _ = db.Exec("DELETE FROM accounts WHERE id = $1", acct.ID) })

	// Don't create any account variables — refs should fail to resolve
	refs := map[string]string{
		"API_KEY": "NONEXISTENT_KEY",
	}

	names := []string{"NONEXISTENT_KEY"}
	acctVars, err := varsStore.GetByNames(acct.ID, names)
	if err != nil {
		t.Fatalf("GetByNames: %v", err)
	}

	varMap := make(map[string]*accountvars.AccountVariable, len(acctVars))
	for i := range acctVars {
		varMap[acctVars[i].Name] = &acctVars[i]
	}

	// Check for missing refs
	var missing []string
	for varKey, acctVarName := range refs {
		if _, ok := varMap[acctVarName]; !ok {
			missing = append(missing, varKey+":"+acctVarName)
		}
	}
	if len(missing) == 0 {
		t.Fatal("expected missing ref error")
	}
	if !strings.Contains(missing[0], "NONEXISTENT_KEY") {
		t.Errorf("expected NONEXISTENT_KEY in missing, got %v", missing)
	}
}

// minimalDeploySpec returns a bare-minimum valid deployment spec for validation tests.
func minimalDeploySpec() *spec.AstroDeploymentSpec {
	return &spec.AstroDeploymentSpec{
		Spec: "deployment/v1",
		Source: spec.DeploymentSource{
			Account: "test",
			Name:    "agent",
			Build:   "b1",
		},
		Agent: spec.DeploymentAgent{
			Image: "test:latest",
			Endpoints: map[string]spec.Endpoint{
				"http": {Port: 8080, Protocol: "http"},
			},
		},
	}
}
