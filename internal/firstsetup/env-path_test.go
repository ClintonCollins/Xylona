package firstsetup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveEnvPath(t *testing.T) {
	t.Parallel()

	t.Run("uses cwd env when it exists", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		executableDir := t.TempDir()
		envPath := filepath.Join(cwd, ".env")
		errWrite := os.WriteFile(envPath, []byte("FOO=1\n"), 0o600)
		if errWrite != nil {
			t.Fatalf("WriteFile() error = %v", errWrite)
		}

		got, errResolve := ResolveEnvPath(cwd, executableDir, filepath.Join(cwd, "data.sqlite"))
		if errResolve != nil {
			t.Fatalf("ResolveEnvPath() error = %v", errResolve)
		}
		if got != envPath {
			t.Fatalf("ResolveEnvPath() = %q, want %q", got, envPath)
		}
	})

	t.Run("uses executable-dir env when cwd has none", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		executableDir := t.TempDir()
		envPath := filepath.Join(executableDir, ".env")
		errWrite := os.WriteFile(envPath, []byte("FOO=1\n"), 0o600)
		if errWrite != nil {
			t.Fatalf("WriteFile() error = %v", errWrite)
		}

		got, errResolve := ResolveEnvPath(cwd, executableDir, filepath.Join(cwd, "data.sqlite"))
		if errResolve != nil {
			t.Fatalf("ResolveEnvPath() error = %v", errResolve)
		}
		if got != envPath {
			t.Fatalf("ResolveEnvPath() = %q, want %q", got, envPath)
		}
	})

	t.Run("creates env beside the database when neither env file exists", func(t *testing.T) {
		t.Parallel()
		cwd := t.TempDir()
		executableDir := t.TempDir()
		dbDir := filepath.Join(cwd, "data")
		errMkdir := os.Mkdir(dbDir, 0o700)
		if errMkdir != nil {
			t.Fatalf("Mkdir() error = %v", errMkdir)
		}
		dbPath := filepath.Join(dbDir, "data.sqlite")
		want := filepath.Join(dbDir, ".env")

		got, errResolve := ResolveEnvPath(cwd, executableDir, dbPath)
		if errResolve != nil {
			t.Fatalf("ResolveEnvPath() error = %v", errResolve)
		}
		if got != want {
			t.Fatalf("ResolveEnvPath() = %q, want %q", got, want)
		}
	})
}

func TestLoadCurrentSecrets(t *testing.T) {
	envDir := t.TempDir()
	envPath := filepath.Join(envDir, ".env")
	errWrite := os.WriteFile(envPath, []byte(
		"COOKIE_HASH_KEY_BASE64=from-file-hash\n"+
			"COOKIE_BLOCK_KEY_BASE64=from-file-block\n"+
			"ENCRYPTION_KEY_BASE64=from-file-enc\n",
	), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile() error = %v", errWrite)
	}

	t.Setenv("COOKIE_HASH_KEY_BASE64", "from-env-hash")
	t.Setenv("COOKIE_BLOCK_KEY_BASE64", "")
	t.Setenv("ENCRYPTION_KEY_BASE64", "")

	secrets, errLoad := LoadCurrentSecrets(envPath)
	if errLoad != nil {
		t.Fatalf("LoadCurrentSecrets() error = %v", errLoad)
	}
	if secrets.CookieHashKey != "from-env-hash" {
		t.Fatalf("CookieHashKey = %q, want process env to win", secrets.CookieHashKey)
	}
	if secrets.CookieBlockKey != "from-file-block" {
		t.Fatalf("CookieBlockKey = %q, want file fallback", secrets.CookieBlockKey)
	}
	if secrets.EncryptionKey != "from-file-enc" {
		t.Fatalf("EncryptionKey = %q, want file fallback", secrets.EncryptionKey)
	}
}

func TestLoadCurrentSecretsRejectsMalformedEnvFile(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	errWrite := os.WriteFile(envPath, []byte("MALFORMED='unterminated\n"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile() error = %v", errWrite)
	}

	_, errLoad := LoadCurrentSecrets(envPath)
	if errLoad == nil {
		t.Fatal("LoadCurrentSecrets() error = nil, want malformed env error")
	}
}
