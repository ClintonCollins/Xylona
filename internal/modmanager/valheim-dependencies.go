package modmanager

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/mod/semver"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// resolveValheimPackages plans the complete dependency graph before changing files.
// Thunderstore dependency versions are minimum requirements; the loader is the
// supported, verified distribution even when a mod declares an older loader.
func resolveValheimPackages(ctx context.Context, provider modproviders.ModProvider, sourceID, versionID string, installed []*models.InstalledMod) ([]thunderstore.ExactPackage, error) {
	exact, supported := provider.(interface {
		ExactVersion(context.Context, string, string) (thunderstore.ExactPackage, error)
	})
	if !supported {
		return nil, errors.New("valheim provider does not support version-specific dependencies")
	}
	if !validValheimPackageID(sourceID) {
		return nil, errors.New("invalid Thunderstore package ID")
	}
	if sourceID == thunderstore.ValheimLoaderID && versionID == "" {
		versionID = thunderstore.ValheimLoaderVersion
	}
	if versionID == "" {
		versions, errVersions := provider.GetVersions(ctx, sourceID, "", modproviders.SearchParams{"community": "valheim"})
		if errVersions != nil {
			return nil, fmt.Errorf("resolve latest package version: %w", errVersions)
		}
		if len(versions) == 0 {
			return nil, errors.New("no versions available for this package")
		}
		versionID = versions[0].VersionID
	}
	if !validValheimPackageVersion(versionID) {
		return nil, errors.New("invalid Thunderstore package version")
	}
	if sourceID == thunderstore.ValheimLoaderID && versionID != thunderstore.ValheimLoaderVersion {
		return nil, fmt.Errorf("BepInEx requires supported version %s", thunderstore.ValheimLoaderVersion)
	}
	existing := make(map[string]*models.InstalledMod)
	for _, mod := range installed {
		if mod.Source == "thunderstore" {
			existing[mod.SourceID] = mod
		}
	}
	resolved := make(map[string]thunderstore.ExactPackage)
	visiting := make(map[string]bool)
	requests := 0
	var resolve func(string, string) error
	resolve = func(id, minimum string) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !validValheimPackageID(id) || !validValheimPackageVersion(minimum) {
			return errors.New("invalid Thunderstore dependency reference")
		}
		if visiting[id] {
			return fmt.Errorf("dependency cycle involving %s", id)
		}
		selected := minimum
		mod := existing[id]
		if id == sourceID {
			selected = versionID
		} else if mod != nil {
			if mod.Enabled != 1 {
				return fmt.Errorf("enable dependency %s before installing this mod", id)
			}
			if validValheimPackageVersion(mod.InstalledVersionID) && semver.Compare("v"+mod.InstalledVersionID, "v"+selected) > 0 {
				selected = mod.InstalledVersionID
			}
			if mod.PinnedVersion.GetOr("") != "" && semver.Compare("v"+mod.InstalledVersionID, "v"+minimum) < 0 {
				return fmt.Errorf("dependency %s is pinned below required version %s", id, minimum)
			}
		}
		if id == thunderstore.ValheimLoaderID {
			selected = thunderstore.ValheimLoaderVersion
		}
		if semver.Compare("v"+selected, "v"+minimum) < 0 {
			return fmt.Errorf("dependency %s requires version %s or newer", id, minimum)
		}
		previous, exists := resolved[id]
		if exists && semver.Compare("v"+previous.Version, "v"+selected) >= 0 {
			return nil
		}
		if len(resolved)+len(visiting) >= 128 || requests >= 512 {
			return errors.New("valheim dependency graph exceeds the package limit")
		}
		requests++
		pkg, errMetadata := exact.ExactVersion(ctx, id, selected)
		if errMetadata != nil {
			return fmt.Errorf("resolve %s %s: %w", id, selected, errMetadata)
		}
		if pkg.ID != id || pkg.Version != selected || len(pkg.Dependencies) > 128 {
			return errors.New("invalid version-specific package metadata")
		}
		visiting[id] = true
		for _, dependency := range pkg.Dependencies {
			index := strings.LastIndexByte(dependency, '-')
			if index < 0 {
				return errors.New("invalid Thunderstore dependency reference")
			}
			errDependency := resolve(dependency[:index], dependency[index+1:])
			if errDependency != nil {
				return errDependency
			}
		}
		delete(visiting, id)
		resolved[id] = pkg
		return nil
	}
	errLoader := resolve(thunderstore.ValheimLoaderID, thunderstore.ValheimLoaderVersion)
	if errLoader != nil {
		return nil, errLoader
	}
	errRoot := resolve(sourceID, versionID)
	if errRoot != nil {
		return nil, errRoot
	}
	// Rebuild the order from the final versions so a superseded dependency's
	// obsolete children are not installed, and cross-branch cycles are rejected.
	ordered := make([]thunderstore.ExactPackage, 0, len(resolved))
	visited := make(map[string]bool)
	var appendPackage func(string) error
	appendPackage = func(id string) error {
		if visiting[id] {
			return fmt.Errorf("dependency cycle involving %s", id)
		}
		if visited[id] {
			return nil
		}
		visiting[id] = true
		pkg := resolved[id]
		for _, dependency := range pkg.Dependencies {
			index := strings.LastIndexByte(dependency, '-')
			errAppend := appendPackage(dependency[:index])
			if errAppend != nil {
				return errAppend
			}
		}
		delete(visiting, id)
		visited[id] = true
		ordered = append(ordered, pkg)
		return nil
	}
	errAppendLoader := appendPackage(thunderstore.ValheimLoaderID)
	if errAppendLoader != nil {
		return nil, errAppendLoader
	}
	errAppendRoot := appendPackage(sourceID)
	return ordered, errAppendRoot
}

func validValheimPackageID(id string) bool {
	namespace, name, found := strings.Cut(id, "-")
	if !found || namespace == "" || name == "" {
		return false
	}
	for _, part := range []string{namespace, name} {
		for _, char := range part {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '_' {
				return false
			}
		}
	}
	return true
}

func validValheimPackageVersion(version string) bool {
	return strings.Count(version, ".") == 2 && semver.IsValid("v"+version) && !strings.ContainsAny(version, "+-")
}
