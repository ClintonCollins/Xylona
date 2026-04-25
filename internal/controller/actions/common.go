package actions

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/defaultpaths"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// OSType identifies the current host operating system.
type OSType string

// Supported host operating systems for local actions.
const (
	Windows OSType = "windows"
	Linux   OSType = "linux"
	Darwin  OSType = "darwin"
)

var (
	// OperatingSystem stores the detected host operating system.
	OperatingSystem OSType
)

func init() {
	errInitOperatingSystem := initOperatingSystem(runtime.GOOS)
	if errInitOperatingSystem != nil {
		log.Warn().Err(errInitOperatingSystem).Str("OS", runtime.GOOS).Msg("Unsupported operating system")
		OperatingSystem = OSType(runtime.GOOS)
	}
}

func detectOperatingSystem(goos string) (OSType, bool) {
	switch goos {
	case "windows":
		return Windows, true
	case "linux":
		return Linux, true
	case "darwin":
		return Darwin, true
	default:
		return "", false
	}
}

func initOperatingSystem(goos string) error {
	osType, ok := detectOperatingSystem(goos)
	if !ok {
		return fmt.Errorf("unsupported operating system: %s", goos)
	}
	OperatingSystem = osType
	return nil
}

// resolveNodeOS returns the OSType of the node identified by nodeID by
// querying its NodeClient for a fresh NodeSnapshot. Falls back to
// OperatingSystem (the controller's own OS) when the node registry is not
// configured, the node is unreachable, or the reported OS cannot be
// detected. Uses a short timeout so callers that hold a hot-path context
// do not block indefinitely on a misbehaving node.
func (inst *Instance) resolveNodeOS(ctx context.Context, nodeID string) OSType {
	if inst == nil || inst.nodeRegistry == nil {
		return OperatingSystem
	}
	client, errGet := inst.nodeRegistry.Get(nodeID)
	if errGet != nil {
		return OperatingSystem
	}
	snapCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snap, errSnap := client.GetNodeSnapshot(snapCtx)
	if errSnap != nil || snap == nil {
		return OperatingSystem
	}
	osType, ok := detectOperatingSystem(strings.ToLower(strings.TrimSpace(snap.OS)))
	if !ok {
		return OperatingSystem
	}
	return osType
}

func joinManagedPath(parts ...string) string {
	if OperatingSystem == Windows {
		return filepath.Join(parts...)
	}
	return path.Join(parts...)
}

func resolveDefaultInstallPath(operatingSystem OSType, home string, user string, userProfile string) (string, error) {
	installPath, errResolve := defaultpaths.ResolveInstallPath(string(operatingSystem), home, user, userProfile)
	if errResolve == nil {
		return installPath, nil
	}
	if errors.Is(errResolve, defaultpaths.ErrMissingUnixHomeUser) {
		return "", errors.New("failed to get home directory")
	}
	if errors.Is(errResolve, defaultpaths.ErrMissingWindowsUserProfile) {
		return "", errors.New("failed to get user profile directory")
	}
	if errors.Is(errResolve, defaultpaths.ErrUnsupportedOS) {
		return "", fmt.Errorf("unsupported operating system: %s", operatingSystem)
	}
	return "", fmt.Errorf("resolve default install path: %w", errResolve)
}

// DefaultInstallPath returns the default root directory for managed servers.
func DefaultInstallPath() (string, error) {
	return resolveDefaultInstallPath(
		OperatingSystem,
		os.Getenv("HOME"),
		os.Getenv("USER"),
		os.Getenv("USERPROFILE"),
	)
}

// The game*Command helpers take an explicit osType so the caller can select
// the command flavor that matches the target node — which may differ from
// the controller's own OS in a hub-spoke deployment. Call sites that don't
// know a target node pass OperatingSystem (the controller's OS) as a
// best-effort default.

func gameBaseCommand(game *models.Game, osType OSType) string {
	startCommand := game.LinuxBaseCommand
	if osType == Windows {
		startCommand = game.WindowsBaseCommand
	}
	return startCommand
}

func gameStartArgsTemplate(game *models.Game, osType OSType) string {
	startTemplate := game.LinuxStartArgsTemplate.GetOr("")
	if osType == Windows {
		startTemplate = game.WindowsStartArgsTemplate.GetOr("")
	}
	return startTemplate
}

func gameStopCommand(game *models.Game, osType OSType) string {
	stopCommand := game.LinuxStopCommand
	if osType == Windows {
		stopCommand = game.WindowsStopCommand
	}
	return stopCommand
}

func gameUpdateCommand(game *models.Game, osType OSType) string {
	updateCommand := game.LinuxUpdateCommand
	if osType == Windows {
		updateCommand = game.WindowsUpdateCommand
	}
	return updateCommand
}

func gameInstallCommand(game *models.Game, osType OSType) string {
	installCommand := game.LinuxInstallCommand
	if osType == Windows {
		installCommand = game.WindowsInstallCommand
	}
	return installCommand
}

func gameInstallCommandType(game *models.Game, osType OSType) string {
	installType := game.LinuxInstallCommandType
	if osType == Windows {
		installType = game.WindowsInstallCommandType
	}
	return installType
}

func gameUpdateCommandType(game *models.Game, osType OSType) string {
	updateType := game.LinuxUpdateCommandType
	if osType == Windows {
		updateType = game.WindowsUpdateCommandType
	}
	return updateType
}

// splitCommandString preserves the legacy double-quote-only tokenization used for
// install and update command strings. It does not support single quotes,
// backslash escaping, or escaped double quotes inside arguments.
//
// TODO(GE-06 follow-up): remove this compatibility parser once install/update
// command-string paths are replaced with structured argv or a proper tokenizer.
// Still actively called from InstallGameServer and UpdateGameServer in actions.go.
func splitCommandString(command string) (string, []string) {
	foundQuote := false
	commandSplit := strings.FieldsFunc(command, func(r rune) bool {
		if r == '"' {
			foundQuote = !foundQuote
		}
		return r == ' ' && !foundQuote
	})
	for i, arg := range commandSplit {
		if strings.HasPrefix(arg, `"`) && strings.HasSuffix(arg, `"`) {
			commandSplit[i] = strings.Trim(arg, `"`)
		}
	}
	if len(commandSplit) == 0 {
		return "", nil
	}
	return commandSplit[0], commandSplit[1:]
}
