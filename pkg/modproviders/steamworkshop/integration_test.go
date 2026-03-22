//go:build integration

package steamworkshop

import (
	"context"
	"os"
	"strings"
	"testing"
)

// knownWorkshopID is a well-known, stable Steam Workshop item ID used for integration tests.
// 2128699613 = Project Zomboid "Expanded Helicopter Events"
const knownWorkshopID = "2128699613"

func TestIntegration_GetPublishedFileDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	apiKey := os.Getenv("STEAM_API_KEY")

	p := New()
	if apiKey != "" {
		p.SetAPIKey(apiKey)
	}

	detail, errDetail := p.getPublishedFileDetails(context.Background(), knownWorkshopID)
	if errDetail != nil {
		t.Fatalf("getPublishedFileDetails() error = %v", errDetail)
	}
	if detail == nil {
		t.Fatal("getPublishedFileDetails() = nil, want non-nil")
	}
	if detail.PublishedFileID != knownWorkshopID {
		t.Errorf("PublishedFileID = %q, want %q", detail.PublishedFileID, knownWorkshopID)
	}
	if detail.Title == "" {
		t.Error("Title is empty, want non-empty")
	}
	if detail.Creator == "" {
		t.Error("Creator is empty, want non-empty")
	}
}

func TestIntegration_GetModDetails(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	p := New()
	details, errDetails := p.GetModDetails(context.Background(), knownWorkshopID, nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}
	if details == nil {
		t.Fatal("GetModDetails() = nil, want non-nil")
	}
	if details.Source != providerID {
		t.Errorf("details.Source = %q, want %q", details.Source, providerID)
	}
	if details.SourceID != knownWorkshopID {
		t.Errorf("details.SourceID = %q, want %q", details.SourceID, knownWorkshopID)
	}
	if details.Name == "" {
		t.Error("details.Name is empty, want non-empty")
	}
	if len(details.Versions) == 0 {
		t.Error("details.Versions is empty, want at least one version")
	}
}

func TestIntegration_Search_RequiresAPIKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	apiKey := os.Getenv("STEAM_API_KEY")
	if apiKey == "" {
		t.Skip("skipping search integration test: STEAM_API_KEY not set")
	}

	p := New()
	p.SetAPIKey(apiKey)

	results, errSearch := p.Search(context.Background(), "helicopter", map[string]any{
		"app_id": "108600",
	})
	if errSearch != nil {
		t.Fatalf("Search() error = %v", errSearch)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results, want at least one")
	}

	found := false
	for _, r := range results {
		if strings.Contains(strings.ToLower(r.Name), "helicopter") ||
			strings.Contains(strings.ToLower(r.Description), "helicopter") {
			found = true
			break
		}
	}
	if !found {
		t.Logf("Search() returned %d results but none mention 'helicopter'; first result: %+v", len(results), results[0])
	}
}
