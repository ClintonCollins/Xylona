package node

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLocalPath(t *testing.T) {
	tests := []struct {
		name         string
		relativePath string
		want         string
		wantErr      error
	}{
		{name: "empty path returns empty", relativePath: "", want: "", wantErr: nil},
		{name: "leading slash trimmed", relativePath: "/world.txt", want: "world.txt", wantErr: nil},
		{name: "nested local path kept", relativePath: "configs/server.cfg", want: "configs/server.cfg", wantErr: nil},
		{name: "parent traversal rejected", relativePath: "../escape.txt", want: "", wantErr: ErrInvalidPath},
		{name: "leading slash with traversal rejected", relativePath: "/../../etc/passwd", want: "", wantErr: ErrInvalidPath},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validateLocalPath(tt.relativePath)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("validateLocalPath(%q) error = %v, want %v", tt.relativePath, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateLocalPath(%q) unexpected error = %v", tt.relativePath, err)
			}
			if got != tt.want {
				t.Fatalf("validateLocalPath(%q) = %q, want %q", tt.relativePath, got, tt.want)
			}
		})
	}
}

func TestListFilesReturnsEntries(t *testing.T) {
	dir := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("hello"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile error = %v", errWrite)
	}
	errMkdir := os.Mkdir(filepath.Join(dir, "sub"), 0o750)
	if errMkdir != nil {
		t.Fatalf("Mkdir error = %v", errMkdir)
	}

	n := &Node{}
	entries, errList := n.ListFiles(dir, "")
	if errList != nil {
		t.Fatalf("ListFiles error = %v", errList)
	}
	if len(entries) != 2 {
		t.Fatalf("ListFiles returned %d entries, want 2", len(entries))
	}

	byName := map[string]FileEntry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	if !byName["sub"].IsDirectory {
		t.Fatalf("expected sub to be a directory")
	}
	if byName["a.txt"].Size != int64(len("hello")) {
		t.Fatalf("ListFiles file size = %d, want %d", byName["a.txt"].Size, len("hello"))
	}
}

func TestListFilesRejectsTraversal(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}
	_, err := n.ListFiles(dir, "../../etc")
	if !errors.Is(err, ErrInvalidPath) {
		t.Fatalf("ListFiles error = %v, want %v", err, ErrInvalidPath)
	}
}

func TestCreateFileOrDirectoryAndDelete(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	errCreateDir := n.CreateFileOrDirectory(dir, "configs", "", true, ProtectionPolicy{})
	if errCreateDir != nil {
		t.Fatalf("CreateFileOrDirectory dir error = %v", errCreateDir)
	}
	if _, errStat := os.Stat(filepath.Join(dir, "configs")); errStat != nil {
		t.Fatalf("expected configs dir, stat error = %v", errStat)
	}

	errCreateFile := n.CreateFileOrDirectory(dir, "configs/server.cfg", "name=value", false, ProtectionPolicy{})
	if errCreateFile != nil {
		t.Fatalf("CreateFileOrDirectory file error = %v", errCreateFile)
	}
	contents, errRead := os.ReadFile(filepath.Join(dir, "configs", "server.cfg"))
	if errRead != nil {
		t.Fatalf("ReadFile error = %v", errRead)
	}
	if string(contents) != "name=value" {
		t.Fatalf("file contents = %q, want %q", contents, "name=value")
	}

	deleted, errDelete := n.DeleteFiles(t.Context(), dir, []string{"configs/server.cfg"}, ProtectionPolicy{})
	if errDelete != nil {
		t.Fatalf("DeleteFiles error = %v", errDelete)
	}
	if len(deleted) != 1 || deleted[0] != "configs/server.cfg" {
		t.Fatalf("DeleteFiles returned %v, want [configs/server.cfg]", deleted)
	}
}

func TestRenameAndMove(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	errCreate := n.CreateFileOrDirectory(dir, "old.txt", "data", false, ProtectionPolicy{})
	if errCreate != nil {
		t.Fatalf("CreateFileOrDirectory error = %v", errCreate)
	}

	newPath, errRename := n.RenameFile(dir, "old.txt", "new.txt", ProtectionPolicy{})
	if errRename != nil {
		t.Fatalf("RenameFile error = %v", errRename)
	}
	if newPath != "new.txt" {
		t.Fatalf("RenameFile returned %q, want %q", newPath, "new.txt")
	}

	errMkdir := n.CreateFileOrDirectory(dir, "dest", "", true, ProtectionPolicy{})
	if errMkdir != nil {
		t.Fatalf("CreateFileOrDirectory(dest) error = %v", errMkdir)
	}
	moved, errMove := n.MoveFiles(t.Context(), dir, []string{"new.txt"}, "dest", ProtectionPolicy{})
	if errMove != nil {
		t.Fatalf("MoveFiles error = %v", errMove)
	}
	if len(moved) != 1 {
		t.Fatalf("MoveFiles returned %v, want one entry", moved)
	}
	if _, errStat := os.Stat(filepath.Join(dir, "dest", "new.txt")); errStat != nil {
		t.Fatalf("expected moved file, stat error = %v", errStat)
	}
}

func TestWriteProtectionEnforced(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	cases := []struct {
		name        string
		relPath     string
		policy      ProtectionPolicy
		expectError error
	}{
		{
			name:        "server executable rejected",
			relPath:     "server.jar",
			policy:      ProtectionPolicy{ServerExecutable: "server.jar"},
			expectError: ErrProtectedPath,
		},
		{
			name:        "base command with path rejected",
			relPath:     "run.sh",
			policy:      ProtectionPolicy{BaseCommand: "./run.sh"},
			expectError: ErrProtectedPath,
		},
		{
			name:        "unrelated path allowed",
			relPath:     "notes.txt",
			policy:      ProtectionPolicy{ServerExecutable: "server.jar"},
			expectError: nil,
		},
		{
			name:        "empty policy skips check",
			relPath:     "server.jar",
			policy:      ProtectionPolicy{},
			expectError: nil,
		},
		{
			name:        "bare java not treated as protected",
			relPath:     "java",
			policy:      ProtectionPolicy{BaseCommand: "java"},
			expectError: nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			errWrite := n.WriteFile(dir, tc.relPath, []byte("x"), tc.policy)
			if tc.expectError != nil {
				if !errors.Is(errWrite, tc.expectError) {
					t.Fatalf("WriteFile policy=%+v err = %v, want %v", tc.policy, errWrite, tc.expectError)
				}
				return
			}
			if errWrite != nil {
				t.Fatalf("WriteFile policy=%+v unexpected err = %v", tc.policy, errWrite)
			}
		})
	}
}

func TestReadAndWriteFile(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	errWrite := n.WriteFile(dir, "notes.txt", []byte("payload"), ProtectionPolicy{})
	if errWrite != nil {
		t.Fatalf("WriteFile error = %v", errWrite)
	}

	got, errRead := n.ReadFile(dir, "notes.txt")
	if errRead != nil {
		t.Fatalf("ReadFile error = %v", errRead)
	}
	if string(got) != "payload" {
		t.Fatalf("ReadFile = %q, want %q", got, "payload")
	}
}
