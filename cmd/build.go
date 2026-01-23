package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/build"
	"github.com/moby/go-archive"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"

	"github.com/postman/astro/apps/astro-cli/internal/spec"
)

var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Build agent and custom component containers from spec",
	Long: `Build container images defined in astro.yml.

This command builds:
- The agent container (container.build)
- Custom-built models (models.*.container.build)
- Custom-built knowledge stores (knowledge.*.container.build)
- Custom-built tools (tools.*.container.build)
- Custom interface services (interfaces.*.service.build)

Components with pre-built images (container.image) are skipped and will be
pulled at deployment time by the deployment server.

Example:
  astro build
  astro build --tag v1.0.0
  astro build --no-cache`,
	RunE: runBuild,
}

var (
	buildTag     string
	buildNoCache bool
)

func init() {
	rootCmd.AddCommand(buildCmd)
	buildCmd.Flags().StringVarP(&buildTag, "tag", "t", "latest", "Tag for the images")
	buildCmd.Flags().BoolVar(&buildNoCache, "no-cache", false, "Build without using cache")
}

func runBuild(cmd *cobra.Command, args []string) error {
	// Get spec file path
	specFile, _ := cmd.Flags().GetString("file")
	verbose, _ := cmd.Flags().GetBool("verbose")

	log.Printf("🔨 Building agent from spec: %s", specFile)

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

	// Create Docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close()

	imagesBuilt := 0

	// Build agent container
	if astroSpec.Container.Build == nil && astroSpec.Container.Image == "" {
		return fmt.Errorf("container.build or container.image must be specified in spec")
	}

	if astroSpec.Container.Build != nil {
		imageName := fmt.Sprintf("%s:%s", astroSpec.Agent, buildTag)
		log.Printf("📦 Building agent container: %s", imageName)

		contextPath := filepath.Join(workingDir, astroSpec.Container.Build.Context)
		dockerfile := astroSpec.Container.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}

		if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, astroSpec.Container.Build.Args, buildNoCache, verbose); err != nil {
			return fmt.Errorf("failed to build agent image: %w", err)
		}

		imagesBuilt++
	} else if astroSpec.Container.Image != "" {
		log.Printf("⏭️  Agent using pre-built image: %s", astroSpec.Container.Image)
	}

	// Build custom model containers (those with build config)
	for name, model := range astroSpec.Models {
		if model.Container.Build != nil {
			imageName := fmt.Sprintf("%s-model-%s:%s", astroSpec.Agent, name, buildTag)
			log.Printf("📦 Building model container: %s", imageName)

			contextPath := filepath.Join(workingDir, model.Container.Build.Context)
			dockerfile := model.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, model.Container.Build.Args, buildNoCache, verbose); err != nil {
				return fmt.Errorf("failed to build model %s: %w", name, err)
			}

			imagesBuilt++
		} else if model.Container.Image != "" {
			log.Printf("⏭️  Model '%s' using pre-built image: %s", name, model.Container.Image)
		}
	}

	// Build custom knowledge store containers (those with build config)
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Build != nil {
			imageName := fmt.Sprintf("%s-knowledge-%s:%s", astroSpec.Agent, name, buildTag)
			log.Printf("📦 Building knowledge store container: %s", imageName)

			contextPath := filepath.Join(workingDir, knowledge.Container.Build.Context)
			dockerfile := knowledge.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, knowledge.Container.Build.Args, buildNoCache, verbose); err != nil {
				return fmt.Errorf("failed to build knowledge store %s: %w", name, err)
			}

			imagesBuilt++
		} else if knowledge.Container.Image != "" {
			log.Printf("⏭️  Knowledge store '%s' using pre-built image: %s", name, knowledge.Container.Image)
		}
	}

	// Build custom tool containers (those with build config)
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			imageName := fmt.Sprintf("%s-tool-%s:%s", astroSpec.Agent, name, buildTag)
			log.Printf("📦 Building tool container: %s", imageName)

			contextPath := filepath.Join(workingDir, tool.Container.Build.Context)
			dockerfile := tool.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, tool.Container.Build.Args, buildNoCache, verbose); err != nil {
				return fmt.Errorf("failed to build tool %s: %w", name, err)
			}

			imagesBuilt++
		} else if tool.Container != nil && tool.Container.Image != "" {
			log.Printf("⏭️  Tool '%s' using pre-built image: %s", name, tool.Container.Image)
		}
	}

	// Build custom interface service containers (those with build config)
	for name, iface := range astroSpec.Interfaces {
		if iface.Service != nil && iface.Service.Build != nil {
			imageName := fmt.Sprintf("%s-interface-%s:%s", astroSpec.Agent, name, buildTag)
			log.Printf("📦 Building interface service container: %s", imageName)

			contextPath := filepath.Join(workingDir, iface.Service.Build.Context)
			dockerfile := iface.Service.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, iface.Service.Build.Args, buildNoCache, verbose); err != nil {
				return fmt.Errorf("failed to build interface %s: %w", name, err)
			}

			imagesBuilt++
		} else if iface.Service != nil && iface.Service.Image != "" {
			log.Printf("⏭️  Interface '%s' using pre-built image: %s", name, iface.Service.Image)
		}
	}

	log.Printf("")
	log.Printf("✅ Build complete! Built %d custom container(s)", imagesBuilt)

	return nil
}

func buildImageSDK(ctx context.Context, cli *client.Client, contextPath, dockerfile, imageName string, buildArgs map[string]string, noCache, verbose bool) error {
	// Create build context tar
	buildContext, err := archive.TarWithOptions(contextPath, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildContext.Close()

	// Convert buildArgs map[string]string to map[string]*string
	buildArgsPtr := make(map[string]*string)
	for k, v := range buildArgs {
		val := v
		buildArgsPtr[k] = &val
	}

	// Prepare build options
	opts := build.ImageBuildOptions{
		Dockerfile: dockerfile,
		Tags:       []string{imageName},
		Remove:     true,
		NoCache:    noCache,
		BuildArgs:  buildArgsPtr,
	}

	// Build the image
	resp, err := cli.ImageBuild(ctx, buildContext, opts)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close()

	// Stream build output
	if err := streamBuildOutput(resp.Body, verbose); err != nil {
		return fmt.Errorf("error during build: %w", err)
	}

	return nil
}

func streamBuildOutput(reader io.Reader, verbose bool) error {
	decoder := json.NewDecoder(reader)
	var lastError string

	for {
		var msg struct {
			Stream      string `json:"stream"`
			Error       string `json:"error"`
			ErrorDetail struct {
				Message string `json:"message"`
			} `json:"errorDetail"`
		}

		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		if msg.Error != "" {
			lastError = msg.Error
			log.Printf("❌ Error: %s", msg.Error)
		}

		if verbose && msg.Stream != "" {
			// Clean up and print build output
			output := strings.TrimSpace(msg.Stream)
			if output != "" {
				log.Printf("   %s", output)
			}
		}
	}

	if lastError != "" {
		return fmt.Errorf("build failed: %s", lastError)
	}

	return nil
}
