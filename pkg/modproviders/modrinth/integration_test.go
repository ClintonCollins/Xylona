//go:build integration

package modrinth

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
		if strings.EqualFold(r.Name, "WorldEdit") || strings.EqualFold(r.SourceID, "worldedit") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Search() did not return WorldEdit in results; got: %+v", results)
	}
}

func TestIntegration_GetModDetails_LuckPerms(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := New()
	details, errDetails := p.GetModDetails(context.Background(), "luckperms", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}
	if details == nil {
		t.Fatal("GetModDetails() = nil, want non-nil")
	}
	if !strings.EqualFold(details.Name, "LuckPerms") {
		t.Errorf("GetModDetails().Name = %q, want %q (case-insensitive)", details.Name, "LuckPerms")
	}
	if details.Source != providerID {
		t.Errorf("GetModDetails().Source = %q, want %q", details.Source, providerID)
	}
}
