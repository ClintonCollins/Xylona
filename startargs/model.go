package startargs

import (
	"encoding/json"
	"fmt"
)

type Ownership string

const (
	OwnershipSystem   Ownership = "system"
	OwnershipLocked   Ownership = "locked"
	OwnershipEditable Ownership = "editable"
)

type ArgBlock struct {
	ID            string    `json:"id"`
	Order         int       `json:"order"`
	Ownership     Ownership `json:"ownership"`
	Tokens        []string  `json:"tokens"`
	Label         string    `json:"label,omitempty"`
	ManagedSource string    `json:"managed_source,omitempty"`
}

type PatchOp string

const (
	PatchOpEdit   PatchOp = "edit"
	PatchOpRemove PatchOp = "remove"
	PatchOpAdd    PatchOp = "add"
)

type Patch struct {
	ID      string   `json:"id"`
	Op      PatchOp  `json:"op"`
	Tokens  []string `json:"tokens,omitempty"`
	Label   string   `json:"label,omitempty"`
	AfterID *string  `json:"afterId"`
}

type BlocklistEntry struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
}

type ResolvedBlock struct {
	ID             string    `json:"id"`
	Ownership      Ownership `json:"ownership"`
	Tokens         []string  `json:"tokens"`
	ResolvedTokens []string  `json:"resolved_tokens"`
	Label          string    `json:"label,omitempty"`
	Provenance     string    `json:"provenance"`
	OriginalTokens []string  `json:"original_tokens,omitempty"`
}

var ValidManagedSources = map[string]struct{}{
	"game_server.port":          {},
	"game_server.query_port":    {},
	"game_server.ip":            {},
	"game_server.max_memory_mb": {},
	"server_executable":         {},
}

func IsValidManagedSource(key string) bool {
	_, ok := ValidManagedSources[key]
	return ok
}

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
