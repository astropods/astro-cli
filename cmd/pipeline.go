package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/moby/moby/client"
	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-cli/internal/buildinfo"
	"github.com/astropods/astro/apps/astro-cli/internal/theme"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// PushPipelineConfig holds all parameters for a push pipeline.
type PushPipelineConfig struct {
	SpecPath     string
	AgentName    string
	Platform     string
	SkipBuild    bool
	SkipPush     bool
	RegistryHost string
	Account      string
	Token        string
	Verbose      bool
	Yes          bool
	Visibility   Visibility
}

// PushPipeline orchestrates the build-push-register flow as a chainable sequence.
// Each public method is a pipeline step; the chain short-circuits on the first error.
//
// Usage:
//
//	err := NewPushPipeline(ctx, cfg).
//		ParseSpec().
//		CollectComponents().
//		Build().
//		Push().
//		TransformSpec().
//		StripSecrets().
//		Register().
//		Err()
type PushPipeline struct {
	ctx context.Context
	cfg PushPipelineConfig

	// State accumulated across steps
	astroSpec  *spec.AstroSpec
	specMap    map[string]any
	components []spec.Component
	tag        string
	readme     string
	visibility Visibility

	err error
}

// NewPushPipeline creates a pipeline ready for chaining.
func NewPushPipeline(ctx context.Context, cfg PushPipelineConfig) *PushPipeline {
	return &PushPipeline{
		ctx: ctx,
		cfg: cfg,
		tag: generateBuildID(),
	}
}

// Err returns the first error that occurred during the pipeline, or nil.
func (p *PushPipeline) Err() error {
	return p.err
}

// Tag returns the generated build ID.
func (p *PushPipeline) Tag() string {
	return p.tag
}

// step runs fn if no previous error has occurred; captures the error otherwise.
func (p *PushPipeline) step(fn func() error) *PushPipeline {
	if p.err != nil {
		return p
	}
	p.err = fn()
	return p
}

// ParseSpec loads, validates, and unmarshals the spec file.
func (p *PushPipeline) ParseSpec() *PushPipeline {
	return p.step(func() error {
		astroSpec, err := spec.ParseSpec(p.cfg.SpecPath)
		if err != nil {
			return fmt.Errorf("failed to parse spec: %w", err)
		}
		p.astroSpec = astroSpec

		workingDir := filepath.Dir(p.cfg.SpecPath)
		warnDeprecatedMetaFields(p.cfg.SpecPath, workingDir)

		specData, err := os.ReadFile(p.cfg.SpecPath) //nolint:gosec
		if err != nil {
			return fmt.Errorf("failed to read spec file: %w", err)
		}
		var specMap map[string]any
		if err := yaml.Unmarshal(specData, &specMap); err != nil {
			return fmt.Errorf("failed to parse spec YAML: %w", err)
		}
		p.specMap = specMap

		return nil
	})
}

// CollectComponents enumerates buildable components from the spec.
func (p *PushPipeline) CollectComponents() *PushPipeline {
	return p.step(func() error {
		p.components = spec.CollectComponents(p.astroSpec, p.cfg.AgentName)
		return nil
	})
}

// Build runs Docker builds for each component. No-ops if SkipBuild is set.
func (p *PushPipeline) Build() *PushPipeline {
	return p.step(func() error {
		if p.cfg.SkipBuild {
			return nil
		}

		printStep("Building images")
		fmt.Println()

		workingDir := filepath.Dir(p.cfg.SpecPath)
		cli, err := newDockerClient()
		if err != nil {
			return err
		}

		envVars := make(map[string]string)
		imagesBuilt := 0

		for _, comp := range p.components {
			contextPath := filepath.Join(workingDir, comp.Build.Context)
			dockerfile := comp.Build.Dockerfile
			if dockerfile == "" {
				dockerfile = "Dockerfile"
			}

			platTag := platformImageTag(comp.ImageName, p.tag, p.cfg.Platform)
			fmt.Printf("%s→%s Building %s[%s %s]%s %s%s%s",
				colorCyan, colorReset, colorDim, comp.Kind, p.cfg.Platform, colorReset, colorBold, platTag, colorReset)

			if err := buildImageSDK(p.ctx, cli, contextPath, dockerfile, platTag,
				comp.Build.Args, comp.Build.Secrets, envVars,
				false, p.cfg.Verbose, false, p.cfg.Platform); err != nil {
				fmt.Printf(" %s✗%s\n", colorRed, colorReset)
				return fmt.Errorf("failed to build %s for %s: %w", comp.Suffix(), p.cfg.Platform, err)
			}
			imagesBuilt++
		}

		// Print skip messages for image-only components
		p.printSkippedComponents()

		fmt.Printf("%s✓%s Built %s%d%s image(s)\n", colorGreen, colorReset, colorBold, imagesBuilt, colorReset)
		return nil
	})
}

// Push pushes images to the registry, or retags locally for local dev servers.
func (p *PushPipeline) Push() *PushPipeline {
	return p.step(func() error {
		if p.cfg.SkipPush {
			return p.retagLocal()
		}
		return p.pushToRegistry()
	})
}

func (p *PushPipeline) pushToRegistry() error {
	for _, comp := range p.components {
		localImageName := platformImageTag(comp.ImageName, p.tag, p.cfg.Platform)
		remoteImageName := fmt.Sprintf("%s/%s/%s:%s", p.cfg.RegistryHost, p.cfg.Account, comp.ImageName, p.tag)

		displayName := comp.Name
		if comp.Kind == spec.ComponentAgent {
			displayName = p.cfg.AgentName
		}
		printPushStart(string(comp.Kind), displayName)

		size, err := pushImageToRegistryStreaming(localImageName, remoteImageName, false, p.cfg.Token)
		if err != nil {
			printPushComplete(false, 0)
			return fmt.Errorf("failed to push %s: %w", comp.Suffix(), err)
		}
		printPushComplete(true, size)
	}
	return nil
}

func (p *PushPipeline) retagLocal() error {
	if p.cfg.SkipBuild {
		// Nothing to retag if we didn't build
		fmt.Printf("%s→%s Skipping image push %s(local dev server detected)%s\n", colorCyan, colorReset, colorDim, colorReset)
		return nil
	}

	fmt.Printf("%s→%s Skipping image push %s(local dev server detected)%s\n", colorCyan, colorReset, colorDim, colorReset)

	dockerCli, err := newDockerClient()
	if err != nil {
		return err
	}

	for _, comp := range p.components {
		local := platformImageTag(comp.ImageName, p.tag, p.cfg.Platform)
		remote := fmt.Sprintf("%s/%s/%s:%s", p.cfg.RegistryHost, p.cfg.Account, comp.ImageName, p.tag)
		if _, err := dockerCli.ImageTag(p.ctx, client.ImageTagOptions{Source: local, Target: remote}); err != nil {
			return fmt.Errorf("failed to retag %s → %s: %w", local, remote, err)
		}
		fmt.Printf("  %s✓%s %s%s%s\n", colorGreen, colorReset, colorDim, remote, colorReset)
	}
	return nil
}

// TransformSpec replaces build blocks with image references in the spec map.
func (p *PushPipeline) TransformSpec() *PushPipeline {
	return p.step(func() error {
		registry := fmt.Sprintf("%s/%s", p.cfg.RegistryHost, p.cfg.Account)
		spec.TransformSpecForRegistry(p.specMap, p.cfg.AgentName, func(imageName string) string {
			return fmt.Sprintf("%s/%s:%s", registry, imageName, p.tag)
		})
		return nil
	})
}

// StripSecrets removes default values from secret inputs.
func (p *PushPipeline) StripSecrets() *PushPipeline {
	return p.step(func() error {
		spec.StripSecretDefaults(p.specMap)
		return nil
	})
}

// LoadReadme reads AGENT.md from the spec's working directory.
func (p *PushPipeline) LoadReadme() *PushPipeline {
	return p.step(func() error {
		workingDir := filepath.Dir(p.cfg.SpecPath)
		readmePath := filepath.Join(workingDir, "AGENT.md")
		if data, err := os.ReadFile(readmePath); err == nil { //nolint:gosec
			p.readme = string(data)
		}
		return nil
	})
}

// ResolveVisibility queries the server for the current agent state and resolves
// the target visibility, prompting for confirmation if needed.
func (p *PushPipeline) ResolveVisibility() *PushPipeline {
	return p.step(func() error {
		p.visibility = VisibilityPrivate
		if p.cfg.Visibility != VisibilityUnset {
			p.visibility = p.cfg.Visibility
		}

		serverAgent := getAgentFromServer(pushBaseURL(), p.cfg.Account, p.cfg.AgentName, false, p.cfg.Token)

		if serverAgent.Exists && serverAgent.Visibility == string(VisibilityPublic) && p.cfg.Visibility != VisibilityPrivate {
			p.visibility = VisibilityPublic
		}

		needsConfirm := (p.visibility == VisibilityPublic && (!serverAgent.Exists || serverAgent.Visibility != string(VisibilityPublic))) ||
			(p.cfg.Visibility == VisibilityPrivate && serverAgent.Exists && serverAgent.Visibility == string(VisibilityPublic))
		if needsConfirm && !p.cfg.Yes {
			if !confirmVisibilityChange(serverAgent.Visibility, string(p.visibility)) {
				return fmt.Errorf("push cancelled")
			}
		}

		return nil
	})
}

// Register registers the agent spec with the server.
func (p *PushPipeline) Register() *PushPipeline {
	return p.step(func() error {
		registryPath := fmt.Sprintf("%s/%s", p.cfg.RegistryHost, p.cfg.Account)

		transformedSpecData, err := yaml.Marshal(p.specMap)
		if err != nil {
			return fmt.Errorf("failed to marshal transformed spec: %w", err)
		}

		printStep("Registering agent with server...")
		if err := registerAgentWithServer(pushBaseURL(), p.cfg.AgentName, p.tag, registryPath,
			string(transformedSpecData), p.readme, string(p.visibility), p.cfg.Verbose, false, p.cfg.Token); err != nil {
			printStepFail()
			return fmt.Errorf("registration failed: %w", err)
		}
		printStepDone("")

		return nil
	})
}

// printSkippedComponents prints skip messages for components that use pre-built images.
func (p *PushPipeline) printSkippedComponents() {
	if p.astroSpec.Agent.Build == nil && p.astroSpec.Agent.Image != "" {
		fmt.Printf("%s→%s Skipping %s[agent]%s using image: %s%s%s\n",
			colorCyan, colorReset, colorDim, colorReset, colorDim, p.astroSpec.Agent.Image, colorReset)
	}
	for name, model := range p.astroSpec.Models {
		resolved := model.ResolvedContainer()
		if model.Container == nil || model.Container.Build == nil {
			if resolved.Image != "" {
				fmt.Printf("%s→%s Skipping %s[model: %s]%s using image: %s%s%s\n",
					colorCyan, colorReset, colorDim, name, colorReset, colorDim, resolved.Image, colorReset)
			}
		}
	}
	for name, knowledge := range p.astroSpec.Knowledge {
		container := knowledge.ResolvedContainer()
		if container.Build == nil && container.Image != "" {
			fmt.Printf("%s→%s Skipping %s[knowledge: %s]%s using image: %s%s%s\n",
				colorCyan, colorReset, colorDim, name, colorReset, colorDim, container.Image, colorReset)
		}
	}
	for name, tool := range p.astroSpec.Integrations {
		if tool.Container == nil || tool.Container.Build == nil {
			if tool.Container != nil && tool.Container.Image != "" {
				fmt.Printf("%s→%s Skipping %s[integration: %s]%s using image: %s%s%s\n",
					colorCyan, colorReset, colorDim, name, colorReset, colorDim, tool.Container.Image, colorReset)
			}
		}
	}
	for name, ingestion := range p.astroSpec.Ingestion {
		if ingestion.Container.Build == nil && ingestion.Container.Image != "" {
			fmt.Printf("%s→%s Skipping %s[ingestion: %s]%s using image: %s%s%s\n",
				colorCyan, colorReset, colorDim, name, colorReset, colorDim, ingestion.Container.Image, colorReset)
		}
	}
}

// PrintSuccess prints the success box after a completed push.
func (p *PushPipeline) PrintSuccess() {
	agentURL := fmt.Sprintf("%s/%s/%s",
		strings.TrimSuffix(buildinfo.DefaultServerURL, "/"), p.cfg.Account, p.cfg.AgentName)

	bold := lipgloss.NewStyle().Bold(true)
	dim := lipgloss.NewStyle().Faint(true)
	link := lipgloss.NewStyle().Foreground(theme.Primary).Underline(true)

	var lines []string
	lines = append(lines, bold.Render("✓ Pushed successfully!"))
	lines = append(lines, dim.Render("Blueprint is "+string(p.visibility)))
	lines = append(lines, "")
	lines = append(lines, "  "+bold.Render(p.cfg.AgentName)+"  "+dim.Render("tag "+p.tag))
	lines = append(lines, "  "+dim.Render("View online → ")+link.Render(agentURL))

	box := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(theme.Primary).
		Padding(0, 2).
		Render(strings.Join(lines, "\n"))

	fmt.Println()
	fmt.Println(box)
	fmt.Println()
}
