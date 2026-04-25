package cfgparse

import (
	"bytes"
	"strings"
	"unicode"
)

// CommandCFGParser parses command/convar style server.cfg files.
type CommandCFGParser struct{}

func init() {
	RegisterFlat(&CommandCFGParser{})
}

// Format returns the format identifier for command/convar configs.
func (p *CommandCFGParser) Format() string {
	return "commandcfg"
}

// Parse reads command/convar config lines and returns flat entries.
func (p *CommandCFGParser) Parse(data []byte) ([]ConfigEntry, error) {
	lines := strings.Split(string(data), "\n")
	entries := make([]ConfigEntry, 0, len(lines))
	var pendingComment strings.Builder
	keyCounts := map[string]int{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}
		if isCommandCFGComment(trimmed) {
			commentText := strings.TrimSpace(trimmed[commentPrefixLen(trimmed):])
			if pendingComment.Len() > 0 {
				pendingComment.WriteByte('\n')
			}
			pendingComment.WriteString(commentText)
			continue
		}

		entry := parseCommandCFGLine(trimmed)
		if entry.Key == "" {
			continue
		}
		entry.Index = keyCounts[entry.Key]
		keyCounts[entry.Key]++
		if pendingComment.Len() > 0 {
			entry.Comment = pendingComment.String()
			pendingComment.Reset()
		}
		entries = append(entries, entry)
	}

	if pendingComment.Len() > 0 {
		entries = append(entries, ConfigEntry{Comment: pendingComment.String()})
	}

	return entries, nil
}

// Write serializes entries back to command/convar config format.
func (p *CommandCFGParser) Write(entries []ConfigEntry) ([]byte, error) {
	var buf bytes.Buffer

	for _, entry := range entries {
		if entry.Comment != "" {
			for commentLine := range strings.SplitSeq(entry.Comment, "\n") {
				buf.WriteString("// ")
				buf.WriteString(commentLine)
				buf.WriteByte('\n')
			}
		}
		if entry.Key == "" {
			continue
		}
		writeCommandCFGEntry(&buf, entry)
		buf.WriteByte('\n')
	}

	return buf.Bytes(), nil
}

func parseCommandCFGLine(line string) ConfigEntry {
	key, value, hasEquals := strings.Cut(line, "=")
	if hasEquals && !strings.ContainsAny(key, " \t") {
		return ConfigEntry{
			Section: "=",
			Key:     strings.TrimSpace(key),
			Value:   unquoteCommandCFGValue(strings.TrimSpace(value)),
		}
	}

	words := splitCommandCFGWords(line)
	if len(words) == 0 {
		return ConfigEntry{}
	}
	if isCommandCFGSetCommand(words[0]) && len(words) >= 2 {
		return ConfigEntry{
			Section: words[0],
			Key:     words[1],
			Value:   strings.Join(words[2:], " "),
		}
	}
	if len(words) == 1 {
		return ConfigEntry{Key: words[0]}
	}
	return ConfigEntry{
		Key:   words[0],
		Value: strings.Join(words[1:], " "),
	}
}

func writeCommandCFGEntry(buf *bytes.Buffer, entry ConfigEntry) {
	value := quoteCommandCFGValue(entry.Value)
	switch {
	case entry.Section == "=":
		buf.WriteString(entry.Key)
		buf.WriteByte('=')
		buf.WriteString(value)
	case entry.Section != "":
		buf.WriteString(entry.Section)
		buf.WriteByte(' ')
		buf.WriteString(entry.Key)
		if entry.Value != "" {
			buf.WriteByte(' ')
			buf.WriteString(value)
		}
	default:
		buf.WriteString(entry.Key)
		if entry.Value != "" {
			buf.WriteByte(' ')
			buf.WriteString(value)
		}
	}
}

func splitCommandCFGWords(line string) []string {
	words := []string{}
	var current strings.Builder
	var quote rune
	escaped := false

	for _, r := range line {
		if escaped {
			current.WriteRune(r)
			escaped = false
			continue
		}
		if quote != 0 && r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			if current.Len() > 0 {
				words = append(words, current.String())
				current.Reset()
			}
			continue
		}
		current.WriteRune(r)
	}
	if escaped {
		current.WriteRune('\\')
	}
	if current.Len() > 0 {
		words = append(words, current.String())
	}

	return words
}

func quoteCommandCFGValue(value string) string {
	if value == "" {
		return `""`
	}
	needsQuote := strings.ContainsFunc(value, unicode.IsSpace) ||
		strings.HasPrefix(value, `"`) ||
		strings.HasPrefix(value, `'`) ||
		isCommandCFGComment(value)
	if !needsQuote {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func unquoteCommandCFGValue(value string) string {
	words := splitCommandCFGWords(value)
	if len(words) == 1 {
		return words[0]
	}
	return strings.Join(words, " ")
}

func isCommandCFGSetCommand(command string) bool {
	switch strings.ToLower(command) {
	case "set", "seta", "sets", "setr":
		return true
	default:
		return false
	}
}

func isCommandCFGComment(line string) bool {
	return strings.HasPrefix(line, "#") ||
		strings.HasPrefix(line, ";") ||
		strings.HasPrefix(line, "//")
}

func commentPrefixLen(line string) int {
	if strings.HasPrefix(line, "//") {
		return 2
	}
	return 1
}
