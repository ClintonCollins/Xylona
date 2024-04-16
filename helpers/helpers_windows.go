package helpers

import (
	"fmt"
	"os"
	"path"
)

func GetAppDirectory() (string, error) {
	appData := os.Getenv("HOMEDRIVE")
	if appData == "" {
		return "", fmt.Errorf("app data doesn't exist")
	}
	appData = path.Join(appData, "Xylona")
	return appData, nil
}

func GetOperatingDirectory() (string, error) {
	userProfile := os.Getenv("USERPROFILE")
	if userProfile == "" {
		return "", fmt.Errorf("user profile doesn't exist")
	}
	userProfile = path.Join(userProfile, "Xylona")
	return userProfile, nil
}

func GetHomeDirectory() (string, error) {
	homeDirectory := os.Getenv("HOMEDRIVE")
	if homeDirectory == "" {
		return "", fmt.Errorf("user profile doesn't exist")
	}
	homeDirectory = path.Join(homeDirectory, "Xylona", "Users")
	return homeDirectory, nil
}

func GetExecDirectory() (string, error) {
	exp, err := os.Executable()
	if err != nil {
		return "", err
	}
	return exp, nil
}
