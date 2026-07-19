package palworldmap

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

var testWebP = []byte("RIFF\x08\x00\x00\x00WEBPdata")

func TestStoreInstall(t *testing.T) {
	t.Run("downloads transposed source paths and resumes from valid files", func(t *testing.T) {
		var requestMu sync.Mutex
		requestedPaths := make(map[string]int)
		server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
			requestMu.Lock()
			requestedPaths[request.URL.Path]++
			requestMu.Unlock()
			responseWriter.Header().Set("Content-Type", "image/webp")
			_, errWrite := responseWriter.Write(testWebP)
			if errWrite != nil {
				t.Errorf("write test tile: %v", errWrite)
			}
		}))
		defer server.Close()

		root := t.TempDir()
		layer := Layer{ID: "test", Label: "Test", BaseURL: server.URL + "/source", MinZoom: 0, MaxZoom: 1}
		store, errStore := newStoreWithConfig(root, server.Client(), []Layer{layer})
		if errStore != nil {
			t.Fatalf("newStore() error = %v", errStore)
		}

		errInstall := store.Install(context.Background())
		if errInstall != nil {
			t.Fatalf("Install() error = %v", errInstall)
		}
		for _, localPath := range []string{
			filepath.Join(root, "test", "0", "0", "0.webp"),
			filepath.Join(root, "test", "1", "0", "0.webp"),
			filepath.Join(root, "test", "1", "0", "1.webp"),
			filepath.Join(root, "test", "1", "1", "0.webp"),
			filepath.Join(root, "test", "1", "1", "1.webp"),
		} {
			contents, errRead := os.ReadFile(localPath)
			if errRead != nil {
				t.Fatalf("read installed tile %s: %v", localPath, errRead)
			}
			if string(contents) != string(testWebP) {
				t.Errorf("installed tile %s = %q, want WebP test data", localPath, contents)
			}
		}

		requestMu.Lock()
		if requestedPaths["/source/1/0/1.webp"] != 1 {
			t.Errorf("transposed source request count = %d, want 1", requestedPaths["/source/1/0/1.webp"])
		}
		initialRequestCount := len(requestedPaths)
		requestMu.Unlock()
		if initialRequestCount != 5 {
			t.Fatalf("source request count = %d, want 5", initialRequestCount)
		}

		errReinstall := store.Install(context.Background())
		if errReinstall != nil {
			t.Fatalf("Install() second run error = %v", errReinstall)
		}
		requestMu.Lock()
		defer requestMu.Unlock()
		for path, count := range requestedPaths {
			if count != 1 {
				t.Errorf("source request %s count after resume = %d, want 1", path, count)
			}
		}
	})

	t.Run("rejects a non-WebP response without installing it", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(responseWriter http.ResponseWriter, _ *http.Request) {
			_, errWrite := responseWriter.Write([]byte("not an image"))
			if errWrite != nil {
				t.Errorf("write invalid test tile: %v", errWrite)
			}
		}))
		defer server.Close()

		root := t.TempDir()
		layer := Layer{ID: "test", Label: "Test", BaseURL: server.URL, MinZoom: 0, MaxZoom: 0}
		store, errStore := newStoreWithConfig(root, server.Client(), []Layer{layer})
		if errStore != nil {
			t.Fatalf("newStore() error = %v", errStore)
		}
		errInstall := store.Install(context.Background())
		if errInstall == nil {
			t.Fatal("Install() error = nil, want invalid WebP error")
		}
		_, errStat := os.Stat(filepath.Join(root, "test", "0", "0", "0.webp"))
		if !os.IsNotExist(errStat) {
			t.Fatalf("installed invalid tile stat error = %v, want not exist", errStat)
		}
	})
}

func TestStoreHandler(t *testing.T) {
	root := t.TempDir()
	tileDirectory := filepath.Join(root, "test", "1", "1")
	errMkdir := os.MkdirAll(tileDirectory, 0o750)
	if errMkdir != nil {
		t.Fatalf("create tile directory: %v", errMkdir)
	}
	errWrite := os.WriteFile(filepath.Join(tileDirectory, "0.webp"), testWebP, 0o600)
	if errWrite != nil {
		t.Fatalf("write tile: %v", errWrite)
	}
	layer := Layer{ID: "test", Label: "Test", MinZoom: 0, MaxZoom: 1}
	store, errStore := newStoreWithConfig(root, http.DefaultClient, []Layer{layer})
	if errStore != nil {
		t.Fatalf("newStore() error = %v", errStore)
	}

	tests := []struct {
		name        string
		method      string
		path        string
		wantStatus  int
		wantContent bool
	}{
		{name: "serves installed tile", method: http.MethodGet, path: "/test/1/1/0.webp", wantStatus: http.StatusOK, wantContent: true},
		{name: "serves installed tile from mounted path", method: http.MethodGet, path: TilePathPrefix + "/test/1/1/0.webp", wantStatus: http.StatusOK, wantContent: true},
		{name: "supports head", method: http.MethodHead, path: "/test/1/1/0.webp", wantStatus: http.StatusOK},
		{name: "rejects out of range coordinate", method: http.MethodGet, path: "/test/1/2/0.webp", wantStatus: http.StatusNotFound},
		{name: "rejects unknown layer", method: http.MethodGet, path: "/unknown/0/0/0.webp", wantStatus: http.StatusNotFound},
		{name: "rejects temporary file", method: http.MethodGet, path: "/test/1/1/.tile-1.part", wantStatus: http.StatusNotFound},
		{name: "rejects writes", method: http.MethodPost, path: "/test/1/1/0.webp", wantStatus: http.StatusMethodNotAllowed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequestWithContext(t.Context(), test.method, test.path, nil)
			response := httptest.NewRecorder()
			store.Handler().ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantContent {
				body, errRead := io.ReadAll(response.Result().Body)
				if errRead != nil {
					t.Fatalf("read response: %v", errRead)
				}
				errClose := response.Result().Body.Close()
				if errClose != nil {
					t.Fatalf("close response: %v", errClose)
				}
				if string(body) != string(testWebP) {
					t.Errorf("body = %q, want tile contents", body)
				}
				if response.Header().Get("Content-Type") != "image/webp" {
					t.Errorf("Content-Type = %q, want image/webp", response.Header().Get("Content-Type"))
				}
			}
		})
	}
}
