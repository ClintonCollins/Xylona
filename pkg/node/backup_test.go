package node

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

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
