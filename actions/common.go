package actions

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"

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

func joinManagedPath(parts ...string) string {
	if OperatingSystem == Windows {
		return filepath.Join(parts...)
	}
	return path.Join(parts...)
}

func resolveDefaultInstallPath(operatingSystem OSType, home string, user string, userProfile string) (string, error) {
	if operatingSystem == Linux || operatingSystem == Darwin {
		if home == "" && user == "" {
			return "", errors.New("failed to get home directory")
		}
		if home != "" {
			return path.Join(home, "xylona"), nil
		}
		return path.Join("/home", user, "xylona"), nil
	}
	if operatingSystem == Windows {
		if userProfile == "" {
			return "", errors.New("failed to get user profile directory")
		}
		return filepath.Join(userProfile, "Xylona"), nil
	}
	return "", fmt.Errorf("unsupported operating system: %s", operatingSystem)
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

func gameBaseCommand(game *models.Game) string {
	startCommand := game.LinuxBaseCommand
	if OperatingSystem == Windows {
		startCommand = game.WindowsBaseCommand
	}
	return startCommand
}

func gameStartArgsTemplate(game *models.Game) string {
	startTemplate := game.LinuxStartArgsTemplate.GetOr("")
	if OperatingSystem == Windows {
		startTemplate = game.WindowsStartArgsTemplate.GetOr("")
	}
	return startTemplate
}

func gameStopCommand(game *models.Game) string {
	stopCommand := game.LinuxStopCommand
	if OperatingSystem == Windows {
		stopCommand = game.WindowsStopCommand
	}
	return stopCommand
}

func gameUpdateCommand(game *models.Game) string {
	updateCommand := game.LinuxUpdateCommand
	if OperatingSystem == Windows {
		updateCommand = game.WindowsUpdateCommand
	}
	return updateCommand
}

func gameInstallCommand(game *models.Game) string {
	installCommand := game.LinuxInstallCommand
	if OperatingSystem == Windows {
		installCommand = game.WindowsInstallCommand
	}
	return installCommand
}

func gameInstallCommandType(game *models.Game) string {
	installType := game.LinuxInstallCommandType
	if OperatingSystem == Windows {
		installType = game.WindowsInstallCommandType
	}
	return installType
}

func gameUpdateCommandType(game *models.Game) string {
	updateType := game.LinuxUpdateCommandType
	if OperatingSystem == Windows {
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
