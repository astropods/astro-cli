package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/astropods/astro/apps/astro-registry/internal/auth"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/gin-gonic/gin"
)

// stubIdPValidator hands back a canned set of claims, or an error.
type stubIdPValidator struct {
	claims *auth.JWTClaims
	err    error
}

func (s *stubIdPValidator) ValidateToken(_ context.Context, _ string) (*auth.JWTClaims, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.claims, nil
}

// stubResolver answers membership queries from a static map.
type stubResolver struct {
	members map[string]string // account name → account ID. Membership = key present.
	err     error
}

func (s *stubResolver) IsMemberWithID(accountName, _ string) (bool, string, error) {
	if s.err != nil {
		return false, "", s.err
	}
	id, ok := s.members[accountName]
	if !ok {
		return false, "", nil
	}
	return true, id, nil
}

func newTestTokenHandler(t *testing.T, idp IdPValidator, mc AccountResolver) (gin.HandlerFunc, *auth.RegistryTokenSigner) {
	t.Helper()
	signer := auth.NewRegistryTokenSigner("test-secret", "astro-registry", "astro-registry", time.Hour)
	cfg := TokenHandlerConfig{
		Logger:            logger.New("error", "text"),
		WorkOSValidator:   idp,
		Signer:            signer,
		MembershipChecker: mc,
		Service:           "astro-registry",
	}
	return Token(cfg), signer
}

func doTokenRequest(handler gin.HandlerFunc, query url.Values, basicUser, basicPass string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/token?"+query.Encode(), nil)
	if basicUser != "" || basicPass != "" {
		req.SetBasicAuth(basicUser, basicPass)
	}
	c.Request = req
	handler(c)
	return rec
}

func TestToken_RejectsMissingBasicAuth(t *testing.T) {
	t.Parallel()
	handler, _ := newTestTokenHandler(t, &stubIdPValidator{}, &stubResolver{})
	q := url.Values{"service": {"astro-registry"}}

	rec := doTokenRequest(handler, q, "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
	if !strings.HasPrefix(rec.Header().Get("WWW-Authenticate"), "Basic ") {
		t.Errorf("expected Basic challenge header, got %q", rec.Header().Get("WWW-Authenticate"))
	}
}

func TestToken_RejectsInvalidIdPCredential(t *testing.T) {
	t.Parallel()
	idp := &stubIdPValidator{err: errors.New("expired or invalid")}
	handler, _ := newTestTokenHandler(t, idp, &stubResolver{})

	q := url.Values{
		"service": {"astro-registry"},
		"scope":   {"repository:saswatds/myapp:push,pull"},
	}
	rec := doTokenRequest(handler, q, "token", "garbage")
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestToken_RejectsWrongService(t *testing.T) {
	t.Parallel()
	idp := &stubIdPValidator{claims: &auth.JWTClaims{}}
	idp.claims.Subject = "user_123"
	handler, _ := newTestTokenHandler(t, idp, &stubResolver{})

	q := url.Values{"service": {"some-other-registry"}}
	rec := doTokenRequest(handler, q, "token", "ok")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rec.Code)
	}
}

func TestToken_IssuesTokenWithGrantedScope(t *testing.T) {
	t.Parallel()
	claims := &auth.JWTClaims{}
	claims.Subject = "user_123"
	idp := &stubIdPValidator{claims: claims}
	mc := &stubResolver{members: map[string]string{"saswatds": "01kggdgfrw46qcsnxeqbr1hr1z"}}
	handler, signer := newTestTokenHandler(t, idp, mc)

	q := url.Values{
		"service": {"astro-registry"},
		"scope":   {"repository:saswatds/myapp:push,pull"},
	}
	rec := doTokenRequest(handler, q, "token", "workos-jwt")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected non-empty token")
	}
	if resp.AccessToken != resp.Token {
		t.Error("access_token must alias token for OAuth2 compat")
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("expected expires_in=3600, got %d", resp.ExpiresIn)
	}

	// Verify scope and account_id were embedded.
	parsed, err := signer.Verify(resp.Token)
	if err != nil {
		t.Fatalf("verify minted token: %v", err)
	}
	if len(parsed.Access) != 1 {
		t.Fatalf("expected 1 access entry, got %d", len(parsed.Access))
	}
	if parsed.Access[0].Name != "saswatds/myapp" {
		t.Errorf("expected access name saswatds/myapp, got %q", parsed.Access[0].Name)
	}
	if parsed.Access[0].AccountID != "01kggdgfrw46qcsnxeqbr1hr1z" {
		t.Errorf("expected account_id embedded, got %q", parsed.Access[0].AccountID)
	}
	if !parsed.HasAccess("saswatds/myapp", "push") || !parsed.HasAccess("saswatds/myapp", "pull") {
		t.Errorf("expected push,pull granted, got %v", parsed.Access[0].Actions)
	}
}

func TestToken_DropsScopeForNonMember(t *testing.T) {
	t.Parallel()
	claims := &auth.JWTClaims{}
	claims.Subject = "user_999"
	idp := &stubIdPValidator{claims: claims}
	// User is not a member of "someoneelse".
	mc := &stubResolver{members: map[string]string{}}
	handler, signer := newTestTokenHandler(t, idp, mc)

	q := url.Values{
		"service": {"astro-registry"},
		"scope":   {"repository:someoneelse/private:push,pull"},
	}
	rec := doTokenRequest(handler, q, "token", "workos")

	// Spec: server returns intersection (empty), never an error for unauthorized scopes.
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with empty access, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp TokenResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	parsed, err := signer.Verify(resp.Token)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if len(parsed.Access) != 0 {
		t.Errorf("expected empty access, got %v", parsed.Access)
	}
}

func TestToken_OrgPermissionFiltering(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		permissions []string
		want        []string
	}{
		{"read-only org user gets pull only", []string{"agents:read"}, []string{"pull"}},
		{"writer gets push and pull", []string{"agents:read", "agents:write"}, []string{"push", "pull"}},
		{"no perms gets dropped scope", []string{}, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims := &auth.JWTClaims{
				OrganizationID: "org_123",
				Permissions:    tc.permissions,
			}
			claims.Subject = "user_123"
			idp := &stubIdPValidator{claims: claims}
			mc := &stubResolver{members: map[string]string{"myorg": "acc_uuid"}}
			handler, signer := newTestTokenHandler(t, idp, mc)

			q := url.Values{
				"service": {"astro-registry"},
				"scope":   {"repository:myorg/img:push,pull"},
			}
			rec := doTokenRequest(handler, q, "token", "workos")
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", rec.Code)
			}
			var resp TokenResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			parsed, err := signer.Verify(resp.Token)
			if err != nil {
				t.Fatalf("verify: %v", err)
			}

			if tc.want == nil {
				if len(parsed.Access) != 0 {
					t.Errorf("expected no access, got %v", parsed.Access)
				}
				return
			}
			if len(parsed.Access) != 1 {
				t.Fatalf("expected 1 access entry, got %d", len(parsed.Access))
			}
			gotActions := parsed.Access[0].Actions
			if !sameSet(gotActions, tc.want) {
				t.Errorf("expected actions %v, got %v", tc.want, gotActions)
			}
		})
	}
}

func TestToken_PersonalAccountSkipsPermissionCheck(t *testing.T) {
	t.Parallel()

	// Personal accounts have OrganizationID == "" — membership alone is sufficient.
	claims := &auth.JWTClaims{}
	claims.Subject = "user_123"
	idp := &stubIdPValidator{claims: claims}
	mc := &stubResolver{members: map[string]string{"saswatds": "acc"}}
	handler, signer := newTestTokenHandler(t, idp, mc)

	q := url.Values{
		"service": {"astro-registry"},
		"scope":   {"repository:saswatds/myapp:push,pull"},
	}
	rec := doTokenRequest(handler, q, "token", "workos")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp TokenResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	parsed, _ := signer.Verify(resp.Token)
	if !parsed.HasAccess("saswatds/myapp", "push") {
		t.Error("personal-account user should be granted push")
	}
}

func TestToken_DropsMalformedScope(t *testing.T) {
	t.Parallel()
	claims := &auth.JWTClaims{}
	claims.Subject = "user_123"
	idp := &stubIdPValidator{claims: claims}
	mc := &stubResolver{members: map[string]string{"saswatds": "acc"}}
	handler, signer := newTestTokenHandler(t, idp, mc)

	q := url.Values{
		"service": {"astro-registry"},
		"scope":   {"garbage", "repository:saswatds/myapp:push,pull"},
	}
	rec := doTokenRequest(handler, q, "token", "workos")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var resp TokenResponse
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	parsed, _ := signer.Verify(resp.Token)
	if len(parsed.Access) != 1 {
		t.Errorf("expected 1 valid scope (malformed dropped), got %d", len(parsed.Access))
	}
}

func TestParseScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in     string
		want   auth.ResourceAccess
		wantOK bool
	}{
		{"repository:ns/img:push,pull",
			auth.ResourceAccess{Type: "repository", Name: "ns/img", Actions: []string{"push", "pull"}},
			true},
		{"repository:ns/img:pull",
			auth.ResourceAccess{Type: "repository", Name: "ns/img", Actions: []string{"pull"}},
			true},
		{"repository:ns/img:push, pull",
			auth.ResourceAccess{Type: "repository", Name: "ns/img", Actions: []string{"push", "pull"}},
			true},
		{"repository:ns/img:bogus", auth.ResourceAccess{}, false},
		{"repository::push", auth.ResourceAccess{}, false},
		{"::", auth.ResourceAccess{}, false},
		{"garbage", auth.ResourceAccess{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := parseScope(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if !ok {
				return
			}
			if got.Type != tc.want.Type || got.Name != tc.want.Name {
				t.Errorf("got %+v, want %+v", got, tc.want)
			}
			if !sameSet(got.Actions, tc.want.Actions) {
				t.Errorf("actions: got %v, want %v", got.Actions, tc.want.Actions)
			}
		})
	}
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}
