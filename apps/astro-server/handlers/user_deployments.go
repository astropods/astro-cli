package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/listcache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const (
	userDeploymentCachePrefix  = "usr:list:deployments:v2:"
	userDeploymentDefaultLimit = 50
	userDeploymentMaxLimit     = 100
)

// UserDeploymentsResponse is one globally ordered page across the explicitly
// selected memberships.
type UserDeploymentsResponse struct {
	Deployments      []AgentDeploymentSummary `json:"deployments"`
	Page             UserResourcePage         `json:"page"`
	Scope            UserResourceScope        `json:"scope"`
	RejectedAccounts []string                 `json:"rejected_accounts,omitempty"`
}

type userDeploymentCursor struct {
	DeployedAt string `json:"deployed_at"`
	ID         string `json:"id"`
}

type userDeploymentRequest struct {
	limit  int
	query  string
	cursor *deploymentstore.UserDeploymentCursor
	scope  userResourceScopeRequest
}

func parseUserDeploymentRequest(
	c *gin.Context,
	memberships []account.AccountWithRole,
) (userDeploymentRequest, error) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if len(query) > maxListQueryLen {
		return userDeploymentRequest{}, fmt.Errorf("q must be at most %d characters", maxListQueryLen)
	}
	scope, err := parseUserResourceScope(c, memberships)
	if err != nil {
		return userDeploymentRequest{}, err
	}

	limit := userDeploymentDefaultLimit
	if raw := strings.TrimSpace(c.Query("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			return userDeploymentRequest{}, fmt.Errorf("limit must be a positive integer")
		}
		limit = min(parsed, userDeploymentMaxLimit)
	}
	if _, supplied := c.GetQuery("offset"); supplied {
		return userDeploymentRequest{}, fmt.Errorf("offset is not supported; use cursor")
	}

	var cursor *deploymentstore.UserDeploymentCursor
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			return userDeploymentRequest{}, fmt.Errorf("cursor is invalid")
		}
		var wire userDeploymentCursor
		if err := json.Unmarshal(decoded, &wire); err != nil || wire.ID == "" {
			return userDeploymentRequest{}, fmt.Errorf("cursor is invalid")
		}
		deployedAt, err := time.Parse(time.RFC3339Nano, wire.DeployedAt)
		if err != nil {
			return userDeploymentRequest{}, fmt.Errorf("cursor is invalid")
		}
		cursor = &deploymentstore.UserDeploymentCursor{DeployedAt: deployedAt, ID: wire.ID}
	}

	return userDeploymentRequest{
		limit:  limit,
		query:  query,
		cursor: cursor,
		scope:  scope,
	}, nil
}

func encodeUserDeploymentCursor(deployment *deploymentstore.Deployment) (string, error) {
	if deployment == nil {
		return "", nil
	}
	wire, err := json.Marshal(userDeploymentCursor{
		DeployedAt: deployment.DeployedAt.Format(time.RFC3339Nano),
		ID:         deployment.ID,
	})
	if err != nil {
		return "", fmt.Errorf("encode user deployment cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(wire), nil
}

func userDeploymentCacheIdentity(request userDeploymentRequest) any {
	cursor := ""
	if request.cursor != nil {
		cursor = request.cursor.DeployedAt.Format(time.RFC3339Nano) + "/" + request.cursor.ID
	}
	return struct {
		Limit  int    `json:"limit"`
		Query  string `json:"query"`
		Cursor string `json:"cursor"`
	}{
		Limit:  request.limit,
		Query:  request.query,
		Cursor: cursor,
	}
}

type userDeploymentListDependencies struct {
	log         *logger.Logger
	deployments *deploymentstore.Store
	agents      *agentindex.Index
	audit       *auditlog.Store
}

// ListUserDeployments serves GET /me/deployments. Unlike the original #1728
// implementation, it executes one membership-guarded global page rather than
// applying the same offset independently to every account.
func ListUserDeployments(
	log *logger.Logger,
	accountStore *account.AccountStore,
	deployStore *deploymentstore.Store,
	agentIndex *agentindex.Index,
	auditStore *auditlog.Store,
	cache k8scache.Cache,
) gin.HandlerFunc {
	dependencies := userDeploymentListDependencies{
		log:         log,
		deployments: deployStore,
		agents:      agentIndex,
		audit:       auditStore,
	}
	return serveUserResourceList(userResourceListConfig[userDeploymentRequest]{
		log:           log,
		accounts:      accountStore,
		cache:         cache,
		resource:      "deployments",
		timingName:    "user-deployments",
		cachePrefix:   userDeploymentCachePrefix,
		parse:         parseUserDeploymentRequest,
		scope:         func(request userDeploymentRequest) userResourceScopeRequest { return request.scope },
		cacheIdentity: userDeploymentCacheIdentity,
		cursorPresent: func(request userDeploymentRequest) bool { return request.cursor != nil },
		// The generation vector covers the selected deployment accounts.
		// latest_build_id may come from a publisher/source account discovered only
		// after the page loads, so publisher-only changes can remain visible for at
		// most the 30-second remote TTL. Preloading those IDs would bypass hot hits.
		generations: deploycache.Generations,
		load: func(ctx context.Context, userID string, accountIDs []string, request userDeploymentRequest) (listcache.LoadResult, error) {
			rows, err := deployStore.ListVisibleDeploymentsForUserPage(
				ctx,
				userID,
				accountIDs,
				request.query,
				request.limit+1,
				request.cursor,
			)
			if err != nil {
				return listcache.LoadResult{}, err
			}
			hasMore := len(rows) > request.limit
			if hasMore {
				rows = rows[:request.limit]
			}
			deployments, enrichmentErr := enrichUserDeploymentRows(ctx, dependencies, rows)
			nextCursor := ""
			if hasMore && len(rows) > 0 {
				nextCursor, err = encodeUserDeploymentCursor(rows[len(rows)-1].Deployment)
				if err != nil {
					return listcache.LoadResult{}, err
				}
			}
			body, err := json.Marshal(UserDeploymentsResponse{
				Deployments: deployments,
				Page: UserResourcePage{
					Limit:      request.limit,
					NextCursor: nextCursor,
				},
				Scope: UserResourceScope{
					Accounts: request.scope.canonicalAccounts,
					All:      request.scope.all,
				},
			})
			if err != nil {
				return listcache.LoadResult{}, err
			}
			return listcache.LoadResult{
				Response: listcache.Response{
					Data:              body,
					ResultCount:       len(deployments),
					NextCursorPresent: nextCursor != "",
				},
				RemoteCacheable: enrichmentErr == nil,
			}, nil
		},
	})
}

func enrichUserDeploymentRows(
	ctx context.Context,
	dependencies userDeploymentListDependencies,
	rows []deploymentstore.UserDeployment,
) ([]AgentDeploymentSummary, error) {
	if len(rows) == 0 {
		return []AgentDeploymentSummary{}, nil
	}

	deploymentIDs := make([]string, 0, len(rows))
	accountIDSet := make(map[string]struct{})
	lineageRefSet := make(map[agentindex.AgentVersionRef]struct{})
	lineageByDeployment := make(map[string]agentindex.AgentVersionRef, len(rows))
	for _, row := range rows {
		deployment := row.Deployment
		deploymentIDs = append(deploymentIDs, deployment.ID)
		accountIDSet[deployment.AccountID] = struct{}{}
		publisherID := deployment.AccountID
		if deployment.SourceAccountID != nil && *deployment.SourceAccountID != "" {
			publisherID = *deployment.SourceAccountID
		}
		ref := agentindex.AgentVersionRef{AccountID: publisherID, Name: deployment.AgentName}
		lineageRefSet[ref] = struct{}{}
		lineageByDeployment[deployment.ID] = ref
	}
	accountIDs := make([]string, 0, len(accountIDSet))
	for id := range accountIDSet {
		accountIDs = append(accountIDs, id)
	}
	lineageRefs := make([]agentindex.AgentVersionRef, 0, len(lineageRefSet))
	for ref := range lineageRefSet {
		lineageRefs = append(lineageRefs, ref)
	}

	var (
		messagingURLs map[string]string
		webConfigured map[string]bool
		latestAudit   map[string]auditlog.ResourceLatest
		deployAudit   map[string]auditlog.ResourceLatest
		latestBuilds  map[string]agentindex.LatestBuildInfo
	)
	var group errgroup.Group
	group.Go(func() error {
		var err error
		messagingURLs, err = dependencies.deployments.GetMessagingURLsContext(ctx, deploymentIDs)
		if err != nil {
			dependencies.log.Warn("Failed to batch deployment messaging URLs", "error", err)
		}
		return err
	})
	group.Go(func() error {
		var err error
		webConfigured, err = dependencies.deployments.GetMessagingWebConfigured(ctx, deploymentIDs)
		if err != nil {
			dependencies.log.Warn("Failed to batch messaging web flags", "error", err)
		}
		return err
	})
	if dependencies.audit != nil {
		group.Go(func() error {
			var err error
			latestAudit, err = dependencies.audit.LatestPerResources(ctx, accountIDs, "deployment", deploymentIDs)
			if err != nil {
				dependencies.log.Warn("Failed to batch deployment audit timestamps", "error", err)
			}
			return err
		})
		group.Go(func() error {
			var err error
			deployAudit, err = dependencies.audit.LatestPerResourcesByAction(
				ctx,
				accountIDs,
				auditlog.DeploymentDeploy,
				"deployment",
				deploymentIDs,
			)
			if err != nil {
				dependencies.log.Warn("Failed to batch deployment authors", "error", err)
			}
			return err
		})
	}
	if dependencies.agents != nil {
		group.Go(func() error {
			var err error
			latestBuilds, err = dependencies.agents.BatchLatestBuildInfo(ctx, lineageRefs)
			if err != nil {
				dependencies.log.Warn("Failed to batch latest deployment builds", "error", err)
			}
			return err
		})
	}
	enrichmentErr := group.Wait()

	result := make([]AgentDeploymentSummary, 0, len(rows))
	for _, row := range rows {
		deployment := row.Deployment
		base := agentDeploymentFromDB(dependencies.log, deployment)
		if url, ok := messagingURLs[deployment.ID]; ok {
			base.ExternalURLs = []ServiceEndpointInfo{{
				Name: "messaging", Type: "messaging", URL: url, Ready: true,
			}}
		}
		latestBuildID := ""
		ref := lineageByDeployment[deployment.ID]
		if latest, ok := latestBuilds[ref.AccountID+"/"+ref.Name]; ok {
			if ref.AccountID == deployment.AccountID || latest.Visibility == "public" {
				latestBuildID = latest.BuildID
			}
		}
		updatedAt := ""
		if latest, ok := latestAudit[deployment.ID]; ok {
			updatedAt = latest.UpdatedAt.Format(time.RFC3339)
		}
		deployedBy := ""
		if latest, ok := deployAudit[deployment.ID]; ok {
			deployedBy = latest.ActorID
		}
		result = append(result, AgentDeploymentSummary{
			ID:                     base.ID,
			Name:                   base.Name,
			DisplayName:            base.DisplayName,
			AvatarColors:           base.AvatarColors,
			BuildID:                base.BuildID,
			LatestBuildID:          latestBuildID,
			Status:                 base.Status,
			Namespace:              base.Namespace,
			AccountID:              deployment.AccountID,
			AccountName:            row.AccountName,
			ExternalURLs:           base.ExternalURLs,
			MessagingWebConfigured: webConfigured[deployment.ID],
			CreatedAt:              deployment.DeployedAt.Format(time.RFC3339),
			UpdatedAt:              updatedAt,
			DeployedBy:             deployedBy,
		})
	}
	return result, enrichmentErr
}
