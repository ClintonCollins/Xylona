package cfgparse

import (
	"bytes"
	"fmt"
	"strings"
)

// INIParser implements FlatConfigParser for the INI format.
type INIParser struct{}

func init() {
	RegisterFlat(&INIParser{})
}

// Format returns the format identifier for INI files.
func (p *INIParser) Format() string {
	return "ini"
}

// Parse parses INI-formatted data into a slice of ConfigEntry values.
func (p *INIParser) Parse(data []byte) ([]ConfigEntry, error) {
	lines := strings.Split(string(data), "\n")
	var entries []ConfigEntry
	var currentSection string
	var pendingComment strings.Builder

	// Track duplicate keys per section for Index assignment.
	type sectionKey struct {
		section string
		key     string
	}
	keyCounts := map[sectionKey]int{}

	for lineNum, line := range lines {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		// Comment line.
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
			if pendingComment.Len() > 0 {
				pendingComment.WriteByte('\n')
			}
			pendingComment.WriteString(trimmed)
			continue
		}

		// Section header.
		if strings.HasPrefix(trimmed, "[") {
			closeIdx := strings.Index(trimmed, "]")
			if closeIdx == -1 {
				return nil, fmt.Errorf("ini: unclosed section header at line %d", lineNum+1)
			}
			currentSection = trimmed[1:closeIdx]
			pendingComment.Reset()
			continue
		}

		// Key=value pair.
		key, value, found := strings.Cut(trimmed, "=")
		if !found {
			return nil, fmt.Errorf("ini: invalid line %d: %q", lineNum+1, trimmed)
		}

		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		sk := sectionKey{section: currentSection, key: key}
		idx := keyCounts[sk]
		keyCounts[sk]++

		entry := ConfigEntry{
			Section: currentSection,
			Key:     key,
			Value:   value,
			Index:   idx,
		}
		if pendingComment.Len() > 0 {
			entry.Comment = pendingComment.String()
			pendingComment.Reset()
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// Write serializes a slice of ConfigEntry values into INI-formatted bytes.
func (p *INIParser) Write(entries []ConfigEntry) ([]byte, error) {
	var buf bytes.Buffer

	// Collect unique sections in order of first appearance.
	var sections []string
	seen := map[string]bool{}
	for _, e := range entries {
		if !seen[e.Section] {
			seen[e.Section] = true
			sections = append(sections, e.Section)
		}
	}

	for i, section := range sections {
		if i > 0 {
			buf.WriteByte('\n')
		}

		if section != "" {
			fmt.Fprintf(&buf, "[%s]\n", section)
		}

		for _, e := range entries {
			if e.Section != section {
				continue
			}
			if e.Comment != "" {
				for cl := range strings.SplitSeq(e.Comment, "\n") {
					cl = strings.TrimSpace(cl)
					// Normalize comment prefix to "# ".
					stripped := strings.TrimLeft(cl, "#; ")
					fmt.Fprintf(&buf, "# %s\n", stripped)
				}
			}
			fmt.Fprintf(&buf, "%s=%s\n", e.Key, e.Value)
		}
	}

	return buf.Bytes(), nil
}
