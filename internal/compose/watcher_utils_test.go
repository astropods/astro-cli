package compose

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	spec "github.com/astropods/astro-spec"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestInvalidWatchEntry(t *testing.T) {
	accepted := []string{"agent", "src", "packages/core", "agent/", "packages/core/", "a/b/c", "my-dir_1.x"}
	for _, entry := range accepted {
		t.Run("accepts "+entry, func(t *testing.T) {
			assert.Empty(t, invalidWatchEntry(entry),
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
			assert.Equal(t, tt.reason, invalidWatchEntry(tt.entry),
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

func TestWatchDirPlan_PrintWarnings(t *testing.T) {
	tests := []struct {
		name string
		plan WatchDirPlan
		want string
	}{
		{
			name: "a plan with nothing to report says nothing",
			plan: WatchDirPlan{Mount: []string{"agent"}},
			want: "",
		},
		{
			name: "a missing directory",
			plan: WatchDirPlan{Missing: []string{"srcc"}},
			want: warned(msgWatchDirMissing("srcc")),
		},
		{
			name: "a rejected entry carries its reason",
			plan: WatchDirPlan{Rejected: []WatchDirRejection{{Dir: "../x", Reason: "the path escapes the project"}}},
			want: warned(msgWatchDirRejected("../x", "the path escapes the project")),
		},
		{
			name: "a duplicate",
			plan: WatchDirPlan{Duplicate: []string{"agent/"}},
			want: warned(msgWatchDirDuplicate("agent/")),
		},
		{
			name: "every bucket, duplicates then rejections then absences",
			plan: WatchDirPlan{
				Duplicate: []string{"agent/"},
				Rejected:  []WatchDirRejection{{Dir: "../x", Reason: "the path escapes the project"}},
				Missing:   []string{"srcc"},
			},
			want: warned(
				msgWatchDirDuplicate("agent/"),
				msgWatchDirRejected("../x", "the path escapes the project"),
				msgWatchDirMissing("srcc"),
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			tt.plan.PrintWarnings(&out)
			assert.Equal(t, tt.want, out.String(),
				"nothing the plan could not mount may be dropped in silence")
		})
	}
}

func TestWatchDirPlan_BindMounts(t *testing.T) {
	work := filepath.Join("some", "project")

	t.Run("a container target is POSIX and a source is native", func(t *testing.T) {
		plan := WatchDirPlan{Mount: []string{"agent", "packages/core"}}

		mounts := plan.BindMounts(work)

		require.Len(t, mounts, 2, "one bind per planned directory, in plan order")
		assert.Equal(t, "/app/agent", mounts[0].Target)
		assert.Equal(t, "/app/packages/core", mounts[1].Target,
			"a nested container path stays slash-separated whatever the host")
		assert.Equal(t, filepath.Join(work, "packages", "core"), mounts[1].Source,
			"the host source is native, so it resolves on the machine running the build")
		for _, m := range mounts {
			assert.Equal(t, types.VolumeTypeBind, m.Type)
			assert.NotContains(t, m.Target, `\`,
				"a host separator in a container path collapses it into one name")
		}
	})

	t.Run("a plan with nothing to mount produces no binds", func(t *testing.T) {
		assert.Empty(t, WatchDirPlan{Missing: []string{"srcc"}}.BindMounts(work))
	})
}
