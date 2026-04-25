package helpers

import (
	"errors"
	"fmt"
	"os"
	"path"
)

// GetAppDirectory returns the Xylona application data directory on Windows.
func GetAppDirectory() (string, error) {
	appData := os.Getenv("HOMEDRIVE")
	if appData == "" {
		return "", errors.New("app data doesn't exist")
	}
	appData = path.Join(appData, "Xylona")
	return appData, nil
}

// GetOperatingDirectory returns the Xylona operating directory on Windows.
func GetOperatingDirectory() (string, error) {
	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		return "", errors.New("user profile doesn't exist")
	}
	userProfile = path.Join(userProfile, "Xylona")
	return userProfile, nil
}

// GetHomeDirectory returns the Xylona home directory for user data on Windows.
func GetHomeDirectory() (string, error) {
	homeDirectory := os.Getenv("HOMEDRIVE")
	if homeDirectory == "" {
		return "", errors.New("user profile doesn't exist")
	}
	homeDirectory = path.Join(homeDirectory, "Xylona", "Users")
	return homeDirectory, nil
}

// GetExecDirectory returns the executable path for the running process.
func GetExecDirectory() (string, error) {
	exp, errExecutable := os.Executable()
	if errExecutable != nil {
		return "", fmt.Errorf("get executable path: %w", errExecutable)
	}
	return exp, nil
}
