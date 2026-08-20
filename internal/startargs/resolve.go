package startargs

import (
	"cmp"
	"slices"

	"github.com/ClintonCollins/Xylona/internal/placeholder"
)

func resolveArgs(template []ArgBlock, patches []Patch, vars map[string]string) []string {
	if len(template) == 0 {
		return []string{}
	}

	orderedTemplate := slices.Clone(template)
	slices.SortStableFunc(orderedTemplate, func(a ArgBlock, b ArgBlock) int {
		return cmp.Compare(a.Order, b.Order)
	})

	templateByID := make(map[string]*ArgBlock, len(orderedTemplate))
	for i := range orderedTemplate {
		templateByID[orderedTemplate[i].ID] = &orderedTemplate[i]
	}

	removedIDs := make(map[string]struct{})
	addsByAnchor := map[string][]Patch{}

	for _, patch := range patches {
		switch patch.Op {
		case PatchOpEdit:
			block := templateByID[patch.ID]
			if block == nil {
				continue
			}
			block.Tokens = patch.Tokens
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

	args := make([]string, 0)
	visitedAddIDs := map[string]struct{}{}

	var emitAnchoredAdds func(anchorID string)
	emitAnchoredAdds = func(anchorID string) {
		children := addsByAnchor[anchorID]
		for _, patch := range children {
			if _, ok := visitedAddIDs[patch.ID]; ok {
				continue
			}
			visitedAddIDs[patch.ID] = struct{}{}

			args = append(args, placeholder.ResolveTokens(patch.Tokens, vars)...)

			emitAnchoredAdds(patch.ID)
		}
	}

	emitAnchoredAdds("")

	for _, block := range orderedTemplate {
		if _, removed := removedIDs[block.ID]; removed {
			continue
		}

		args = append(args, placeholder.ResolveTokens(block.Tokens, vars)...)
		emitAnchoredAdds(block.ID)
	}

	return args
}
