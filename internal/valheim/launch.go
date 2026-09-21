// Package valheim validates native server launch.
package valheim

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// NativeRuntime identifies the supported native dedicated-server launch.
const NativeRuntime = "valheim-native"

// PrepareNative resolves only the native distribution and its isolated save root.
func PrepareNative(platform, directory, executable, serverID string, args []string, environment map[string]string) (string, string, map[string]string, error) {
	name := "valheim_server.exe"
	if platform == "linux" {
		name = "valheim_server.x86_64"
	} else if platform != "windows" {
		return "", "", nil, errors.New("valheim requires native Windows or Linux")
	}
	root, errRoot := filepath.EvalSymlinks(directory)
	if errRoot != nil {
		return "", "", nil, fmt.Errorf("resolve Valheim installation: %w", errRoot)
	}
	root, errRoot = filepath.Abs(root)
	if errRoot != nil {
		return "", "", nil, fmt.Errorf("resolve absolute installation: %w", errRoot)
	}
	requested := executable
	if !filepath.IsAbs(requested) {
		requested = filepath.Join(root, requested)
	}
	resolved, errExecutable := filepath.EvalSymlinks(requested)
	if errExecutable != nil {
		return "", "", nil, fmt.Errorf("valheim executable missing; verify the installed distribution: %w", errExecutable)
	}
	info, errStat := os.Stat(resolved)
	if errStat != nil || !info.Mode().IsRegular() {
		return "", "", nil, errors.New("valheim executable must be a regular file")
	}
	if resolved != filepath.Join(root, name) {
		return "", "", nil, errors.New("managed Valheim launch requires the native executable in the installation root")
	}
	data := filepath.Join(root, "data")
	errData := os.MkdirAll(data, 0o750)
	if errData != nil {
		return "", "", nil, fmt.Errorf("prepare Valheim save directory: %w", errData)
	}
	resolvedData, errData := filepath.EvalSymlinks(data)
	if errData != nil || resolvedData != data {
		return "", "", nil, errors.New("valheim data directory must not redirect outside its managed location")
	}
	errArgs := ValidateArgs(args, serverID)
	if errArgs != nil {
		return "", "", nil, errArgs
	}
	for key, value := range environment {
		if strings.EqualFold(key, "SteamAppId") && value != "892970" {
			return "", "", nil, errors.New("valheim requires SteamAppId=892970")
		}
	}
	trusted := map[string]string{"SteamAppId": "892970"}
	if platform == "linux" {
		for _, relative := range []string{"UnityPlayer.so", "linux64/steamclient.so"} {
			path, errPath := filepath.EvalSymlinks(filepath.Join(root, relative))
			if errPath != nil {
				return "", "", nil, fmt.Errorf("valheim runtime file %s missing; verify the installed distribution", relative)
			}
			libraryInfo, errLibraryStat := os.Stat(path)
			if errLibraryStat != nil || !libraryInfo.Mode().IsRegular() {
				return "", "", nil, errors.New("valheim runtime library must be a regular file")
			}
			rel, errRelative := filepath.Rel(root, path)
			if errRelative != nil || !filepath.IsLocal(rel) {
				return "", "", nil, errors.New("valheim runtime libraries must remain inside the installation")
			}
		}
		trusted["LD_LIBRARY_PATH"] = filepath.Join(root, "linux64")
	}
	return root, resolved, trusted, nil
}

// ValidateArgs protects managed switches while preserving unrelated expert arguments.
func ValidateArgs(args []string, serverID string) error {
	values := map[string]string{}
	managed := map[string]bool{"-name": true, "-password": true, "-port": true, "-savedir": true, "-instanceid": true, "-logfile": true, "-nographics": true, "-batchmode": true, "-world": true, "-public": true, "-crossplay": true}
	for index := 0; index < len(args); index++ {
		flag := strings.ToLower(args[index])
		key, _, hasEquals := strings.Cut(flag, "=")
		if !managed[key] {
			continue
		}
		canonical := key
		if key == "-logfile" {
			canonical = "-logFile"
		}
		if args[index] != canonical || hasEquals {
			return errors.New("valheim managed switches must use their documented spelling and separate argument values")
		}
		_, duplicate := values[key]
		if duplicate {
			return fmt.Errorf("duplicate Valheim switch %s", key)
		}
		if key == "-nographics" || key == "-batchmode" || key == "-crossplay" {
			values[key] = "true"
			continue
		}
		index++
		if index >= len(args) {
			return fmt.Errorf("missing value for Valheim switch %s", key)
		}
		values[key] = args[index]
	}
	for _, key := range []string{"-name", "-port", "-savedir", "-instanceid", "-logfile", "-nographics", "-batchmode", "-world"} {
		if values[key] == "" {
			return fmt.Errorf("required Valheim switch %s missing", key)
		}
	}
	if values["-savedir"] != "data" || values["-instanceid"] != serverID || values["-logfile"] != "-" {
		return errors.New("valheim save directory, instance ID and console logging must retain their managed values")
	}
	port, errPort := strconv.Atoi(values["-port"])
	if errPort != nil || port < 1 || port > 65534 {
		return errors.New("valheim game port must be between 1 and 65534 and reserves the next UDP port")
	}
	world := values["-world"]
	if world == "." || world == ".." || strings.ContainsAny(world, "/\\<>:\"|?*") || strings.HasSuffix(world, ".") || strings.HasSuffix(world, " ") || strings.ContainsFunc(world, unicode.IsControl) {
		return errors.New("valheim world name must be one safe filename component")
	}
	device, _, _ := strings.Cut(strings.ToUpper(world), ".")
	if device == "CON" || device == "PRN" || device == "AUX" || device == "NUL" || (len(device) == 4 && (strings.HasPrefix(device, "COM") || strings.HasPrefix(device, "LPT")) && device[3] >= '1' && device[3] <= '9') {
		return errors.New("valheim world name must not be a reserved device name")
	}
	public := values["-public"]
	if public != "" && public != "0" && public != "1" {
		return errors.New("valheim public discovery must be 0 or 1")
	}
	password := values["-password"]
	if password != "" {
		return ValidateJoinPassword(password, values["-name"])
	}
	return nil
}
