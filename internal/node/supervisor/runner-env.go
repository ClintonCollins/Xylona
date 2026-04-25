package supervisor

import (
	"fmt"
	"slices"
	"strings"
)

var (
	commonChildEnvironmentKeys = []string{
		`PATH`,
		`LANG`,
		`LANGUAGE`,
	}
	unixChildEnvironmentKeys = []string{
		`HOME`,
		`TMPDIR`,
	}
	windowsChildEnvironmentKeys = []string{
		`SYSTEMROOT`,
		`WINDIR`,
		`COMSPEC`,
		`PATHEXT`,
		`USERPROFILE`,
		`APPDATA`,
		`LOCALAPPDATA`,
		`TEMP`,
		`TMP`,
	}
)

func buildChildEnvironment(runtime Runtime, sourceEnv []string) []string {
	childEnvironment := make([]string, 0, len(commonChildEnvironmentKeys)+len(unixChildEnvironmentKeys)+len(windowsChildEnvironmentKeys))
	addedKeys := make(map[string]struct{})

	appendKey := func(key string) {
		lookupKey := key
		if runtime == RuntimeWindows {
			lookupKey = strings.ToUpper(key)
		}
		if _, exists := addedKeys[lookupKey]; exists {
			return
		}

		value, ok := lookupEnvironmentValue(runtime, sourceEnv, key)
		if !ok {
			return
		}

		childEnvironment = append(childEnvironment, fmt.Sprintf(`%s=%s`, key, value))
		addedKeys[lookupKey] = struct{}{}
	}

	for _, key := range commonChildEnvironmentKeys {
		appendKey(key)
	}

	for _, key := range localeEnvironmentKeys(sourceEnv) {
		appendKey(key)
	}

	switch runtime {
	case RuntimeWindows:
		for _, key := range windowsChildEnvironmentKeys {
			appendKey(key)
		}
	default:
		for _, key := range unixChildEnvironmentKeys {
			appendKey(key)
		}
	}

	return childEnvironment
}

func localeEnvironmentKeys(sourceEnv []string) []string {
	localeKeys := make([]string, 0)
	for _, entry := range sourceEnv {
		key, _, ok := strings.Cut(entry, `=`)
		if !ok {
			continue
		}
		if !strings.HasPrefix(key, `LC_`) {
			continue
		}
		if key == `LC_` {
			continue
		}
		localeKeys = append(localeKeys, key)
	}

	slices.Sort(localeKeys)
	return slices.Compact(localeKeys)
}

func lookupEnvironmentValue(runtime Runtime, sourceEnv []string, targetKey string) (string, bool) {
	for _, entry := range sourceEnv {
		key, value, ok := strings.Cut(entry, `=`)
		if !ok {
			continue
		}

		if runtime == RuntimeWindows {
			if strings.EqualFold(key, targetKey) {
				return value, true
			}
			continue
		}

		if key == targetKey {
			return value, true
		}
	}

	return ``, false
}
