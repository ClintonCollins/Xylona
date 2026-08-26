package firstsetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
)

// ResolveEnvPath picks the .env file first-run setup should read and write.
func ResolveEnvPath(cwd string, executableDir string, dbPath string) (string, error) {
	cwdEnv := filepath.Join(cwd, ".env")
	exists, errExists := fileExists(cwdEnv)
	if errExists != nil {
		return "", fmt.Errorf("firstsetup: inspect working-directory env file: %w", errExists)
	}
	if exists {
		return cwdEnv, nil
	}

	cleanedCWD := filepath.Clean(cwd)
	cleanedExecutable := filepath.Clean(executableDir)
	if strings.TrimSpace(executableDir) != "" && cleanedExecutable != cleanedCWD {
		executableEnv := filepath.Join(executableDir, ".env")
		executableExists, errExecutableExists := fileExists(executableEnv)
		if errExecutableExists != nil {
			return "", fmt.Errorf("firstsetup: inspect executable-directory env file: %w", errExecutableExists)
		}
		if executableExists {
			return executableEnv, nil
		}
	}

	dbDir := filepath.Dir(dbPath)
	if strings.TrimSpace(dbDir) == "" {
		dbDir = cwd
	}
	return filepath.Join(dbDir, ".env"), nil
}

// LoadCurrentSecrets reads cookie and encryption keys from the process
// environment, falling back to values in envPath.
func LoadCurrentSecrets(envPath string) (Secrets, error) {
	fileValues := map[string]string{}
	if strings.TrimSpace(envPath) != "" {
		parsed, errRead := godotenv.Read(envPath)
		if errRead != nil && !os.IsNotExist(errRead) {
			return Secrets{}, fmt.Errorf("firstsetup: read env file: %w", errRead)
		}
		if errRead == nil {
			fileValues = parsed
		}
	}

	return Secrets{
		CookieHashKey:  firstNonEmpty(os.Getenv(envCookieHashKey), fileValues[envCookieHashKey]),
		CookieBlockKey: firstNonEmpty(os.Getenv(envCookieBlockKey), fileValues[envCookieBlockKey]),
		EncryptionKey:  firstNonEmpty(os.Getenv(envEncryptionKey), fileValues[envEncryptionKey]),
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
