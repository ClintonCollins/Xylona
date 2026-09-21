package thunderstore

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
)

func TestDownloadValheim(t *testing.T) {
	tests := []struct {
		name         string
		paths        []string
		mode         fs.FileMode
		want         []string
		corruptField int
	}{
		{name: "layouts", paths: []string{"manifest.json", "README.md", "icon.png", "Example.dll", "assets/data", "plugins/Other.dll", "BepInEx/plugins/Nested/Third.dll", "patchers/Patch.dll", "BepInEx/patchers/Fourth.dll", "config/example.cfg", "BepInEx/config/other.cfg", "LICENSE"}, want: []string{"BepInEx/plugins/Team-Mod/Example.dll", "BepInEx/plugins/Team-Mod/assets/data", "BepInEx/plugins/Team-Mod/Other.dll", "BepInEx/plugins/Team-Mod/Nested/Third.dll", "BepInEx/patchers/Team-Mod/Patch.dll", "BepInEx/patchers/Team-Mod/Fourth.dll", "BepInEx/config/example.cfg", "BepInEx/config/other.cfg", "BepInEx/plugins/Team-Mod/LICENSE"}},
		{name: "checksum mismatch", paths: []string{"Example.dll"}, corruptField: 16},
		{name: "size mismatch", paths: []string{"Example.dll"}, corruptField: 24},
		{name: "traversal", paths: []string{"../escape.dll"}},
		{name: "absolute", paths: []string{"/escape.dll"}},
		{name: "backslash", paths: []string{`plugins\escape.dll`}},
		{name: "device", paths: []string{"plugins/CON.dll"}},
		{name: "alternate stream", paths: []string{"plugins/a.dll:stream"}},
		{name: "case collision", paths: []string{"Example.dll", "example.dll"}},
		{name: "directory case collision", paths: []string{"plugins/Example/a.dll", "plugins/example/b.dll"}},
		{name: "mapping collision", paths: []string{"Example.dll", "plugins/Example.dll"}},
		{name: "file directory collision", paths: []string{"assets", "assets/data"}},
		{name: "mapped directory collision", paths: []string{"plugins/assets", "assets/data"}},
		{name: "symlink", paths: []string{"link.dll"}, mode: fs.ModeSymlink | 0o777},
		{name: "loader overwrite", paths: []string{"BepInEx/core/BepInEx.dll"}},
		{name: "script", paths: []string{"install.sh"}},
		{name: "metadata only", paths: []string{"manifest.json", "README.md"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			for _, name := range tc.paths {
				header := &zip.FileHeader{Name: name, Method: zip.Deflate}
				if tc.mode != 0 {
					header.SetMode(tc.mode)
				}
				entry, errCreate := writer.CreateHeader(header)
				if errCreate != nil {
					t.Fatal(errCreate)
				}
				_, errWrite := entry.Write([]byte("content"))
				if errWrite != nil {
					t.Fatal(errWrite)
				}
			}
			errClose := writer.Close()
			if errClose != nil {
				t.Fatal(errClose)
			}
			if tc.corruptField != 0 {
				central := bytes.Index(buffer.Bytes(), []byte("PK\x01\x02"))
				buffer.Bytes()[central+tc.corruptField] ^= 1
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, errWrite := w.Write(buffer.Bytes())
				if errWrite != nil {
					t.Error(errWrite)
				}
			}))
			defer server.Close()
			provider := &Provider{httpClient: server.Client(), baseURL: server.URL}
			directory := t.TempDir()
			files, errDownload := provider.DownloadValheim(t.Context(), "Team-Mod", "1.0.0", directory, "linux")
			if tc.want == nil {
				if errDownload == nil {
					t.Fatal("accepted unsafe or empty archive")
				}
				if tc.corruptField == 0 {
					entries, errRead := os.ReadDir(directory)
					if errRead != nil || len(entries) != 0 {
						t.Fatalf("failed validation wrote files: %v, %v", entries, errRead)
					}
				}
				return
			}
			if errDownload != nil {
				t.Fatal(errDownload)
			}
			var paths []string
			for _, file := range files {
				paths = append(paths, file.Path)
				data, errRead := os.ReadFile(filepath.Join(directory, file.Path))
				if errRead != nil {
					t.Fatal(errRead)
				}
				if string(data) != "content" || file.Size != int64(len(data)) || file.Hash != fmt.Sprintf("%x", sha256.Sum256(data)) {
					t.Fatalf("incorrect file: %+v", file)
				}
			}
			slices.Sort(paths)
			slices.Sort(tc.want)
			if !slices.Equal(paths, tc.want) {
				t.Fatalf("paths = %v, want %v", paths, tc.want)
			}
		})
	}
}

func TestInspectValheimArchiveLimits(t *testing.T) {
	for _, count := range []int{1, maxValheimFiles + 1} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			for index := range count {
				header := &zip.FileHeader{Name: fmt.Sprintf("file%d.dll", index)}
				if count == 1 {
					header.UncompressedSize64 = maxValheimFileSize + 1
				}
				_, errCreate := writer.CreateRaw(header)
				if errCreate != nil {
					t.Fatal(errCreate)
				}
			}
			errClose := writer.Close()
			if errClose != nil {
				t.Fatal(errClose)
			}
			artifact := modproviders.DownloadedFile{Hash: fmt.Sprintf("%x", sha256.Sum256(buffer.Bytes())), Size: int64(buffer.Len())}
			_, errInspect := inspectValheimArchive(bytes.NewReader(buffer.Bytes()), artifact, "Team-Mod", "linux")
			if errInspect == nil {
				t.Fatal("accepted archive exceeding limits")
			}
		})
	}
}

func TestInspectValheimArchiveIntegrity(t *testing.T) {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	_, errCreate := writer.Create("example.dll")
	if errCreate != nil {
		t.Fatal(errCreate)
	}
	errClose := writer.Close()
	if errClose != nil {
		t.Fatal(errClose)
	}
	for _, test := range []struct {
		name, packageID, hash string
	}{
		{name: "changed bytes", packageID: "Team-Mod", hash: strings.Repeat("0", 64)},
		{name: "unrecognized loader", packageID: ValheimLoaderID, hash: fmt.Sprintf("%x", sha256.Sum256(buffer.Bytes()))},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifact := modproviders.DownloadedFile{Hash: test.hash, Size: int64(buffer.Len())}
			_, errInspect := inspectValheimArchive(bytes.NewReader(buffer.Bytes()), artifact, test.packageID, "linux")
			if errInspect == nil {
				t.Fatal("accepted unverified archive")
			}
		})
	}
}

func TestDownloadValheimLoader(t *testing.T) {
	archivePath := "../../../docs/game-integrations/valheim/contracts-artifacts/BepInExPack_Valheim-5.4.2333.zip"
	data, errRead := os.ReadFile(archivePath)
	if os.IsNotExist(errRead) {
		t.Skip("recorded loader archive unavailable")
	}
	if errRead != nil {
		t.Fatal(errRead)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write(data)
		if errWrite != nil {
			t.Error(errWrite)
		}
	}))
	defer server.Close()
	provider := &Provider{httpClient: server.Client(), baseURL: server.URL}
	for _, targetOS := range []string{"windows", "linux"} {
		t.Run(targetOS, func(t *testing.T) {
			files, errDownload := provider.DownloadValheim(t.Context(), ValheimLoaderID, ValheimLoaderVersion, t.TempDir(), targetOS)
			if errDownload != nil {
				t.Fatal(errDownload)
			}
			foundNative := false
			for _, file := range files {
				if strings.HasSuffix(file.Path, ".sh") || strings.HasPrefix(file.Path, "BepInExPack_Valheim/") {
					t.Fatalf("unexpected path: %s", file.Path)
				}
				if (targetOS == "windows" && file.Path == "winhttp.dll") || (targetOS == "linux" && file.Path == "doorstop_libs/libdoorstop_x64.so") {
					foundNative = true
				}
			}
			if !foundNative {
				t.Fatal("missing native loader")
			}
		})
	}
	_, errVersion := provider.DownloadValheim(t.Context(), ValheimLoaderID, "5.4.0", t.TempDir(), "linux")
	if errVersion == nil {
		t.Fatal("accepted unsupported loader")
	}
}

func TestDownloadValheimDependencyOnly(t *testing.T) {
	for _, tc := range []struct {
		name         string
		archivePath  string
		dependencies string
		wantSuccess  bool
		wantLookup   bool
	}{
		{name: "modpack", archivePath: "manifest.json", dependencies: `["Team-Mod-1.0.0"]`, wantSuccess: true, wantLookup: true},
		{name: "empty package", archivePath: "manifest.json", dependencies: `[]`, wantLookup: true},
		{name: "unsafe modpack", archivePath: "../escape.dll", dependencies: `["Team-Mod-1.0.0"]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			_, errCreate := writer.Create(tc.archivePath)
			if errCreate != nil {
				t.Fatal(errCreate)
			}
			errClose := writer.Close()
			if errClose != nil {
				t.Fatal(errClose)
			}
			lookups := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				if strings.HasPrefix(request.URL.Path, "/api/experimental/") {
					lookups++
					_, errWrite := fmt.Fprintf(w, `{"namespace":"Team","name":"Pack","version_number":"1.0.0","download_url":"https://example.com/package.zip","dependencies":%s}`, tc.dependencies)
					if errWrite != nil {
						t.Error(errWrite)
					}
					return
				}
				_, errWrite := w.Write(buffer.Bytes())
				if errWrite != nil {
					t.Error(errWrite)
				}
			}))
			defer server.Close()
			provider := &Provider{httpClient: server.Client(), baseURL: server.URL}
			files, errDownload := provider.DownloadValheim(t.Context(), "Team-Pack", "1.0.0", t.TempDir(), "linux")
			if (errDownload == nil) != tc.wantSuccess || len(files) != 0 {
				t.Fatalf("files = %v, error = %v", files, errDownload)
			}
			if (lookups > 0) != tc.wantLookup {
				t.Fatalf("metadata lookups = %d, want lookup %v", lookups, tc.wantLookup)
			}
		})
	}
}
