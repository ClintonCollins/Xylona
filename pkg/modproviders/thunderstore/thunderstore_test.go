package thunderstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

func newTestProvider(srv *httptest.Server) *Provider {
	provider := New()
	provider.baseURL = srv.URL
	return provider
}

func writeTestJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	errEncode := json.NewEncoder(w).Encode(value)
	if errEncode != nil {
		t.Errorf("encode test response: %v", errEncode)
	}
}

func TestSearch(t *testing.T) {
	t.Run("maps current API results and parameters", func(t *testing.T) {
		var capturedQuery string
		var capturedOrdering string
		var capturedCategory string
		var capturedUserAgent string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/cyberstorm/listing/valheim/" {
				http.NotFound(w, r)
				return
			}
			capturedQuery = r.URL.Query().Get("q")
			capturedOrdering = r.URL.Query().Get("ordering")
			capturedCategory = r.URL.Query().Get("included_categories")
			capturedUserAgent = r.Header.Get("User-Agent")
			writeTestJSON(t, w, packageSearchResponse{
				Count: 1,
				Results: []packagePreview{{
					Categories:    []categoryInfo{{Name: "Libraries", Slug: "libraries"}},
					Description:   "BepInEx pack for Valheim",
					DownloadCount: 15_000_000,
					IconURL:       "https://example.com/icon.png",
					LastUpdated:   "2025-08-29T13:16:39Z",
					Name:          "BepInExPack_Valheim",
					Namespace:     "denikson",
				}},
			})
		}))
		defer srv.Close()

		provider := newTestProvider(srv)
		params := modproviders.SearchParams{
			"community":                  "valheim",
			modproviders.ParamSortBy:     "downloads",
			modproviders.ParamCategories: []string{"libraries"},
		}
		result, errSearch := provider.Search(context.Background(), "BepInEx", params)
		if errSearch != nil {
			t.Fatalf("Search() error = %v", errSearch)
		}
		if result.TotalHits != 1 {
			t.Errorf("Search().TotalHits = %d, want 1", result.TotalHits)
		}
		if len(result.Results) != 1 {
			t.Fatalf("Search() len = %d, want 1", len(result.Results))
		}
		first := result.Results[0]
		if first.SourceID != "denikson-BepInExPack_Valheim" {
			t.Errorf("result.SourceID = %q, want denikson-BepInExPack_Valheim", first.SourceID)
		}
		if first.Author != "denikson" || first.Downloads != 15_000_000 {
			t.Errorf("result = %+v, want mapped author and downloads", first)
		}
		if len(first.Categories) != 1 || first.Categories[0] != "Libraries" {
			t.Errorf("result.Categories = %v, want [Libraries]", first.Categories)
		}
		if capturedQuery != "BepInEx" {
			t.Errorf("q = %q, want BepInEx", capturedQuery)
		}
		if capturedOrdering != "most-downloaded" {
			t.Errorf("ordering = %q, want most-downloaded", capturedOrdering)
		}
		if capturedCategory != "libraries" {
			t.Errorf("included_categories = %q, want libraries", capturedCategory)
		}
		if capturedUserAgent != userAgent {
			t.Errorf("User-Agent = %q, want %q", capturedUserAgent, userAgent)
		}
	})

	t.Run("honors offsets across API pages", func(t *testing.T) {
		requestedPages := make([]string, 0, 2)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			page := r.URL.Query().Get("page")
			requestedPages = append(requestedPages, page)
			response := packageSearchResponse{Count: 45}
			switch page {
			case "2":
				next := "page-3"
				response.Next = &next
				for index := 20; index < 40; index++ {
					response.Results = append(response.Results, packagePreview{
						Name:      fmt.Sprintf("Mod%d", index),
						Namespace: "Author",
					})
				}
			case "3":
				for index := 40; index < 45; index++ {
					response.Results = append(response.Results, packagePreview{
						Name:      fmt.Sprintf("Mod%d", index),
						Namespace: "Author",
					})
				}
			default:
				http.Error(w, "unexpected page", http.StatusBadRequest)
				return
			}
			writeTestJSON(t, w, response)
		}))
		defer srv.Close()

		provider := newTestProvider(srv)
		params := modproviders.SearchParams{
			modproviders.ParamLimit:  15,
			modproviders.ParamOffset: 30,
		}
		result, errSearch := provider.Search(context.Background(), "", params)
		if errSearch != nil {
			t.Fatalf("Search() error = %v", errSearch)
		}
		if len(result.Results) != 15 {
			t.Fatalf("Search() len = %d, want 15", len(result.Results))
		}
		if result.Results[0].Name != "Mod30" || result.Results[14].Name != "Mod44" {
			t.Errorf("Search() bounds = %q..%q, want Mod30..Mod44", result.Results[0].Name, result.Results[14].Name)
		}
		if strings.Join(requestedPages, ",") != "2,3" {
			t.Errorf("requested pages = %v, want [2 3]", requestedPages)
		}
	})

	t.Run("returns HTTP errors", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "rate limited", http.StatusTooManyRequests)
		}))
		defer srv.Close()

		provider := newTestProvider(srv)
		_, errSearch := provider.Search(context.Background(), "test", nil)
		if errSearch == nil {
			t.Fatal("Search() error = nil, want HTTP error")
		}
	})
}

func TestGetModDetailsAndVersions(t *testing.T) {
	detailsJSON := `{
		"categories":[{"name":"Mods","slug":"mods"}],
		"community_identifier":"valheim",
		"dependencies":[{"name":"BepInExPack_Valheim","namespace":"denikson","version_number":"5.4.2202"}],
		"description":"A configurable Valheim mod",
		"download_count":8500000,
		"download_url":"https://example.com/0.9.12.0.zip",
		"icon_url":"https://example.com/icon.png",
		"latest_version_number":"0.9.12.0",
		"name":"ValheimPlus",
		"namespace":"ValheimPlus",
		"size":1048576,
		"team":{"name":"ValheimPlus"}
	}`
	versions := []packageVersionResponse{
		{VersionNumber: "0.9.11.0", DateTimeCreated: "2022-01-01T00:00:00Z", DownloadURL: "https://example.com/0.9.11.0.zip"},
		{VersionNumber: "0.9.12.0", DateTimeCreated: "2023-01-01T00:00:00Z", DownloadURL: "https://example.com/0.9.12.0.zip"},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/cyberstorm/listing/valheim/ValheimPlus/ValheimPlus/":
			w.Header().Set("Content-Type", "application/json")
			_, errWrite := io.WriteString(w, detailsJSON)
			if errWrite != nil {
				t.Errorf("write details response: %v", errWrite)
			}
		case "/api/cyberstorm/package/ValheimPlus/ValheimPlus/versions/":
			writeTestJSON(t, w, versions)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	provider := newTestProvider(srv)
	params := modproviders.SearchParams{"community": "valheim"}
	details, errDetails := provider.GetModDetails(context.Background(), "ValheimPlus-ValheimPlus", params)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}
	if details.SourceID != "ValheimPlus-ValheimPlus" || details.Author != "ValheimPlus" {
		t.Errorf("GetModDetails() = %+v, want mapped identity", details)
	}
	if details.Downloads != 8_500_000 || len(details.Versions) != 2 {
		t.Errorf("GetModDetails() downloads/versions = %d/%d, want 8500000/2", details.Downloads, len(details.Versions))
	}
	if details.Versions[0].VersionID != "0.9.12.0" {
		t.Errorf("GetModDetails().Versions[0] = %q, want latest 0.9.12.0", details.Versions[0].VersionID)
	}
	if details.Versions[0].FileSize != 1_048_576 {
		t.Errorf("latest FileSize = %d, want 1048576", details.Versions[0].FileSize)
	}
	if len(details.Versions[0].Dependencies) != 1 {
		t.Fatalf("latest Dependencies len = %d, want 1", len(details.Versions[0].Dependencies))
	}
	dependency := details.Versions[0].Dependencies[0]
	if dependency.SourceID != "denikson-BepInExPack_Valheim-5.4.2202" || !dependency.Required {
		t.Errorf("latest dependency = %+v, want required BepInEx version", dependency)
	}

	gotVersions, errVersions := provider.GetVersions(context.Background(), "ValheimPlus-ValheimPlus", "", params)
	if errVersions != nil {
		t.Fatalf("GetVersions() error = %v", errVersions)
	}
	if len(gotVersions) != 2 || gotVersions[0].VersionID != "0.9.12.0" {
		t.Errorf("GetVersions() = %+v, want two newest-first versions", gotVersions)
	}
}

func TestCheckForUpdate(t *testing.T) {
	tests := []struct {
		name        string
		versions    []packageVersionResponse
		wantVersion string
		wantErr     error
	}{
		{
			name: "returns newest by creation time",
			versions: []packageVersionResponse{
				{VersionNumber: "1.0.0", DateTimeCreated: "2023-01-01T00:00:00Z"},
				{VersionNumber: "2.0.0", DateTimeCreated: "2024-01-01T00:00:00Z"},
			},
			wantVersion: "2.0.0",
		},
		{name: "reports no available versions", versions: []packageVersionResponse{}, wantErr: modproviders.ErrNoUpdateAvailable},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeTestJSON(t, w, test.versions)
			}))
			defer srv.Close()

			provider := newTestProvider(srv)
			version, errCheck := provider.CheckForUpdate(context.Background(), "Author-Package", "")
			if test.wantErr != nil {
				if !errors.Is(errCheck, test.wantErr) {
					t.Fatalf("CheckForUpdate() error = %v, want %v", errCheck, test.wantErr)
				}
				return
			}
			if errCheck != nil {
				t.Fatalf("CheckForUpdate() error = %v", errCheck)
			}
			if version == nil || version.VersionID != test.wantVersion {
				t.Errorf("CheckForUpdate() = %+v, want %q", version, test.wantVersion)
			}
		})
	}
}

func TestDownload(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/octet-stream")
		_, errWrite := w.Write([]byte("payload"))
		if errWrite != nil {
			t.Errorf("write download response: %v", errWrite)
		}
	}))
	defer srv.Close()

	provider := newTestProvider(srv)
	targetDir := t.TempDir()
	files, errDownload := provider.Download(context.Background(), "Author-SharedMod", "1.0.0", targetDir)
	if errDownload != nil {
		t.Fatalf("Download() error = %v", errDownload)
	}
	if capturedPath != "/package/download/Author/SharedMod/1.0.0/" {
		t.Errorf("Download() path = %q, want package download path", capturedPath)
	}
	if len(files) != 1 || files[0].Path != "Author-SharedMod-1.0.0.zip" || files[0].Size != 7 {
		t.Fatalf("Download() files = %+v, want one seven-byte archive", files)
	}
	data, errRead := os.ReadFile(filepath.Join(targetDir, files[0].Path))
	if errRead != nil {
		t.Fatalf("read downloaded file: %v", errRead)
	}
	if string(data) != "payload" {
		t.Errorf("downloaded content = %q, want payload", data)
	}

	tests := []struct {
		name      string
		sourceID  string
		versionID string
	}{
		{name: "unsafe source ID", sourceID: "Author-../outside", versionID: "1.0.0"},
		{name: "unsafe version ID", sourceID: "Author-SharedMod", versionID: "../outside"},
		{name: "empty version ID", sourceID: "Author-SharedMod", versionID: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, errUnsafe := provider.Download(context.Background(), test.sourceID, test.versionID, targetDir)
			if errUnsafe == nil {
				t.Fatalf("Download(%q, %q) error = nil, want invalid identifier error", test.sourceID, test.versionID)
			}
		})
	}
}

func TestSearchRejectsOversizedResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, errWrite := io.WriteString(w, strings.Repeat(" ", maxAPIResponseBytes+1))
		if errWrite != nil {
			t.Errorf("write oversized response: %v", errWrite)
		}
	}))
	defer srv.Close()

	provider := newTestProvider(srv)
	_, errSearch := provider.Search(context.Background(), "", nil)
	if errSearch == nil || !strings.Contains(errSearch.Error(), "response exceeded") {
		t.Fatalf("Search() error = %v, want response exceeded error", errSearch)
	}
}

func TestHelpers(t *testing.T) {
	t.Run("extract community", func(t *testing.T) {
		tests := []struct {
			name   string
			params modproviders.SearchParams
			want   string
		}{
			{name: "nil params", params: nil, want: ""},
			{name: "no key", params: modproviders.SearchParams{"other": "value"}, want: ""},
			{name: "string", params: modproviders.SearchParams{"community": " valheim "}, want: "valheim"},
			{name: "wrong type", params: modproviders.SearchParams{"community": 42}, want: ""},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := extractCommunity(test.params)
				if got != test.want {
					t.Errorf("extractCommunity() = %q, want %q", got, test.want)
				}
			})
		}
	})

	t.Run("parse source ID", func(t *testing.T) {
		tests := []struct {
			name      string
			value     string
			wantName  string
			wantError bool
		}{
			{name: "valid", value: "Author-Package_Name", wantName: "Package_Name"},
			{name: "trims components", value: " Author - Package_Name ", wantName: "Package_Name"},
			{name: "missing separator", value: "Package", wantError: true},
			{name: "missing name", value: "Author-", wantError: true},
			{name: "parent traversal", value: "Author-../Package", wantError: true},
			{name: "backslash", value: `Author-Package\Name`, wantError: true},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				reference, errParse := parseSourceID(test.value)
				if (errParse != nil) != test.wantError {
					t.Fatalf("parseSourceID(%q) error = %v, wantError %v", test.value, errParse, test.wantError)
				}
				if !test.wantError && reference.name != test.wantName {
					t.Errorf("parseSourceID(%q).name = %q, want %q", test.value, reference.name, test.wantName)
				}
			})
		}
	})
}
