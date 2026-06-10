package cmd

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"bufio"

	"github.com/containerd/platforms"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"net/url"

	"github.com/moby/moby/api/types/build"
	"github.com/moby/moby/client"

	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/go-archive"
	"github.com/moby/patternmatcher/ignorefile"

	spec "github.com/astropods/astro/packages/astro-spec"
)

// runBuild assumes the spec at specPath is valid; callers must validate before invoking.
func runBuild(ctx context.Context, specPath, agentName, tag string, platforms []string, noCache, verbose, quiet bool) error {
	workingDir := filepath.Dir(specPath)

	astroSpec, err := spec.ParseSpec(specPath)
	if err != nil {
		return fmt.Errorf("failed to parse spec: %w", err)
	}

	if !quiet {
		fmt.Printf("%s→%s Agent: %s%s%s\n", colorCyan, colorReset, colorBold, agentName, colorReset)
	}

	// TODO: eliminate this or allow env file to be a parameter
	envVars := make(map[string]string)
	//envVars, err := utils.LoadEnvFile(workingDir, utils.DefaultEnvFile)
	//if err != nil {
	//	return fmt.Errorf("failed to read .env file: %w", err)
	//}
	//if envVars == nil {
	//	envVars = make(map[string]string)
	//}

	cli, err := newDockerClient()
	if err != nil {
		return err
	}

	// Validate that agent has either a build or image
	if astroSpec.Agent.Build == nil && astroSpec.Agent.Image == "" {
		return fmt.Errorf("agent.build or agent.image must be specified in spec")
	}

	components := spec.CollectComponents(astroSpec, agentName)
	imagesBuilt := 0

	for _, comp := range components {
		contextPath := filepath.Join(workingDir, comp.Build.Context)
		dockerfile := comp.Build.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}

		for _, plat := range platforms {
			platTag := platformImageTag(comp.ImageName, tag, plat)
			if !quiet {
				fmt.Printf("%s→%s Building %s[%s %s]%s %s%s%s", colorCyan, colorReset, colorDim, comp.Kind, plat, colorReset, colorBold, platTag, colorReset)
			}

			if err := buildImageSDK(ctx, cli, contextPath, dockerfile, platTag, comp.Build.Args, comp.Build.Secrets, envVars, noCache, verbose, quiet, plat); err != nil {
				if !quiet {
					fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				}
				return fmt.Errorf("failed to build %s for %s: %w", comp.Suffix(), plat, err)
			}

			imagesBuilt++
		}
	}

	// Print skip messages for image-only components
	if !quiet {
		if astroSpec.Agent.Build == nil && astroSpec.Agent.Image != "" {
			fmt.Printf("%s→%s Skipping %s[agent]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, colorReset, colorDim, astroSpec.Agent.Image, colorReset)
		}
		for name, model := range astroSpec.Models {
			if model.Container == nil || model.Container.Build == nil {
				resolved := model.ResolvedContainer()
				if resolved.Image != "" {
					fmt.Printf("%s→%s Skipping %s[model: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, resolved.Image, colorReset)
				}
			}
		}
		for name, knowledge := range astroSpec.Knowledge {
			container := knowledge.ResolvedContainer()
			if container.Build == nil && container.Image != "" {
				fmt.Printf("%s→%s Skipping %s[knowledge: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, container.Image, colorReset)
			}
		}
		for name, tool := range astroSpec.Integrations {
			if (tool.Container == nil || tool.Container.Build == nil) && tool.Container != nil && tool.Container.Image != "" {
				fmt.Printf("%s→%s Skipping %s[integration: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, tool.Container.Image, colorReset)
			}
		}
		for name, ingestion := range astroSpec.Ingestion {
			if ingestion.Container.Build == nil && ingestion.Container.Image != "" {
				fmt.Printf("%s→%s Skipping %s[ingestion: %s]%s using image: %s%s%s\n", colorCyan, colorReset, colorDim, name, colorReset, colorDim, ingestion.Container.Image, colorReset)
			}
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
	f, err := os.Open(filepath.Clean(dockerfilePath))
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

	pullOpts := client.ImagePullOptions{}
	if platform != "" {
		p, err := platforms.Parse(platform)
		if err == nil {
			pullOpts.Platforms = []ocispec.Platform{p}
		}
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
		_, _ = io.Copy(io.Discard, reader) //nolint:errcheck
		_ = reader.Close()                 //nolint:errcheck
	}
}

func readDockerignore(contextPath string) ([]string, error) {
	f, err := os.Open(filepath.Clean(filepath.Join(contextPath, ".dockerignore")))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close() //nolint:errcheck
	return ignorefile.ReadAll(f)
}

func buildImageSDK(ctx context.Context, cli *client.Client, contextPath, dockerfile, imageName string, buildArgs map[string]string, secrets []spec.BuildSecret, envVars map[string]string, noCache, verbose, quiet bool, platform string) error {
	// Pre-pull base images so BuildKit resolves them from local cache
	prePullBaseImages(ctx, cli, contextPath, dockerfile, buildArgs, platform, quiet)

	excludes, err := readDockerignore(contextPath)
	if err != nil {
		return fmt.Errorf("failed to read .dockerignore: %w", err)
	}
	// Create build context tar
	buildContext, err := archive.TarWithOptions(contextPath, &archive.TarOptions{
		ExcludePatterns: excludes,
	})
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
	opts := client.ImageBuildOptions{
		Dockerfile: dockerfile,
		Tags:       []string{imageName},
		Remove:     true,
		NoCache:    noCache,
		BuildArgs:  buildArgsPtr,
	}

	// Platforms is honored by both classic and BuildKit code paths on the
	// daemon side, so set it whenever the caller specified one — independent
	// of whether we end up opting into BuildKit below.
	if platform != "" {
		p, err := platforms.Parse(platform)
		if err != nil {
			return fmt.Errorf("invalid platform %q: %w", platform, err)
		}
		opts.Platforms = []ocispec.Platform{p}
	}

	// Only opt into BuildKit when we need a feature only it supports — today
	// that's build secrets. Multi-arch alone does NOT require BuildKit; the
	// classic builder honors opts.Platforms.
	//
	// We avoid this code path unless secrets force us into it because it has
	// known reliability issues on Engine 29.x: when the engine's /build shim
	// is asked to handle a BuildKit build with a session attached, the
	// daemon-side session manager runs a 5s-interval health check
	// (buildkit/session/grpc.go:71-133, monitorHealth) that closes the
	// session on the second consecutive failure (~10s). When that fires
	// mid-build, the build surfaces non-deterministically as one of:
	// "archive/tar: invalid tar header", "unexpected EOF", or an indefinite
	// hang at "load remote build context". We were not able to make those
	// health checks succeed from the client side — registering filesync,
	// synchronizing the session dial with the build POST, and bumping
	// docker/moby module versions all failed to prevent the +10s timeout.
	// See tools/bkprobe for a standalone reproduction harness that uses
	// bkclient.Solve against a docker-container driver builder; that path
	// works, but requires a separately-provisioned buildkitd container and
	// is not currently used by the CLI.
	if len(secrets) > 0 {
		opts.Version = build.BuilderBuildKit

		sess, err := session.NewSession(ctx, filepath.Base(contextPath))
		if err != nil {
			return fmt.Errorf("failed to create build session: %w", err)
		}
		defer sess.Close() //nolint:errcheck

		secretMap := make(map[string][]byte)
		for _, s := range secrets {
			if val, ok := envVars[s.Env]; ok {
				secretMap[s.ID] = []byte(val)
			}
		}
		sess.Allow(secretsprovider.FromMap(secretMap))

		// Defense-in-depth: enforce that /session has been dialed before we
		// POST /build, so the daemon sees the session registered under
		// sess.ID() by the time it processes the build request. The race
		// this protects against is plausible from the code shape (sess.Run
		// runs in a goroutine; cli.ImageBuild runs on the main thread) but
		// we never empirically observed it firing — the failures we saw on
		// this path were caused by the daemon-side monitorHealth timeout
		// described above, which this barrier does NOT address. Keeping the
		// ordering guarantee because it's cheap and correct, not because
		// it was the load-bearing fix.
		sessionReady := make(chan struct{})
		var sessionReadyOnce sync.Once
		dialSession := func(ctx context.Context, proto string, meta map[string][]string) (net.Conn, error) {
			conn, err := cli.DialHijack(ctx, "/session", proto, meta)
			if err == nil {
				sessionReadyOnce.Do(func() { close(sessionReady) })
			}
			return conn, err
		}

		sessionRunErr := make(chan error, 1)
		go func() { sessionRunErr <- sess.Run(ctx, dialSession) }()

		select {
		case <-sessionReady:
		case err := <-sessionRunErr:
			return fmt.Errorf("build session failed before connect: %w", err)
		case <-time.After(10 * time.Second):
			return fmt.Errorf("build session did not connect within 10s")
		case <-ctx.Done():
			return ctx.Err()
		}

		opts.SessionID = sess.ID()
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

// resolveBuildPlatform returns the build platform and whether to skip the
// registry push based on the target server URL. A local server (localhost /
// 127.0.0.1 / ::1) builds for the native platform and retags images locally
// instead of pushing to a remote registry. A remote server builds for
// linux/amd64 and pushes normally.
func resolveBuildPlatform(serverURL string) (platform string, skipPush bool) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "linux/amd64", false
	}
	switch u.Hostname() {
	case "localhost", "127.0.0.1", "::1":
		return nativePlatform(), true
	}
	return "linux/amd64", false
}
