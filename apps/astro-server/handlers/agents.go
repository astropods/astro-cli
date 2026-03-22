package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/astropods/astro/apps/astro-server/internal/account"
	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/heartstore"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/metricsstore"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/astropods/astro/apps/astro-server/internal/openmeter"
	spec "github.com/astropods/astro/packages/astro-spec"
	"github.com/gin-gonic/gin"
	"gopkg.in/yaml.v3"
)

// AgentMetrics holds computed metrics for an agent.
type AgentMetrics struct {
	LifetimeMessages int64 `json:"lifetime_messages"`
}

// AgentResponse represents an agent with all its versions
type AgentResponse struct {
	Account    string                 `json:"account"`
	Name       string                 `json:"name"`
	Registry   string                 `json:"registry"`
	Visibility string                 `json:"visibility"`
	Versions   []AgentVersionResponse `json:"versions"`
	HeartCount int                    `json:"heart_count"`
	Hearted    bool                   `json:"hearted"`
	Metrics    *AgentMetrics          `json:"metrics"`
}

// AgentVersionResponse represents a specific version of an agent
type AgentVersionResponse struct {
	BuildID            string                `json:"build_id"`
	Spec               map[string]any        `json:"spec"`
	Readme             string                `json:"readme"`
	AgentCard          *spec.ParsedAgentCard `json:"agent_card,omitempty"`
	PublishedAt        string                `json:"published_at"`
	ValidationWarnings []map[string]any      `json:"validation_warnings,omitempty"`
}

// buildVersionResponse converts an agentindex.Version into an AgentVersionResponse.
// The agent card is deserialized from the pre-parsed JSON stored at registration time.
// For agents that predate the agent card feature, it synthesizes a card from legacy
// spec fields (meta.description, meta.tags) so existing agents continue to display correctly.
func buildVersionResponse(v *agentindex.AgentVersion) AgentVersionResponse {
	resp := AgentVersionResponse{
		BuildID:     v.BuildID,
		Spec:        v.Spec,
		Readme:      v.Readme,
		PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
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
		if parsed, err := spec.ParseAgentCard(readme); err == nil && parsed != nil {
			card = parsed
		}
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
// returning the serialized JSON for storage. Returns empty string on parse failure.
func buildAgentCardJSON(readme string, specMap map[string]any) string {
	if readme == "" {
		return ""
	}
	card, err := spec.ParseAgentCard(readme)
	if err != nil || card == nil {
		return ""
	}

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
	Visibility  string `json:"visibility,omitempty"` // "public" or "private"; only applied on first registration
}

// agentMetrics returns an AgentMetrics value for the given count.
func agentMetrics(lifetimeMessages int64) *AgentMetrics {
	return &AgentMetrics{LifetimeMessages: lifetimeMessages}
}

// ListAgents handles GET /api/v1/agents
// Lists agents with visibility='public' (public catalog)
func ListAgents(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, hearts *heartstore.Store, metrics *metricsstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Info("Listing public agents from index")

		agents, err := index.ListPublicAgents()
		if err != nil {
			log.Error("Failed to list agents", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list agents from index",
				"details": err.Error(),
			})
			return
		}

		// Build a cache of account ID -> name to avoid repeated lookups
		accountNames := make(map[string]string)

		// Bulk-fetch heart counts per account to avoid N+1
		heartCounts := make(map[string]map[string]int) // accountID -> agentName -> count

		// Bulk-fetch message counts per account
		msgCounts := make(map[string]map[string]int64) // accountID -> agentName -> count

		responses := make([]AgentResponse, 0, len(agents))
		for _, agent := range agents {
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

			versions := make([]AgentVersionResponse, 0, len(agent.Versions))
			for _, v := range agent.Versions {
				versions = append(versions, buildVersionResponse(v))
			}

			var mc map[string]int64
			if m, ok := msgCounts[agent.AccountID]; ok {
				mc = m
			}

			responses = append(responses, AgentResponse{
				Account:    accountName,
				Name:       agent.Name,
				Registry:   agent.Registry,
				Visibility: agent.Visibility,
				Versions:   versions,
				HeartCount: heartCounts[agent.AccountID][agent.Name],
				Metrics:    agentMetrics(mc[agent.Name]),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"agents": responses,
			"count":  len(responses),
		})
	}
}

// ListAccountAgents handles GET /api/v1/agents/:account
// Lists all public agents for an account. Members also see private agents.
func ListAccountAgents(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, hearts *heartstore.Store, metrics *metricsstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		agents, err := index.ListForAccount(acct.ID)
		if err != nil {
			log.Error("Failed to list agents for account", "error", err, "account", accountName)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list agents",
				"details": err.Error(),
			})
			return
		}

		// Bulk-fetch heart counts for this account
		counts, _ := hearts.BulkCount(c.Request.Context(), acct.ID)
		if counts == nil {
			counts = map[string]int{}
		}

		// Bulk-fetch message counts for this account
		mc, _ := metrics.BulkMessageCounts(acct.ID)
		if mc == nil {
			mc = map[string]int64{}
		}

		responses := make([]AgentResponse, 0, len(agents))
		for _, agent := range agents {
			if agent.Visibility == "private" && !isMember {
				continue
			}

			versions := make([]AgentVersionResponse, 0, len(agent.Versions))
			for _, v := range agent.Versions {
				versions = append(versions, buildVersionResponse(v))
			}

			responses = append(responses, AgentResponse{
				Account:    accountName,
				Name:       agent.Name,
				Registry:   agent.Registry,
				Visibility: agent.Visibility,
				Versions:   versions,
				HeartCount: counts[agent.Name],
				Metrics:    agentMetrics(mc[agent.Name]),
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"agents": responses,
			"count":  len(responses),
		})
	}
}

// GetAgent handles GET /api/v1/agents/:account/:name
// Private agents are only visible to account members; public agents are visible to all
func GetAgent(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore, hearts *heartstore.Store, metrics *metricsstore.Store) gin.HandlerFunc {
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

		resp := AgentResponse{
			Account:    accountName,
			Name:       agent.Name,
			Registry:   agent.Registry,
			Visibility: agent.Visibility,
			Versions:   versions,
			Metrics:    agentMetrics(mc[name]),
		}
		if heartInfo != nil {
			resp.HeartCount = heartInfo.Count
			resp.Hearted = heartInfo.Hearted
		}

		c.JSON(http.StatusOK, resp)
	}
}

// RegisterAgent handles POST /api/v1/agents/:account/:name/register
// Registers a new agent or updates an existing one in the index.
// Requires agents:write permission (enforced by middleware).
// If minCLIVersion is non-empty, pushes from older CLI versions are rejected with 426.
func RegisterAgent(log *logger.Logger, index *agentindex.Index, omClient *openmeter.Client, minCLIVersion string) gin.HandlerFunc {
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
		agentName := c.Param("name")

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

				validator := deployment.NewValidator()
				result := validator.ValidateSpec(&astroSpec, nil, nil, nil)

				// Keep only spec-structural errors; exclude deploy-time errors
				for _, e := range result.Errors {
					if strings.HasPrefix(e.Field, "credentials.") ||
						strings.HasSuffix(e.Field, ".trigger.schedule") {
						continue
					}
					validationWarnings = append(validationWarnings, e)
				}
			}
		}

		warningsBytes, _ := json.Marshal(validationWarnings)
		validationWarningsJSON = string(warningsBytes)

		// Parse agent card from readme and merge spec-derived integrations at registration time
		agentCardJSON := buildAgentCardJSON(req.Readme, specMap)

		if err := index.Register(accountID, agentName, req.BuildID, req.Registry, accountName, specMap, req.Readme, agentCardJSON, validationWarningsJSON); err != nil {
			log.Error("Failed to register agent", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to register agent",
				"details": err.Error(),
			})
			return
		}

		// Emit agent_build metering event (fire-and-forget)
		go openmeter.EmitAgentBuild(c.Request.Context(), omClient, log, accountID, agentName)

		// Set visibility if provided (only "public" or "private" are valid)
		if req.Visibility == "public" || req.Visibility == "private" {
			if err := index.SetVisibility(accountID, agentName, req.Visibility); err != nil {
				log.Warn("Failed to set visibility during registration", "error", err, "visibility", req.Visibility)
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

// SetAgentVisibilityRequest represents the request to change agent visibility
type SetAgentVisibilityRequest struct {
	Visibility string `json:"visibility" binding:"required"`
}

// ArchiveAgent handles POST /api/v1/agents/:account/:name/archive
// Soft-deletes an agent by setting archived_at, hiding it from listings
// while preserving data for existing deployments.
// Requires agents:write permission (enforced by middleware).
func ArchiveAgent(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
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

		log.Info("Agent archived", "account", accountName, "name", agentName)
		c.Status(http.StatusNoContent)
	}
}

// SetAgentVisibility handles PUT /api/v1/agents/:account/:name/visibility
// Toggles an agent between public and private visibility.
// Requires agents:write permission (enforced by middleware).
func SetAgentVisibility(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
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

		c.JSON(http.StatusOK, gin.H{
			"message":    "visibility updated",
			"account":    accountName,
			"name":       agentName,
			"visibility": req.Visibility,
		})
	}
}
