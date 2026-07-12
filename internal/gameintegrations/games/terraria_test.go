package games

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestTerrariaInstallAndUpdate(t *testing.T) {
	tests := []struct {
		name            string
		operatingSystem string
		platform        string
		executable      string
	}{
		{
			name:            "windows package",
			operatingSystem: "windows",
			platform:        "Windows",
			executable:      "TerrariaServer.exe",
		},
		{
			name:            "linux package",
			operatingSystem: "linux",
			platform:        "Linux",
			executable:      "TerrariaServer.bin.x86_64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			archiveBytes := buildTerrariaTestArchive(t, tt.platform, tt.executable, "version-one")
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/names":
					_, errWrite := io.WriteString(writer, `["terraria-server-test.zip"]`)
					if errWrite != nil {
						t.Errorf("write metadata response: %v", errWrite)
					}
				case "/download/terraria-server-test.zip":
					_, errWrite := writer.Write(archiveBytes)
					if errWrite != nil {
						t.Errorf("write archive response: %v", errWrite)
					}
				default:
					http.NotFound(writer, request)
				}
			}))
			defer server.Close()

			directory := t.TempDir()
			game := &Terraria{
				serverNamesURL: server.URL + "/names",
				downloadURL:    server.URL + "/download",
				httpClient:     server.Client(),
				runtimeOS:      tt.operatingSystem,
			}
			gameServer := &models.GameServer{Directory: directory}
			var output bytes.Buffer
			errInstall := game.Install(gameServer, &output, io.Discard)
			if errInstall != nil {
				t.Fatalf("Install() error = %v", errInstall)
			}

			executablePath := filepath.Join(directory, tt.executable)
			executableData, errReadExecutable := os.ReadFile(executablePath)
			if errReadExecutable != nil {
				t.Fatalf("read installed executable: %v", errReadExecutable)
			}
			if string(executableData) != "version-one" {
				t.Fatalf("installed executable = %q, want version-one", executableData)
			}
			otherPlatformFile := filepath.Join(directory, "other-platform.txt")
			_, errOtherPlatform := os.Stat(otherPlatformFile)
			if !os.IsNotExist(errOtherPlatform) {
				t.Fatalf("other platform file Stat() error = %v, want os.ErrNotExist", errOtherPlatform)
			}

			configPath := filepath.Join(directory, "serverconfig.txt")
			errWriteConfig := os.WriteFile(configPath, []byte("user-config"), 0o600)
			if errWriteConfig != nil {
				t.Fatalf("write user config: %v", errWriteConfig)
			}
			archiveBytes = buildTerrariaTestArchive(t, tt.platform, tt.executable, "version-two")
			errUpdate := game.Update(gameServer, &output, io.Discard)
			if errUpdate != nil {
				t.Fatalf("Update() error = %v", errUpdate)
			}

			executableData, errReadExecutable = os.ReadFile(executablePath)
			if errReadExecutable != nil {
				t.Fatalf("read updated executable: %v", errReadExecutable)
			}
			if string(executableData) != "version-two" {
				t.Fatalf("updated executable = %q, want version-two", executableData)
			}
			configData, errReadConfig := os.ReadFile(configPath)
			if errReadConfig != nil {
				t.Fatalf("read preserved config: %v", errReadConfig)
			}
			if string(configData) != "user-config" {
				t.Fatalf("preserved config = %q, want user-config", configData)
			}
			if !strings.Contains(output.String(), "dedicated server files are ready") {
				t.Fatalf("output = %q, want completion message", output.String())
			}
		})
	}
}

func TestTerrariaArchiveRelativePath(t *testing.T) {
	tests := []struct {
		name         string
		archivePath  string
		platform     string
		wantPath     string
		wantSelected bool
		wantError    bool
	}{
		{
			name:         "selected file",
			archivePath:  "1456/Windows/TerrariaServer.exe",
			platform:     "Windows",
			wantPath:     "TerrariaServer.exe",
			wantSelected: true,
		},
		{
			name:         "selected nested file",
			archivePath:  "1456/Linux/lib64/libFAudio.so.0",
			platform:     "Linux",
			wantPath:     "lib64/libFAudio.so.0",
			wantSelected: true,
		},
		{
			name:        "different platform",
			archivePath: "1456/Mac/TerrariaServer",
			platform:    "Linux",
		},
		{
			name:        "path traversal",
			archivePath: "1456/Linux/../../escape",
			platform:    "Linux",
			wantError:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotSelected, errPath := terrariaArchiveRelativePath(tt.archivePath, tt.platform)
			if (errPath != nil) != tt.wantError {
				t.Fatalf("terrariaArchiveRelativePath() error = %v, wantError %t", errPath, tt.wantError)
			}
			if gotPath != tt.wantPath || gotSelected != tt.wantSelected {
				t.Fatalf(
					"terrariaArchiveRelativePath() = (%q, %t), want (%q, %t)",
					gotPath,
					gotSelected,
					tt.wantPath,
					tt.wantSelected,
				)
			}
		})
	}
}

func buildTerrariaTestArchive(t *testing.T, platform string, executable string, executableContents string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	files := map[string]string{
		"1456/" + platform + "/" + executable:    executableContents,
		"1456/" + platform + "/serverconfig.txt": "shipped-config",
		"1456/Other/other-platform.txt":          "not-selected",
	}
	for name, contents := range files {
		writer, errCreate := archive.Create(name)
		if errCreate != nil {
			t.Fatalf("create archive entry %q: %v", name, errCreate)
		}
		_, errWrite := io.WriteString(writer, contents)
		if errWrite != nil {
			t.Fatalf("write archive entry %q: %v", name, errWrite)
		}
	}
	errClose := archive.Close()
	if errClose != nil {
		t.Fatalf("close test archive: %v", errClose)
	}
	return buffer.Bytes()
}
