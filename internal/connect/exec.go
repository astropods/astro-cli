package connect

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"time"

	connectv1 "github.com/astropods/astro/packages/astro-proto/connect/v1"
)

// execShellCommand runs a ShellCommand and returns a CommandResult.
func execShellCommand(cmd *connectv1.ShellCommand) *connectv1.CommandResult {
	shell := cmd.Shell
	if shell == "" {
		if runtime.GOOS == "windows" {
			shell = "cmd"
		} else {
			shell = "/bin/sh"
		}
	}

	ctx := context.Background()
	if cmd.TimeoutSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(cmd.TimeoutSeconds)*time.Second)
		defer cancel()
	}

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"/c", cmd.Command}
	} else {
		args = []string{"-c", cmd.Command}
	}

	c := exec.CommandContext(ctx, shell, args...) //nolint:gosec // shell commands are dispatched by the server
	if cmd.WorkingDir != "" {
		c.Dir = cmd.WorkingDir
	}
	for k, v := range cmd.Env {
		c.Env = append(c.Env, k+"="+v)
	}

	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr

	err := c.Run()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			stderr.WriteString(err.Error())
		}
	}

	return &connectv1.CommandResult{
		CommandID: cmd.CommandID,
		ExitCode:  int32(exitCode),
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
	}
}
