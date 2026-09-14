//go:build !windows

package cmd

import "os"

// dockerEndpointMissing reports whether the default Docker socket is absent,
// which separates "not installed" from "installed but not running".
func dockerEndpointMissing() bool {
	_, err := os.Lstat("/var/run/docker.sock")
	return os.IsNotExist(err)
}
