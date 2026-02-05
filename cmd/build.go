package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/api/types/build"
	"github.com/joho/godotenv"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/go-archive"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"

	"github.com/postman/astro/packages/astro-spec"
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
  ast build
  ast build --tag v1.0.0
  ast build --no-cache`,
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
	quiet, _ := cmd.Flags().GetBool("quiet")

	if !quiet {
		fmt.Printf("%s→%s Parsing spec: %s\n", colorCyan, colorReset, specFile)
	}

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

	if !quiet {
		fmt.Printf("%s→%s Agent: %s%s%s (v%s)\n", colorCyan, colorReset, colorBold, astroSpec.Agent, colorReset, astroSpec.Meta.Version)
	}

	// Load .env file for secrets
	envVars := make(map[string]string)
	envPath := filepath.Join(workingDir, ".env")
	if _, err := os.Stat(envPath); err == nil {
		envMap, err := godotenv.Read(envPath)
		if err != nil {
			return fmt.Errorf("failed to read .env file: %w", err)
		}
		envVars = envMap
	}

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
		if !quiet {
			fmt.Printf("%s→%s Building %s[agent]%s %s%s%s", colorCyan, colorReset, colorDim, colorReset, colorBold, imageName, colorReset)
		}

		contextPath := filepath.Join(workingDir, astroSpec.Container.Build.Context)
		dockerfile := astroSpec.Container.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}

		if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, astroSpec.Container.Build.Args, astroSpec.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet); err != nil {
			if !quiet {
				fmt.Printf(" %s✗%s\n", colorRed, colorReset)
			}
			return fmt.Errorf("failed to build agent image: %w", err)
		}

		if !quiet {
			fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
		}
		imagesBuilt++
	} else if astroSpec.Container.Image != "" && !quiet {
		fmt.Printf("%s→%s Skipping %s[agent]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, colorReset, colorDim, astroSpec.Container.Image, colorReset)
	}

	// Build custom model containers (those with build config)
	for name, model := range astroSpec.Models {
		if model.Container.Build != nil {
			imageName := fmt.Sprintf("%s-model-%s:%s", astroSpec.Agent, name, buildTag)
			if !quiet {
				fmt.Printf("%s→%s Building %s[model: %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, colorReset, colorBold, imageName, colorReset)
			}

			contextPath := filepath.Join(workingDir, model.Container.Build.Context)
			dockerfile := model.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, model.Container.Build.Args, model.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet); err != nil {
				if !quiet {
					fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				}
				return fmt.Errorf("failed to build model %s: %w", name, err)
			}

			if !quiet {
				fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
			}
			imagesBuilt++
		} else if model.Container.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[model: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, model.Container.Image, colorReset)
		}
	}

	// Build custom knowledge store containers (those with build config)
	for name, knowledge := range astroSpec.Knowledge {
		if knowledge.Container.Build != nil {
			imageName := fmt.Sprintf("%s-knowledge-%s:%s", astroSpec.Agent, name, buildTag)
			if !quiet {
				fmt.Printf("%s→%s Building %s[knowledge: %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, colorReset, colorBold, imageName, colorReset)
			}

			contextPath := filepath.Join(workingDir, knowledge.Container.Build.Context)
			dockerfile := knowledge.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, knowledge.Container.Build.Args, knowledge.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet); err != nil {
				if !quiet {
					fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				}
				return fmt.Errorf("failed to build knowledge store %s: %w", name, err)
			}

			if !quiet {
				fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
			}
			imagesBuilt++
		} else if knowledge.Container.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[knowledge: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, knowledge.Container.Image, colorReset)
		}
	}

	// Build custom tool containers (those with build config)
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			imageName := fmt.Sprintf("%s-tool-%s:%s", astroSpec.Agent, name, buildTag)
			if !quiet {
				fmt.Printf("%s→%s Building %s[tool: %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, colorReset, colorBold, imageName, colorReset)
			}

			contextPath := filepath.Join(workingDir, tool.Container.Build.Context)
			dockerfile := tool.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, tool.Container.Build.Args, tool.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet); err != nil {
				if !quiet {
					fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				}
				return fmt.Errorf("failed to build tool %s: %w", name, err)
			}

			if !quiet {
				fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
			}
			imagesBuilt++
		} else if tool.Container != nil && tool.Container.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[tool: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, tool.Container.Image, colorReset)
		}
	}

	// Build custom interface service containers (those with build config)
	for name, iface := range astroSpec.Interfaces {
		if iface.Service != nil && iface.Service.Build != nil {
			imageName := fmt.Sprintf("%s-interface-%s:%s", astroSpec.Agent, name, buildTag)
			if !quiet {
				fmt.Printf("%s→%s Building %s[interface: %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, colorReset, colorBold, imageName, colorReset)
			}

			contextPath := filepath.Join(workingDir, iface.Service.Build.Context)
			dockerfile := iface.Service.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, imageName, iface.Service.Build.Args, iface.Service.Build.Secrets, envVars, buildNoCache, verbose, quiet); err != nil {
				if !quiet {
					fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				}
				return fmt.Errorf("failed to build interface %s: %w", name, err)
			}

			if !quiet {
				fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
			}
			imagesBuilt++
		} else if iface.Service != nil && iface.Service.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[interface: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, iface.Service.Image, colorReset)
		}
	}

	if !quiet {
		fmt.Printf("%s✓%s Built %s%d%s image(s)\n", colorGreen, colorReset, colorBold, imagesBuilt, colorReset)
	}

	return nil
}

func buildImageSDK(ctx context.Context, cli *client.Client, contextPath, dockerfile, imageName string, buildArgs map[string]string, secrets []spec.BuildSecret, envVars map[string]string, noCache, verbose, quiet bool) error {
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

	// If secrets are defined, create a BuildKit session with secrets provider
	if len(secrets) > 0 {
		// Create session for BuildKit
		sess, err := session.NewSession(ctx, filepath.Base(contextPath))
		if err != nil {
			return fmt.Errorf("failed to create build session: %w", err)
		}

		// Build secret map from env vars
		secretMap := make(map[string][]byte)
		for _, s := range secrets {
			if val, ok := envVars[s.Env]; ok {
				secretMap[s.ID] = []byte(val)
			}
		}

		// Add secrets provider to session (FromMap returns an Attachable directly)
		sess.Allow(secretsprovider.FromMap(secretMap))

		// Create dialer for session
		dialSession := func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
			return cli.DialHijack(ctx, "/session", proto, meta)
		}

		// Run session in background
		go sess.Run(ctx, dialSession)
		defer sess.Close()

		// Enable BuildKit and attach session
		opts.Version = build.BuilderBuildKit
		opts.SessionID = sess.ID()
	}

	// Build the image
	resp, err := cli.ImageBuild(ctx, buildContext, opts)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close()

	// Stream build output
	if err := streamBuildOutput(resp.Body, verbose, quiet); err != nil {
		return fmt.Errorf("error during build: %w", err)
	}

	return nil
}

func streamBuildOutput(reader io.Reader, verbose, quiet bool) error {
	decoder := json.NewDecoder(reader)
	var lastError string
	var currentStep string

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
			if !quiet {
				fmt.Printf("\n      %s%s%s", colorRed, msg.Error, colorReset)
			}
		}

		if msg.Stream != "" {
			output := strings.TrimSpace(msg.Stream)
			// Show step progress (e.g., "Step 1/5 : FROM golang:1.21")
			if strings.HasPrefix(output, "Step ") {
				if !quiet {
					// Clear previous step indicator and show new one
					if currentStep != "" {
						fmt.Printf("\r      %s%s%s", colorDim, output, colorReset)
					} else {
						fmt.Printf("\n      %s%s%s", colorDim, output, colorReset)
					}
					currentStep = output
				}
			} else if verbose && output != "" {
				// In verbose mode, show all output
				fmt.Printf("\n      %s%s%s", colorDim, output, colorReset)
			}
		}
	}

	// Clear step line
	if !quiet && currentStep != "" {
		fmt.Printf("\r%s", strings.Repeat(" ", 80))
		fmt.Printf("\r")
	}

	if lastError != "" {
		return fmt.Errorf("build failed: %s", lastError)
	}

	return nil
}
