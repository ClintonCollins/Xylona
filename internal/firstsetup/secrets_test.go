package firstsetup

import (
	"encoding/base64"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/joho/godotenv"

	"github.com/ClintonCollins/Xylona/pkg/xycrypt"
)

func TestEnsureSecrets(t *testing.T) {
	t.Parallel()

	existingHash := encodeKey(t, 64)
	existingBlock := encodeKey(t, 32)
	existingEncryption := encodeKey(t, 32)

	tests := []struct {
		name                  string
		current               Secrets
		createDBFile          bool
		existingEnv           string
		wantErr               error
		wantGenerated         []string
		wantKeepCurrent       bool
		wantEnvPreservedKeys  []string
		wantEnvAbsentOriginal string
		existingEnvMode       fs.FileMode
	}{
		{
			name:          "generates missing cookie and encryption keys when no database exists",
			wantGenerated: []string{"COOKIE_BLOCK_KEY_BASE64", "COOKIE_HASH_KEY_BASE64", "ENCRYPTION_KEY_BASE64"},
		},
		{
			name: "leaves existing keys unchanged and writes nothing",
			current: Secrets{
				CookieHashKey:  existingHash,
				CookieBlockKey: existingBlock,
				EncryptionKey:  existingEncryption,
			},
			wantKeepCurrent: true,
		},
		{
			name: "leaves an existing env file untouched when all keys already exist",
			current: Secrets{
				CookieHashKey:  existingHash,
				CookieBlockKey: existingBlock,
				EncryptionKey:  existingEncryption,
			},
			existingEnv: "COOKIE_HASH_KEY_BASE64=" + existingHash + "\n" +
				"COOKIE_BLOCK_KEY_BASE64=" + existingBlock + "\n" +
				"ENCRYPTION_KEY_BASE64=" + existingEncryption + "\n" +
				"CUSTOM_FLAG=keep-me\n",
			wantKeepCurrent:      true,
			wantEnvPreservedKeys: []string{"COOKIE_HASH_KEY_BASE64", "COOKIE_BLOCK_KEY_BASE64", "ENCRYPTION_KEY_BASE64", "CUSTOM_FLAG"},
			existingEnvMode:      0o400,
		},
		{
			name: "refuses to generate an encryption key when the database file already exists",
			current: Secrets{
				CookieHashKey:  existingHash,
				CookieBlockKey: existingBlock,
			},
			createDBFile: true,
			wantErr:      ErrEncryptionKeyMissingExistingDatabase,
		},
		{
			name: "generates only missing keys and merges them into an existing env file",
			current: Secrets{
				CookieHashKey: existingHash,
			},
			existingEnv: "COOKIE_HASH_KEY_BASE64=" + existingHash + "\nCUSTOM_FLAG=keep-me\n",
			wantGenerated: []string{
				"COOKIE_BLOCK_KEY_BASE64",
				"ENCRYPTION_KEY_BASE64",
			},
			wantEnvPreservedKeys: []string{"COOKIE_HASH_KEY_BASE64", "CUSTOM_FLAG"},
		},
		{
			name:          "replaces blank assignments and preserves unrelated content",
			existingEnv:   "CUSTOM_FLAG=keep-me\nCOOKIE_HASH_KEY_BASE64=\nCOOKIE_BLOCK_KEY_BASE64=\"\"\nENCRYPTION_KEY_BASE64=''\n",
			wantGenerated: []string{"COOKIE_BLOCK_KEY_BASE64", "COOKIE_HASH_KEY_BASE64", "ENCRYPTION_KEY_BASE64"},
			wantEnvPreservedKeys: []string{
				"CUSTOM_FLAG",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tempDir := t.TempDir()
			dbPath := filepath.Join(tempDir, "data.sqlite")
			envPath := filepath.Join(tempDir, ".env")

			if tt.createDBFile {
				errWrite := os.WriteFile(dbPath, []byte("placeholder"), 0o600)
				if errWrite != nil {
					t.Fatalf("WriteFile() database error = %v", errWrite)
				}
			}
			if tt.existingEnv != "" {
				errWrite := os.WriteFile(envPath, []byte(tt.existingEnv), 0o600)
				if errWrite != nil {
					t.Fatalf("WriteFile() env error = %v", errWrite)
				}
				existingEnvMode := tt.existingEnvMode
				if existingEnvMode == 0 {
					existingEnvMode = 0o644
				}
				errChmod := os.Chmod(envPath, existingEnvMode)
				if errChmod != nil {
					t.Fatalf("Chmod() env error = %v", errChmod)
				}
			}

			result, errEnsure := EnsureSecrets(EnsureSecretsInput{
				Current: tt.current,
				DBPath:  dbPath,
				EnvPath: envPath,
			})
			if tt.wantErr != nil {
				if !errors.Is(errEnsure, tt.wantErr) {
					t.Fatalf("EnsureSecrets() error = %v, want %v", errEnsure, tt.wantErr)
				}
				return
			}
			if errEnsure != nil {
				t.Fatalf("EnsureSecrets() error = %v", errEnsure)
			}

			if tt.wantKeepCurrent {
				if result != tt.current {
					t.Fatalf("EnsureSecrets() secrets = %+v, want original %+v", result, tt.current)
				}
				if tt.existingEnv == "" {
					_, errStat := os.Stat(envPath)
					if !errors.Is(errStat, os.ErrNotExist) {
						t.Fatalf("EnsureSecrets() wrote env file, want no file when nothing was generated")
					}
					return
				}
			}

			assertKeySize(t, "COOKIE_HASH_KEY_BASE64", result.CookieHashKey, 64)
			assertKeySize(t, "COOKIE_BLOCK_KEY_BASE64", result.CookieBlockKey, 32)
			assertKeySize(t, "ENCRYPTION_KEY_BASE64", result.EncryptionKey, xycrypt.EncryptionKeySize)

			if tt.current.CookieHashKey != "" && result.CookieHashKey != tt.current.CookieHashKey {
				t.Fatalf("EnsureSecrets() replaced cookie hash key")
			}
			if tt.wantKeepCurrent && tt.existingEnv != "" {
				content, errReadContent := os.ReadFile(envPath)
				if errReadContent != nil {
					t.Fatalf("ReadFile() env error = %v", errReadContent)
				}
				if string(content) != tt.existingEnv {
					t.Fatalf("env content changed = %q, want %q", string(content), tt.existingEnv)
				}
			}

			envMap, errRead := godotenv.Read(envPath)
			if errRead != nil {
				t.Fatalf("godotenv.Read() error = %v", errRead)
			}
			for _, key := range tt.wantGenerated {
				if envMap[key] == "" {
					t.Fatalf("env missing generated key %s", key)
				}
			}
			for _, key := range tt.wantEnvPreservedKeys {
				if envMap[key] == "" {
					t.Fatalf("env dropped existing key %s", key)
				}
			}
			if envMap["CUSTOM_FLAG"] != "" && envMap["CUSTOM_FLAG"] != "keep-me" {
				t.Fatalf("env CUSTOM_FLAG = %q, want keep-me", envMap["CUSTOM_FLAG"])
			}

			if runtime.GOOS == "windows" {
				return
			}
			info, errStat := os.Stat(envPath)
			if errStat != nil {
				t.Fatalf("Stat() env error = %v", errStat)
			}
			wantMode := fs.FileMode(0o600)
			if tt.wantKeepCurrent {
				wantMode = tt.existingEnvMode
			}
			if info.Mode().Perm() != wantMode {
				t.Fatalf("env mode = %v, want %v", info.Mode().Perm(), wantMode)
			}
		})
	}
}

func encodeKey(t *testing.T, size int) string {
	t.Helper()
	buf := make([]byte, size)
	for i := range buf {
		buf[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

func assertKeySize(t *testing.T, name string, encoded string, want int) {
	t.Helper()
	decoded, errDecode := base64.StdEncoding.DecodeString(encoded)
	if errDecode != nil {
		t.Fatalf("%s decode error = %v", name, errDecode)
	}
	if len(decoded) != want {
		t.Fatalf("%s decoded length = %d, want %d", name, len(decoded), want)
	}
}
