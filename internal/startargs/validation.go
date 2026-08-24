package startargs

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrLockedStartArgumentEdit reports a non-superuser attempt to change a locked patch.
var ErrLockedStartArgumentEdit = errors.New("locked start arguments may only be changed by a superuser")

// DefinitionConfig contains the platform templates and blocklist for a game definition.
type DefinitionConfig struct {
	LinuxTemplateJSON   string
	LinuxBaseCommand    string
	WindowsTemplateJSON string
	WindowsBaseCommand  string
	BlocklistJSON       string
}

// ServerConfig contains the template, patches, and values used for one game server.
type ServerConfig struct {
	TemplateJSON        string
	PatchesJSON         string
	ExistingPatchesJSON string
	BlocklistJSON       string
	Variables           map[string]string
	AllowLockedEdits    bool
}

// ValidateDefinition validates a game's complete structured start-argument configuration.
func ValidateDefinition(config DefinitionConfig) error {
	linuxBlocks, errLinux := validateDefinitionTemplate(config.LinuxTemplateJSON, config.LinuxBaseCommand)
	if errLinux != nil {
		return fmt.Errorf("linux start args: %w", errLinux)
	}

	windowsBlocks, errWindows := validateDefinitionTemplate(config.WindowsTemplateJSON, config.WindowsBaseCommand)
	if errWindows != nil {
		return fmt.Errorf("windows start args: %w", errWindows)
	}

	errShared := validateSharedTemplateIDs(linuxBlocks, windowsBlocks)
	if errShared != nil {
		return errShared
	}

	errBlocklist := validateBlocklist(config.BlocklistJSON, nil)
	if errBlocklist != nil {
		return fmt.Errorf("start arg blocklist: %w", errBlocklist)
	}

	return nil
}

// ValidateServerUpdate strictly validates a proposed server start-argument patch update.
func ValidateServerUpdate(config ServerConfig) error {
	template, errTemplate := ParseTemplate(config.TemplateJSON)
	if errTemplate != nil {
		return fmt.Errorf("parse game server start args template: %w", errTemplate)
	}

	patchesJSON := strings.TrimSpace(config.PatchesJSON)
	if len(template) == 0 && (patchesJSON == "" || patchesJSON == "[]") {
		return nil
	}
	if len(template) == 0 {
		return errors.New("this game does not have a start args template for the server platform")
	}

	patches, errPatches := parsePatches(patchesJSON)
	if errPatches != nil {
		return fmt.Errorf("parse game server start args patches: %w", errPatches)
	}

	allowLockedEdits := config.AllowLockedEdits
	if !allowLockedEdits {
		existingPatches, errExisting := parsePatches(config.ExistingPatchesJSON)
		if errExisting != nil {
			return fmt.Errorf("parse existing game server start args patches: %w", errExisting)
		}

		errLocked := validateLockedPatchesUnchanged(template, existingPatches, patches)
		if errLocked != nil {
			return errLocked
		}
		allowLockedEdits = true
	}

	errStructure := validateServerPatches(template, patches, allowLockedEdits)
	if errStructure != nil {
		return errStructure
	}

	args := resolveArgs(template, patches, config.Variables)
	errBlocklist := validateBlocklist(config.BlocklistJSON, args)
	if errBlocklist != nil {
		return errBlocklist
	}

	return nil
}

// ResolveServer tolerantly applies persisted patches and validates the final arguments.
func ResolveServer(config ServerConfig) ([]string, error) {
	template, errTemplate := ParseTemplate(config.TemplateJSON)
	if errTemplate != nil {
		return nil, fmt.Errorf("parse start args template: %w", errTemplate)
	}

	patches, errPatches := parsePatches(config.PatchesJSON)
	if errPatches != nil {
		return nil, fmt.Errorf("parse start arg patches: %w", errPatches)
	}

	args := resolveArgs(template, patches, config.Variables)
	errBlocklist := validateBlocklist(config.BlocklistJSON, args)
	if errBlocklist != nil {
		return nil, errBlocklist
	}

	return args, nil
}

func validateDefinitionTemplate(templateJSON string, baseCommand string) ([]ArgBlock, error) {
	blocks, errTemplate := ParseTemplate(templateJSON)
	if errTemplate != nil {
		return nil, fmt.Errorf("parse start args template: %w", errTemplate)
	}

	errBlocks := validateTemplateBlocks(blocks)
	if errBlocks != nil {
		return nil, errBlocks
	}

	if len(blocks) > 0 && strings.TrimSpace(baseCommand) == "" {
		return nil, errors.New("base command is required when a start args template is configured")
	}

	return blocks, nil
}

func validateTemplateBlocks(blocks []ArgBlock) error {
	seenIDs := make(map[string]struct{}, len(blocks))
	for _, block := range blocks {
		blockID := strings.TrimSpace(block.ID)
		if blockID == "" {
			return errors.New("template block id is required")
		}

		_, exists := seenIDs[blockID]
		if exists {
			return fmt.Errorf("duplicate template block id %q", blockID)
		}
		seenIDs[blockID] = struct{}{}

		if len(block.Tokens) == 0 {
			return fmt.Errorf("template block %q must contain at least one token", blockID)
		}

		switch block.Ownership {
		case OwnershipSystem, OwnershipLocked, OwnershipEditable:
		default:
			return fmt.Errorf("template block %q has invalid ownership %q", blockID, block.Ownership)
		}

		if block.ManagedSource != "" && !isValidManagedSource(block.ManagedSource) {
			return fmt.Errorf("template block %q has invalid managed source %q", blockID, block.ManagedSource)
		}
	}

	return nil
}

func validateSharedTemplateIDs(primary []ArgBlock, secondary []ArgBlock) error {
	secondaryByID := make(map[string]ArgBlock, len(secondary))
	for _, block := range secondary {
		secondaryByID[block.ID] = block
	}

	for _, block := range primary {
		other, exists := secondaryByID[block.ID]
		if !exists {
			continue
		}
		if block.Ownership != other.Ownership {
			return fmt.Errorf("shared template block %q must use the same ownership on both platforms", block.ID)
		}
		if block.Label != other.Label {
			return fmt.Errorf("shared template block %q must use the same label on both platforms", block.ID)
		}
		if block.ManagedSource != other.ManagedSource {
			return fmt.Errorf("shared template block %q must use the same managed source on both platforms", block.ID)
		}
		if len(block.Tokens) != len(other.Tokens) {
			return fmt.Errorf("shared template block %q must use the same token arity on both platforms", block.ID)
		}
	}

	return nil
}

func validateServerPatches(template []ArgBlock, patches []Patch, allowLockedEdits bool) error {
	templateByID := make(map[string]ArgBlock, len(template))
	referenceableIDs := make(map[string]struct{}, len(template))
	seenPatchIDs := make(map[string]struct{}, len(patches))
	for _, block := range template {
		templateByID[block.ID] = block
		referenceableIDs[block.ID] = struct{}{}
	}

	for _, patch := range patches {
		patchID := strings.TrimSpace(patch.ID)
		if patchID == "" {
			return errors.New("patch id is required")
		}
		_, duplicate := seenPatchIDs[patchID]
		if duplicate {
			return fmt.Errorf("duplicate patch id %q", patchID)
		}
		seenPatchIDs[patchID] = struct{}{}

		switch patch.Op {
		case PatchOpAdd:
			if len(patch.Tokens) == 0 {
				return fmt.Errorf("add patch %q must contain at least one token", patchID)
			}
			_, exists := templateByID[patchID]
			if exists {
				return fmt.Errorf("add patch %q collides with an existing template block", patchID)
			}
			if patch.AfterID != nil {
				afterID := strings.TrimSpace(*patch.AfterID)
				if afterID == "" {
					return fmt.Errorf("add patch %q has an empty afterId", patchID)
				}
				_, exists = referenceableIDs[afterID]
				if !exists {
					return fmt.Errorf("add patch %q references unknown afterId %q", patchID, afterID)
				}
			}
			referenceableIDs[patchID] = struct{}{}
		case PatchOpEdit:
			if len(patch.Tokens) == 0 {
				return fmt.Errorf("edit patch %q must contain at least one token", patchID)
			}
			block, exists := templateByID[patchID]
			if exists && !canPatchBlock(block, allowLockedEdits) {
				return fmt.Errorf("patch %q targets a non-editable template block", patchID)
			}
		case PatchOpRemove:
			block, exists := templateByID[patchID]
			if exists && !canPatchBlock(block, allowLockedEdits) {
				return fmt.Errorf("patch %q targets a non-editable template block", patchID)
			}
		default:
			return fmt.Errorf("patch %q has invalid operation %q", patchID, patch.Op)
		}
	}

	return nil
}

func canPatchBlock(block ArgBlock, allowLockedEdits bool) bool {
	return block.Ownership == OwnershipEditable ||
		(allowLockedEdits && block.Ownership == OwnershipLocked)
}

func validateLockedPatchesUnchanged(template []ArgBlock, existing []Patch, updated []Patch) error {
	lockedIDs := make(map[string]struct{})
	for _, block := range template {
		if block.Ownership == OwnershipLocked {
			lockedIDs[block.ID] = struct{}{}
		}
	}

	existingLocked := patchesByID(existing, lockedIDs)
	updatedLocked := patchesByID(updated, lockedIDs)
	if len(existingLocked) != len(updatedLocked) {
		return ErrLockedStartArgumentEdit
	}
	for id, existingPatch := range existingLocked {
		updatedPatch, exists := updatedLocked[id]
		if !exists || !equalPatch(existingPatch, updatedPatch) {
			return ErrLockedStartArgumentEdit
		}
	}

	return nil
}

func patchesByID(patches []Patch, ids map[string]struct{}) map[string]Patch {
	filtered := make(map[string]Patch)
	for _, patch := range patches {
		if _, exists := ids[patch.ID]; exists {
			filtered[patch.ID] = patch
		}
	}
	return filtered
}

func equalPatch(left Patch, right Patch) bool {
	return left.ID == right.ID &&
		left.Op == right.Op &&
		slices.Equal(left.Tokens, right.Tokens) &&
		left.Label == right.Label &&
		equalOptionalString(left.AfterID, right.AfterID)
}

func equalOptionalString(left *string, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validateBlocklist(blocklistJSON string, args []string) error {
	entries, errParse := ParseBlocklist(blocklistJSON)
	if errParse != nil {
		return fmt.Errorf("parse start arg blocklist: %w", errParse)
	}

	blocklist, errCompile := compileBlocklist(entries)
	if errCompile != nil {
		return fmt.Errorf("compile start arg blocklist: %w", errCompile)
	}

	violation := blocklist.validate(args)
	if violation != nil {
		return fmt.Errorf("blocked start argument %q: %s", violation.token, violation.reason)
	}

	return nil
}
