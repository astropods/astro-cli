package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zalando/go-keyring"
)

const testOrgID = "org_a"

// fakeTokenEndpoint accepts each refresh token once and rotates it on every exchange.
type fakeTokenEndpoint struct {
	*httptest.Server

	lifetime time.Duration
	delay    time.Duration

	mu         sync.Mutex
	current    string
	requests   int
	exchanges  int
	issued     string
	onReject   func()
	onExchange func()
}

func newFakeTokenEndpoint(t *testing.T, refreshToken string, lifetime time.Duration) *fakeTokenEndpoint {
	t.Helper()
	f := &fakeTokenEndpoint{current: refreshToken, lifetime: lifetime}
	f.Server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.Close)
	return f
}

func (f *fakeTokenEndpoint) serve(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	f.mu.Lock()
	defer f.mu.Unlock()

	f.requests++
	if r.PostForm.Get("refresh_token") != f.current {
		if f.onReject != nil {
			f.onReject()
		}
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(TokenError{Error: "invalid_grant", ErrorDescription: "Refresh token already exchanged."})
		return
	}

	if f.onExchange != nil {
		f.onExchange()
	}
	// Hold the old token valid long enough for concurrent callers to read it.
	time.Sleep(f.delay)
	f.exchanges++
	f.current = fmt.Sprintf("refresh_%d", f.exchanges)
	f.issued = makeScopedTestJWT(time.Now().Add(f.lifetime), r.PostForm.Get("organization_id"), f.exchanges)
	_ = json.NewEncoder(w).Encode(TokenResponse{
		AccessToken:  f.issued,
		RefreshToken: f.current,
		ExpiresIn:    int(f.lifetime.Seconds()),
		TokenType:    "Bearer",
	})
}

func (f *fakeTokenEndpoint) exchangeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exchanges
}

func (f *fakeTokenEndpoint) lastIssued() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.issued
}

func (f *fakeTokenEndpoint) requestCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func (f *fakeTokenEndpoint) manager() *TokenManager {
	return &TokenManager{storage: createTestStorage(), client: createTestClient(f.URL)}
}

func makeScopedTestJWT(exp time.Time, orgID string, n int) string {
	payload, err := json.Marshal(map[string]any{"exp": exp.Unix(), "org_id": orgID, "n": n})
	if err != nil {
		panic(err)
	}
	return "header." + base64.RawURLEncoding.EncodeToString(payload) + ".sig"
}

func setupRefreshTest(t *testing.T, profile *Profile) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvAccessToken, "")
	resetEnvToken()
	t.Cleanup(resetEnvToken)
	writeTokenTestCredentials(t, &Credentials{
		CurrentProfile: "default",
		Profiles:       map[string]*Profile{"default": profile},
	})
}

func loadTestProfile(t *testing.T) *Profile {
	t.Helper()
	profile, err := createTestStorage().GetCurrentProfile()
	require.NoError(t, err)
	return profile
}

func TestTokenManager_ConcurrentRefreshesShareOneExchange(t *testing.T) {
	tests := []struct {
		name    string
		profile *Profile
		get     func(context.Context, *TokenManager) (string, error)
	}{
		{
			name: "org-scoped token",
			profile: &Profile{
				AccessToken:  makeTestJWT(time.Now().Add(time.Hour)),
				RefreshToken: "refresh_0",
				ExpiresAt:    time.Now().Add(time.Hour),
			},
			get: func(ctx context.Context, m *TokenManager) (string, error) {
				return m.GetOrgScopedAccessToken(ctx, testOrgID)
			},
		},
		{
			name: "personal token near expiry",
			profile: &Profile{
				AccessToken:  makeTestJWT(time.Now().Add(time.Minute)),
				RefreshToken: "refresh_0",
				ExpiresAt:    time.Now().Add(time.Minute),
			},
			get: func(ctx context.Context, m *TokenManager) (string, error) {
				return m.GetValidAccessToken(ctx)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupRefreshTest(t, tt.profile)
			endpoint := newFakeTokenEndpoint(t, "refresh_0", 15*time.Minute)
			endpoint.delay = 50 * time.Millisecond

			const callers = 8
			tokens := make([]string, callers)
			errs := make([]error, callers)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range callers {
				m := endpoint.manager()
				wg.Go(func() {
					<-start
					tokens[i], errs[i] = tt.get(context.Background(), m)
				})
			}
			close(start)
			wg.Wait()

			for i := range callers {
				require.NoError(t, errs[i], "caller %d", i)
				assert.Equal(t, tokens[0], tokens[i], "caller %d", i)
			}
			assert.Equal(t, 1, endpoint.exchangeCount())
			assert.Equal(t, "refresh_1", loadTestProfile(t).RefreshToken)
		})
	}
}

func TestGetOrgScopedAccessToken_Cache(t *testing.T) {
	fresh := makeScopedTestJWT(time.Now().Add(time.Hour), testOrgID, 0)
	nearExpiry := makeScopedTestJWT(time.Now().Add(2*time.Minute), testOrgID, 0)

	tests := []struct {
		name          string
		cached        map[string]*OrgToken
		force         bool
		wantCached    bool
		wantExchanges int
	}{
		{
			name:       "fresh token is reused",
			cached:     map[string]*OrgToken{testOrgID: {AccessToken: fresh, ExpiresAt: time.Now().Add(time.Hour)}},
			wantCached: true,
		},
		{
			name:          "token near expiry is renewed",
			cached:        map[string]*OrgToken{testOrgID: {AccessToken: nearExpiry, ExpiresAt: time.Now().Add(2 * time.Minute)}},
			wantExchanges: 1,
		},
		{
			name:          "token whose JWT is near expiry is renewed",
			cached:        map[string]*OrgToken{testOrgID: {AccessToken: nearExpiry, ExpiresAt: time.Now().Add(time.Hour)}},
			wantExchanges: 1,
		},
		{
			name:          "token for another organization is not used",
			cached:        map[string]*OrgToken{"org_b": {AccessToken: fresh, ExpiresAt: time.Now().Add(time.Hour)}},
			wantExchanges: 1,
		},
		{
			name:          "nothing cached",
			wantExchanges: 1,
		},
		{
			name:          "force bypasses a fresh token",
			cached:        map[string]*OrgToken{testOrgID: {AccessToken: fresh, ExpiresAt: time.Now().Add(time.Hour)}},
			force:         true,
			wantExchanges: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupRefreshTest(t, &Profile{
				AccessToken:  makeTestJWT(time.Now().Add(time.Hour)),
				RefreshToken: "refresh_0",
				ExpiresAt:    time.Now().Add(time.Hour),
				OrgTokens:    tt.cached,
			})
			endpoint := newFakeTokenEndpoint(t, "refresh_0", 15*time.Minute)
			m := endpoint.manager()

			var token string
			var err error
			if tt.force {
				token, err = m.ForceRefreshOrgScopedAccessToken(context.Background(), testOrgID)
			} else {
				token, err = m.GetOrgScopedAccessToken(context.Background(), testOrgID)
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantExchanges, endpoint.exchangeCount())

			stored := loadTestProfile(t)
			if tt.wantCached {
				assert.Equal(t, fresh, token)
				assert.Equal(t, "refresh_0", stored.RefreshToken)
				return
			}
			assert.Equal(t, endpoint.lastIssued(), token)
			assert.Equal(t, "refresh_1", stored.RefreshToken)
			require.Contains(t, stored.OrgTokens, testOrgID)
			assert.Equal(t, token, stored.OrgTokens[testOrgID].AccessToken)
			assert.WithinDuration(t, time.Now().Add(15*time.Minute), stored.OrgTokens[testOrgID].ExpiresAt, time.Minute)

			again, err := endpoint.manager().GetOrgScopedAccessToken(context.Background(), testOrgID)
			require.NoError(t, err)
			assert.Equal(t, token, again, "a second command must reuse the saved token")
			assert.Equal(t, tt.wantExchanges, endpoint.exchangeCount())
		})
	}
}

func TestTokenManager_RetriesRejectedGrantWithRotatedToken(t *testing.T) {
	tests := []struct {
		name    string
		get     func(context.Context, *TokenManager) (string, error)
		wantErr func(*testing.T, error)
	}{
		{
			name: "org-scoped token",
			get: func(ctx context.Context, m *TokenManager) (string, error) {
				return m.GetOrgScopedAccessToken(ctx, testOrgID)
			},
			wantErr: func(t *testing.T, err error) {
				assert.EqualError(t, err, "org-scoped token refresh failed: invalid_grant - Refresh token already exchanged.")
			},
		},
		{
			name: "personal token",
			get: func(ctx context.Context, m *TokenManager) (string, error) {
				return m.GetValidAccessToken(ctx)
			},
			wantErr: func(t *testing.T, err error) {
				assert.ErrorIs(t, err, ErrSessionEnded)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name+": rotated on disk", func(t *testing.T) {
			setupRefreshTest(t, &Profile{
				AccessToken:  makeTestJWT(time.Now().Add(time.Minute)),
				RefreshToken: "refresh_stale",
				ExpiresAt:    time.Now().Add(time.Minute),
			})
			endpoint := newFakeTokenEndpoint(t, "refresh_live", 15*time.Minute)
			// Simulates a process that rotated the token without taking the lock.
			endpoint.onReject = func() {
				storage := createTestStorage()
				creds, err := storage.LoadCredentials()
				if assert.NoError(t, err) {
					creds.Profiles["default"].RefreshToken = "refresh_live"
					assert.NoError(t, storage.SaveCredentials(creds))
				}
			}

			token, err := tt.get(context.Background(), endpoint.manager())
			require.NoError(t, err)
			assert.NotEmpty(t, token)
			assert.Equal(t, 2, endpoint.requestCount())
			assert.Equal(t, "refresh_1", loadTestProfile(t).RefreshToken)
		})

		t.Run(tt.name+": unchanged on disk", func(t *testing.T) {
			setupRefreshTest(t, &Profile{
				AccessToken:  makeTestJWT(time.Now().Add(time.Minute)),
				RefreshToken: "refresh_stale",
				ExpiresAt:    time.Now().Add(time.Minute),
			})
			endpoint := newFakeTokenEndpoint(t, "refresh_live", 15*time.Minute)

			_, err := tt.get(context.Background(), endpoint.manager())
			require.Error(t, err)
			tt.wantErr(t, err)
			assert.Equal(t, 1, endpoint.requestCount(), "the same refresh token must not be sent twice")
			assert.Equal(t, "refresh_stale", loadTestProfile(t).RefreshToken)
		})
	}
}

func TestStorage_WriteDuringRefreshKeepsBothChanges(t *testing.T) {
	setupRefreshTest(t, &Profile{
		AccessToken:  makeTestJWT(time.Now().Add(time.Hour)),
		RefreshToken: "refresh_0",
		ExpiresAt:    time.Now().Add(time.Hour),
	})
	endpoint := newFakeTokenEndpoint(t, "refresh_0", 15*time.Minute)
	endpoint.delay = 100 * time.Millisecond
	accounts := []StoredAccount{{Name: "acme", OrganizationID: testOrgID}}
	saved := make(chan error, 1)
	endpoint.onExchange = func() {
		go func() { saved <- createTestStorage().SetAccounts(accounts) }()
	}

	_, err := endpoint.manager().GetOrgScopedAccessToken(context.Background(), testOrgID)
	require.NoError(t, err)
	require.NoError(t, <-saved)

	stored := loadTestProfile(t)
	assert.Equal(t, "refresh_1", stored.RefreshToken, "a write must not restore the spent refresh token")
	assert.Equal(t, accounts, stored.Accounts, "the refresh must not drop the other write")
}

func TestGetOrgScopedAccessToken_KeyringReadsOnlyTheRequestedOrg(t *testing.T) {
	setupRefreshTest(t, &Profile{})
	keyring.MockInit()
	storage := &Storage{binaryName: "ast", useKeyring: true}
	tokenA := makeScopedTestJWT(time.Now().Add(time.Hour), "org_a", 0)
	tokenB := makeScopedTestJWT(time.Now().Add(time.Hour), "org_b", 0)
	require.NoError(t, storage.SaveCredentials(&Credentials{
		CurrentProfile: "default",
		Profiles: map[string]*Profile{"default": {
			AccessToken:  makeTestJWT(time.Now().Add(time.Hour)),
			RefreshToken: "refresh_0",
			ExpiresAt:    time.Now().Add(time.Hour),
			OrgTokens: map[string]*OrgToken{
				"org_a": {AccessToken: tokenA, ExpiresAt: time.Now().Add(time.Hour)},
				"org_b": {AccessToken: tokenB, ExpiresAt: time.Now().Add(time.Hour)},
			},
		}},
	}))
	endpoint := newFakeTokenEndpoint(t, "refresh_0", 15*time.Minute)
	m := &TokenManager{storage: storage, client: createTestClient(endpoint.URL)}

	got, err := m.GetOrgScopedAccessToken(context.Background(), "org_a")
	require.NoError(t, err)
	assert.Equal(t, tokenA, got)

	// A missing keyring item is a cache miss, not an error.
	require.NoError(t, keyring.Delete(KeyringService, orgTokenKeyringKey("default", "org_b")))
	renewed, err := m.GetOrgScopedAccessToken(context.Background(), "org_b")
	require.NoError(t, err)
	assert.Equal(t, endpoint.lastIssued(), renewed)
	assert.Equal(t, 1, endpoint.exchangeCount())

	// The exchange for org_b rewrote only org_b, so org_a is still served from the cache.
	got, err = m.GetOrgScopedAccessToken(context.Background(), "org_a")
	require.NoError(t, err)
	assert.Equal(t, tokenA, got)
	assert.Equal(t, 1, endpoint.exchangeCount())
}
