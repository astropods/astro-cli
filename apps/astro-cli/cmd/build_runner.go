package cmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"bufio"

	"github.com/docker/docker/api/types/build"
	dockerimage "github.com/docker/docker/api/types/image"
	controlapi "github.com/moby/buildkit/api/services/control"

	"github.com/astropods/astro/apps/astro-cli/internal/utils"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/go-archive"
	"github.com/moby/moby/client"
	"github.com/spf13/cobra"
	"google.golang.org/protobuf/proto"

	spec "github.com/astropods/astro/packages/astro-spec"
)

var (
	buildTag      string
	buildNoCache  bool
	buildPlatform string
)

func runBuild(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	quiet, _ := cmd.Flags().GetBool("quiet")

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	specPath, err := resolveSpecPath(cmd, workingDir)
	if err != nil {
		return err
	}

	if !quiet {
		fmt.Printf("%s→%s Parsing spec: %s\n", colorCyan, colorReset, filepath.Base(specPath))
	}

	// Parse Astro spec
	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	if !quiet {
		fmt.Printf("%s→%s Agent: %s%s%s\n", colorCyan, colorReset, colorBold, astroSpec.Name, colorReset)
	}

	// Load .env file for secrets
	envVars, err := utils.LoadEnvFile(workingDir, utils.DefaultEnvFile)
	if err != nil {
		return fmt.Errorf("failed to read .env file: %w", err)
	}
	if envVars == nil {
		envVars = make(map[string]string)
	}

	// Create Docker client
	ctx := context.Background()
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return fmt.Errorf("failed to create Docker client: %w", err)
	}
	defer cli.Close() //nolint:errcheck

	imagesBuilt := 0
	platforms := parsePlatforms(buildPlatform)

	// Build agent container
	if astroSpec.Agent.Build == nil && astroSpec.Agent.Image == "" {
		return fmt.Errorf("agent.build or agent.image must be specified in spec")
	}

	if astroSpec.Agent.Build != nil {
		baseName := astroSpec.Name
		contextPath := filepath.Join(workingDir, astroSpec.Agent.Build.Context)
		dockerfile := astroSpec.Agent.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}

		for _, plat := range platforms {
			platTag := platformImageTag(baseName, buildTag, plat)
			if !quiet {
				fmt.Printf("%s→%s Building %s[agent %s]%s %s%s%s", colorCyan, colorReset, colorDim, plat, colorReset, colorBold, platTag, colorReset)
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, platTag, astroSpec.Agent.Build.Args, astroSpec.Agent.Build.Secrets, envVars, buildNoCache, verbose, quiet, plat); err != nil {
				if !quiet {
					fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				}
				return fmt.Errorf("failed to build agent image for %s: %w", plat, err)
			}

			if !quiet {
				fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
			}
			imagesBuilt++
		}
	} else if astroSpec.Agent.Image != "" && !quiet {
		fmt.Printf("%s→%s Skipping %s[agent]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, colorReset, colorDim, astroSpec.Agent.Image, colorReset)
	}

	// Build custom model containers (those with build config)
	for name, model := range astroSpec.Models {
		if model.Container != nil && model.Container.Build != nil {
			baseName := fmt.Sprintf("%s-model-%s", astroSpec.Name, name)
			contextPath := filepath.Join(workingDir, model.Container.Build.Context)
			dockerfile := model.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			for _, plat := range platforms {
				platTag := platformImageTag(baseName, buildTag, plat)
				if !quiet {
					fmt.Printf("%s→%s Building %s[model: %s %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, plat, colorReset, colorBold, platTag, colorReset)
				}

				if err := buildImageSDK(ctx, cli, contextPath, dockerfile, platTag, model.Container.Build.Args, model.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet, plat); err != nil {
					if !quiet {
						fmt.Printf(" %s✗%s\n", colorRed, colorReset)
					}
					return fmt.Errorf("failed to build model %s for %s: %w", name, plat, err)
				}

				if !quiet {
					fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
				}
				imagesBuilt++
			}
		} else {
			resolved := model.ResolvedContainer()
			if resolved.Image != "" && !quiet {
				fmt.Printf("%s→%s Skipping %s[model: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, resolved.Image, colorReset)
			}
		}
	}

	// Build custom knowledge store containers (those with build config)
	for name, knowledge := range astroSpec.Knowledge {
		container := knowledge.ResolvedContainer()
		if container.Build != nil {
			baseName := fmt.Sprintf("%s-knowledge-%s", astroSpec.Name, name)
			contextPath := filepath.Join(workingDir, container.Build.Context)
			dockerfile := container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			for _, plat := range platforms {
				platTag := platformImageTag(baseName, buildTag, plat)
				if !quiet {
					fmt.Printf("%s→%s Building %s[knowledge: %s %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, plat, colorReset, colorBold, platTag, colorReset)
				}

				if err := buildImageSDK(ctx, cli, contextPath, dockerfile, platTag, container.Build.Args, container.Build.Secrets, envVars, buildNoCache, verbose, quiet, plat); err != nil {
					if !quiet {
						fmt.Printf(" %s✗%s\n", colorRed, colorReset)
					}
					return fmt.Errorf("failed to build knowledge store %s for %s: %w", name, plat, err)
				}

				if !quiet {
					fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
				}
				imagesBuilt++
			}
		} else if container.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[knowledge: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, container.Image, colorReset)
		}
	}

	// Build custom tool containers (those with build config)
	for name, tool := range astroSpec.Tools {
		if tool.Container != nil && tool.Container.Build != nil {
			baseName := fmt.Sprintf("%s-tool-%s", astroSpec.Name, name)
			contextPath := filepath.Join(workingDir, tool.Container.Build.Context)
			dockerfile := tool.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			for _, plat := range platforms {
				platTag := platformImageTag(baseName, buildTag, plat)
				if !quiet {
					fmt.Printf("%s→%s Building %s[tool: %s %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, plat, colorReset, colorBold, platTag, colorReset)
				}

				if err := buildImageSDK(ctx, cli, contextPath, dockerfile, platTag, tool.Container.Build.Args, tool.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet, plat); err != nil {
					if !quiet {
						fmt.Printf(" %s✗%s\n", colorRed, colorReset)
					}
					return fmt.Errorf("failed to build tool %s for %s: %w", name, plat, err)
				}

				if !quiet {
					fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
				}
				imagesBuilt++
			}
		} else if tool.Container != nil && tool.Container.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[tool: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, tool.Container.Image, colorReset)
		}
	}

	// Build custom ingestion containers (those with build config)
	for name, ingestion := range astroSpec.Ingestion {
		if ingestion.Container.Build != nil {
			baseName := fmt.Sprintf("%s-ingestion-%s", astroSpec.Name, name)
			contextPath := filepath.Join(workingDir, ingestion.Container.Build.Context)
			dockerfile := ingestion.Container.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			for _, plat := range platforms {
				platTag := platformImageTag(baseName, buildTag, plat)
				if !quiet {
					fmt.Printf("%s→%s Building %s[ingestion: %s %s]%s %s%s%s", colorCyan, colorReset, colorDim, name, plat, colorReset, colorBold, platTag, colorReset)
				}

				if err := buildImageSDK(ctx, cli, contextPath, dockerfile, platTag, ingestion.Container.Build.Args, ingestion.Container.Build.Secrets, envVars, buildNoCache, verbose, quiet, plat); err != nil {
					if !quiet {
						fmt.Printf(" %s✗%s\n", colorRed, colorReset)
					}
					return fmt.Errorf("failed to build ingestion %s for %s: %w", name, plat, err)
				}

				if !quiet {
					fmt.Printf(" %s✓%s\n", colorGreen, colorReset)
				}
				imagesBuilt++
			}
		} else if ingestion.Container.Image != "" && !quiet {
			fmt.Printf("%s→%s Skipping %s[ingestion: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, ingestion.Container.Image, colorReset)
		}
	}

	if !quiet {
		fmt.Printf("%s✓%s Built %s%d%s image(s) for %s%d%s platform(s)\n", colorGreen, colorReset, colorBold, imagesBuilt, colorReset, colorBold, len(platforms), colorReset)
	}

	return nil
}

// parseDockerfileBaseImages reads a Dockerfile and returns the base images from FROM instructions.
// It resolves build arg references (e.g. FROM ${BASE_IMAGE}) using the provided buildArgs map.
// Images named "scratch" and build stage aliases are excluded.
func parseDockerfileBaseImages(dockerfilePath string, buildArgs map[string]string) ([]string, error) {
	f, err := os.Open(dockerfilePath)
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck

	var images []string
	stages := make(map[string]bool) // track named build stages

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(strings.ToUpper(line), "FROM ") {
			continue
		}

		// Parse: FROM [--platform=...] image [:tag] [AS name]
		fields := strings.Fields(line)[1:] // drop "FROM"
		if len(fields) == 0 {
			continue
		}

		// Skip --platform or other flags
		idx := 0
		for idx < len(fields) && strings.HasPrefix(fields[idx], "--") {
			idx++
		}
		if idx >= len(fields) {
			continue
		}

		img := fields[idx]

		// Resolve build arg references like ${VAR} or $VAR
		img = os.Expand(img, func(key string) string {
			if v, ok := buildArgs[key]; ok {
				return v
			}
			return ""
		})

		// Track "AS name" for multi-stage builds
		if idx+2 < len(fields) && strings.EqualFold(fields[idx+1], "AS") {
			stages[fields[idx+2]] = true
		}

		if img != "" && !strings.EqualFold(img, "scratch") && !stages[img] {
			images = append(images, img)
		}
	}

	return images, scanner.Err()
}

// prePullBaseImages pulls the base images referenced in a Dockerfile so that BuildKit
// can resolve them from the local cache instead of timing out on registry metadata fetches.
func prePullBaseImages(ctx context.Context, cli *client.Client, contextPath, dockerfile string, buildArgs map[string]string, platform string, quiet bool) {
	dockerfilePath := filepath.Join(contextPath, dockerfile)
	images, err := parseDockerfileBaseImages(dockerfilePath, buildArgs)
	if err != nil {
		// Non-fatal: if we can't parse, let the build handle it
		return
	}

	pullOpts := dockerimage.PullOptions{}
	if platform != "" {
		pullOpts.Platform = platform
	}

	seen := make(map[string]bool)
	for _, img := range images {
		if seen[img] {
			continue
		}
		seen[img] = true

		if !quiet {
			fmt.Printf("      %sPre-pulling %s%s\n", colorDim, img, colorReset)
		}

		reader, err := cli.ImagePull(ctx, img, pullOpts)
		if err != nil {
			if !quiet {
				fmt.Printf("      %sPre-pull skipped (%s): %s%s\n", colorDim, img, err, colorReset)
			}
			continue
		}
		// Drain the pull output to completion
		io.Copy(io.Discard, reader) //nolint:errcheck
		reader.Close()              //nolint:errcheck
	}
}

func buildImageSDK(ctx context.Context, cli *client.Client, contextPath, dockerfile, imageName string, buildArgs map[string]string, secrets []spec.BuildSecret, envVars map[string]string, noCache, verbose, quiet bool, platform string) error {
	// Pre-pull base images so BuildKit resolves them from local cache
	prePullBaseImages(ctx, cli, contextPath, dockerfile, buildArgs, platform, quiet)

	// Create build context tar
	buildContext, err := archive.TarWithOptions(contextPath, &archive.TarOptions{})
	if err != nil {
		return fmt.Errorf("failed to create build context: %w", err)
	}
	defer buildContext.Close() //nolint:errcheck

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

	// BuildKit is needed for cross-platform builds or secrets
	needBuildKit := platform != "" || len(secrets) > 0
	if needBuildKit {
		sess, err := session.NewSession(ctx, filepath.Base(contextPath))
		if err != nil {
			return fmt.Errorf("failed to create build session: %w", err)
		}

		// Add secrets provider if secrets are defined
		if len(secrets) > 0 {
			secretMap := make(map[string][]byte)
			for _, s := range secrets {
				if val, ok := envVars[s.Env]; ok {
					secretMap[s.ID] = []byte(val)
				}
			}
			sess.Allow(secretsprovider.FromMap(secretMap))
		}

		dialSession := func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
			return cli.DialHijack(ctx, "/session", proto, meta)
		}

		go sess.Run(ctx, dialSession) //nolint:errcheck
		defer sess.Close()            //nolint:errcheck

		opts.Version = build.BuilderBuildKit
		opts.SessionID = sess.ID()

		if platform != "" {
			opts.Platform = platform
		}
	}

	// Build the image
	resp, err := cli.ImageBuild(ctx, buildContext, opts)
	if err != nil {
		return fmt.Errorf("failed to build image: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	// Stream build output
	if err := streamBuildOutput(resp.Body, verbose, quiet); err != nil {
		return fmt.Errorf("error during build: %w", err)
	}

	return nil
}

func streamBuildOutput(reader io.Reader, _, quiet bool) error {
	if !quiet {
		fmt.Println()
	}

	var lastError string
	decoder := json.NewDecoder(reader)
	seenVertices := make(map[string]bool)

	for {
		var msg struct {
			ID     string `json:"id"`
			Aux    string `json:"aux"`
			Stream string `json:"stream"`
			Error  string `json:"error"`
		}

		if err := decoder.Decode(&msg); err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		// Handle errors
		if msg.Error != "" {
			lastError = msg.Error
			if !quiet {
				fmt.Printf("      %sERROR: %s%s\n", colorRed, msg.Error, colorReset)
			}
		}

		// Handle traditional Docker build stream output
		if msg.Stream != "" && !quiet {
			fmt.Print(msg.Stream)
		}

		// Handle BuildKit trace data
		if msg.ID == "moby.buildkit.trace" && msg.Aux != "" && !quiet {
			// Decode base64
			data, err := base64.StdEncoding.DecodeString(msg.Aux)
			if err != nil {
				continue
			}

			// Parse protobuf
			var status controlapi.StatusResponse
			if err := proto.Unmarshal(data, &status); err != nil {
				continue
			}

			// Print vertex names (build steps)
			for _, v := range status.Vertexes {
				if v.Name != "" && !seenVertices[v.Digest] {
					seenVertices[v.Digest] = true
					fmt.Printf("      %s%s%s\n", colorCyan, v.Name, colorReset)
				}
				if v.Error != "" {
					lastError = v.Error
					fmt.Printf("      %sERROR: %s%s\n", colorRed, v.Error, colorReset)
				}
			}

			// Print logs (command output)
			for _, l := range status.Logs {
				if len(l.Msg) > 0 {
					lines := strings.Split(string(l.Msg), "\n")
					for _, line := range lines {
						if line != "" {
							fmt.Printf("      %s%s%s\n", colorDim, line, colorReset)
						}
					}
				}
			}
		}
	}

	if lastError != "" {
		return fmt.Errorf("build failed: %s", lastError)
	}

	return nil
}

// parsePlatforms splits a comma-separated platform string into a slice.
func parsePlatforms(s string) []string {
	return strings.Split(s, ",")
}

// platformImageTag returns a platform-specific image tag.
// e.g. ("myagent", "latest", "linux/amd64") -> "myagent-linux-amd64:latest"
func platformImageTag(baseName, tag, platform string) string {
	sanitized := strings.ReplaceAll(platform, "/", "-")
	return fmt.Sprintf("%s-%s:%s", baseName, sanitized, tag)
}

// nativePlatform returns the platform string for the host machine.
func nativePlatform() string {
	return fmt.Sprintf("linux/%s", runtime.GOARCH)
}
