package utils

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
)

const DefaultEnvFile = ".env"

var ErrEnvFileNotFound = errors.New("env file not found")

func ResolveEnvPath(workingDir, envFile string) string {
	if filepath.IsAbs(envFile) {
		return envFile
	}
	return filepath.Join(workingDir, envFile)
}

func LoadEnvFile(workingDir, envFile string, required bool) (map[string]string, error) {
	path := ResolveEnvPath(workingDir, envFile)
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		if required {
			return nil, ErrEnvFileNotFound
		}
		return nil, nil
	}
	return godotenv.Read(path)
}
