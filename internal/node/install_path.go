package node

// install_path.go resolves the node-local default install root for managed
// game-server directories. Kept in pkg/node (rather than reused from
// pkg/actions) to avoid an import cycle: the actions package depends on
// pkg/node, not the other way around. The logic mirrors
// actions.resolveDefaultInstallPath and the two functions should stay in
// lockstep if either is changed.

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
)

// resolveDefaultInstallPath returns the default root directory for managed
// game-server directories on this node. Computed from the host's GOOS plus
// HOME/USER/USERPROFILE env vars so hub-spoke deployments get a path
// appropriate for the node, not the controller.
func resolveDefaultInstallPath(goos, home, user, userProfile string) (string, error) {
	switch goos {
	case "linux", "darwin":
		if home == "" && user == "" {
			return "", errors.New("node: failed to resolve install root (no $HOME or $USER)")
		}
		if home != "" {
			return path.Join(home, "xylona"), nil
		}
		return path.Join("/home", user, "xylona"), nil
	case "windows":
		if userProfile == "" {
			return "", errors.New("node: failed to resolve install root (no %USERPROFILE%)")
		}
		return filepath.Join(userProfile, "Xylona"), nil
	default:
		return "", fmt.Errorf("node: unsupported OS for default install path: %s", goos)
	}
}

// DefaultInstallPath returns resolveDefaultInstallPath for the host process's
// own OS and env. Exposed for snapshot population; callers outside the node
// package consume the value through NodeSnapshot.DefaultInstallPath.
func DefaultInstallPath() (string, error) {
	return resolveDefaultInstallPath(
		runtime.GOOS,
		os.Getenv("HOME"),
		os.Getenv("USER"),
		os.Getenv("USERPROFILE"),
	)
}
