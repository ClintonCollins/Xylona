package gameintegrations

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"
)

const (
	// InternalUpdateBackupDirectory contains rollback data retained by an
	// internal game updater until the controller finalizes the update.
	InternalUpdateBackupDirectory = ".update-backup/.internal-update"
	// InternalUpdateManifestPath is the node-relative path to the durable
	// internal update transaction manifest.
	InternalUpdateManifestPath = InternalUpdateBackupDirectory + "/manifest.json"
	// InternalUpdateCommittedPath marks a transaction whose staged payload was
	// fully applied but whose rollback data is still awaiting finalization.
	InternalUpdateCommittedPath = InternalUpdateBackupDirectory + "/committed"
	// InternalUpdateFilesDirectory contains the original paths retained for
	// rollback.
	InternalUpdateFilesDirectory = InternalUpdateBackupDirectory + "/files"

	updateTransactionManifestVersion = 1
	maxUpdateTransactionEntries      = 100_000
)

// UpdateTransactionEntry records one live path affected by an internal game
// update. Directory entries represent whole-directory replacements.
type UpdateTransactionEntry struct {
	Path      string `json:"path"`
	Existed   bool   `json:"existed"`
	Directory bool   `json:"directory,omitempty"`
}

// UpdateTransactionManifest is the durable controller/node contract for an
// internal game update rollback.
type UpdateTransactionManifest struct {
	Version int                      `json:"version"`
	GameID  string                   `json:"game_id"`
	Entries []UpdateTransactionEntry `json:"entries"`
}

// NewUpdateTransactionManifest creates a manifest in its canonical form.
func NewUpdateTransactionManifest(gameID string, entries []UpdateTransactionEntry) UpdateTransactionManifest {
	manifest := UpdateTransactionManifest{
		Version: updateTransactionManifestVersion,
		GameID:  strings.TrimSpace(gameID),
		Entries: append([]UpdateTransactionEntry(nil), entries...),
	}
	slices.SortFunc(manifest.Entries, func(left UpdateTransactionEntry, right UpdateTransactionEntry) int {
		return strings.Compare(left.Path, right.Path)
	})
	return manifest
}

// ValidateUpdateTransactionManifest rejects malformed or unsafe rollback
// manifests before controller or node code performs filesystem operations.
func ValidateUpdateTransactionManifest(manifest UpdateTransactionManifest) error {
	if manifest.Version != updateTransactionManifestVersion {
		return fmt.Errorf("unsupported internal update manifest version %d", manifest.Version)
	}
	if strings.TrimSpace(manifest.GameID) == "" {
		return errors.New("internal update manifest game ID is required")
	}
	if len(manifest.Entries) == 0 {
		return errors.New("internal update manifest has no entries")
	}
	if len(manifest.Entries) > maxUpdateTransactionEntries {
		return fmt.Errorf("internal update manifest exceeds %d entries", maxUpdateTransactionEntries)
	}

	seen := make(map[string]struct{}, len(manifest.Entries))
	for _, entry := range manifest.Entries {
		cleaned := path.Clean(strings.TrimSpace(entry.Path))
		if cleaned == "." || !fs.ValidPath(cleaned) || cleaned != entry.Path {
			return fmt.Errorf("internal update manifest contains unsafe path %q", entry.Path)
		}
		if cleaned == ".update-backup" || strings.HasPrefix(cleaned, ".update-backup/") ||
			strings.HasPrefix(cleaned, ".xylona-") {
			return fmt.Errorf("internal update manifest contains reserved path %q", entry.Path)
		}
		_, duplicate := seen[cleaned]
		if duplicate {
			return fmt.Errorf("internal update manifest contains duplicate path %q", entry.Path)
		}
		seen[cleaned] = struct{}{}
	}
	return nil
}
