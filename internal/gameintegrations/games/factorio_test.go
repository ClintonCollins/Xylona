package games

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestFactorioDownloadArchiveBounds(t *testing.T) {
	testCases := []struct {
		name      string
		handler   http.HandlerFunc
		wantError string
	}{
		{
			name: "rejects oversized content length before streaming",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				writer.Header().Set("Content-Length", "9")
				writer.WriteHeader(http.StatusOK)
			},
			wantError: "Content-Length exceeds 8 bytes",
		},
		{
			name: "rejects oversized chunked response",
			handler: func(writer http.ResponseWriter, _ *http.Request) {
				flusher, ok := writer.(http.Flusher)
				if ok {
					flusher.Flush()
				}
				_, errWrite := writer.Write([]byte("123456789"))
				if errWrite != nil {
					t.Errorf("response Write() error = %v", errWrite)
				}
			},
			wantError: "archive exceeds 8 bytes",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(testCase.handler)
			defer server.Close()
			game := &Factorio{
				headlessURL:     server.URL,
				httpClient:      server.Client(),
				archiveMaxBytes: 8,
			}
			archivePath := filepath.Join(t.TempDir(), "factorio.tar.xz")

			errDownload := game.downloadArchive(archivePath)
			if errDownload == nil || !strings.Contains(errDownload.Error(), testCase.wantError) {
				t.Fatalf("downloadArchive() error = %v, want substring %q", errDownload, testCase.wantError)
			}
			_, errStat := os.Stat(archivePath)
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("Stat(incomplete archive) error = %v, want not exist", errStat)
			}
		})
	}
}

func TestExtractFactorioTarLimitsAndPreflight(t *testing.T) {
	testCases := []struct {
		name      string
		entries   []factorioTestArchiveEntry
		limits    factorioArchiveLimits
		wantError string
	}{
		{
			name: "per-entry bytes",
			entries: []factorioTestArchiveEntry{
				{name: "factorio/data/large.dat", body: "12345", mode: 0o640, typeFlag: tar.TypeReg},
			},
			limits:    factorioArchiveLimits{entryBytes: 4, totalBytes: 10, files: 10},
			wantError: "exceeds 4 bytes",
		},
		{
			name: "aggregate bytes",
			entries: []factorioTestArchiveEntry{
				{name: "factorio/data/one.dat", body: "123", mode: 0o640, typeFlag: tar.TypeReg},
				{name: "factorio/data/two.dat", body: "456", mode: 0o640, typeFlag: tar.TypeReg},
			},
			limits:    factorioArchiveLimits{entryBytes: 10, totalBytes: 5, files: 10},
			wantError: "exceeds 5 extracted bytes",
		},
		{
			name: "regular file count",
			entries: []factorioTestArchiveEntry{
				{name: "factorio/data/one.dat", body: "1", mode: 0o640, typeFlag: tar.TypeReg},
				{name: "factorio/data/two.dat", body: "2", mode: 0o640, typeFlag: tar.TypeReg},
			},
			limits:    factorioArchiveLimits{entryBytes: 10, totalBytes: 10, files: 1},
			wantError: "exceeds 1 regular files",
		},
		{
			name: "duplicate normalized path",
			entries: []factorioTestArchiveEntry{
				{name: "factorio/data/file.dat", body: "1", mode: 0o640, typeFlag: tar.TypeReg},
				{name: "factorio/data/file.dat", body: "2", mode: 0o640, typeFlag: tar.TypeReg},
			},
			limits:    factorioArchiveLimits{entryBytes: 10, totalBytes: 10, files: 10},
			wantError: "duplicate normalized path",
		},
		{
			name: "file directory collision",
			entries: []factorioTestArchiveEntry{
				{name: "factorio/data", body: "file", mode: 0o640, typeFlag: tar.TypeReg},
				{name: "factorio/data/child", body: "child", mode: 0o640, typeFlag: tar.TypeReg},
			},
			limits:    factorioArchiveLimits{entryBytes: 10, totalBytes: 20, files: 10},
			wantError: "collides with file",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			archiveBytes := buildFactorioTestTar(t, testCase.entries)
			errExtract := extractFactorioRawTarForTest(t, archiveBytes, testCase.limits)
			if errExtract == nil || !strings.Contains(errExtract.Error(), testCase.wantError) {
				t.Fatalf("extractFactorioTar() error = %v, want substring %q", errExtract, testCase.wantError)
			}
		})
	}

	t.Run("truncated regular entry", func(t *testing.T) {
		var archive bytes.Buffer
		writer := tar.NewWriter(&archive)
		errHeader := writer.WriteHeader(&tar.Header{
			Name:     "factorio/data/truncated.dat",
			Mode:     0o640,
			Size:     10,
			Typeflag: tar.TypeReg,
		})
		if errHeader != nil {
			t.Fatalf("WriteHeader() error = %v", errHeader)
		}
		_, errWrite := writer.Write([]byte("abc"))
		if errWrite != nil {
			t.Fatalf("Write() error = %v", errWrite)
		}

		errExtract := extractFactorioRawTarForTest(t, archive.Bytes(), factorioArchiveLimits{
			entryBytes: 20,
			totalBytes: 20,
			files:      2,
		})
		if errExtract == nil || !errors.Is(errExtract, io.ErrUnexpectedEOF) {
			t.Fatalf("extractFactorioTar() error = %v, want unexpected EOF", errExtract)
		}
	})
}

type factorioTestArchiveEntry struct {
	name     string
	body     string
	mode     int64
	typeFlag byte
}

func writeFactorioTestArchive(t *testing.T, entries []factorioTestArchiveEntry) string {
	t.Helper()

	tarBytes := buildFactorioTestTar(t, entries)
	var archive bytes.Buffer
	xzWriter, errXZ := xz.NewWriter(&archive)
	if errXZ != nil {
		t.Fatalf("xz.NewWriter() error = %v", errXZ)
	}
	_, errWriteTar := xzWriter.Write(tarBytes)
	if errWriteTar != nil {
		t.Fatalf("xzWriter.Write() error = %v", errWriteTar)
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

func buildFactorioTestTar(t *testing.T, entries []factorioTestArchiveEntry) []byte {
	t.Helper()

	var archive bytes.Buffer
	tarWriter := tar.NewWriter(&archive)
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
	return archive.Bytes()
}

func extractFactorioRawTarForTest(t *testing.T, archive []byte, limits factorioArchiveLimits) error {
	t.Helper()

	root, errRoot := os.OpenRoot(t.TempDir())
	if errRoot != nil {
		t.Fatalf("OpenRoot() error = %v", errRoot)
	}
	defer func() {
		errClose := root.Close()
		if errClose != nil {
			t.Errorf("root.Close() error = %v", errClose)
		}
	}()
	return extractFactorioTar(root, tar.NewReader(bytes.NewReader(archive)), limits)
}
