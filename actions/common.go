package actions

import (
	"fmt"
	"os"
	"runtime"

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

func gameStartCommand(game *models.Game) string {
	startCommand := game.LinuxStartCommand
	if OperatingSystem == Windows {
		startCommand = game.WindowsStartCommand
	}
	return startCommand
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
