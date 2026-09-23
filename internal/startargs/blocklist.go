// Package startargs parses, resolves, and validates managed server start arguments.
package startargs

import (
	"fmt"
	"regexp"
	"strings"
)

type compiledBlocklist struct {
	entries []compiledEntry
}

type compiledEntry struct {
	regex  *regexp.Regexp
	reason string
}

type blocklistViolation struct {
	token  string
	reason string
}

func compileBlocklist(entries []BlocklistEntry) (*compiledBlocklist, error) {
	compiled := &compiledBlocklist{
		entries: make([]compiledEntry, 0, len(entries)),
	}

	for _, entry := range entries {
		regex, errCompile := regexp.Compile(entry.Pattern)
		if errCompile != nil {
			return nil, fmt.Errorf("compiling blocklist pattern %q: %w", entry.Pattern, errCompile)
		}

		compiled.entries = append(compiled.entries, compiledEntry{
			regex:  regex,
			reason: entry.Reason,
		})
	}

	return compiled, nil
}

// validate returns the first token that matches the blocklist, skipping
// trusted tokens the game definition spells out itself.
func (bl *compiledBlocklist) validate(tokens []string, trusted map[string]struct{}) *blocklistViolation {
	if bl == nil {
		return nil
	}

	for _, token := range tokens {
		if _, ok := trusted[token]; ok {
			continue
		}
		for _, entry := range bl.entries {
			if !entry.regex.MatchString(token) {
				continue
			}

			return &blocklistViolation{
				token:  token,
				reason: entry.reason,
			}
		}
	}

	return nil
}

// definitionTokens returns the template tokens a game definition writes out
// literally. The blocklist guards against operator overrides, so a
// definition's own argument (such as Minecraft's locked Log4j fix) never blocks
// its start. Tokens that take a placeholder value stay subject to the blocklist.
func definitionTokens(template []ArgBlock) map[string]struct{} {
	tokens := make(map[string]struct{})
	for _, block := range template {
		for _, token := range block.Tokens {
			if !strings.Contains(token, "{{") && !strings.Contains(token, "%GAMESERVER_") {
				tokens[token] = struct{}{}
			}
		}
	}
	return tokens
}
