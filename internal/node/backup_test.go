package node

import (
	"archive/zip"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCreateBackupArchiveWritesZipOnNode(t *testing.T) {
	root := t.TempDir()
	worldDir := filepath.Join(root, "world")
	errMkdir := os.MkdirAll(worldDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll world error = %v", errMkdir)
	}
	errWrite := os.WriteFile(filepath.Join(worldDir, "level.dat"), []byte("node-level"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile level error = %v", errWrite)
	}

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	n := &Node{}
	sizeBytes, checksum, errCreate := n.CreateBackupArchive(t.Context(), root, nil, archivePath)
	if errCreate != nil {
		t.Fatalf("CreateBackupArchive() error = %v", errCreate)
	}
	if sizeBytes <= 0 {
		t.Fatalf("CreateBackupArchive() size = %d, want > 0", sizeBytes)
	}
	if checksum == "" {
		t.Fatal("CreateBackupArchive() checksum is empty")
	}

	reader, errOpen := zip.OpenReader(archivePath)
	if errOpen != nil {
		t.Fatalf("OpenReader backup error = %v", errOpen)
	}
	defer func() {
		errClose := reader.Close()
		if errClose != nil {
			t.Fatalf("Close backup reader error = %v", errClose)
		}
	}()

	entries := make(map[string]struct{}, len(reader.File))
	for _, file := range reader.File {
		entries[file.Name] = struct{}{}
	}
	if _, ok := entries["world/"]; !ok {
		t.Fatalf("backup entries = %v, want world directory", entries)
	}
	if _, ok := entries["world/level.dat"]; !ok {
		t.Fatalf("backup entries = %v, want world/level.dat", entries)
	}
}

func TestCreateBackupArchiveRejectsTraversalInclude(t *testing.T) {
	root := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "backup.zip")

	n := &Node{}
	_, _, errCreate := n.CreateBackupArchive(t.Context(), root, []string{"../outside"}, archivePath)
	if errCreate == nil {
		t.Fatal("CreateBackupArchive() error = nil, want traversal include rejection")
	}
	if _, errStat := os.Stat(archivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat archive error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestCreateBackupArchiveRejectsSymlinkAndCleansPartial(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires extra privileges in some environments")
	}

	root := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(root, "target.txt"), []byte("target"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile target error = %v", errWrite)
	}
	errSymlink := os.Symlink("target.txt", filepath.Join(root, "linked.txt"))
	if errSymlink != nil {
		t.Fatalf("Symlink error = %v", errSymlink)
	}

	archivePath := filepath.Join(t.TempDir(), "backup.zip")
	n := &Node{}
	_, _, errCreate := n.CreateBackupArchive(t.Context(), root, nil, archivePath)
	if errCreate == nil {
		t.Fatal("CreateBackupArchive() error = nil, want symlink rejection")
	}
	if _, errStat := os.Stat(archivePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat archive error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestExtractBackupArchiveCorruptArchiveLeavesLiveFileUnchanged(t *testing.T) {
	root := t.TempDir()
	livePath := filepath.Join(root, "world", "level.dat")
	errMkdir := os.MkdirAll(filepath.Dir(livePath), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll live parent error = %v", errMkdir)
	}
	errWriteLive := os.WriteFile(livePath, []byte("live-level"), 0o600)
	if errWriteLive != nil {
		t.Fatalf("WriteFile live level error = %v", errWriteLive)
	}

	archivePath := filepath.Join(t.TempDir(), "restore.zip")
	writeCorruptBackupArchive(t, archivePath, "world/level.dat", []byte("archive-level-replacement"))

	n := &Node{}
	errExtract := n.ExtractBackupArchive(t.Context(), root, archivePath, ExtractModeOverlay)
	if errExtract == nil {
		t.Fatal("ExtractBackupArchive error = nil, want corrupt archive error")
	}

	contents, errRead := os.ReadFile(livePath)
	if errRead != nil {
		t.Fatalf("ReadFile live level error = %v", errRead)
	}
	if string(contents) != "live-level" {
		t.Fatalf("live level contents = %q, want unchanged %q", contents, "live-level")
	}
}

func TestExtractBackupArchiveOverlayPreservesExtraFiles(t *testing.T) {
	root := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("keep"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile extra error = %v", errWrite)
	}

	archivePath := filepath.Join(t.TempDir(), "restore.zip")
	writeNodeBackupArchive(t, archivePath, map[string]string{
		"world/level.dat": "restored",
	})

	n := &Node{}
	errExtract := n.ExtractBackupArchive(t.Context(), root, archivePath, ExtractModeOverlay)
	if errExtract != nil {
		t.Fatalf("ExtractBackupArchive() error = %v", errExtract)
	}

	extraContents, errRead := os.ReadFile(filepath.Join(root, "extra.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile extra error = %v", errRead)
	}
	if string(extraContents) != "keep" {
		t.Fatalf("extra contents = %q, want keep", string(extraContents))
	}
	restoredContents, errRead := os.ReadFile(filepath.Join(root, "world", "level.dat"))
	if errRead != nil {
		t.Fatalf("ReadFile restored error = %v", errRead)
	}
	if string(restoredContents) != "restored" {
		t.Fatalf("restored contents = %q, want restored", string(restoredContents))
	}
}

func TestExtractBackupArchiveExactDeletesExtraFiles(t *testing.T) {
	root := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(root, "extra.txt"), []byte("remove"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile extra error = %v", errWrite)
	}

	archivePath := filepath.Join(t.TempDir(), "restore.zip")
	writeNodeBackupArchive(t, archivePath, map[string]string{
		"keep.txt": "restored",
	})

	n := &Node{}
	errExtract := n.ExtractBackupArchive(t.Context(), root, archivePath, ExtractModeExact)
	if errExtract != nil {
		t.Fatalf("ExtractBackupArchive() error = %v", errExtract)
	}
	if _, errStat := os.Stat(filepath.Join(root, "extra.txt")); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat extra error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestExtractBackupArchiveRejectsDestinationSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires extra privileges in some environments")
	}

	root := t.TempDir()
	outsidePath := filepath.Join(t.TempDir(), "outside.txt")
	linkPath := filepath.Join(root, "link.txt")
	errLink := os.Symlink(outsidePath, linkPath)
	if errLink != nil {
		t.Fatalf("Symlink error = %v", errLink)
	}

	archivePath := filepath.Join(t.TempDir(), "restore.zip")
	writeNodeBackupArchive(t, archivePath, map[string]string{
		"link.txt": "blocked",
	})

	n := &Node{}
	errExtract := n.ExtractBackupArchive(t.Context(), root, archivePath, ExtractModeOverlay)
	if !errors.Is(errExtract, ErrRestoreDestinationSymlink) {
		t.Fatalf("ExtractBackupArchive() error = %v, want %v", errExtract, ErrRestoreDestinationSymlink)
	}
	if _, errStat := os.Stat(outsidePath); !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("Stat outside error = %v, want %v", errStat, os.ErrNotExist)
	}
}

func TestExtractBackupArchiveExactDoesNotPruneBeforeFailedExtraction(t *testing.T) {
	root := t.TempDir()
	orphanPath := filepath.Join(root, "orphan.txt")
	errWriteOrphan := os.WriteFile(orphanPath, []byte("keep-me"), 0o600)
	if errWriteOrphan != nil {
		t.Fatalf("WriteFile orphan error = %v", errWriteOrphan)
	}

	archivePath := filepath.Join(t.TempDir(), "restore.zip")
	writeCorruptBackupArchive(t, archivePath, "world/level.dat", []byte("archive-level"))

	n := &Node{}
	errExtract := n.ExtractBackupArchive(t.Context(), root, archivePath, ExtractModeExact)
	if errExtract == nil {
		t.Fatal("ExtractBackupArchive error = nil, want corrupt archive error")
	}

	contents, errRead := os.ReadFile(orphanPath)
	if errRead != nil {
		t.Fatalf("ReadFile orphan error = %v", errRead)
	}
	if string(contents) != "keep-me" {
		t.Fatalf("orphan contents = %q, want unchanged %q", contents, "keep-me")
	}
}

func writeNodeBackupArchive(t *testing.T, archivePath string, entries map[string]string) {
	t.Helper()

	archiveFile, errCreate := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		t.Fatalf("OpenFile archive error = %v", errCreate)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for name, contents := range entries {
		writer, errEntry := zipWriter.Create(name)
		if errEntry != nil {
			t.Fatalf("Create zip entry error = %v", errEntry)
		}
		_, errWrite := writer.Write([]byte(contents))
		if errWrite != nil {
			t.Fatalf("Write zip entry error = %v", errWrite)
		}
	}
	errCloseZip := zipWriter.Close()
	errCloseFile := archiveFile.Close()
	if errCloseZip != nil {
		t.Fatalf("Close zip writer error = %v", errCloseZip)
	}
	if errCloseFile != nil {
		t.Fatalf("Close archive file error = %v", errCloseFile)
	}
}

func writeCorruptBackupArchive(t *testing.T, archivePath string, entryName string, contents []byte) {
	t.Helper()

	archiveFile, errCreate := os.OpenFile(archivePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errCreate != nil {
		t.Fatalf("OpenFile archive error = %v", errCreate)
	}
	zipWriter := zip.NewWriter(archiveFile)
	writer, errEntry := zipWriter.Create(entryName)
	if errEntry != nil {
		t.Fatalf("Create zip entry error = %v", errEntry)
	}
	_, errWrite := writer.Write(contents)
	if errWrite != nil {
		t.Fatalf("Write zip entry error = %v", errWrite)
	}
	errCloseZip := zipWriter.Close()
	if errCloseZip != nil {
		t.Fatalf("Close zip writer error = %v", errCloseZip)
	}
	errCloseFile := archiveFile.Close()
	if errCloseFile != nil {
		t.Fatalf("Close archive file error = %v", errCloseFile)
	}

	reader, errOpen := zip.OpenReader(archivePath)
	if errOpen != nil {
		t.Fatalf("OpenReader archive error = %v", errOpen)
	}
	if len(reader.File) != 1 {
		t.Fatalf("archive entry count = %d, want 1", len(reader.File))
	}
	offset, errOffset := reader.File[0].DataOffset()
	if errOffset != nil {
		t.Fatalf("DataOffset error = %v", errOffset)
	}
	if reader.File[0].CompressedSize64 == 0 {
		t.Fatal("compressed zip entry size = 0, want corruptible data")
	}
	errCloseReader := reader.Close()
	if errCloseReader != nil {
		t.Fatalf("Close reader error = %v", errCloseReader)
	}

	archiveBytes, errRead := os.ReadFile(archivePath)
	if errRead != nil {
		t.Fatalf("ReadFile archive error = %v", errRead)
	}
	if offset < 0 || int64(len(archiveBytes)) <= offset {
		t.Fatalf("zip data offset = %d outside archive size %d", offset, len(archiveBytes))
	}
	archiveBytes[offset] ^= 0xff
	errWriteArchive := os.WriteFile(archivePath, archiveBytes, 0o600) //nolint:gosec // G703: test archive path is created under t.TempDir.
	if errWriteArchive != nil {
		t.Fatalf("WriteFile corrupted archive error = %v", errWriteArchive)
	}
}
