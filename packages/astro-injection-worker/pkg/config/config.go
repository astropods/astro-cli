package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Config holds the injection worker configuration
type Config struct {
	// Source configuration
	SourceType   string
	SourceConfig map[string]interface{}

	// Pipeline configuration
	Pipeline []PipelineStep

	// Target configuration
	TargetType string
	TargetHost string
	TargetPort int

	// Embedder configuration
	EmbedderURL string

	// Authentication
	GithubToken string
	GitlabToken string

	// Target collection configuration
	CollectionName string
	VectorSize     int

	// Flags
	Persistent bool
	DryRun     bool
}

// PipelineStep represents a step in the injection pipeline
type PipelineStep struct {
	Step   string                 `json:"step"`
	Model  string                 `json:"model,omitempty"`
	Target string                 `json:"target,omitempty"`
	Config map[string]interface{} `json:"config,omitempty"`
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		SourceType: os.Getenv("INJECTION_SOURCE_TYPE"),
		Persistent: os.Getenv("INJECTION_PERSISTENT") == "true",
		DryRun:     os.Getenv("DRY_RUN") == "true",
	}

	// Load source config from environment
	sourceConfigJSON := os.Getenv("INJECTION_SOURCE_CONFIG")
	if sourceConfigJSON != "" {
		if err := json.Unmarshal([]byte(sourceConfigJSON), &cfg.SourceConfig); err != nil {
			return nil, fmt.Errorf("failed to parse source config: %w", err)
		}
	} else {
		cfg.SourceConfig = make(map[string]interface{})
	}

	// Load pipeline configuration
	pipelineJSON := os.Getenv("INJECTION_PIPELINE")
	if pipelineJSON != "" {
		if err := json.Unmarshal([]byte(pipelineJSON), &cfg.Pipeline); err != nil {
			return nil, fmt.Errorf("failed to parse pipeline: %w", err)
		}
	}

	// Load target configuration from connection strings
	cfg.TargetHost = os.Getenv("VECTOR_HOST")
	if cfg.TargetHost == "" {
		cfg.TargetHost = os.Getenv("QDRANT_HOST")
	}

	portStr := os.Getenv("VECTOR_PORT")
	if portStr == "" {
		portStr = os.Getenv("QDRANT_PORT")
	}
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		cfg.TargetPort = port
	} else {
		cfg.TargetPort = 6333 // default Qdrant port
	}

	// Load embedder URL
	cfg.EmbedderURL = os.Getenv("MODEL_EMBEDDER_URL")
	if cfg.EmbedderURL == "" {
		cfg.EmbedderURL = os.Getenv("EMBEDDER_URL")
	}

	// Load authentication tokens
	cfg.GithubToken = os.Getenv("GITHUB_TOKEN")
	cfg.GitlabToken = os.Getenv("GITLAB_TOKEN")

	// Load collection configuration
	cfg.CollectionName = os.Getenv("INJECTION_COLLECTION_NAME")
	if cfg.CollectionName == "" {
		cfg.CollectionName = "astro-docs" // fallback default
	}

	vectorSizeStr := os.Getenv("INJECTION_VECTOR_SIZE")
	if vectorSizeStr != "" {
		size, err := strconv.Atoi(vectorSizeStr)
		if err != nil {
			return nil, fmt.Errorf("invalid vector size: %w", err)
		}
		cfg.VectorSize = size
	} else {
		cfg.VectorSize = 384 // fallback default
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.SourceType == "" {
		return fmt.Errorf("source type is required")
	}

	// GitHub token is optional for public repos
	// GitLab token is optional for public repos

	if c.TargetHost == "" {
		return fmt.Errorf("target host is required")
	}

	return nil
}
