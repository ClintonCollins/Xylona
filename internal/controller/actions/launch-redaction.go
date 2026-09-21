package actions

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

var terminalEscape = regexp.MustCompile("\x1b(?:\\[[0-?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\))")

func redactLaunchError(text string, secrets ...string) string {
	// Longest first prevents a short secret from exposing the rest of a longer
	// credential that contains it. Redact before truncating at any boundary.
	values := slices.Clone(secrets)
	slices.SortFunc(values, func(a, b string) int { return len(b) - len(a) })
	for _, value := range values {
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	text = plainText(text)
	for _, value := range values {
		value = plainText(value)
		if value != "" {
			text = strings.ReplaceAll(text, value, "[redacted]")
		}
	}
	return text
}

func plainText(text string) string {
	text = terminalEscape.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Map(func(r rune) rune {
		if r == '\r' {
			return '\n'
		}
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, text)
}
