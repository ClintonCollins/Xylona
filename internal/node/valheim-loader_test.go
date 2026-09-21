package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareInstalledValheimLoader(t *testing.T) {
	for _, platform := range []string{"linux", "windows"} {
		t.Run(platform, func(t *testing.T) {
			directory := t.TempDir()
			for _, name := range []string{"BepInEx/core", "BepInEx/plugins/Author-Mod/assets", "BepInEx/patchers/Author-Patcher", "doorstop_libs"} {
				errEmpty := os.MkdirAll(filepath.Join(directory, name), 0o750)
				if errEmpty != nil {
					t.Fatal(errEmpty)
				}
			}
			trusted := map[string]string{"SteamAppId": "892970"}
			errVanilla := prepareInstalledValheimLoader(directory, platform, trusted)
			if errVanilla != nil || len(trusted) != 1 {
				t.Fatalf("vanilla preparation = %v, %v", trusted, errVanilla)
			}
			errPlugin := os.WriteFile(filepath.Join(directory, "BepInEx/plugins/Author-Mod/mod.dll"), []byte("plugin"), 0o600)
			if errPlugin != nil {
				t.Fatal(errPlugin)
			}
			errMissingLoader := prepareInstalledValheimLoader(directory, platform, trusted)
			if errMissingLoader == nil || !strings.Contains(errMissingLoader.Error(), "reinstall BepInExPack_Valheim") {
				t.Fatalf("plugin without loader error = %v", errMissingLoader)
			}
			files := []string{"BepInEx/core/BepInEx.Preloader.dll", "BepInEx/core/BepInEx.dll", "winhttp.dll"}
			if platform == "linux" {
				files[2] = "doorstop_libs/libdoorstop_x64.so"
			}
			for _, name := range files {
				path := filepath.Join(directory, name)
				errDirectory := os.MkdirAll(filepath.Dir(path), 0o750)
				if errDirectory != nil {
					t.Fatal(errDirectory)
				}
				errWrite := os.WriteFile(path, []byte("runtime"), 0o600)
				if errWrite != nil {
					t.Fatal(errWrite)
				}
			}
			errPrepare := prepareInstalledValheimLoader(directory, platform, trusted)
			if errPrepare != nil {
				t.Fatal(errPrepare)
			}
			if platform == "linux" && (trusted["DOORSTOP_ENABLED"] != "1" || trusted["LD_PRELOAD"] != filepath.Join(directory, files[2]) || trusted["DOORSTOP_TARGET_ASSEMBLY"] != filepath.Join(directory, files[0]) || trusted["LD_LIBRARY_PATH"] != filepath.Join(directory, "linux64")+":"+filepath.Join(directory, "doorstop_libs")) {
				t.Fatalf("loader environment = %v", trusted)
			}
			configPath := filepath.Join(directory, "BepInEx", "config", "BepInEx.cfg")
			config, errRead := os.ReadFile(configPath)
			if errRead != nil || !strings.Contains(string(config), "Type = GameObject") {
				t.Fatalf("Valheim configuration = %q, %v", config, errRead)
			}
			errCustom := os.WriteFile(configPath, []byte("custom configuration"), 0o600)
			if errCustom != nil {
				t.Fatal(errCustom)
			}
			errAgain := prepareInstalledValheimLoader(directory, platform, trusted)
			if errAgain != nil {
				t.Fatal(errAgain)
			}
			config, errRead = os.ReadFile(configPath)
			if errRead != nil || string(config) != "custom configuration" {
				t.Fatalf("existing configuration was changed: %q, %v", config, errRead)
			}
			if platform == "windows" {
				errDisabled := os.WriteFile(filepath.Join(directory, "doorstop_config.ini"), []byte("[General]\nenabled=false\n"), 0o600)
				if errDisabled != nil {
					t.Fatal(errDisabled)
				}
				errDisabled = prepareInstalledValheimLoader(directory, platform, trusted)
				if errDisabled == nil || !strings.Contains(errDisabled.Error(), "enabled = true") {
					t.Fatalf("disabled loader error = %v", errDisabled)
				}
			}
			errRemove := os.Remove(filepath.Join(directory, files[0]))
			if errRemove != nil {
				t.Fatal(errRemove)
			}
			errPartial := prepareInstalledValheimLoader(directory, platform, trusted)
			if errPartial == nil || !strings.Contains(errPartial.Error(), "reinstall BepInExPack_Valheim") {
				t.Fatalf("partial installation error = %v", errPartial)
			}
		})
	}
}

func TestPrepareInstalledValheimLoaderRejectsEscape(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	errLink := os.Symlink(outside, filepath.Join(directory, "BepInEx"))
	if errLink != nil {
		t.Skipf("symlinks unavailable: %v", errLink)
	}
	errDirectory := os.Mkdir(filepath.Join(outside, "core"), 0o750)
	if errDirectory != nil {
		t.Fatal(errDirectory)
	}
	errPrepare := prepareInstalledValheimLoader(directory, "linux", map[string]string{})
	if errPrepare == nil {
		t.Fatal("escaped loader directory accepted")
	}
}
