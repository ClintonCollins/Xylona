// Package processenv builds the small, explicit host environment inherited by
// child processes and appends validated per-process variables.
package processenv

import (
	"fmt"
	"slices"
	"strings"
)

var (
	commonChildEnvironmentKeys = []string{
		"PATH",
		"LANG",
		"LANGUAGE",
	}
	unixChildEnvironmentKeys = []string{
		"HOME",
		"TMPDIR",
	}
	windowsChildEnvironmentKeys = []string{
		"SYSTEMROOT",
		"WINDIR",
		"COMSPEC",
		"PATHEXT",
		"USERPROFILE",
		"APPDATA",
		"LOCALAPPDATA",
		"TEMP",
		"TMP",
	}
)

// Build returns only the host variables required to locate programs and
// provide normal locale, home, and temporary-directory behavior.
func Build(goos string, sourceEnv []string) []string {
	childEnvironment := make([]string, 0, len(commonChildEnvironmentKeys)+len(unixChildEnvironmentKeys)+len(windowsChildEnvironmentKeys))
	addedKeys := make(map[string]struct{})

	appendKey := func(key string) {
		lookupKey := normalizeName(goos, key)
		_, exists := addedKeys[lookupKey]
		if exists {
			return
		}

		value, ok := lookupValue(goos, sourceEnv, key)
		if !ok {
			return
		}

		childEnvironment = append(childEnvironment, fmt.Sprintf("%s=%s", key, value))
		addedKeys[lookupKey] = struct{}{}
	}

	for _, key := range commonChildEnvironmentKeys {
		appendKey(key)
	}
	for _, key := range localeKeys(sourceEnv) {
		appendKey(key)
	}

	if goos == "windows" {
		for _, key := range windowsChildEnvironmentKeys {
			appendKey(key)
		}
	} else {
		for _, key := range unixChildEnvironmentKeys {
			appendKey(key)
		}
	}

	return childEnvironment
}

// Append adds entries in stable key order. Callers must validate variables
// before passing them here.
func Append(baseEnv []string, launchEnv map[string]string) []string {
	if len(launchEnv) == 0 {
		return baseEnv
	}

	childEnvironment := append([]string(nil), baseEnv...)
	keys := make([]string, 0, len(launchEnv))
	for key := range launchEnv {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	for _, key := range keys {
		childEnvironment = append(childEnvironment, fmt.Sprintf("%s=%s", key, launchEnv[key]))
	}
	return childEnvironment
}

func localeKeys(sourceEnv []string) []string {
	keys := make([]string, 0)
	for _, entry := range sourceEnv {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || !strings.HasPrefix(key, "LC_") || key == "LC_" {
			continue
		}
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return slices.Compact(keys)
}

func lookupValue(goos string, sourceEnv []string, targetKey string) (string, bool) {
	for _, entry := range sourceEnv {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		if normalizeName(goos, key) == normalizeName(goos, targetKey) {
			return value, true
		}
	}
	return "", false
}

func normalizeName(goos string, name string) string {
	if goos == "windows" {
		return strings.ToUpper(name)
	}
	return name
}
