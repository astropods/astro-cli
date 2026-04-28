package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// secretTestServer returns a test server that handles variables API calls and
// a setup func that writes credentials pointing at the server.
func secretTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, func()) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	setup := func() {
		t.Helper()
		t.Setenv("HOME", t.TempDir())
		t.Setenv("NO_COLOR", "1")
		writeAccountTestCredentials(t, accountTestCreds("alice"))
		secretsServerURLOverride = srv.URL
		t.Cleanup(func() { secretsServerURLOverride = "" })
	}
	return srv, setup
}

func TestSecretList(t *testing.T) {
	value := "plainvalue"
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, r.URL.Path[len(r.URL.Path)-len("/variables"):], "/variables")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"variables": []map[string]any{
				{"name": "DB_URL", "secret": false, "description": "database url", "value": &value},
				{"name": "API_KEY", "secret": true, "description": ""},
			},
		})
	}

	_, setup := secretTestServer(t, handler)

	t.Run("shows header and type column by default", func(t *testing.T) {
		setup()
		buf := &bytes.Buffer{}
		secretListCmd.SetOut(buf)
		require.NoError(t, runSecretList(secretListCmd, nil))
		out := buf.String()
		require.Contains(t, out, "Updated")
		require.Contains(t, out, "Name")
		require.Contains(t, out, "Type")
		require.Contains(t, out, "DB_URL")
		require.Contains(t, out, "API_KEY")
		require.Contains(t, out, "variable")
		require.Contains(t, out, "secret")
		require.NotContains(t, out, "plainvalue", "values must be hidden by default")
	})

	t.Run("shows VALUE header and values with --show-values", func(t *testing.T) {
		setup()
		require.NoError(t, secretListCmd.Flags().Set("values", "true"))
		t.Cleanup(func() { secretListCmd.Flags().Set("values", "false") }) //nolint:errcheck
		buf := &bytes.Buffer{}
		secretListCmd.SetOut(buf)
		require.NoError(t, runSecretList(secretListCmd, nil))
		out := buf.String()
		require.Contains(t, out, "Value")
		require.NotContains(t, out, "Type")
		require.Contains(t, out, "plainvalue")
		require.Contains(t, out, "******", "secret variables show masked value with --values")
	})
}

func TestSecretList_Empty(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"variables": []any{}}) //nolint:errcheck,gosec
	}
	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretListCmd.SetOut(buf)
	require.NoError(t, runSecretList(secretListCmd, nil))
	require.Contains(t, buf.String(), "No secrets found")
}

func TestSecretList_NotLoggedIn(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	writeAccountTestCredentials(t, &auth.Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*auth.Profile{},
	})
	require.Error(t, runSecretList(secretListCmd, nil))
}

func TestSecretCreate(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound) // variable does not exist yet
			return
		}
		json.NewDecoder(r.Body).Decode(&received) //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"results": []map[string]any{
				{"name": "MY_KEY", "status": "created"},
			},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretCreateCmd.SetOut(buf)
	require.NoError(t, runSecretCreateWithValue(secretCreateCmd, []string{"MY_KEY"}, "s3cret", false, false))

	vars := received["variables"].([]any)
	entry := vars[0].(map[string]any)
	require.Equal(t, "MY_KEY", entry["name"])
	require.Equal(t, "s3cret", entry["value"])
	require.Equal(t, true, entry["secret"])
	require.Contains(t, buf.String(), "Created secret")
}

func TestSecretCreate_Plain(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewDecoder(r.Body).Decode(&received) //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"results": []map[string]any{{"name": "X", "status": "created"}},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretCreateCmd.SetOut(buf)
	require.NoError(t, runSecretCreateWithValue(secretCreateCmd, []string{"XVAL"}, "val", true, false))

	vars := received["variables"].([]any)
	entry := vars[0].(map[string]any)
	require.Equal(t, false, entry["secret"])
	require.Contains(t, buf.String(), "Created variable")
}

func TestSecretCreate_AlreadyExists(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"name": "EXISTING", "secret": false}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	err := runSecretCreateWithValue(secretCreateCmd, []string{"EXISTING"}, "val", false, false)
	require.ErrorContains(t, err, "already exists")
}

func TestSecretCreate_OverwriteFlag(t *testing.T) {
	var postCalled bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postCalled = true
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
				"results": []map[string]any{{"name": "EXISTING", "status": "created"}},
			})
		}
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretCreateCmd.SetOut(buf)
	require.NoError(t, runSecretCreateWithValue(secretCreateCmd, []string{"EXISTING"}, "val", false, true))
	require.True(t, postCalled, "POST should be called when --overwrite is set")
	require.Contains(t, buf.String(), "Created secret")
}

func TestSecretCreate_Description(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		json.NewDecoder(r.Body).Decode(&received) //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"results": []map[string]any{{"name": "MY_KEY", "status": "created"}},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	require.NoError(t, secretCreateCmd.Flags().Set("description", "my description"))
	t.Cleanup(func() { secretCreateCmd.Flags().Set("description", "") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	secretCreateCmd.SetOut(buf)
	require.NoError(t, runSecretCreateWithValue(secretCreateCmd, []string{"MY_KEY"}, "s3cret", false, false))

	vars := received["variables"].([]any)
	entry := vars[0].(map[string]any)
	assert.Equal(t, "my description", entry["description"])
}

func TestSecretNameValidation(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "valid", input: "MY_KEY", wantErr: false},
		{name: "valid with digits", input: "KEY_123", wantErr: false},
		{name: "valid short", input: "KEY", wantErr: false},
		{name: "digit start not allowed", input: "1BAD", wantErr: true},
		{name: "hyphen not allowed", input: "MY-KEY", wantErr: true},
		{name: "space not allowed", input: "MY KEY", wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := spec.ValidateVarName(tc.input)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSecretUpdate(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		require.Contains(t, r.URL.Path, "/MY_KEY")
		json.NewDecoder(r.Body).Decode(&received)                                //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{"message": "variable updated"}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretUpdateCmd.SetOut(buf)
	require.NoError(t, runSecretUpdateWithValue(secretUpdateCmd, []string{"MY_KEY"}, "newval", true, false, false))
	require.Equal(t, "newval", received["value"])
	require.Contains(t, buf.String(), "Updated secret")
}

func TestSecretUpdate_Plain(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		json.NewEncoder(w).Encode(map[string]any{"message": "variable updated"}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretUpdateCmd.SetOut(buf)
	require.NoError(t, runSecretUpdateWithValue(secretUpdateCmd, []string{"MY_VAR"}, "newval", false, false, false))
	require.Contains(t, buf.String(), "Updated variable")
}

func TestSecretUpdate_PlainFlag(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPut, r.Method)
		json.NewDecoder(r.Body).Decode(&received)                                //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{"message": "variable updated"}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretUpdateCmd.SetOut(buf)
	require.NoError(t, runSecretUpdateWithValue(secretUpdateCmd, []string{"MY_KEY"}, "newval", false, true, false))
	require.Equal(t, false, received["secret"])
	require.Contains(t, buf.String(), "Updated variable")
}

func TestSecretUpdate_NotFound(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "variable not found"}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	err := runSecretUpdateWithValue(secretUpdateCmd, []string{"MISSING"}, "v", true, false, false)
	require.ErrorContains(t, err, "not found")
}

func TestSecretGet(t *testing.T) {
	value := "postgres://localhost/db"
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Contains(t, r.URL.Path, "/variables/DB_URL")
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"name": "DB_URL", "secret": false, "description": "database url",
			"value": value, "created_at": "2026-01-15T10:00:00Z", "updated_at": "2026-01-15T10:30:00Z",
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretGetCmd.SetOut(buf)
	require.NoError(t, runSecretGet(secretGetCmd, []string{"DB_URL"}))
	out := buf.String()
	require.Contains(t, out, "DB_URL")
	require.Contains(t, out, "database url")
	require.Contains(t, out, "postgres://localhost/db")
	require.Contains(t, out, "2026-01-15")
}

func TestSecretGet_Secret(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"name": "API_KEY", "secret": true, "description": "",
			"created_at": "2026-01-15T10:00:00Z", "updated_at": "2026-01-15T10:30:00Z",
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretGetCmd.SetOut(buf)
	require.NoError(t, runSecretGet(secretGetCmd, []string{"API_KEY"}))
	out := buf.String()
	require.Contains(t, out, "API_KEY")
	require.Contains(t, out, "******")
}

func TestSecretDelete(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodDelete, r.Method)
		require.Contains(t, r.URL.Path, "/MY_KEY")
		json.NewEncoder(w).Encode(map[string]any{"message": "variable deleted"}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	buf := &bytes.Buffer{}
	secretDeleteCmd.SetOut(buf)
	require.NoError(t, runSecretDelete(secretDeleteCmd, []string{"MY_KEY"}))
	require.Contains(t, buf.String(), "Deleted secret")
}

func TestSecretDelete_NotFound(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	_, setup := secretTestServer(t, handler)
	setup()

	require.ErrorContains(t, runSecretDelete(secretDeleteCmd, []string{"GONE"}), "not found")
}

func TestSecretImport_PlainKeys(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"variables": []any{}}) //nolint:errcheck,gosec
			return
		}
		json.NewDecoder(r.Body).Decode(&received) //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"results": []map[string]any{
				{"name": "API_KEY", "status": "created"},
				{"name": "DB_URL", "status": "created"},
			},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	envContent := "API_KEY=sk-123\nDB_URL=postgres://localhost/db\n"
	f := t.TempDir() + "/test.env"
	require.NoError(t, os.WriteFile(f, []byte(envContent), 0600))

	require.NoError(t, secretImportCmd.Flags().Set("plain-keys", "DB_URL"))
	t.Cleanup(func() { secretImportCmd.Flags().Set("plain-keys", "") }) //nolint:errcheck
	require.NoError(t, secretImportCmd.Flags().Set("file", f))
	t.Cleanup(func() { secretImportCmd.Flags().Set("file", "") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	secretImportCmd.SetOut(buf)
	require.NoError(t, runSecretImport(secretImportCmd, nil))

	vars := received["variables"].([]any)
	require.Len(t, vars, 2)
	byName := map[string]map[string]any{}
	for _, v := range vars {
		m := v.(map[string]any)
		byName[m["name"].(string)] = m
	}
	require.Equal(t, true, byName["API_KEY"]["secret"])
	require.Equal(t, false, byName["DB_URL"]["secret"])

	out := buf.String()
	require.Contains(t, out, "Imported secret")
	require.Contains(t, out, "Imported variable")
}

func TestSecretImport_Plain(t *testing.T) {
	var received map[string]any
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{"variables": []any{}}) //nolint:errcheck,gosec
			return
		}
		json.NewDecoder(r.Body).Decode(&received) //nolint:errcheck,gosec
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"results": []map[string]any{
				{"name": "API_KEY", "status": "created"},
				{"name": "DB_URL", "status": "created"},
			},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	envContent := "API_KEY=sk-123\nDB_URL=postgres://localhost/db\n"
	f := t.TempDir() + "/test.env"
	require.NoError(t, os.WriteFile(f, []byte(envContent), 0600))

	require.NoError(t, secretImportCmd.Flags().Set("plain", "true"))
	t.Cleanup(func() { secretImportCmd.Flags().Set("plain", "false") }) //nolint:errcheck,gosec
	require.NoError(t, secretImportCmd.Flags().Set("file", f))
	t.Cleanup(func() { secretImportCmd.Flags().Set("file", "") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	secretImportCmd.SetOut(buf)
	require.NoError(t, runSecretImport(secretImportCmd, nil))

	vars := received["variables"].([]any)
	byName := map[string]map[string]any{}
	for _, v := range vars {
		m := v.(map[string]any)
		byName[m["name"].(string)] = m
	}
	require.Equal(t, false, byName["API_KEY"]["secret"])
	require.Equal(t, false, byName["DB_URL"]["secret"])

	out := buf.String()
	require.NotContains(t, out, "Imported secret")
	require.Contains(t, out, "Imported variable")
}

func TestSecretImport_SkipsExisting(t *testing.T) {
	postCalled := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
				"variables": []map[string]any{{"name": "EXISTING"}},
			})
			return
		}
		postCalled = true
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"results": []map[string]any{{"name": "NEW_KEY", "status": "created"}},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	envContent := "EXISTING=old\nNEW_KEY=val\n"
	f := t.TempDir() + "/test.env"
	require.NoError(t, os.WriteFile(f, []byte(envContent), 0600))

	require.NoError(t, secretImportCmd.Flags().Set("file", f))
	t.Cleanup(func() { secretImportCmd.Flags().Set("file", "") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	secretImportCmd.SetOut(buf)
	require.NoError(t, runSecretImport(secretImportCmd, nil))

	require.True(t, postCalled, "should still POST for NEW_KEY")
	require.Contains(t, buf.String(), "Skipped")
	require.Contains(t, buf.String(), "EXISTING")
}

func TestSecretImport_OverwriteFlag(t *testing.T) {
	var listCalled bool
	handler := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			listCalled = true
		}
		json.NewEncoder(w).Encode(map[string]any{ //nolint:errcheck,gosec
			"variables": []any{},
			"results":   []map[string]any{{"name": "KEY", "status": "created"}},
		})
	}

	_, setup := secretTestServer(t, handler)
	setup()

	f := t.TempDir() + "/test.env"
	require.NoError(t, os.WriteFile(f, []byte("KEY=val\n"), 0600))

	require.NoError(t, secretImportCmd.Flags().Set("overwrite", "true"))
	t.Cleanup(func() { secretImportCmd.Flags().Set("overwrite", "false") }) //nolint:errcheck
	require.NoError(t, secretImportCmd.Flags().Set("file", f))
	t.Cleanup(func() { secretImportCmd.Flags().Set("file", "") }) //nolint:errcheck

	secretImportCmd.SetOut(&bytes.Buffer{})
	require.NoError(t, runSecretImport(secretImportCmd, nil))
	require.False(t, listCalled, "should not fetch existing when --overwrite is set")
}

func TestSecretImport_SkipsBlankValues(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"variables": []any{}}) //nolint:errcheck,gosec
	}

	_, setup := secretTestServer(t, handler)
	setup()

	f := t.TempDir() + "/test.env"
	require.NoError(t, os.WriteFile(f, []byte("EMPTY=\nALSO_EMPTY=   \n# comment\n\n"), 0600))
	require.NoError(t, secretImportCmd.Flags().Set("file", f))
	t.Cleanup(func() { secretImportCmd.Flags().Set("file", "") }) //nolint:errcheck

	buf := &bytes.Buffer{}
	secretImportCmd.SetOut(buf)
	require.NoError(t, runSecretImport(secretImportCmd, nil))
	require.Contains(t, buf.String(), "No variables to import")
}
