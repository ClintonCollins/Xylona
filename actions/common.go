package actions

import (
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

type OSType string

const (
	Windows OSType = "windows"
	Linux   OSType = "linux"
	Darwin  OSType = "darwin"
)

var (
	OperatingSystem OSType
)

func init() {
	osType, ok := detectOperatingSystem(runtime.GOOS)
	if !ok {
		log.Error().Str("OS", runtime.GOOS).Msg("Unsupported operating system")
		os.Exit(1)
	}
	OperatingSystem = osType
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

func DefaultInstallPath() string {
	if OperatingSystem == Linux || OperatingSystem == Darwin {
		home := os.Getenv("HOME")
		user := os.Getenv("USER")
		if home == "" && user == "" {
			log.Error().Msg("Failed to get home directory")
			os.Exit(1)
		}
		if home != "" {
			return fmt.Sprintf("%s/xylona", home)
		}
		return fmt.Sprintf("/home/%s/xylona", user)
	}
	return fmt.Sprintf("%s/Xylona", os.Getenv("USERPROFILE"))
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
