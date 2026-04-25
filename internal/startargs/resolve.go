package startargs

import (
	"sort"

	"github.com/ClintonCollins/Xylona/internal/placeholder"
)

// ResolveArgs applies patches and placeholder resolution to a start-args template.
func ResolveArgs(template []ArgBlock, patches []Patch, vars map[string]string) ([]string, []ResolvedBlock, error) {
	if len(template) == 0 {
		return []string{}, nil, nil
	}

	orderedTemplate := cloneBlocks(template)
	sort.SliceStable(orderedTemplate, func(i int, j int) bool {
		return orderedTemplate[i].Order < orderedTemplate[j].Order
	})

	templateByID := make(map[string]*ArgBlock, len(orderedTemplate))
	for i := range orderedTemplate {
		templateByID[orderedTemplate[i].ID] = &orderedTemplate[i]
	}

	editedIDs := make(map[string]struct{})
	removedIDs := make(map[string]struct{})
	originalTokens := map[string][]string{}
	addsByAnchor := map[string][]Patch{}

	for _, patch := range patches {
		switch patch.Op {
		case PatchOpEdit:
			block := templateByID[patch.ID]
			if block == nil {
				continue
			}
			if _, ok := originalTokens[patch.ID]; !ok {
				originalTokens[patch.ID] = cloneStrings(block.Tokens)
			}
			block.Tokens = cloneStrings(patch.Tokens)
			if patch.Label != "" {
				block.Label = patch.Label
			}
			editedIDs[patch.ID] = struct{}{}
		case PatchOpRemove:
			block := templateByID[patch.ID]
			if block == nil {
				continue
			}
			removedIDs[patch.ID] = struct{}{}
		case PatchOpAdd:
			anchorID := ""
			if patch.AfterID != nil {
				anchorID = *patch.AfterID
			}
			addsByAnchor[anchorID] = append(addsByAnchor[anchorID], patch)
		}
	}

	resolvedBlocks := make([]ResolvedBlock, 0, len(orderedTemplate)+len(patches))
	visitedAddIDs := map[string]struct{}{}

	var emitAnchoredAdds func(anchorID string)
	emitAnchoredAdds = func(anchorID string) {
		children := addsByAnchor[anchorID]
		for _, patch := range children {
			if _, ok := visitedAddIDs[patch.ID]; ok {
				continue
			}
			visitedAddIDs[patch.ID] = struct{}{}

			tokens := cloneStrings(patch.Tokens)
			resolvedTokens := placeholder.ResolveTokens(tokens, vars)
			resolvedBlocks = append(resolvedBlocks, ResolvedBlock{
				ID:             patch.ID,
				Ownership:      OwnershipEditable,
				Tokens:         tokens,
				ResolvedTokens: resolvedTokens,
				Label:          patch.Label,
				Provenance:     "added",
			})

			emitAnchoredAdds(patch.ID)
		}
	}

	emitAnchoredAdds("")

	for _, block := range orderedTemplate {
		if _, removed := removedIDs[block.ID]; removed {
			continue
		}

		tokens := cloneStrings(block.Tokens)
		resolvedTokens := placeholder.ResolveTokens(tokens, vars)
		resolvedBlock := ResolvedBlock{
			ID:             block.ID,
			Ownership:      block.Ownership,
			Tokens:         tokens,
			ResolvedTokens: resolvedTokens,
			Label:          block.Label,
			Provenance:     templateBlockProvenance(block, editedIDs),
		}
		original := originalTokens[block.ID]
		if len(original) > 0 {
			resolvedBlock.OriginalTokens = original
		}

		resolvedBlocks = append(resolvedBlocks, resolvedBlock)
		emitAnchoredAdds(block.ID)
	}

	args := make([]string, 0)
	for _, block := range resolvedBlocks {
		args = append(args, block.ResolvedTokens...)
	}

	return args, resolvedBlocks, nil
}

func cloneBlocks(blocks []ArgBlock) []ArgBlock {
	if len(blocks) == 0 {
		return nil
	}

	cloned := make([]ArgBlock, 0, len(blocks))
	for _, block := range blocks {
		copied := block
		copied.Tokens = cloneStrings(block.Tokens)
		cloned = append(cloned, copied)
	}

	return cloned
}

func cloneStrings(tokens []string) []string {
	if len(tokens) == 0 {
		return nil
	}

	cloned := make([]string, len(tokens))
	copy(cloned, tokens)
	return cloned
}

func templateBlockProvenance(block ArgBlock, editedIDs map[string]struct{}) string {
	if _, ok := editedIDs[block.ID]; ok {
		return "edited"
	}

	switch block.Ownership {
	case OwnershipSystem:
		return "system"
	case OwnershipLocked:
		return "locked"
	default:
		return "default"
	}
}
