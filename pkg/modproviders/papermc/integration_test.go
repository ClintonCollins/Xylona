//go:build integration

package papermc

import (
	"context"
	"testing"
)

func TestIntegration_GetVersions_PaperContains1214(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := New()
	endpoint := p.baseURL + "/projects/paper"

	var projectResp projectVersionsResponse
	errFetch := p.getJSON(context.Background(), endpoint, &projectResp)
	if errFetch != nil {
		t.Fatalf("get paper versions: %v", errFetch)
	}

	found := false
	for _, v := range projectResp.Versions {
		if v == "1.21.4" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("paper versions list did not contain 1.21.4; got: %v", projectResp.Versions)
	}
}

func TestIntegration_GetBuilds_Paper1214HasAtLeastOneBuild(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	p := New()
	builds, errBuilds := p.GetVersions(context.Background(), "paper", "1.21.4", nil)
	if errBuilds != nil {
		t.Fatalf("GetVersions(paper, 1.21.4) error = %v", errBuilds)
	}
	if len(builds) == 0 {
		t.Fatal("GetVersions(paper, 1.21.4) returned no builds, want at least one")
	}

	// Spot-check the first build has expected fields.
	first := builds[0]
	if first.VersionID == "" {
		t.Error("builds[0].VersionID is empty")
	}
	if first.VersionString == "" {
		t.Error("builds[0].VersionString is empty")
	}
	if len(first.GameVersions) == 0 || first.GameVersions[0] != "1.21.4" {
		t.Errorf("builds[0].GameVersions = %v, want [1.21.4]", first.GameVersions)
	}
}
