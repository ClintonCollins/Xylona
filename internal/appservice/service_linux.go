//go:build linux

package appservice

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// LinuxAccess describes permissions required by a service account.
type LinuxAccess uint8

const (
	// LinuxRead requires read access.
	LinuxRead LinuxAccess = 1 << iota
	// LinuxWrite requires write access.
	LinuxWrite
	// LinuxExecute requires execute or directory traversal access.
	LinuxExecute
)

const systemdOperationTimeout = 3 * time.Minute

var (
	currentEffectiveUID      = os.Geteuid
	systemdUnitBaseDirectory = systemdUnitDirectory
	executeSystemctl         = executeSystemctlCommand
)

type systemAccountLookup struct{}

func (systemAccountLookup) Current() (*user.User, error) {
	account, errCurrent := user.Current()
	if errCurrent != nil {
		return nil, fmt.Errorf("resolve current operating-system user: %w", errCurrent)
	}
	return account, nil
}

func (systemAccountLookup) Lookup(username string) (*user.User, error) {
	account, errLookup := user.Lookup(username)
	if errLookup != nil {
		return nil, fmt.Errorf("resolve operating-system user %s: %w", username, errLookup)
	}
	return account, nil
}

func (systemAccountLookup) LookupID(uid string) (*user.User, error) {
	account, errLookup := user.LookupId(uid)
	if errLookup != nil {
		return nil, fmt.Errorf("resolve operating-system user ID %s: %w", uid, errLookup)
	}
	return account, nil
}

func (systemAccountLookup) LookupGroupID(gid string) (*user.Group, error) {
	group, errLookup := user.LookupGroupId(gid)
	if errLookup != nil {
		return nil, fmt.Errorf("resolve operating-system group ID %s: %w", gid, errLookup)
	}
	return group, nil
}

func (systemAccountLookup) GroupIDs(account *user.User) ([]string, error) {
	groupIDs, errGroups := account.GroupIds()
	if errGroups != nil {
		return nil, fmt.Errorf("resolve supplementary groups for %s: %w", account.Username, errGroups)
	}
	return groupIDs, nil
}

func platformInstall(ctx context.Context, definition Definition, options InstallOptions) (InstallResult, error) {
	if currentEffectiveUID() != 0 {
		return InstallResult{}, errors.New("install a systemd service: run this command as root or through sudo")
	}
	resolvedDefinition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return InstallResult{}, errDefinition
	}

	account, warning, errAccount := resolveLinuxInstallAccount(options)
	if errAccount != nil {
		return InstallResult{}, errAccount
	}
	if !LinuxAccountCanAccess(
		resolvedDefinition.ExecutablePath,
		account,
		LinuxRead|LinuxExecute,
	) {
		return InstallResult{}, fmt.Errorf(
			"service user %s cannot read and execute %s",
			account.Username,
			resolvedDefinition.ExecutablePath,
		)
	}
	if !LinuxAccountCanTraverse(resolvedDefinition.ExecutablePath, account) {
		return InstallResult{}, fmt.Errorf(
			"service user %s cannot traverse the executable path %s",
			account.Username,
			resolvedDefinition.ExecutablePath,
		)
	}
	if !LinuxAccountCanTraverse(resolvedDefinition.WorkingDirectory, account) {
		return InstallResult{}, fmt.Errorf(
			"service user %s cannot traverse the working directory %s",
			account.Username,
			resolvedDefinition.WorkingDirectory,
		)
	}
	unit, errUnit := buildSystemdUnit(resolvedDefinition, account)
	if errUnit != nil {
		return InstallResult{}, errUnit
	}
	unitPath := filepath.Join(systemdUnitBaseDirectory, resolvedDefinition.UnitName)
	_, errExisting := os.Lstat(unitPath)
	if errExisting == nil {
		return InstallResult{}, fmt.Errorf("systemd unit %s already exists; uninstall it explicitly before reinstalling", unitPath)
	}
	if !errors.Is(errExisting, os.ErrNotExist) {
		return InstallResult{}, fmt.Errorf("inspect systemd unit %s: %w", unitPath, errExisting)
	}

	errWrite := writeSystemdUnitAtomically(unitPath, []byte(unit))
	if errWrite != nil {
		return InstallResult{}, errWrite
	}
	result := InstallResult{
		ExecutablePath: resolvedDefinition.ExecutablePath,
		User:           account.Username,
		Warning:        warning,
	}

	errReload := runSystemctl(ctx, "daemon-reload")
	if errReload != nil {
		errRollback := rollbackSystemdUnit(unitPath, resolvedDefinition.UnitName)
		return InstallResult{}, errors.Join(
			fmt.Errorf("reload systemd after installing %s: %w", resolvedDefinition.UnitName, errReload),
			errRollback,
		)
	}
	errEnable := runSystemctl(ctx, "enable", resolvedDefinition.UnitName)
	if errEnable != nil {
		errRollback := rollbackSystemdUnit(unitPath, resolvedDefinition.UnitName)
		return InstallResult{}, errors.Join(
			fmt.Errorf("enable systemd unit %s: %w", resolvedDefinition.UnitName, errEnable),
			errRollback,
		)
	}

	canWriteInstallDirectory := LinuxAccountCanAccess(
		filepath.Dir(resolvedDefinition.ExecutablePath),
		account,
		LinuxWrite|LinuxExecute,
	)
	if !canWriteInstallDirectory {
		updateWarning := "The service account cannot write the executable directory, so in-application binary updates will be unavailable."
		if result.Warning == "" {
			result.Warning = updateWarning
		} else {
			result.Warning += " " + updateWarning
		}
	}

	if options.Start {
		errStart := platformStart(ctx, resolvedDefinition)
		if errStart != nil {
			return result, fmt.Errorf(
				"systemd unit %s was installed and enabled but failed to start: %w",
				resolvedDefinition.UnitName,
				errStart,
			)
		}
	}
	return result, nil
}

func platformStart(ctx context.Context, definition Definition) error {
	resolvedDefinition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	if strings.TrimSpace(resolvedDefinition.UnitName) == "" {
		return errors.New("systemd unit name is required")
	}
	errStart := runSystemctl(ctx, "start", resolvedDefinition.UnitName)
	if errStart != nil {
		return fmt.Errorf("start systemd unit %s: %w", resolvedDefinition.UnitName, errStart)
	}
	return nil
}

func platformStop(ctx context.Context, definition Definition) error {
	resolvedDefinition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	if strings.TrimSpace(resolvedDefinition.UnitName) == "" {
		return errors.New("systemd unit name is required")
	}
	errStop := runSystemctl(ctx, "stop", resolvedDefinition.UnitName)
	if errStop != nil {
		return fmt.Errorf("stop systemd unit %s: %w", resolvedDefinition.UnitName, errStop)
	}
	return nil
}

func platformStatus(ctx context.Context, definition Definition) (string, error) {
	resolvedDefinition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return "", errDefinition
	}
	if strings.TrimSpace(resolvedDefinition.UnitName) == "" {
		return "", errors.New("systemd unit name is required")
	}

	output, errShow := outputSystemctl(
		ctx,
		"show",
		resolvedDefinition.UnitName,
		"--property=LoadState",
		"--property=ActiveState",
		"--property=SubState",
	)
	if errShow != nil {
		return "", fmt.Errorf("query systemd unit %s: %w", resolvedDefinition.UnitName, errShow)
	}
	properties := make(map[string]string)
	for line := range strings.SplitSeq(strings.TrimSpace(output), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			properties[key] = value
		}
	}
	if properties["LoadState"] == "not-found" || properties["LoadState"] == "" {
		return "", fmt.Errorf("systemd unit %s is not installed", resolvedDefinition.UnitName)
	}
	activeState := properties["ActiveState"]
	subState := properties["SubState"]
	if subState == "" || subState == activeState {
		return activeState, nil
	}
	return fmt.Sprintf("%s (%s)", activeState, subState), nil
}

func platformUninstall(ctx context.Context, definition Definition) error {
	if currentEffectiveUID() != 0 {
		return errors.New("uninstall a systemd service: run this command as root or through sudo")
	}
	resolvedDefinition, errDefinition := resolveDefinition(definition)
	if errDefinition != nil {
		return errDefinition
	}
	unitPath := filepath.Join(systemdUnitBaseDirectory, resolvedDefinition.UnitName)
	content, errRead := os.ReadFile(unitPath)
	if errRead != nil {
		return fmt.Errorf("read systemd unit %s before uninstall: %w", unitPath, errRead)
	}
	if !strings.HasPrefix(string(content), systemdManagedMarker+"\n") {
		return fmt.Errorf("refuse to remove unmanaged systemd unit %s", unitPath)
	}

	errStop := runSystemctl(ctx, "stop", resolvedDefinition.UnitName)
	if errStop != nil {
		return fmt.Errorf("stop systemd unit before uninstall: %w", errStop)
	}
	errDisable := runSystemctl(ctx, "disable", resolvedDefinition.UnitName)
	if errDisable != nil {
		return fmt.Errorf("disable systemd unit before uninstall: %w", errDisable)
	}
	errRemove := os.Remove(unitPath)
	if errRemove != nil {
		return fmt.Errorf("remove systemd unit %s: %w", unitPath, errRemove)
	}
	errReload := runSystemctl(ctx, "daemon-reload")
	if errReload != nil {
		return fmt.Errorf("systemd unit was removed, but daemon-reload failed: %w", errReload)
	}
	return nil
}

func platformRun(Definition, RunFunc, LogConfigurator) error {
	return errors.New("systemd starts Xylona in foreground mode; the hidden service host is Windows-only")
}

// ResolveLinuxAccount resolves an explicit service user or the current
// invoking user, preferring sudo's original numeric UID when present.
func ResolveLinuxAccount(requestedUser string) (Account, string, error) {
	return resolveInstallAccount(
		requestedUser,
		os.Getenv("SUDO_UID"),
		os.Getenv("SUDO_USER"),
		systemAccountLookup{},
	)
}

// RequireLinuxRoot rejects service installation before pairing or other
// persistent mutations occur.
func RequireLinuxRoot(operation string) error {
	if currentEffectiveUID() == 0 {
		return nil
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		operation = "manage a systemd service"
	}
	return fmt.Errorf("%s: run this command as root or through sudo", operation)
}

// LinuxAccountCanAccess reports whether the account has the requested
// traditional Unix permission bits for a path.
func LinuxAccountCanAccess(path string, account Account, required LinuxAccess) bool {
	info, errInfo := os.Stat(path)
	if errInfo != nil {
		return false
	}
	if account.UID == "0" {
		if required&LinuxExecute != 0 && !info.IsDir() && info.Mode().Perm()&0o111 == 0 {
			return false
		}
		return true
	}
	stat, statOK := info.Sys().(*syscall.Stat_t)
	if !statOK {
		return false
	}

	requiredBits := accessModeBits(required)
	mode := uint32(info.Mode().Perm())
	accountUID, errUID := strconv.ParseUint(account.UID, 10, 32)
	if errUID != nil {
		return false
	}
	if uint32(accountUID) == stat.Uid {
		return mode&(requiredBits<<6) == requiredBits<<6
	}
	if accountHasGroup(account, strconv.FormatUint(uint64(stat.Gid), 10)) {
		return mode&(requiredBits<<3) == requiredBits<<3
	}
	return mode&requiredBits == requiredBits
}

// LinuxAccountCanTraverse reports whether an account can traverse every
// directory in an existing absolute or relative path. The resolved path is
// checked as well so symlink targets cannot hide an inaccessible ancestor.
func LinuxAccountCanTraverse(path string, account Account) bool {
	absolutePath, errAbsolute := filepath.Abs(path)
	if errAbsolute != nil {
		return false
	}
	absolutePath = filepath.Clean(absolutePath)
	if !linuxAccountCanTraverseDirectoryChain(absolutePath, account) {
		return false
	}

	resolvedPath, errResolved := filepath.EvalSymlinks(absolutePath)
	if errResolved != nil {
		return false
	}
	resolvedPath = filepath.Clean(resolvedPath)
	if resolvedPath == absolutePath {
		return true
	}
	return linuxAccountCanTraverseDirectoryChain(resolvedPath, account)
}

// ChownLinuxPath assigns one newly created path to a service account without
// traversing directories or following symlinks.
func ChownLinuxPath(path string, account Account) error {
	info, errInfo := os.Lstat(path)
	if errInfo != nil {
		return fmt.Errorf("inspect newly created service path %s: %w", path, errInfo)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("refuse to change ownership of symlink %s", path)
	}
	uid, errUID := strconv.Atoi(account.UID)
	if errUID != nil {
		return fmt.Errorf("parse service user ID %q: %w", account.UID, errUID)
	}
	gid, errGID := strconv.Atoi(account.PrimaryGroupID)
	if errGID != nil {
		return fmt.Errorf("parse service primary group ID %q: %w", account.PrimaryGroupID, errGID)
	}
	errChown := os.Lchown(path, uid, gid)
	if errChown != nil {
		return fmt.Errorf("assign %s to %s:%s: %w", path, account.Username, account.PrimaryGroup, errChown)
	}
	return nil
}

func resolveLinuxInstallAccount(options InstallOptions) (Account, string, error) {
	if options.Account != nil {
		return *options.Account, "", nil
	}
	return ResolveLinuxAccount(options.User)
}

func writeSystemdUnitAtomically(unitPath string, content []byte) (resultErr error) {
	tempFile, errCreate := os.CreateTemp(filepath.Dir(unitPath), "."+filepath.Base(unitPath)+".tmp-*")
	if errCreate != nil {
		return fmt.Errorf("create temporary systemd unit: %w", errCreate)
	}
	tempPath := tempFile.Name()
	defer func() {
		errRemove := os.Remove(tempPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			resultErr = errors.Join(resultErr, fmt.Errorf("remove temporary systemd unit: %w", errRemove))
		}
	}()

	errChmod := tempFile.Chmod(0o644)
	if errChmod != nil {
		errClose := tempFile.Close()
		return errors.Join(fmt.Errorf("set systemd unit permissions: %w", errChmod), errClose)
	}
	_, errWrite := tempFile.Write(content)
	if errWrite != nil {
		errClose := tempFile.Close()
		return errors.Join(fmt.Errorf("write temporary systemd unit: %w", errWrite), errClose)
	}
	errSync := tempFile.Sync()
	errClose := tempFile.Close()
	if errSync != nil || errClose != nil {
		return errors.Join(
			wrapOperationError("sync temporary systemd unit", errSync),
			wrapOperationError("close temporary systemd unit", errClose),
		)
	}
	errLink := os.Link(tempPath, unitPath)
	if errLink != nil {
		return fmt.Errorf("install systemd unit %s without replacing an existing unit: %w", unitPath, errLink)
	}
	return nil
}

func wrapOperationError(operation string, errValue error) error {
	if errValue == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", operation, errValue)
}

func rollbackSystemdUnit(unitPath string, unitName string) error {
	errDisable := runSystemctl(context.Background(), "disable", unitName)
	errRemove := os.Remove(unitPath)
	if errors.Is(errRemove, os.ErrNotExist) {
		errRemove = nil
	}
	errReload := runSystemctl(context.Background(), "daemon-reload")
	return errors.Join(
		wrapRollbackError("disable unit", errDisable),
		wrapRollbackError("remove unit", errRemove),
		wrapRollbackError("reload systemd", errReload),
	)
}

func wrapRollbackError(operation string, errValue error) error {
	if errValue == nil {
		return nil
	}
	return fmt.Errorf("roll back incomplete systemd installation (%s): %w", operation, errValue)
}

func runSystemctl(ctx context.Context, arguments ...string) error {
	_, errOutput := outputSystemctl(ctx, arguments...)
	return errOutput
}

func outputSystemctl(ctx context.Context, arguments ...string) (string, error) {
	return executeSystemctl(ctx, arguments...)
}

func executeSystemctlCommand(ctx context.Context, arguments ...string) (string, error) {
	operationContext, cancelOperation := context.WithTimeout(ctx, systemdOperationTimeout)
	defer cancelOperation()

	command := exec.CommandContext(operationContext, "systemctl", arguments...)
	output, errRun := command.CombinedOutput()
	trimmedOutput := strings.TrimSpace(string(output))
	if errRun != nil {
		if trimmedOutput == "" {
			return "", fmt.Errorf("execute systemctl: %w", errRun)
		}
		return "", fmt.Errorf("%w: %s", errRun, trimmedOutput)
	}
	return trimmedOutput, nil
}

func accessModeBits(required LinuxAccess) uint32 {
	var bits uint32
	if required&LinuxRead != 0 {
		bits |= 0o4
	}
	if required&LinuxWrite != 0 {
		bits |= 0o2
	}
	if required&LinuxExecute != 0 {
		bits |= 0o1
	}
	return bits
}

func accountHasGroup(account Account, groupID string) bool {
	if account.PrimaryGroupID == groupID {
		return true
	}
	return slices.Contains(account.GroupIDs, groupID)
}

func linuxAccountCanTraverseDirectoryChain(path string, account Account) bool {
	currentPath := path
	for {
		if !LinuxAccountCanAccess(currentPath, account, LinuxExecute) {
			return false
		}
		parentPath := filepath.Dir(currentPath)
		if parentPath == currentPath {
			return true
		}
		currentPath = parentPath
	}
}
