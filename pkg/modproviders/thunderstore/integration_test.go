//go:build integration

package thunderstore

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_Search_BepInExPackValheim(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := New()
	results, errSearch := p.Search(context.Background(), "BepInExPack", map[string]any{"community": "valheim"})
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results, want at least one")
	}

	found := false
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Name), "bepinex") ||
			strings.Contains(strings.ToLower(r.SourceID), "bepinex") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search() did not return a BepInEx result; got: %+v", results)
	}
}

func TestIntegration_GetModDetails_BepInExPackValheim(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// First search to discover the canonical full_name.
	p := New()
	results, errSearch := p.Search(context.Background(), "BepInExPack_Valheim", map[string]any{"community": "valheim"})
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}

	var sourceID string
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.SourceID), "bepinexpack_valheim") {
			sourceID = r.SourceID
			break
		}
	}
	if sourceID == "" {
		t.Skip("BepInExPack_Valheim not found in search results — skipping details test")
	}

	details, errDetails := p.GetModDetails(context.Background(), sourceID, map[string]any{"community": "valheim"})
	if errDetails != nil {
		t.Fatalf("GetModDetails(%q) error = %v", sourceID, errDetails)
	}
	if details == nil {
		t.Fatal("GetModDetails() = nil, want non-nil")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if details.SourceID != sourceID {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, sourceID)
	}
	if len(details.Versions) == 0 {
		t.Error("details.Versions is empty, want at least one version")
	}
}
