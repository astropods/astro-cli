package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/account"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/middleware"
	"github.com/postman/astro/packages/astro-spec"
	"gopkg.in/yaml.v3"
)

// AgentResponse represents an agent with all its versions
type AgentResponse struct {
	Account  string                 `json:"account"`
	Name     string                 `json:"name"`
	Registry string                 `json:"registry"`
	Versions []AgentVersionResponse `json:"versions"`
}

// AgentVersionResponse represents a specific version of an agent
type AgentVersionResponse struct {
	BuildID            string           `json:"build_id"`
	Version            string           `json:"version,omitempty"`
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
}

// ListAgents handles GET /api/v1/agents
// Lists only agents with published semver versions (public catalog)
func ListAgents(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Info("Listing public agents from index")

		agents, err := index.ListPublic()
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
					Version:     v.Version,
					Spec:        v.Spec,
					Readme:      v.Readme,
					PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
				})
			}

			responses = append(responses, AgentResponse{
				Account:  accountName,
				Name:     agent.Name,
				Registry: agent.Registry,
				Versions: versions,
			})
		}

		c.JSON(http.StatusOK, gin.H{
			"agents": responses,
			"count":  len(responses),
		})
	}
}

// GetAgent handles GET /api/v1/agents/:account/:name
// Public visitors see only published versions; authenticated account members see all builds
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
		isOwner := false
		if user, exists := middleware.GetUser(c); exists {
			isMember, err := accountStore.IsMember(acct.ID, user.ID)
			if err == nil && isMember {
				isOwner = true
			}
		}

		if isOwner {
			// Account member: return all builds with semver annotations
			publishedVersions, _ := index.GetPublishedVersionsForAgent(acct.ID, name)
			buildToVersion := make(map[string]string)
			for _, pv := range publishedVersions {
				buildToVersion[pv.BuildID] = pv.Version
			}

			versions := make([]AgentVersionResponse, 0, len(agent.Versions))
			for _, v := range agent.Versions {
				versions = append(versions, AgentVersionResponse{
					BuildID:            v.BuildID,
					Version:            buildToVersion[v.BuildID],
					Spec:               v.Spec,
					Readme:             v.Readme,
					PublishedAt:        v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
					ValidationWarnings: v.ValidationWarnings,
				})
			}

			c.JSON(http.StatusOK, AgentResponse{
				Account:  accountName,
				Name:     agent.Name,
				Registry: agent.Registry,
				Versions: versions,
			})
		} else {
			// Public visitor: only published versions
			publishedVersions, err := index.GetPublishedVersionsForAgent(acct.ID, name)
			if err != nil || len(publishedVersions) == 0 {
				c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
				return
			}

			versions := make([]AgentVersionResponse, 0, len(publishedVersions))
			for _, v := range publishedVersions {
				versions = append(versions, AgentVersionResponse{
					BuildID:     v.BuildID,
					Version:     v.Version,
					Spec:        v.Spec,
					Readme:      v.Readme,
					PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
				})
			}

			c.JSON(http.StatusOK, AgentResponse{
				Account:  accountName,
				Name:     agent.Name,
				Registry: agent.Registry,
				Versions: versions,
			})
		}
	}
}

// RegisterAgent handles POST /api/v1/agents/:account/:name/register
// Registers a new agent or updates an existing one in the index
func RegisterAgent(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
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

		// Resolve account and verify access
		var accountID string
		if accountStore != nil && accountName != "" {
			acct, err := accountStore.GetByName(accountName)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
				return
			}
			accountID = acct.ID

			// Verify user has write access (owner or admin)
			user, exists := middleware.GetUser(c)
			if !exists {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
				return
			}

			hasRole, err := accountStore.HasRole(acct.ID, user.ID, "owner", "admin")
			if err != nil || !hasRole {
				c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
				return
			}
		}

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

// PublishAgentRequest represents the request to publish an agent version
type PublishAgentRequest struct {
	BuildID string `json:"build_id" binding:"required"`
	Version string `json:"version" binding:"required"`
}

// PublishAgent handles POST /api/v1/agents/:account/:name/publish
// Assigns a semver version to a build, making it publicly visible
func PublishAgent(log *logger.Logger, index *agentindex.Index, accountStore *account.AccountStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountName := c.Param("account")
		agentName := c.Param("name")

		var req PublishAgentRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid request body",
				"details": err.Error(),
			})
			return
		}

		// Resolve account and verify access
		acct, err := accountStore.GetByName(accountName)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
			return
		}

		user, exists := middleware.GetUser(c)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}

		hasRole, err := accountStore.HasRole(acct.ID, user.ID, "owner", "admin")
		if err != nil || !hasRole {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions for this account"})
			return
		}

		// Verify build exists
		_, err = index.GetVersion(acct.ID, agentName, req.BuildID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "build not found", "details": err.Error()})
			return
		}

		if err := index.Publish(acct.ID, agentName, req.BuildID, req.Version); err != nil {
			log.Error("Failed to publish agent", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "failed to publish agent",
				"details": err.Error(),
			})
			return
		}

		log.Info("Agent published",
			"account", accountName,
			"name", agentName,
			"build_id", req.BuildID,
			"version", req.Version,
		)

		c.JSON(http.StatusCreated, gin.H{
			"message":  "agent published",
			"account":  accountName,
			"name":     agentName,
			"build_id": req.BuildID,
			"version":  req.Version,
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

		var accountID string
		if accountStore != nil && accountName != "" {
			acct, err := accountStore.GetByName(accountName)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "account not found"})
				return
			}
			accountID = acct.ID
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
