package mojang

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newTestProvider(srv *httptest.Server) *Provider {
	p := New()
	p.manifestURL = srv.URL + "/mc/game/version_manifest.json"
	p.httpClient = srv.Client()
	return p
}

func TestGetModDetails_ReturnsReleaseVersionsNewestFirst(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mc/game/version_manifest.json":
			w.Header().Set("Content-Type", "application/json")
			payload := map[string]any{
				"latest": map[string]string{"release": "1.21.5"},
				"versions": []map[string]string{
					{"id": "1.21.5", "type": "release", "url": r.Host + `/versions/1.21.5.json`},
					{"id": "24w12a", "type": "snapshot", "url": r.Host + `/versions/24w12a.json`},
					{"id": "1.21.4", "type": "release", "url": r.Host + `/versions/1.21.4.json`},
				},
			}
			errEncode := json.NewEncoder(w).Encode(payload)
			if errEncode != nil {
				http.Error(w, errEncode.Error(), http.StatusInternalServerError)
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	p := newTestProvider(srv)
	details, errDetails := p.GetModDetails(context.Background(), "vanilla", nil)
	if errDetails != nil {
		t.Fatalf("GetModDetails() error = %v", errDetails)
	}
	if len(details.Versions) != 2 {
		t.Fatalf("GetModDetails() versions len = %d, want 2", len(details.Versions))
	}
	if details.Versions[0].VersionID != "1.21.5" {
		t.Errorf("GetModDetails() versions[0] = %q, want %q", details.Versions[0].VersionID, "1.21.5")
	}
	if details.Versions[1].VersionID != "1.21.4" {
		t.Errorf("GetModDetails() versions[1] = %q, want %q", details.Versions[1].VersionID, "1.21.4")
	}
}

func TestDownload_WritesMinecraftServerJar(t *testing.T) {
	baseURL := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mc/game/version_manifest.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"latest":{"release":"1.21.5"},
				"versions":[{"id":"1.21.5","type":"release","url":"` + baseURL + `/versions/1.21.5.json"}]
			}`))
		case "/versions/1.21.5.json":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"downloads":{"server":{"url":"` + baseURL + `/downloads/server.jar","size":4,"sha1":"unused"}}
			}`))
		case "/downloads/server.jar":
			_, _ = w.Write([]byte("jar!"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	baseURL = srv.URL

	p := newTestProvider(srv)
	targetDir := t.TempDir()
	files, errDownload := p.Download(context.Background(), "vanilla", "1.21.5", targetDir)
	if errDownload != nil {
		t.Fatalf("Download() error = %v", errDownload)
	}
	if len(files) != 1 || files[0].Path != serverJarName || !files[0].IsPrimary {
		t.Fatalf("Download() files = %+v, want primary minecraft_server.jar", files)
	}

	jarBytes, errRead := os.ReadFile(filepath.Join(targetDir, serverJarName))
	if errRead != nil {
		t.Fatalf("read downloaded jar: %v", errRead)
	}
	if string(jarBytes) != "jar!" {
		t.Errorf("downloaded jar = %q, want %q", string(jarBytes), "jar!")
	}
}
