// Package startargs parses, resolves, and validates managed server start arguments.
package startargs

import (
	"fmt"
	"regexp"
)

// CompiledBlocklist is a regex-ready blocklist for validating argument tokens.
type CompiledBlocklist struct {
	entries []compiledEntry
}

type compiledEntry struct {
	pattern string
	regex   *regexp.Regexp
	reason  string
}

// BlocklistViolation describes the first blocked token encountered during validation.
type BlocklistViolation struct {
	Token   string
	Pattern string
	Reason  string
}

// CompileBlocklist compiles blocklist entries into regular expressions.
func CompileBlocklist(entries []BlocklistEntry) (*CompiledBlocklist, error) {
	compiled := &CompiledBlocklist{
		entries: make([]compiledEntry, 0, len(entries)),
	}

	for _, entry := range entries {
		regex, errCompile := regexp.Compile(entry.Pattern)
		if errCompile != nil {
			return nil, fmt.Errorf("compiling blocklist pattern %q: %w", entry.Pattern, errCompile)
		}

		compiled.entries = append(compiled.entries, compiledEntry{
			pattern: entry.Pattern,
			regex:   regex,
			reason:  entry.Reason,
		})
	}

	return compiled, nil
}

// Validate returns the first token that matches the compiled blocklist.
func (bl *CompiledBlocklist) Validate(tokens []string) *BlocklistViolation {
	if bl == nil {
		return nil
	}

	for _, token := range tokens {
		for _, entry := range bl.entries {
			if !entry.regex.MatchString(token) {
				continue
			}

			return &BlocklistViolation{
				Token:   token,
				Pattern: entry.pattern,
				Reason:  entry.reason,
			}
		}
	}

	return nil
}
