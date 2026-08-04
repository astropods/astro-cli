package cmd

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/moby/moby/client"
	"golang.org/x/sync/errgroup"

	bkclient "github.com/moby/buildkit/client"
	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/session/secrets/secretsprovider"
	"github.com/moby/buildkit/util/progress/progressui"
	"github.com/tonistiigi/fsutil"

	spec "github.com/astropods/astro-spec"
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

			if err := buildImageBuildKit(ctx, cli, contextPath, dockerfile, platTag, comp.Build.Args, comp.Build.Secrets, envVars, noCache, verbose, quiet, plat); err != nil {
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

// buildImageBuildKit builds an image via BuildKit's gRPC Solve API against
// the Docker daemon's embedded BuildKit endpoint — the same path docker buildx
// uses with the "docker" driver.
func buildImageBuildKit(ctx context.Context, dockerCli *client.Client, contextPath, dockerfile, imageName string, buildArgs map[string]string, buildSecrets []spec.BuildSecret, envVars map[string]string, noCache, verbose, quiet bool, platform string) error {
	bkc, err := bkclient.New(ctx, "", bkclient.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
		return dockerCli.DialHijack(ctx, "/grpc", "h2c", nil)
	}))
	if err != nil {
		return fmt.Errorf("connect buildkit: %w", err)
	}
	defer bkc.Close() //nolint:errcheck

	contextFS, err := fsutil.NewFS(contextPath)
	if err != nil {
		return fmt.Errorf("init context fs: %w", err)
	}

	attachables := []session.Attachable{}
	if len(buildSecrets) > 0 {
		secretMap := make(map[string][]byte)
		for _, s := range buildSecrets {
			if val, ok := envVars[s.Env]; ok {
				secretMap[s.ID] = []byte(val)
			}
		}
		attachables = append(attachables, secretsprovider.FromMap(secretMap))
	}

	frontendAttrs := map[string]string{
		"filename": dockerfile,
	}
	if platform != "" {
		frontendAttrs["platform"] = platform
	}
	for k, v := range buildArgs {
		frontendAttrs["build-arg:"+k] = v
	}
	if noCache {
		frontendAttrs["no-cache"] = ""
	}

	opts := bkclient.SolveOpt{
		Frontend:      "dockerfile.v0",
		FrontendAttrs: frontendAttrs,
		LocalMounts: map[string]fsutil.FS{
			"context":    contextFS,
			"dockerfile": contextFS,
		},
		Session: attachables,
		Exports: []bkclient.ExportEntry{
			{
				// "moby" is an undocumented buildkit exporter that writes to
				// the daemon's moby image store, making the image visible to
				// the classic Docker API (docker.Client.ImageTag/ImagePush).
				// "image" writes to the buildkit-only containerd namespace
				// and is invisible to the classic API. Same trick buildx uses
				// in driver/docker/IsMobyDriver branches (build/opt.go:466).
				Type:  "moby",
				Attrs: map[string]string{"name": imageName},
			},
		},
	}

	ch := make(chan *bkclient.SolveStatus, 16)

	mode := progressui.AutoMode
	if quiet {
		mode = progressui.QuietMode
	} else if verbose {
		mode = progressui.PlainMode
	}
	display, err := progressui.NewDisplay(os.Stderr, mode)
	if err != nil {
		return fmt.Errorf("init progress display: %w", err)
	}

	eg, gctx := errgroup.WithContext(ctx)
	eg.Go(func() error {
		_, err := bkc.Solve(gctx, nil, opts, ch)
		return err
	})
	eg.Go(func() error {
		_, err := display.UpdateFrom(gctx, ch)
		return err
	})
	if err := eg.Wait(); err != nil {
		return fmt.Errorf("buildkit solve: %w", err)
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
// registry push based on the target server URL and the agent's runtime. A local
// server (localhost / 127.0.0.1 / ::1) builds for the native platform and retags
// images locally instead of pushing to a remote registry; a remote server builds
// for linux/amd64 and pushes normally. AgentCore agents always build for
// linux/arm64 because AWS Bedrock AgentCore Runtime only runs arm64 containers;
// skipPush still follows the server (a local registry retags locally).
func resolveBuildPlatform(serverURL string, agentCore bool) (platform string, skipPush bool) {
	localhost := false
	if u, err := url.Parse(serverURL); err == nil {
		switch u.Hostname() {
		case "localhost", "127.0.0.1", "::1":
			localhost = true
		}
	}
	if agentCore {
		return "linux/arm64", localhost
	}
	if localhost {
		return nativePlatform(), true
	}
	return "linux/amd64", false
}

