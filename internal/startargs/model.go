package startargs

import (
	"encoding/json"
	"fmt"
)

// Ownership controls whether an argument block is system-managed or user-editable.
type Ownership string

// Ownership values describe who controls a block within the start-args editor.
const (
	OwnershipSystem   Ownership = "system"
	OwnershipLocked   Ownership = "locked"
	OwnershipEditable Ownership = "editable"
)

// ArgBlock is a single managed block of start-argument tokens.
type ArgBlock struct {
	ID            string    `json:"id"`
	Order         int       `json:"order"`
	Ownership     Ownership `json:"ownership"`
	Tokens        []string  `json:"tokens"`
	Label         string    `json:"label,omitempty"`
	ManagedSource string    `json:"managed_source,omitempty"`
}

// PatchOp identifies how a patch changes an argument block.
type PatchOp string

// PatchOp values describe the supported patch operations.
const (
	PatchOpEdit   PatchOp = "edit"
	PatchOpRemove PatchOp = "remove"
	PatchOpAdd    PatchOp = "add"
)

// Patch describes a change applied to a managed argument template.
type Patch struct {
	ID      string   `json:"id"`
	Op      PatchOp  `json:"op"`
	Tokens  []string `json:"tokens,omitempty"`
	Label   string   `json:"label,omitempty"`
	AfterID *string  `json:"afterId"`
}

// BlocklistEntry defines a forbidden argument pattern and its reason.
type BlocklistEntry struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
}

var validManagedSources = map[string]struct{}{
	"game_server.port":              {},
	"game_server.port_plus_1":       {},
	"game_server.port_plus_2":       {},
	"game_server.query_port":        {},
	"game_server.query_port_plus_1": {},
	"game_server.ip":                {},
	"game_server.max_memory_mb":     {},
	"game_server.max_players":       {},
	"game_server.server_name":       {},
	"server_executable":             {},
	"steam_gslt":                    {},
}

func isValidManagedSource(key string) bool {
	_, ok := validManagedSources[key]
	return ok
}

// ParseTemplate decodes a JSON start-argument template.
func ParseTemplate(jsonStr string) ([]ArgBlock, error) {
	if jsonStr == "" {
		return nil, nil
	}

	var blocks []ArgBlock
	errUnmarshal := json.Unmarshal([]byte(jsonStr), &blocks)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing template JSON: %w", errUnmarshal)
	}

	return blocks, nil
}

func parsePatches(jsonStr string) ([]Patch, error) {
	if jsonStr == "" {
		return nil, nil
	}

	var patches []Patch
	errUnmarshal := json.Unmarshal([]byte(jsonStr), &patches)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing patches JSON: %w", errUnmarshal)
	}

	return patches, nil
}

// ParseBlocklist decodes a JSON blocklist for start-argument validation.
func ParseBlocklist(jsonStr string) ([]BlocklistEntry, error) {
	if jsonStr == "" {
		return nil, nil
	}

	var entries []BlocklistEntry
	errUnmarshal := json.Unmarshal([]byte(jsonStr), &entries)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing blocklist JSON: %w", errUnmarshal)
	}

	return entries, nil
}
