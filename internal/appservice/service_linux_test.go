//go:build linux

package appservice

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type fakeSystemctl struct {
	calls      []string
	failAction string
	showOutput string
}

func (f *fakeSystemctl) execute(_ context.Context, arguments ...string) (string, error) {
	call := strings.Join(arguments, " ")
	f.calls = append(f.calls, call)
	if len(arguments) > 0 && arguments[0] == f.failAction {
		return "", errors.New("injected systemctl failure")
	}
	if len(arguments) > 0 && arguments[0] == "show" {
		return f.showOutput, nil
	}
	return "", nil
}

func TestSystemdServiceLifecycle(t *testing.T) {
	t.Run("install writes reloads and enables managed unit", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		definition := linuxServiceTestDefinition(t)

		result, errInstall := platformInstall(t.Context(), definition, InstallOptions{
			Account: linuxRootTestAccount(),
		})
		if errInstall != nil {
			t.Fatalf("platformInstall() error = %v", errInstall)
		}
		if result.ExecutablePath == "" || result.User != "root" {
			t.Fatalf("install result = %+v, want resolved executable and root user", result)
		}
		unitPath := filepath.Join(systemdUnitBaseDirectory, definition.UnitName)
		content, errRead := os.ReadFile(unitPath)
		if errRead != nil {
			t.Fatalf("read installed systemd unit: %v", errRead)
		}
		if !strings.HasPrefix(string(content), systemdManagedMarker+"\n") {
			t.Fatalf("installed unit is missing managed marker:\n%s", content)
		}
		wantCalls := []string{"daemon-reload", "enable " + definition.UnitName}
		if !slices.Equal(fake.calls, wantCalls) {
			t.Fatalf("systemctl calls = %q, want %q", fake.calls, wantCalls)
		}
	})

	t.Run("enable failure rolls unit back", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		fake.failAction = "enable"
		definition := linuxServiceTestDefinition(t)

		_, errInstall := platformInstall(t.Context(), definition, InstallOptions{
			Account: linuxRootTestAccount(),
		})
		if errInstall == nil || !strings.Contains(errInstall.Error(), "enable systemd unit") {
			t.Fatalf("platformInstall() error = %v, want enable failure", errInstall)
		}
		unitPath := filepath.Join(systemdUnitBaseDirectory, definition.UnitName)
		_, errStat := os.Stat(unitPath)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("unit remained after rollback, stat error = %v", errStat)
		}
		if !slices.Contains(fake.calls, "disable "+definition.UnitName) {
			t.Fatalf("rollback did not disable unit, calls = %q", fake.calls)
		}
		if fake.calls[len(fake.calls)-1] != "daemon-reload" {
			t.Fatalf("rollback did not finish with daemon-reload, calls = %q", fake.calls)
		}
	})

	t.Run("start failure leaves installed unit for diagnosis", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		fake.failAction = "start"
		definition := linuxServiceTestDefinition(t)

		result, errInstall := platformInstall(t.Context(), definition, InstallOptions{
			Account: linuxRootTestAccount(),
			Start:   true,
		})
		if errInstall == nil || !strings.Contains(errInstall.Error(), "installed and enabled but failed to start") {
			t.Fatalf("platformInstall() error = %v, want retained start failure", errInstall)
		}
		if result.ExecutablePath == "" {
			t.Fatal("install result was lost after start failure")
		}
		unitPath := filepath.Join(systemdUnitBaseDirectory, definition.UnitName)
		_, errStat := os.Stat(unitPath)
		if errStat != nil {
			t.Fatalf("installed unit was removed after start failure: %v", errStat)
		}
	})

	t.Run("uninstall refuses foreign unit", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		definition := linuxServiceTestDefinition(t)
		unitPath := filepath.Join(systemdUnitBaseDirectory, definition.UnitName)
		errWrite := os.WriteFile(unitPath, []byte("[Service]\nExecStart=/bin/foreign\n"), 0o600)
		if errWrite != nil {
			t.Fatalf("write foreign unit: %v", errWrite)
		}

		errUninstall := platformUninstall(t.Context(), definition)
		if errUninstall == nil || !strings.Contains(errUninstall.Error(), "refuse to remove unmanaged") {
			t.Fatalf("platformUninstall() error = %v, want unmanaged-unit refusal", errUninstall)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("systemctl was called for foreign unit: %q", fake.calls)
		}
	})

	t.Run("uninstall removes only managed unit", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		definition := linuxServiceTestDefinition(t)
		unitPath := filepath.Join(systemdUnitBaseDirectory, definition.UnitName)
		errWrite := os.WriteFile(unitPath, []byte(systemdManagedMarker+"\n[Service]\n"), 0o600)
		if errWrite != nil {
			t.Fatalf("write managed unit: %v", errWrite)
		}

		errUninstall := platformUninstall(t.Context(), definition)
		if errUninstall != nil {
			t.Fatalf("platformUninstall() error = %v", errUninstall)
		}
		_, errStat := os.Stat(unitPath)
		if !errors.Is(errStat, os.ErrNotExist) {
			t.Fatalf("managed unit remains after uninstall, stat error = %v", errStat)
		}
		wantCalls := []string{
			"stop " + definition.UnitName,
			"disable " + definition.UnitName,
			"daemon-reload",
		}
		if !slices.Equal(fake.calls, wantCalls) {
			t.Fatalf("systemctl calls = %q, want %q", fake.calls, wantCalls)
		}
	})

	t.Run("status reports active and substate", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		fake.showOutput = "LoadState=loaded\nActiveState=active\nSubState=running\n"
		definition := linuxServiceTestDefinition(t)

		state, errStatus := platformStatus(t.Context(), definition)
		if errStatus != nil {
			t.Fatalf("platformStatus() error = %v", errStatus)
		}
		if state != "active (running)" {
			t.Fatalf("service state = %q, want active (running)", state)
		}
	})

	t.Run("non-root install fails before mutation", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		currentEffectiveUID = func() int {
			return 1000
		}
		definition := linuxServiceTestDefinition(t)

		_, errInstall := platformInstall(t.Context(), definition, InstallOptions{
			Account: linuxRootTestAccount(),
		})
		if errInstall == nil || !strings.Contains(errInstall.Error(), "run this command as root") {
			t.Fatalf("platformInstall() error = %v, want root requirement", errInstall)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("systemctl was called without root: %q", fake.calls)
		}
	})

	t.Run("unusable executable fails before mutation", func(t *testing.T) {
		fake := configureFakeSystemd(t)
		definition := linuxServiceTestDefinition(t)
		errMode := os.Chmod(definition.ExecutablePath, 0o600)
		if errMode != nil {
			t.Fatalf("remove test executable permission: %v", errMode)
		}

		_, errInstall := platformInstall(t.Context(), definition, InstallOptions{
			Account: linuxRootTestAccount(),
		})
		if errInstall == nil || !strings.Contains(errInstall.Error(), "cannot read and execute") {
			t.Fatalf("platformInstall() error = %v, want executable access rejection", errInstall)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("systemctl was called for unusable executable: %q", fake.calls)
		}
	})
}

func TestChownLinuxPathRejectsSymlink(t *testing.T) {
	targetPath := filepath.Join(t.TempDir(), "identity")
	errWrite := os.WriteFile(targetPath, []byte("identity"), 0o600)
	if errWrite != nil {
		t.Fatalf("write symlink target: %v", errWrite)
	}
	linkPath := filepath.Join(t.TempDir(), "identity-link")
	errLink := os.Symlink(targetPath, linkPath)
	if errLink != nil {
		t.Fatalf("create identity symlink: %v", errLink)
	}

	errChown := ChownLinuxPath(linkPath, *linuxRootTestAccount())
	if errChown == nil || !strings.Contains(errChown.Error(), "refuse to change ownership of symlink") {
		t.Fatalf("ChownLinuxPath() error = %v, want symlink rejection", errChown)
	}
}

func TestLinuxAccountCanTraverse(t *testing.T) {
	// A direct child of the shared temp directory keeps all ancestors
	// traversable by the synthetic account used below.
	baseDirectory, errTemp := os.MkdirTemp(os.TempDir(), "xylona-traverse-*") //nolint:usetesting // Direct temp child is required for synthetic-user traversal.
	if errTemp != nil {
		t.Fatalf("create direct temporary directory: %v", errTemp)
	}
	t.Cleanup(func() {
		errRemove := os.RemoveAll(baseDirectory)
		if errRemove != nil {
			t.Errorf("remove direct temporary directory: %v", errRemove)
		}
	})
	errBaseMode := os.Chmod(baseDirectory, 0o711) //nolint:gosec // Test fixture needs other-user traversal.
	if errBaseMode != nil {
		t.Fatalf("make base directory traversable: %v", errBaseMode)
	}
	accessibleDirectory := filepath.Join(baseDirectory, "accessible")
	errAccessible := os.Mkdir(accessibleDirectory, 0o711) //nolint:gosec // Test fixture needs other-user traversal.
	if errAccessible != nil {
		t.Fatalf("create accessible directory: %v", errAccessible)
	}
	privateDirectory := filepath.Join(accessibleDirectory, "private")
	errPrivate := os.Mkdir(privateDirectory, 0o700)
	if errPrivate != nil {
		t.Fatalf("create private directory: %v", errPrivate)
	}

	unrelatedAccount := Account{
		Username:       "service-test",
		UID:            "424242",
		PrimaryGroup:   "service-test",
		PrimaryGroupID: "424242",
		GroupIDs:       []string{"424242"},
	}
	if !LinuxAccountCanTraverse(accessibleDirectory, unrelatedAccount) {
		t.Fatalf("account should traverse %s", accessibleDirectory)
	}
	if LinuxAccountCanTraverse(privateDirectory, unrelatedAccount) {
		t.Fatalf("account unexpectedly traversed %s", privateDirectory)
	}

	linkTargetParent := filepath.Join(baseDirectory, "private-target-parent")
	errTargetParent := os.Mkdir(linkTargetParent, 0o700)
	if errTargetParent != nil {
		t.Fatalf("create private symlink target parent: %v", errTargetParent)
	}
	linkTarget := filepath.Join(linkTargetParent, "target")
	errTarget := os.Mkdir(linkTarget, 0o711) //nolint:gosec // Test fixture needs other-user traversal.
	if errTarget != nil {
		t.Fatalf("create symlink target: %v", errTarget)
	}
	linkPath := filepath.Join(accessibleDirectory, "linked-target")
	errLink := os.Symlink(linkTarget, linkPath)
	if errLink != nil {
		t.Fatalf("create directory symlink: %v", errLink)
	}
	if LinuxAccountCanTraverse(linkPath, unrelatedAccount) {
		t.Fatalf("account unexpectedly traversed symlink target through private parent %s", linkTargetParent)
	}
}

func configureFakeSystemd(t *testing.T) *fakeSystemctl {
	t.Helper()

	originalEUID := currentEffectiveUID
	originalUnitDirectory := systemdUnitBaseDirectory
	originalExecute := executeSystemctl
	fake := &fakeSystemctl{}
	currentEffectiveUID = func() int {
		return 0
	}
	systemdUnitBaseDirectory = t.TempDir()
	executeSystemctl = fake.execute
	t.Cleanup(func() {
		currentEffectiveUID = originalEUID
		systemdUnitBaseDirectory = originalUnitDirectory
		executeSystemctl = originalExecute
	})
	return fake
}

func linuxServiceTestDefinition(t *testing.T) Definition {
	t.Helper()

	executablePath := filepath.Join(t.TempDir(), "xylona-node")
	errWrite := os.WriteFile(executablePath, []byte("test executable"), 0o600)
	if errWrite != nil {
		t.Fatalf("write test service executable: %v", errWrite)
	}
	errMode := os.Chmod(executablePath, 0o700) //nolint:gosec // The production path must be executable.
	if errMode != nil {
		t.Fatalf("make test service binary executable: %v", errMode)
	}
	return Definition{
		Name:             "XylonaNode",
		UnitName:         "xylona-node.service",
		DisplayName:      "Xylona Node",
		Description:      "Xylona remote game server node",
		ExecutablePath:   executablePath,
		WorkingDirectory: t.TempDir(),
		Arguments:        []string{"--data-dir", t.TempDir()},
	}
}

func linuxRootTestAccount() *Account {
	return &Account{
		Username:       "root",
		UID:            "0",
		PrimaryGroup:   "root",
		PrimaryGroupID: "0",
		GroupIDs:       []string{"0"},
	}
}
