//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/windows"
)

const (
	localSystemSID       = "S-1-5-18"
	administratorsSID    = "S-1-5-32-544"
	identityDACLPrefix   = "D:P"
	identityFileACEFlags = ""
	identityDirACEFlags  = "OICI"
)

func ensureIdentityDataDir(dataDir string) error {
	errMkdir := os.MkdirAll(dataDir, 0o700)
	if errMkdir != nil {
		return fmt.Errorf("create data directory: %w", errMkdir)
	}
	errProtect := protectIdentityPathSecurity(dataDir, true)
	if errProtect != nil {
		return fmt.Errorf("protect data directory: %w", errProtect)
	}
	return nil
}

func protectIdentityPathSecurity(path string, directory bool) error {
	sids, errSIDs := identityAllowedSIDs()
	if errSIDs != nil {
		return errSIDs
	}

	flags := identityFileACEFlags
	if directory {
		flags = identityDirACEFlags
	}
	descriptorParts := make([]string, len(sids)+1)
	descriptorParts[0] = identityDACLPrefix
	for i, sid := range sids {
		descriptorParts[i+1] = fmt.Sprintf("(A;%s;FA;;;%s)", flags, sid)
	}
	securityDescriptor, errDescriptor := windows.SecurityDescriptorFromString(strings.Join(descriptorParts, ""))
	if errDescriptor != nil {
		return fmt.Errorf("build protected DACL: %w", errDescriptor)
	}
	dacl, _, errDACL := securityDescriptor.DACL()
	if errDACL != nil {
		return fmt.Errorf("read protected DACL: %w", errDACL)
	}

	errSet := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		dacl,
		nil,
	)
	if errSet != nil {
		return fmt.Errorf("set protected DACL on %s: %w", path, errSet)
	}
	return nil
}

func identityAllowedSIDs() ([]string, error) {
	tokenUser, errUser := windows.GetCurrentProcessToken().GetTokenUser()
	if errUser != nil {
		return nil, fmt.Errorf("read current Windows user: %w", errUser)
	}
	currentUserSID := tokenUser.User.Sid.String()
	if currentUserSID == "" {
		return nil, errors.New("read current Windows user: empty SID")
	}

	sids := []string{localSystemSID, administratorsSID}
	if currentUserSID != localSystemSID && currentUserSID != administratorsSID {
		sids = append(sids, currentUserSID)
	}
	return sids, nil
}
