package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
	"github.com/postman/astro/apps/astro-server/internal/openmeter"
	"github.com/postman/astro/packages/astro-spec"
	"gopkg.in/yaml.v3"
)

// AgentResponse represents an agent with all its versions
type AgentResponse struct {
	Account    string                 `json:"account"`
	Name       string                 `json:"name"`
	Registry   string                 `json:"registry"`
	Visibility string                 `json:"visibility"`
	Versions   []AgentVersionResponse `json:"versions"`
}

// AgentVersionResponse represents a specific version of an agent
type AgentVersionResponse struct {
	BuildID            string           `json:"build_id"`
	Spec               map[string]any   `json:"spec"`
	Readme             string           `json:"readme"`
	PublishedAt        string           `json:"published_at"`
	ValidationWarnings []map[string]any `json:"validation_warnings,omitempty"`
}

// RegisterAgentRequest represents the request to register an agent
type RegisterAgentRequest struct {
	BuildID     string `json:"build_id" binding:"required"`
	Registry    string `json:"registry" binding:"required"`
	SpecContent string `json:"spec_content" binding:"required"`
	Readme      string `json:"readme"`
	Visibility  string `json:"visibility,omitempty"` // "public" or "private"; only applied on first registration
}

// ListAgents handles GET /api/v1/agents
// Lists agents with visibility='public' (public catalog)
func ListAgents(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore) gin.HandlerFunc {
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

			versions := make([]AgentVersionResponse, 0, len(agent.Versions))
			for _, v := range agent.Versions {
				versions = append(versions, AgentVersionResponse{
					BuildID:     v.BuildID,
					Spec:        v.Spec,
					Readme:      v.Readme,
					PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
				})
			}

			responses = append(responses, AgentResponse{
				Account:    accountName,
				Name:       agent.Name,
				Registry:   agent.Registry,
				Visibility: agent.Visibility,
				Versions:   versions,
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
func GetAgent(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore) gin.HandlerFunc {
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
		if user, exists := middleware.GetUser(c); exists {
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
			resp := AgentVersionResponse{
				BuildID:     v.BuildID,
				Spec:        v.Spec,
				Readme:      v.Readme,
				PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
			}
			if isMember {
				resp.ValidationWarnings = v.ValidationWarnings
			}
			versions = append(versions, resp)
		}

		c.JSON(http.StatusOK, AgentResponse{
			Account:    accountName,
			Name:       agent.Name,
			Registry:   agent.Registry,
			Visibility: agent.Visibility,
			Versions:   versions,
		})
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
			cliVersion := c.GetHeader("X-CLI-Version")
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

		if err := index.Register(accountID, agentName, req.BuildID, req.Registry, specMap, req.Readme, validationWarningsJSON); err != nil {
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
		c.JSON(http.StatusCreated, response)
	}
}

// SetAgentVisibilityRequest represents the request to change agent visibility
type SetAgentVisibilityRequest struct {
	Visibility string `json:"visibility" binding:"required"`
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

// GetAgentConfig handles GET /api/v1/agents/:account/:name/config
// Returns required credentials and config for deploying the latest build
func GetAgentConfig(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		name := c.Param("name")

		log.Info("Getting required config for agent",
			"account", accountName,
			"name", name,
		)

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}
		accountID := acct.ID

		isMember, err := accountStore.IsMember(accountID, user.ID)
		if err != nil || !isMember {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		// Get latest build from index
		agentVersion, err := index.GetLatestVersion(accountID, name)
		if err != nil {
			log.Error("Failed to get latest agent build", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "No builds found for agent",
				"details": err.Error(),
			})
			return
		}

		buildID := agentVersion.BuildID

		// Parse spec from stored map
		specJSON, err := json.Marshal(agentVersion.Spec)
		if err != nil {
			log.Error("Failed to marshal spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to process agent spec",
				"details": err.Error(),
			})
			return
		}

		var astroSpec spec.AstroSpec
		if err := json.Unmarshal(specJSON, &astroSpec); err != nil {
			log.Error("Failed to unmarshal spec", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to parse agent spec",
				"details": err.Error(),
			})
			return
		}

		// Parse optional interfaces query param (e.g. ?interfaces=slack,web)
		var interfaces []string
		if ifaceParam := c.Query("interfaces"); ifaceParam != "" {
			for _, s := range strings.Split(ifaceParam, ",") {
				s = strings.TrimSpace(s)
				if s != "" {
					interfaces = append(interfaces, s)
				}
			}
		}

		// Get required credentials
		validator := deployment.NewValidator()
		credentials := validator.GetRequiredCredentials(&astroSpec, interfaces)

		c.JSON(http.StatusOK, gin.H{
			"agent":       name,
			"build_id":    buildID,
			"credentials": credentials,
		})
	}
}
