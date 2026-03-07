// Package daemon manages the ast connect background process lifecycle.
package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/astropods/astro/apps/astro-cli/internal/auth"
)

// Paths returns the PID file and log file paths for the daemon.
func Paths(binaryName string) (pidFile, logFile string, err error) {
	dir, err := auth.ConfigDir(binaryName)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(dir, "connect.pid"), filepath.Join(dir, "connect.log"), nil
}

// Start re-execs the current binary as a detached background process.
// It passes --foreground along with any extra flags so the child runs the
// connect loop directly instead of trying to daemonize again.
func Start(binaryName string, extraArgs []string) error {
	pidFile, logFile, err := Paths(binaryName)
	if err != nil {
		return err
	}

	// Check if already running
	if pid, running := ReadPID(pidFile); running {
		return fmt.Errorf("daemon already running (PID %d)", pid)
	}

	// Ensure config dir exists
	if err := os.MkdirAll(filepath.Dir(pidFile), 0700); err != nil {
		return err
	}

	// Open log file
	lf, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600) //nolint:gosec // path from config dir
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	// Build args: connect --foreground [extraArgs...]
	args := []string{"connect", "--foreground"}
	args = append(args, extraArgs...)

	exe, err := os.Executable()
	if err != nil {
		_ = lf.Close()
		return fmt.Errorf("resolve executable: %w", err)
	}

	cmd := exec.Command(exe, args...) //nolint:gosec // re-execing self with known args
	cmd.Stdout = lf
	cmd.Stderr = lf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		_ = lf.Close()
		return fmt.Errorf("start daemon: %w", err)
	}

	// Write PID file
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0600); err != nil {
		// Kill the child if we can't record its PID
		_ = cmd.Process.Kill()
		_ = lf.Close()
		return fmt.Errorf("write PID file: %w", err)
	}

	fmt.Printf("Daemon started (PID %d), logging to %s\n", cmd.Process.Pid, logFile)

	// Detach — don't wait for the child
	_ = cmd.Process.Release()
	_ = lf.Close()
	return nil
}

// Stop sends SIGTERM to the daemon process.
func Stop(binaryName string) error {
	pidFile, _, err := Paths(binaryName)
	if err != nil {
		return err
	}

	pid, running := ReadPID(pidFile)
	if !running {
		return errors.New("daemon is not running")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Clean up PID file
	_ = os.Remove(pidFile)
	fmt.Printf("Daemon stopped (PID %d)\n", pid)
	return nil
}

// Status prints whether the daemon is running.
func Status(binaryName string) error {
	pidFile, logFile, err := Paths(binaryName)
	if err != nil {
		return err
	}

	pid, running := ReadPID(pidFile)
	if running {
		fmt.Printf("Daemon is running (PID %d)\n", pid)
		fmt.Printf("Log file: %s\n", logFile)
	} else {
		fmt.Println("Daemon is not running")
	}
	return nil
}

// ReadPID reads the PID file and checks if the process is alive.
func ReadPID(pidFile string) (int, bool) {
	data, err := os.ReadFile(pidFile) //nolint:gosec
	if err != nil {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, false
	}

	// Check if process is alive (signal 0 doesn't send anything, just checks)
	proc, err := os.FindProcess(pid)
	if err != nil {
		return pid, false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// Process doesn't exist — stale PID file
		_ = os.Remove(pidFile)
		return pid, false
	}

	return pid, true
}
