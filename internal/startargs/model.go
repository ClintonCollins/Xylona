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

// ResolvedBlock is an argument block after placeholder resolution and patching.
type ResolvedBlock struct {
	ID             string    `json:"id"`
	Ownership      Ownership `json:"ownership"`
	Tokens         []string  `json:"tokens"`
	ResolvedTokens []string  `json:"resolved_tokens"`
	Label          string    `json:"label,omitempty"`
	Provenance     string    `json:"provenance"`
	OriginalTokens []string  `json:"original_tokens,omitempty"`
}

// ValidManagedSources lists managed-source keys supported by the start-args editor.
var ValidManagedSources = map[string]struct{}{
	"game_server.port":          {},
	"game_server.query_port":    {},
	"game_server.ip":            {},
	"game_server.max_memory_mb": {},
	"server_executable":         {},
}

// IsValidManagedSource reports whether the key is supported for managed substitution.
func IsValidManagedSource(key string) bool {
	_, ok := ValidManagedSources[key]
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

// ParsePatches decodes a JSON list of start-argument patches.
func ParsePatches(jsonStr string) ([]Patch, error) {
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
