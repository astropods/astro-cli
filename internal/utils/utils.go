package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

// DefaultEnvFile is the default name of the environment file (e.g. ".env").
const DefaultEnvFile = ".env"

// ErrEnvFileNotFound reports a named env file that is missing or is not a
// regular file. Callers that received the name from a user should fail on it;
// a caller reading the default .env should carry on without one.
var ErrEnvFileNotFound = errors.New("no env file")

// EnvFilePath resolves an env file name to the path it will be read from: the
// name itself when absolute, otherwise relative to dir.
func EnvFilePath(dir, envFile string) string {
	if filepath.IsAbs(envFile) {
		return envFile
	}
	return filepath.Join(dir, envFile)
}

// ReadEnvFile reads the env file at path. It returns ErrEnvFileNotFound when
// the path is missing or is not a regular file, so a caller can tell an absent
// file from an unparseable one.
func ReadEnvFile(path string) (map[string]string, error) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("%w at %s", ErrEnvFileNotFound, path)
	}
	return godotenv.Read(path)
}

// LoadEnvFile reads the env file named envFile, resolved against workingDir by
// EnvFilePath. An empty envFile names no file at all and returns (nil, nil);
// any other name that does not resolve to a regular file returns
// ErrEnvFileNotFound.
func LoadEnvFile(workingDir, envFile string) (map[string]string, error) {
	if envFile == "" {
		return nil, nil
	}
	return ReadEnvFile(EnvFilePath(workingDir, envFile))
}
