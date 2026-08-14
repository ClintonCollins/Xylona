package startargs

import (
	"slices"
	"strings"
	"unicode"
)

// FindSimilarArg finds an existing block that likely matches a new argument block.
func FindSimilarArg(newTokens []string, existingBlocks []ArgBlock) *ArgBlock {
	if len(newTokens) == 0 || len(existingBlocks) == 0 {
		return nil
	}

	newPrefix := flagPrefix(newTokens[0])
	if newPrefix == "" {
		for i := range existingBlocks {
			if slices.Equal(newTokens, existingBlocks[i].Tokens) {
				return &existingBlocks[i]
			}
		}
		return nil
	}

	for i := range existingBlocks {
		if len(existingBlocks[i].Tokens) == 0 {
			continue
		}

		existingPrefix := flagPrefix(existingBlocks[i].Tokens[0])
		if existingPrefix == "" {
			continue
		}
		if existingPrefix == newPrefix {
			return &existingBlocks[i]
		}
	}

	return nil
}

func flagPrefix(token string) string {
	if token == "" {
		return ""
	}
	if !strings.HasPrefix(token, "-") {
		return ""
	}

	for i, r := range token {
		if unicode.IsDigit(r) || r == '=' {
			return token[:i]
		}
	}

	return token
}
