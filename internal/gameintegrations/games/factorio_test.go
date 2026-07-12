package games

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ulikunitz/xz"
)

func TestExtractFactorioArchive(t *testing.T) {
	t.Run("extracts files beneath the Factorio top-level directory", func(t *testing.T) {
		archivePath := writeFactorioTestArchive(t, []factorioTestArchiveEntry{
			{name: "factorio/bin/", mode: 0o755, typeFlag: tar.TypeDir},
			{name: "factorio/bin/x64/", mode: 0o755, typeFlag: tar.TypeDir},
			{name: "factorio/bin/x64/factorio", body: "server-binary", mode: 0o755, typeFlag: tar.TypeReg},
			{name: "factorio/data/server-settings.example.json", body: "{}", mode: 0o644, typeFlag: tar.TypeReg},
		})
		destination := t.TempDir()

		errExtract := extractFactorioArchive(archivePath, destination)
		if errExtract != nil {
			t.Fatalf("extractFactorioArchive() error = %v", errExtract)
		}
		binary, errRead := os.ReadFile(filepath.Join(destination, "bin", "x64", "factorio"))
		if errRead != nil {
			t.Fatalf("ReadFile(extracted binary) error = %v", errRead)
		}
		if string(binary) != "server-binary" {
			t.Errorf("extracted binary = %q, want %q", string(binary), "server-binary")
		}
	})

	testCases := []struct {
		name      string
		entryName string
		typeFlag  byte
		wantError string
	}{
		{name: "parent traversal", entryName: "factorio/../../outside", typeFlag: tar.TypeReg, wantError: "unsafe path"},
		{name: "absolute path", entryName: "/factorio/outside", typeFlag: tar.TypeReg, wantError: "unsafe path"},
		{name: "symbolic link", entryName: "factorio/bin/link", typeFlag: tar.TypeSymlink, wantError: "unsupported type"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			archivePath := writeFactorioTestArchive(t, []factorioTestArchiveEntry{{
				name:     testCase.entryName,
				body:     "unsafe",
				mode:     0o644,
				typeFlag: testCase.typeFlag,
			}})
			errExtract := extractFactorioArchive(archivePath, t.TempDir())
			if errExtract == nil {
				t.Fatal("extractFactorioArchive() error = nil, want error")
			}
			if !strings.Contains(errExtract.Error(), testCase.wantError) {
				t.Errorf("extractFactorioArchive() error = %q, want substring %q", errExtract, testCase.wantError)
			}
		})
	}
}

type factorioTestArchiveEntry struct {
	name     string
	body     string
	mode     int64
	typeFlag byte
}

func writeFactorioTestArchive(t *testing.T, entries []factorioTestArchiveEntry) string {
	t.Helper()

	var archive bytes.Buffer
	xzWriter, errXZ := xz.NewWriter(&archive)
	if errXZ != nil {
		t.Fatalf("xz.NewWriter() error = %v", errXZ)
	}
	tarWriter := tar.NewWriter(xzWriter)
	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Mode:     entry.mode,
			Size:     int64(len(entry.body)),
			Typeflag: entry.typeFlag,
		}
		if entry.typeFlag != tar.TypeReg {
			header.Size = 0
		}
		errHeader := tarWriter.WriteHeader(header)
		if errHeader != nil {
			t.Fatalf("WriteHeader(%q) error = %v", entry.name, errHeader)
		}
		if header.Size > 0 {
			_, errWrite := tarWriter.Write([]byte(entry.body))
			if errWrite != nil {
				t.Fatalf("Write(%q) error = %v", entry.name, errWrite)
			}
		}
	}
	errTarClose := tarWriter.Close()
	if errTarClose != nil {
		t.Fatalf("tarWriter.Close() error = %v", errTarClose)
	}
	errXZClose := xzWriter.Close()
	if errXZClose != nil {
		t.Fatalf("xzWriter.Close() error = %v", errXZClose)
	}

	archivePath := filepath.Join(t.TempDir(), "factorio.tar.xz")
	errWrite := os.WriteFile(archivePath, archive.Bytes(), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(test archive) error = %v", errWrite)
	}
	return archivePath
}
