package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/listcache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

const (
	userResourceLocalTTL         = 5 * time.Second
	userResourceRemoteTTL        = 30 * time.Second
	userResourceLocalItems       = 1024
	maxUserResourceAccountParams = 50
)

type UserResourcePage struct {
	Limit      int    `json:"limit"`
	NextCursor string `json:"next_cursor,omitempty"`
}

type UserResourceScope struct {
	Accounts []string `json:"accounts"`
	All      bool     `json:"all"`
}

type userResourceScopeRequest struct {
	selected          []account.AccountWithRole
	rejected          []string
	all               bool
	canonicalAccounts []string
}

func selectCrossAccountMemberships(
	memberships []account.AccountWithRole,
	requested []string,
) ([]account.AccountWithRole, []string) {
	if len(requested) == 0 {
		return memberships, []string{}
	}

	byName := make(map[string]account.AccountWithRole, len(memberships))
	for _, membership := range memberships {
		byName[membership.Name] = membership
	}

	selected := make([]account.AccountWithRole, 0, len(requested))
	rejected := make([]string, 0)
	seen := make(map[string]struct{}, len(requested))
	for _, raw := range requested {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		membership, ok := byName[name]
		if !ok {
			rejected = append(rejected, name)
			continue
		}
		selected = append(selected, membership)
	}
	return selected, rejected
}

func parseUserResourceScope(c *gin.Context, memberships []account.AccountWithRole) (userResourceScopeRequest, error) {
	scope := strings.TrimSpace(c.Query("scope"))
	requested := c.QueryArray("account")
	if len(requested) > maxUserResourceAccountParams {
		return userResourceScopeRequest{}, fmt.Errorf("at most %d account values are allowed", maxUserResourceAccountParams)
	}
	if scope != "" && scope != "all" {
		return userResourceScopeRequest{}, fmt.Errorf("scope must be 'all' when supplied")
	}
	if scope == "all" && len(requested) > 0 {
		return userResourceScopeRequest{}, fmt.Errorf("scope=all and account cannot be combined")
	}
	if scope == "" && len(requested) == 0 {
		return userResourceScopeRequest{}, fmt.Errorf("at least one account or scope=all is required")
	}

	selected, rejected := selectCrossAccountMemberships(memberships, requested)
	if scope == "all" {
		// scope=all is intentionally not capped: unlike explicit account values,
		// this list comes from the authoritative membership lookup and cannot be
		// expanded with caller-supplied cache dimensions. Large legitimate scopes
		// are monitored in serveUserResourceList rather than rejected here.
		selected = append([]account.AccountWithRole(nil), memberships...)
		rejected = nil
	}
	if scope == "" && len(selected) == 0 && len(rejected) == 0 {
		return userResourceScopeRequest{}, fmt.Errorf("at least one non-empty account is required")
	}
	canonicalAccounts := make([]string, 0, len(selected))
	for _, membership := range selected {
		canonicalAccounts = append(canonicalAccounts, membership.Name)
	}
	sort.Strings(canonicalAccounts)
	sort.Strings(rejected)
	return userResourceScopeRequest{
		selected:          selected,
		rejected:          rejected,
		all:               scope == "all",
		canonicalAccounts: canonicalAccounts,
	}, nil
}

type userResourceListConfig[Request any] struct {
	log           *logger.Logger
	accounts      *account.AccountStore
	cache         k8scache.Cache
	resource      string
	timingName    string
	cachePrefix   string
	parse         func(*gin.Context, []account.AccountWithRole) (Request, error)
	scope         func(Request) userResourceScopeRequest
	cacheIdentity func(Request) any
	cursorPresent func(Request) bool
	generations   func(context.Context, k8scache.Cache, []string) []string
	load          func(context.Context, string, []string, Request) (listcache.LoadResult, error)
}

func userResourceCacheKey(
	userID string,
	scope userResourceScopeRequest,
	requestIdentity any,
	generations []string,
) (string, error) {
	payload, err := json.Marshal(struct {
		User        string   `json:"user"`
		Accounts    []string `json:"accounts"`
		All         bool     `json:"all"`
		Request     any      `json:"request"`
		Generations []string `json:"generations"`
	}{
		User:        userID,
		Accounts:    scope.canonicalAccounts,
		All:         scope.all,
		Request:     requestIdentity,
		Generations: generations,
	})
	if err != nil {
		return "", fmt.Errorf("encode user resource cache key: %w", err)
	}
	digest := sha256.Sum256(payload)
	return fmt.Sprintf("%x", digest[:]), nil
}

func withRejectedAccounts(data []byte, rejected []string) ([]byte, error) {
	if len(rejected) == 0 {
		return data, nil
	}
	// Rejected names are request-specific and deliberately excluded from the
	// shared cache key, so they must be added after the cache lookup. RawMessage
	// keeps the page payload opaque while decoding only its top-level envelope.
	// Re-marshaling may reorder top-level keys; JSON object order is not part of
	// the response contract.
	var response map[string]json.RawMessage
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("decode user resource response: %w", err)
	}
	rejectedData, err := json.Marshal(rejected)
	if err != nil {
		return nil, fmt.Errorf("encode rejected accounts: %w", err)
	}
	response["rejected_accounts"] = rejectedData
	result, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("encode user resource response: %w", err)
	}
	return result, nil
}

// serveUserResourceList owns the common authenticated list lifecycle. Resource
// handlers supply only request identity, generation lookup, and typed loading.
func serveUserResourceList[Request any](config userResourceListConfig[Request]) gin.HandlerFunc {
	responseCache := listcache.New(
		config.cache,
		config.cachePrefix,
		userResourceLocalTTL,
		userResourceRemoteTTL,
		userResourceLocalItems,
	)
	return func(c *gin.Context) {
		startedAt := time.Now()
		user, ok := middleware.GetUser(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		memberships, err := config.accounts.GetAccountsForUserContext(c.Request.Context(), user.ID)
		if err != nil {
			config.log.Error(
				"Failed to load memberships for user resource list",
				"resource", config.resource,
				"error", err,
				"user_id", user.ID,
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load accounts"})
			return
		}
		request, err := config.parse(c, memberships)
		if err != nil {
			writeListFilterError(c, err)
			return
		}
		scope := config.scope(request)
		accountIDs := make([]string, 0, len(scope.selected))
		for _, membership := range scope.selected {
			accountIDs = append(accountIDs, membership.ID)
		}
		if scope.all && len(accountIDs) > maxUserResourceAccountParams {
			config.log.Warn(
				"Large all-account user resource scope",
				"resource", config.resource,
				"user_id", user.ID,
				"selected_account_count", len(accountIDs),
				"warning_threshold", maxUserResourceAccountParams,
			)
		}
		generations := config.generations(c.Request.Context(), config.cache, accountIDs)
		cacheKey, err := userResourceCacheKey(user.ID, scope, config.cacheIdentity(request), generations)
		if err != nil {
			config.log.Error("Failed to build user resource cache key", "resource", config.resource, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list " + config.resource})
			return
		}
		response, cacheResult, err := responseCache.GetOrLoad(
			c.Request.Context(),
			cacheKey,
			func(ctx context.Context) (listcache.LoadResult, error) {
				return config.load(ctx, user.ID, accountIDs, request)
			},
		)
		if err != nil {
			config.log.Error(
				"Failed to list user resources",
				"resource", config.resource,
				"error", err,
				"selected_account_count", len(accountIDs),
			)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list " + config.resource})
			return
		}
		responseData, err := withRejectedAccounts(response.Data, scope.rejected)
		if err != nil {
			config.log.Error("Failed to add rejected accounts to user resource response", "resource", config.resource, "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list " + config.resource})
			return
		}

		duration := time.Since(startedAt)
		c.Header("X-Astro-Cache", cacheResult)
		c.Header("Cache-Control", "private, no-store")
		c.Header("Server-Timing", fmt.Sprintf("%s;dur=%.2f", config.timingName, float64(duration.Microseconds())/1000))
		config.log.Info(
			"Listed user resources",
			"resource", config.resource,
			"selected_account_count", len(accountIDs),
			"scope_all", scope.all,
			"result_count", response.ResultCount,
			"request_cursor_present", config.cursorPresent(request),
			"next_cursor_present", response.NextCursorPresent,
			"cache", cacheResult,
			"duration_ms", duration.Milliseconds(),
		)
		c.Data(http.StatusOK, "application/json", responseData)
	}
}
