package compose

import (
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/compose/v5/pkg/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper to dereference a *string from env maps, returning "" if nil.
func envVal(env map[string]*string, key string) string {
	if v, ok := env[key]; ok && v != nil {
		return *v
	}
	return ""
}

func TestBuildProject_MinimalSpec(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	if project.Name != "my-agent" {
		t.Errorf("Name = %q, want %q", project.Name, "my-agent")
	}

	if _, ok := project.Services["agent"]; !ok {
		t.Error("missing agent service")
	}

	agent := project.Services["agent"]
	if agent.Image != "agent:latest" {
		t.Errorf("agent.Image = %q, want %q", agent.Image, "agent:latest")
	}
}

// TestBuildProject_NoDeployOnlyEnvLocally pins which platform variables local dev
// does NOT provide. `ast docs` tells agent authors to read ASTRO_AGENT_NAME /
// ASTRO_AGENT_BUILD for OTel service metadata and OTEL_EXPORTER_OTLP_ENDPOINT to
// decide whether to export at all — all three are deployed-only, and an agent that
// reads them without a fallback degrades silently rather than failing loudly. If a
// future change starts injecting one of these locally, the docs go stale with no
// other signal, so assert their absence here.
func TestBuildProject_NoDeployOnlyEnvLocally(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{Adapters: []string{"web"}},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	require.NoError(t, err)

	agent := project.Services["agent"]
	for _, key := range []string{
		"OTEL_EXPORTER_OTLP_ENDPOINT",
		"ASTRO_AGENT_NAME",
		"ASTRO_AGENT_BUILD",
		"ASTRO_AGENT_ID",
	} {
		if _, ok := agent.Environment[key]; ok {
			t.Errorf("%s must not be injected locally — it is deployed-only per `ast docs`", key)
		}
	}

	// The two platform variables local dev does inject, for contrast.
	assert.Equal(t, "astro-messaging:9090", envVal(agent.Environment, "GRPC_SERVER_ADDR"))
	assert.NotEmpty(t, envVal(agent.Environment, "AGENT_FILES_DIR"))
}

func TestBuildProject_ScopedName(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "@example/release-note-helper",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	if project.Name != "release-note-helper" {
		t.Errorf("Name = %q, want %q", project.Name, "release-note-helper")
	}
}

func TestBuildProject_AgentDataVolumeScopedName(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, "/work", nil)
	require.NoError(t, err)

	vol, ok := project.Volumes[agentDataVolume]
	require.True(t, ok, "agent-data volume must be declared")
	// A non-empty name is required: compose rejects an empty volume name at Up
	// time. The name is scoped to the project so each agent gets its own volume;
	// a shared fixed name would leak chat history and uploads between agents.
	require.NotEmpty(t, vol.Name, "agent-data volume needs a non-empty name")
	assert.Equal(t, "my-agent-agent-data", vol.Name)
	assert.Contains(t, vol.Name, project.Name, "volume name must be project-scoped")
}

// bindTargets returns the agent service's bind-mount targets, in mount order.
func bindTargets(t *testing.T, project *types.Project) []string {
	t.Helper()
	svc, err := project.GetService("agent")
	require.NoError(t, err)
	var out []string
	for _, v := range svc.Volumes {
		if v.Type == types.VolumeTypeBind {
			out = append(out, v.Target)
		}
	}
	return out
}

// symlinkOrSkip links newname to oldname. Windows gates symlink creation
// behind a privilege a runner may not grant, and what these tests check is the
// path handling rather than the linking, so an ungranted privilege skips
// instead of failing. Anywhere else a failure here is real.
func symlinkOrSkip(t *testing.T, oldname, newname string) {
	t.Helper()
	err := os.Symlink(oldname, newname)
	if err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		t.Skipf("symlink creation unavailable on this runner: %v", err)
	}
	require.NoError(t, err)
}

// warned renders what warnf writes for msgs, in order. It calls warnf itself so
// an assertion cannot drift from the real prefix and line ending.
func warned(msgs ...string) string {
	var b strings.Builder
	for _, m := range msgs {
		warnf(&b, "%s", m)
	}
	return b.String()
}

func buildWithWatch(t *testing.T, dev *spec.Dev, dirs ...string) *types.Project {
	t.Helper()
	work := t.TempDir()
	for _, d := range dirs {
		require.NoError(t, os.MkdirAll(filepath.Join(work, d), 0o755))
	}
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   dev,
	}
	project, err := BuildProject(s, work, nil)
	require.NoError(t, err)
	return project
}

func TestBuildProject_MountsEveryWatchedDir(t *testing.T) {
	project := buildWithWatch(t, &spec.Dev{Watch: []string{"agent", "src"}}, "agent", "src")

	assert.Equal(t, []string{"/app/agent", "/app/src"}, bindTargets(t, project),
		"an edit under a watched dir must not keep running the image's copy")
}

func TestBuildProject_MountsOnlyAgentWhenTheSpecNamesNothing(t *testing.T) {
	project := buildWithWatch(t, nil, "agent", "src")

	assert.Equal(t, []string{"/app/agent"}, bindTargets(t, project),
		"an existing agent keeps the behavior it had before dev.watch existed")
}

func TestBuildProject_SkipsAWatchedDirThatIsAbsent(t *testing.T) {
	project := buildWithWatch(t, &spec.Dev{Watch: []string{"agent", "lib"}}, "agent")

	assert.Equal(t, []string{"/app/agent"}, bindTargets(t, project),
		"mounting an absent directory would create it on the host")
}

func TestBuildProject_LeavesAnUnwatchedDirToTheImage(t *testing.T) {
	project := buildWithWatch(t, &spec.Dev{Watch: []string{"agent"}}, "agent", "dist", "node_modules")

	targets := bindTargets(t, project)
	assert.NotContains(t, targets, "/app/dist",
		"a directory the image builds must keep the image's copy")
	assert.NotContains(t, targets, "/app/node_modules",
		"the host tree may be absent, or built for another platform")
}

func TestBuildProject_MountsNoSourceWithoutABuild(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
	}

	project, err := BuildProject(s, work, nil)
	require.NoError(t, err)

	assert.Empty(t, bindTargets(t, project), "a prebuilt image has no source to reload from")
}

func TestBuildProject_WarnsAboutAWatchDirThatIsNotThere(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{"agent", "srcc"}},
	}

	var warnings strings.Builder
	_, err := BuildProject(s, work, nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Equal(t, warned(MsgWatchDirMissing("srcc")), warnings.String(),
		"the typo itself is the useful part, and agent/ mounted fine so nothing else warns")
}

func TestBuildProject_WarnsAboutARejectedWatchEntry(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{"../sibling"}},
	}

	var warnings strings.Builder
	_, err := BuildProject(s, work, nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Equal(t,
		warned(MsgWatchDirRejected("../sibling", "the path escapes the project")),
		warnings.String())
}

func TestBuildProject_StaysQuietWhenEveryWatchDirIsThere(t *testing.T) {
	work := t.TempDir()
	for _, d := range []string{"agent", "src"} {
		require.NoError(t, os.MkdirAll(filepath.Join(work, d), 0o755))
	}
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{"agent", "src"}},
	}

	var warnings strings.Builder
	_, err := BuildProject(s, work, nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Empty(t, warnings.String(), "a spec that names what exists has nothing to be told")
}

// The Slack warning printed to real stdout before BuildProject took a writer,
// so nothing could assert it.
func TestBuildProject_WarnsAboutSlackWithoutABotToken(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{Adapters: []string{"slack"}},
			},
		},
	}

	var warnings strings.Builder
	_, err := BuildProject(s, t.TempDir(), nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Equal(t, warned(msgSlackTokenMissing()), warnings.String())
}

func TestPlanWatchDirs_RefusesAPathOutsideTheProject(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))

	for _, dir := range []string{"/etc", "../sibling", "..", "", ".", "src/../.."} {
		plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{dir}}, work)
		require.NoError(t, err, "a bad entry is reported, not an error")

		assert.Empty(t, plan.Mount, "%q must not be mounted", dir)
		require.Len(t, plan.Rejected, 1, "%q must be reported with a reason", dir)
		assert.Equal(t, dir, plan.Rejected[0].Dir)
		assert.NotEmpty(t, plan.Rejected[0].Reason)
	}
}

func TestPlanWatchDirs_MountsAPathEquivalentEntryOnce(t *testing.T) {
	work := t.TempDir()
	for _, d := range []string{"agent", "src", filepath.Join("packages", "core")} {
		require.NoError(t, os.MkdirAll(filepath.Join(work, d), 0o755))
	}

	tests := []struct {
		name      string
		watch     []string
		wantMount []string
		wantDupes []string
	}{
		{
			name:      "the same spelling twice",
			watch:     []string{"agent", "agent"},
			wantMount: []string{"agent"},
			wantDupes: []string{"agent"},
		},
		{
			name:      "a nested path with a trailing separator",
			watch:     []string{"packages/core", "packages/core/"},
			wantMount: []string{"packages/core"},
			wantDupes: []string{"packages/core/"},
		},
		{
			name:      "a trailing separator is the same directory",
			watch:     []string{"agent", "agent/"},
			wantMount: []string{"agent"},
			wantDupes: []string{"agent/"},
		},
		{
			name:      "first seen order survives a duplicate between two others",
			watch:     []string{"src", "src/", "agent"},
			wantMount: []string{"src", "agent"},
			wantDupes: []string{"src/"},
		},
		{
			name:      "distinct directories are not duplicates",
			watch:     []string{"agent", "src"},
			wantMount: []string{"agent", "src"},
			wantDupes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanWatchDirs(&spec.Dev{Watch: tt.watch}, work)
			require.NoError(t, err)

			assert.Equal(t, tt.wantMount, plan.Mount,
				"two mounts on one container path make Docker refuse the container")
			assert.Equal(t, tt.wantDupes, plan.Duplicate,
				"a skipped duplicate is reported with the spelling the spec used")
		})
	}
}

func TestBuildProject_EmitsOneBindPerContainerPath(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{"agent", "agent/"}},
	}

	var warnings strings.Builder
	project, err := BuildProject(s, work, nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Equal(t, []string{"/app/agent"}, bindTargets(t, project),
		"a repeated entry must not become a second mount on the same target")
	assert.Equal(t, warned(MsgWatchDirDuplicate("agent/")), warnings.String(),
		"the duplicate is reported with the spelling the spec used")
}

func TestPlanWatchDirs_RefusesASymlinkOutOfTheProject(t *testing.T) {
	work := t.TempDir()
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "elsewhere"), 0o755))
	symlinkOrSkip(t, filepath.Join(outside, "elsewhere"), filepath.Join(work, "src"))

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"src"}}, work)
	require.NoError(t, err)

	assert.Empty(t, plan.Mount, "a symlink out of the project must not be mounted")
	require.Len(t, plan.Rejected, 1)
	assert.Contains(t, plan.Rejected[0].Reason, "resolves outside the project",
		"the lexical check passes here, so the reason must name the resolution")
}

func TestPlanWatchDirs_AllowsASymlinkInsideTheProject(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "packages", "core"), 0o755))
	symlinkOrSkip(t, filepath.Join(work, "packages", "core"), filepath.Join(work, "src"))

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"src"}}, work)
	require.NoError(t, err)

	assert.Equal(t, []string{"src"}, plan.Mount,
		"a symlink that stays inside the project is a normal layout")
	assert.Empty(t, plan.Rejected)
}

func TestPlanWatchDirs_RefusesTheProjectRoot(t *testing.T) {
	work := t.TempDir()

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"."}}, work)
	require.NoError(t, err)

	assert.Contains(t, plan.Rejected[0].Reason, "hide everything the image built",
		"mounting the root is the failure dev.watch exists to avoid")
}

func TestPlanWatchDirs_AllowsANestedDir(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "packages", "core"), 0o755))

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"packages/core"}}, work)
	require.NoError(t, err)

	assert.Equal(t, []string{"packages/core"}, plan.Mount,
		"a nested path inside the project is fine, and Mount is slash form on every host")
	assert.Empty(t, plan.Rejected)
}

func TestBuildProject_MountsANestedDirAtAPosixContainerPath(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "packages", "core"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{"packages/core"}},
	}

	project, err := BuildProject(s, work, nil, WithWarnings(io.Discard))
	require.NoError(t, err)

	targets := bindTargets(t, project)
	assert.Equal(t, []string{"/app/packages/core"}, targets,
		"a container path is POSIX whatever the host separator is")
	for _, target := range targets {
		assert.NotContains(t, target, `\`,
			"a host separator in a container path collapses the whole thing into one name")
	}
}

func TestBuildProject_RefusesANativelySpelledNestedDir(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "packages", "core"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{`packages\core`}},
	}

	var warnings strings.Builder
	project, err := BuildProject(s, work, nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Empty(t, bindTargets(t, project),
		"a spec is slash-separated on every host, so the native spelling is refused even where it would resolve")
	assert.Contains(t, warnings.String(), "use /")
}

func TestBuildProject_SilencesWatchWarningsWithoutSilencingTheWriter(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev: &spec.Dev{
			Watch: []string{"agent", "srcc"},
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{Adapters: []string{"slack"}},
			},
		},
	}

	var warnings strings.Builder
	_, err := BuildProject(s, work, nil,
		WithWarnings(&warnings),
		WithWatchDirWarnings(false),
	)
	require.NoError(t, err)

	assert.Equal(t, warned(msgSlackTokenMissing()), warnings.String(),
		"only the watch warnings go quiet, so the writer still carries everything else")
}

func TestPlanWatchDirs_ReportsANamedDirThatIsMissing(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"agent", "srcc"}}, work)
	require.NoError(t, err)

	assert.Equal(t, []string{"agent"}, plan.Mount)
	assert.Equal(t, []string{"srcc"}, plan.Missing,
		"a typo in dev.watch must be reported, not silently reload nothing")
}

func TestPlanWatchDirs_StaysQuietAboutTheImplicitDefault(t *testing.T) {
	plan, err := PlanWatchDirs(nil, t.TempDir())
	require.NoError(t, err)

	assert.Empty(t, plan.Mount)
	assert.Empty(t, plan.Missing,
		"the agent/ default is assumed rather than asked for, so its absence is not a mistake")
}

func TestPlanWatchDirs_ReportsANamedPathThatIsNotADirectory(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(work, "src"), []byte("x"), 0o600))

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"src"}}, work)
	require.NoError(t, err)

	assert.Empty(t, plan.Mount)
	require.Len(t, plan.Rejected, 1, "a file is present, so it is unmountable rather than missing")
	assert.Equal(t, "src", plan.Rejected[0].Dir)
	assert.Contains(t, plan.Rejected[0].Reason, "only a directory can be mounted")
}

func TestPlanWatchDirs_FailsOnAStatErrorThatIsNotAbsence(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "src"), 0o755))

	// chmod cannot force this for a process running as root, which is how CI
	// containers commonly run, so the stat is replaced instead.
	original := statDir
	statDir = func(string) (os.FileInfo, error) { return nil, fs.ErrPermission }
	t.Cleanup(func() { statDir = original })

	_, err := PlanWatchDirs(&spec.Dev{Watch: []string{"src"}}, work)

	require.Error(t, err, "an unreadable directory is a broken checkout, not an optional one")
	assert.Contains(t, err.Error(), "src", "the error must name the directory it could not read")
	assert.ErrorIs(t, err, fs.ErrPermission, "the cause must survive wrapping")
}

func TestSubdir(t *testing.T) {
	root := filepath.Join("proj")

	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "a child", target: filepath.Join("proj", "agent"), want: true},
		{name: "a grandchild", target: filepath.Join("proj", "a", "b"), want: true},
		{name: "root itself", target: "proj", want: false},
		{name: "a sibling", target: "other", want: false},
		{name: "a parent", target: ".", want: false},
		{name: "a climb back in", target: filepath.Join("proj", "..", "proj", "a"), want: true},
		{name: "a climb out", target: filepath.Join("proj", "..", "other"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, subdir(root, tt.target),
				"subdir must report only paths strictly inside root")
		})
	}
}

func TestPlanWatchDirs_ReportsARepeatedEntryOnceWhateverItsOutcome(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(work, "afile"), []byte("x"), 0o644))

	tests := []struct {
		name         string
		watch        []string
		wantMissing  []string
		wantRejected []WatchDirRejection
		wantDupes    []string
	}{
		{
			name:        "a missing directory named twice",
			watch:       []string{"nope", "nope"},
			wantMissing: []string{"nope"},
			wantDupes:   []string{"nope"},
		},
		{
			name:  "a rejected entry named twice",
			watch: []string{"../sibling", "../sibling"},
			wantRejected: []WatchDirRejection{
				{Dir: "../sibling", Reason: "the path escapes the project"},
			},
			wantDupes: []string{"../sibling"},
		},
		{
			name:  "a non-directory named twice",
			watch: []string{"afile", "afile/"},
			wantRejected: []WatchDirRejection{
				{Dir: "afile", Reason: "only a directory can be mounted"},
			},
			wantDupes: []string{"afile/"},
		},
		{
			name:  "an empty entry and . are separate mistakes",
			watch: []string{"", "."},
			wantRejected: []WatchDirRejection{
				{Dir: "", Reason: "an empty entry names no directory"},
				{Dir: ".", Reason: "the project root would hide everything the image built"},
			},
			wantDupes: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := PlanWatchDirs(&spec.Dev{Watch: tt.watch}, work)
			require.NoError(t, err)

			assert.Empty(t, plan.Mount, "none of these entries is mountable")
			assert.Equal(t, tt.wantMissing, plan.Missing,
				"a repeat must not produce a second identical warning")
			assert.Equal(t, tt.wantRejected, plan.Rejected,
				"a repeat must not produce a second identical warning")
			assert.Equal(t, tt.wantDupes, plan.Duplicate,
				"the repeat is reported as a duplicate, with the spelling the spec used")
		})
	}
}

func TestPlanWatchDirs_RefusesASymlinkToTheProjectRoot(t *testing.T) {
	work := t.TempDir()
	symlinkOrSkip(t, ".", filepath.Join(work, "rootlink"))

	plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{"rootlink"}}, work)
	require.NoError(t, err)

	assert.Empty(t, plan.Mount,
		"a link to the root mounts the whole project, the same thing watch: [.] is refused for")
	require.Len(t, plan.Rejected, 1)
	assert.Equal(t, "the project root would hide everything the image built",
		plan.Rejected[0].Reason,
		"the reason must name what it resolved to, not claim it left the project")
}

func TestBuildProject_WarnsOnceForARepeatedMissingWatchDir(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Build: &spec.BuildConfig{Context: "."}},
		Dev:   &spec.Dev{Watch: []string{"agent", "srcc", "srcc"}},
	}

	var warnings strings.Builder
	_, err := BuildProject(s, work, nil, WithWarnings(&warnings))
	require.NoError(t, err)

	assert.Equal(t,
		warned(MsgWatchDirDuplicate("srcc"), MsgWatchDirMissing("srcc")),
		warnings.String(),
		"the same typo twice must not print the same missing-directory warning twice")
}

func TestBuildProject_SlackInterface(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
				},
			},
		},
	}

	envVars := map[string]string{
		"SLACK_BOT_TOKEN": "xoxb-test",
		"SLACK_APP_TOKEN": "xapp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	// Should create messaging sidecar
	messaging, ok := project.Services["astro-messaging"]
	if !ok {
		t.Fatal("missing astro-messaging service")
	}

	// Messaging env should have slack tokens
	if envVal(messaging.Environment, "SLACK_BOT_TOKEN") != "xoxb-test" {
		t.Errorf("SLACK_BOT_TOKEN = %q, want %q", envVal(messaging.Environment, "SLACK_BOT_TOKEN"), "xoxb-test")
	}
	if envVal(messaging.Environment, "SLACK_ENABLED") != "true" {
		t.Error("SLACK_ENABLED should be true")
	}

	// Dev mode should always be enabled in ast dev
	if envVal(messaging.Environment, "DEV") != "true" {
		t.Error("DEV should be true for messaging in dev mode")
	}

	// Should NOT have playground (only slack, no web)
	if _, ok := project.Services["playground"]; ok {
		t.Error("playground should not exist for slack-only interface")
	}

	// Agent should have GRPC_SERVER_ADDR
	agent := project.Services["agent"]
	if envVal(agent.Environment, "GRPC_SERVER_ADDR") != "astro-messaging:9090" {
		t.Error("agent should have GRPC_SERVER_ADDR")
	}
}

// dev.interfaces.messaging.log_level overrides the messaging sidecar's
// default LOG_LEVEL=info. Empty / unset falls back to info.
func TestBuildProject_MessagingLogLevel(t *testing.T) {
	cases := []struct {
		name     string
		override string
		want     string
	}{
		{"default when unset", "", "debug"},
		{"override applied", "info", "info"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &spec.AstroSpec{
				Name:  "my-agent",
				Agent: spec.Container{Image: "agent:latest"},
				Dev: &spec.Dev{
					Interfaces: &spec.DevInterfaces{
						Messaging: &spec.DevMessaging{
							Adapters: []string{"slack"},
							LogLevel: tc.override,
						},
					},
				},
			}
			envVars := map[string]string{
				"SLACK_BOT_TOKEN": "xoxb-test",
				"SLACK_APP_TOKEN": "xapp-test",
			}
			project, err := BuildProject(s, "/work", envVars)
			if err != nil {
				t.Fatalf("BuildProject() error = %v", err)
			}
			messaging, ok := project.Services["astro-messaging"]
			if !ok {
				t.Fatal("missing astro-messaging service")
			}
			if got := envVal(messaging.Environment, "LOG_LEVEL"); got != tc.want {
				t.Errorf("LOG_LEVEL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildProject_SlackInterface_MissingToken(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack", "web"},
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", map[string]string{})
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	messaging, ok := project.Services["astro-messaging"]
	if !ok {
		t.Fatal("missing astro-messaging service")
	}

	if envVal(messaging.Environment, "SLACK_ENABLED") != "" {
		t.Error("SLACK_ENABLED should not be set when token is missing")
	}
	if envVal(messaging.Environment, "WEB_ENABLED") != "true" {
		t.Error("WEB_ENABLED should still be true")
	}
}

func TestBuildProject_WebInterface(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"web"},
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	// Should create messaging sidecar (playground is bundled into messaging, not a separate service)
	if _, ok := project.Services["astro-messaging"]; !ok {
		t.Error("missing astro-messaging service")
	}
	if _, ok := project.Services["playground"]; ok {
		t.Error("playground should not be a separate service — it is bundled into messaging")
	}

	// Messaging is the chat backend only: WEB_ENABLED stays true, but the chat
	// UI is served by the CLI (internal/chatui) and chat history is persisted
	// via CHAT_DB_PATH.
	messaging := project.Services["astro-messaging"]
	if envVal(messaging.Environment, "WEB_ENABLED") != "true" {
		t.Error("WEB_ENABLED should be true")
	}
	if envVal(messaging.Environment, "CHAT_DB_PATH") != chatDBPath {
		t.Errorf("CHAT_DB_PATH should be %q, got %q", chatDBPath, envVal(messaging.Environment, "CHAT_DB_PATH"))
	}
	if envVal(messaging.Environment, "DEV") != "true" {
		t.Error("DEV should be true for messaging in dev mode")
	}

	// Messaging should expose gRPC (19090->9090) and HTTP (MessagingWebHostPort
	// ->8080). The CLI serves the chat UI on 3100 and proxies to the HTTP port.
	if len(messaging.Ports) != 2 {
		t.Errorf("messaging ports = %d, want 2 (grpc + http)", len(messaging.Ports))
	}
	hasGrpc := false
	hasWeb := false
	for _, p := range messaging.Ports {
		if p.Target == 9090 && p.Published == "19090" {
			hasGrpc = true
		}
		if p.Target == 8080 && p.Published == MessagingWebHostPort {
			hasWeb = true
		}
	}
	if !hasGrpc {
		t.Errorf("messaging should publish gRPC as 19090->9090, got %#v", messaging.Ports)
	}
	if !hasWeb {
		t.Errorf("messaging should publish web as %s->8080, got %#v", MessagingWebHostPort, messaging.Ports)
	}

	// Chat history persistence volume should be mounted at /data.
	hasChatVolume := false
	for _, v := range messaging.Volumes {
		if v.Target == chatDataMountPath {
			hasChatVolume = true
		}
	}
	if !hasChatVolume {
		t.Errorf("messaging should mount a chat-data volume at %s, got %#v", chatDataMountPath, messaging.Volumes)
	}

	// A one-shot init container must chown the fresh (root-owned) chat-data
	// volume to the non-root astro uid before the sidecar starts; otherwise the
	// sidecar cannot create its SQLite DB on /data and crashes (SQLITE_CANTOPEN).
	init, ok := project.Services["astro-messaging-init"]
	if !ok {
		t.Fatal("missing astro-messaging-init service (chowns chat-data volume for non-root sidecar)")
	}
	if init.User != "0:0" {
		t.Errorf("init container must run as root to chown the volume, got user %q", init.User)
	}
	initMountsData := false
	for _, v := range init.Volumes {
		if v.Target == chatDataMountPath {
			initMountsData = true
		}
	}
	if !initMountsData {
		t.Errorf("init container must mount the chat-data volume at %s, got %#v", chatDataMountPath, init.Volumes)
	}
	dep, ok := messaging.DependsOn["astro-messaging-init"]
	if !ok {
		t.Fatal("messaging must depend on astro-messaging-init")
	}
	if dep.Condition != types.ServiceConditionCompletedSuccessfully {
		t.Errorf("messaging should wait for init to complete successfully, got condition %q", dep.Condition)
	}

	// Collector should not be present in dev mode (runs as K8s sidecar only).
	if _, ok := project.Services["astro-collector"]; ok {
		t.Error("astro-collector should not be present in dev compose")
	}
}

func TestBuildProject_CloudProviderCredentials(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{
			"anthropic": {Provider: "anthropic"},
		},
		Integrations: map[string]spec.Integration{
			"github": {Provider: "github"},
		},
	}

	envVars := map[string]string{
		"ANTHROPIC_API_KEY": "sk-test",
		"GITHUB_TOKEN":      "ghp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "ANTHROPIC_API_KEY") != "sk-test" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", envVal(agent.Environment, "ANTHROPIC_API_KEY"), "sk-test")
	}
	if envVal(agent.Environment, "GITHUB_TOKEN") != "ghp-test" {
		t.Errorf("GITHUB_TOKEN = %q, want %q", envVal(agent.Environment, "GITHUB_TOKEN"), "ghp-test")
	}

	// Cloud providers should NOT create services
	if _, ok := project.Services["model-anthropic"]; ok {
		t.Error("cloud model provider should not create a service")
	}
	if _, ok := project.Services["tool-github"]; ok {
		t.Error("cloud integration provider should not create a service")
	}
}

func TestBuildProject_CustomProviderCredentials(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"my-service": {
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "API_KEY", Datatype: "string", Secret: true},
					{Name: "SECRET", Datatype: "string", Secret: true},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"jira": {Provider: "my-service"},
		},
	}

	// Resolver emits <PROVIDER>_<VARNAME>: MY_SERVICE_API_KEY / MY_SERVICE_SECRET.
	envVars := map[string]string{
		"MY_SERVICE_API_KEY": "key1",
		"MY_SERVICE_SECRET":  "s3cret",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "MY_SERVICE_API_KEY") != "key1" {
		t.Errorf("MY_SERVICE_API_KEY = %q, want %q", envVal(agent.Environment, "MY_SERVICE_API_KEY"), "key1")
	}
	if envVal(agent.Environment, "MY_SERVICE_SECRET") != "s3cret" {
		t.Errorf("MY_SERVICE_SECRET = %q, want %q", envVal(agent.Environment, "MY_SERVICE_SECRET"), "s3cret")
	}
}

func TestBuildProject_CustomProviderMissingEnvVar(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"my-service": {
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "API_KEY", Datatype: "string", Secret: true},
					{Name: "SECRET", Datatype: "string", Secret: true},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"jira": {Provider: "my-service"},
		},
	}

	// Only provide one of two variables (using the resolver-correct prefixed name).
	envVars := map[string]string{
		"MY_SERVICE_API_KEY": "key1",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "MY_SERVICE_API_KEY") != "key1" {
		t.Error("present env var should be injected")
	}
	if _, ok := agent.Environment["MY_SERVICE_SECRET"]; ok {
		t.Error("absent env var should not be injected")
	}
}

func TestBuildProject_KnowledgeStore(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"docs": {Provider: "qdrant"},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["knowledge-docs"]
	if !ok {
		t.Fatal("missing knowledge-docs service")
	}
	if svc.Image != "qdrant/qdrant:latest" {
		t.Errorf("Image = %q, want %q", svc.Image, "qdrant/qdrant:latest")
	}

	// Should have persistent volume
	if len(svc.Volumes) == 0 {
		t.Error("persistent knowledge store should have volumes")
	}
	if _, ok := project.Volumes["knowledge-docs-data"]; !ok {
		t.Error("missing volume knowledge-docs-data")
	}

	// Agent should get QDRANT_HOST and QDRANT_PORT
	agent := project.Services["agent"]
	if envVal(agent.Environment, "QDRANT_HOST") != "knowledge-docs" {
		t.Errorf("QDRANT_HOST = %q, want %q", envVal(agent.Environment, "QDRANT_HOST"), "knowledge-docs")
	}
}

func TestBuildProject_CustomKnowledgePersistence(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Container: &spec.ContainerConfig{
					Image:  "pgvector/pgvector:pg17",
					Port:   5432,
					Volume: "/var/lib/postgresql/data",
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["knowledge-db"]
	if !ok {
		t.Fatal("missing knowledge-db service")
	}
	if svc.Image != "pgvector/pgvector:pg17" {
		t.Errorf("Image = %q, want %q", svc.Image, "pgvector/pgvector:pg17")
	}
	if len(svc.Volumes) == 0 {
		t.Error("persistent custom container should have volumes")
	} else if svc.Volumes[0].Target != "/var/lib/postgresql/data" {
		t.Errorf("volume target = %q, want %q", svc.Volumes[0].Target, "/var/lib/postgresql/data")
	}
	if _, ok := project.Volumes["knowledge-db-data"]; !ok {
		t.Error("missing volume knowledge-db-data")
	}
}

func TestBuildProject_KnowledgeInputsInjected(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {
				Container: &spec.ContainerConfig{
					Image:  "postgres:17",
					Port:   5432,
					Volume: "/var/lib/postgresql/data",
				},
				Inputs: []spec.Input{
					{Name: "POSTGRES_PASSWORD", Datatype: "string", Default: "default-pw"},
					{Name: "POSTGRES_DB", Datatype: "string", Default: "my_db"},
				},
			},
		},
	}

	// Test with default values
	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["knowledge-db"]
	if envVal(svc.Environment, "POSTGRES_PASSWORD") != "default-pw" {
		t.Errorf("POSTGRES_PASSWORD = %q, want %q", envVal(svc.Environment, "POSTGRES_PASSWORD"), "default-pw")
	}
	if envVal(svc.Environment, "POSTGRES_DB") != "my_db" {
		t.Errorf("POSTGRES_DB = %q, want %q", envVal(svc.Environment, "POSTGRES_DB"), "my_db")
	}

	// Test with envVars override
	project2, err := BuildProject(s, "/work", map[string]string{
		"POSTGRES_PASSWORD": "override-pw",
	})
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc2 := project2.Services["knowledge-db"]
	if envVal(svc2.Environment, "POSTGRES_PASSWORD") != "override-pw" {
		t.Errorf("POSTGRES_PASSWORD = %q, want %q", envVal(svc2.Environment, "POSTGRES_PASSWORD"), "override-pw")
	}
	// POSTGRES_DB should still use default
	if envVal(svc2.Environment, "POSTGRES_DB") != "my_db" {
		t.Errorf("POSTGRES_DB = %q, want %q", envVal(svc2.Environment, "POSTGRES_DB"), "my_db")
	}
}

func TestBuildProject_PostgresCredentialsAutoInjected(t *testing.T) {
	// Container-mode postgres should get USER/PASSWORD/DB on both the sidecar
	// and the agent without the user declaring any inputs — mirrors prod, where
	// generateKnowledgeCredentials does the same.
	s := &spec.AstroSpec{
		Name:  "recruiter-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"db": {Provider: "postgres"},
		},
	}

	t.Run("defaults when no envVars", func(t *testing.T) {
		project, err := BuildProject(s, "/work", nil)
		require.NoError(t, err)

		sidecar := project.Services["knowledge-db"]
		assert.Equal(t, "astro", envVal(sidecar.Environment, "POSTGRES_USER"))
		assert.Equal(t, "localdev", envVal(sidecar.Environment, "POSTGRES_PASSWORD"))
		assert.Equal(t, "recruiter_agent", envVal(sidecar.Environment, "POSTGRES_DB"))

		agent := project.Services["agent"]
		assert.Equal(t, "astro", envVal(agent.Environment, "POSTGRES_USER"))
		assert.Equal(t, "localdev", envVal(agent.Environment, "POSTGRES_PASSWORD"))
		assert.Equal(t, "recruiter_agent", envVal(agent.Environment, "POSTGRES_DB"))
		assert.Equal(t, "knowledge-db", envVal(agent.Environment, "POSTGRES_HOST"))
		assert.Equal(t, "5432", envVal(agent.Environment, "POSTGRES_PORT"))
	})

	t.Run("envVars override defaults on both sidecar and agent", func(t *testing.T) {
		project, err := BuildProject(s, "/work", map[string]string{
			"POSTGRES_USER":     "custom_user",
			"POSTGRES_PASSWORD": "custom_pw",
			"POSTGRES_DB":       "custom_db",
		})
		require.NoError(t, err)

		for _, svcName := range []string{"knowledge-db", "agent"} {
			svc := project.Services[svcName]
			assert.Equal(t, "custom_user", envVal(svc.Environment, "POSTGRES_USER"), svcName)
			assert.Equal(t, "custom_pw", envVal(svc.Environment, "POSTGRES_PASSWORD"), svcName)
			assert.Equal(t, "custom_db", envVal(svc.Environment, "POSTGRES_DB"), svcName)
		}
	})
}

func TestBuildProject_IntegrationInputsInjected(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Integrations: map[string]spec.Integration{
			"mcp": {
				Container: &spec.ContainerConfig{
					Image: "my-mcp:latest",
					Port:  8080,
				},
				Inputs: []spec.Input{
					{Name: "API_KEY", Datatype: "string", Default: "test-key"},
				},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["tool-mcp"]
	if envVal(svc.Environment, "API_KEY") != "test-key" {
		t.Errorf("API_KEY = %q, want %q", envVal(svc.Environment, "API_KEY"), "test-key")
	}
}

func TestBuildProject_KnowledgeExtraPortsPublished(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"graph": {Provider: "neo4j"},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["knowledge-graph"]
	if !ok {
		t.Fatal("missing knowledge-graph service")
	}

	hasBoltPort := false
	hasDefaultPort := false
	for _, p := range svc.Ports {
		if p.Target == 7687 && p.Published == "7687" {
			hasBoltPort = true
		}
		if p.Target == 7474 && p.Published == "7474" {
			hasDefaultPort = true
		}
	}
	if !hasDefaultPort {
		t.Error("Neo4j default HTTP port 7474 should be published")
	}
	if !hasBoltPort {
		t.Error("Neo4j bolt port 7687 should be published via ExtraPorts")
	}
}

func TestBuildProject_QdrantExtraPortsPublished(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Knowledge: map[string]spec.Knowledge{
			"docs": {Provider: "qdrant"},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["knowledge-docs"]
	hasGrpcPort := false
	for _, p := range svc.Ports {
		if p.Target == 6334 && p.Published == "6334" {
			hasGrpcPort = true
		}
	}
	if !hasGrpcPort {
		t.Error("Qdrant gRPC port 6334 should be published via ExtraPorts")
	}
}

func TestBuildProject_SlackConfigJSON(t *testing.T) {
	boolPtr := func(v bool) *bool { return &v }

	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
					Slack: &spec.SlackAdapterConfig{
						ActionableReactions: []string{"ticket", "bug"},
						AllowedChannelIDs:   []string{"C123", "C999"},
						AllowedUserIDs:      []string{"U123", "U999"},
						SocketMode:          boolPtr(false),
						AutoThread:          boolPtr(true),
					},
				},
			},
		},
	}

	envVars := map[string]string{
		"SLACK_BOT_TOKEN": "xoxb-test",
		"SLACK_APP_TOKEN": "xapp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	messaging := project.Services["astro-messaging"]

	if _, ok := messaging.Environment["SLACK_ACTIONABLE_REACTIONS"]; ok {
		t.Error("legacy SLACK_ACTIONABLE_REACTIONS should no longer be set")
	}
	if _, ok := messaging.Environment["SLACK_SOCKET_MODE"]; ok {
		t.Error("legacy SLACK_SOCKET_MODE should no longer be set")
	}

	raw := envVal(messaging.Environment, "SLACK_CONFIG")
	if raw == "" {
		t.Fatal("SLACK_CONFIG env var not set")
	}

	var cfg spec.SlackAdapterConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("SLACK_CONFIG is not valid JSON: %v", err)
	}
	if len(cfg.ActionableReactions) != 2 || cfg.ActionableReactions[0] != "ticket" || cfg.ActionableReactions[1] != "bug" {
		t.Errorf("ActionableReactions = %v, want [ticket bug]", cfg.ActionableReactions)
	}
	if len(cfg.AllowedChannelIDs) != 2 || cfg.AllowedChannelIDs[0] != "C123" || cfg.AllowedChannelIDs[1] != "C999" {
		t.Errorf("AllowedChannelIDs = %v, want [C123 C999]", cfg.AllowedChannelIDs)
	}
	if len(cfg.AllowedUserIDs) != 2 || cfg.AllowedUserIDs[0] != "U123" || cfg.AllowedUserIDs[1] != "U999" {
		t.Errorf("AllowedUserIDs = %v, want [U123 U999]", cfg.AllowedUserIDs)
	}
	if cfg.SocketMode == nil || *cfg.SocketMode != false {
		t.Errorf("SocketMode = %v, want false", cfg.SocketMode)
	}
	if cfg.AutoThread == nil || *cfg.AutoThread != true {
		t.Errorf("AutoThread = %v, want true", cfg.AutoThread)
	}
}

func TestBuildProject_SlackNoConfigBlock(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
				},
			},
		},
	}

	envVars := map[string]string{
		"SLACK_BOT_TOKEN": "xoxb-test",
		"SLACK_APP_TOKEN": "xapp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	messaging := project.Services["astro-messaging"]
	if _, ok := messaging.Environment["SLACK_CONFIG"]; ok {
		t.Error("SLACK_CONFIG should not be set when no slack block is configured")
	}
	if _, ok := messaging.Environment["SLACK_ACTIONABLE_REACTIONS"]; ok {
		t.Error("legacy SLACK_ACTIONABLE_REACTIONS should never be set")
	}
}

func TestBuildProject_SlackConfigReactionsOnly(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Messaging: &spec.DevMessaging{
					Adapters: []string{"slack"},
					Slack: &spec.SlackAdapterConfig{
						ActionableReactions: []string{"ticket"},
					},
				},
			},
		},
	}

	envVars := map[string]string{
		"SLACK_BOT_TOKEN": "xoxb-test",
		"SLACK_APP_TOKEN": "xapp-test",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	messaging := project.Services["astro-messaging"]
	raw := envVal(messaging.Environment, "SLACK_CONFIG")
	if raw == "" {
		t.Fatal("SLACK_CONFIG env var not set")
	}

	var cfg spec.SlackAdapterConfig
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("SLACK_CONFIG is not valid JSON: %v", err)
	}
	if len(cfg.ActionableReactions) != 1 || cfg.ActionableReactions[0] != "ticket" {
		t.Errorf("ActionableReactions = %v, want [ticket]", cfg.ActionableReactions)
	}
	if cfg.SocketMode != nil {
		t.Errorf("SocketMode should be omitted (nil), got %v", *cfg.SocketMode)
	}
	if cfg.AutoThread != nil {
		t.Errorf("AutoThread should be omitted (nil), got %v", *cfg.AutoThread)
	}
}

func TestBuildProject_IngestionScheduleHasProfile(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"schedule": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/schedule/Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "schedule"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["ingestion-schedule"]
	if !ok {
		t.Fatal("missing ingestion-schedule service")
	}
	// schedule-type ingestions are on-demand — must have the ingestion profile
	if len(svc.Profiles) != 1 || svc.Profiles[0] != "ingestion" {
		t.Errorf("profiles = %v, want [ingestion]", svc.Profiles)
	}
	// must not expose ports
	if len(svc.Ports) != 0 {
		t.Errorf("unexpected ports on schedule ingestion: %v", svc.Ports)
	}
}

func TestBuildProject_IngestionStartupHasProfile(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"data": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/data/Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "startup"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["ingestion-data"]
	if !ok {
		t.Fatal("missing ingestion-data service")
	}
	if len(svc.Profiles) != 1 || svc.Profiles[0] != "ingestion" {
		t.Errorf("profiles = %v, want [ingestion]", svc.Profiles)
	}
}

func TestBuildProject_IngestionWebhookNoProfileExposesPort(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"webhook": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/webhook/Dockerfile"},
					Port:  3001,
				},
				Trigger: spec.IngestionTrigger{Type: "webhook"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc, ok := project.Services["ingestion-webhook"]
	if !ok {
		t.Fatal("missing ingestion-webhook service")
	}
	// webhook ingestions are persistent servers — must NOT have the ingestion profile
	if len(svc.Profiles) != 0 {
		t.Errorf("webhook ingestion should have no profiles, got %v", svc.Profiles)
	}
	// must expose port 3001
	if len(svc.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Ports))
	}
	if svc.Ports[0].Target != 3001 {
		t.Errorf("port target = %d, want 3001", svc.Ports[0].Target)
	}
	if svc.Ports[0].Published != "3001" {
		t.Errorf("port published = %q, want 3001", svc.Ports[0].Published)
	}
}

func TestBuildProject_IngestionWebhookDefaultPort(t *testing.T) {
	// When no port is specified in the spec, webhook should default to 3001
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"webhook": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "ingestion/webhook/Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "webhook"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	svc := project.Services["ingestion-webhook"]
	if len(svc.Ports) != 1 || svc.Ports[0].Target != 3001 {
		t.Errorf("expected default port 3001, got %v", svc.Ports)
	}
}

func TestBuildProject_FrontendInterface(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Frontend: true},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if len(agent.Ports) != 1 {
		t.Fatalf("agent ports = %d, want 1", len(agent.Ports))
	}
	if agent.Ports[0].Target != 80 {
		t.Errorf("port target = %d, want 80 (default)", agent.Ports[0].Target)
	}
	if agent.Ports[0].Published != "3200" {
		t.Errorf("port published = %q, want 3200", agent.Ports[0].Published)
	}
}

func TestBuildEnvironment_FrontendInjectsPORT(t *testing.T) {
	t.Run("default port 80", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name: "my-agent",
			Agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: true},
			},
		}
		env := BuildEnvironment(s, nil)
		if got := envVal(env, "PORT"); got != "80" {
			t.Errorf("PORT = %q, want 80", got)
		}
	})

	t.Run("dev.interfaces.frontend.port override", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name: "my-agent",
			Agent: spec.Container{
				Image:      "agent:latest",
				Interfaces: &spec.Interfaces{Frontend: true},
			},
			Dev: &spec.Dev{
				Interfaces: &spec.DevInterfaces{
					Frontend: &spec.DevFrontend{Port: 3000},
				},
			},
		}
		env := BuildEnvironment(s, nil)
		if got := envVal(env, "PORT"); got != "3000" {
			t.Errorf("PORT = %q, want 3000", got)
		}
	})

	t.Run("no frontend → no PORT", func(t *testing.T) {
		s := &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "agent:latest"},
		}
		env := BuildEnvironment(s, nil)
		if _, ok := env["PORT"]; ok {
			t.Error("PORT should not be set when frontend is disabled")
		}
	})
}

func TestBuildProject_FrontendInterfaceCustomPort(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Frontend: true},
		},
		Dev: &spec.Dev{
			Interfaces: &spec.DevInterfaces{
				Frontend: &spec.DevFrontend{Port: 3000},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if len(agent.Ports) != 1 {
		t.Fatalf("agent ports = %d, want 1", len(agent.Ports))
	}
	if agent.Ports[0].Target != 3000 {
		t.Errorf("port target = %d, want 3000 (custom)", agent.Ports[0].Target)
	}
}

func TestBuildProject_NoFrontendNoPorts(t *testing.T) {
	s := &spec.AstroSpec{
		Name: "my-agent",
		Agent: spec.Container{
			Image:      "agent:latest",
			Interfaces: &spec.Interfaces{Messaging: true},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if len(agent.Ports) != 0 {
		t.Errorf("agent should have no ports when frontend is not enabled, got %d", len(agent.Ports))
	}
}

func TestBuildProject_CustomProviderPrefixedKeys(t *testing.T) {
	// When ast configure stores keys as CLOUDFLARE_AI_API_KEY (prefix + var name),
	// buildEnvironment must find them and inject both prefixed and bare forms.
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Providers: map[string]spec.CustomProvider{
			"cloudflare": {
				Scope: []string{"integrations"},
				Variables: []spec.Input{
					{Name: "AI_API_KEY", Datatype: "string", Secret: true},
					{Name: "ACCOUNT_ID", Datatype: "string", Secret: true},
				},
			},
		},
		Integrations: map[string]spec.Integration{
			"cloudflare": {Provider: "cloudflare"},
		},
	}

	// Keys stored by ast configure with CLOUDFLARE_ prefix
	envVars := map[string]string{
		"CLOUDFLARE_AI_API_KEY": "cf-key-123",
		"CLOUDFLARE_ACCOUNT_ID": "acc-456",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]

	// Resolver-correct prefixed names only — matches what the deployer injects
	// in prod. Bare names are deliberately not emitted (used to be, caused
	// dev/prod divergence — see ai-gateway-astro-server changelog).
	if envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY") != "cf-key-123" {
		t.Errorf("CLOUDFLARE_AI_API_KEY = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_AI_API_KEY"), "cf-key-123")
	}
	if envVal(agent.Environment, "CLOUDFLARE_ACCOUNT_ID") != "acc-456" {
		t.Errorf("CLOUDFLARE_ACCOUNT_ID = %q, want %q", envVal(agent.Environment, "CLOUDFLARE_ACCOUNT_ID"), "acc-456")
	}
	if _, ok := agent.Environment["AI_API_KEY"]; ok {
		t.Error("bare AI_API_KEY must not be emitted; only the resolver-correct CLOUDFLARE_AI_API_KEY")
	}
	if _, ok := agent.Environment["ACCOUNT_ID"]; ok {
		t.Error("bare ACCOUNT_ID must not be emitted; only the resolver-correct CLOUDFLARE_ACCOUNT_ID")
	}
}

func TestBuildProject_CustomLabels(t *testing.T) {
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Agent: spec.Container{Image: "agent:latest"},
		Ingestion: map[string]spec.Ingestion{
			"run": {
				Container: spec.ContainerConfig{
					Build: &spec.BuildConfig{Context: ".", Dockerfile: "Dockerfile"},
				},
				Trigger: spec.IngestionTrigger{Type: "startup"},
			},
		},
	}

	project, err := BuildProject(s, "/work", nil)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	requiredKeys := []string{
		api.ProjectLabel,
		api.ServiceLabel,
		api.VersionLabel,
		api.WorkingDirLabel,
		api.OneoffLabel,
	}

	for name, svc := range project.Services {
		for _, key := range requiredKeys {
			if _, ok := svc.CustomLabels[key]; !ok {
				t.Errorf("service %q missing required CustomLabel %q", name, key)
			}
		}
		if svc.CustomLabels[api.ProjectLabel] != project.Name {
			t.Errorf("service %q: ProjectLabel = %q, want %q", name, svc.CustomLabels[api.ProjectLabel], project.Name)
		}
		if svc.CustomLabels[api.ServiceLabel] != name {
			t.Errorf("service %q: ServiceLabel = %q, want %q", name, svc.CustomLabels[api.ServiceLabel], name)
		}
		if svc.CustomLabels[api.WorkingDirLabel] != project.WorkingDir {
			t.Errorf("service %q: WorkingDirLabel = %q, want %q", name, svc.CustomLabels[api.WorkingDirLabel], project.WorkingDir)
		}
		if svc.CustomLabels[api.OneoffLabel] != "False" {
			t.Errorf("service %q: OneoffLabel = %q, want %q", name, svc.CustomLabels[api.OneoffLabel], "False")
		}
	}
}

func TestBuildProject_ResolverCredentialNames(t *testing.T) {
	// Single-entry cloud provider → bare ANTHROPIC_API_KEY (§8.1). Matches
	// what the deployer injects in prod — dev and prod use the same
	// env-var names so agent code is portable.
	s := &spec.AstroSpec{
		Name:  "my-agent",
		Meta:  spec.Meta{},
		Agent: spec.Container{Image: "agent:latest"},
		Models: map[string]spec.Model{
			"fallback": {Provider: "anthropic"},
		},
	}

	envVars := map[string]string{
		"ANTHROPIC_API_KEY": "sk-anthropic",
	}

	project, err := BuildProject(s, "/work", envVars)
	if err != nil {
		t.Fatalf("BuildProject() error = %v", err)
	}

	agent := project.Services["agent"]
	if envVal(agent.Environment, "ANTHROPIC_API_KEY") != "sk-anthropic" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", envVal(agent.Environment, "ANTHROPIC_API_KEY"), "sk-anthropic")
	}
	// Old <NAME>_<SUFFIX> convention should NOT appear — the resolver names by
	// provider, with entry-name qualification only when multiple entries share
	// a provider.
	if _, ok := agent.Environment["FALLBACK_API_KEY"]; ok {
		t.Error("FALLBACK_API_KEY (old per-entry convention) must not appear; resolver names by provider")
	}
}

func TestBuildProject_AgentCoreRuntime(t *testing.T) {
	newSpec := func() *spec.AstroSpec {
		return &spec.AstroSpec{
			Name:  "my-agent",
			Agent: spec.Container{Image: "agent:latest", Annotations: map[string]string{"runtime": "agentcore"}},
			Dev: &spec.Dev{
				Interfaces: &spec.DevInterfaces{
					Messaging: &spec.DevMessaging{Adapters: []string{"web"}},
				},
			},
		}
	}

	// AgentCore dev: the agent serves /invocations; messaging invokes it by the
	// compose service DNS name over HTTP (topology inverted vs the gRPC default).
	t.Run("agentcore flips transport", func(t *testing.T) {
		project, err := BuildProject(newSpec(), "/work", nil)
		if err != nil {
			t.Fatalf("BuildProject() error = %v", err)
		}
		agent := project.Services["agent"]
		if envVal(agent.Environment, "ASTRO_RUNTIME") != "agentcore" {
			t.Errorf("agent ASTRO_RUNTIME = %q, want agentcore", envVal(agent.Environment, "ASTRO_RUNTIME"))
		}
		if got := envVal(agent.Environment, "GRPC_SERVER_ADDR"); got != "" {
			t.Errorf("agent GRPC_SERVER_ADDR = %q, want empty (agent serves, not dials)", got)
		}
		messaging := project.Services["astro-messaging"]
		if envVal(messaging.Environment, "AGENT_TRANSPORT") != "agentcore" {
			t.Errorf("messaging AGENT_TRANSPORT = %q, want agentcore", envVal(messaging.Environment, "AGENT_TRANSPORT"))
		}
		if got := envVal(messaging.Environment, "AGENT_RUNTIME_ENDPOINT"); got != "http://agent:8080" {
			t.Errorf("messaging AGENT_RUNTIME_ENDPOINT = %q, want http://agent:8080", got)
		}
	})

	// Default (eks) is unchanged: agent dials the messaging gRPC server.
	t.Run("eks default unchanged", func(t *testing.T) {
		s := newSpec()
		s.Agent.Annotations = nil // no runtime annotation -> default (eks)
		project, err := BuildProject(s, "/work", nil)
		if err != nil {
			t.Fatalf("BuildProject() error = %v", err)
		}
		agent := project.Services["agent"]
		if envVal(agent.Environment, "GRPC_SERVER_ADDR") != "astro-messaging:9090" {
			t.Errorf("agent GRPC_SERVER_ADDR = %q, want astro-messaging:9090", envVal(agent.Environment, "GRPC_SERVER_ADDR"))
		}
		if got := envVal(agent.Environment, "ASTRO_RUNTIME"); got != "" {
			t.Errorf("agent ASTRO_RUNTIME = %q, want empty for eks", got)
		}
		if got := envVal(project.Services["astro-messaging"].Environment, "AGENT_TRANSPORT"); got != "" {
			t.Errorf("messaging AGENT_TRANSPORT = %q, want empty for eks", got)
		}
	})
}

// neo4j serves bolt on 7687 and its browser on 7474, so an agent handed the
// default port has nothing to connect to.
func TestBuildProject_KnowledgePortIsTheOneAgentsConnectOn(t *testing.T) {
	tests := []struct {
		provider string
		hostKey  string
		portKey  string
		wantPort string
	}{
		{provider: "neo4j", hostKey: "NEO4J_HOST", portKey: "NEO4J_PORT", wantPort: "7687"},
		{provider: "qdrant", hostKey: "QDRANT_HOST", portKey: "QDRANT_PORT", wantPort: "6333"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			s := &spec.AstroSpec{
				Name:      "graph-agent",
				Agent:     spec.Container{Image: "agent:latest"},
				Knowledge: map[string]spec.Knowledge{"graph": {Provider: tt.provider}},
			}

			project, err := BuildProject(s, "/work", nil)
			require.NoError(t, err)

			agent := project.Services["agent"]
			assert.Equal(t, "knowledge-graph", envVal(agent.Environment, tt.hostKey))
			assert.Equal(t, tt.wantPort, envVal(agent.Environment, tt.portKey))
		})
	}
}

func TestInvalidWatchEntry(t *testing.T) {
	accepted := []string{"agent", "src", "packages/core", "agent/", "packages/core/", "a/b/c", "my-dir_1.x"}
	for _, entry := range accepted {
		t.Run("accepts "+entry, func(t *testing.T) {
			assert.Empty(t, InvalidWatchEntry(entry),
				"a relative slash-separated path is the whole accepted form")
		})
	}

	rejected := []struct {
		entry  string
		reason string
	}{
		{"", "an empty entry names no directory"},
		{`packages\core`, `a backslash is not a path separator here, use /`},
		{`a\b/c`, `a backslash is not a path separator here, use /`},
		{"C:/x", "a drive letter cannot be mounted"},
		{"/etc", "an absolute path would mount something outside the project"},
		{"/", "an absolute path would mount something outside the project"},
		{".", "the project root would hide everything the image built"},
		{"./", "the project root would hide everything the image built"},
		{"./agent", "a . path component is not allowed, name the directory directly"},
		{"a/./b", "a . path component is not allowed, name the directory directly"},
		{"..", "the path escapes the project"},
		{"../sibling", "the path escapes the project"},
		{"a/../../b", "the path escapes the project"},
		{"a//b", "an empty path component names no directory"},
	}
	for _, tt := range rejected {
		t.Run("rejects "+tt.entry, func(t *testing.T) {
			assert.Equal(t, tt.reason, InvalidWatchEntry(tt.entry),
				"the reason is what the author reads, so it has to name the actual rule")
		})
	}
}

func TestPlanWatchDirs_ReportsAnUnresolvablePathAndKeepsGoing(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(work, "agent"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(work, "afile"), []byte("x"), 0o644))
	symlinkOrSkip(t, filepath.Join(work, "loop"), filepath.Join(work, "loop"))

	for _, entry := range []string{"afile/sub", "loop"} {
		t.Run(entry, func(t *testing.T) {
			plan, err := PlanWatchDirs(&spec.Dev{Watch: []string{entry, "agent"}}, work)

			require.NoError(t, err,
				"a mistake in dev-only config must not stop the compose project being built")
			assert.Equal(t, []string{"agent"}, plan.Mount,
				"the entries after an unresolvable one must still be planned")

			// Which bucket it lands in is the OS's call: Windows reports a file
			// mid-path as ERROR_PATH_NOT_FOUND, which is absence, where Unix
			// reports ENOTDIR, which is not. Being reported at all is the
			// guarantee worth pinning.
			reported := slices.Contains(plan.Missing, entry)
			for _, r := range plan.Rejected {
				reported = reported || r.Dir == entry
			}
			assert.True(t, reported,
				"an unresolvable entry must be reported rather than dropped in silence")
		})
	}
}
