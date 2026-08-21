package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/blueprintcache"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/listcache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

const userBlueprintCachePrefix = "usr:list:blueprints:v2:"

type UserBlueprintsResponse struct {
	Blueprints       []AgentResponse   `json:"blueprints"`
	Page             UserResourcePage  `json:"page"`
	Scope            UserResourceScope `json:"scope"`
	RejectedAccounts []string          `json:"rejected_accounts,omitempty"`
}

type userBlueprintCursorWire struct {
	Sort        string  `json:"sort"`
	PublishedAt *string `json:"published_at,omitempty"`
	Name        string  `json:"name"`
	AccountID   string  `json:"account_id"`
}

type userBlueprintRequest struct {
	filters BlueprintListFilters
	cursor  *agentindex.UserBlueprintCursor
	scope   userResourceScopeRequest
}

func parseUserBlueprintRequest(c *gin.Context, memberships []account.AccountWithRole) (userBlueprintRequest, error) {
	scope, err := parseUserResourceScope(c, memberships)
	if err != nil {
		return userBlueprintRequest{}, err
	}
	if _, supplied := c.GetQuery("offset"); supplied {
		return userBlueprintRequest{}, fmt.Errorf("offset is not supported; use cursor")
	}
	filters, err := ParseBlueprintListFilters(c)
	if err != nil {
		return userBlueprintRequest{}, err
	}
	filters.Query = strings.ToLower(filters.Query)
	filters.Tag = strings.ToLower(filters.Tag)
	if filters.Sort == "newest" && len(scope.selected) > 1 {
		return userBlueprintRequest{}, fmt.Errorf("sort=newest supports one account until the broad-scope order has an index")
	}
	filters.Offset = 0

	var cursor *agentindex.UserBlueprintCursor
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			return userBlueprintRequest{}, fmt.Errorf("cursor is invalid")
		}
		var wire userBlueprintCursorWire
		if err := json.Unmarshal(decoded, &wire); err != nil || wire.Name == "" || wire.AccountID == "" || wire.Sort != filters.Sort {
			return userBlueprintRequest{}, fmt.Errorf("cursor is invalid")
		}
		cursor = &agentindex.UserBlueprintCursor{Sort: wire.Sort, Name: wire.Name, AccountID: wire.AccountID}
		if wire.PublishedAt != nil {
			publishedAt, err := time.Parse(time.RFC3339Nano, *wire.PublishedAt)
			if err != nil {
				return userBlueprintRequest{}, fmt.Errorf("cursor is invalid")
			}
			cursor.PublishedAt = &publishedAt
		}
	}
	return userBlueprintRequest{filters: filters, cursor: cursor, scope: scope}, nil
}

func encodeUserBlueprintCursor(row agentindex.UserBlueprint, sort string) (string, error) {
	wire := userBlueprintCursorWire{Sort: sort, Name: row.Agent.Name, AccountID: row.Agent.AccountID}
	if sort == "newest" && row.PublishedAt != nil {
		value := row.PublishedAt.Format(time.RFC3339Nano)
		wire.PublishedAt = &value
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return "", fmt.Errorf("encode user blueprint cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func userBlueprintCacheIdentity(request userBlueprintRequest) any {
	return struct {
		Filters BlueprintListFilters     `json:"filters"`
		Cursor  *userBlueprintCursorWire `json:"cursor,omitempty"`
	}{
		Filters: request.filters,
		Cursor:  blueprintCursorWire(request.cursor),
	}
}

func blueprintCursorWire(cursor *agentindex.UserBlueprintCursor) *userBlueprintCursorWire {
	if cursor == nil {
		return nil
	}
	wire := &userBlueprintCursorWire{Sort: cursor.Sort, Name: cursor.Name, AccountID: cursor.AccountID}
	if cursor.PublishedAt != nil {
		value := cursor.PublishedAt.Format(time.RFC3339Nano)
		wire.PublishedAt = &value
	}
	return wire
}

func userBlueprintResponses(
	rows []agentindex.UserBlueprint,
	metadata map[string]agentindex.UserBlueprintMetadata,
	avatarStore *avatar.Store,
) []AgentResponse {
	responses := make([]AgentResponse, 0, len(rows))
	for _, row := range rows {
		agent := row.Agent
		versions := make([]AgentVersionResponse, 0, len(agent.Versions))
		for _, version := range agent.Versions {
			versions = append(versions, buildVersionResponse(version))
		}
		meta := metadata[agent.AccountID+"/"+agent.Name]
		publishers := make([]AgentPublisher, 0, len(meta.Publishers))
		for _, publisher := range meta.Publishers {
			publishers = append(publishers, AgentPublisher{Name: publisher.Name, Account: publisher.Account})
		}
		response := AgentResponse{
			Account: row.AccountName, Name: agent.Name, Registry: agent.Registry, Visibility: agent.Visibility,
			ArchivedAt: agent.ArchivedAt, NameReserved: agent.NameReserved, Versions: versions,
			HeartCount: meta.HeartCount, Metrics: agentMetrics(meta.LifetimeMessages, meta.DeployCount), Publishers: publishers,
		}
		if avatarStore != nil {
			response.AvatarURL = avatarStore.AgentAvatarURL(row.AccountName, agent.Name, agent.AvatarUpdatedAt)
		}
		if agent.AvatarColors != nil {
			response.AvatarColors = append(json.RawMessage(nil), (*agent.AvatarColors)...)
		}
		responses = append(responses, response)
	}
	return responses
}

// ListUserBlueprints serves one globally ordered page across an explicit
// membership scope. Initial list rendering never calls WorkOS or object storage.
func ListUserBlueprints(
	log *logger.Logger,
	index *agentindex.Index,
	accountStore *account.AccountStore,
	avatarStore *avatar.Store,
	cache k8scache.Cache,
) gin.HandlerFunc {
	return serveUserResourceList(userResourceListConfig[userBlueprintRequest]{
		log:           log,
		accounts:      accountStore,
		cache:         cache,
		resource:      "blueprints",
		timingName:    "user-blueprints",
		cachePrefix:   userBlueprintCachePrefix,
		parse:         parseUserBlueprintRequest,
		scope:         func(request userBlueprintRequest) userResourceScopeRequest { return request.scope },
		cacheIdentity: userBlueprintCacheIdentity,
		cursorPresent: func(request userBlueprintRequest) bool { return request.cursor != nil },
		generations:   blueprintcache.Generations,
		load: func(ctx context.Context, userID string, accountIDs []string, request userBlueprintRequest) (listcache.LoadResult, error) {
			opts := toBlueprintListOptions(request.filters)
			opts.Limit = request.filters.Limit + 1
			rows, err := index.ListVisibleBlueprintsForUserPage(ctx, userID, accountIDs, opts, request.cursor)
			if err != nil {
				return listcache.LoadResult{}, err
			}
			hasMore := len(rows) > request.filters.Limit
			if hasMore {
				rows = rows[:request.filters.Limit]
			}
			refs := make([]agentindex.AgentVersionRef, 0, len(rows))
			for _, row := range rows {
				refs = append(refs, agentindex.AgentVersionRef{AccountID: row.Agent.AccountID, Name: row.Agent.Name})
			}
			metadata, enrichmentErr := index.BatchUserBlueprintMetadata(ctx, refs)
			if enrichmentErr != nil {
				log.Warn("user blueprints: batch user blueprint metadata failed", "error", enrichmentErr)
			}
			nextCursor := ""
			if hasMore && len(rows) > 0 {
				nextCursor, err = encodeUserBlueprintCursor(rows[len(rows)-1], request.filters.Sort)
				if err != nil {
					return listcache.LoadResult{}, err
				}
			}
			data, err := json.Marshal(UserBlueprintsResponse{
				Blueprints: userBlueprintResponses(rows, metadata, avatarStore),
				Page:       UserResourcePage{Limit: request.filters.Limit, NextCursor: nextCursor},
				Scope:      UserResourceScope{Accounts: request.scope.canonicalAccounts, All: request.scope.all},
			})
			return listcache.LoadResult{
				Response: listcache.Response{
					Data:              data,
					ResultCount:       len(rows),
					NextCursorPresent: nextCursor != "",
				},
				RemoteCacheable: enrichmentErr == nil,
			}, err
		},
	})
}
