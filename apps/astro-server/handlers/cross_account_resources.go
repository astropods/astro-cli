package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/knowledgestore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
	"golang.org/x/sync/errgroup"
)

const crossAccountResourceConcurrency = 6

type CrossAccountResourceResult[T any] struct {
	Account string `json:"account"`
	Data    T      `json:"data"`
	Count   int    `json:"count"`
	Limit   int    `json:"limit"`
	Offset  int    `json:"offset"`
	HasMore bool   `json:"has_more"`
}

type CrossAccountResourceResponse[T any] struct {
	Results          []CrossAccountResourceResult[T] `json:"results"`
	FailedAccounts   []string                        `json:"failed_accounts"`
	RejectedAccounts []string                        `json:"rejected_accounts"`
}

type CrossAccountBlueprintsResponse = CrossAccountResourceResponse[ListAgentsResponse]
type CrossAccountKnowledgeResponse = CrossAccountResourceResponse[[]KnowledgeResponse]

type crossAccountResourcePage[T any] struct {
	data    T
	count   int
	limit   int
	offset  int
	hasMore bool
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

func crossAccountMemberships(
	c *gin.Context,
	log *logger.Logger,
	accountStore *account.AccountStore,
) ([]account.AccountWithRole, []string, bool) {
	user, ok := middleware.GetUser(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return nil, nil, false
	}

	memberships, err := accountStore.GetAccountsForUser(user.ID)
	if err != nil {
		log.Error("Failed to load cross-account memberships", "error", err, "user_id", user.ID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load accounts"})
		return nil, nil, false
	}

	selected, rejected := selectCrossAccountMemberships(memberships, c.QueryArray("account"))
	return selected, rejected, true
}

func loadCrossAccountResources[T any](
	ctx context.Context,
	log *logger.Logger,
	resource string,
	accounts []account.AccountWithRole,
	load func(context.Context, account.AccountWithRole) (crossAccountResourcePage[T], error),
) ([]CrossAccountResourceResult[T], []string) {
	loaded := make([]CrossAccountResourceResult[T], len(accounts))
	failed := make([]bool, len(accounts))
	var group errgroup.Group
	group.SetLimit(crossAccountResourceConcurrency)

	for i, membership := range accounts {
		group.Go(func() error {
			page, err := load(ctx, membership)
			if err != nil {
				log.Warn(
					"Failed to load account in cross-account resource list",
					"resource", resource,
					"account", membership.Name,
					"account_id", membership.ID,
					"error", err,
				)
				failed[i] = true
				return nil
			}
			loaded[i] = CrossAccountResourceResult[T]{
				Account: membership.Name,
				Data:    page.data,
				Count:   page.count,
				Limit:   page.limit,
				Offset:  page.offset,
				HasMore: page.hasMore,
			}
			return nil
		})
	}
	_ = group.Wait()

	results := make([]CrossAccountResourceResult[T], 0, len(accounts))
	failedAccounts := make([]string, 0)
	for i, membership := range accounts {
		if failed[i] {
			failedAccounts = append(failedAccounts, membership.Name)
			continue
		}
		results = append(results, loaded[i])
	}
	return results, failedAccounts
}

func serveCrossAccountResources[T any](
	c *gin.Context,
	log *logger.Logger,
	accountStore *account.AccountStore,
	resource string,
	load func(context.Context, account.AccountWithRole) (crossAccountResourcePage[T], error),
) {
	memberships, rejectedAccounts, ok := crossAccountMemberships(c, log, accountStore)
	if !ok {
		return
	}
	results, failedAccounts := loadCrossAccountResources(
		c.Request.Context(),
		log,
		resource,
		memberships,
		load,
	)
	c.JSON(http.StatusOK, CrossAccountResourceResponse[T]{
		Results:          results,
		FailedAccounts:   failedAccounts,
		RejectedAccounts: rejectedAccounts,
	})
}

func ListCrossAccountBlueprints(
	log *logger.Logger,
	index *agentindex.Index,
	accountStore *account.AccountStore,
	hearts *heartstore.Store,
	metrics *metricsstore.Store,
	deploys *deploymentstore.Store,
	avatarStore *avatar.Store,
	auditStore *auditlog.Store,
	workos userGetter,
) gin.HandlerFunc {
	dependencies := accountAgentListDependencies{
		index:       index,
		accounts:    accountStore,
		hearts:      hearts,
		metrics:     metrics,
		deployments: deploys,
		avatars:     avatarStore,
		audit:       auditStore,
		workosUsers: workos,
	}
	return func(c *gin.Context) {
		filters, err := ParseBlueprintListFilters(c)
		if err != nil {
			writeListFilterError(c, err)
			return
		}
		serveCrossAccountResources(
			c,
			log,
			accountStore,
			"blueprints",
			func(ctx context.Context, membership account.AccountWithRole) (crossAccountResourcePage[ListAgentsResponse], error) {
				agents, total, err := listAccountAgentResponses(
					ctx,
					dependencies,
					accountAgentListScope{
						id:   membership.ID,
						name: membership.Name,
					},
					toBlueprintListOptions(filters),
				)
				if err != nil {
					return crossAccountResourcePage[ListAgentsResponse]{}, err
				}
				response := ListAgentsResponse{
					Agents:  agents,
					Count:   total,
					Limit:   filters.Limit,
					Offset:  filters.Offset,
					HasMore: filters.Offset+len(agents) < total,
				}
				return crossAccountResourcePage[ListAgentsResponse]{
					data:    response,
					count:   total,
					limit:   filters.Limit,
					offset:  filters.Offset,
					hasMore: response.HasMore,
				}, nil
			})
	}
}

func ListCrossAccountKnowledgeStores(
	log *logger.Logger,
	accountStore *account.AccountStore,
	knowledgeStore *knowledgestore.Store,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit, offset, err := parseListPagination(c)
		if err != nil {
			writeListFilterError(c, err)
			return
		}
		serveCrossAccountResources(
			c,
			log,
			accountStore,
			"knowledge",
			func(ctx context.Context, membership account.AccountWithRole) (crossAccountResourcePage[[]KnowledgeResponse], error) {
				stores, total, err := knowledgeStore.ListByAccountPage(ctx, membership.ID, limit, offset)
				if err != nil {
					return crossAccountResourcePage[[]KnowledgeResponse]{}, err
				}
				responses := make([]KnowledgeResponse, 0, len(stores))
				for _, store := range stores {
					responses = append(responses, toKnowledgeResponse(store))
				}
				return crossAccountResourcePage[[]KnowledgeResponse]{
					data:    responses,
					count:   total,
					limit:   limit,
					offset:  offset,
					hasMore: offset+len(responses) < total,
				}, nil
			})
	}
}
