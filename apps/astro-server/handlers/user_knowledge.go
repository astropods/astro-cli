package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgecache"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/listcache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/gin-gonic/gin"
)

const userKnowledgeCachePrefix = "usr:list:knowledge:v2:"

type UserKnowledgeResponse struct {
	Stores           []KnowledgeResponse `json:"stores"`
	Page             UserResourcePage    `json:"page"`
	Scope            UserResourceScope   `json:"scope"`
	RejectedAccounts []string            `json:"rejected_accounts,omitempty"`
}

type userKnowledgeCursorWire struct {
	CreatedAt string `json:"created_at"`
	ID        string `json:"id"`
}

type userKnowledgeRequest struct {
	limit  int
	query  string
	cursor *knowledgestore.UserKnowledgeCursor
	scope  userResourceScopeRequest
}

func parseUserKnowledgeRequest(c *gin.Context, memberships []account.AccountWithRole) (userKnowledgeRequest, error) {
	query := strings.ToLower(strings.TrimSpace(c.Query("q")))
	if len(query) > maxListQueryLen {
		return userKnowledgeRequest{}, fmt.Errorf("q must be at most %d characters", maxListQueryLen)
	}
	scope, err := parseUserResourceScope(c, memberships)
	if err != nil {
		return userKnowledgeRequest{}, err
	}
	if _, supplied := c.GetQuery("offset"); supplied {
		return userKnowledgeRequest{}, fmt.Errorf("offset is not supported; use cursor")
	}
	limit, _, err := parseListPagination(c)
	if err != nil {
		return userKnowledgeRequest{}, err
	}
	var cursor *knowledgestore.UserKnowledgeCursor
	if raw := strings.TrimSpace(c.Query("cursor")); raw != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil {
			return userKnowledgeRequest{}, fmt.Errorf("cursor is invalid")
		}
		var wire userKnowledgeCursorWire
		if err := json.Unmarshal(decoded, &wire); err != nil || wire.ID == "" {
			return userKnowledgeRequest{}, fmt.Errorf("cursor is invalid")
		}
		createdAt, err := time.Parse(time.RFC3339Nano, wire.CreatedAt)
		if err != nil {
			return userKnowledgeRequest{}, fmt.Errorf("cursor is invalid")
		}
		cursor = &knowledgestore.UserKnowledgeCursor{CreatedAt: createdAt, ID: wire.ID}
	}
	return userKnowledgeRequest{limit: limit, query: query, cursor: cursor, scope: scope}, nil
}

func encodeUserKnowledgeCursor(store *knowledgestore.KnowledgeStore) (string, error) {
	if store == nil {
		return "", nil
	}
	data, err := json.Marshal(userKnowledgeCursorWire{CreatedAt: store.CreatedAt.Format(time.RFC3339Nano), ID: store.ID})
	if err != nil {
		return "", fmt.Errorf("encode user knowledge cursor: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func userKnowledgeCacheIdentity(request userKnowledgeRequest) any {
	cursor := ""
	if request.cursor != nil {
		cursor = request.cursor.CreatedAt.Format(time.RFC3339Nano) + "/" + request.cursor.ID
	}
	return struct {
		Cursor string `json:"cursor"`
		Limit  int    `json:"limit"`
		Query  string `json:"query"`
	}{
		Cursor: cursor,
		Limit:  request.limit,
		Query:  request.query,
	}
}

func ListUserKnowledgeStores(
	log *logger.Logger,
	accountStore *account.AccountStore,
	store *knowledgestore.Store,
	cache k8scache.Cache,
) gin.HandlerFunc {
	return serveUserResourceList(userResourceListConfig[userKnowledgeRequest]{
		log:           log,
		accounts:      accountStore,
		cache:         cache,
		resource:      "knowledge stores",
		timingName:    "user-knowledge",
		cachePrefix:   userKnowledgeCachePrefix,
		parse:         parseUserKnowledgeRequest,
		scope:         func(request userKnowledgeRequest) userResourceScopeRequest { return request.scope },
		cacheIdentity: userKnowledgeCacheIdentity,
		cursorPresent: func(request userKnowledgeRequest) bool { return request.cursor != nil },
		generations:   knowledgecache.Generations,
		load: func(ctx context.Context, userID string, accountIDs []string, request userKnowledgeRequest) (listcache.LoadResult, error) {
			rows, err := store.ListVisibleForUserPage(ctx, userID, accountIDs, request.query, request.limit+1, request.cursor)
			if err != nil {
				return listcache.LoadResult{}, err
			}
			hasMore := len(rows) > request.limit
			if hasMore {
				rows = rows[:request.limit]
			}
			responses := make([]KnowledgeResponse, 0, len(rows))
			for _, row := range rows {
				response := toKnowledgeResponse(row.Store)
				response.AccountID = row.Store.AccountID
				response.Account = row.AccountName
				responses = append(responses, response)
			}
			nextCursor := ""
			if hasMore && len(rows) > 0 {
				nextCursor, err = encodeUserKnowledgeCursor(rows[len(rows)-1].Store)
				if err != nil {
					return listcache.LoadResult{}, err
				}
			}
			data, err := json.Marshal(UserKnowledgeResponse{
				Stores: responses,
				Page:   UserResourcePage{Limit: request.limit, NextCursor: nextCursor},
				Scope:  UserResourceScope{Accounts: request.scope.canonicalAccounts, All: request.scope.all},
			})
			return listcache.LoadResult{
				Response: listcache.Response{
					Data:              data,
					ResultCount:       len(responses),
					NextCursorPresent: nextCursor != "",
				},
				RemoteCacheable: true,
			}, err
		},
	})
}
