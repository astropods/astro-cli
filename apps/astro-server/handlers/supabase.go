package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/gin-gonic/gin"
)

// supabaseProvider is the WorkOS Pipes connected-account slug for the custom
// Supabase OAuth provider. It must match the slug configured in WorkOS.
const supabaseProvider = "supabase"

const supabaseAPIBase = "https://api.supabase.com"

// supabaseUserAgent is sent on every outbound Supabase API call. Supabase uses
// the User-Agent for OAuth partner attribution (our affiliate identity).
const supabaseUserAgent = "Astro AI"

// supabaseUserAgentTransport stamps supabaseUserAgent on every request. Per the
// http.RoundTripper contract it clones the request before mutating headers, and
// resolves its base lazily so tests that swap http.DefaultTransport still work.
type supabaseUserAgentTransport struct{ base http.RoundTripper }

func (t supabaseUserAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	r := req.Clone(req.Context())
	r.Header.Set("User-Agent", supabaseUserAgent)
	return base.RoundTrip(r)
}

// supabaseHTTPClient bounds outbound calls to the Supabase Management API so a
// slow or unresponsive upstream can't tie up a request goroutine indefinitely,
// and stamps the affiliate User-Agent on every request.
var supabaseHTTPClient = &http.Client{
	Timeout:   15 * time.Second,
	Transport: supabaseUserAgentTransport{},
}

// SupabaseHandlerConfig holds the base URLs used to build the OAuth redirect
// round-trip. WorkOS Pipes owns the OAuth credentials and token lifecycle, so
// no client secret or callback config lives here anymore.
type SupabaseHandlerConfig struct {
	// WebhookBaseURL is the public API base URL the WorkOS ReturnTo callback is
	// built from (e.g. https://astropods.com).
	WebhookBaseURL string
	// FrontendURL is the base URL of the web app for the final browser redirect.
	FrontendURL string
}

// defaultSupabaseDest is where the OAuth round-trip lands when the flow didn't
// carry its own redirect_to (e.g. started from a deep link).
const defaultSupabaseDest = "/knowledge/new?provider=supabase"

// isSafeRedirectPath returns true when path is a same-origin relative URL we can
// safely redirect to. We require a leading slash and reject "//foo" or paths
// containing "@" (both can be abused to redirect off-site).
func isSafeRedirectPath(path string) bool {
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "/") {
		return false
	}
	if strings.HasPrefix(path, "//") {
		return false
	}
	if strings.Contains(path, "@") {
		return false
	}
	return true
}

// appendParam appends key=value to a path, choosing ? or & based on whether the
// path already has a query string.
func appendParam(path, param string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return path + sep + param
}

// SupabaseProject is a project from the Supabase Management API.
type SupabaseProject struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	OrgID  string `json:"organization_id"`
}

// SupabaseConnectResponse is returned by the connect endpoint: a redirect URL to
// start the OAuth flow, or connected:true when already connected.
type SupabaseConnectResponse struct {
	RedirectURL string `json:"redirect_url,omitempty"`
	Connected   bool   `json:"connected,omitempty"`
}

// SupabaseConnect handles POST /api/v1/accounts/:account/supabase/connect.
// Short-circuits when a connection already exists; otherwise returns a WorkOS
// Pipes authorization URL for the browser to follow.
func SupabaseConnect(log *logger.Logger, pipesClient *pipes.Client, cfg SupabaseHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req struct {
			RedirectTo string `json:"redirect_to"`
		}
		_ = c.ShouldBindJSON(&req)

		// Already connected? WorkOS serves a token → short-circuit.
		_, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err == nil {
			c.JSON(http.StatusOK, SupabaseConnectResponse{Connected: true})
			return
		}
		if !errors.Is(err, pipes.ErrNeedsReauthorization) && !errors.Is(err, pipes.ErrNotInstalled) {
			log.Error("supabase: check existing connection", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check connection"})
			return
		}

		redirectTo := req.RedirectTo
		if !isSafeRedirectPath(redirectTo) {
			redirectTo = ""
		}
		// WorkOS returns the browser to our account-scoped callback, which then
		// bounces to the frontend. Encoding redirect_to lets the callback know
		// where the flow began (e.g. Settings vs. the add-store form).
		callbackURL := fmt.Sprintf("%s/api/v1/accounts/%s/supabase/callback?redirect_to=%s",
			cfg.WebhookBaseURL, acct.Name, url.QueryEscape(redirectTo))

		authURL, err := pipesClient.GetAuthorizationURL(c.Request.Context(), pipes.GetAuthorizationURLInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
			ReturnTo:       callbackURL,
		})
		if err != nil {
			log.Error("supabase: get authorization URL", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate Supabase authorization URL"})
			return
		}

		c.JSON(http.StatusOK, SupabaseConnectResponse{RedirectURL: authURL})
	}
}

// SupabaseCallback handles GET /api/v1/accounts/:account/supabase/callback.
// WorkOS redirects the browser here after the Supabase OAuth flow completes; we
// confirm the token landed and bounce back to the frontend where the flow began.
func SupabaseCallback(log *logger.Logger, pipesClient *pipes.Client, cfg SupabaseHandlerConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.Redirect(http.StatusFound, cfg.FrontendURL+appendParam(defaultSupabaseDest, "supabase_error=not_authenticated"))
			return
		}

		redirectTo := c.Query("redirect_to")
		if !isSafeRedirectPath(redirectTo) {
			redirectTo = defaultSupabaseDest
		}

		if _, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		}); err != nil {
			log.Error("supabase: token unavailable after callback", "error", err, "user", session.UserID)
			c.Redirect(http.StatusFound, cfg.FrontendURL+appendParam(redirectTo, "supabase_error=token_unavailable"))
			return
		}

		c.Redirect(http.StatusFound, cfg.FrontendURL+appendParam(redirectTo, "supabase_connected=true"))
	}
}

// SupabaseStatus handles GET /api/v1/accounts/:account/supabase/status.
// Connected means WorkOS can serve a token — token freshness is WorkOS's concern
// (it refreshes on demand), so an expired access token still reads as connected.
func SupabaseStatus(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if _, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		}); err != nil {
			c.JSON(http.StatusOK, gin.H{"connected": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"connected": true})
	}
}

// SupabaseListProjects handles GET /api/v1/accounts/:account/supabase/projects.
// Fetches a WorkOS-managed access token and lists the user's Supabase projects.
func SupabaseListProjects(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pipes.ErrNotInstalled) || errors.Is(err, pipes.ErrNeedsReauthorization) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "supabase_not_connected"})
				return
			}
			log.Error("supabase: get access token for projects", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve token"})
			return
		}

		projects, err := supabaseFetchProjects(c.Request.Context(), token.AccessToken)
		if err != nil {
			if errors.Is(err, errSupabaseUnauthorized) {
				// WorkOS served a token but Supabase rejected it — treat as a
				// dead connection and prompt a reconnect.
				log.Warn("supabase: token rejected by API, prompting reconnect", "user", session.UserID)
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "supabase_not_connected"})
				return
			}
			log.Error("supabase: list projects", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list Supabase projects"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"projects": projects})
	}
}

// supabaseProjectRefPattern matches a Supabase project ref (20 lowercase
// alphanumerics). Used to validate the path param before building the API URL.
var supabaseProjectRefPattern = regexp.MustCompile(`^[a-z0-9]{16,40}$`)

// SupabaseProjectHealth handles GET /api/v1/accounts/:account/supabase/projects/:ref/health.
// Returns the live health of a Supabase project's database service, used to show
// a status tile on a Supabase-backed knowledge store. The response is proxied
// from Supabase verbatim (we don't impose a schema on the provider's payload).
func SupabaseProjectHealth(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		ref := c.Param("ref")
		if !supabaseProjectRefPattern.MatchString(ref) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project ref"})
			return
		}

		token, err := pipesClient.GetAccessToken(c.Request.Context(), pipes.GetAccessTokenInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		})
		if err != nil {
			if errors.Is(err, pipes.ErrNotInstalled) || errors.Is(err, pipes.ErrNeedsReauthorization) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "supabase_not_connected"})
				return
			}
			log.Error("supabase: get access token for health", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve token"})
			return
		}

		services, err := supabaseFetchHealth(c.Request.Context(), token.AccessToken, ref)
		if err != nil {
			if errors.Is(err, errSupabaseUnauthorized) {
				c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "supabase_not_connected"})
				return
			}
			log.Error("supabase: get project health", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get project health"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"services": services})
	}
}

// SupabaseDisconnect handles DELETE /api/v1/accounts/:account/supabase.
func SupabaseDisconnect(log *logger.Logger, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		session, ok := middleware.GetSession(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
			return
		}
		if err := pipesClient.DeleteConnection(c.Request.Context(), pipes.DeleteConnectionInput{
			Provider:       supabaseProvider,
			UserID:         session.UserID,
			OrganizationID: session.OrganizationID,
		}); err != nil {
			log.Error("supabase: delete connection", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to disconnect"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"disconnected": true})
	}
}

// errSupabaseUnauthorized is returned when the Supabase API rejects the token
// (revoked or expired). Handlers check this with errors.Is.
var errSupabaseUnauthorized = fmt.Errorf("supabase token unauthorized")

// truncateBody caps a response body slice to a fixed size for safe inclusion in
// error messages and logs. Supabase API errors can return large or potentially
// sensitive payloads, so we never log more than 256 bytes.
func truncateBody(b []byte) []byte {
	const max = 256
	if len(b) > max {
		return append(append([]byte{}, b[:max]...), []byte("…")...)
	}
	return b
}

func supabaseFetchProjects(ctx context.Context, accessToken string) ([]SupabaseProject, error) {
	return supabaseFetchProjectsFromURL(ctx, accessToken, supabaseAPIBase)
}

func supabaseFetchProjectsFromURL(ctx context.Context, accessToken, baseURL string) ([]SupabaseProject, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/projects", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := supabaseHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errSupabaseUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase projects endpoint returned %d: %s", resp.StatusCode, truncateBody(raw))
	}

	var projects []SupabaseProject
	if err := json.Unmarshal(raw, &projects); err != nil {
		return nil, fmt.Errorf("decode projects response: %w", err)
	}

	return projects, nil
}

// supabasePoolerOverlay resolves a Supabase project's session-pooler connection
// coordinates via the Management API and returns them as a credential overlay
// (HOST/PORT/USERNAME[/DATABASE]) to merge over a store's credentials. Session
// mode always uses port 5432. Errors are returned raw for the caller to map;
// use respondSupabaseResolveError for a standard HTTP response.
func supabasePoolerOverlay(ctx context.Context, pipesClient *pipes.Client, userID, orgID, ref string) (map[string]string, error) {
	token, err := pipesClient.GetAccessToken(ctx, pipes.GetAccessTokenInput{
		Provider:       supabaseProvider,
		UserID:         userID,
		OrganizationID: orgID,
	})
	if err != nil {
		return nil, err
	}
	pooler, err := supabaseFetchPoolerConfig(ctx, token.AccessToken, ref)
	if err != nil {
		return nil, err
	}
	overlay := map[string]string{
		"HOST":     pooler.DBHost,
		"PORT":     "5432",
		"USERNAME": pooler.DBUser,
	}
	if pooler.DBName != "" {
		overlay["DATABASE"] = pooler.DBName
	}
	return overlay, nil
}

// respondSupabaseResolveError writes a standard HTTP response for a pooler
// resolution failure: a disconnected/expired Supabase connection reads as 422
// supabase_not_connected; anything else is a 502.
func respondSupabaseResolveError(c *gin.Context, log *logger.Logger, err error) {
	if errors.Is(err, pipes.ErrNotInstalled) || errors.Is(err, pipes.ErrNeedsReauthorization) || errors.Is(err, errSupabaseUnauthorized) {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "supabase_not_connected"})
		return
	}
	log.Error("supabase: resolve pooler config", "error", err)
	c.JSON(http.StatusBadGateway, gin.H{"error": "failed to resolve Supabase connection details"})
}

// SupabasePoolerConfig holds the connection-pooler (Supavisor) coordinates for a
// project's primary database, as reported by the Management API. We connect
// Supabase-backed knowledge stores through this pooler instead of the direct
// db.<ref>.supabase.co endpoint: the direct endpoint is IPv6-only and therefore
// unreachable from our IPv4-only VPCs, whereas the pooler is IPv4-reachable.
// The pooler host (which Supavisor cluster, aws-0 vs aws-1) and user
// (postgres.<ref>) are not derivable from the region, so they must come from
// the API rather than be constructed client-side.
type SupabasePoolerConfig struct {
	DatabaseType string `json:"database_type"` // "PRIMARY" for the writable db; others are read replicas
	DBHost       string `json:"db_host"`
	DBName       string `json:"db_name"`
	DBPort       int    `json:"db_port"`
	DBUser       string `json:"db_user"`
}

// supabaseFetchPoolerConfig returns the connection-pooler coordinates for a
// project's database from the Supabase Management API.
func supabaseFetchPoolerConfig(ctx context.Context, accessToken, ref string) (SupabasePoolerConfig, error) {
	return supabaseFetchPoolerConfigFromURL(ctx, accessToken, supabaseAPIBase, ref)
}

func supabaseFetchPoolerConfigFromURL(ctx context.Context, accessToken, baseURL, ref string) (SupabasePoolerConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v1/projects/"+ref+"/config/database/pooler", nil)
	if err != nil {
		return SupabasePoolerConfig{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := supabaseHTTPClient.Do(req)
	if err != nil {
		return SupabasePoolerConfig{}, err
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return SupabasePoolerConfig{}, errSupabaseUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return SupabasePoolerConfig{}, fmt.Errorf("supabase pooler config endpoint returned %d: %s", resp.StatusCode, truncateBody(raw))
	}

	// The endpoint returns an array with one entry per database (primary + any
	// read replicas); some API versions return a bare object. Accept both, and
	// prefer the PRIMARY (writable) entry — read replicas can't accept writes.
	var many []SupabasePoolerConfig
	if err := json.Unmarshal(raw, &many); err == nil && len(many) > 0 {
		var fallback SupabasePoolerConfig
		for _, p := range many {
			if p.DBHost == "" {
				continue
			}
			if p.DatabaseType == "PRIMARY" {
				return p, nil
			}
			if fallback.DBHost == "" {
				fallback = p
			}
		}
		if fallback.DBHost != "" {
			return fallback, nil
		}
	}
	var one SupabasePoolerConfig
	if err := json.Unmarshal(raw, &one); err == nil && one.DBHost != "" {
		return one, nil
	}
	return SupabasePoolerConfig{}, fmt.Errorf("supabase pooler config: no db_host in response")
}

// supabaseFetchHealth returns the raw services-health array for a project. We
// proxy it verbatim (json.RawMessage) rather than decode into a fixed schema, so
// the client renders exactly what Supabase reports.
func supabaseFetchHealth(ctx context.Context, accessToken, ref string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		supabaseAPIBase+"/v1/projects/"+ref+"/health?services=db", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := supabaseHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized {
		return nil, errSupabaseUnauthorized
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("supabase health endpoint returned %d: %s", resp.StatusCode, truncateBody(raw))
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("supabase health endpoint returned invalid JSON")
	}
	return json.RawMessage(raw), nil
}
