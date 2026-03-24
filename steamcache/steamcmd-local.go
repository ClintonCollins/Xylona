package steamcache

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
)

type keyValueTokenKind int

const (
	keyValueTokenString keyValueTokenKind = iota
	keyValueTokenOpenBrace
	keyValueTokenCloseBrace
)

type keyValueToken struct {
	kind  keyValueTokenKind
	value string
}

func fetchReleasesFromLocalSteamCMD(ctx context.Context, appID string) ([]SteamRelease, error) {
	steamCMDPath, errLookPath := exec.LookPath("steamcmd")
	if errLookPath != nil {
		steamCMDPath, errLookPath = exec.LookPath("steamcmd.exe")
	}
	if errLookPath != nil {
		return nil, fmt.Errorf("steamcmd executable not found: %w", errLookPath)
	}

	cmd := exec.CommandContext(
		ctx,
		steamCMDPath,
		"+login",
		"anonymous",
		"+app_info_print",
		appID,
		"+quit",
	)
	output, errOutput := cmd.CombinedOutput()
	if errOutput != nil {
		return nil, fmt.Errorf(
			"running steamcmd app_info_print for app %s: %w\n%s",
			appID,
			errOutput,
			strings.TrimSpace(string(output)),
		)
	}

	releases, errParse := parseLocalSteamCMDReleases(output, appID)
	if errParse != nil {
		return nil, fmt.Errorf("parsing local steamcmd output for app %s: %w", appID, errParse)
	}

	return releases, nil
}

func parseLocalSteamCMDReleases(output []byte, appID string) ([]SteamRelease, error) {
	startMarker := []byte(`"` + appID + `"`)
	startIndex := bytes.Index(output, startMarker)
	if startIndex < 0 {
		return nil, fmt.Errorf("app %s not found in steamcmd output", appID)
	}

	tokens, errTokens := tokenizeSteamCMDKeyValues(output[startIndex:])
	if errTokens != nil {
		return nil, errTokens
	}
	if len(tokens) < 2 {
		return nil, errors.New("steamcmd output does not contain an app block")
	}
	if tokens[0].kind != keyValueTokenString || tokens[0].value != appID {
		return nil, fmt.Errorf("steamcmd output did not start with app %s", appID)
	}
	if tokens[1].kind != keyValueTokenOpenBrace {
		return nil, errors.New("steamcmd app block is missing an opening brace")
	}

	tokenIndex := 2
	root, errParse := parseSteamCMDKeyValueObject(tokens, &tokenIndex)
	if errParse != nil {
		return nil, errParse
	}

	return parseLocalSteamReleases(root)
}

func tokenizeSteamCMDKeyValues(input []byte) ([]keyValueToken, error) {
	tokens := make([]keyValueToken, 0, len(input)/8)
	for index := 0; index < len(input); index++ {
		switch input[index] {
		case '"':
			index++
			start := index
			for index < len(input) && input[index] != '"' {
				if input[index] == '\\' && index+1 < len(input) {
					index += 2
					continue
				}
				index++
			}
			if index >= len(input) {
				return nil, errors.New("unterminated quoted string in steamcmd output")
			}
			tokens = append(tokens, keyValueToken{
				kind:  keyValueTokenString,
				value: string(input[start:index]),
			})
		case '{':
			tokens = append(tokens, keyValueToken{kind: keyValueTokenOpenBrace})
		case '}':
			tokens = append(tokens, keyValueToken{kind: keyValueTokenCloseBrace})
		}
	}
	return tokens, nil
}

func parseSteamCMDKeyValueObject(
	tokens []keyValueToken,
	tokenIndex *int,
) (map[string]any, error) {
	result := make(map[string]any)
	for *tokenIndex < len(tokens) {
		current := tokens[*tokenIndex]
		if current.kind == keyValueTokenCloseBrace {
			*tokenIndex++
			return result, nil
		}
		if current.kind != keyValueTokenString {
			return nil, fmt.Errorf("unexpected token kind %d in steamcmd object", current.kind)
		}

		key := current.value
		*tokenIndex++
		if *tokenIndex >= len(tokens) {
			return nil, fmt.Errorf("steamcmd object ended after key %q", key)
		}

		valueToken := tokens[*tokenIndex]
		if valueToken.kind == keyValueTokenString {
			result[key] = valueToken.value
			*tokenIndex++
			continue
		}
		if valueToken.kind != keyValueTokenOpenBrace {
			return nil, fmt.Errorf("steamcmd key %q is missing a value", key)
		}

		*tokenIndex++
		child, errChild := parseSteamCMDKeyValueObject(tokens, tokenIndex)
		if errChild != nil {
			return nil, errChild
		}
		result[key] = child
	}

	return nil, errors.New("steamcmd object ended without a closing brace")
}

func parseLocalSteamReleases(root map[string]any) ([]SteamRelease, error) {
	depots, okDepots := mapValue(root, "depots")
	if !okDepots {
		return nil, errors.New("steamcmd output is missing depots")
	}

	branches, okBranches := mapValue(depots, "branches")
	if !okBranches {
		return nil, errors.New("steamcmd output is missing depots.branches")
	}

	releasesByName := make(map[string]SteamRelease, len(branches))
	for branchName, rawBranch := range branches {
		branch, okBranch := rawBranch.(map[string]any)
		if !okBranch {
			continue
		}

		timeUpdated := stringValue(branch, "timeupdated")
		if strings.TrimSpace(timeUpdated) == "" {
			timeUpdated = stringValue(branch, "timebuildupdated")
		}

		releasesByName[branchName] = SteamRelease{
			Name:             branchName,
			DisplayLabel:     steamReleaseLabel(branchName, stringValue(branch, "description")),
			BuildID:          stringValue(branch, "buildid"),
			Description:      strings.TrimSpace(stringValue(branch, "description")),
			TimeUpdated:      strings.TrimSpace(timeUpdated),
			DepotManifestIDs: make(map[string]string),
		}
	}

	for depotID, rawDepot := range depots {
		if depotID == "branches" || depotID == "overridescddb" || depotID == "privatebranches" {
			continue
		}

		depot, okDepot := rawDepot.(map[string]any)
		if !okDepot {
			continue
		}

		manifests, okManifests := mapValue(depot, "manifests")
		if !okManifests {
			continue
		}

		for branchName, rawManifest := range manifests {
			release, okRelease := releasesByName[branchName]
			if !okRelease {
				continue
			}

			manifest, okManifest := rawManifest.(map[string]any)
			if !okManifest {
				continue
			}

			gid := stringValue(manifest, "gid")
			if gid == "" {
				continue
			}
			release.DepotManifestIDs[depotID] = gid
			releasesByName[branchName] = release
		}
	}

	releases := make([]SteamRelease, 0, len(releasesByName))
	for _, release := range releasesByName {
		releases = append(releases, release)
	}

	slices.SortStableFunc(releases, compareSteamReleases)
	return releases, nil
}

func mapValue(values map[string]any, key string) (map[string]any, bool) {
	rawValue, okValue := values[key]
	if !okValue {
		return nil, false
	}

	result, okResult := rawValue.(map[string]any)
	return result, okResult
}

func stringValue(values map[string]any, key string) string {
	rawValue, okValue := values[key]
	if !okValue {
		return ""
	}

	result, okResult := rawValue.(string)
	if !okResult {
		return ""
	}

	return strings.TrimSpace(result)
}
