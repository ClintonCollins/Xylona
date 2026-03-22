//go:build integration

package hangar

import (
	"context"
	"strings"
	"testing"
)

func TestIntegration_Search_WorldEdit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := New()
	results, errSearch := p.Search(context.Background(), "WorldEdit", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results, want at least one")
	}

	found := false
	for _, r := range results {
		if strings.EqualFold(r.Name, "WorldEdit") || strings.Contains(strings.ToLower(r.SourceID), "worldedit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search() did not return WorldEdit in results; got: %+v", results)
	}
}

func TestIntegration_GetModDetails_WorldEdit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := New()

	// First search to find the real author/slug sourceID.
	results, errSearch := p.Search(context.Background(), "WorldEdit", nil)
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results; cannot proceed with GetModDetails")
	}

	var sourceID string
	for _, r := range results {
		if strings.EqualFold(r.Name, "WorldEdit") || strings.Contains(strings.ToLower(r.SourceID), "worldedit") {
			sourceID = r.SourceID
			break
		}
	}
	if sourceID == "" {
		t.Skip("WorldEdit not found in search results; skipping GetModDetails test")
	}

	details, errDetails := p.GetModDetails(context.Background(), sourceID, nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails(%q) error = %v", sourceID, errDetails)
	}
	if details == nil {
		t.Fatal("GetModDetails() = nil, want non-nil")
	}
	if !strings.EqualFold(details.Name, "WorldEdit") {
		t.Errorf("GetModDetails().Name = %q, want %q (case-insensitive)", details.Name, "WorldEdit")
	}
	if details.Source != providerID {
		t.Errorf("GetModDetails().Source = %q, want %q", details.Source, providerID)
	}
}
