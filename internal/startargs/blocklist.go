// Package startargs parses, resolves, and validates managed server start arguments.
package startargs

import (
	"fmt"
	"regexp"
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

func (bl *compiledBlocklist) validate(tokens []string) *blocklistViolation {
	if bl == nil {
		return nil
	}

	for _, token := range tokens {
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
