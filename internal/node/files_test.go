package node

import (
	"crypto/sha1" // #nosec G505 -- test coverage for Mojang-published checksum verification.
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"testing/iotest"
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

func TestListFilesReportsExecutableMetadata(t *testing.T) {
	dir := t.TempDir()

	executableName := "run.sh"
	executableMode := os.FileMode(0o700)
	executableContent := []byte("#!/bin/sh\n")
	if runtime.GOOS == "windows" {
		executableName = "run.cmd"
		executableMode = 0o600
		executableContent = []byte("@echo off\n")
	}

	executablePath := filepath.Join(dir, executableName)
	errWrite := os.WriteFile(executablePath, executableContent, executableMode)
	if errWrite != nil {
		t.Fatalf("WriteFile executable error = %v", errWrite)
	}
	errWriteData := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("plain"), 0o600)
	if errWriteData != nil {
		t.Fatalf("WriteFile data error = %v", errWriteData)
	}

	n := &Node{}
	entries, errList := n.ListFiles(dir, "")
	if errList != nil {
		t.Fatalf("ListFiles error = %v", errList)
	}

	byName := map[string]FileEntry{}
	for _, entry := range entries {
		byName[entry.Name] = entry
	}
	if !byName[executableName].IsExecutable {
		t.Fatalf("%s IsExecutable = false, want true", executableName)
	}
	if byName["notes.txt"].IsExecutable {
		t.Fatalf("notes.txt IsExecutable = true, want false")
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

func TestFileReadsRejectSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	errWrite := os.WriteFile(outsideFile, []byte("classified"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile outside error = %v", errWrite)
	}

	linkPath := filepath.Join(root, "escape-link")
	errLink := os.Symlink(outsideFile, linkPath)
	if errLink != nil {
		t.Skipf("symlinks unavailable: %v", errLink)
	}

	insideFile := filepath.Join(root, "safe.txt")
	errInside := os.WriteFile(insideFile, []byte("ok"), 0o600)
	if errInside != nil {
		t.Fatalf("WriteFile inside error = %v", errInside)
	}
	insideLink := filepath.Join(root, "inside-link")
	errInsideLink := os.Symlink(insideFile, insideLink)
	if errInsideLink != nil {
		t.Skipf("symlinks unavailable: %v", errInsideLink)
	}

	outsideDirLink := filepath.Join(root, "escape-dir")
	errDirLink := os.Symlink(outsideDir, outsideDirLink)
	if errDirLink != nil {
		t.Skipf("directory symlinks unavailable: %v", errDirLink)
	}

	n := &Node{}

	_, errRead := n.ReadFile(root, "escape-link")
	if !errors.Is(errRead, ErrInvalidPath) {
		t.Fatalf("ReadFile(escape-link) error = %v, want %v", errRead, ErrInvalidPath)
	}

	_, errOpen := n.OpenFile(root, "escape-link")
	if !errors.Is(errOpen, ErrInvalidPath) {
		t.Fatalf("OpenFile(escape-link) error = %v, want %v", errOpen, ErrInvalidPath)
	}

	_, errStat := n.StatFile(root, "escape-link")
	if !errors.Is(errStat, ErrInvalidPath) {
		t.Fatalf("StatFile(escape-link) error = %v, want %v", errStat, ErrInvalidPath)
	}

	_, errList := n.ListFiles(root, "escape-dir")
	if !errors.Is(errList, ErrInvalidPath) {
		t.Fatalf("ListFiles(escape-dir) error = %v, want %v", errList, ErrInvalidPath)
	}

	got, errReadInside := n.ReadFile(root, "inside-link")
	if errReadInside != nil {
		t.Fatalf("ReadFile(inside-link) error = %v", errReadInside)
	}
	if string(got) != "ok" {
		t.Fatalf("ReadFile(inside-link) = %q, want %q", got, "ok")
	}
}

func TestCreateFileOrDirectoryAndDelete(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "server")
	n := &Node{}

	errCreateDir := n.CreateFileOrDirectory(dir, "configs", "", true, ProtectionPolicy{})
	if errCreateDir != nil {
		t.Fatalf("CreateFileOrDirectory dir error = %v", errCreateDir)
	}
	_, errStatConfigs := os.Stat(filepath.Join(dir, "configs"))
	if errStatConfigs != nil {
		t.Fatalf("expected configs dir, stat error = %v", errStatConfigs)
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

	deleted, errDelete = n.DeleteFiles(t.Context(), dir, []string{""}, ProtectionPolicy{})
	if errDelete != nil {
		t.Fatalf("DeleteFiles root error = %v", errDelete)
	}
	if len(deleted) != 1 || deleted[0] != "" {
		t.Fatalf("DeleteFiles root returned %v, want [empty path]", deleted)
	}
	_, errStatRoot := os.Stat(dir)
	if !errors.Is(errStatRoot, os.ErrNotExist) {
		t.Fatalf("server root stat error = %v, want not exist", errStatRoot)
	}
}

func TestFileMutationsRejectSymlinkEscape(t *testing.T) {
	downloadServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write([]byte("payload"))
		if errWrite != nil {
			t.Errorf("write download response: %v", errWrite)
		}
	}))
	defer downloadServer.Close()

	tests := []struct {
		name        string
		outsideName string
		seedOutside bool
		mutate      func(t *testing.T, n *Node, root string) error
	}{
		{
			name:        "write",
			outsideName: "written.txt",
			mutate: func(_ *testing.T, n *Node, root string) error {
				return n.WriteFile(root, "escape/written.txt", []byte("escaped"), ProtectionPolicy{})
			},
		},
		{
			name:        "create file",
			outsideName: "created.txt",
			mutate: func(_ *testing.T, n *Node, root string) error {
				return n.CreateFileOrDirectory(root, "escape/created.txt", "escaped", false, ProtectionPolicy{})
			},
		},
		{
			name:        "create directory",
			outsideName: "created-dir",
			mutate: func(_ *testing.T, n *Node, root string) error {
				return n.CreateFileOrDirectory(root, "escape/created-dir", "", true, ProtectionPolicy{})
			},
		},
		{
			name:        "delete",
			outsideName: "victim.txt",
			seedOutside: true,
			mutate: func(t *testing.T, n *Node, root string) error {
				_, errDelete := n.DeleteFiles(t.Context(), root, []string{"escape/victim.txt"}, ProtectionPolicy{})
				return errDelete
			},
		},
		{
			name:        "rename",
			outsideName: "renamed.txt",
			mutate: func(_ *testing.T, n *Node, root string) error {
				_, errRename := n.RenameFile(root, "source.txt", "escape/renamed.txt", ProtectionPolicy{})
				return errRename
			},
		},
		{
			name:        "move",
			outsideName: "source.txt",
			mutate: func(t *testing.T, n *Node, root string) error {
				_, errMove := n.MoveFiles(t.Context(), root, []string{"source.txt"}, "escape", ProtectionPolicy{})
				return errMove
			},
		},
		{
			name:        "copy",
			outsideName: "copied.txt",
			mutate: func(t *testing.T, n *Node, root string) error {
				_, errCopy := n.CopyFiles(t.Context(), root, []CopyFileOperation{
					{SourceRelativePath: "source.txt", DestinationRelativePath: "escape/copied.txt"},
				}, ProtectionPolicy{})
				return errCopy
			},
		},
		{
			name:        "download",
			outsideName: "payload.txt",
			mutate: func(t *testing.T, n *Node, root string) error {
				withDownloadTestHTTPClient(t, downloadServer.Client())
				_, errDownload := n.DownloadFileFromURL(t.Context(), root, downloadServer.URL+"/payload.txt", "escape", DownloadIntegrity{}, ProtectionPolicy{})
				return errDownload
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "root")
			outside := filepath.Join(base, "outside")
			for _, directory := range []string{root, outside} {
				errMkdir := os.Mkdir(directory, 0o750)
				if errMkdir != nil {
					t.Fatalf("Mkdir(%s) error = %v", directory, errMkdir)
				}
			}
			errSymlink := os.Symlink(outside, filepath.Join(root, "escape"))
			if errSymlink != nil {
				t.Skipf("symlinks unavailable: %v", errSymlink)
			}
			errSource := os.WriteFile(filepath.Join(root, "source.txt"), []byte("source"), 0o600)
			if errSource != nil {
				t.Fatalf("write source: %v", errSource)
			}
			outsidePath := filepath.Join(outside, tt.outsideName)
			if tt.seedOutside {
				errSeed := os.WriteFile(outsidePath, []byte("keep"), 0o600)
				if errSeed != nil {
					t.Fatalf("seed outside file: %v", errSeed)
				}
			}

			errMutate := tt.mutate(t, &Node{}, root)
			if errMutate != nil {
				t.Logf("mutation rejected: %v", errMutate)
			}

			_, errStat := os.Stat(outsidePath)
			if tt.seedOutside {
				if errStat != nil {
					t.Fatalf("outside target was removed: %v", errStat)
				}
				return
			}
			if !errors.Is(errStat, os.ErrNotExist) {
				t.Fatalf("outside target stat error = %v, want not exist", errStat)
			}
		})
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
	_, errStatMoved := os.Stat(filepath.Join(dir, "dest", "new.txt"))
	if errStatMoved != nil {
		t.Fatalf("expected moved file, stat error = %v", errStatMoved)
	}

	moved, errMove = n.MoveFiles(t.Context(), dir, []string{"dest/new.txt"}, "", ProtectionPolicy{})
	if errMove != nil {
		t.Fatalf("MoveFiles to root error = %v", errMove)
	}
	if len(moved) != 1 || moved[0] != "dest/new.txt" {
		t.Fatalf("MoveFiles to root returned %v, want [dest/new.txt]", moved)
	}
	_, errStatRoot := os.Stat(filepath.Join(dir, "new.txt"))
	if errStatRoot != nil {
		t.Fatalf("expected file moved to root, stat error = %v", errStatRoot)
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

func TestDownloadFileFromURLRejectsNonSuccessStatus(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()
	withDownloadTestHTTPClient(t, server.Client())

	_, errDownload := n.DownloadFileFromURL(
		t.Context(),
		dir,
		server.URL+"/server.jar",
		"",
		DownloadIntegrity{},
		ProtectionPolicy{},
	)
	if !errors.Is(errDownload, ErrUnexpectedHTTPStatus) {
		t.Fatalf("DownloadFileFromURL() error = %v, want %v", errDownload, ErrUnexpectedHTTPStatus)
	}
	_, errStat := os.Stat(filepath.Join(dir, "server.jar"))
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("downloaded file stat error = %v, want not exist", errStat)
	}
}

func TestDownloadHTTPClientHasNoOverallTimeout(t *testing.T) {
	client := downloadHTTPClient()
	if client.Timeout != 0 {
		t.Fatalf("download HTTP client timeout = %v, want no overall timeout", client.Timeout)
	}
}

func TestDownloadFileFromURLRejectsLoopbackTarget(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	_, errDownload := n.DownloadFileFromURL(
		t.Context(),
		dir,
		"http://127.0.0.1:8080/file.txt",
		"",
		DownloadIntegrity{},
		ProtectionPolicy{},
	)
	if errDownload == nil {
		t.Fatal("DownloadFileFromURL() expected error, got nil")
	}
	if !strings.Contains(errDownload.Error(), "private or reserved") {
		t.Fatalf("DownloadFileFromURL() error = %v, want SSRF validation failure", errDownload)
	}
}

func TestDownloadFileFromURLRejectsPrivateTargetAtDialTime(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	originalValidate := validateDownloadTarget
	validateDownloadTarget = func(string) error { return nil }
	t.Cleanup(func() {
		validateDownloadTarget = originalValidate
	})

	_, errDownload := n.DownloadFileFromURL(
		t.Context(),
		dir,
		"http://127.0.0.1:8080/file.txt",
		"",
		DownloadIntegrity{},
		ProtectionPolicy{},
	)
	if errDownload == nil || !strings.Contains(errDownload.Error(), "private or reserved") {
		t.Fatalf("DownloadFileFromURL() error = %v, want dial-time SSRF rejection", errDownload)
	}
}

func TestValidateDownloadRedirectTargetRejectsPrivateRedirect(t *testing.T) {
	req, errRequest := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://127.0.0.1:8080/private.txt", nil)
	if errRequest != nil {
		t.Fatalf("NewRequest() error = %v", errRequest)
	}
	viaReq, errViaRequest := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://downloads.example.com/public.txt", nil)
	if errViaRequest != nil {
		t.Fatalf("NewRequest(via) error = %v", errViaRequest)
	}

	errValidate := validateDownloadRedirectTarget(req, []*http.Request{viaReq})
	if errValidate == nil {
		t.Fatal("validateDownloadRedirectTarget() expected error, got nil")
	}
	if !strings.Contains(errValidate.Error(), "private or reserved") {
		t.Fatalf("validateDownloadRedirectTarget() error = %v, want SSRF validation failure", errValidate)
	}
}

func TestDownloadFileFromURLVerifiesIntegrityBeforePromotion(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}
	body := []byte("server jar payload")
	sha256Sum := sha256.Sum256(body)
	sha1Sum := sha1.Sum(body) // #nosec G401 -- test coverage for Mojang-published checksum verification.

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write(body)
		if errWrite != nil {
			t.Fatalf("Write response: %v", errWrite)
		}
	}))
	defer server.Close()
	withDownloadTestHTTPClient(t, server.Client())

	result, errDownload := n.DownloadFileFromURL(
		t.Context(),
		dir,
		server.URL+"/server.jar",
		"",
		DownloadIntegrity{
			ExpectedSize:   int64(len(body)),
			ExpectedSHA256: fmt.Sprintf("%x", sha256Sum),
			ExpectedSHA1:   fmt.Sprintf("%x", sha1Sum),
		},
		ProtectionPolicy{},
	)
	if errDownload != nil {
		t.Fatalf("DownloadFileFromURL() error = %v", errDownload)
	}
	if result.RelativePath != "server.jar" {
		t.Fatalf("RelativePath = %q, want server.jar", result.RelativePath)
	}
	if result.BytesWritten != int64(len(body)) {
		t.Fatalf("BytesWritten = %d, want %d", result.BytesWritten, len(body))
	}
	if result.SHA256 != fmt.Sprintf("%x", sha256Sum) {
		t.Fatalf("SHA256 = %q, want %x", result.SHA256, sha256Sum)
	}
	if result.SHA1 != fmt.Sprintf("%x", sha1Sum) {
		t.Fatalf("SHA1 = %q, want %x", result.SHA1, sha1Sum)
	}
	data, errRead := os.ReadFile(filepath.Join(dir, "server.jar"))
	if errRead != nil {
		t.Fatalf("ReadFile downloaded artifact: %v", errRead)
	}
	if string(data) != string(body) {
		t.Fatalf("downloaded artifact = %q, want %q", data, body)
	}
}

func TestDownloadFileFromURLRemovesIntegrityMismatch(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, errWrite := w.Write([]byte("unexpected payload"))
		if errWrite != nil {
			t.Fatalf("Write response: %v", errWrite)
		}
	}))
	defer server.Close()
	withDownloadTestHTTPClient(t, server.Client())

	_, errDownload := n.DownloadFileFromURL(
		t.Context(),
		dir,
		server.URL+"/server.jar",
		"",
		DownloadIntegrity{
			ExpectedSize:   int64(len("expected payload")),
			ExpectedSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("expected payload"))),
		},
		ProtectionPolicy{},
	)
	if !errors.Is(errDownload, ErrDownloadIntegrityMismatch) {
		t.Fatalf("DownloadFileFromURL() error = %v, want %v", errDownload, ErrDownloadIntegrityMismatch)
	}
	_, errStat := os.Stat(filepath.Join(dir, "server.jar"))
	if !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("downloaded file stat error = %v, want not exist", errStat)
	}
}

func TestDownloadFileFromURLEnforcesHardSizeCap(t *testing.T) {
	bodyReadErr := errors.New("response body was read")
	tests := []struct {
		name          string
		contentLength int64
		body          io.ReadCloser
		wantErr       error
		wantBytes     int64
		wantDetail    string
	}{
		{
			name:          "advertised content length is rejected before reading",
			contentLength: 17,
			body:          io.NopCloser(iotest.ErrReader(bodyReadErr)),
			wantErr:       ErrDownloadTooLarge,
			wantDetail:    "content length 17 bytes exceeds limit 16 bytes",
		},
		{
			name:          "unknown content length is capped while streaming",
			contentLength: -1,
			body:          io.NopCloser(strings.NewReader("0123456789abcdef!!!")),
			wantErr:       ErrDownloadTooLarge,
		},
		{
			name:          "content length equal to the limit is allowed",
			contentLength: 16,
			body:          io.NopCloser(strings.NewReader("0123456789abcdef")),
			wantBytes:     16,
		},
	}

	originalLimit := maxDownloadFromURLBytes
	maxDownloadFromURLBytes = 16
	t.Cleanup(func() {
		maxDownloadFromURLBytes = originalLimit
	})

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			n := &Node{}

			client := &http.Client{Transport: downloadRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Status:        "200 OK",
					Header:        make(http.Header),
					Body:          tc.body,
					ContentLength: tc.contentLength,
					Request:       req,
				}, nil
			})}
			withDownloadTestHTTPClient(t, client)

			result, errDownload := n.DownloadFileFromURL(
				t.Context(),
				dir,
				"https://downloads.example.test/server.jar",
				"",
				DownloadIntegrity{},
				ProtectionPolicy{},
			)
			if tc.wantErr != nil {
				if !errors.Is(errDownload, tc.wantErr) {
					t.Fatalf("DownloadFileFromURL() error = %v, want %v", errDownload, tc.wantErr)
				}
				if errors.Is(errDownload, bodyReadErr) {
					t.Fatalf("DownloadFileFromURL() read an oversized response body: %v", errDownload)
				}
				if tc.wantDetail != "" && !strings.Contains(errDownload.Error(), tc.wantDetail) {
					t.Fatalf("DownloadFileFromURL() error = %v, want containing %q", errDownload, tc.wantDetail)
				}
				_, errStat := os.Stat(filepath.Join(dir, "server.jar"))
				if !errors.Is(errStat, os.ErrNotExist) {
					t.Fatalf("downloaded file stat error = %v, want not exist", errStat)
				}
				return
			}
			if errDownload != nil {
				t.Fatalf("DownloadFileFromURL() unexpected error = %v", errDownload)
			}
			if result.BytesWritten != tc.wantBytes {
				t.Fatalf("DownloadFileFromURL() bytes written = %d, want %d", result.BytesWritten, tc.wantBytes)
			}
		})
	}
}

func TestDownloadSizeLimit(t *testing.T) {
	t.Run("default is 100 GB", func(t *testing.T) {
		const want int64 = 100_000_000_000
		if defaultMaxDownloadFromURLBytes != want {
			t.Fatalf("defaultMaxDownloadFromURLBytes = %d, want %d", defaultMaxDownloadFromURLBytes, want)
		}
	})

	t.Run("oversized expected size is rejected", func(t *testing.T) {
		originalLimit := maxDownloadFromURLBytes
		maxDownloadFromURLBytes = 16
		t.Cleanup(func() {
			maxDownloadFromURLBytes = originalLimit
		})

		_, errLimit := downloadSizeLimit(DownloadIntegrity{ExpectedSize: 32})
		if !errors.Is(errLimit, ErrDownloadTooLarge) {
			t.Fatalf("downloadSizeLimit() error = %v, want %v", errLimit, ErrDownloadTooLarge)
		}
	})
}

type downloadRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn downloadRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func withDownloadTestHTTPClient(t *testing.T, client *http.Client) {
	t.Helper()

	oldClient := downloadHTTPClient
	oldValidate := validateDownloadTarget
	downloadHTTPClient = func() *http.Client {
		return client
	}
	validateDownloadTarget = func(string) error {
		return nil
	}
	t.Cleanup(func() {
		downloadHTTPClient = oldClient
		validateDownloadTarget = oldValidate
	})
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

func TestWriteFileFromReaderReplacesContentAndReturnsDigest(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}
	errInitial := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("old"), 0o600)
	if errInitial != nil {
		t.Fatalf("WriteFile initial error = %v", errInitial)
	}

	result, errWrite := n.WriteFileFromReader(dir, "notes.txt", strings.NewReader("new payload"), ProtectionPolicy{})
	if errWrite != nil {
		t.Fatalf("WriteFileFromReader error = %v", errWrite)
	}

	got, errRead := os.ReadFile(filepath.Join(dir, "notes.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile error = %v", errRead)
	}
	if string(got) != "new payload" {
		t.Fatalf("file content = %q, want %q", string(got), "new payload")
	}
	wantSHA := fmt.Sprintf("%x", sha256.Sum256([]byte("new payload")))
	if result.BytesWritten != int64(len("new payload")) {
		t.Fatalf("BytesWritten = %d, want %d", result.BytesWritten, len("new payload"))
	}
	if result.SHA256 != wantSHA {
		t.Fatalf("SHA256 = %q, want %q", result.SHA256, wantSHA)
	}

	matches, errGlob := filepath.Glob(filepath.Join(dir, ".xylona-write-*"))
	if errGlob != nil {
		t.Fatalf("Glob temp files error = %v", errGlob)
	}
	if len(matches) != 0 {
		t.Fatalf("leftover temp files = %v, want none", matches)
	}
}

func TestCopyFilesCopiesContentAndProtectsDestination(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	errWrite := os.WriteFile(filepath.Join(dir, "source.txt"), []byte("copy me"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile source error = %v", errWrite)
	}

	copied, errCopy := n.CopyFiles(t.Context(), dir, []CopyFileOperation{
		{SourceRelativePath: "source.txt", DestinationRelativePath: "nested/destination.txt"},
		{SourceRelativePath: "source.txt", DestinationRelativePath: "destination.txt"},
	}, ProtectionPolicy{})
	if errCopy != nil {
		t.Fatalf("CopyFiles error = %v", errCopy)
	}
	if len(copied) != 2 || copied[0] != "nested/destination.txt" || copied[1] != "destination.txt" {
		t.Fatalf("CopyFiles copied = %v, want nested and top-level destinations", copied)
	}
	data, errRead := os.ReadFile(filepath.Join(dir, "nested", "destination.txt"))
	if errRead != nil {
		t.Fatalf("ReadFile copied error = %v", errRead)
	}
	if string(data) != "copy me" {
		t.Fatalf("copied content = %q, want %q", string(data), "copy me")
	}
	topLevelData, errReadTopLevel := os.ReadFile(filepath.Join(dir, "destination.txt"))
	if errReadTopLevel != nil || string(topLevelData) != "copy me" {
		t.Fatalf("top-level copied content = %q, error = %v", topLevelData, errReadTopLevel)
	}

	errMkdir := os.MkdirAll(filepath.Join(dir, "source-dir", "nested"), 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll source directory error = %v", errMkdir)
	}
	errWriteNested := os.WriteFile(filepath.Join(dir, "source-dir", "nested", "file.txt"), []byte("nested"), 0o600)
	if errWriteNested != nil {
		t.Fatalf("WriteFile nested source error = %v", errWriteNested)
	}
	_, errCopyDirectory := n.CopyFiles(t.Context(), dir, []CopyFileOperation{
		{SourceRelativePath: "source-dir", DestinationRelativePath: "copied-dir"},
	}, ProtectionPolicy{})
	if errCopyDirectory != nil {
		t.Fatalf("CopyFiles directory error = %v", errCopyDirectory)
	}
	nestedData, errReadNested := os.ReadFile(filepath.Join(dir, "copied-dir", "nested", "file.txt"))
	if errReadNested != nil || string(nestedData) != "nested" {
		t.Fatalf("copied nested file = %q, error = %v", nestedData, errReadNested)
	}

	_, errProtected := n.CopyFiles(t.Context(), dir, []CopyFileOperation{
		{SourceRelativePath: "source.txt", DestinationRelativePath: "server.jar"},
	}, ProtectionPolicy{ServerExecutable: "server.jar"})
	if !errors.Is(errProtected, ErrProtectedPath) {
		t.Fatalf("CopyFiles protected err = %v, want %v", errProtected, ErrProtectedPath)
	}
}

func TestStatAndOpenFile(t *testing.T) {
	dir := t.TempDir()
	n := &Node{}

	errWrite := os.WriteFile(filepath.Join(dir, "archive.zip"), []byte("zip payload"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile error = %v", errWrite)
	}

	entry, errStat := n.StatFile(dir, "archive.zip")
	if errStat != nil {
		t.Fatalf("StatFile error = %v", errStat)
	}
	if entry.Name != "archive.zip" || entry.Size != int64(len("zip payload")) || entry.IsDirectory {
		t.Fatalf("StatFile entry = %+v, want archive.zip file", entry)
	}

	stream, errOpen := n.OpenFile(dir, "archive.zip")
	if errOpen != nil {
		t.Fatalf("OpenFile error = %v", errOpen)
	}
	data, errRead := io.ReadAll(stream)
	errClose := stream.Close()
	if errRead != nil {
		t.Fatalf("ReadAll stream error = %v", errRead)
	}
	if errClose != nil {
		t.Fatalf("Close stream error = %v", errClose)
	}
	if string(data) != "zip payload" {
		t.Fatalf("OpenFile stream = %q, want %q", string(data), "zip payload")
	}
}
