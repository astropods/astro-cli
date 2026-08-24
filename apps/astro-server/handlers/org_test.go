package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/memberemails"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/org"
	"github.com/astropods/astro/apps/astro-server/internal/slackidentity"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

// fakeMemberRoleSyncer is a test stub for memberRoleSyncer. It captures the
// arguments passed to each call so tests can assert that the handler forwards
// the caller's user id, and returns configurable errors to exercise the 403
// mapping for ErrOwnerManagementForbidden.
type fakeMemberRoleSyncer struct {
	changeErr        error
	changePrevRole   string
	removeErr        error
	changeCalledWith struct {
		accountID    string
		userID       string
		newRole      string
		callerUserID string
	}
	removeCalledWith struct {
		accountID string
		userID    string
	}
}

func (f *fakeMemberRoleSyncer) ChangeMemberRole(_ context.Context, accountID, userID, newRole, callerUserID string) (string, error) {
	f.changeCalledWith.accountID = accountID
	f.changeCalledWith.userID = userID
	f.changeCalledWith.newRole = newRole
	f.changeCalledWith.callerUserID = callerUserID
	return f.changePrevRole, f.changeErr
}

func (f *fakeMemberRoleSyncer) RemoveMember(_ context.Context, accountID, userID string) error {
	f.removeCalledWith.accountID = accountID
	f.removeCalledWith.userID = userID
	return f.removeErr
}

func init() {
	gin.SetMode(gin.TestMode)
}

// injectTestOrgAccount sets an org account + user + session in context for testing org handlers
func injectTestOrgAccount(acct *account.Account, user *auth.User) gin.HandlerFunc {
	return func(c *gin.Context) {
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		c.Next()
	}
}

// injectTestOrgAccountWithSession sets account, user, and session in context
func injectTestOrgAccountWithSession(acct *account.Account, user *auth.User, session *auth.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		if acct != nil {
			c.Set(string(auth.AccountContextKey), acct)
		}
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		if session != nil {
			c.Set(string(auth.SessionContextKey), session)
		}
		c.Next()
	}
}

// fakeEmailDirectory records write-backs so tests can assert the mirror heals.
type fakeEmailDirectory struct {
	mu        sync.Mutex
	emails    map[string]string
	attempted map[string]time.Time
	readErr   error
	upserted  map[string]string
	stamped   []string
	upsertErr error
}

func (f *fakeEmailDirectory) EmailsForUsers(_ context.Context, userIDs []string) (map[string]memberemails.MemberEmail, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	out := map[string]memberemails.MemberEmail{}
	for _, uid := range userIDs {
		e, hasEmail := f.emails[uid]
		at, hasAttempt := f.attempted[uid]
		if !hasEmail && !hasAttempt {
			continue
		}
		out[uid] = memberemails.MemberEmail{Email: e, AttemptedAt: at}
	}
	return out, nil
}

func (f *fakeEmailDirectory) RecordReconcileAttempt(_ context.Context, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.stamped = append(f.stamped, userID)
	return nil
}

func (f *fakeEmailDirectory) UpsertWorkOS(_ context.Context, userID, email string, _ bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.upserted == nil {
		f.upserted = map[string]string{}
	}
	f.upserted[userID] = email
	return f.upsertErr
}

type fakeUserFetcher struct {
	mu    sync.Mutex
	users map[string]*auth.User
	err   error
	calls int
}

func (f *fakeUserFetcher) GetUser(_ context.Context, userID string) (*auth.User, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	u, ok := f.users[userID]
	if !ok {
		return nil, errors.New("not found")
	}
	return u, nil
}

// --- member email tests ---

func TestResolveMemberEmails_PrefersMirrorOverWorkOS(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{emails: map[string]string{"user-1": "mirrored@example.com"}}
	fetcher := &fakeUserFetcher{}

	got := resolveMemberEmails(context.Background(), log, dir, fetcher, []string{"user-1"})

	if got["user-1"] != "mirrored@example.com" {
		t.Errorf("expected mirrored email, got %q", got["user-1"])
	}
	if fetcher.calls != 0 {
		t.Errorf("expected no WorkOS calls, got %d", fetcher.calls)
	}
}

func TestResolveMemberEmails_FetchesMissingAndHealsMirror(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{}
	fetcher := &fakeUserFetcher{users: map[string]*auth.User{
		"user-2": {ID: "user-2", Email: "invited@example.com", EmailVerified: true},
	}}

	got := resolveMemberEmails(context.Background(), log, dir, fetcher, []string{"user-2"})

	if got["user-2"] != "invited@example.com" {
		t.Errorf("expected fetched email, got %q", got["user-2"])
	}
	if dir.upserted["user-2"] != "invited@example.com" {
		t.Errorf("expected write-back to the mirror, got %q", dir.upserted["user-2"])
	}
}

func TestResolveMemberEmails_WorkOSFailureKeepsMirroredResults(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{emails: map[string]string{"user-1": "mirrored@example.com"}}
	fetcher := &fakeUserFetcher{err: errors.New("workos down")}

	got := resolveMemberEmails(context.Background(), log, dir, fetcher, []string{"user-1", "user-2"})

	if got["user-1"] != "mirrored@example.com" {
		t.Errorf("expected mirrored email retained, got %q", got["user-1"])
	}
	if _, ok := got["user-2"]; ok {
		t.Errorf("expected no entry for the unresolved user, got %q", got["user-2"])
	}
}

func TestResolveMemberEmails_NoFetcherReturnsMirrorOnly(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{emails: map[string]string{"user-1": "mirrored@example.com"}}

	got := resolveMemberEmails(context.Background(), log, dir, nil, []string{"user-1", "user-2"})

	if len(got) != 1 || got["user-1"] != "mirrored@example.com" {
		t.Errorf("expected mirror-only result, got %v", got)
	}
}

func TestResolveMemberEmails_CapsWorkOSLookups(t *testing.T) {
	log := logger.New("error", "json")
	users := map[string]*auth.User{}
	ids := make([]string, 0, memberEmailLookupLimit+5)
	for i := 0; i < memberEmailLookupLimit+5; i++ {
		uid := fmt.Sprintf("user-%d", i)
		ids = append(ids, uid)
		users[uid] = &auth.User{ID: uid, Email: uid + "@example.com"}
	}
	fetcher := &fakeUserFetcher{users: users}

	got := resolveMemberEmails(context.Background(), log, &fakeEmailDirectory{}, fetcher, ids)

	if fetcher.calls != memberEmailLookupLimit {
		t.Errorf("expected %d lookups, got %d", memberEmailLookupLimit, fetcher.calls)
	}
	if len(got) != memberEmailLookupLimit {
		t.Errorf("expected %d resolved emails, got %d", memberEmailLookupLimit, len(got))
	}
}

// A member WorkOS cannot name must not be re-queried on every listing; the
// stamp puts them on the same backoff the reconcile job honors.
func TestResolveMemberEmails_StampsUnresolvableUsers(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{}
	fetcher := &fakeUserFetcher{err: errors.New("workos 404")}

	resolveMemberEmails(context.Background(), log, dir, fetcher, []string{"user-1"})

	if len(dir.stamped) != 1 || dir.stamped[0] != "user-1" {
		t.Errorf("expected an attempt stamped for user-1, got %v", dir.stamped)
	}
}

func TestResolveMemberEmails_SkipsRecentlyAttemptedUsers(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{attempted: map[string]time.Time{"user-1": time.Now()}}
	fetcher := &fakeUserFetcher{users: map[string]*auth.User{
		"user-1": {ID: "user-1", Email: "late@example.com"},
	}}

	got := resolveMemberEmails(context.Background(), log, dir, fetcher, []string{"user-1"})

	if fetcher.calls != 0 {
		t.Errorf("expected the backoff to skip the lookup, got %d calls", fetcher.calls)
	}
	if len(got) != 0 {
		t.Errorf("expected no resolved emails, got %v", got)
	}
}

func TestResolveMemberEmails_RetriesAfterBackoffWindow(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{attempted: map[string]time.Time{
		"user-1": time.Now().Add(-memberemails.RetryBackoff - time.Minute),
	}}
	fetcher := &fakeUserFetcher{users: map[string]*auth.User{
		"user-1": {ID: "user-1", Email: "resolved@example.com"},
	}}

	got := resolveMemberEmails(context.Background(), log, dir, fetcher, []string{"user-1"})

	if got["user-1"] != "resolved@example.com" {
		t.Errorf("expected the retry to resolve, got %q", got["user-1"])
	}
}

func TestNameUnprofiledMembers_OnlyTouchesRowsWithNoProfile(t *testing.T) {
	log := logger.New("error", "json")
	dir := &fakeEmailDirectory{emails: map[string]string{
		"user-1": "named@example.com",
		"user-2": "invited@example.com",
	}}

	members := []MemberResponse{
		{UserID: "user-1", Username: "named", DisplayName: "Named"},
		{UserID: "user-2"},
	}
	nameUnprofiledMembers(context.Background(), log, dir, nil, members)

	if members[0].Email != "" {
		t.Errorf("expected no email on a profiled member, got %q", members[0].Email)
	}
	if members[1].Email != "invited@example.com" {
		t.Errorf("expected the invited address, got %q", members[1].Email)
	}
}

// --- ListMembers tests ---

func TestListMembers_Success(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "wm-1", time.Now()).
			AddRow("acct-1", "user-2", nil, time.Now()))

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(acct, user), ListMembers(log, store, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	var members []account.AccountMember
	if err := json.Unmarshal(resp["members"], &members); err != nil {
		t.Fatalf("failed to unmarshal members: %v", err)
	}

	if len(members) != 2 {
		t.Errorf("expected 2 members, got %d", len(members))
	}
}

// When a slackidentity store is wired in, ListMembers must populate the
// per-member slack_workspaces field so the grants UI can warn about
// unlinked targets without a second round-trip.
func TestListMembers_PopulatesSlackWorkspaces(t *testing.T) {
	db, mock, _ := sqlmock.New()
	defer db.Close()
	store := account.NewAccountStore(db)
	slackStore := slackidentity.NewStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"account_id", "user_id", "workos_membership_id", "created_at"}).
			AddRow("acct-1", "user-1", "wm-1", time.Now()).
			AddRow("acct-1", "user-2", "wm-2", time.Now()))

	// Personal profiles batch — match by SQL fragment to stay loose on
	// whitespace/argument formatting.
	mock.ExpectQuery("SELECT am.user_id, a.name, a.display_name").
		WithArgs(pq.Array([]string{"user-1", "user-2"})).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "name", "display_name", "avatar_updated_at"}).
			AddRow("user-1", "alice", "Alice", nil).
			AddRow("user-2", "bob", "Bob", nil))

	// Slack identities batch — user-1 has two linked workspaces, user-2 none.
	now := time.Now()
	mock.ExpectQuery("FROM slack_identity_mappings\\s+WHERE workos_user_id = ANY").
		WithArgs(pq.Array([]string{"user-1", "user-2"})).
		WillReturnRows(sqlmock.NewRows([]string{
			"team_id", "slack_user_id", "workos_user_id",
			"organization_id",
			"team_name", "team_domain", "team_icon_url", "slack_username",
			"created_at", "updated_at", "revoked_at",
		}).
			AddRow("T1", "U1", "user-1", "", "Acme", "acme", "https://icon.png", "alice", now, now, nil).
			AddRow("T2", "U2", "user-1", "", "Foo", "foo", "", "alice", now, now, nil))

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "personal"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(acct, user), ListMembers(log, store, nil, nil, slackStore, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Members []MemberResponse `json:"members"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(resp.Members))
	}

	byUser := map[string]MemberResponse{}
	for _, m := range resp.Members {
		byUser[m.UserID] = m
	}
	if got := len(byUser["user-1"].SlackWorkspaces); got != 2 {
		t.Errorf("user-1 should have 2 workspaces, got %d (full: %+v)", got, byUser["user-1"])
	}
	if got := len(byUser["user-2"].SlackWorkspaces); got != 0 {
		t.Errorf("user-2 should have 0 workspaces, got %d", got)
	}
	if w := byUser["user-1"].SlackWorkspaces; len(w) > 0 {
		if w[0].TeamName != "Acme" || w[0].IconURL != "https://icon.png" {
			t.Errorf("user-1 first workspace metadata: %+v", w[0])
		}
	}
}

func TestListMembers_NoAccount(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(nil, nil), ListMembers(log, store, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when no account, got %d", rec.Code)
	}
}

func TestListMembers_DBError(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	mock.ExpectQuery("SELECT .+ FROM account_members am LEFT JOIN account_member_workos mw").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(acct, user), ListMembers(log, store, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on DB error, got %d", rec.Code)
	}
}

// --- AddMember tests (handler-level validation only) ---

func TestAddMember_InvalidBody_MissingFields(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	// syncSvc is nil — we test that validation fires before sync is called
	router.POST("/members", injectTestOrgAccount(acct, nil), AddMember(log, nil, nil, nil, nil, nil))

	tests := []struct {
		name string
		body string
	}{
		{"missing user_id", `{"role": "admin"}`},
		{"missing role", `{"user_id": "user-1"}`},
		{"empty body", `{}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAddMember_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/members", injectTestOrgAccount(nil, nil), AddMember(log, nil, nil, nil, nil, nil))

	body := `{"user_id": "user-1", "role": "admin"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- UpdateMemberRole tests ---

func TestUpdateMemberRole_InvalidBody(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccount(acct, nil), UpdateMemberRole(log, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodPut, "/members/user-1", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMemberRole_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccount(nil, nil), UpdateMemberRole(log, nil, nil, nil, nil))

	body := `{"role": "admin"}`
	req := httptest.NewRequest(http.MethodPut, "/members/user-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- RemoveMember tests ---

func TestRemoveMember_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.DELETE("/members/:user_id", injectTestOrgAccount(nil, nil), RemoveMember(log, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- CreateInvitations tests ---

func TestCreateInvitations_NonOrgAccount(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "personal", Type: "personal", WorkOSOrganizationID: ""}
	user := &auth.User{ID: "user-1", Email: "test@example.com"}

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccount(acct, user), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "member"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-org account, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["error"] != "invitations are only supported for organization accounts" {
		t.Errorf("unexpected error: %s", resp["error"])
	}
}

func TestCreateInvitations_NoAuth(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}

	router := gin.New()
	// No user injected
	router.POST("/invitations", injectTestOrgAccount(acct, nil), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "member"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no user, got %d", rec.Code)
	}
}

func TestCreateInvitations_InvalidBody(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccount(acct, user), CreateInvitations(log, nil, nil))

	// Empty invitations array should fail
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(`{"invitations":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateInvitations_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccount(nil, nil), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "member"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- RevokeInvitation tests ---

func TestRevokeInvitation_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	// No account injected — handler should return 500
	router.DELETE("/invitations/:id", injectTestOrgAccount(nil, nil), RevokeInvitation(log, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/invitations/inv-123", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when no account, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- ListAccountInvitations tests ---

func TestListAccountInvitations_NoWorkOSOrgID(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "personal", Type: "personal", WorkOSOrganizationID: ""}

	router := gin.New()
	router.GET("/invitations", injectTestOrgAccount(acct, nil), ListAccountInvitations(log, nil))

	req := httptest.NewRequest(http.MethodGet, "/invitations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]json.RawMessage
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if string(resp["invitations"]) != "[]" {
		t.Errorf("expected empty invitations array, got %s", string(resp["invitations"]))
	}
}

func TestListAccountInvitations_NoAccount(t *testing.T) {
	log := logger.New("error", "json")

	router := gin.New()
	router.GET("/invitations", injectTestOrgAccount(nil, nil), ListAccountInvitations(log, nil))

	req := httptest.NewRequest(http.MethodGet, "/invitations", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rec.Code)
	}
}

// --- Role Escalation Prevention tests ---
// These now check session.Role from JWT instead of DB role lookup

func TestAddMember_OwnerRoleCannotBeGrantedOnJoin(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1", Email: "admin@example.com"}
	session := &auth.Session{Role: "admin"}

	router := gin.New()
	router.POST("/members", injectTestOrgAccountWithSession(acct, caller, session), AddMember(log, nil, nil, nil, nil, nil))

	body := `{"user_id": "new-user", "role": "owner"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("adding a member as owner should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMemberRole_RoleEscalation_NonOwnerCannotPromoteToOwner(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1", Email: "admin@example.com"}
	session := &auth.Session{Role: "admin"}

	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("owner-9"))

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccountWithSession(acct, caller, session), UpdateMemberRole(log, nil, store, nil, nil))

	body := `{"role": "owner"}`
	req := httptest.NewRequest(http.MethodPut, "/members/target-user", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("non-owner should not be able to promote to owner, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateInvitations_RoleEscalation_NonOwnerCannotInviteAsOwner(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	caller := &auth.User{ID: "caller-1", Email: "admin@example.com"}
	session := &auth.Session{Role: "admin"}

	router := gin.New()
	router.POST("/invitations", injectTestOrgAccountWithSession(acct, caller, session), CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "invite@example.com", "kind": "email", "role": "owner"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("inviting as owner should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Cross-account access test ---

func TestListMembers_CrossAccount_Denied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	// ResolveAccount: return org-b
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("org-b").
		WillReturnRows(sqlmock.NewRows([]string{"id", "name", "type", "workos_org_id", "deleted_at", "created_at", "updated_at", "display_name", "avatar_colors", "avatar_updated_at", "cluster_id", "account_number", "bio", "location", "local_timezone", "pronouns", "website", "social_links", "blueprint_order"}).
			AddRow("acct-b", "org-b", "organization", "org_b_wos", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// RequireAccountMember: IsMember returns 0 (user-a is not a member of org-b)
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-b", "user-a").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(auth.UserContextKey), &auth.User{ID: "user-a"})
		c.Set(string(auth.SessionContextKey), &auth.Session{OrganizationID: ""})
		c.Next()
	})
	memberRoutes := router.Group("/accounts/:account/members")
	memberRoutes.Use(middleware.ResolveAccount(store))
	// Match main.go: memberRoutes uses RequireAccountMember, not RequireAccountPermission
	memberRoutes.Use(middleware.RequireAccountMember(store))
	memberRoutes.GET("", ListMembers(log, store, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/accounts/org-b/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("cross-account access should be denied, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- ListMembers permission boundary tests ---

func TestListMembers_NonMember_Denied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	// IsMember returns 0
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	user := &auth.User{ID: "user-1"}

	router := gin.New()
	router.GET("/members", injectTestOrgAccount(acct, user), ListMembers(log, store, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodGet, "/members", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected 403 for non-member, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- RemoveMember permission boundary tests ---

func TestRemoveMember_SelfRemoval_NoOrgManage_Allowed(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "user-1"}
	// Session without org:manage — self-removal should still be allowed
	session := &auth.Session{OrganizationID: "org_123", Permissions: []string{}}

	syncSvc := org.NewSync(nil, store, nil, db, logger.New("error", "json"))

	// syncSvc.RemoveMember will try to look up the account — let it fail with DB error
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		RemoveMember(log, syncSvc, store, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should NOT be 403 — self-removal bypasses org:manage check
	if rec.Code == http.StatusForbidden {
		t.Errorf("self-removal should not require org:manage, got 403: %s", rec.Body.String())
	}
}

func TestRemoveMember_OtherRemoval_WithOrgManage_Allowed(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "user-1"}
	session := &auth.Session{OrganizationID: "org_123", Permissions: []string{"org:manage"}}

	syncSvc := org.NewSync(nil, store, nil, db, logger.New("error", "json"))

	// syncSvc will try to look up account — let it fail
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		RemoveMember(log, syncSvc, store, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should NOT be 403 — caller has org:manage
	if rec.Code == http.StatusForbidden {
		t.Errorf("user with org:manage should be able to remove others, got 403: %s", rec.Body.String())
	}
}

func TestRemoveMember_OtherRemoval_WithoutOrgManage_Denied(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "user-1"}
	// Session scoped to org but without org:manage permission
	session := &auth.Session{OrganizationID: "org_123", Permissions: []string{}}

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		RemoveMember(log, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("removing others without org:manage should be denied, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveMember_OtherRemoval_OrgMismatch_Denied(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "user-1"}
	// Session scoped to a DIFFERENT org
	session := &auth.Session{OrganizationID: "org_OTHER", Permissions: []string{"org:manage"}}

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		RemoveMember(log, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("org mismatch should be denied, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRemoveMember_OtherRemoval_PersonalAccount_MemberAllowed(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	// Personal account — HasAccountPermission checks IsMember instead of JWT permissions
	acct := &account.Account{ID: "acct-1", Name: "myacct", Type: "personal"}
	user := &auth.User{ID: "user-1"}

	syncSvc := org.NewSync(nil, store, nil, db, logger.New("error", "json"))

	// HasAccountPermission for personal account calls IsMember — return true
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// syncSvc.RemoveMember will try to look up account — let it fail
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccount(acct, user),
		RemoveMember(log, syncSvc, store, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should NOT be 403 — personal account membership grants all permissions
	if rec.Code == http.StatusForbidden {
		t.Errorf("personal account member should be able to remove others, got 403: %s", rec.Body.String())
	}
}

func TestRemoveMember_NoUser_Denied(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	// Inject account but no user
	router.DELETE("/members/:user_id",
		injectTestOrgAccount(acct, nil),
		RemoveMember(log, nil, nil, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/user-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with no user, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- AddMember role validation tests ---

func TestAddMember_InvalidRole_Rejected(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	router.POST("/members", injectTestOrgAccount(acct, nil), AddMember(log, nil, nil, nil, nil, nil))

	body := `{"user_id": "user-1", "role": "superadmin"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid role should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAddMember_AdminCanAssignAdmin(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1"}
	session := &auth.Session{Role: "admin"}

	syncSvc := org.NewSync(nil, store, nil, db, logger.New("error", "json"))

	// syncSvc will try to look up account — let it fail
	mock.ExpectQuery("SELECT .+ FROM accounts").
		WithArgs("acct-1").
		WillReturnError(sqlmock.ErrCancelled)

	router := gin.New()
	router.POST("/members",
		injectTestOrgAccountWithSession(acct, caller, session),
		AddMember(log, syncSvc, store, nil, nil, nil))

	body := `{"user_id": "new-user", "role": "admin"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	// Should NOT be 403 — admin can assign admin (owner guard only blocks non-owners from assigning owner)
	if rec.Code == http.StatusForbidden {
		t.Errorf("admin should be able to assign admin role, got 403: %s", rec.Body.String())
	}
}

func TestAddMember_MemberSessionCannotAssignOwner(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}
	caller := &auth.User{ID: "caller-1"}
	session := &auth.Session{Role: "member"}

	router := gin.New()
	router.POST("/members",
		injectTestOrgAccountWithSession(acct, caller, session),
		AddMember(log, nil, nil, nil, nil, nil))

	body := `{"user_id": "new-user", "role": "owner"}`
	req := httptest.NewRequest(http.MethodPost, "/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("adding a member as owner should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- UpdateMemberRole validation tests ---

func TestUpdateMemberRole_InvalidRole_Rejected(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization"}

	router := gin.New()
	router.PUT("/members/:user_id", injectTestOrgAccount(acct, nil), UpdateMemberRole(log, nil, nil, nil, nil))

	body := `{"role": "superuser"}`
	req := httptest.NewRequest(http.MethodPut, "/members/user-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid role should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- CreateInvitations permission boundary tests ---

func TestCreateInvitations_BulkMixed_NonOwnerAssignsOwner_Rejected(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	caller := &auth.User{ID: "caller-1"}
	session := &auth.Session{Role: "admin"}

	router := gin.New()
	router.POST("/invitations",
		injectTestOrgAccountWithSession(acct, caller, session),
		CreateInvitations(log, nil, nil))

	// First invitation is fine (member role), second triggers owner escalation guard
	body := `{"invitations": [{"value": "a@example.com", "kind": "email", "role": "member"}, {"value": "b@example.com", "kind": "email", "role": "owner"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("an owner-role invitation in a batch should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Full middleware chain tests ---
// These wire ResolveAccount → RequireAccountMember → RequireAccountPermission
// matching the exact route structure in main.go, to verify that the middleware
// + handler + permission checks work together as a stack.

// orgAccountCols matches scanAccount's expected column shape for org permission tests.
var orgAccountCols = account.SQLMockScanColumns

// injectAuth returns middleware that sets user and session in context.
func injectAuth(user *auth.User, session *auth.Session) gin.HandlerFunc {
	return func(c *gin.Context) {
		if user != nil {
			c.Set(string(auth.UserContextKey), user)
		}
		if session != nil {
			c.Set(string(auth.SessionContextKey), session)
		}
		c.Next()
	}
}

func TestFullChain_AddMember_MemberWithoutOrgManage_Denied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	// ResolveAccount lookup
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(orgAccountCols).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// RequireAccountMember: IsMember returns true so request reaches RequireAccountPermission
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := gin.New()
	router.Use(injectAuth(
		&auth.User{ID: "user-1"},
		&auth.Session{OrganizationID: "org_123", Role: "member", Permissions: []string{}},
	))
	base := router.Group("/accounts/:account")
	memberRoutes := base.Group("/members")
	memberRoutes.Use(middleware.ResolveAccount(store))
	memberRoutes.Use(middleware.RequireAccountMember(store))
	memberManageRoutes := memberRoutes.Group("")
	memberManageRoutes.Use(middleware.RequireAccountPermission(store, "org:manage"))
	memberManageRoutes.POST("", AddMember(log, nil, store, nil, nil, nil))

	body := `{"user_id":"new-user","role":"member"}`
	req := httptest.NewRequest(http.MethodPost, "/accounts/myorg/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("member without org:manage should be blocked by middleware, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFullChain_UpdateMemberRole_MemberWithoutOrgManage_Denied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(orgAccountCols).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// RequireAccountMember: IsMember returns true so request reaches RequireAccountPermission
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := gin.New()
	router.Use(injectAuth(
		&auth.User{ID: "user-1"},
		&auth.Session{OrganizationID: "org_123", Role: "member", Permissions: []string{}},
	))
	base := router.Group("/accounts/:account")
	memberRoutes := base.Group("/members")
	memberRoutes.Use(middleware.ResolveAccount(store))
	memberRoutes.Use(middleware.RequireAccountMember(store))
	memberManageRoutes := memberRoutes.Group("")
	memberManageRoutes.Use(middleware.RequireAccountPermission(store, "org:manage"))
	memberManageRoutes.PUT("/:user_id", UpdateMemberRole(log, nil, store, nil, nil))

	body := `{"role":"admin"}`
	req := httptest.NewRequest(http.MethodPut, "/accounts/myorg/members/user-2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("member without org:manage should be blocked, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFullChain_RemoveMember_OtherRemoval_MemberDenied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	// ResolveAccount
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(orgAccountCols).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// RequireAccountMember: IsMember
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	router := gin.New()
	router.Use(injectAuth(
		&auth.User{ID: "user-1"},
		&auth.Session{OrganizationID: "org_123", Role: "member", Permissions: []string{}},
	))
	base := router.Group("/accounts/:account")
	memberRoutes := base.Group("/members")
	memberRoutes.Use(middleware.ResolveAccount(store))
	memberRoutes.Use(middleware.RequireAccountMember(store))
	memberRoutes.DELETE("/:user_id", RemoveMember(log, nil, store, nil, nil, nil))

	// user-1 tries to remove user-2 — handler checks HasAccountPermission("org:manage") → false
	req := httptest.NewRequest(http.MethodDelete, "/accounts/myorg/members/user-2", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("member removing others without org:manage should be denied, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestFullChain_OrgManage_WrongOrgScope_Denied(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")

	// ResolveAccount: account belongs to org_123
	mock.ExpectQuery("SELECT .+ FROM accounts a LEFT JOIN account_organizations ao").
		WithArgs("myorg").
		WillReturnRows(sqlmock.NewRows(orgAccountCols).
			AddRow("acct-1", "myorg", "organization", "org_123", nil, time.Now(), time.Now(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil))

	// RequireAccountMember: IsMember returns true so request reaches RequireAccountPermission
	mock.ExpectQuery("SELECT COUNT.+ FROM account_members").
		WithArgs("acct-1", "user-1").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	// Session is scoped to DIFFERENT org (org_OTHER)
	router := gin.New()
	router.Use(injectAuth(
		&auth.User{ID: "user-1"},
		&auth.Session{OrganizationID: "org_OTHER", Role: "admin", Permissions: []string{"org:manage"}},
	))
	base := router.Group("/accounts/:account")
	memberRoutes := base.Group("/members")
	memberRoutes.Use(middleware.ResolveAccount(store))
	memberRoutes.Use(middleware.RequireAccountMember(store))
	memberManageRoutes := memberRoutes.Group("")
	memberManageRoutes.Use(middleware.RequireAccountPermission(store, "org:manage"))
	memberManageRoutes.POST("", AddMember(log, nil, store, nil, nil, nil))

	body := `{"user_id":"new-user","role":"member"}`
	req := httptest.NewRequest(http.MethodPost, "/accounts/myorg/members", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("org:manage scoped to wrong org should be denied, expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateInvitations_InvalidRole_Rejected(t *testing.T) {
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	caller := &auth.User{ID: "caller-1"}

	router := gin.New()
	router.POST("/invitations",
		injectTestOrgAccount(acct, caller),
		CreateInvitations(log, nil, nil))

	body := `{"invitations": [{"value": "a@example.com", "kind": "email", "role": "superadmin"}]}`
	req := httptest.NewRequest(http.MethodPost, "/invitations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid role should be rejected, expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

// --- Role hierarchy tests: only owners can manage existing owners ---

func TestRemoveMember_TheOwnerCannotBeRemoved(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "admin-1"}
	session := &auth.Session{OrganizationID: "org_123", Role: "admin", Permissions: []string{"org:manage"}}

	fake := &fakeMemberRoleSyncer{removeErr: org.ErrOwnerRequired}

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		RemoveMember(log, fake, store, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/owner-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("removing the owner should return 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.removeCalledWith.userID != "owner-1" {
		t.Errorf("expected target user-id=owner-1, got %q", fake.removeCalledWith.userID)
	}
}

func TestRemoveMember_AdminCanRemoveNonOwner(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "admin-1"}
	session := &auth.Session{OrganizationID: "org_123", Role: "admin", Permissions: []string{"org:manage"}}

	fake := &fakeMemberRoleSyncer{removeErr: nil}

	router := gin.New()
	router.DELETE("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		RemoveMember(log, fake, store, nil, nil, nil))

	req := httptest.NewRequest(http.MethodDelete, "/members/member-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Errorf("admin should be able to remove non-owner members, got 403: %s", rec.Body.String())
	}
}

func TestUpdateMemberRole_TheOwnerCannotBeDemoted(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "admin-1"}
	session := &auth.Session{OrganizationID: "org_123", Role: "admin", Permissions: []string{"org:manage"}}

	fake := &fakeMemberRoleSyncer{changeErr: org.ErrOwnerRequired}

	router := gin.New()
	router.PUT("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		UpdateMemberRole(log, fake, store, nil, nil))

	body := `{"role": "member"}`
	req := httptest.NewRequest(http.MethodPut, "/members/owner-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("demoting the owner should return 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.changeCalledWith.callerUserID != "admin-1" {
		t.Errorf("expected the caller's user id passed to sync, got %q", fake.changeCalledWith.callerUserID)
	}
	if fake.changeCalledWith.newRole != "member" {
		t.Errorf("expected newRole=member passed to sync, got %q", fake.changeCalledWith.newRole)
	}
}

func TestUpdateMemberRole_OnlyTheRecordedOwnerCanTransfer(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "admin-1"}
	session := &auth.Session{OrganizationID: "org_123", Role: "admin", Permissions: []string{"org:manage"}}

	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("owner-1"))

	fake := &fakeMemberRoleSyncer{}

	router := gin.New()
	router.PUT("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		UpdateMemberRole(log, fake, store, nil, nil))

	body := `{"role": "owner"}`
	req := httptest.NewRequest(http.MethodPut, "/members/member-2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("an admin who is not the owner cannot transfer, got %d: %s", rec.Code, rec.Body.String())
	}
	if fake.changeCalledWith.newRole != "" {
		t.Error("the guard should reject before reaching the sync layer")
	}
}

func TestUpdateMemberRole_OwnerTransfersOwnership(t *testing.T) {
	db, mock, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "owner-1"}
	session := &auth.Session{OrganizationID: "org_123", Role: "owner", Permissions: []string{"org:manage"}}

	mock.ExpectQuery("SELECT owner_user_id FROM accounts").
		WithArgs("acct-1").
		WillReturnRows(sqlmock.NewRows([]string{"owner_user_id"}).AddRow("owner-1"))

	fake := &fakeMemberRoleSyncer{changePrevRole: "admin"}

	router := gin.New()
	router.PUT("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		UpdateMemberRole(log, fake, store, nil, nil))

	body := `{"role": "owner"}`
	req := httptest.NewRequest(http.MethodPut, "/members/member-2", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Fatalf("the recorded owner may transfer, got 403: %s", rec.Body.String())
	}
	if fake.changeCalledWith.callerUserID != "owner-1" {
		t.Errorf("expected the caller's user id passed to sync, got %q", fake.changeCalledWith.callerUserID)
	}
	if fake.changeCalledWith.newRole != "owner" {
		t.Errorf("expected newRole=owner passed to sync, got %q", fake.changeCalledWith.newRole)
	}
}

func TestUpdateMemberRole_AdminCanChangeNonOwner(t *testing.T) {
	db, _, _ := sqlmock.New()
	store := account.NewAccountStore(db)
	log := logger.New("error", "json")
	acct := &account.Account{ID: "acct-1", Name: "myorg", Type: "organization", WorkOSOrganizationID: "org_123"}
	user := &auth.User{ID: "admin-1"}
	session := &auth.Session{OrganizationID: "org_123", Role: "admin", Permissions: []string{"org:manage"}}

	fake := &fakeMemberRoleSyncer{changeErr: nil, changePrevRole: "member"}

	router := gin.New()
	router.PUT("/members/:user_id",
		injectTestOrgAccountWithSession(acct, user, session),
		UpdateMemberRole(log, fake, store, nil, nil))

	body := `{"role": "admin"}`
	req := httptest.NewRequest(http.MethodPut, "/members/member-1", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code == http.StatusForbidden {
		t.Errorf("admin should be able to promote a non-owner, got 403: %s", rec.Body.String())
	}
}
