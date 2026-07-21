package riverqueue

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/riverqueue/river/rivertype"

	"github.com/astropods/astro/apps/astro-server/internal/deploymentstore"
)

// ---------------------------------------------------------------------------
// statusOrNil
// ---------------------------------------------------------------------------

func TestStatusOrNil_NilDeployment(t *testing.T) {
	got := statusOrNil(nil)
	if got != "<nil>" {
		t.Errorf("statusOrNil(nil) = %q, want %q", got, "<nil>")
	}
}

func TestStatusOrNil_NonNilDeployment(t *testing.T) {
	dep := &deploymentstore.Deployment{Status: "active"}
	got := statusOrNil(dep)
	if got != "active" {
		t.Errorf("statusOrNil(dep) = %q, want %q", got, "active")
	}
}

// ---------------------------------------------------------------------------
// Job kind registry
// ---------------------------------------------------------------------------

type duplicateJobArgsA struct{}

func (duplicateJobArgsA) Kind() string { return "__test.duplicate" }

type duplicateJobArgsB struct{}

func (duplicateJobArgsB) Kind() string { return "__test.duplicate" }

func TestRegisteredJobKinds_AvailableWithoutWorkers(t *testing.T) {
	infos := RegisteredJobKinds()
	if len(infos) == 0 {
		t.Fatal("RegisteredJobKinds returned no jobs")
	}

	seen := make(map[string]JobKindInfo, len(infos))
	for i, info := range infos {
		if i > 0 && info.Kind < infos[i-1].Kind {
			t.Fatalf("RegisteredJobKinds not sorted: %q before %q", info.Kind, infos[i-1].Kind)
		}
		if !json.Valid(info.ArgsSchema) {
			t.Fatalf("args schema for %q is not valid JSON: %s", info.Kind, info.ArgsSchema)
		}
		seen[info.Kind] = info
	}

	for _, kind := range []string{
		"deploy",
		"github_build",
		"metering.heartbeat",
		"wakeup",
	} {
		if _, ok := seen[kind]; !ok {
			t.Errorf("RegisteredJobKinds missing %q", kind)
		}
	}
}

func TestRegisterJobKind_DuplicateDoesNotPanic(t *testing.T) {
	originalRegistry := kindRegistry
	originalDuplicates := duplicateJobKinds
	t.Cleanup(func() {
		kindRegistry = originalRegistry
		duplicateJobKinds = originalDuplicates
	})
	kindRegistry = map[string]kindEntry{}
	duplicateJobKinds = map[string]int{}

	registerJobKind[duplicateJobArgsA]()
	registerJobKind[duplicateJobArgsB]()

	if len(kindRegistry) != 1 {
		t.Fatalf("registered kinds = %d, want 1", len(kindRegistry))
	}
	if duplicateJobKinds["__test.duplicate"] != 1 {
		t.Fatalf("duplicate count = %d, want 1", duplicateJobKinds["__test.duplicate"])
	}
}

func TestJobArgsKindTypesAreRegistered(t *testing.T) {
	kindTypes, registeredTypes := parseJobKindRegistrations(t)

	for typeName := range kindTypes {
		if !registeredTypes[typeName] {
			t.Errorf("%s has Kind() but is missing registerJobKind[%s]()", typeName, typeName)
		}
	}
	for typeName := range registeredTypes {
		if !kindTypes[typeName] {
			t.Errorf("registerJobKind[%s]() has no matching Kind() method", typeName)
		}
	}
}

func parseJobKindRegistrations(t *testing.T) (map[string]bool, map[string]bool) {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}

	kindTypes := map[string]bool{}
	registeredTypes := map[string]bool{}
	fset := token.NewFileSet()

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}

		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != "Kind" || fn.Recv == nil || len(fn.Recv.List) == 0 {
				continue
			}
			if typeName := receiverTypeName(fn.Recv.List[0].Type); strings.HasSuffix(typeName, "Args") {
				kindTypes[typeName] = true
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if typeName, ok := registerJobKindTypeName(call.Fun); ok {
				registeredTypes[typeName] = true
			}
			return true
		})
	}

	return kindTypes, registeredTypes
}

func receiverTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return receiverTypeName(e.X)
	default:
		return ""
	}
}

func registerJobKindTypeName(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.IndexExpr:
		if ident, ok := e.X.(*ast.Ident); ok && ident.Name == "registerJobKind" {
			return exprTypeName(e.Index), true
		}
	case *ast.IndexListExpr:
		if ident, ok := e.X.(*ast.Ident); ok && ident.Name == "registerJobKind" && len(e.Indices) == 1 {
			return exprTypeName(e.Indices[0]), true
		}
	}
	return "", false
}

func exprTypeName(expr ast.Expr) string {
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

// ---------------------------------------------------------------------------
// DeployArgs
// ---------------------------------------------------------------------------

func TestDeployArgs_Kind(t *testing.T) {
	args := DeployArgs{DeploymentID: "dep-123"}
	if kind := args.Kind(); kind != "deploy" {
		t.Errorf("DeployArgs.Kind() = %q, want %q", kind, "deploy")
	}
}

func TestDeployArgs_InsertOpts(t *testing.T) {
	opts := DeployArgs{}.InsertOpts()

	if opts.Queue != queueDeploy {
		t.Errorf("Queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
	if !opts.UniqueOpts.ByArgs {
		t.Error("UniqueOpts.ByArgs should be true")
	}

	// ByState excludes completed/discarded so re-apply and reconciler
	// re-enqueue can create new jobs after the original finishes.
	wantStates := map[rivertype.JobState]bool{
		rivertype.JobStateAvailable: true,
		rivertype.JobStatePending:   true,
		rivertype.JobStateRunning:   true,
		rivertype.JobStateRetryable: true,
		rivertype.JobStateScheduled: true,
	}
	if len(opts.UniqueOpts.ByState) != len(wantStates) {
		t.Fatalf("ByState length = %d, want %d", len(opts.UniqueOpts.ByState), len(wantStates))
	}
	for _, s := range opts.UniqueOpts.ByState {
		if !wantStates[s] {
			t.Errorf("unexpected state in ByState: %v", s)
		}
	}
}

// ---------------------------------------------------------------------------
// UndeployArgs
// ---------------------------------------------------------------------------

func TestUndeployArgs_Kind(t *testing.T) {
	args := UndeployArgs{DeploymentID: "dep-456"}
	if kind := args.Kind(); kind != "undeploy" {
		t.Errorf("UndeployArgs.Kind() = %q, want %q", kind, "undeploy")
	}
}

func TestUndeployArgs_InsertOpts(t *testing.T) {
	opts := UndeployArgs{}.InsertOpts()

	if opts.Queue != queueDeploy {
		t.Errorf("Queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
}

// ---------------------------------------------------------------------------
// WakeUpArgs
// ---------------------------------------------------------------------------

func TestWakeUpArgs_Kind(t *testing.T) {
	args := WakeUpArgs{DeploymentID: "dep-789"}
	if kind := args.Kind(); kind != "wakeup" {
		t.Errorf("WakeUpArgs.Kind() = %q, want %q", kind, "wakeup")
	}
}

func TestWakeUpArgs_InsertOpts(t *testing.T) {
	opts := WakeUpArgs{}.InsertOpts()

	if opts.Queue != queueDeploy {
		t.Errorf("Queue = %q, want %q", opts.Queue, queueDeploy)
	}
	if opts.MaxAttempts != 3 {
		t.Errorf("MaxAttempts = %d, want 3", opts.MaxAttempts)
	}
}
