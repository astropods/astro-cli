package sandbox

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSandboxes struct {
	instances map[string]Instance
	records   []Record

	ensured  []string
	tokens   []string
	removed  []string
	ensedErr error
	listErr  error
}

func newFakeSandboxes() *fakeSandboxes {
	return &fakeSandboxes{instances: map[string]Instance{}}
}

func (f *fakeSandboxes) Ensure(_ context.Context, name, token string) (Instance, error) {
	f.ensured = append(f.ensured, name)
	f.tokens = append(f.tokens, token)
	if f.ensedErr != nil {
		return Instance{}, f.ensedErr
	}
	if inst, ok := f.instances[name]; ok {
		return inst, nil
	}
	inst := Instance{
		Name:     name,
		Host:     hostFor(name),
		Endpoint: "http://" + hostFor(name) + ":49983",
		Started:  true,
	}
	f.instances[name] = Instance{Name: name, Host: inst.Host, Endpoint: inst.Endpoint}
	return inst, nil
}

func (f *fakeSandboxes) Remove(_ context.Context, name string) error {
	f.removed = append(f.removed, name)
	delete(f.instances, name)
	return nil
}

func (f *fakeSandboxes) List(context.Context) ([]Record, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.records, nil
}

func newTestServer(t *testing.T) (http.Handler, *fakeSandboxes, string) {
	t.Helper()
	fake := newFakeSandboxes()
	secret, err := NewSecret()
	require.NoError(t, err)
	srv := NewServer(fake, secret, slog.New(slog.DiscardHandler))
	token, err := SignToken("local", "http://host.docker.internal:3199", secret)
	require.NoError(t, err)
	return srv.Handler(), fake, token
}

func do(t *testing.T, h http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestAttachReturnsAHandleWithTheFieldsTheSDKReads(t *testing.T) {
	h, _, token := newTestServer(t)

	rec := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var keys map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &keys))

	got := make([]string, 0, len(keys))
	for k := range keys {
		got = append(got, k)
	}
	sort.Strings(got)
	assert.Equal(t, []string{"class", "endpoint", "expires_at", "headers", "name", "state"}, got,
		"astro-server's SandboxHandleResponse has exactly these fields, and the SDK reads them")

	var handle HandleResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &handle))
	assert.Equal(t, "conv-1", handle.Name)
	assert.Equal(t, "running", handle.State)
	assert.Equal(t, DefaultClass, handle.Class)
	assert.Contains(t, handle.Endpoint, ":49983")
	assert.NotEmpty(t, handle.Headers[sessionTokenHeader],
		"the agent needs the session token to reach the data plane")
}

func TestAttachIsIdempotentByNameAndKeepsTheSameSessionToken(t *testing.T) {
	h, fake, token := newTestServer(t)

	first := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)
	require.Equal(t, http.StatusOK, first.Code)
	second := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)
	require.Equal(t, http.StatusOK, second.Code)

	var a, b HandleResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &b))

	assert.Equal(t, a.Endpoint, b.Endpoint)
	assert.Equal(t, a.Headers[sessionTokenHeader], b.Headers[sessionTokenHeader],
		"the run hook installs a token once, so a second attach must return the one the sandbox accepts")
	assert.Equal(t, []string{"conv-1", "conv-1"}, fake.ensured)
}

func TestTwoNamesGetTwoSandboxes(t *testing.T) {
	h, _, token := newTestServer(t)

	first := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)
	second := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-2", token)

	var a, b HandleResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &b))

	assert.NotEqual(t, a.Endpoint, b.Endpoint, "one sandbox per conversation, not one per agent")
	assert.NotEqual(t, a.Headers[sessionTokenHeader], b.Headers[sessionTokenHeader],
		"one sandbox's token must not reach another")
}

func TestAttachReportsAFailureToBringTheSandboxUp(t *testing.T) {
	h, fake, token := newTestServer(t)
	fake.ensedErr = ErrNotReady

	rec := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)
	assert.Equal(t, http.StatusBadGateway, rec.Code,
		"astro-server maps a launch failure to 502, so the SDK sees one status in both places")
}

func TestGetReturnsTheRecordAndListSortsNewestFirst(t *testing.T) {
	h, fake, token := newTestServer(t)
	now := time.Now()
	fake.records = []Record{
		{Name: "conv-old", State: "running", CreatedAt: now.Add(-time.Hour)},
		{Name: "conv-new", State: "running", CreatedAt: now},
	}

	rec := do(t, h, http.MethodGet, "/api/v1/sandboxes/conv-old", token)
	require.Equal(t, http.StatusOK, rec.Code)
	var record RecordResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &record))
	assert.Equal(t, "conv-old", record.Name)
	assert.NotEmpty(t, record.CreatedAt)

	rec = do(t, h, http.MethodGet, "/api/v1/sandboxes", token)
	require.Equal(t, http.StatusOK, rec.Code)
	var list ListResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &list))
	require.Len(t, list.Sandboxes, 2)
	assert.Equal(t, "conv-new", list.Sandboxes[0].Name, "astro-server lists newest first")
}

func TestGetReportsAnUnknownNameAsNotFound(t *testing.T) {
	h, _, token := newTestServer(t)

	rec := do(t, h, http.MethodGet, "/api/v1/sandboxes/conv-missing", token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeleteRemovesTheSandboxAndReturnsNoContent(t *testing.T) {
	h, fake, token := newTestServer(t)

	require.Equal(t, http.StatusOK, do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token).Code)
	rec := do(t, h, http.MethodDelete, "/api/v1/sandboxes/conv-1", token)

	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"conv-1"}, fake.removed)
}

func TestDeleteForgetsTheSessionTokenSoAFreshSandboxGetsAFreshOne(t *testing.T) {
	h, _, token := newTestServer(t)

	first := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)
	require.Equal(t, http.StatusNoContent, do(t, h, http.MethodDelete, "/api/v1/sandboxes/conv-1", token).Code)
	second := do(t, h, http.MethodPut, "/api/v1/sandboxes/conv-1", token)

	var a, b HandleResponse
	require.NoError(t, json.Unmarshal(first.Body.Bytes(), &a))
	require.NoError(t, json.Unmarshal(second.Body.Bytes(), &b))
	assert.NotEqual(t, a.Headers[sessionTokenHeader], b.Headers[sessionTokenHeader],
		"a deleted sandbox's token must not authorize the replacement")
}

func TestStopDropsTheSandbox(t *testing.T) {
	h, fake, token := newTestServer(t)

	rec := do(t, h, http.MethodPost, "/api/v1/sandboxes/conv-1/stop", token)
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"conv-1"}, fake.removed,
		"a container has no suspend that keeps a session, so stop drops it")
}

func TestEveryRouteRefusesAMissingOrInvalidToken(t *testing.T) {
	h, _, _ := newTestServer(t)
	otherSecretToken, err := SignToken("local", "http://elsewhere", otherSecret)
	require.NoError(t, err)

	for _, route := range []struct{ method, path string }{
		{http.MethodPut, "/api/v1/sandboxes/conv-1"},
		{http.MethodGet, "/api/v1/sandboxes"},
		{http.MethodGet, "/api/v1/sandboxes/conv-1"},
		{http.MethodDelete, "/api/v1/sandboxes/conv-1"},
		{http.MethodPost, "/api/v1/sandboxes/conv-1/stop"},
	} {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			missing := do(t, h, route.method, route.path, "")
			assert.Equal(t, http.StatusUnauthorized, missing.Code)
			assert.Contains(t, missing.Body.String(), "missing deploy token")

			invalid := do(t, h, route.method, route.path, otherSecretToken)
			assert.Equal(t, http.StatusUnauthorized, invalid.Code)
			assert.Contains(t, invalid.Body.String(), "invalid deploy token")
		})
	}
}
