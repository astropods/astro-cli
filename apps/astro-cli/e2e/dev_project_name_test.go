//go:build integration

package e2e

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	composeBuilder "github.com/astropods/astro/apps/astro-cli/internal/compose"
	spec "github.com/astropods/astro/packages/astro-spec"

	"github.com/docker/cli/cli/command"
	cliflags "github.com/docker/cli/cli/flags"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/docker/compose/v5/pkg/compose"
)

// These tests guard the fix for the "No resource found to remove for project
// @org/my-agent" bug. They boot a minimal nginx "agent" via the compose Go
// SDK using a scoped spec name and verify:
//
//  1. Up uses the sanitized compose project name (matches composeBuilder.ProjectName).
//  2. Down called with the raw scoped spec name does NOT clean up the project
//     (this reproduces the pre-fix bug).
//  3. Down called via composeBuilder.ProjectName DOES clean up the project.
//
// The tests require a working local Docker daemon. Run with:
//
//	go test -tags integration -run TestDevProjectName ./e2e/...

const (
	// nginx:alpine is a small image (~10MB) that stays running on its own,
	// which keeps this test deterministic without needing a custom command
	// (spec.Container has no Command field — the agent image decides).
	longRunningImage = "docker.io/library/nginx:alpine"
	scopedSpecName   = "@astro-it/project-name-fix"
	sanitizedProject = "project-name-fix"
)

func newIntegrationCompose(t *testing.T) api.Compose {
	t.Helper()
	dockerCli, err := command.NewDockerCli()
	if err != nil {
		t.Skipf("docker CLI unavailable: %v", err)
	}
	if err := dockerCli.Initialize(cliflags.NewClientOptions()); err != nil {
		t.Skipf("docker CLI init failed (likely no daemon): %v", err)
	}
	svc, err := compose.NewComposeService(dockerCli)
	if err != nil {
		t.Fatalf("new compose service: %v", err)
	}
	return svc
}

func minimalScopedSpec() *spec.AstroSpec {
	return &spec.AstroSpec{
		Name: scopedSpecName,
		Agent: spec.Container{
			Image: longRunningImage,
		},
	}
}

func containerIDsForProject(t *testing.T, project string) []string {
	t.Helper()
	out, err := exec.Command("docker", "ps", "-a",
		"--filter", "label=com.docker.compose.project="+project,
		"--format", "{{.ID}}").CombinedOutput()
	if err != nil {
		t.Fatalf("docker ps failed: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	ids := make([]string, 0, len(lines))
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			ids = append(ids, l)
		}
	}
	return ids
}

func forceDownProject(svc api.Compose, project string) {
	// Best-effort cleanup used by t.Cleanup. Ignores errors on purpose:
	// cleanup must never fail the test. We call Down with both the raw and
	// sanitized names so a stuck container from a prior failed run is
	// torn down regardless of which name originally created it.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = svc.Down(ctx, project, api.DownOptions{RemoveOrphans: true})
}

// TestDevProjectName_DownWithRawNameIsNoOp reproduces the pre-fix bug: calling
// Down with the raw scoped spec name after an Up (which used the sanitized
// name) leaves containers in place. This test is the "negative" counterpart
// to TestDevProjectName_DownWithSanitizedNameCleans; having both makes the
// regression protection explicit: if either assumption breaks, at least one
// test fails loudly.
func TestDevProjectName_DownWithRawNameIsNoOp(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	svc := newIntegrationCompose(t)

	s := minimalScopedSpec()
	project, err := composeBuilder.BuildProject(s, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}

	upCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := svc.Up(upCtx, project, api.UpOptions{
		Start: api.StartOptions{
			Project:  project,
			Services: []string{"agent"},
		},
		Create: api.CreateOptions{Services: []string{"agent"}},
	}); err != nil {
		t.Fatalf("svc.Up: %v", err)
	}
	t.Cleanup(func() { forceDownProject(svc, sanitizedProject) })

	// Down with the RAW spec name must NOT clean up anything — this is
	// exactly the scenario that produced the user's warning. We expect
	// containers under the sanitized project to still be present.
	downCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_ = svc.Down(downCtx, scopedSpecName, api.DownOptions{RemoveOrphans: true})

	if ids := containerIDsForProject(t, sanitizedProject); len(ids) == 0 {
		t.Errorf("expected containers under %q to survive Down(%q), but none found", sanitizedProject, scopedSpecName)
	}
}

// TestDevProjectName_DownWithSanitizedNameCleans is the happy path: Down with
// ProjectName cleans everything up.
func TestDevProjectName_DownWithSanitizedNameCleans(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker CLI not available")
	}
	svc := newIntegrationCompose(t)

	s := minimalScopedSpec()
	project, err := composeBuilder.BuildProject(s, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("BuildProject: %v", err)
	}

	upCtx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := svc.Up(upCtx, project, api.UpOptions{
		Start: api.StartOptions{
			Project:  project,
			Services: []string{"agent"},
		},
		Create: api.CreateOptions{Services: []string{"agent"}},
	}); err != nil {
		t.Fatalf("svc.Up: %v", err)
	}
	t.Cleanup(func() { forceDownProject(svc, sanitizedProject) })

	if ids := containerIDsForProject(t, sanitizedProject); len(ids) == 0 {
		t.Fatalf("expected at least one container under %q after Up, found none", sanitizedProject)
	}

	downCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := svc.Down(downCtx, composeBuilder.ProjectName(s), api.DownOptions{RemoveOrphans: true}); err != nil {
		t.Fatalf("svc.Down: %v", err)
	}

	if ids := containerIDsForProject(t, sanitizedProject); len(ids) != 0 {
		t.Errorf("Down should have removed all containers under %q, still present: %v", sanitizedProject, ids)
	}
}
