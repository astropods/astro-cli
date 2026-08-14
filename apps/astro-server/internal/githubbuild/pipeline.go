package githubbuild

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astropods/astro/apps/astro-server/internal/agentindex"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/githubconnection"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/readmeassets"
	spec "github.com/astropods/astro-spec"
)

// GitHubBuildConfig holds all parameters for a GitHub build pipeline.
type GitHubBuildConfig struct {
	Token       string // GitHub access token
	RepoName    string // e.g. "owner/repo" or "owner/repo/sub/path"
	CommitSHA   string
	AgentName   string
	AccountName string // account handle; used for readme-asset storage keys
	BuildID     string
	AccountID   string
	ProxyHost   string // proxy registry host for image references
	RegistryURL string
	Local       bool // local dev mode (no push)

	// Dependencies
	Builder      *Builder
	GHStore      *githubconnection.Store
	AgentIndex   *agentindex.Index
	ReadmeAssets *readmeassets.Store // optional; AGENT.md image vacuum skipped when nil
	RecordID     string              // build record ID for status tracking
	Log          *logger.Logger

	// AIGatewayEnabled toggles the validator's astro-gateway provider gate.
	// Pushed from cfg.Deployment.AIGatewayURL != "" at the worker wiring site.
	AIGatewayEnabled bool
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

// ValidateSpec rejects specs with structural errors (missing image/build, bad
// provider, invalid trigger type). Deploy-time values like credentials and
// schedule expressions are not known at build time and are skipped.
func (p *GitHubBuildPipeline) ValidateSpec() *GitHubBuildPipeline {
	return p.step("validating-spec", func() error {
		result := deployment.NewValidatorWithOptions(deployment.ValidatorOptions{
			AIGatewayEnabled: p.cfg.AIGatewayEnabled,
		}).ValidateSpec(p.astroSpec, nil, nil, nil)
		var msgs []string
		for _, e := range result.Errors {
			if strings.HasPrefix(e.Field, "variables.") || strings.HasSuffix(e.Field, ".trigger.schedule") {
				continue
			}
			if e.Field == "" {
				msgs = append(msgs, e.Message)
				continue
			}
			msgs = append(msgs, e.Field+": "+e.Message)
		}
		if len(msgs) == 0 {
			return nil
		}
		return PermanentError{Err: SpecError{
			Reason: specInvalidReason(msgs),
			Err:    fmt.Errorf("spec validation failed: %s", strings.Join(msgs, "; ")),
		}}
	})
}

// specProblemLimit caps how many validation problems the notification lists. A
// spec with a dozen errors would otherwise arrive as one unreadable line, and the
// build log already holds the full set.
const specProblemLimit = 3

// specInvalidReason builds the build.failed reason for a spec that fails
// validation. Each problem keeps the validator's "field: message" form, which
// reads the way compiler and linter output does, and the lead sentence ends in a
// period rather than a colon so the field paths stay the punctuation the reader
// notices. At most specProblemLimit problems are listed; the rest are counted and
// left to the log.
func specInvalidReason(problems []string) string {
	shown := problems
	if len(shown) > specProblemLimit {
		shown = shown[:specProblemLimit]
	}
	reason := fmt.Sprintf("astropods.yml is invalid. %s.", strings.Join(shown, "; "))
	if rest := len(problems) - len(shown); rest > 0 {
		reason += fmt.Sprintf(" The build log lists %d more.", rest)
	}
	return reason
}

// yamlLinePrefix matches the "line N: message" form the yaml package produces
// for a scanner or parser error once its own "yaml: " prefix is gone. That shape
// carries generic parser vocabulary ("mapping values are not allowed in this
// context"), which is why it is the one message the reason forwards. The errors
// that quote the reader's own file, a duplicate mapping key or an unknown anchor,
// arrive in the "unmarshal errors:" form instead and take the line-only path
// below. TestYAMLSyntaxReasonNeverQuotesTheSpec locks that in: it fails if a yaml
// upgrade ever routes a content-bearing message through here.
var yamlLinePrefix = regexp.MustCompile(`^line (\d+): (.+)$`)

// yamlLineNumber finds a line number anywhere in a parser message, for the forms
// that carry one but do not lead with it.
var yamlLineNumber = regexp.MustCompile(`line (\d+)`)

// yamlSyntaxReason turns a YAML parse error into the sentence the reader sees in
// the build.failed notification. It never forwards the parser's message except in
// the one shape known to be prose: a type error reads "cannot unmarshal !!seq
// into map[string]interface {}", which would put a Go type, a YAML tag, and a
// fragment of the reader's own file into their inbox. Those keep the line number
// and drop the rest, since the build log holds the full text.
func yamlSyntaxReason(err error) string {
	msg := strings.Join(strings.Fields(strings.TrimPrefix(err.Error(), "yaml: ")), " ")
	msg = strings.TrimRight(msg, ".")
	if m := yamlLinePrefix.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("astropods.yml has a syntax error on line %s: %s.", m[1], m[2])
	}
	if m := yamlLineNumber.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("astropods.yml is not valid YAML. Check line %s.", m[1])
	}
	return "astropods.yml is not valid YAML."
}

// FetchSpec downloads astropods.yml from GitHub and parses it.
func (p *GitHubBuildPipeline) FetchSpec() *GitHubBuildPipeline {
	return p.step("fetching-spec", func() error {
		astroSpec, specYAML, err := FetchAstroSpec(p.ctx, p.cfg.Token, p.cfg.RepoName, p.cfg.CommitSHA)
		if err != nil {
			return fmt.Errorf("fetch astropods.yml: %w", err)
		}
		if specYAML == "" {
			short := p.cfg.CommitSHA[:min(7, len(p.cfg.CommitSHA))]
			return PermanentError{Err: SpecError{
				Reason: fmt.Sprintf("No astropods.yml found at commit %s.", short),
				Err:    fmt.Errorf("astropods.yml not found in repo at commit %s", short),
			}}
		}

		p.astroSpec = astroSpec
		p.specYAML = specYAML

		var specMap map[string]any
		if err := yaml.Unmarshal([]byte(specYAML), &specMap); err != nil {
			return PermanentError{Err: SpecError{
				Reason: yamlSyntaxReason(err),
				Err:    fmt.Errorf("parse spec YAML: %w", err),
			}}
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
		readme, _ := FetchAgentReadme(p.ctx, p.cfg.Token, p.cfg.RepoName, p.cfg.CommitSHA)
		p.readme = readme
		return nil
	})
}

// ProcessReadmeImages vacuums local images referenced by AGENT.md into the
// shared assets store and rewrites their links to CDN URLs. Images are fetched
// from the repo at the build commit. Missing or unstorable images are left as
// their original reference (logged), never failing the build.
func (p *GitHubBuildPipeline) ProcessReadmeImages() *GitHubBuildPipeline {
	return p.step("processing-readme-images", func() error {
		if p.cfg.ReadmeAssets == nil || p.readme == "" {
			return nil
		}
		rewritten, warnings := p.cfg.ReadmeAssets.ProcessMarkdown(
			p.ctx, p.cfg.AccountName, p.cfg.AgentName, p.readme,
			func(relPath string) ([]byte, error) {
				return FetchFileBytes(p.ctx, p.cfg.Token, p.cfg.RepoName, p.cfg.CommitSHA, relPath)
			},
		)
		for _, warning := range warnings {
			if p.cfg.Log != nil {
				p.cfg.Log.Warn("readme image skipped", "agent", p.cfg.AgentName, "detail", warning)
			}
		}
		p.readme = rewritten
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
