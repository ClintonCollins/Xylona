package node

// install_path.go resolves the node-local default install root for managed
// game-server directories. Kept as a node wrapper so error messages and
// call sites still describe node-local path resolution.

import (
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/ClintonCollins/Xylona/internal/defaultpaths"
)

// resolveDefaultInstallPath returns the default root directory for managed
// game-server directories on this node. Computed from the host's GOOS plus
// HOME/USER/USERPROFILE env vars so hub-spoke deployments get a path
// appropriate for the node, not the controller.
func resolveDefaultInstallPath(goos, home, user, userProfile string) (string, error) {
	installPath, errResolve := defaultpaths.ResolveInstallPath(goos, home, user, userProfile)
	if errResolve == nil {
		return installPath, nil
	}
	if errors.Is(errResolve, defaultpaths.ErrMissingUnixHomeUser) {
		return "", errors.New("node: failed to resolve install root (no $HOME or $USER)")
	}
	if errors.Is(errResolve, defaultpaths.ErrMissingWindowsUserProfile) {
		return "", errors.New("node: failed to resolve install root (no %USERPROFILE%)")
	}
	if errors.Is(errResolve, defaultpaths.ErrUnsupportedOS) {
		return "", fmt.Errorf("node: unsupported OS for default install path: %s", goos)
	}
	return "", errResolve
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
