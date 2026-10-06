// Package deviceid derives a stable identifier for this machine without
// sending the operating system's own machine id anywhere.
package deviceid

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// domain separates this hash from any other tool hashing the same machine id,
// so the two values cannot be matched.
const domain = "astropods-gateway-device:"

// Source reads the operating system's machine id. Tests replace it.
var Source = osMachineID

// ID returns the device id: a hash of the OS machine id, so reinstalling the
// CLI keeps the same device. Where the OS exposes none, a random id is created
// once and kept in fallbackPath.
func ID(fallbackPath string) (string, error) {
	if raw, err := Source(); err == nil && strings.TrimSpace(raw) != "" {
		return hash(strings.TrimSpace(raw)), nil
	}
	return fallback(fallbackPath)
}

func hash(raw string) string {
	sum := sha256.Sum256([]byte(domain + raw))
	return hex.EncodeToString(sum[:16])
}

func fallback(path string) (string, error) {
	data, err := os.ReadFile(path) //nolint:gosec
	if err == nil && strings.TrimSpace(string(data)) != "" {
		return strings.TrimSpace(string(data)), nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(id+"\n"), 0o600); err != nil {
		return "", err
	}
	return id, nil
}

var ioregUUID = regexp.MustCompile(`"IOPlatformUUID"\s*=\s*"([^"]+)"`)

func osMachineID() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		out, err := exec.Command("ioreg", "-rd1", "-c", "IOPlatformExpertDevice").Output()
		if err != nil {
			return "", err
		}
		m := ioregUUID.FindSubmatch(out)
		if m == nil {
			return "", errors.New("deviceid: no IOPlatformUUID in ioreg output")
		}
		return string(m[1]), nil
	case "linux":
		for _, p := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
			if data, err := os.ReadFile(p); err == nil && strings.TrimSpace(string(data)) != "" { //nolint:gosec
				return string(data), nil
			}
		}
		return "", errors.New("deviceid: no machine-id file")
	case "windows":
		out, err := exec.Command("reg", "query", `HKLM\SOFTWARE\Microsoft\Cryptography`, "/v", "MachineGuid").Output()
		if err != nil {
			return "", err
		}
		fields := strings.Fields(string(out))
		for i, f := range fields {
			if f == "REG_SZ" && i+1 < len(fields) {
				return fields[i+1], nil
			}
		}
		return "", errors.New("deviceid: no MachineGuid in reg output")
	}
	return "", errors.New("deviceid: unsupported OS " + runtime.GOOS)
}
