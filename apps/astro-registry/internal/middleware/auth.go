package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-registry/internal/auth"
	"github.com/astropods/astro/apps/astro-registry/internal/config"
	"github.com/astropods/astro/apps/astro-registry/internal/logger"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware provides authentication middleware for protecting routes
type AuthMiddleware struct {
	log              *logger.Logger
	cfg              *config.Config
	jwtValidator     *auth.JWTValidator
	registrySigner   *auth.RegistryTokenSigner
	registryService  string
	registryRealmCfg string
}

// NewAuthMiddleware creates a new auth middleware instance
func NewAuthMiddleware(log *logger.Logger, cfg *config.Config) *AuthMiddleware {
	// Note: WorkOS access tokens don't include an 'aud' claim, so we pass empty string
	// to skip audience validation. The issuer already includes the client ID for validation.
	jwtValidator := auth.NewJWTValidator(
		cfg.Auth.JWKSEndpoint,
		cfg.Auth.JWTIssuer,
		"", // No audience validation for WorkOS tokens
	)

	registrySigner := auth.NewRegistryTokenSigner(
		cfg.Auth.RegistryTokenSecret,
		cfg.Auth.RegistryTokenIssuer,
		cfg.Auth.RegistryTokenIssuer, // service name == issuer name
		cfg.Auth.RegistryTokenTTL,
	)

	return &AuthMiddleware{
		log:              log,
		cfg:              cfg,
		jwtValidator:     jwtValidator,
		registrySigner:   registrySigner,
		registryService:  cfg.Auth.RegistryTokenIssuer,
		registryRealmCfg: cfg.Auth.RegistryTokenRealm,
	}
}

// JWTValidator exposes the WorkOS JWT validator for handlers that need it
// (e.g. the /token endpoint).
func (m *AuthMiddleware) JWTValidator() *auth.JWTValidator {
	return m.jwtValidator
}

// RegistrySigner exposes the registry token signer for handlers (e.g. /token).
func (m *AuthMiddleware) RegistrySigner() *auth.RegistryTokenSigner {
	return m.registrySigner
}

// RequireAuth middleware ensures the request is authenticated via Bearer token.
// Accepts either:
//   - a registry-signed scope token (iss = astro-registry), or
//   - a WorkOS access token (legacy path, used by UI / non-Docker pulls).
//
// On any failure, returns 401 with a Distribution-spec WWW-Authenticate Bearer
// header pointing at the /token realm so the Docker daemon transparently
// re-exchanges credentials. See docs/03-architecture/registry-token-auth.md.
func (m *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			m.log.Warn("Auth failed: no Authorization header", "method", c.Request.Method, "path", c.Request.URL.Path)
			m.unauthorized(c, "Authorization header required")
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			m.log.Warn("Auth failed: not a Bearer token", "method", c.Request.Method, "path", c.Request.URL.Path)
			m.unauthorized(c, "Bearer token required")
			return
		}

		token := strings.TrimPrefix(authHeader, "Bearer ")
		if !m.authenticate(c, token) {
			m.unauthorized(c, "Invalid or expired token")
			return
		}

		c.Next()
	}
}

// authenticate routes the token to the registry-token verifier or the WorkOS
// verifier based on iss, then sets context values + (for registry tokens)
// enforces scope against the request path/method.
func (m *AuthMiddleware) authenticate(c *gin.Context, token string) bool {
	issuer, err := auth.PeekIssuer(token)
	if err != nil {
		m.log.Warn("Auth failed: cannot parse token", "error", err)
		return false
	}

	if issuer == m.cfg.Auth.RegistryTokenIssuer {
		return m.authenticateRegistryToken(c, token)
	}
	return m.authenticateWorkOSToken(c, token)
}

func (m *AuthMiddleware) authenticateRegistryToken(c *gin.Context, token string) bool {
	claims, err := m.registrySigner.Verify(token)
	if err != nil {
		m.log.Warn("Auth failed: registry token validation failed", "error", err)
		return false
	}

	// Enforce scope against the actual operation. Short paths (e.g. /v2/) and
	// the version check have no repository scope to assert.
	if repo, action, ok := repositoryAndAction(c.Request); ok {
		if !claims.HasAccess(repo, action) {
			m.log.Warn("Auth failed: registry token lacks scope",
				"repository", repo, "action", action,
				"method", c.Request.Method, "path", c.Request.URL.Path)
			return false
		}
	}

	user := &auth.User{ID: claims.Subject}
	session := &auth.Session{
		ID:     claims.ID,
		UserID: claims.Subject,
	}
	c.Set(string(auth.UserContextKey), user)
	c.Set(string(auth.SessionContextKey), session)
	c.Set(string(auth.RegistryClaimsContextKey), claims)
	return true
}

func (m *AuthMiddleware) authenticateWorkOSToken(c *gin.Context, token string) bool {
	claims, err := m.jwtValidator.ValidateToken(c.Request.Context(), token)
	if err != nil {
		m.log.Warn("Auth failed: WorkOS token validation failed", "error", err, "method", c.Request.Method, "path", c.Request.URL.Path)
		return false
	}

	session := &auth.Session{
		ID:             claims.SessionID,
		UserID:         claims.Subject,
		OrganizationID: claims.OrganizationID,
		Role:           claims.Role,
		Permissions:    claims.Permissions,
	}
	user := &auth.User{
		ID:             claims.Subject,
		OrganizationID: claims.OrganizationID,
	}
	c.Set(string(auth.UserContextKey), user)
	c.Set(string(auth.SessionContextKey), session)
	return true
}

// unauthorized writes a 401 with a Distribution-spec WWW-Authenticate Bearer
// challenge so the Docker daemon knows to call the realm.
func (m *AuthMiddleware) unauthorized(c *gin.Context, description string) {
	c.Header("WWW-Authenticate", m.bearerChallenge(c))
	c.AbortWithStatusJSON(http.StatusUnauthorized, auth.ErrorResponse{
		Error:       "unauthorized",
		Description: description,
	})
}

// bearerChallenge builds a WWW-Authenticate Bearer header per
// https://distribution.github.io/distribution/spec/auth/token/#requesting-a-token
func (m *AuthMiddleware) bearerChallenge(c *gin.Context) string {
	realm := m.realmURL(c)
	parts := []string{
		fmt.Sprintf(`realm=%q`, realm),
		fmt.Sprintf(`service=%q`, m.registryService),
	}
	if repo, action, ok := repositoryAndAction(c.Request); ok {
		actions := []string{action}
		if action == "push" {
			actions = []string{"push", "pull"}
		}
		parts = append(parts, fmt.Sprintf(`scope="repository:%s:%s"`, repo, strings.Join(actions, ",")))
	}
	return "Bearer " + strings.Join(parts, ",")
}

// realmURL returns the public URL of the /token endpoint advertised to clients.
// Sourced from REGISTRY_TOKEN_REALM, which is required at boot.
func (m *AuthMiddleware) realmURL(_ *gin.Context) string {
	return m.registryRealmCfg
}

// repositoryAndAction extracts the "<ns>/<image>" repository name and the
// required Distribution action ("pull" or "push") from a /v2/* request.
// Returns ok=false for the version check, the catalog, or any path that does
// not have at least <ns>/<image>.
func repositoryAndAction(r *http.Request) (string, string, bool) {
	// Strip /v2 prefix.
	p := strings.TrimPrefix(r.URL.Path, "/v2")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return "", "", false
	}
	parts := strings.Split(p, "/")
	if len(parts) < 2 {
		return "", "", false
	}
	// Repository path is everything up to the first /manifests, /blobs, or /tags segment.
	repoEnd := len(parts)
	for i, seg := range parts {
		if seg == "manifests" || seg == "blobs" || seg == "tags" {
			repoEnd = i
			break
		}
	}
	if repoEnd < 2 {
		return "", "", false
	}
	repo := strings.Join(parts[:repoEnd], "/")

	action := "pull"
	switch r.Method {
	case http.MethodPut, http.MethodPost, http.MethodPatch, http.MethodDelete:
		action = "push"
	}
	return repo, action, true
}

// GetUser retrieves the authenticated user from the context
func GetUser(c *gin.Context) (*auth.User, bool) {
	user, exists := c.Get(string(auth.UserContextKey))
	if !exists {
		return nil, false
	}
	u, ok := user.(*auth.User)
	return u, ok
}

// GetSession retrieves the session from the context
func GetSession(c *gin.Context) (*auth.Session, bool) {
	session, exists := c.Get(string(auth.SessionContextKey))
	if !exists {
		return nil, false
	}
	s, ok := session.(*auth.Session)
	return s, ok
}

// GetRegistryClaims returns the verified registry-token claims if the request
// authenticated via a registry-scope token, or (nil, false) otherwise.
func GetRegistryClaims(c *gin.Context) (*auth.RegistryTokenClaims, bool) {
	v, exists := c.Get(string(auth.RegistryClaimsContextKey))
	if !exists {
		return nil, false
	}
	claims, ok := v.(*auth.RegistryTokenClaims)
	return claims, ok
}
