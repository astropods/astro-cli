package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"text/template"
)

// InstallService generates and installs a platform-specific service definition.
func InstallService(binaryName string, extraArgs []string) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable: %w", err)
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(exe, extraArgs)
	case "linux":
		return installSystemd(exe, extraArgs)
	default:
		return fmt.Errorf("unsupported platform: %s (use --daemon instead)", runtime.GOOS)
	}
}

// UninstallService removes the platform-specific service definition.
func UninstallService() error {
	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd()
	case "linux":
		return uninstallSystemd()
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

// --- macOS launchd ---

const launchdLabel = "com.postman.ast-connect"

func launchdPlistPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", launchdLabel+".plist")
}

var launchdTmpl = template.Must(template.New("plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{ .Label }}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{ .Exe }}</string>
        <string>connect</string>
        <string>--foreground</string>
{{- range .ExtraArgs }}
        <string>{{ . }}</string>
{{- end }}
    </array>
    <key>KeepAlive</key>
    <true/>
    <key>RunAtLoad</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{ .LogFile }}</string>
    <key>StandardErrorPath</key>
    <string>{{ .LogFile }}</string>
</dict>
</plist>
`))

func installLaunchd(exe string, extraArgs []string) error {
	plistPath := launchdPlistPath()
	if err := os.MkdirAll(filepath.Dir(plistPath), 0750); err != nil { //nolint:gosec
		return err
	}

	home, _ := os.UserHomeDir()
	logFile := filepath.Join(home, ".ast", "connect.log")

	f, err := os.Create(plistPath) //nolint:gosec // path is derived from user home dir
	if err != nil {
		return fmt.Errorf("create plist: %w", err)
	}
	defer f.Close() //nolint:errcheck

	if err := launchdTmpl.Execute(f, struct {
		Label     string
		Exe       string
		ExtraArgs []string
		LogFile   string
	}{
		Label:     launchdLabel,
		Exe:       exe,
		ExtraArgs: extraArgs,
		LogFile:   logFile,
	}); err != nil {
		return fmt.Errorf("write plist: %w", err)
	}

	fmt.Printf("Service installed: %s\n", plistPath)
	fmt.Printf("Start with: launchctl load %s\n", plistPath)
	fmt.Printf("Stop with:  launchctl unload %s\n", plistPath)
	return nil
}

func uninstallLaunchd() error {
	plistPath := launchdPlistPath()
	if _, err := os.Stat(plistPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed (no %s)", plistPath)
	}
	if err := os.Remove(plistPath); err != nil {
		return err
	}
	fmt.Printf("Service uninstalled: %s\n", plistPath)
	fmt.Println("Run 'launchctl unload' first if the service is currently loaded.")
	return nil
}

// --- Linux systemd ---

func systemdUnitPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "systemd", "user", "ast-connect.service")
}

var systemdTmpl = template.Must(template.New("unit").Parse(`[Unit]
Description=Astro Connect Daemon
After=network.target

[Service]
ExecStart={{ .Exe }} connect --foreground{{ range .ExtraArgs }} {{ . }}{{ end }}
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
`))

func installSystemd(exe string, extraArgs []string) error {
	unitPath := systemdUnitPath()
	if err := os.MkdirAll(filepath.Dir(unitPath), 0750); err != nil { //nolint:gosec
		return err
	}

	f, err := os.Create(unitPath) //nolint:gosec // path is derived from user home dir
	if err != nil {
		return fmt.Errorf("create unit: %w", err)
	}
	defer f.Close() //nolint:errcheck

	if err := systemdTmpl.Execute(f, struct {
		Exe       string
		ExtraArgs []string
	}{
		Exe:       exe,
		ExtraArgs: extraArgs,
	}); err != nil {
		return fmt.Errorf("write unit: %w", err)
	}

	fmt.Printf("Service installed: %s\n", unitPath)
	fmt.Println("Enable with: systemctl --user enable ast-connect")
	fmt.Println("Start with:  systemctl --user start ast-connect")
	fmt.Println("Stop with:   systemctl --user stop ast-connect")
	return nil
}

func uninstallSystemd() error {
	unitPath := systemdUnitPath()
	if _, err := os.Stat(unitPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed (no %s)", unitPath)
	}
	if err := os.Remove(unitPath); err != nil {
		return err
	}
	fmt.Printf("Service uninstalled: %s\n", unitPath)
	fmt.Println("Run 'systemctl --user disable ast-connect' first if the service is currently enabled.")
	return nil
}
