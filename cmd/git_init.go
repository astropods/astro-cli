package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// runGit runs git in dir and returns its first line of output on failure, which
// is the part worth showing the user.
func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = err.Error()
		}
		return firstLine(detail), err
	}
	return strings.TrimSpace(stdout.String()), nil
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

// initGitRepo makes the generated project a git repository whose first commit
// is the scaffold, so a coding agent's later work shows up as a diff against
// it. Nothing here is fatal: a missing git or an unset commit identity must
// not cost the user the project that was just written.
func initGitRepo(ctx context.Context, w io.Writer, targetDir string) {
	dim := lipgloss.NewStyle().Faint(true)
	note := func(s string) {
		fmt.Fprintln(w, dim.Render(s)) //nolint:errcheck,gosec
	}

	if _, err := exec.LookPath("git"); err != nil {
		note(msgGitInitSkipped(msgGitNotInstalled()))
		return
	}
	// A nested repository inside an existing checkout is never what the user
	// wants, and the outer repo would not track it.
	if out, err := runGit(ctx, targetDir, "rev-parse", "--is-inside-work-tree"); err == nil && out == "true" {
		note(msgGitInitSkipped(msgGitAlreadyInRepo()))
		return
	}

	for _, args := range [][]string{{"init"}, {"add", "."}} {
		if detail, err := runGit(ctx, targetDir, args...); err != nil {
			note(msgGitInitSkipped(detail))
			return
		}
	}
	if detail, err := runGit(ctx, targetDir, "commit", "-m", msgInitialCommitSubject()); err != nil {
		note(msgGitCommitSkipped(detail))
		return
	}
	note(msgGitRepoInitialized())
}
