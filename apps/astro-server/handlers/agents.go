package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	spec "github.com/astropods/astro-spec"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
	"github.com/astropods/astro/apps/astro-server/internal/auth"
	"github.com/astropods/astro/apps/astro-server/internal/avatar"
	"github.com/astropods/astro/apps/astro-server/internal/colorextract"
	"github.com/astropods/astro/apps/astro-server/internal/deploycache"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
	githubclient "github.com/astropods/astro/apps/astro-server/internal/github"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/githubwebhook"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/identitygen"
	"github.com/astropods/astro/apps/astro-server/internal/k8scache"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/pipes"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
	"gopkg.in/yaml.v3"
)

// AgentMetrics holds computed metrics for an agent.
type AgentMetrics struct {
	LifetimeMessages int64 `json:"lifetime_messages"`
	DeployCount      int64 `json:"deploy_count"`
}

// AgentPublisher identifies a user who has pushed this agent.
type AgentPublisher struct {
	Name    string `json:"name"`
	Account string `json:"account,omitempty"`
}

// AgentResponse represents an agent with all its versions
type AgentResponse struct {
	Account      string                 `json:"account"`
	Name         string                 `json:"name"`
	Registry     string                 `json:"registry"`
	Visibility   string                 `json:"visibility"`
	AvatarURL    string                 `json:"avatar_url,omitempty"`
	AvatarColors json.RawMessage        `json:"avatar_colors,omitempty"`
	ArchivedAt   *time.Time             `json:"archived_at,omitempty"`
	NameReserved bool                   `json:"name_reserved"`
	Versions     []AgentVersionResponse `json:"versions"`
	HeartCount   int                    `json:"heart_count"`
	Hearted      bool                   `json:"hearted"`
	Metrics      *AgentMetrics          `json:"metrics"`
	Publishers   []AgentPublisher       `json:"publishers,omitempty"`
}

// AgentVersionResponse represents a specific version of an agent
type AgentVersionResponse struct {
	BuildID            string                `json:"build_id"`
	Spec               map[string]any        `json:"spec"`
	Readme             string                `json:"readme"`
	AgentCard          *spec.ParsedAgentCard `json:"agent_card,omitempty"`
	PublishedAt        string                `json:"published_at"`
	ValidationWarnings []map[string]any      `json:"validation_warnings,omitempty"`
	// CommitMessage, CommitSHA, and RepoFullName describe the git commit that
	// produced this build, present only for GitHub-sourced builds returned by
	// account list queries.
	CommitMessage string `json:"commit_message,omitempty"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	RepoFullName  string `json:"repo_full_name,omitempty"`
}

// buildVersionResponse converts an agentindex.Version into an AgentVersionResponse.
// The agent card is deserialized from the pre-parsed JSON stored at registration time.
// For agents that predate the agent card feature, it synthesizes a card from legacy
// spec fields (meta.description, meta.tags) so existing agents continue to display correctly.
func buildVersionResponse(v *agentindex.AgentVersion) AgentVersionResponse {
	resp := AgentVersionResponse{
		BuildID:       v.BuildID,
		Spec:          v.Spec,
		Readme:        v.Readme,
		PublishedAt:   v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
		CommitMessage: v.CommitMessage,
		CommitSHA:     v.CommitSHA,
		RepoFullName:  v.RepoFullName,
	}
	if v.AgentCardJSON != "" && v.AgentCardJSON != "{}" {
		var card spec.ParsedAgentCard
		if err := json.Unmarshal([]byte(v.AgentCardJSON), &card); err == nil {
			resp.AgentCard = &card
		}
	} else {
		// Fallback: synthesize an agent card from legacy spec fields and raw readme
		resp.AgentCard = buildLegacyAgentCard(v.Spec, v.Readme)
	}
	return resp
}

// buildLegacyAgentCard constructs a ParsedAgentCard for agents that don't have
// a pre-parsed agent_card_json. It first tries to parse the readme as an AGENT.md
// (with YAML frontmatter). If that yields no structured metadata, it falls back to
// legacy spec fields (meta.description, meta.tags). This covers both:
//   - Agents pushed with AGENT.md before the agent_card_json column existed
//   - Old agents that only had a plain README and meta fields in the spec
func buildLegacyAgentCard(specMap map[string]any, readme string) *spec.ParsedAgentCard {
	// Try parsing readme as AGENT.md with frontmatter
	var card *spec.ParsedAgentCard
	if readme != "" {
		card = spec.ParseAgentCard(readme)
	}
	if card == nil {
		card = &spec.ParsedAgentCard{}
	}

	// Fill in gaps from legacy spec.meta fields
	legacyDesc, legacyTags := spec.ExtractLegacyMeta(specMap)
	if card.Description == "" && legacyDesc != "" {
		card.Description = legacyDesc
	}
	if len(card.Tags) == 0 && len(legacyTags) > 0 {
		card.Tags = legacyTags
	}
	if card.Body == "" && readme != "" {
		card.Body = readme
	}

	// Merge spec-derived integrations into whatever the frontmatter already resolved
	card.ResolvedIntegrations = spec.MergeResolvedIntegrations(
		card.ResolvedIntegrations,
		extractSpecProviders(specMap),
	)

	// Only return the card if it has some content
	if card.Description == "" && len(card.Tags) == 0 && card.Body == "" && len(card.ResolvedIntegrations) == 0 {
		return nil
	}
	return card
}

// buildAgentCardJSON parses the raw AGENT.md content and merges spec-derived integrations,
// returning the serialized JSON for storage. Returns empty string on marshal failure.
func buildAgentCardJSON(readme string, specMap map[string]any) string {
	if readme == "" {
		return ""
	}
	card := spec.ParseAgentCard(readme)

	// Extract provider IDs from spec integrations and merge into resolved list
	card.ResolvedIntegrations = spec.MergeResolvedIntegrations(
		card.ResolvedIntegrations,
		extractSpecProviders(specMap),
	)

	data, err := json.Marshal(card)
	if err != nil {
		return ""
	}
	return string(data)
}

// extractSpecProviders pulls provider strings from the spec's integrations map.
func extractSpecProviders(specMap map[string]any) []string {
	integrations, ok := specMap["integrations"].(map[string]any)
	if !ok {
		return nil
	}
	var providers []string
	for _, v := range integrations {
		entry, ok := v.(map[string]any)
		if !ok {
			continue
		}
		provider, ok := entry["provider"].(string)
		if !ok || provider == "" {
			continue
		}
		providers = append(providers, provider)
	}
	return providers
}

// RegisterAgentRequest represents the request to register an agent
type RegisterAgentRequest struct {
	BuildID     string `json:"build_id" binding:"required"`
	Registry    string `json:"registry" binding:"required"`
	SpecContent string `json:"spec_content" binding:"required"`
	Readme      string `json:"readme"`
	// ReadmeAssets maps each AGENT.md-referenced local image path to the CDN URL
	// it was uploaded to (via /readme-assets). The server rewrites Readme with
	// these before storing so links resolve to the assets CDN.
	ReadmeAssets map[string]string `json:"readme_assets,omitempty"`
	Visibility   string            `json:"visibility,omitempty"` // "public" or "private"; only applied on first registration
}

// agentMetrics returns an AgentMetrics value for the given counts.
func agentMetrics(lifetimeMessages, deployCount int64) *AgentMetrics {
	return &AgentMetrics{LifetimeMessages: lifetimeMessages, DeployCount: deployCount}
}

// ListAgents handles GET /api/v1/agents
// Lists agents with visibility='public' (public catalog)
func toBlueprintListOptions(f BlueprintListFilters) agentindex.BlueprintListOptions {
	return agentindex.BlueprintListOptions{
		Query:      f.Query,
		Tag:        f.Tag,
		Visibility: f.Visibility,
		Sort:       f.Sort,
		Limit:      f.Limit,
		Offset:     f.Offset,
	}
}

func writeBlueprintListResponse(c *gin.Context, responses []AgentResponse, filters BlueprintListFilters, total int) {
	hasMore := filters.Offset+len(responses) < total
	c.JSON(http.StatusOK, gin.H{
		"agents":   responses,
		"count":    total,
		"limit":    filters.Limit,
		"offset":   filters.Offset,
		"has_more": hasMore,
	})
}

func ListAgents(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, hearts *heartstore.Store, metrics *metricsstore.Store, deploys *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store, workos userGetter) gin.HandlerFunc {
	return func(c *gin.Context) {
		filters, err := ParseBlueprintListFilters(c)
		if err != nil {
			writeListFilterError(c, err)
			return
		}
		// Public catalog is always public-only; visibility filter applies on GET /agents/:account.
		if filters.Visibility != "" {
			writeListFilterError(c, fmt.Errorf("visibility is not supported on the public catalog"))
			return
		}

		log.Info("Listing public agents from index")

		page, err := index.ListPublicAgents(toBlueprintListOptions(filters))
		if err != nil {
			log.Error("Failed to list agents", "error", err)
			writeBlueprintListInternalError(c, "Failed to list agents from index")
			return
		}

		// Build a cache of account ID -> name to avoid repeated lookups
		accountNames := make(map[string]string)

		// Bulk-fetch heart counts per account to avoid N+1
		heartCounts := make(map[string]map[string]int) // accountID -> agentName -> count

		// Bulk-fetch message counts per account
		msgCounts := make(map[string]map[string]int64) // accountID -> agentName -> count

		// Bulk-fetch deploy counts per account
		deployCounts := make(map[string]map[string]int64) // accountID -> agentName -> count

		publisherActors := make(map[string]map[string][]string) // accountID -> agentName -> actorIDs
		userCache := make(map[string]*auth.User)

		responses := make([]AgentResponse, 0, len(page.Agents))
		for _, agent := range page.Agents {
			// Resolve account name
			accountName, ok := accountNames[agent.AccountID]
			if !ok {
				acct, err := accountStore.GetByID(agent.AccountID)
				if err != nil {
					accountName = agent.AccountID // fallback to ID
				} else {
					accountName = acct.Name
				}
				accountNames[agent.AccountID] = accountName
			}

			// Lazy-load heart counts for this account
			if _, ok := heartCounts[agent.AccountID]; !ok {
				counts, err := hearts.BulkCount(c.Request.Context(), agent.AccountID)
				if err != nil {
					counts = map[string]int{}
				}
				heartCounts[agent.AccountID] = counts
			}

			// Lazy-load message counts for this account
			if _, ok := msgCounts[agent.AccountID]; !ok {
				mc, _ := metrics.BulkMessageCounts(agent.AccountID)
				if mc == nil {
					mc = map[string]int64{}
				}
				msgCounts[agent.AccountID] = mc
			}

			// Lazy-load deploy counts for this account
			if _, ok := deployCounts[agent.AccountID]; !ok {
				dc, _ := deploys.BulkDeploymentCounts(agent.AccountID)
				if dc == nil {
					dc = map[string]int64{}
				}
				deployCounts[agent.AccountID] = dc
			}

			versions := make([]AgentVersionResponse, 0, len(agent.Versions))
			for _, v := range agent.Versions {
				versions = append(versions, buildVersionResponse(v))
			}

			resp := AgentResponse{
				Account:    accountName,
				Name:       agent.Name,
				Registry:   agent.Registry,
				Visibility: agent.Visibility,
				Versions:   versions,
				HeartCount: heartCounts[agent.AccountID][agent.Name],
				Metrics:    agentMetrics(msgCounts[agent.AccountID][agent.Name], deployCounts[agent.AccountID][agent.Name]),
			}
			if avatarStore != nil {
				resp.AvatarURL = avatarStore.AgentAvatarURL(accountName, agent.Name, agent.AvatarUpdatedAt)
				var existing json.RawMessage
				if agent.AvatarColors != nil {
					existing = *agent.AvatarColors
				}
				resp.AvatarColors = colorextract.EnsureCurrent(c.Request.Context(), existing,
					func(ctx context.Context) ([]byte, error) {
						return avatarStore.ReadAgentAvatar(ctx, accountName, agent.Name)
					},
					func(ctx context.Context, j []byte) error {
						return index.SetAvatarColors(agent.AccountID, agent.Name, j)
					},
				)
			}
			if auditStore != nil && workos != nil {
				if _, ok := publisherActors[agent.AccountID]; !ok {
					bulk, err := auditStore.BulkDistinctActorsFor(c.Request.Context(), agent.AccountID, auditlog.AgentRegister, "agent", nil)
					if err != nil {
						bulk = map[string][]string{}
					}
					publisherActors[agent.AccountID] = bulk
				}
				if actorIDs := publisherActors[agent.AccountID][agent.Name]; len(actorIDs) > 0 {
					resp.Publishers = resolvePublishers(c.Request.Context(), actorIDs, workos, accountStore, userCache)
				}
			}
			responses = append(responses, resp)
		}

		writeBlueprintListResponse(c, responses, filters, page.Total)
	}
}

// ListAccountAgents handles GET /api/v1/agents/:account
// Lists all public agents for an account. Members also see private agents.
func ListAccountAgents(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, hearts *heartstore.Store, metrics *metricsstore.Store, deploys *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store, workos userGetter) gin.HandlerFunc {
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

		accountName := c.Param("account")

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		isMember := false
		if user, exists := middleware.GetUser(c); exists {
			if ok, err := accountStore.IsMember(acct.ID, user.ID); err == nil && ok {
				isMember = true
			}
		}

		listOpts := toBlueprintListOptions(filters)
		if !isMember {
			// Restrict SQL to public rows so count/has_more cannot leak private catalog metadata.
			listOpts.Visibility = "public"
		}

		responses, total, err := listAccountAgentResponses(
			c.Request.Context(),
			dependencies,
			accountAgentListScope{
				id:   acct.ID,
				name: accountName,
			},
			listOpts,
		)
		if err != nil {
			log.Error("Failed to list agents for account", "error", err, "account", accountName)
			writeBlueprintListInternalError(c, "Failed to list agents")
			return
		}

		writeBlueprintListResponse(c, responses, filters, total)
	}
}

type accountAgentListDependencies struct {
	index       *agentindex.Index
	accounts    *account.AccountStore
	hearts      *heartstore.Store
	metrics     *metricsstore.Store
	deployments *deploymentstore.Store
	avatars     *avatar.Store
	audit       *auditlog.Store
	workosUsers userGetter
}

type accountAgentListScope struct {
	id   string
	name string
}

func listAccountAgentResponses(
	ctx context.Context,
	dependencies accountAgentListDependencies,
	scope accountAgentListScope,
	listOpts agentindex.BlueprintListOptions,
) ([]AgentResponse, int, error) {
	page, err := dependencies.index.ListForAccount(scope.id, listOpts)
	if err != nil {
		return nil, 0, err
	}

	counts, _ := dependencies.hearts.BulkCount(ctx, scope.id)
	if counts == nil {
		counts = map[string]int{}
	}
	messageCounts, _ := dependencies.metrics.BulkMessageCounts(scope.id)
	if messageCounts == nil {
		messageCounts = map[string]int64{}
	}
	deployCounts, _ := dependencies.deployments.BulkDeploymentCounts(scope.id)
	if deployCounts == nil {
		deployCounts = map[string]int64{}
	}

	publisherActors := make(map[string][]string)
	if dependencies.audit != nil && dependencies.workosUsers != nil {
		if bulk, err := dependencies.audit.BulkDistinctActorsFor(ctx, scope.id, auditlog.AgentRegister, "agent", nil); err == nil {
			publisherActors = bulk
		}
	}
	userCache := make(map[string]*auth.User)

	responses := make([]AgentResponse, 0, len(page.Agents))
	for _, agent := range page.Agents {
		versions := make([]AgentVersionResponse, 0, len(agent.Versions))
		for _, version := range agent.Versions {
			versions = append(versions, buildVersionResponse(version))
		}

		resp := AgentResponse{
			Account:    scope.name,
			Name:       agent.Name,
			Registry:   agent.Registry,
			Visibility: agent.Visibility,
			Versions:   versions,
			HeartCount: counts[agent.Name],
			Metrics:    agentMetrics(messageCounts[agent.Name], deployCounts[agent.Name]),
		}
		if dependencies.avatars != nil {
			resp.AvatarURL = dependencies.avatars.AgentAvatarURL(scope.name, agent.Name, agent.AvatarUpdatedAt)
			var existing json.RawMessage
			if agent.AvatarColors != nil {
				existing = *agent.AvatarColors
			}
			resp.AvatarColors = colorextract.EnsureCurrent(ctx, existing,
				func(ctx context.Context) ([]byte, error) {
					return dependencies.avatars.ReadAgentAvatar(ctx, scope.name, agent.Name)
				},
				func(ctx context.Context, colors []byte) error {
					return dependencies.index.SetAvatarColors(agent.AccountID, agent.Name, colors)
				},
			)
		}
		if actorIDs := publisherActors[agent.Name]; len(actorIDs) > 0 {
			resp.Publishers = resolvePublishers(ctx, actorIDs, dependencies.workosUsers, dependencies.accounts, userCache)
		}
		responses = append(responses, resp)
	}

	return responses, page.Total, nil
}

type userGetter interface {
	GetUser(ctx context.Context, userID string) (*auth.User, error)
}

type accountLister interface {
	GetAccountsForUser(userID string) ([]account.AccountWithRole, error)
}

// resolvePublishers maps a list of WorkOS actor IDs to AgentPublishers.
// Actors that cannot be resolved (missing name and handle) are silently skipped.
// userCache, if non-nil, is used to avoid redundant WorkOS lookups across many calls.
func resolvePublishers(ctx context.Context, actorIDs []string, users userGetter, accounts accountLister, userCache map[string]*auth.User) []AgentPublisher {
	var publishers []AgentPublisher
	for _, actorID := range actorIDs {
		var user *auth.User
		if userCache != nil {
			user = userCache[actorID]
		}
		if user == nil {
			u, err := users.GetUser(ctx, actorID)
			if err != nil {
				continue
			}
			user = u
			if userCache != nil {
				userCache[actorID] = user
			}
		}
		pub := AgentPublisher{}
		if accts, err := accounts.GetAccountsForUser(actorID); err == nil {
			for _, a := range accts {
				if a.Type == "personal" {
					pub.Account = a.Name
					break
				}
			}
		}
		fullName := strings.TrimSpace(user.FirstName + " " + user.LastName)
		if fullName != "" {
			pub.Name = fullName
		} else if pub.Account != "" {
			pub.Name = pub.Account
		} else {
			continue
		}
		publishers = append(publishers, pub)
	}
	return publishers
}

// GetAgent handles GET /api/v1/agents/:account/:name
// Private agents are only visible to account members; public agents are visible to all
func GetAgent(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, hearts *heartstore.Store, metrics *metricsstore.Store, deploys *deploymentstore.Store, avatarStore *avatar.Store, auditStore *auditlog.Store, workos *auth.WorkOSClient) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		name := c.Param("name")

		log.Info("Getting agent details", "account", accountName, "name", name)

		// Resolve account
		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		agent, err := index.Get(acct.ID, name)
		if err != nil {
			log.Error("Failed to get agent", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Agent not found",
				"details": err.Error(),
			})
			return
		}

		// Check if the caller is an authenticated account member
		isMember := false
		userID := ""
		if user, exists := middleware.GetUser(c); exists {
			userID = user.ID
			if ok, err := accountStore.IsMember(acct.ID, user.ID); err == nil && ok {
				isMember = true
			}
		}

		// Private agents are only visible to members
		if agent.Visibility == "private" && !isMember {
			c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
			return
		}

		versions := make([]AgentVersionResponse, 0, len(agent.Versions))
		for _, v := range agent.Versions {
			resp := buildVersionResponse(v)
			if isMember {
				resp.ValidationWarnings = v.ValidationWarnings
			}
			versions = append(versions, resp)
		}

		// Heart info
		heartInfo, _ := hearts.Info(c.Request.Context(), acct.ID, name, userID)

		// Message count
		mc, _ := metrics.BulkMessageCounts(acct.ID)
		if mc == nil {
			mc = map[string]int64{}
		}

		// Deploy count
		dc, _ := deploys.BulkDeploymentCounts(acct.ID)
		if dc == nil {
			dc = map[string]int64{}
		}

		resp := AgentResponse{
			Account:      accountName,
			Name:         agent.Name,
			Registry:     agent.Registry,
			Visibility:   agent.Visibility,
			ArchivedAt:   agent.ArchivedAt,
			NameReserved: agent.NameReserved,
			Versions:     versions,
			Metrics:      agentMetrics(mc[name], dc[name]),
		}
		if avatarStore != nil {
			resp.AvatarURL = avatarStore.AgentAvatarURL(accountName, name, agent.AvatarUpdatedAt)
			var existing json.RawMessage
			if agent.AvatarColors != nil {
				existing = *agent.AvatarColors
			}
			resp.AvatarColors = colorextract.EnsureCurrent(c.Request.Context(), existing,
				func(ctx context.Context) ([]byte, error) {
					return avatarStore.ReadAgentAvatar(ctx, accountName, agent.Name)
				},
				func(ctx context.Context, j []byte) error {
					return index.SetAvatarColors(agent.AccountID, agent.Name, j)
				},
			)
		}
		if heartInfo != nil {
			resp.HeartCount = heartInfo.Count
			resp.Hearted = heartInfo.Hearted
		}

		if auditStore != nil && workos != nil {
			if actorIDs, err := auditStore.DistinctActorsFor(c.Request.Context(), acct.ID, auditlog.AgentRegister, "agent", name); err == nil {
				resp.Publishers = resolvePublishers(c.Request.Context(), actorIDs, workos, accountStore, nil)
			}
		}

		c.JSON(http.StatusOK, resp)
	}
}

// RegisterAgent handles POST /api/v1/agents/:account/:name/register
// Registers a new agent or updates an existing one in the index.
// Requires agents:write permission (enforced by middleware).
// If minCLIVersion is non-empty, pushes from older CLI versions are rejected with 426.
// aiGatewayEnabled toggles the validator's astro-gateway provider gate — pushed
// from cfg.Deployment.AIGatewayURL != "" at the main.go wiring site so a spec
// using provider:astro-gateway in a gateway-less env fails at admission.
func RegisterAgent(log *logger.Logger, index *agentindex.Index, minCLIVersion string, db *sql.DB, auditStore *auditlog.Store, avatarStore *avatar.Store, deployStore *deploymentstore.Store, cache k8scache.Cache, aiGatewayEnabled bool) gin.HandlerFunc {
	// Pre-parse the minimum version at startup so we don't parse on every request.
	var minVer *semver.Version
	if minCLIVersion != "" {
		var err error
		minVer, err = semver.NewVersion(minCLIVersion)
		if err != nil {
			log.Warn("Invalid MIN_CLI_VERSION, version gate disabled", "value", minCLIVersion, "error", err)
		}
	}

	return func(c *gin.Context) {
		// Enforce minimum CLI version when configured.
		if minVer != nil {
			cliVersion := c.GetHeader("X-Cli-Version")
			if cliVersion == "" || cliVersion == "dev" {
				c.JSON(http.StatusUpgradeRequired, gin.H{
					"error": fmt.Sprintf("CLI version could not be verified — minimum version is %s. Please install a release build", minVer),
				})
				return
			} else if cv, err := semver.NewVersion(cliVersion); err != nil {
				log.Warn("Unparseable X-CLI-Version header", "value", cliVersion)
			} else if cv.LessThan(minVer) {
				c.JSON(http.StatusUpgradeRequired, gin.H{
					"error": fmt.Sprintf("CLI version %s is below the minimum required version %s — please upgrade", cliVersion, minVer),
				})
				return
			}
		}

		accountName := c.Param("account")
		rawName := c.Param("name")

		// Reject org-scoped names (e.g. "@org/agent") — the CLI should strip these before pushing
		if strings.Contains(rawName, "/") || strings.HasPrefix(rawName, "@") {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid agent name %q: must not contain the @org/ prefix; upgrade your CLI", rawName),
			})
			return
		}
		if err := spec.ValidateName(rawName); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": fmt.Sprintf("invalid agent name %q: %v", rawName, err),
			})
			return
		}
		agentName := rawName

		var req RegisterAgentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			log.Error("Invalid request body", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}
		accountID := acct.ID

		log.Info("Registering agent",
			"account", accountName,
			"name", agentName,
			"build_id", req.BuildID,
		)

		// Parse YAML spec to structured format
		var specMap map[string]any
		if err := yaml.Unmarshal([]byte(req.SpecContent), &specMap); err != nil {
			log.Error("Failed to parse spec YAML", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid spec YAML",
				"details": err.Error(),
			})
			return
		}

		// Validate spec structure and collect warnings
		var validationWarningsJSON string
		var validationWarnings []deployment.ValidationError

		specJSON, err := json.Marshal(specMap)
		if err == nil {
			var astroSpec spec.AstroSpec
			if err := json.Unmarshal(specJSON, &astroSpec); err == nil {
				// Reject specs that contain default values for secret inputs.
				// The CLI must strip these before pushing (v0.5.2+).
				if violations := spec.SecretDefaultViolations(&astroSpec); len(violations) > 0 {
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Secret inputs must not have default values in the registered spec",
						"details": fmt.Sprintf("found %d secret input(s) with defaults: %s. Upgrade your CLI to v0.5.2+ which strips these automatically", len(violations), strings.Join(violations, ", ")),
					})
					return
				}

				result := deployment.NewValidatorWithOptions(deployment.ValidatorOptions{
					AIGatewayEnabled: aiGatewayEnabled,
				}).ValidateSpec(&astroSpec, nil, nil, nil)

				// Structural errors (missing image/build, bad provider, invalid trigger
				// type) are hard failures. Deploy-time values (credentials, schedule
				// expressions) are not known at registration and stored as warnings.
				var structuralErrs []deployment.ValidationError
				for _, e := range result.Errors {
					if strings.HasPrefix(e.Field, "variables.") ||
						strings.HasSuffix(e.Field, ".trigger.schedule") {
						validationWarnings = append(validationWarnings, e)
					} else {
						structuralErrs = append(structuralErrs, e)
					}
				}
				if len(structuralErrs) > 0 {
					msgs := make([]string, len(structuralErrs))
					for i, e := range structuralErrs {
						msgs[i] = e.Field + ": " + e.Message
					}
					c.JSON(http.StatusBadRequest, gin.H{
						"error":   "Invalid spec",
						"details": strings.Join(msgs, "; "),
					})
					return
				}
			}
		}

		warningsBytes, _ := json.Marshal(validationWarnings)
		validationWarningsJSON = string(warningsBytes)

		// Rewrite AGENT.md image references to the CDN URLs they were uploaded to
		// (via /readme-assets) before parsing and storage, so both the stored
		// readme and the parsed card body link to the assets CDN.
		req.Readme = spec.RewriteMarkdownImages(req.Readme, req.ReadmeAssets)

		// Parse agent card from readme and merge spec-derived integrations at registration time
		agentCardJSON := buildAgentCardJSON(req.Readme, specMap)

		if err := index.Register(accountID, agentName, req.BuildID, req.Registry, acct.ID, specMap, req.Readme, agentCardJSON, validationWarningsJSON); err != nil {
			log.Error("Failed to register agent", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to register agent",
				"details": err.Error(),
			})
			return
		}

		// Publishing a new build shifts `latest_build_id` for every downstream
		// deployment whose lineage points at this agent. Bust their per-account
		// deploy caches so the "Update available" pill shows up immediately
		// instead of waiting for SafetyTTL.
		if affected := deploycache.InvalidateForLineage(c.Request.Context(), cache, deployStore, accountID, agentName); len(affected) > 0 {
			log.Info("Publish: invalidated deploy cache for downstream consumers",
				"agent", agentName,
				"affected_accounts", len(affected),
			)
		}

		// Set visibility if provided (only "public" or "private" are valid)
		if req.Visibility == "public" || req.Visibility == "private" {
			if err := index.SetVisibility(accountID, agentName, req.Visibility); err != nil {
				log.Warn("Failed to set visibility during registration", "error", err, "visibility", req.Visibility)
			}
		}

		evt := auditlog.FromGinContext(c, accountID)
		evt.Action = auditlog.AgentRegister
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Registered agent " + agentName
		evt.Metadata = map[string]any{"build_id": req.BuildID}
		auditStore.LogAsync(log, evt)

		// Generate and upload the placeholder avatar if one doesn't already
		// exist. Failures are non-fatal — the periodic backfill job will
		// retry missing avatars.
		if avatarStore != nil {
			if exists, _ := avatarStore.AgentAvatarExists(c.Request.Context(), accountName, agentName); !exists {
				if jpegBytes, err := identitygen.GenerateIdentityJPEG(identitygen.IdentityOptions{
					Seed: accountName + "/" + agentName,
				}); err != nil {
					log.Warn("Failed to generate blueprint avatar", "account", accountName, "name", agentName, "error", err)
				} else if err := avatarStore.WriteAgentAvatarJPEG(c.Request.Context(), accountName, agentName, jpegBytes); err != nil {
					log.Warn("Failed to upload blueprint avatar", "account", accountName, "name", agentName, "error", err)
				} else {
					_ = touchAgentAvatar(log, index, accountID, accountName, agentName)
					extractAndStoreColors(c.Request.Context(), log,
						func(context.Context) ([]byte, error) { return jpegBytes, nil },
						func(j []byte) error { return index.SetAvatarColors(accountID, agentName, j) },
						"account", accountName, "name", agentName,
					)
				}
			}
		}

		response := gin.H{
			"message":  "Agent registered successfully",
			"account":  accountName,
			"name":     agentName,
			"build_id": req.BuildID,
		}
		if len(validationWarnings) > 0 {
			response["validation_warnings"] = validationWarnings
		}
		if strings.TrimSpace(req.Readme) == "" {
			response["hints"] = []string{
				"No AGENT.md provided — add one next to your astropods.yml to make your agent more discoverable",
			}
		}
		c.JSON(http.StatusCreated, response)
	}
}

// CreateBlueprintRequest is the body for creating a blueprint shell (no build required).
type CreateBlueprintRequest struct {
	Name       string `json:"name" binding:"required"`
	Visibility string `json:"visibility,omitempty"` // "public" or "private" (default: "private")
}

// CreateBlueprintResponse is returned after a successful blueprint creation.
type CreateBlueprintResponse struct {
	Account string `json:"account"`
	Name    string `json:"name"`
}

// CreateBlueprint handles POST /api/v1/agents/:account.
// Creates an agent shell with no builds so users can connect a GitHub repo before pushing.
func CreateBlueprint(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, auditStore *auditlog.Store, avatarStore *avatar.Store, db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req CreateBlueprintRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
			return
		}

		// Validate name using the same rules as account names.
		if err := account.ValidateAccountName(req.Name); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		if strings.Contains(req.Name, "/") || strings.HasPrefix(req.Name, "@") {
			c.JSON(http.StatusBadRequest, gin.H{"error": "agent name must not contain @org/ prefix"})
			return
		}

		if err := index.Create(acct.ID, req.Name); err != nil {
			if errors.Is(err, agentindex.ErrAlreadyExists) {
				c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("agent %q already exists", req.Name)})
				return
			}
			var pqErr *pq.Error
			if errors.As(err, &pqErr) && pqErr.Code == "23505" {
				c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("agent %q already exists", req.Name)})
				return
			}
			log.Error("Failed to create blueprint", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create blueprint"})
			return
		}

		if req.Visibility == "public" || req.Visibility == "private" {
			if err := index.SetVisibility(acct.ID, req.Name, req.Visibility); err != nil {
				log.Warn("Failed to set visibility on new blueprint", "error", err)
			}
		}

		// Generate and upload the placeholder avatar. Failures are non-fatal —
		// the blueprint is already created in the DB, and the periodic backfill
		// job will retry missing avatars. Don't block blueprint creation on an
		// S3 glitch.
		if avatarStore != nil {
			if jpegBytes, err := identitygen.GenerateIdentityJPEG(identitygen.IdentityOptions{
				Seed: accountName + "/" + req.Name,
			}); err != nil {
				log.Warn("Failed to generate blueprint avatar", "account", accountName, "name", req.Name, "error", err)
			} else if err := avatarStore.WriteAgentAvatarJPEG(c.Request.Context(), accountName, req.Name, jpegBytes); err != nil {
				log.Warn("Failed to upload blueprint avatar", "account", accountName, "name", req.Name, "error", err)
			} else {
				_ = touchAgentAvatar(log, index, acct.ID, accountName, req.Name)
				extractAndStoreColors(c.Request.Context(), log,
					func(context.Context) ([]byte, error) { return jpegBytes, nil },
					func(j []byte) error { return index.SetAvatarColors(acct.ID, req.Name, j) },
					"account", accountName, "name", req.Name,
				)
			}
		}

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AgentRegister
		evt.ResourceType = "agent"
		evt.ResourceID = req.Name
		evt.ResourceName = req.Name
		evt.Description = "Created blueprint " + req.Name
		auditStore.LogAsync(log, evt)

		log.Info("Blueprint created", "account", accountName, "name", req.Name)
		c.JSON(http.StatusCreated, CreateBlueprintResponse{Account: accountName, Name: req.Name})
	}
}

// SetAgentVisibilityRequest represents the request to change agent visibility
type SetAgentVisibilityRequest struct {
	Visibility string `json:"visibility" binding:"required"`
}

// ArchiveAgent handles POST /api/v1/agents/:account/:name/archive
// Soft-deletes an agent by setting archived_at, hiding it from listings
// while preserving data for existing deployments.
// Requires agents:write permission (enforced by middleware).
func ArchiveAgent(log *logger.Logger, index *agentindex.Index, db *sql.DB, auditStore *auditlog.Store, ghStore *githubconnection.Store, webhookStore *githubwebhook.Store, pipesClient *pipes.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if err := index.Archive(acct.ID, agentName); err != nil {
			log.Error("Failed to archive agent", "error", err, "account", accountName, "name", agentName)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "failed to archive agent",
				"details": err.Error(),
			})
			return
		}

		// Best-effort: disconnect any linked GitHub repo so it can be reused.
		// Extract session before the goroutine — gin.Context must not be accessed after the handler returns.
		session, sessionOK := middleware.GetSession(c)
		go func() {
			if ghStore == nil || webhookStore == nil {
				return
			}
			conn, err := ghStore.Get(context.Background(), acct.ID, agentName)
			if err != nil {
				return // no connection — nothing to do
			}
			repoBase := githubconnection.RepoBase(conn.RepoFullName)
			if delErr := ghStore.Delete(context.Background(), acct.ID, agentName); delErr != nil {
				log.Warn("github: delete connection on archive", "error", delErr, "agent", agentName)
			} else {
				log.Info("GitHub connection removed on archive", "account", acct.Name, "agent", agentName)
			}
			if wid, deleted, _ := webhookStore.DeleteIfNoConnections(context.Background(), repoBase); deleted && sessionOK {
				token, tokenErr := pipesClient.GetAccessToken(context.Background(), pipes.GetAccessTokenInput{
					Provider:       "github",
					UserID:         session.UserID,
					OrganizationID: session.OrganizationID,
				})
				if tokenErr == nil {
					gh := githubclient.New(token.AccessToken)
					if delErr := gh.DeleteWebhook(context.Background(), repoBase, wid); delErr != nil {
						log.Warn("github: delete webhook on archive", "error", delErr, "repo", repoBase)
					}
				}
			}
		}()

		log.Info("Agent archived", "account", accountName, "name", agentName)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AgentArchive
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Archived agent " + agentName
		auditStore.LogAsync(log, evt)

		c.Status(http.StatusNoContent)
	}
}

// SetAgentVisibility handles PUT /api/v1/agents/:account/:name/visibility
// Toggles an agent between public and private visibility.
// Requires agents:write permission (enforced by middleware).
func SetAgentVisibility(log *logger.Logger, index *agentindex.Index, auditStore *auditlog.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		var req SetAgentVisibilityRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		if err := index.SetVisibility(acct.ID, agentName, req.Visibility); err != nil {
			log.Error("Failed to set agent visibility", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to set visibility",
				"details": err.Error(),
			})
			return
		}

		log.Info("Agent visibility updated",
			"account", accountName,
			"name", agentName,
			"visibility", req.Visibility,
		)

		evt := auditlog.FromGinContext(c, acct.ID)
		evt.Action = auditlog.AgentSetVisibility
		evt.ResourceType = "agent"
		evt.ResourceID = agentName
		evt.ResourceName = agentName
		evt.Description = "Set agent " + agentName + " visibility to " + req.Visibility
		evt.Metadata = map[string]any{"visibility": req.Visibility}
		auditStore.LogAsync(log, evt)

		c.JSON(http.StatusOK, gin.H{
			"message":    "visibility updated",
			"account":    accountName,
			"name":       agentName,
			"visibility": req.Visibility,
		})
	}
}
