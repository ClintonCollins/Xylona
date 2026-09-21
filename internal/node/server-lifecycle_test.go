package node

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServerLifecycleGuardSerializesFileWrites(t *testing.T) {
	directory := t.TempDir()
	nodeInst := &Node{}
	unlock, errLock := nodeInst.lockServerLifecycle(directory)
	if errLock != nil {
		t.Fatal(errLock)
	}
	released := false
	defer func() {
		if !released {
			unlock()
		}
	}()
	done := make(chan error, 1)
	go func() { done <- nodeInst.WriteFile(directory, "adminlist.txt", []byte("player"), ProtectionPolicy{}) }()
	select {
	case errWrite := <-done:
		t.Fatalf("write passed held lifecycle guard: %v", errWrite)
	case <-time.After(25 * time.Millisecond):
	}
	unlock()
	released = true
	select {
	case errWrite := <-done:
		if errWrite != nil {
			t.Fatal(errWrite)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("write did not resume after lifecycle guard released")
	}
	data, errRead := os.ReadFile(filepath.Join(directory, "adminlist.txt"))
	if errRead != nil {
		t.Fatal(errRead)
	}
	if string(data) != "player" {
		t.Fatalf("content = %q", data)
	}
}

func TestReplaceRootedFilePreservesTargetOnFailure(t *testing.T) {
	directory := t.TempDir()
	errWrite := os.WriteFile(filepath.Join(directory, "adminlist.txt"), []byte("original"), 0o600)
	if errWrite != nil {
		t.Fatal(errWrite)
	}
	root, errRoot := os.OpenRoot(directory)
	if errRoot != nil {
		t.Fatal(errRoot)
	}
	defer closeMutationRoot(root)
	errReplace := replaceRootedFile(root, "missing-temp", "adminlist.txt")
	if errReplace == nil {
		t.Fatal("expected missing source to fail")
	}
	data, errRead := root.ReadFile("adminlist.txt")
	if errRead != nil {
		t.Fatal(errRead)
	}
	if string(data) != "original" {
		t.Fatalf("target changed after failed replacement: %q", data)
	}
}
