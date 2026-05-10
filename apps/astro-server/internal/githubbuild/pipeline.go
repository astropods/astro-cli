package githubbuild

import (
	"context"
	"errors"
	"fmt"

	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	spec "github.com/astropods/astro/packages/astro-spec"
)

// GitHubBuildConfig holds all parameters for a GitHub build pipeline.
type GitHubBuildConfig struct {
	Token       string // GitHub access token
	RepoName    string // e.g. "owner/repo" or "owner/repo/sub/path"
	CommitSHA   string
	AgentName   string
	BuildID     string
	AccountID   string
	ProxyHost   string // proxy registry host for image references
	RegistryURL string
	Local       bool // local dev mode (no push)

	// Dependencies
	Builder    *Builder
	GHStore    *githubconnection.Store
	AgentIndex *agentindex.Index
	RecordID   string // build record ID for status tracking
	Log        *logger.Logger
}

// GitHubBuildPipeline orchestrates the GitHub build flow as a chainable sequence.
// Each public method is a pipeline step; the chain short-circuits on the first error.
//
// Usage:
//
//	err := NewGitHubBuildPipeline(ctx, cfg).
//		FetchSpec().
//		CollectComponents().
//		CreateComponentRecords().
//		RunBuildJobs().
//		FetchReadme().
//		TransformSpec().
//		StripSecrets().
//		Register().
//		Err()
type GitHubBuildPipeline struct {
	ctx context.Context
	cfg GitHubBuildConfig

	// State accumulated across steps
	astroSpec    *spec.AstroSpec
	specYAML     string
	specMap      map[string]any
	components   []spec.Component
	componentIDs map[string]int64
	readme       string

	err error
}

// NewGitHubBuildPipeline creates a pipeline ready for chaining.
func NewGitHubBuildPipeline(ctx context.Context, cfg GitHubBuildConfig) *GitHubBuildPipeline {
	return &GitHubBuildPipeline{
		ctx: ctx,
		cfg: cfg,
	}
}

// Err returns the first error that occurred during the pipeline, or nil.
func (p *GitHubBuildPipeline) Err() error {
	return p.err
}

// step runs fn if no previous error has occurred. It updates the DB step name
// before executing so the UI can track progress.
func (p *GitHubBuildPipeline) step(name string, fn func() error) *GitHubBuildPipeline {
	if p.err != nil {
		return p
	}
	p.updateStep(name)
	p.err = fn()
	return p
}

func (p *GitHubBuildPipeline) updateStep(name string) {
	if p.cfg.GHStore != nil && p.cfg.RecordID != "" {
		if err := p.cfg.GHStore.UpdateBuildStep(context.Background(), p.cfg.RecordID, name); err != nil && p.cfg.Log != nil {
			p.cfg.Log.Error("failed to update build step", "step", name, "error", err)
		}
	}
}

// FetchSpec downloads astropods.yml from GitHub and parses it.
func (p *GitHubBuildPipeline) FetchSpec() *GitHubBuildPipeline {
	return p.step("fetching-spec", func() error {
		astroSpec, specYAML, err := FetchAstroSpec(p.ctx, p.cfg.Token, p.cfg.RepoName, p.cfg.CommitSHA)
		if err != nil {
			return fmt.Errorf("fetch astropods.yml: %w", err)
		}
		if specYAML == "" {
			return PermanentError{Err: fmt.Errorf("astropods.yml not found in repo at commit %s", p.cfg.CommitSHA[:min(7, len(p.cfg.CommitSHA))])}
		}

		p.astroSpec = astroSpec
		p.specYAML = specYAML

		var specMap map[string]any
		if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
			return PermanentError{Err: fmt.Errorf("parse spec YAML: %w", err)}
		}
		p.specMap = specMap

		return nil
	})
}

// CollectComponents enumerates buildable components from the spec.
func (p *GitHubBuildPipeline) CollectComponents() *GitHubBuildPipeline {
	return p.step("collecting-components", func() error {
		p.components = spec.CollectComponents(p.astroSpec, p.cfg.AgentName)
		return nil
	})
}

// CreateComponentRecords creates DB records for each component so the UI shows them.
func (p *GitHubBuildPipeline) CreateComponentRecords() *GitHubBuildPipeline {
	return p.step("creating-records", func() error {
		p.componentIDs = make(map[string]int64, len(p.components))
		for _, comp := range p.components {
			jobName := fmt.Sprintf("build-%s-%s", p.cfg.BuildID, comp.Suffix())
			id, err := p.cfg.GHStore.CreateBuildComponent(context.Background(), p.cfg.RecordID, comp.Suffix(), jobName)
			if err != nil {
				if p.cfg.Log != nil {
					p.cfg.Log.Error("failed to create build component record", "component", comp.Suffix(), "error", err)
				}
			} else {
				p.componentIDs[comp.Suffix()] = id
			}
		}
		return nil
	})
}

// RunBuildJobs runs BuildKit K8s jobs for each component.
func (p *GitHubBuildPipeline) RunBuildJobs() *GitHubBuildPipeline {
	return p.step("building", func() error {
		for i, comp := range p.components {
			suffix := comp.Suffix()
			compID := p.componentIDs[suffix]

			p.updateStep(fmt.Sprintf("building (%d/%d: %s)", i+1, len(p.components), suffix))

			if compID > 0 {
				_ = p.cfg.GHStore.UpdateBuildComponentStatus(context.Background(), compID, "building")
			}

			jobName := fmt.Sprintf("build-%s-%s", p.cfg.BuildID, suffix)
			var destination string
			if !p.cfg.Local {
				destination = p.cfg.Builder.ECRImagePath(p.cfg.AccountID, comp.ImageName, p.cfg.BuildID)
			}

			if p.cfg.Log != nil {
				p.cfg.Log.Info("Building component",
					"component", suffix,
					"destination", destination,
					"progress", fmt.Sprintf("%d/%d", i+1, len(p.components)))
			}

			if destination != "" {
				if err := p.cfg.Builder.EnsureRepository(p.ctx, destination); err != nil {
					if p.cfg.Log != nil {
						p.cfg.Log.Error("failed to ensure ECR repository", "error", err)
					}
					if compID > 0 {
						_ = p.cfg.GHStore.UpdateBuildComponentStatus(context.Background(), compID, "failed")
					}
					_ = p.cfg.GHStore.FailPendingBuildComponents(context.Background(), p.cfg.RecordID)
					return fmt.Errorf("ensure ECR repo: %w", err)
				}
			}

			buildLogs, err := p.cfg.Builder.RunJob(p.ctx, jobName, p.cfg.Token, p.cfg.RepoName, p.cfg.CommitSHA, *comp.Build, destination)

			// Persist logs regardless of outcome (truncate to 512KB).
			if compID > 0 && buildLogs != "" {
				if len(buildLogs) > 512*1024 {
					buildLogs = buildLogs[:512*1024]
				}
				_ = p.cfg.GHStore.SaveBuildComponentLogs(context.Background(), compID, buildLogs)
			}

			if err != nil {
				if compID > 0 {
					_ = p.cfg.GHStore.UpdateBuildComponentStatus(context.Background(), compID, "failed")
				}
				_ = p.cfg.GHStore.FailPendingBuildComponents(context.Background(), p.cfg.RecordID)

				wrapped := fmt.Errorf("build %s: %w", suffix, err)
				var bfe BuildFailedError
				if errors.As(err, &bfe) {
					return PermanentError{Err: wrapped}
				}
				return wrapped
			}

			if compID > 0 {
				_ = p.cfg.GHStore.UpdateBuildComponentStatus(context.Background(), compID, "succeeded")
			}
		}
		return nil
	})
}

// FetchReadme downloads AGENT.md from GitHub.
func (p *GitHubBuildPipeline) FetchReadme() *GitHubBuildPipeline {
	return p.step("fetching-readme", func() error {
		readme, _ := FetchFileContent(p.ctx, p.cfg.Token, p.cfg.RepoName, p.cfg.CommitSHA, "AGENT.md")
		p.readme = readme
		return nil
	})
}

// TransformSpec replaces build blocks with image references in the spec map.
func (p *GitHubBuildPipeline) TransformSpec() *GitHubBuildPipeline {
	return p.step("transforming-spec", func() error {
		if p.cfg.ProxyHost == "" {
			return nil
		}
		spec.TransformSpecForRegistry(p.specMap, p.cfg.AgentName, func(imageName string) string {
			return fmt.Sprintf("%s/%s/%s:%s", p.cfg.ProxyHost, p.cfg.AccountID, imageName, p.cfg.BuildID)
		})
		return nil
	})
}

// StripSecrets removes default values from secret inputs.
func (p *GitHubBuildPipeline) StripSecrets() *GitHubBuildPipeline {
	return p.step("stripping-secrets", func() error {
		spec.StripSecretDefaults(p.specMap)
		return nil
	})
}

// Register registers the agent spec in the agent index.
func (p *GitHubBuildPipeline) Register() *GitHubBuildPipeline {
	return p.step("registering", func() error {
		if err := p.cfg.AgentIndex.Register(
			p.cfg.AccountID, p.cfg.AgentName, p.cfg.BuildID,
			p.cfg.RegistryURL, p.cfg.AccountID,
			p.specMap, p.readme, BuildAgentCardJSON(p.readme, p.specMap), "[]",
		); err != nil {
			return fmt.Errorf("register agent: %w", err)
		}
		return nil
	})
}
