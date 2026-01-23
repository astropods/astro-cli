package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/daemon"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/postman/astro/apps/astro-cli/internal/spec"
)

var publishCmd = &cobra.Command{
	Use:   "publish",
	Short: "Publish agent, custom components, and spec to OCI registry",
	Long: `Publish container images and spec to an OCI registry.

This will:
1. Tag and push the agent container image
2. Tag and push custom-built component images (models, knowledge, tools)
3. Package and push the astro.yml spec as an OCI artifact using ORAS

The deployment server pulls the spec and all images to provision the agent
infrastructure. Pre-built images (like redis, qdrant) are pulled directly
from their public registries.

Example:
  astro publish --registry ghcr.io/company
  astro publish --registry ghcr.io/company --tag v1.0.0
  astro publish --build --registry ghcr.io/company

Requirements:
  - Docker registry credentials must be configured (docker login)`,
	RunE: runPublish,
}

var (
	registry      string
	publishTag    string
	buildFirst    bool
	serverURL     string
	skipRegister  bool
)

func init() {
	rootCmd.AddCommand(publishCmd)
	publishCmd.Flags().StringVarP(&registry, "registry", "r", "", "OCI registry URL (required)")
	publishCmd.Flags().StringVarP(&publishTag, "tag", "t", "latest", "Tag to publish")
	publishCmd.Flags().BoolVar(&buildFirst, "build", false, "Build before publishing")
	publishCmd.Flags().StringVar(&serverURL, "server", "", "Astro server URL for agent registration (optional)")
	publishCmd.Flags().BoolVar(&skipRegister, "skip-register", false, "Skip registering agent with server")
	publishCmd.MarkFlagRequired("registry")
}

func runPublish(cmd *cobra.Command, args []string) error {
	// Get spec file path
	specFile, _ := cmd.Flags().GetString("file")
	verbose, _ := cmd.Flags().GetBool("verbose")

	log.Printf("📦 Publishing agent to registry: %s", registry)

	// Parse astro.yml
	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath := filepath.Join(workingDir, specFile)
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	log.Printf("✅ Loaded spec for agent: %s (v%s)", astroSpec.Agent, astroSpec.Meta.Version)

	// Build image first if requested
	if buildFirst {
		log.Printf("🔨 Building agent image before publish...")
		if err := runBuild(cmd, args); err != nil {
			return fmt.Errorf("failed to build image: %w", err)
		}
		log.Printf("")
	}

	registryClean := strings.TrimSuffix(registry, "/")
	imagesPushed := 0

	// 1. Publish agent container image
	localImageName := fmt.Sprintf("%s:%s", astroSpec.Agent, publishTag)
	remoteImageName := fmt.Sprintf("%s/%s:%s", registryClean, astroSpec.Agent, publishTag)

	log.Printf("🏷️  Tagging agent image: %s -> %s", localImageName, remoteImageName)
	if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
		return fmt.Errorf("failed to tag agent image: %w", err)
	}

	log.Printf("⬆️  Pushing agent image: %s", remoteImageName)
	if err := pushImage(localImageName, remoteImageName, verbose); err != nil {
		return fmt.Errorf("failed to push agent image: %w", err)
	}
	imagesPushed++

	// 2. Publish custom-built model images
	for name, model := range astroSpec.Models {
		if model.Container.Build != nil {
			localImageName := fmt.Sprintf("%s-model-%s:%s", astroSpec.Agent, name, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s-model-%s:%s", registryClean, astroSpec.Agent, name, publishTag)

			log.Printf("🏷️  Tagging model image: %s -> %s", localImageName, remoteImageName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to tag model %s: %w", name, err)
			}

			log.Printf("⬆️  Pushing model image: %s", remoteImageName)
			if err := pushImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to push model %s: %w", name, err)
			}
			imagesPushed++
		}
	}

	// 3. Publish custom-built knowledge store images
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Build != nil {
			localImageName := fmt.Sprintf("%s-knowledge-%s:%s", astroSpec.Agent, name, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s-knowledge-%s:%s", registryClean, astroSpec.Agent, name, publishTag)

			log.Printf("🏷️  Tagging knowledge store image: %s -> %s", localImageName, remoteImageName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to tag knowledge store %s: %w", name, err)
			}

			log.Printf("⬆️  Pushing knowledge store image: %s", remoteImageName)
			if err := pushImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to push knowledge store %s: %w", name, err)
			}
			imagesPushed++
		}
	}

	// 4. Publish custom-built tool images
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			localImageName := fmt.Sprintf("%s-tool-%s:%s", astroSpec.Agent, name, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s-tool-%s:%s", registryClean, astroSpec.Agent, name, publishTag)

			log.Printf("🏷️  Tagging tool image: %s -> %s", localImageName, remoteImageName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to tag tool %s: %w", name, err)
			}

			log.Printf("⬆️  Pushing tool image: %s", remoteImageName)
			if err := pushImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to push tool %s: %w", name, err)
			}
			imagesPushed++
		}
	}

	// 5. Publish custom-built interface service images
	for name, iface := range astroSpec.Interfaces {
		if iface.Service != nil && iface.Service.Build != nil {
			localImageName := fmt.Sprintf("%s-interface-%s:%s", astroSpec.Agent, name, publishTag)
			remoteImageName := fmt.Sprintf("%s/%s-interface-%s:%s", registryClean, astroSpec.Agent, name, publishTag)

			log.Printf("🏷️  Tagging interface service image: %s -> %s", localImageName, remoteImageName)
			if err := tagImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to tag interface %s: %w", name, err)
			}

			log.Printf("⬆️  Pushing interface service image: %s", remoteImageName)
			if err := pushImage(localImageName, remoteImageName, verbose); err != nil {
				return fmt.Errorf("failed to push interface %s: %w", name, err)
			}
			imagesPushed++
		}
	}

	log.Printf("")
	log.Printf("✅ Image publish complete!")
	log.Printf("   Pushed %d custom container image(s)", imagesPushed)

	// 5. Register agent with server (if server URL is provided)
	if !skipRegister && serverURL != "" {
		log.Printf("📝 Registering agent with server...")

		if err := registerAgent(serverURL, astroSpec.Agent, astroSpec.Meta.Version, registryClean, specPath, publishTag, verbose); err != nil {
			log.Printf("⚠️  Warning: Failed to register agent with server: %v", err)
			log.Printf("   Agent images were published successfully, but registration failed")
		} else {
			log.Printf("✅ Agent registered successfully!")
		}
	} else if skipRegister {
		log.Printf("⏭️  Skipping agent registration (--skip-register flag set)")
	} else {
		log.Printf("ℹ️  No server URL provided (use --server to register agent)")
	}

	return nil
}

func tagImage(sourceImage, targetImage string, verbose bool) error {
	tagCmd := exec.Command("docker", "tag", sourceImage, targetImage)

	if verbose {
		tagCmd.Stdout = os.Stdout
		tagCmd.Stderr = os.Stderr
	}

	if err := tagCmd.Run(); err != nil {
		return fmt.Errorf("failed to tag image: %w", err)
	}

	return nil
}

func pushImage(localImageName, remoteImageName string, verbose bool) error {
	// Use crane to copy image from Docker daemon to registry
	// This bypasses Docker daemon proxy configuration

	if verbose {
		log.Printf("   Loading image from Docker daemon: %s", localImageName)
	}

	// Parse the local image name reference
	localRef, err := name.ParseReference(localImageName)
	if err != nil {
		return fmt.Errorf("failed to parse local image name: %w", err)
	}

	// Load the image from Docker daemon
	img, err := daemon.Image(localRef)
	if err != nil {
		return fmt.Errorf("failed to load image from Docker daemon: %w", err)
	}

	if verbose {
		log.Printf("   Pushing to registry: %s", remoteImageName)
	}

	// Parse the remote image name reference
	remoteRef, err := name.ParseReference(remoteImageName)
	if err != nil {
		return fmt.Errorf("failed to parse remote image name: %w", err)
	}

	// Push the image to the remote registry
	// crane.Push uses Docker credentials automatically
	if err := crane.Push(img, remoteRef.Name()); err != nil {
		return fmt.Errorf("failed to push image with crane: %w", err)
	}

	return nil
}

// registerAgent registers the agent with the astro server
func registerAgent(serverURL, name, version, registry, specPath, publishTag string, verbose bool) error {
	// Read and parse spec file
	specData, err := os.ReadFile(specPath)
	if err != nil {
		return fmt.Errorf("failed to read spec file: %w", err)
	}

	// Parse YAML spec
	var specObj map[string]interface{}
	if err := yaml.Unmarshal(specData, &specObj); err != nil {
		return fmt.Errorf("failed to parse spec YAML: %w", err)
	}

	// Transform spec: replace build sections with actual image references
	specObj = transformSpecForRegistry(specObj, registry, name, publishTag)

	// Marshal back to YAML
	transformedSpecData, err := yaml.Marshal(specObj)
	if err != nil {
		return fmt.Errorf("failed to marshal transformed spec: %w", err)
	}

	// Prepare request payload
	payload := map[string]string{
		"name":         name,
		"version":      version,
		"registry":     registry,
		"spec_content": string(transformedSpecData),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send POST request to server
	url := fmt.Sprintf("%s/api/v1/agents/register", strings.TrimSuffix(serverURL, "/"))
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		var errorResp map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorResp); err == nil {
			return fmt.Errorf("server returned error (status %d): %v", resp.StatusCode, errorResp)
		}
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	if verbose {
		var result map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			log.Printf("   Server response: %v", result)
		}
	}

	return nil
}

// transformSpecForRegistry replaces build sections with actual image references
func transformSpecForRegistry(specObj map[string]interface{}, registry, agentName, tag string) map[string]interface{} {
	// Replace container.build with container.image
	if container, ok := specObj["container"].(map[string]interface{}); ok {
		if _, hasBuild := container["build"]; hasBuild {
			delete(container, "build")
			container["image"] = fmt.Sprintf("%s/%s:%s", registry, agentName, tag)
		}
	}

	// Replace models.*.container.build with models.*.container.image
	if models, ok := specObj["models"].(map[string]interface{}); ok {
		for modelName, modelData := range models {
			if model, ok := modelData.(map[string]interface{}); ok {
				if container, ok := model["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-model-%s:%s", registry, agentName, modelName, tag)
					}
				}
			}
		}
	}

	// Replace knowledge.*.container.build with knowledge.*.container.image
	if knowledge, ok := specObj["knowledge"].(map[string]interface{}); ok {
		for knowledgeName, knowledgeData := range knowledge {
			if knowledgeItem, ok := knowledgeData.(map[string]interface{}); ok {
				if container, ok := knowledgeItem["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-knowledge-%s:%s", registry, agentName, knowledgeName, tag)
					}
				}
			}
		}
	}

	// Replace tools.*.container.build with tools.*.container.image
	if tools, ok := specObj["tools"].(map[string]interface{}); ok {
		for toolName, toolData := range tools {
			if tool, ok := toolData.(map[string]interface{}); ok {
				if container, ok := tool["container"].(map[string]interface{}); ok {
					if _, hasBuild := container["build"]; hasBuild {
						delete(container, "build")
						container["image"] = fmt.Sprintf("%s/%s-tool-%s:%s", registry, agentName, toolName, tag)
					}
				}
			}
		}
	}

	// Replace interfaces.*.service.build with interfaces.*.service.image
	if interfaces, ok := specObj["interfaces"].(map[string]interface{}); ok {
		for ifaceName, ifaceData := range interfaces {
			if iface, ok := ifaceData.(map[string]interface{}); ok {
				if service, ok := iface["service"].(map[string]interface{}); ok {
					if _, hasBuild := service["build"]; hasBuild {
						delete(service, "build")
						service["image"] = fmt.Sprintf("%s/%s-interface-%s:%s", registry, agentName, ifaceName, tag)
					}
				}
			}
		}
	}

	return specObj
}

