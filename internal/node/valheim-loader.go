package node

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func prepareInstalledValheimLoader(directory, platform string, trusted map[string]string) error {
	root, errRoot := os.OpenRoot(directory)
	if errRoot != nil {
		return fmt.Errorf("open Valheim installation: %w", errRoot)
	}
	defer closeMutationRoot(root)
	loaderPresent := false
	for _, name := range []string{"BepInEx/core", "BepInEx/plugins", "BepInEx/patchers", "winhttp.dll", "doorstop_libs", "doorstop_config.ini"} {
		info, errStat := root.Stat(name)
		if errStat == nil {
			if info.IsDir() {
				hasFiles := false
				errWalk := fs.WalkDir(root.FS(), name, func(_ string, entry fs.DirEntry, errEntry error) error {
					if errEntry != nil {
						return errEntry
					}
					if !entry.IsDir() {
						hasFiles = true
						return fs.SkipAll
					}
					return nil
				})
				if errWalk != nil {
					return fmt.Errorf("inspect Valheim loader directory %s: %w", name, errWalk)
				}
				if !hasFiles {
					continue
				}
			}
			loaderPresent = true
			break
		}
		if !errors.Is(errStat, os.ErrNotExist) {
			return fmt.Errorf("inspect Valheim loader %s: %w", name, errStat)
		}
	}
	if !loaderPresent {
		return nil
	}
	required := []string{"BepInEx/core/BepInEx.Preloader.dll", "BepInEx/core/BepInEx.dll"}
	switch platform {
	case "windows":
		required = append(required, "winhttp.dll")
	case "linux":
		required = append(required, "doorstop_libs/libdoorstop_x64.so")
	default:
		return errors.New("BepInEx requires native Windows or Linux")
	}
	for _, name := range required {
		info, errStat := root.Stat(name)
		if errStat != nil {
			return fmt.Errorf("incomplete BepInEx installation: reinstall BepInExPack_Valheim; check %s: %w", name, errStat)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return fmt.Errorf("invalid BepInEx runtime file %s: reinstall BepInExPack_Valheim", name)
		}
	}
	errConfig := ensureValheimLoaderConfig(root, "BepInEx/config/BepInEx.cfg", "[Logging.Console]\nEnabled = true\nForceBepInExTTYDriver = true\n\n[Preloader.Entrypoint]\nType = GameObject\n")
	if errConfig != nil {
		return errConfig
	}
	if platform == "windows" {
		errDoorstop := ensureValheimLoaderConfig(root, "doorstop_config.ini", "[General]\nenabled = true\ntarget_assembly = BepInEx\\core\\BepInEx.Preloader.dll\n")
		if errDoorstop != nil {
			return errDoorstop
		}
		data, errRead := root.ReadFile("doorstop_config.ini")
		if errRead != nil {
			return fmt.Errorf("read BepInEx Doorstop configuration: %w", errRead)
		}
		section, enabled, target := "", "", ""
		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "[") {
				section = strings.ToLower(line)
			}
			key, value, found := strings.Cut(line, "=")
			if section != "[general]" || !found {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(key)) {
			case "enabled":
				enabled = strings.ToLower(strings.TrimSpace(value))
			case "target_assembly":
				target = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
			}
		}
		if enabled != "true" || !strings.EqualFold(target, "BepInEx/core/BepInEx.Preloader.dll") {
			return errors.New("BepInEx doorstop_config.ini must set [General] enabled = true and target_assembly = BepInEx\\core\\BepInEx.Preloader.dll")
		}
		return nil
	}
	trusted["DOORSTOP_ENABLED"] = "1"
	trusted["DOORSTOP_TARGET_ASSEMBLY"] = filepath.Join(directory, "BepInEx", "core", "BepInEx.Preloader.dll")
	trusted["LD_LIBRARY_PATH"] = filepath.Join(directory, "linux64") + ":" + filepath.Join(directory, "doorstop_libs")
	trusted["LD_PRELOAD"] = filepath.Join(directory, "doorstop_libs", "libdoorstop_x64.so")
	return nil
}

func ensureValheimLoaderConfig(root *os.Root, name, defaults string) error {
	info, errStat := root.Stat(name)
	if errStat == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("BepInEx configuration %s must be a regular file", name)
		}
		return nil
	}
	if !errors.Is(errStat, os.ErrNotExist) {
		return fmt.Errorf("inspect BepInEx configuration %s: %w", name, errStat)
	}
	errDirectory := root.MkdirAll(filepath.Dir(name), 0o750)
	if errDirectory != nil {
		return fmt.Errorf("create BepInEx configuration directory: %w", errDirectory)
	}
	file, errOpen := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if errOpen != nil {
		return fmt.Errorf("create BepInEx configuration %s: %w", name, errOpen)
	}
	_, errWrite := file.WriteString(defaults)
	errClose := file.Close()
	if errWrite != nil || errClose != nil {
		return fmt.Errorf("write BepInEx configuration %s: %w", name, errors.Join(errWrite, errClose))
	}
	return nil
}
