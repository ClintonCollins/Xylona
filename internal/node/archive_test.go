package node

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCreateAndExtractFileArchive(t *testing.T) {
	dir := t.TempDir()
	errMkdir := os.MkdirAll(filepath.Join(dir, "logs"), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll logs error = %v", errMkdir)
	}
	errWrite := os.WriteFile(filepath.Join(dir, "logs", "latest.log"), []byte("hello"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile latest.log error = %v", errWrite)
	}
	errMkdirArchive := os.MkdirAll(filepath.Join(dir, "archives"), 0o750)
	if errMkdirArchive != nil {
		t.Fatalf("MkdirAll archives error = %v", errMkdirArchive)
	}

	n := &Node{}
	archivePath, archiveProgress, errArchive := n.CreateFileArchive(
		t.Context(),
		dir,
		"archives/latest-log",
		[]string{"logs/latest.log"},
		ArchiveCompressionZIP,
		ProtectionPolicy{},
	)
	if errArchive != nil {
		t.Fatalf("CreateFileArchive error = %v", errArchive)
	}
	if archivePath != "archives/latest-log.zip" {
		t.Fatalf("CreateFileArchive path = %q, want %q", archivePath, "archives/latest-log.zip")
	}
	if archiveProgress.FilesCompressed != 1 {
		t.Fatalf("CreateFileArchive files compressed = %d, want 1", archiveProgress.FilesCompressed)
	}

	extracted, extractProgress, errExtract := n.ExtractFileArchive(
		t.Context(),
		dir,
		archivePath,
		"restored",
		ProtectionPolicy{},
	)
	if errExtract != nil {
		t.Fatalf("ExtractFileArchive error = %v", errExtract)
	}
	if len(extracted) != 1 || extracted[0] != "latest.log" {
		t.Fatalf("ExtractFileArchive paths = %v, want [latest.log]", extracted)
	}
	if extractProgress.FilesExtracted != 1 {
		t.Fatalf("ExtractFileArchive files extracted = %d, want 1", extractProgress.FilesExtracted)
	}
	contents, errRead := os.ReadFile(filepath.Join(dir, "restored", "latest.log"))
	if errRead != nil {
		t.Fatalf("ReadFile restored latest.log error = %v", errRead)
	}
	if string(contents) != "hello" {
		t.Fatalf("restored contents = %q, want %q", contents, "hello")
	}
}

func TestCreateFileArchiveRejectsProtectedDestination(t *testing.T) {
	dir := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("hello"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile notes.txt error = %v", errWrite)
	}

	n := &Node{}
	_, _, errArchive := n.CreateFileArchive(
		t.Context(),
		dir,
		"server",
		[]string{"notes.txt"},
		ArchiveCompressionZIP,
		ProtectionPolicy{ServerExecutable: "server.zip"},
	)
	if !errors.Is(errArchive, ErrProtectedPath) {
		t.Fatalf("CreateFileArchive error = %v, want %v", errArchive, ErrProtectedPath)
	}
}

func TestCreateFileArchiveRejectsTraversalSource(t *testing.T) {
	dir := t.TempDir()
	outsidePath := filepath.Join(filepath.Dir(dir), "outside.txt")
	errWrite := os.WriteFile(outsidePath, []byte("secret"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile outside.txt error = %v", errWrite)
	}

	n := &Node{}
	_, _, errArchive := n.CreateFileArchive(
		t.Context(),
		dir,
		"archive",
		[]string{"../outside.txt"},
		ArchiveCompressionZIP,
		ProtectionPolicy{},
	)
	if !errors.Is(errArchive, ErrInvalidPath) {
		t.Fatalf("CreateFileArchive error = %v, want %v", errArchive, ErrInvalidPath)
	}
}

func TestExtractFileArchiveRejectsEscapingEntry(t *testing.T) {
	_, _, errEntry := cleanArchiveEntryPath("../escape.txt")
	if !errors.Is(errEntry, ErrInvalidPath) {
		t.Fatalf("cleanArchiveEntryPath error = %v, want %v", errEntry, ErrInvalidPath)
	}
}

func TestExtractFileArchiveRejectsProtectedEntry(t *testing.T) {
	dir := t.TempDir()
	archivePath := filepath.Join(dir, "import.zip")
	createNodeTestZipArchive(t, archivePath, map[string]string{
		"server.jar": "blocked",
	})

	n := &Node{}
	_, _, errExtract := n.ExtractFileArchive(
		t.Context(),
		dir,
		"import.zip",
		"",
		ProtectionPolicy{ServerExecutable: "server.jar"},
	)
	if !errors.Is(errExtract, ErrProtectedPath) {
		t.Fatalf("ExtractFileArchive error = %v, want %v", errExtract, ErrProtectedPath)
	}

	protectedPath := filepath.Join(dir, "server.jar")
	if _, errStat := os.Stat(protectedPath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want %v", protectedPath, errStat, os.ErrNotExist)
	}
}

func TestExtractFileArchiveEnforcesExpansionBudgets(t *testing.T) {
	tests := []struct {
		name             string
		maxEntries       int64
		maxEntryBytes    int64
		maxTotalBytes    int64
		entries          []nodeTestZipEntry
		wantError        string
		wantMissingEntry string
	}{
		{
			name:          "entry count",
			maxEntries:    1,
			maxEntryBytes: 100,
			maxTotalBytes: 100,
			entries: []nodeTestZipEntry{
				{name: "a.txt", contents: "a"},
				{name: "b.txt", contents: "b"},
			},
			wantError:        "more than 1 entries",
			wantMissingEntry: "a.txt",
		},
		{
			name:             "per entry actual bytes",
			maxEntries:       10,
			maxEntryBytes:    3,
			maxTotalBytes:    100,
			entries:          []nodeTestZipEntry{{name: "large.txt", contents: "1234"}},
			wantError:        "entry exceeds 3 bytes",
			wantMissingEntry: "large.txt",
		},
		{
			name:          "total actual bytes",
			maxEntries:    10,
			maxEntryBytes: 10,
			maxTotalBytes: 5,
			entries: []nodeTestZipEntry{
				{name: "a.txt", contents: "123"},
				{name: "b.txt", contents: "456"},
			},
			wantError:        "exceeds 5 expanded bytes",
			wantMissingEntry: "b.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setArchiveExtractionLimits(t, tt.maxEntries, tt.maxEntryBytes, tt.maxTotalBytes)
			dir := t.TempDir()
			archivePath := filepath.Join(dir, "import.zip")
			writeOrderedNodeTestZipArchive(t, archivePath, tt.entries)

			n := &Node{}
			_, _, errExtract := n.ExtractFileArchive(t.Context(), dir, "import.zip", "restored", ProtectionPolicy{})
			if errExtract == nil || !strings.Contains(errExtract.Error(), tt.wantError) {
				t.Fatalf("ExtractFileArchive() error = %v, want containing %q", errExtract, tt.wantError)
			}
			partialPath := filepath.Join(dir, "restored", tt.wantMissingEntry)
			_, errStat := os.Stat(partialPath)
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("Stat(%q) error = %v, want %v", partialPath, errStat, os.ErrNotExist)
			}
		})
	}
}

func TestExtractFileArchiveRejectsDestinationSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires extra privileges in some environments")
	}

	dir := t.TempDir()
	outside := t.TempDir()
	errSymlink := os.Symlink(outside, filepath.Join(dir, "restored"))
	if errSymlink != nil {
		t.Fatalf("Symlink error = %v", errSymlink)
	}
	archivePath := filepath.Join(dir, "import.zip")
	writeOrderedNodeTestZipArchive(t, archivePath, []nodeTestZipEntry{{name: "escaped.txt", contents: "blocked"}})

	n := &Node{}
	_, _, errExtract := n.ExtractFileArchive(t.Context(), dir, "import.zip", "restored", ProtectionPolicy{})
	if errExtract == nil {
		t.Fatal("ExtractFileArchive() error = nil, want symlink escape rejection")
	}
	outsidePath := filepath.Join(outside, "escaped.txt")
	_, errStat := os.Stat(outsidePath)
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat(%q) error = %v, want %v", outsidePath, errStat, os.ErrNotExist)
	}
}

type nodeTestZipEntry struct {
	name     string
	contents string
}

func setArchiveExtractionLimits(t *testing.T, entries, entryBytes, totalBytes int64) {
	t.Helper()
	oldEntries := maxArchiveExtractEntries
	oldEntryBytes := maxArchiveExtractEntryBytes
	oldTotalBytes := maxArchiveExtractTotalBytes
	maxArchiveExtractEntries = entries
	maxArchiveExtractEntryBytes = entryBytes
	maxArchiveExtractTotalBytes = totalBytes
	t.Cleanup(func() {
		maxArchiveExtractEntries = oldEntries
		maxArchiveExtractEntryBytes = oldEntryBytes
		maxArchiveExtractTotalBytes = oldTotalBytes
	})
}

func writeOrderedNodeTestZipArchive(t *testing.T, archivePath string, entries []nodeTestZipEntry) {
	t.Helper()

	file, errCreate := os.Create(archivePath)
	if errCreate != nil {
		t.Fatalf("Create(%q) error = %v", archivePath, errCreate)
	}
	zipWriter := zip.NewWriter(file)
	for _, entry := range entries {
		writer, errCreateEntry := zipWriter.Create(entry.name)
		if errCreateEntry != nil {
			t.Fatalf("zip.Create(%q) error = %v", entry.name, errCreateEntry)
		}
		_, errWrite := writer.Write([]byte(entry.contents))
		if errWrite != nil {
			t.Fatalf("zip entry write %q error = %v", entry.name, errWrite)
		}
	}
	errCloseZip := zipWriter.Close()
	errCloseFile := file.Close()
	if errCloseZip != nil {
		t.Fatalf("zip.Close() error = %v", errCloseZip)
	}
	if errCloseFile != nil {
		t.Fatalf("file.Close() error = %v", errCloseFile)
	}
}

func createNodeTestZipArchive(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	file, errCreate := os.Create(archivePath)
	if errCreate != nil {
		t.Fatalf("Create(%q) error = %v", archivePath, errCreate)
	}
	zipWriter := zip.NewWriter(file)
	for name, contents := range entries {
		writer, errCreateEntry := zipWriter.Create(name)
		if errCreateEntry != nil {
			t.Fatalf("zip.Create(%q) error = %v", name, errCreateEntry)
		}
		_, errWrite := writer.Write([]byte(contents))
		if errWrite != nil {
			t.Fatalf("zip entry write %q error = %v", name, errWrite)
		}
	}
	errCloseZip := zipWriter.Close()
	if errCloseZip != nil {
		t.Fatalf("zip.Close() error = %v", errCloseZip)
	}
	errCloseFile := file.Close()
	if errCloseFile != nil {
		t.Fatalf("file.Close() error = %v", errCloseFile)
	}
}
