package utils

import (
	"fmt"
	"os"
)

// EnvFileFlag is a pflag.Value for a flag naming an env file, validating the
// path while flags are parsed rather than partway through a command. Set only
// runs when the user passes the flag, so a default the user never asked for is
// never required to exist.
type EnvFileFlag struct {
	path string
}

// NewEnvFileFlag returns a flag value holding def until the user sets it.
func NewEnvFileFlag(def string) *EnvFileFlag {
	return &EnvFileFlag{path: def}
}

// Set validates that the named file exists, resolved the same way the reader
// resolves it, and records it.
func (f *EnvFileFlag) Set(v string) error {
	// An empty value names no file, which is what LoadEnvFile documents and
	// what a test resetting the flag between cases relies on. A command that
	// needs the file says so itself.
	if v == "" {
		f.path = ""
		return nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("get working directory: %w", err)
	}
	path := EnvFilePath(cwd, v)
	fi, statErr := os.Stat(path)
	if statErr != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%w at %s", ErrEnvFileNotFound, path)
	}
	f.path = v
	return nil
}

// String is the value cobra shows as the flag's default in --help.
func (f *EnvFileFlag) String() string {
	if f == nil {
		return ""
	}
	return f.path
}

// Type must report "string": pflag's GetString rejects a flag whose Value
// reports any other type, which would make every flagString read of these
// flags return "" instead of the path.
func (f *EnvFileFlag) Type() string { return "string" }
