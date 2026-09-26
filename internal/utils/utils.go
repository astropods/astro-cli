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

func CheckEnvFile(workingDir, envFile string) error {
	fi, err := os.Stat(ResolveEnvPath(workingDir, envFile))
	if err != nil || !fi.Mode().IsRegular() {
		return ErrEnvFileNotFound
	}
	return nil
}

func LoadEnvFile(workingDir, envFile string, required bool) (map[string]string, error) {
	if err := CheckEnvFile(workingDir, envFile); err != nil {
		if required {
			return nil, err
		}
		return nil, nil
	}
	return godotenv.Read(ResolveEnvPath(workingDir, envFile))
}
