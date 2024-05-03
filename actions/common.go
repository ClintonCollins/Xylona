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
)

var (
	OperatingSystem OSType
)

func init() {
	switch runtime.GOOS {
	case "windows":
		OperatingSystem = Windows
	case "linux":
		OperatingSystem = Linux
	default:
		log.Error().Str("OS", runtime.GOOS).Msg("Unsupported operating system")
		os.Exit(1)
	}
}

func DefaultInstallPath() string {
	if OperatingSystem == Linux {
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
