//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsIdentityPathSecurity(t *testing.T) {
	t.Run("new storage paths receive protected DACLs", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "node-data")
		errSave := saveIdentity(dataDir, windowsTestIdentity())
		if errSave != nil {
			t.Fatalf("saveIdentity: %v", errSave)
		}

		identityPath := filepath.Join(dataDir, identityFileName)
		for _, path := range []string{dataDir, identityPath} {
			if !windowsTestDACLProtected(t, path) {
				t.Errorf("%s DACL is not protected", path)
			}
		}
	})

	t.Run("existing permissions are repaired and stale fixed temp is ignored", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "node-data")
		errSave := saveIdentity(dataDir, windowsTestIdentity())
		if errSave != nil {
			t.Fatalf("initial saveIdentity: %v", errSave)
		}

		identityPath := filepath.Join(dataDir, identityFileName)
		setWindowsTestUnprotectedDACL(t, dataDir)
		setWindowsTestUnprotectedDACL(t, identityPath)

		loaded, errLoad := loadIdentity(dataDir)
		if errLoad != nil {
			t.Fatalf("loadIdentity with existing permissions: %v", errLoad)
		}
		if loaded.NodeID != "windows-node" {
			t.Fatalf("loadIdentity NodeID = %q, want windows-node", loaded.NodeID)
		}
		for _, path := range []string{dataDir, identityPath} {
			if !windowsTestDACLProtected(t, path) {
				t.Errorf("loadIdentity did not protect %s", path)
			}
		}

		tmpPath := identityPath + ".tmp"
		errWrite := os.WriteFile(tmpPath, []byte("stale temporary data"), 0o600)
		if errWrite != nil {
			t.Fatalf("write stale temp identity: %v", errWrite)
		}
		setWindowsTestUnprotectedDACL(t, tmpPath)

		errSave = saveIdentity(dataDir, windowsTestIdentity())
		if errSave != nil {
			t.Fatalf("saveIdentity with existing permissions: %v", errSave)
		}
		if !windowsTestDACLProtected(t, identityPath) {
			t.Fatal("rewritten identity DACL is not protected")
		}
		if !windowsTestDACLProtected(t, dataDir) {
			t.Fatal("saveIdentity did not protect the existing data directory DACL")
		}
		staleData, errRead := os.ReadFile(tmpPath)
		if errRead != nil {
			t.Fatalf("read stale temp identity: %v", errRead)
		}
		if string(staleData) != "stale temporary data" {
			t.Fatalf("stale temp identity was modified: %q", staleData)
		}
	})
}

func windowsTestDACLProtected(t *testing.T, path string) bool {
	t.Helper()

	descriptor, errDescriptor := windows.GetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
	)
	if errDescriptor != nil {
		t.Fatalf("GetNamedSecurityInfo: %v", errDescriptor)
	}
	control, _, errControl := descriptor.Control()
	if errControl != nil {
		t.Fatalf("SecurityDescriptor.Control: %v", errControl)
	}
	return control&windows.SE_DACL_PROTECTED != 0
}

func setWindowsTestUnprotectedDACL(t *testing.T, path string) {
	t.Helper()

	descriptor, errDescriptor := windows.SecurityDescriptorFromString("D:(A;;FA;;;WD)")
	if errDescriptor != nil {
		t.Fatalf("SecurityDescriptorFromString: %v", errDescriptor)
	}
	dacl, _, errDACL := descriptor.DACL()
	if errDACL != nil {
		t.Fatalf("DACL: %v", errDACL)
	}
	securityInformation := windows.SECURITY_INFORMATION(
		windows.DACL_SECURITY_INFORMATION | windows.UNPROTECTED_DACL_SECURITY_INFORMATION,
	)
	errSet := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		securityInformation,
		nil,
		nil,
		dacl,
		nil,
	)
	if errSet != nil {
		t.Fatalf("SetNamedSecurityInfo: %v", errSet)
	}
}

func windowsTestIdentity() *nodeIdentity {
	return &nodeIdentity{
		NodeID:        "windows-node",
		CertPEM:       "test-cert",
		KeyPEM:        "test-key",
		Fingerprint:   "test-fingerprint",
		ControllerURL: "https://controller.test",
		SharedSecret:  "test-shared-secret",
	}
}
