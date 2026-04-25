// Package defaultpaths contains shared default path resolution helpers.
package defaultpaths

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
)

var (
	ErrMissingUnixHomeUser       = errors.New("missing HOME or USER")
	ErrMissingWindowsUserProfile = errors.New("missing USERPROFILE")
	ErrUnsupportedOS             = errors.New("unsupported operating system")
)

// ResolveInstallPath resolves the default managed server root for a host OS.
func ResolveInstallPath(goos string, home string, user string, userProfile string) (string, error) {
	switch goos {
	case "linux", "darwin":
		if home == "" && user == "" {
			return "", ErrMissingUnixHomeUser
		}
		if home != "" {
			return path.Join(home, "xylona"), nil
		}
		return path.Join("/home", user, "xylona"), nil
	case "windows":
		if userProfile == "" {
			return "", ErrMissingWindowsUserProfile
		}
		return filepath.Join(userProfile, "Xylona"), nil
	default:
		return "", fmt.Errorf("%w: %s", ErrUnsupportedOS, goos)
	}
}
