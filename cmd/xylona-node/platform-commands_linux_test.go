//go:build linux

package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/appservice"
)

func TestPrepareNodeServiceDataPath(t *testing.T) {
	t.Run("rejects inaccessible existing ancestor", func(t *testing.T) {
		// A direct child of the shared temp directory keeps all ancestors
		// traversable by the synthetic account used below.
		baseDirectory, errTemp := os.MkdirTemp(os.TempDir(), "xylona-node-access-*") //nolint:usetesting // Direct temp child is required for synthetic-user traversal.
		if errTemp != nil {
			t.Fatalf("create direct temporary directory: %v", errTemp)
		}
		t.Cleanup(func() {
			errRemove := os.RemoveAll(baseDirectory)
			if errRemove != nil {
				t.Errorf("remove direct temporary directory: %v", errRemove)
			}
		})
		errBaseMode := os.Chmod(baseDirectory, 0o711) //nolint:gosec // Test fixture needs other-user traversal.
		if errBaseMode != nil {
			t.Fatalf("make base directory traversable: %v", errBaseMode)
		}
		privateParent := filepath.Join(baseDirectory, "private")
		errPrivate := os.Mkdir(privateParent, 0o700)
		if errPrivate != nil {
			t.Fatalf("create private parent: %v", errPrivate)
		}
		dataDirectory := filepath.Join(privateParent, "node")
		errData := os.Mkdir(dataDirectory, 0o777) //nolint:gosec // Test fixture isolates ancestor traversal from leaf access.
		if errData != nil {
			t.Fatalf("create node data directory: %v", errData)
		}
		errDataMode := os.Chmod(dataDirectory, 0o777) //nolint:gosec // Override umask so only the ancestor blocks the synthetic account.
		if errDataMode != nil {
			t.Fatalf("make node data directory accessible: %v", errDataMode)
		}

		preparation := &nodeServicePreparation{absoluteDataDir: dataDirectory}
		account := appservice.Account{
			Username:       "service-test",
			UID:            "424242",
			PrimaryGroup:   "service-test",
			PrimaryGroupID: "424242",
			GroupIDs:       []string{"424242"},
		}
		_, errPrepare := prepareNodeServiceDataPath(preparation, account)
		if errPrepare == nil || !strings.Contains(errPrepare.Error(), "cannot traverse an ancestor") {
			t.Fatalf("prepareNodeServiceDataPath() error = %v, want inaccessible-ancestor rejection", errPrepare)
		}
	})

	t.Run("tracks and assigns every newly created directory", func(t *testing.T) {
		account := currentLinuxTestAccount(t)
		dataDirectory := filepath.Join(t.TempDir(), "nested", "node")
		preparation := &nodeServicePreparation{absoluteDataDir: dataDirectory}

		createdDataDirectories, errPrepare := prepareNodeServiceDataPath(preparation, account)
		if errPrepare != nil {
			t.Fatalf("prepareNodeServiceDataPath() error = %v", errPrepare)
		}
		if len(createdDataDirectories) != 2 {
			t.Fatalf(
				"created directories = %q, want nested parent and node leaf",
				createdDataDirectories,
			)
		}
		errAssign := assignNewNodeServicePaths(preparation, account, createdDataDirectories)
		if errAssign != nil {
			t.Fatalf("assignNewNodeServicePaths() error = %v", errAssign)
		}
		for _, directoryPath := range createdDataDirectories {
			info, errInfo := os.Stat(directoryPath)
			if errInfo != nil {
				t.Fatalf("stat assigned directory %s: %v", directoryPath, errInfo)
			}
			stat, statOK := info.Sys().(*syscall.Stat_t)
			if !statOK {
				t.Fatalf("directory %s has no Linux stat metadata", directoryPath)
			}
			if strconv.FormatUint(uint64(stat.Uid), 10) != account.UID {
				t.Fatalf("directory %s owner = %d, want %s", directoryPath, stat.Uid, account.UID)
			}
		}
	})

	t.Run("ownership failure preserves identity with recovery guidance", func(t *testing.T) {
		dataDirectory := t.TempDir()
		identityPath := filepath.Join(dataDirectory, identityFileName)
		errWrite := os.WriteFile(identityPath, []byte("paired-identity"), 0o600)
		if errWrite != nil {
			t.Fatalf("write paired identity: %v", errWrite)
		}
		preparation := &nodeServicePreparation{
			absoluteDataDir: dataDirectory,
			identityCreated: true,
		}
		account := appservice.Account{
			Username:       "service-test",
			UID:            "not-a-user-id",
			PrimaryGroup:   "service-test",
			PrimaryGroupID: "424242",
		}

		errAssign := assignNewNodeServicePaths(preparation, account, nil)
		if errAssign == nil ||
			!strings.Contains(errAssign.Error(), identityPath) ||
			!strings.Contains(errAssign.Error(), "data directory "+dataDirectory) ||
			!strings.Contains(errAssign.Error(), "installer-created ancestors") ||
			!strings.Contains(errAssign.Error(), "then rerun service install") {
			t.Fatalf(
				"assignNewNodeServicePaths() error = %v, want exact-path recovery guidance",
				errAssign,
			)
		}
		_, errStat := os.Stat(identityPath)
		if errStat != nil {
			t.Fatalf("paired identity was not preserved: %v", errStat)
		}
	})
}

func currentLinuxTestAccount(t *testing.T) appservice.Account {
	t.Helper()

	account, _, errAccount := appservice.ResolveLinuxAccount("")
	if errAccount != nil {
		t.Fatalf("resolve current Linux test account: %v", errAccount)
	}
	return account
}
