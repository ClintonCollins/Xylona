package modmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
)

type extractedModProvider struct {
	mockProvider
	content map[string]string
}

func (p *extractedModProvider) DownloadValheim(_ context.Context, _, _, destination, targetOS string) ([]modproviders.DownloadedFile, error) {
	if targetOS != runtime.GOOS {
		return nil, fmt.Errorf("unexpected target OS: %s", targetOS)
	}
	var files []modproviders.DownloadedFile
	for name, content := range p.content {
		filename := filepath.Join(destination, filepath.FromSlash(name))
		errMkdir := os.MkdirAll(filepath.Dir(filename), 0o750)
		if errMkdir != nil {
			return nil, fmt.Errorf("create test directory: %w", errMkdir)
		}
		errWrite := os.WriteFile(filename, []byte(content), 0o600)
		if errWrite != nil {
			return nil, fmt.Errorf("write test file: %w", errWrite)
		}
		hash := sha256.Sum256([]byte(content))
		files = append(files, modproviders.DownloadedFile{Path: name, Size: int64(len(content)), Hash: hex.EncodeToString(hash[:]), IsPrimary: !valheimConfig(name)})
	}
	return files, nil
}

type extractedModClient struct {
	valheimFileClient
	failTarget string
}

func (c extractedModClient) GetNodeSnapshot(context.Context) (*node.NodeSnapshot, error) {
	return &node.NodeSnapshot{OS: runtime.GOOS}, nil
}

func (c extractedModClient) RenameFile(ctx context.Context, directory, old, next string, policy node.ProtectionPolicy) (string, error) {
	if next == c.failTarget && filepath.Base(filepath.Dir(old)) != "previous" {
		return "", errors.New("injected promotion failure")
	}
	renamed, errRename := c.valheimFileClient.RenameFile(ctx, directory, old, next, policy)
	if errRename != nil {
		return "", fmt.Errorf("rename test file: %w", errRename)
	}
	return renamed, nil
}

func TestValheimExtractedModLifecycle(t *testing.T) {
	for _, remote := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote=%v", remote), func(t *testing.T) {
			conn := dbtest.NewMigratedConnection(t, "valheim-mods.sqlite")
			seedTestFixture(t, conn)
			_, errGame := conn.SQLDb.ExecContext(t.Context(), "UPDATE game_server SET game_id = 'valheim' WHERE id = 'server-1'")
			if errGame != nil {
				t.Fatal(errGame)
			}
			manager := New(conn)
			directory := t.TempDir()
			fileClient := newTestModFileClient(t, conn)
			capable, ok := fileClient.(valheimFileClient)
			if !ok {
				t.Fatal("missing file capability")
			}
			client := extractedModClient{valheimFileClient: capable}
			var installClient FileClient
			if remote {
				installClient = client
			}
			provider := &extractedModProvider{details: &modproviders.ModDetails{Name: "Example", Author: "Author"}, content: map[string]string{"BepInEx/plugins/Author-Example/nested/mod.dll": "first", "BepInEx/config/example.cfg": "default"}}
			mod, errInstall := manager.changeValheimPackage(t.Context(), installClient, provider, "server-1", "Author-Example", "1.0.0", directory, nil)
			if errInstall != nil {
				t.Fatal(errInstall)
			}
			assertFile := func(name, want string) {
				t.Helper()
				content, errRead := os.ReadFile(filepath.Join(directory, filepath.FromSlash(name)))
				if errRead != nil || string(content) != want {
					t.Fatalf("%s = %q, %v; want %q", name, content, errRead, want)
				}
			}
			assertFile("BepInEx/plugins/Author-Example/nested/mod.dll", "first")
			errConfig := os.WriteFile(filepath.Join(directory, "BepInEx", "config", "example.cfg"), []byte("user setting"), 0o600)
			if errConfig != nil {
				t.Fatal(errConfig)
			}
			errDisable := manager.Disable(t.Context(), client, mod.ID, directory, "BepInEx/plugins")
			if errDisable != nil {
				t.Fatal(errDisable)
			}
			assertFile(".xylona-disabled/"+mod.ID+"/BepInEx/plugins/Author-Example/nested/mod.dll", "first")
			mod, errInstall = conn.GetInstalledModByID(mod.ID)
			if errInstall != nil {
				t.Fatal(errInstall)
			}
			provider.content = map[string]string{"BepInEx/plugins/Author-Example/nested/replacement.dll": "second", "BepInEx/config/example.cfg": "new default"}
			mod, errInstall = manager.changeValheimPackage(t.Context(), installClient, provider, "server-1", "Author-Example", "2.0.0", directory, mod)
			if errInstall != nil {
				t.Fatal(errInstall)
			}
			assertFile(".xylona-disabled/"+mod.ID+"/BepInEx/plugins/Author-Example/nested/replacement.dll", "second")
			assertFile("BepInEx/config/example.cfg", "user setting")
			errEnable := manager.Enable(t.Context(), client, mod.ID, directory, "BepInEx/plugins")
			if errEnable != nil {
				t.Fatal(errEnable)
			}
			assertFile("BepInEx/plugins/Author-Example/nested/replacement.dll", "second")
			errRemove := manager.Uninstall(t.Context(), client, mod.ID, directory)
			if errRemove != nil {
				t.Fatal(errRemove)
			}
			assertFile("BepInEx/config/example.cfg", "user setting")
			_, errStat := os.Stat(filepath.Join(directory, "BepInEx/plugins/Author-Example/nested/replacement.dll"))
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("uninstalled DLL still present: %v", errStat)
			}
		})
	}
}

func TestValheimModRollbackAndOwnership(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "valheim-mod-rollback.sqlite")
	seedTestFixture(t, conn)
	manager := New(conn)
	directory := t.TempDir()
	capable, ok := newTestModFileClient(t, conn).(valheimFileClient)
	if !ok {
		t.Fatal("missing file capability")
	}
	client := extractedModClient{valheimFileClient: capable}
	provider := &extractedModProvider{details: &modproviders.ModDetails{Name: "Example"}, content: map[string]string{"BepInEx/plugins/Author-Example/one.dll": "old"}}
	mod, errInstall := manager.changeValheimPackage(t.Context(), client, provider, "server-1", "Author-Example", "1.0.0", directory, nil)
	if errInstall != nil {
		t.Fatal(errInstall)
	}
	provider.content = map[string]string{"BepInEx/plugins/Author-Example/two.dll": "new"}
	client.failTarget = "BepInEx/plugins/Author-Example/two.dll"
	_, errUpdate := manager.changeValheimPackage(t.Context(), client, provider, "server-1", "Author-Example", "2.0.0", directory, mod)
	if errUpdate == nil {
		t.Fatal("expected promotion failure")
	}
	content, errRead := os.ReadFile(filepath.Join(directory, "BepInEx/plugins/Author-Example/one.dll"))
	if errRead != nil || string(content) != "old" {
		t.Fatalf("rollback did not restore old file: %q, %v", content, errRead)
	}
	client.failTarget = ""
	provider.content = map[string]string{"BepInEx/plugins/Author-Example/one.dll": "conflict"}
	_, errConflict := manager.changeValheimPackage(t.Context(), client, provider, "server-1", "Other-Package", "1.0.0", directory, nil)
	if errConflict == nil {
		t.Fatal("expected ownership conflict")
	}
	provider.content = map[string]string{"BepInEx/core/BepInEx.Preloader.dll": "loader", "doorstop_config.ini": "default"}
	loader, errLoader := manager.changeValheimPackage(t.Context(), client, provider, "server-1", thunderstore.ValheimLoaderID, thunderstore.ValheimLoaderVersion, directory, nil)
	if errLoader != nil {
		t.Fatal(errLoader)
	}
	if manager.protectValheimLoader(loader) == nil {
		t.Fatal("loader removal should be blocked while mods remain")
	}
	errConfig := os.WriteFile(filepath.Join(directory, "doorstop_config.ini"), []byte("user settings"), 0o600)
	if errConfig != nil {
		t.Fatal(errConfig)
	}
	_, errReinstall := manager.changeValheimPackage(t.Context(), client, provider, "server-1", thunderstore.ValheimLoaderID, thunderstore.ValheimLoaderVersion, directory, loader)
	if errReinstall != nil {
		t.Fatal(errReinstall)
	}
	config, errReadConfig := os.ReadFile(filepath.Join(directory, "doorstop_config.ini"))
	if errReadConfig != nil || string(config) != "user settings" {
		t.Fatalf("loader config = %q, %v", config, errReadConfig)
	}
}

func TestValheimModpackRecord(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "valheim-modpack.sqlite")
	seedTestFixture(t, conn)
	_, errGame := conn.SQLDb.ExecContext(t.Context(), "UPDATE game_server SET game_id = 'valheim' WHERE id = 'server-1'")
	if errGame != nil {
		t.Fatal(errGame)
	}
	manager := New(conn)
	directory := t.TempDir()
	provider := &extractedModProvider{details: &modproviders.ModDetails{Name: "Modpack"}}
	mod, errInstall := manager.changeValheimPackage(t.Context(), nil, provider, "server-1", "Author-Modpack", "1.0.0", directory, nil)
	if errInstall != nil {
		t.Fatal(errInstall)
	}
	files, errFiles := conn.GetInstalledModFilesByModID(mod.ID)
	if errFiles != nil || len(files) != 0 {
		t.Fatalf("modpack files = %v, %v", files, errFiles)
	}
	errRemove := manager.Uninstall(t.Context(), newTestModFileClient(t, conn), mod.ID, directory)
	if errRemove != nil {
		t.Fatal(errRemove)
	}
}
