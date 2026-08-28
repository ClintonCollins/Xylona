package firstsetup

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/joho/godotenv"

	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
)

const (
	cookieHashKeyBytes  = 64
	cookieBlockKeyBytes = 32

	envCookieHashKey  = "COOKIE_HASH_KEY_BASE64"
	envCookieBlockKey = "COOKIE_BLOCK_KEY_BASE64"
	envEncryptionKey  = "ENCRYPTION_KEY_BASE64"
	envHost           = "HOST"
)

// Secrets holds the controller runtime keys used during first-run setup.
type Secrets struct {
	CookieHashKey  string
	CookieBlockKey string
	EncryptionKey  string
}

// EnsureSecretsInput is the current process view of first-run values and paths.
type EnsureSecretsInput struct {
	Current     Secrets
	DBPath      string
	EnvPath     string
	HostDefault string
}

// EnsureSecrets generates missing first-run secrets, persists only new values,
// and leaves existing values unchanged.
func EnsureSecrets(input EnsureSecretsInput) (Secrets, error) {
	secrets := input.Current
	generated := make(map[string]string)
	if strings.TrimSpace(input.HostDefault) != "" {
		generated[envHost] = input.HostDefault
	}

	if strings.TrimSpace(secrets.CookieHashKey) == "" {
		encoded, errGenerate := generateEncodedKey(cookieHashKeyBytes)
		if errGenerate != nil {
			return Secrets{}, errGenerate
		}
		secrets.CookieHashKey = encoded
		generated[envCookieHashKey] = encoded
	}

	if strings.TrimSpace(secrets.CookieBlockKey) == "" {
		encoded, errGenerate := generateEncodedKey(cookieBlockKeyBytes)
		if errGenerate != nil {
			return Secrets{}, errGenerate
		}
		secrets.CookieBlockKey = encoded
		generated[envCookieBlockKey] = encoded
	}

	if strings.TrimSpace(secrets.EncryptionKey) == "" {
		dbExists, errExists := fileExists(input.DBPath)
		if errExists != nil {
			return Secrets{}, errExists
		}
		if dbExists {
			return Secrets{}, ErrEncryptionKeyMissingExistingDatabase
		}
		encoded, errGenerate := generateEncodedKey(xycrypt.EncryptionKeySize)
		if errGenerate != nil {
			return Secrets{}, errGenerate
		}
		secrets.EncryptionKey = encoded
		generated[envEncryptionKey] = encoded
	}

	errPersist := persistGeneratedSecrets(input.EnvPath, generated)
	if errPersist != nil {
		return Secrets{}, errPersist
	}
	return secrets, nil
}

// ApplySecretsToEnv copies secrets into the current process environment so a
// subsequent godotenv.Load does not need to override existing values.
func ApplySecretsToEnv(secrets Secrets) error {
	assignments := [][2]string{
		{envCookieHashKey, secrets.CookieHashKey},
		{envCookieBlockKey, secrets.CookieBlockKey},
		{envEncryptionKey, secrets.EncryptionKey},
	}
	for _, assignment := range assignments {
		errSet := os.Setenv(assignment[0], assignment[1])
		if errSet != nil {
			return fmt.Errorf("firstsetup: set %s: %w", assignment[0], errSet)
		}
	}
	return nil
}

func generateEncodedKey(size int) (string, error) {
	buf := make([]byte, size)
	_, errRead := rand.Read(buf)
	if errRead != nil {
		return "", fmt.Errorf("firstsetup: generate key: %w", errRead)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func fileExists(path string) (bool, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return false, nil
	}
	info, errStat := os.Stat(trimmed)
	if errStat != nil {
		if os.IsNotExist(errStat) {
			return false, nil
		}
		return false, fmt.Errorf("firstsetup: stat path: %w", errStat)
	}
	return !info.IsDir(), nil
}

func persistGeneratedSecrets(envPath string, generated map[string]string) error {
	if len(generated) == 0 {
		return nil
	}

	trimmedPath := strings.TrimSpace(envPath)
	if trimmedPath == "" {
		return ErrEnvPathRequired
	}
	errMkdir := os.MkdirAll(filepath.Dir(trimmedPath), 0o700)
	if errMkdir != nil {
		return fmt.Errorf("firstsetup: create env directory: %w", errMkdir)
	}

	existing := map[string]string{}
	content, errRead := os.ReadFile(trimmedPath)
	if errRead != nil {
		if !os.IsNotExist(errRead) {
			return fmt.Errorf("firstsetup: read env file: %w", errRead)
		}
	} else {
		parsed, errParse := godotenv.Unmarshal(string(content))
		if errParse != nil {
			return fmt.Errorf("firstsetup: parse env file: %w", errParse)
		}
		existing = parsed
	}

	keys := make([]string, 0, len(generated))
	for key := range generated {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	updatedContent := string(content)
	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		existingValue, exists := existing[key]
		if exists {
			if strings.TrimSpace(existingValue) != "" {
				continue
			}
			updatedContent = replaceEnvAssignment(updatedContent, key, generated[key])
			continue
		}
		lines = append(lines, key+"="+generated[key])
	}
	if len(lines) > 0 {
		prefix := ""
		if updatedContent != "" && !strings.HasSuffix(updatedContent, "\n") {
			prefix = "\n"
		}
		updatedContent += prefix + strings.Join(lines, "\n") + "\n"
	}
	if updatedContent == string(content) {
		if os.IsNotExist(errRead) {
			return nil
		}
		errChmod := os.Chmod(trimmedPath, 0o600)
		if errChmod != nil {
			return fmt.Errorf("firstsetup: secure env file permissions: %w", errChmod)
		}
		return nil
	}
	return replaceEnvFile(trimmedPath, []byte(updatedContent))
}

func replaceEnvFile(path string, content []byte) (resultErr error) {
	tempFile, errCreate := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if errCreate != nil {
		return fmt.Errorf("firstsetup: create temporary env file: %w", errCreate)
	}
	tempPath := tempFile.Name()
	defer func() {
		errRemove := os.Remove(tempPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("firstsetup: remove temporary env file: %w", errRemove))
		}
	}()

	errChmod := tempFile.Chmod(0o600)
	if errChmod != nil {
		errClose := tempFile.Close()
		return errors.Join(
			fmt.Errorf("firstsetup: secure temporary env file: %w", errChmod),
			wrapSecretFileError("close temporary env file", errClose),
		)
	}
	_, errWrite := tempFile.Write(content)
	if errWrite != nil {
		errClose := tempFile.Close()
		return errors.Join(
			fmt.Errorf("firstsetup: write temporary env file: %w", errWrite),
			wrapSecretFileError("close temporary env file", errClose),
		)
	}
	errSync := tempFile.Sync()
	errClose := tempFile.Close()
	if errSync != nil || errClose != nil {
		return errors.Join(
			wrapSecretFileError("sync temporary env file", errSync),
			wrapSecretFileError("close temporary env file", errClose),
		)
	}
	errRename := os.Rename(tempPath, path)
	if errRename != nil {
		return fmt.Errorf("firstsetup: replace env file: %w", errRename)
	}
	return nil
}

func wrapSecretFileError(action string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("firstsetup: %s: %w", action, err)
}

func replaceEnvAssignment(content string, key string, value string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lineEnding := ""
		lineWithoutEnding := line
		if strings.HasSuffix(lineWithoutEnding, "\r") {
			lineEnding = "\r"
			lineWithoutEnding = strings.TrimSuffix(lineWithoutEnding, "\r")
		}
		assignment := strings.TrimSpace(lineWithoutEnding)
		withoutExport, hasExport := strings.CutPrefix(assignment, "export ")
		if hasExport {
			assignment = strings.TrimSpace(withoutExport)
		}
		name, _, found := strings.Cut(assignment, "=")
		if found && strings.TrimSpace(name) == key {
			lines[i] = key + "=" + value + lineEnding
		}
	}
	return strings.Join(lines, "\n")
}
