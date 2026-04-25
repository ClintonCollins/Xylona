package node

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/rs/zerolog/log"
)

var (
	errMinecraftVersionJSONNotFound = errors.New("node: minecraft version.json not found")
	steamBuildIDPattern             = regexp.MustCompile(`"buildid"\s+"(\d+)"`)
)

type minecraftVersionJSON struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProbeInstalledVersion inspects node-local files for a narrow set of
// installed-version markers. It does not query providers or depend on
// controller/database state.
func (n *Node) ProbeInstalledVersion(req InstalledVersionProbeRequest) (InstalledVersionProbeResult, error) {
	switch req.Kind {
	case InstalledVersionProbeKindMinecraftJar:
		return n.probeMinecraftJar(req)
	case InstalledVersionProbeKindSteamManifest:
		return n.probeSteamManifest(req)
	default:
		return InstalledVersionProbeResult{}, ErrInvalidPath
	}
}

func (n *Node) probeMinecraftJar(req InstalledVersionProbeRequest) (InstalledVersionProbeResult, error) {
	relativePaths := append([]string(nil), req.RelativePaths...)
	if len(relativePaths) == 0 {
		relativePaths = []string{"minecraft_server.jar"}
	}

	for _, relativePath := range relativePaths {
		validatedPath, ok, errPath := probePath(relativePath)
		if errPath != nil {
			return InstalledVersionProbeResult{}, errPath
		}
		if !ok {
			continue
		}

		fullPath, errResolve := resolveWithinRoot(req.Directory, validatedPath)
		if errResolve != nil {
			return InstalledVersionProbeResult{}, errResolve
		}
		version, errVersion := readMinecraftJarVersion(fullPath)
		if errVersion != nil {
			if errors.Is(errVersion, os.ErrNotExist) ||
				errors.Is(errVersion, errMinecraftVersionJSONNotFound) ||
				errors.Is(errVersion, zip.ErrFormat) {
				continue
			}
			return InstalledVersionProbeResult{}, errVersion
		}
		if strings.TrimSpace(version) == "" {
			continue
		}
		return InstalledVersionProbeResult{
			Found:      true,
			Version:    strings.TrimSpace(version),
			SourcePath: filepath.ToSlash(validatedPath),
		}, nil
	}
	return InstalledVersionProbeResult{}, nil
}

func (n *Node) probeSteamManifest(req InstalledVersionProbeRequest) (InstalledVersionProbeResult, error) {
	relativePaths, errPaths := steamManifestProbePaths(req.Directory, req.RelativePaths, req.PreferredSteamAppID)
	if errPaths != nil {
		return InstalledVersionProbeResult{}, errPaths
	}

	for _, relativePath := range relativePaths {
		validatedPath, ok, errPath := probePath(relativePath)
		if errPath != nil {
			return InstalledVersionProbeResult{}, errPath
		}
		if !ok {
			continue
		}

		fullPath, errResolve := resolveWithinRoot(req.Directory, validatedPath)
		if errResolve != nil {
			return InstalledVersionProbeResult{}, errResolve
		}
		data, errRead := os.ReadFile(fullPath)
		if errRead != nil {
			if errors.Is(errRead, os.ErrNotExist) {
				continue
			}
			return InstalledVersionProbeResult{}, fmt.Errorf("node: read steam manifest: %w", errRead)
		}
		buildID := readSteamBuildID(data)
		if buildID == "" {
			continue
		}
		return InstalledVersionProbeResult{
			Found:      true,
			Version:    buildID,
			SourcePath: filepath.ToSlash(validatedPath),
		}, nil
	}
	return InstalledVersionProbeResult{}, nil
}

func probePath(relativePath string) (string, bool, error) {
	validatedPath, errPath := validateLocalPath(strings.ReplaceAll(strings.TrimSpace(relativePath), `\`, "/"))
	if errPath != nil {
		return "", false, errPath
	}
	if validatedPath == "" || validatedPath == "." {
		return "", false, nil
	}
	return validatedPath, true, nil
}

func readMinecraftJarVersion(jarPath string) (string, error) {
	reader, errOpen := zip.OpenReader(jarPath)
	if errOpen != nil {
		return "", fmt.Errorf("node: open minecraft jar: %w", errOpen)
	}
	defer func() {
		errClose := reader.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close minecraft jar")
		}
	}()

	for _, file := range reader.File {
		if file.Name != "version.json" {
			continue
		}
		versionReader, errFile := file.Open()
		if errFile != nil {
			return "", fmt.Errorf("node: open minecraft version.json: %w", errFile)
		}
		var version minecraftVersionJSON
		errDecode := json.NewDecoder(versionReader).Decode(&version)
		errClose := versionReader.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("node: close minecraft version.json")
		}
		if errDecode != nil {
			return "", fmt.Errorf("node: decode minecraft version.json: %w", errDecode)
		}
		if strings.TrimSpace(version.Name) != "" {
			return strings.TrimSpace(version.Name), nil
		}
		return strings.TrimSpace(version.ID), nil
	}
	return "", errMinecraftVersionJSONNotFound
}

func steamManifestProbePaths(directory string, explicitPaths []string, preferredAppID string) ([]string, error) {
	if len(explicitPaths) > 0 {
		return append([]string(nil), explicitPaths...), nil
	}

	appID := strings.TrimSpace(preferredAppID)
	if appID != "" {
		return []string{
			fmt.Sprintf("appmanifest_%s.acf", appID),
			filepath.ToSlash(filepath.Join("steamapps", fmt.Sprintf("appmanifest_%s.acf", appID))),
		}, nil
	}

	paths := []string{}
	for _, relativeDir := range []string{"", "steamapps"} {
		searchDir := filepath.Join(directory, relativeDir)
		pattern := filepath.Join(searchDir, "appmanifest_*.acf")
		matches, errGlob := filepath.Glob(pattern)
		if errGlob != nil {
			return nil, fmt.Errorf("node: glob steam manifests: %w", errGlob)
		}
		for _, match := range matches {
			relativePath, errRel := filepath.Rel(directory, match)
			if errRel != nil {
				return nil, fmt.Errorf("node: calculate steam manifest relative path: %w", errRel)
			}
			paths = append(paths, filepath.ToSlash(relativePath))
		}
	}
	return paths, nil
}

func readSteamBuildID(data []byte) string {
	match := steamBuildIDPattern.FindSubmatch(data)
	if match == nil {
		return ""
	}
	return strings.TrimSpace(string(match[1]))
}
