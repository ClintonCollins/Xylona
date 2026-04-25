package supervisor

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestSetupCmdSetsExplicitChildEnvironment(t *testing.T) {
	inst, errNew := New(context.Background())
	if errNew != nil {
		t.Fatalf("failed to create supervisor instance: %v", errNew)
	}

	preparedCommand := PreparedCommand{
		ID:               "env-explicit-test",
		BaseCommand:      "echo",
		Args:             []string{"hello"},
		WorkingDirectory: t.TempDir(),
		Status:           xylona.Status_ONLINE,
	}
	newCommand := inst.initNewCommand(preparedCommand, nil)

	cmd, errSetup := inst.setupCmd(newCommand, preparedCommand)
	if errSetup != nil {
		t.Fatalf("setupCmd() error = %v", errSetup)
	}
	if cmd.Env == nil {
		t.Fatal("setupCmd() left cmd.Env nil, want explicit child environment")
	}
}

func TestSetupCmdFiltersSecretsFromChildEnvironment(t *testing.T) {
	tmpRoot := t.TempDir()

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_GB.UTF-8")
	t.Setenv("HOME", "/home/clinton")
	t.Setenv("TMPDIR", tmpRoot)
	t.Setenv("USERPROFILE", `C:\Users\clinton`)
	t.Setenv("APPDATA", `C:\Users\clinton\AppData\Roaming`)
	t.Setenv("LOCALAPPDATA", `C:\Users\clinton\AppData\Local`)
	t.Setenv("TEMP", `C:\Users\clinton\AppData\Local\Temp`)
	t.Setenv("TMP", `C:\Users\clinton\AppData\Local\Temp`)
	t.Setenv("SYSTEMROOT", `C:\Windows`)
	t.Setenv("WINDIR", `C:\Windows`)
	t.Setenv("COMSPEC", `C:\Windows\System32\cmd.exe`)
	t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
	t.Setenv("JWT_SECRET_KEY_BASE64", "jwt-secret")
	t.Setenv("ENCRYPTION_KEY_BASE64", "encryption-secret")
	t.Setenv("DB_FILE_PATH", filepath.Join(tmpRoot, "data.sqlite"))
	t.Setenv("COOKIE_HASH_KEY_BASE64", "cookie-hash-secret")
	t.Setenv("COOKIE_BLOCK_KEY_BASE64", "cookie-block-secret")

	inst, errNew := New(context.Background())
	if errNew != nil {
		t.Fatalf("failed to create supervisor instance: %v", errNew)
	}

	preparedCommand := PreparedCommand{
		ID:               "env-filter-test",
		BaseCommand:      "echo",
		Args:             []string{"hello"},
		WorkingDirectory: t.TempDir(),
		Status:           xylona.Status_ONLINE,
	}
	newCommand := inst.initNewCommand(preparedCommand, nil)

	cmd, errSetup := inst.setupCmd(newCommand, preparedCommand)
	if errSetup != nil {
		t.Fatalf("setupCmd() error = %v", errSetup)
	}

	gotEnv := envMap(cmd.Env)
	for _, secretKey := range []string{
		"JWT_SECRET_KEY_BASE64",
		"ENCRYPTION_KEY_BASE64",
		"DB_FILE_PATH",
		"COOKIE_HASH_KEY_BASE64",
		"COOKIE_BLOCK_KEY_BASE64",
	} {
		if _, ok := gotEnv[secretKey]; ok {
			t.Fatalf("child environment unexpectedly contains %s", secretKey)
		}
	}
}

func TestSetupCmdKeepsAllowedEnvironmentEntries(t *testing.T) {
	tmpRoot := t.TempDir()

	t.Setenv("PATH", "/usr/bin")
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_GB.UTF-8")

	switch CurrentRuntime {
	case RuntimeWindows:
		t.Setenv("SYSTEMROOT", `C:\Windows`)
		t.Setenv("WINDIR", `C:\Windows`)
		t.Setenv("COMSPEC", `C:\Windows\System32\cmd.exe`)
		t.Setenv("PATHEXT", ".COM;.EXE;.BAT;.CMD")
		t.Setenv("USERPROFILE", `C:\Users\clinton`)
		t.Setenv("APPDATA", `C:\Users\clinton\AppData\Roaming`)
		t.Setenv("LOCALAPPDATA", `C:\Users\clinton\AppData\Local`)
		t.Setenv("TEMP", `C:\Users\clinton\AppData\Local\Temp`)
		t.Setenv("TMP", `C:\Users\clinton\AppData\Local\Temp`)
	case RuntimeLinux, RuntimeDarwin, RuntimeUnknown:
		t.Setenv("HOME", "/home/clinton")
		t.Setenv("TMPDIR", tmpRoot)
	}

	inst, errNew := New(context.Background())
	if errNew != nil {
		t.Fatalf("failed to create supervisor instance: %v", errNew)
	}

	preparedCommand := PreparedCommand{
		ID:               "env-allowlist-test",
		BaseCommand:      "echo",
		Args:             []string{"hello"},
		WorkingDirectory: t.TempDir(),
		Status:           xylona.Status_ONLINE,
	}
	newCommand := inst.initNewCommand(preparedCommand, nil)

	cmd, errSetup := inst.setupCmd(newCommand, preparedCommand)
	if errSetup != nil {
		t.Fatalf("setupCmd() error = %v", errSetup)
	}

	gotEnv := envMap(cmd.Env)
	for _, requiredKey := range []string{"PATH", "LANG", "LC_ALL"} {
		if _, ok := gotEnv[requiredKey]; !ok {
			t.Fatalf("child environment missing required key %s", requiredKey)
		}
	}

	switch CurrentRuntime {
	case RuntimeWindows:
		for _, requiredKey := range []string{
			"SYSTEMROOT",
			"WINDIR",
			"COMSPEC",
			"PATHEXT",
			"USERPROFILE",
			"APPDATA",
			"LOCALAPPDATA",
			"TEMP",
			"TMP",
		} {
			if _, ok := gotEnv[requiredKey]; !ok {
				t.Fatalf("child environment missing required Windows key %s", requiredKey)
			}
		}
	default:
		for _, requiredKey := range []string{"HOME", "TMPDIR"} {
			if _, ok := gotEnv[requiredKey]; !ok {
				t.Fatalf("child environment missing required Unix key %s", requiredKey)
			}
		}
	}
}

func envMap(env []string) map[string]string {
	got := make(map[string]string, len(env))
	for _, entry := range env {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		got[key] = value
	}
	return got
}
