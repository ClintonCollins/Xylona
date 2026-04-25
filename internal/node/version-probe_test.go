package node

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeMinecraftJar(t *testing.T, path string, versionJSON string) {
	t.Helper()

	file, errCreate := os.Create(path)
	if errCreate != nil {
		t.Fatalf("Create jar: %v", errCreate)
	}

	zipWriter := zip.NewWriter(file)
	versionFile, errEntry := zipWriter.Create("version.json")
	if errEntry != nil {
		errClose := file.Close()
		if errClose != nil {
			t.Fatalf("Close jar after Create entry error: %v", errClose)
		}
		t.Fatalf("Create version.json: %v", errEntry)
	}
	_, errWrite := versionFile.Write([]byte(versionJSON))
	if errWrite != nil {
		errClose := file.Close()
		if errClose != nil {
			t.Fatalf("Close jar after Write error: %v", errClose)
		}
		t.Fatalf("Write version.json: %v", errWrite)
	}

	errCloseZip := zipWriter.Close()
	errCloseFile := file.Close()
	if errCloseZip != nil {
		t.Fatalf("Close zip writer: %v", errCloseZip)
	}
	if errCloseFile != nil {
		t.Fatalf("Close jar file: %v", errCloseFile)
	}
}

func writeZipWithoutVersionJSON(t *testing.T, path string) {
	t.Helper()

	file, errCreate := os.Create(path)
	if errCreate != nil {
		t.Fatalf("Create zip: %v", errCreate)
	}

	zipWriter := zip.NewWriter(file)
	entry, errEntry := zipWriter.Create("assets/example.txt")
	if errEntry != nil {
		errClose := file.Close()
		if errClose != nil {
			t.Fatalf("Close zip after Create entry error: %v", errClose)
		}
		t.Fatalf("Create zip entry: %v", errEntry)
	}
	_, errWrite := entry.Write([]byte("not a server jar"))
	if errWrite != nil {
		errClose := file.Close()
		if errClose != nil {
			t.Fatalf("Close zip after Write error: %v", errClose)
		}
		t.Fatalf("Write zip entry: %v", errWrite)
	}

	errCloseZip := zipWriter.Close()
	errCloseFile := file.Close()
	if errCloseZip != nil {
		t.Fatalf("Close zip writer: %v", errCloseZip)
	}
	if errCloseFile != nil {
		t.Fatalf("Close zip file: %v", errCloseFile)
	}
}

func TestProbeInstalledVersionReadsMinecraftJar(t *testing.T) {
	dir := t.TempDir()
	writeMinecraftJar(t, filepath.Join(dir, "server.jar"), `{"id":"1.21.4","name":"1.21.4"}`)

	n := &Node{}
	result, errProbe := n.ProbeInstalledVersion(InstalledVersionProbeRequest{
		Directory:     dir,
		Kind:          InstalledVersionProbeKindMinecraftJar,
		RelativePaths: []string{"missing.jar", "server.jar"},
	})
	if errProbe != nil {
		t.Fatalf("ProbeInstalledVersion error = %v", errProbe)
	}
	if !result.Found {
		t.Fatalf("Found = false, want true")
	}
	if result.Version != "1.21.4" {
		t.Fatalf("Version = %q, want %q", result.Version, "1.21.4")
	}
	if result.SourcePath != "server.jar" {
		t.Fatalf("SourcePath = %q, want %q", result.SourcePath, "server.jar")
	}
}

func TestProbeInstalledVersionSkipsMinecraftJarsWithoutVersionJSON(t *testing.T) {
	dir := t.TempDir()
	writeZipWithoutVersionJSON(t, filepath.Join(dir, "client.jar"))
	writeMinecraftJar(t, filepath.Join(dir, "server.jar"), `{"id":"1.21.4","name":"1.21.4"}`)

	n := &Node{}
	result, errProbe := n.ProbeInstalledVersion(InstalledVersionProbeRequest{
		Directory:     dir,
		Kind:          InstalledVersionProbeKindMinecraftJar,
		RelativePaths: []string{"client.jar", "server.jar"},
	})
	if errProbe != nil {
		t.Fatalf("ProbeInstalledVersion error = %v", errProbe)
	}
	if !result.Found {
		t.Fatalf("Found = false, want true")
	}
	if result.Version != "1.21.4" {
		t.Fatalf("Version = %q, want %q", result.Version, "1.21.4")
	}
	if result.SourcePath != "server.jar" {
		t.Fatalf("SourcePath = %q, want %q", result.SourcePath, "server.jar")
	}
}

func TestProbeInstalledVersionReadsSteamManifest(t *testing.T) {
	dir := t.TempDir()
	errMkdir := os.Mkdir(filepath.Join(dir, "steamapps"), 0o750)
	if errMkdir != nil {
		t.Fatalf("Mkdir steamapps: %v", errMkdir)
	}
	manifestPath := filepath.Join(dir, "steamapps", "appmanifest_294420.acf")
	errWrite := os.WriteFile(manifestPath, []byte(`"AppState" { "buildid" "123456" }`), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile manifest: %v", errWrite)
	}

	n := &Node{}
	result, errProbe := n.ProbeInstalledVersion(InstalledVersionProbeRequest{
		Directory:           dir,
		Kind:                InstalledVersionProbeKindSteamManifest,
		PreferredSteamAppID: "294420",
	})
	if errProbe != nil {
		t.Fatalf("ProbeInstalledVersion error = %v", errProbe)
	}
	if !result.Found {
		t.Fatalf("Found = false, want true")
	}
	if result.Version != "123456" {
		t.Fatalf("Version = %q, want %q", result.Version, "123456")
	}
	if result.SourcePath != "steamapps/appmanifest_294420.acf" {
		t.Fatalf("SourcePath = %q, want %q", result.SourcePath, "steamapps/appmanifest_294420.acf")
	}
}
