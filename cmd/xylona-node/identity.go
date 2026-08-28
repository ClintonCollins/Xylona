package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// identityFileName is the on-disk filename inside the data directory that
// holds the node's persistent pairing identity. A JSON file keeps the node
// binary free of any SQLite dependency; if the identity grows enough to need
// richer migrations, this file is the boundary to replace.
const identityFileName = "node-identity.json"

// nodeIdentity is the on-disk representation of everything the node needs to
// accept controller RPCs: its own cert material (so it can stand up an HTTPS
// listener), the controller's URL it reports back to, the node ID assigned by
// the controller during pairing, and the long-lived shared secret used as the
// bearer token on every RPC.
type nodeIdentity struct {
	NodeID        string `json:"node_id"`
	CertPEM       string `json:"cert_pem"`
	KeyPEM        string `json:"key_pem"`
	Fingerprint   string `json:"fingerprint"`
	ControllerURL string `json:"controller_url"`
	SharedSecret  string `json:"shared_secret"`
	SchemaVersion int    `json:"schema_version"`
}

// currentIdentitySchemaVersion identifies the on-disk layout. Bumping this
// value lets future migrations detect and upgrade older files.
const currentIdentitySchemaVersion = 1

// errIdentityMissing is returned by loadIdentity when no identity file exists.
// Callers use errors.Is to distinguish "node has not been paired yet" from
// disk/parse errors.
var errIdentityMissing = errors.New("node identity file not found")

// loadIdentity reads the node-identity.json file from dataDir. If no file
// exists the returned error wraps errIdentityMissing so the caller can decide
// to fall through to pairing instead.
func loadIdentity(dataDir string) (*nodeIdentity, error) {
	path := filepath.Join(dataDir, identityFileName)
	errDataDir := ensureIdentityDataDir(dataDir)
	if errDataDir != nil {
		return nil, fmt.Errorf("prepare data directory %s: %w", dataDir, errDataDir)
	}

	_, errStat := os.Stat(path)
	if errStat != nil {
		if errors.Is(errStat, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", errIdentityMissing, path)
		}
		return nil, fmt.Errorf("stat node identity: %w", errStat)
	}

	errProtect := protectIdentityPathSecurity(path, false)
	if errProtect != nil {
		return nil, fmt.Errorf("protect node identity %s: %w", path, errProtect)
	}

	data, errRead := os.ReadFile(path)
	if errRead != nil {
		if errors.Is(errRead, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", errIdentityMissing, path)
		}
		return nil, fmt.Errorf("read node identity: %w", errRead)
	}

	identity := &nodeIdentity{}
	errParse := json.Unmarshal(data, identity)
	if errParse != nil {
		return nil, fmt.Errorf("parse node identity %s: %w", path, errParse)
	}

	errValidate := identity.validate()
	if errValidate != nil {
		return nil, errValidate
	}
	return identity, nil
}

// saveIdentity writes id to dataDir atomically. The caller is responsible for
// providing a complete, valid identity; validate is called first so obvious
// mistakes (empty secret, etc.) never reach disk.
func saveIdentity(dataDir string, id *nodeIdentity) error {
	if id == nil {
		return errors.New("identity is nil")
	}

	if id.SchemaVersion == 0 {
		id.SchemaVersion = currentIdentitySchemaVersion
	}
	errValidate := id.validate()
	if errValidate != nil {
		return errValidate
	}

	data, errMarshal := json.MarshalIndent(id, "", "  ")
	if errMarshal != nil {
		return fmt.Errorf("marshal node identity: %w", errMarshal)
	}

	errDataDir := ensureIdentityDataDir(dataDir)
	if errDataDir != nil {
		return fmt.Errorf("prepare data directory %s: %w", dataDir, errDataDir)
	}

	path := filepath.Join(dataDir, identityFileName)
	tmpPath, errWrite := writeIdentityTemp(dataDir, data)
	if errWrite != nil {
		return fmt.Errorf("write node identity tmp: %w", errWrite)
	}
	errRename := os.Rename(tmpPath, path)
	if errRename != nil {
		return removeIdentityTemp(
			tmpPath,
			fmt.Errorf("rename node identity %s -> %s: %w", tmpPath, path, errRename),
		)
	}
	return nil
}

func writeIdentityTemp(dataDir string, data []byte) (string, error) {
	file, errOpen := os.CreateTemp(dataDir, identityFileName+".*.tmp")
	if errOpen != nil {
		return "", fmt.Errorf("create temporary identity in %s: %w", dataDir, errOpen)
	}
	path := file.Name()

	errProtect := protectIdentityPathSecurity(path, false)
	if errProtect != nil {
		return "", closeAndRemoveIdentityTemp(file, path, fmt.Errorf("protect %s: %w", path, errProtect))
	}

	_, errWrite := file.Write(data)
	if errWrite != nil {
		return "", closeAndRemoveIdentityTemp(file, path, fmt.Errorf("write %s: %w", path, errWrite))
	}

	errClose := file.Close()
	if errClose != nil {
		return "", removeIdentityTemp(path, fmt.Errorf("close %s: %w", path, errClose))
	}
	return path, nil
}

func closeAndRemoveIdentityTemp(file *os.File, path string, cause error) error {
	errClose := file.Close()
	if errClose != nil {
		cause = errors.Join(cause, fmt.Errorf("close %s: %w", path, errClose))
	}
	return removeIdentityTemp(path, cause)
}

func removeIdentityTemp(path string, cause error) error {
	errRemove := os.Remove(path)
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return errors.Join(cause, fmt.Errorf("remove node identity tmp %s: %w", path, errRemove))
	}
	return cause
}

// validate fails fast on missing required fields. It is intentionally strict:
// corrupt or partial identity files should not produce a running node.
func (id *nodeIdentity) validate() error {
	if strings.TrimSpace(id.NodeID) == "" {
		return errors.New("node identity missing node_id")
	}
	if strings.TrimSpace(id.CertPEM) == "" || strings.TrimSpace(id.KeyPEM) == "" {
		return errors.New("node identity missing certificate material")
	}
	if strings.TrimSpace(id.Fingerprint) == "" {
		return errors.New("node identity missing fingerprint")
	}
	if strings.TrimSpace(id.SharedSecret) == "" {
		return errors.New("node identity missing shared_secret")
	}
	return nil
}
