package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/agentindex"
	"github.com/postman/astro/apps/astro-server/internal/deployment"
	"github.com/postman/astro/apps/astro-server/internal/logger"
	"github.com/postman/astro/apps/astro-server/internal/spec"
	"gopkg.in/yaml.v3"
)

// AgentResponse represents an agent with all its versions
type AgentResponse struct {
	Name     string                 `json:"name"`
	Registry string                 `json:"registry"`
	Versions []AgentVersionResponse `json:"versions"`
}

// AgentVersionResponse represents a specific version of an agent
type AgentVersionResponse struct {
	Version     string                 `json:"version"`
	Spec        map[string]interface{} `json:"spec"`
	PublishedAt string                 `json:"published_at"`
}

// RegisterAgentRequest represents the request to register an agent
type RegisterAgentRequest struct {
	Name        string `json:"name" binding:"required"`
	Version     string `json:"version" binding:"required"`
	Registry    string `json:"registry" binding:"required"`
	SpecContent string `json:"spec_content" binding:"required"`
}

// ListAgents handles GET /api/v1/agents
// Lists all available agents in the index
func ListAgents(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		log.Info("Listing agents from index")

		agents, err := index.List()
		if err != nil {
			log.Error("Failed to list agents", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list agents from index",
				"details": err.Error(),
			})
			return
		}

		responses := make([]AgentResponse, 0, len(agents))
		for _, agent := range agents {
			versions := make([]AgentVersionResponse, 0, len(agent.Versions))
			for _, v := range agent.Versions {
				versions = append(versions, AgentVersionResponse{
					Version:     v.Version,
					Spec:        v.Spec,
					PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
				})
			}

			responses = append(responses, AgentResponse{
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

// GetAgent handles GET /api/v1/agents/:name
// Gets information about a specific agent with all versions
func GetAgent(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")

		log.Info("Getting agent details", "name", name)

		agent, err := index.Get(name)
		if err != nil {
			log.Error("Failed to get agent", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Agent not found",
				"details": err.Error(),
			})
			return
		}

		versions := make([]AgentVersionResponse, 0, len(agent.Versions))
		for _, v := range agent.Versions {
			versions = append(versions, AgentVersionResponse{
				Version:     v.Version,
				Spec:        v.Spec,
				PublishedAt: v.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
			})
		}

		c.JSON(http.StatusOK, AgentResponse{
			Name:     agent.Name,
			Registry: agent.Registry,
			Versions: versions,
		})
	}
}

// GetAgentVersion handles GET /api/v1/agents/:name/:version
// Gets information about a specific agent version
func GetAgentVersion(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		version := c.Param("version")

		log.Info("Getting agent version details",
			"name", name,
			"version", version,
		)

		agentVersion, err := index.GetVersion(name, version)
		if err != nil {
			log.Error("Failed to get agent version", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Agent version not found",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, AgentVersionResponse{
			Version:     agentVersion.Version,
			Spec:        agentVersion.Spec,
			PublishedAt: agentVersion.PublishedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
}

// RegisterAgent handles POST /api/v1/agents/register
// Registers a new agent or updates an existing one in the index
func RegisterAgent(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req RegisterAgentRequest

		if err := c.ShouldBindJSON(&req); err != nil {
			log.Error("Invalid request body", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid request body",
				"details": err.Error(),
			})
			return
		}

		log.Info("Registering agent",
			"name", req.Name,
			"version", req.Version,
		)

		// Parse YAML spec to structured format
		var spec map[string]interface{}
		if err := yaml.Unmarshal([]byte(req.SpecContent), &spec); err != nil {
			log.Error("Failed to parse spec YAML", "error", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "Invalid spec YAML",
				"details": err.Error(),
			})
			return
		}

		if err := index.Register(req.Name, req.Version, req.Registry, spec); err != nil {
			log.Error("Failed to register agent", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to register agent",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusCreated, gin.H{
			"message": "Agent registered successfully",
			"name":    req.Name,
			"version": req.Version,
		})
	}
}

// GetAgentCredentials handles GET /api/v1/agents/:name/:version/credentials
// Returns required credentials for deploying a specific agent version
func GetAgentCredentials(log *logger.Logger, index *agentindex.Index) gin.HandlerFunc {
	return func(c *gin.Context) {
		name := c.Param("name")
		version := c.Param("version")

		log.Info("Getting required credentials for agent",
			"name", name,
			"version", version,
		)

		// Get agent version from index
		agentVersion, err := index.GetVersion(name, version)
		if err != nil {
			log.Error("Failed to get agent version", "error", err)
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "Agent version not found",
				"details": err.Error(),
			})
			return
		}

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

		// Get required credentials
		validator := deployment.NewValidator()
		credentials := validator.GetRequiredCredentials(&astroSpec)

		c.JSON(http.StatusOK, gin.H{
			"agent":       name,
			"version":     version,
			"credentials": credentials,
		})
	}
}
