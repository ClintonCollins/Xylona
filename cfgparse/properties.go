package cfgparse

import (
	"bytes"
	"strings"
)

// PropertiesParser parses Java .properties files.
type PropertiesParser struct{}

func init() {
	RegisterFlat(&PropertiesParser{})
}

// Format returns the format identifier for this parser.
func (p *PropertiesParser) Format() string {
	return "properties"
}

// Parse reads a .properties file and returns config entries.
func (p *PropertiesParser) Parse(data []byte) ([]ConfigEntry, error) {
	lines := strings.Split(string(data), "\n")
	var entries []ConfigEntry
	var pendingComment strings.Builder
	keyCounts := map[string]int{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "!") {
			commentText := strings.TrimSpace(trimmed[1:])
			if pendingComment.Len() > 0 {
				pendingComment.WriteByte('\n')
			}
			pendingComment.WriteString(commentText)
			continue
		}

		key, value := splitKeyValue(trimmed)
		idx := keyCounts[key]
		keyCounts[key]++

		entry := ConfigEntry{
			Key:   key,
			Value: value,
			Index: idx,
		}
		if pendingComment.Len() > 0 {
			entry.Comment = pendingComment.String()
			pendingComment.Reset()
		}
		entries = append(entries, entry)
	}

	// Trailing comment with no following entry.
	if pendingComment.Len() > 0 {
		entries = append(entries, ConfigEntry{
			Comment: pendingComment.String(),
		})
	}

	return entries, nil
}

// Write serializes config entries back to .properties format.
func (p *PropertiesParser) Write(entries []ConfigEntry) ([]byte, error) {
	var buf bytes.Buffer

	for _, entry := range entries {
		if entry.Comment != "" {
			for cl := range strings.SplitSeq(entry.Comment, "\n") {
				buf.WriteString("# ")
				buf.WriteString(cl)
				buf.WriteByte('\n')
			}
		}
		if entry.Key != "" {
			buf.WriteString(entry.Key)
			buf.WriteByte('=')
			buf.WriteString(entry.Value)
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes(), nil
}

// splitKeyValue splits a property line on the first unescaped = or : separator.
func splitKeyValue(line string) (string, string) {
	sepIdx := -1
	for i, ch := range line {
		if ch == '=' || ch == ':' {
			sepIdx = i
			break
		}
	}
	if sepIdx < 0 {
		return strings.TrimSpace(line), ""
	}
	key := strings.TrimSpace(line[:sepIdx])
	value := strings.TrimSpace(line[sepIdx+1:])
	return key, value
}
