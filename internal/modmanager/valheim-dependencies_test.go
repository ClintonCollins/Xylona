package modmanager

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/modproviders/thunderstore"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type valheimDependencyProvider struct {
	modproviders.ModProvider
	dependencies map[string][]string
	calls        []string
}

func (p *valheimDependencyProvider) GetVersions(context.Context, string, string, modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	return []modproviders.ModVersion{{VersionID: "2.0.0"}}, nil
}

func (p *valheimDependencyProvider) ExactVersion(_ context.Context, id, version string) (thunderstore.ExactPackage, error) {
	key := id + "@" + version
	p.calls = append(p.calls, key)
	deps, exists := p.dependencies[key]
	if !exists {
		return thunderstore.ExactPackage{}, fmt.Errorf("missing fixture package %s", key)
	}
	return thunderstore.ExactPackage{ID: id, Version: version, Dependencies: deps}, nil
}

func TestResolveValheimPackages(t *testing.T) {
	loader := thunderstore.ValheimLoaderID + "@" + thunderstore.ValheimLoaderVersion
	for _, test := range []struct {
		name         string
		version      string
		dependencies map[string][]string
		installed    []*models.InstalledMod
		want         []string
		errContains  string
	}{
		{
			name: "automatic loader and recursive dependencies",
			dependencies: map[string][]string{
				"Author-Mod@2.0.0":     {"Author-Library-1.0.0"},
				"Author-Library@1.0.0": {"denikson-BepInExPack_Valheim-5.4.2105"},
			},
			want: []string{loader, "Author-Library@1.0.0", "Author-Mod@2.0.0"},
		},
		{
			name:         "exact older root uses its own dependencies",
			version:      "1.0.0",
			dependencies: map[string][]string{"Author-Mod@1.0.0": {}},
			want:         []string{loader, "Author-Mod@1.0.0"},
		},
		{
			name: "shared dependency uses highest requirement",
			dependencies: map[string][]string{
				"Author-Mod@2.0.0":    {"Author-A-1.0.0", "Author-B-1.0.0"},
				"Author-A@1.0.0":      {"Author-Shared-1.0.0"},
				"Author-B@1.0.0":      {"Author-Shared-2.0.0"},
				"Author-Shared@1.0.0": {}, "Author-Shared@2.0.0": {},
			},
			want: []string{loader, "Author-Shared@2.0.0", "Author-A@1.0.0", "Author-B@1.0.0", "Author-Mod@2.0.0"},
		},
		{
			name:         "keeps newer installed dependency",
			dependencies: map[string][]string{"Author-Mod@2.0.0": {"Author-Library-1.0.0"}, "Author-Library@3.0.0": {}},
			installed:    []*models.InstalledMod{{Source: "thunderstore", SourceID: "Author-Library", InstalledVersionID: "3.0.0", Enabled: 1}},
			want:         []string{loader, "Author-Library@3.0.0", "Author-Mod@2.0.0"},
		},
		{
			name:         "pinned dependency cannot be upgraded silently",
			dependencies: map[string][]string{"Author-Mod@2.0.0": {"Author-Library-2.0.0"}},
			installed:    []*models.InstalledMod{{Source: "thunderstore", SourceID: "Author-Library", InstalledVersionID: "1.0.0", Enabled: 1, PinnedVersion: null.From("1.0.0")}},
			errContains:  "pinned",
		},
		{
			name:         "disabled dependency requires explicit enable",
			dependencies: map[string][]string{"Author-Mod@2.0.0": {"Author-Library-1.0.0"}},
			installed:    []*models.InstalledMod{{Source: "thunderstore", SourceID: "Author-Library", InstalledVersionID: "1.0.0"}},
			errContains:  "enable dependency",
		},
		{
			name:         "cycle",
			dependencies: map[string][]string{"Author-Mod@2.0.0": {"Author-Library-1.0.0"}, "Author-Library@1.0.0": {"Author-Mod-2.0.0"}},
			errContains:  "cycle",
		},
		{
			name:         "unsupported newer loader",
			dependencies: map[string][]string{"Author-Mod@2.0.0": {"denikson-BepInExPack_Valheim-6.0.0"}},
			errContains:  "requires version",
		},
		{
			name:         "malformed dependency",
			dependencies: map[string][]string{"Author-Mod@2.0.0": {"../invalid-1.0.0"}},
			errContains:  "invalid Thunderstore dependency",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &valheimDependencyProvider{dependencies: test.dependencies}
			provider.dependencies[loader] = []string{}
			packages, errResolve := resolveValheimPackages(t.Context(), provider, "Author-Mod", test.version, test.installed)
			if test.errContains != "" {
				if errResolve == nil || !strings.Contains(errResolve.Error(), test.errContains) {
					t.Fatalf("error = %v, want %q", errResolve, test.errContains)
				}
				return
			}
			if errResolve != nil {
				t.Fatal(errResolve)
			}
			got := make([]string, 0, len(packages))
			for _, pkg := range packages {
				got = append(got, pkg.ID+"@"+pkg.Version)
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("install order = %v, want %v", got, test.want)
			}
		})
	}
}
