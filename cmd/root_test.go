package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestResolveSpecPathFromCwd(t *testing.T) {
	tests := []struct {
		name    string
		setup   func(t *testing.T) (cmd *cobra.Command, dir string)
		want    string
		wantErr bool
	}{
		{
			name: "explicit -f uses that path",
			setup: func(t *testing.T) (*cobra.Command, string) {
				dir := t.TempDir()
				_ = touch(filepath.Join(dir, "my-spec.yml"))
				chdirInto(t, dir)
				cmd := runCobra(t, []string{"-f", "my-spec.yml", "child"})
				return cmd, dir
			},
			want: "my-spec.yml",
		},
		{
			name: "default finds first alias",
			setup: func(t *testing.T) (*cobra.Command, string) {
				dir := t.TempDir()
				_ = touch(filepath.Join(dir, "astroai.yml"))
				chdirInto(t, dir)
				cmd := runCobra(t, []string{"child"})
				return cmd, dir
			},
			want: "astroai.yml",
		},
		{
			name: "explicit -f absolute path returns as-is",
			setup: func(t *testing.T) (*cobra.Command, string) {
				absPath := filepath.Join(t.TempDir(), "abs-spec.yml")
				_ = touch(absPath)
				cmd := runCobra(t, []string{"-f", absPath, "child"})
				return cmd, absPath
			},
			want: "",
		},
		{
			name: "prefers astropods over astroai over astro",
			setup: func(t *testing.T) (*cobra.Command, string) {
				dir := t.TempDir()
				_ = touch(filepath.Join(dir, "astropods.yml"))
				_ = touch(filepath.Join(dir, "astroai.yml"))
				_ = touch(filepath.Join(dir, "astro.yml"))
				chdirInto(t, dir)
				cmd := runCobra(t, []string{"child"})
				return cmd, dir
			},
			want: "astropods.yml",
		},
		{
			name: "no spec file returns error",
			setup: func(t *testing.T) (*cobra.Command, string) {
				dir := t.TempDir()
				chdirInto(t, dir)
				cmd := runCobra(t, []string{"child"})
				return cmd, dir
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, dir := tt.setup(t)
			got, err := resolveSpecPathFromCwd(cmd)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				if err != nil && len(SpecFileAliases) > 0 && !strings.Contains(err.Error(), SpecFileAliases[0]) {
					t.Errorf("error should mention aliases; got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			wantPath := tt.want
			if wantPath == "" {
				wantPath = dir
			} else {
				wantPath = filepath.Join(dir, tt.want)
			}
			if !pathsEqual(t, got, wantPath) {
				t.Errorf("got %v, want %v", got, wantPath)
			}
		})
	}
}

func chdirInto(t *testing.T, dir string) {
	t.Helper()
	orig, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

func runCobra(t *testing.T, args []string) *cobra.Command {
	t.Helper()
	root := &cobra.Command{Use: "test"}
	root.PersistentFlags().StringP("file", "f", "astropods.yml", "")
	child := &cobra.Command{Use: "child", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(child)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return child
}

func pathsEqual(t *testing.T, got, want string) bool {
	t.Helper()
	gotCanon, _ := filepath.EvalSymlinks(got)
	wantCanon, _ := filepath.EvalSymlinks(want)
	return gotCanon == wantCanon
}

func touch(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	return f.Close()
}
