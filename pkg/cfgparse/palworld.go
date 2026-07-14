package cfgparse

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
)

const palworldSettingsSection = "/Script/Pal.PalGameWorldSettings"

// ErrPalworldOptionSettingsMissing is returned when a Palworld settings file
// does not contain the required OptionSettings tuple.
var ErrPalworldOptionSettingsMissing = errors.New("palworld OptionSettings entry is missing")

// PalworldParser parses PalWorldSettings.ini OptionSettings tuples.
type PalworldParser struct{}

func init() {
	RegisterFlat(&PalworldParser{})
}

// Format returns the format identifier for Palworld settings files.
func (p *PalworldParser) Format() string {
	return "palworld"
}

// Parse converts the OptionSettings tuple into editable flat entries while
// retaining nested tuple values such as CrossplayPlatforms.
func (p *PalworldParser) Parse(data []byte) ([]ConfigEntry, error) {
	text := string(data)
	openIndex, closeIndex, errTuple := palworldOptionSettingsBounds(text)
	if errTuple != nil {
		return nil, errTuple
	}

	fields, errFields := splitPalworldFields(text[openIndex+1 : closeIndex])
	if errFields != nil {
		return nil, errFields
	}

	entries := make([]ConfigEntry, 0, len(fields))
	keyCounts := make(map[string]int)
	for _, field := range fields {
		key, rawValue, found := strings.Cut(field, "=")
		if !found {
			return nil, fmt.Errorf("palworld OptionSettings field %q has no value", field)
		}
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("palworld OptionSettings contains an empty key")
		}

		value, errValue := parsePalworldValue(key, strings.TrimSpace(rawValue))
		if errValue != nil {
			return nil, fmt.Errorf("palworld OptionSettings field %q: %w", key, errValue)
		}
		entries = append(entries, ConfigEntry{
			Key:   key,
			Value: value,
			Index: keyCounts[key],
		})
		keyCounts[key]++
	}

	return entries, nil
}

// Write renders entries using Palworld's required two-line settings format.
func (p *PalworldParser) Write(entries []ConfigEntry) ([]byte, error) {
	var output bytes.Buffer
	output.WriteByte('[')
	output.WriteString(palworldSettingsSection)
	output.WriteString("]\nOptionSettings=(")

	wroteEntry := false
	for _, entry := range entries {
		key := strings.TrimSpace(entry.Key)
		if key == "" {
			continue
		}
		value, errValue := formatPalworldValue(key, entry.Value)
		if errValue != nil {
			return nil, fmt.Errorf("palworld OptionSettings field %q: %w", key, errValue)
		}
		if wroteEntry {
			output.WriteByte(',')
		}
		output.WriteString(key)
		output.WriteByte('=')
		output.WriteString(value)
		wroteEntry = true
	}

	output.WriteString(")\n")
	return output.Bytes(), nil
}

func palworldOptionSettingsBounds(text string) (int, int, error) {
	lowerText := strings.ToLower(text)
	settingsIndex := strings.Index(lowerText, "optionsettings")
	if settingsIndex < 0 {
		return 0, 0, ErrPalworldOptionSettingsMissing
	}

	equalsOffset := strings.Index(text[settingsIndex:], "=")
	if equalsOffset < 0 {
		return 0, 0, ErrPalworldOptionSettingsMissing
	}
	openIndex := settingsIndex + equalsOffset + 1
	for openIndex < len(text) && (text[openIndex] == ' ' || text[openIndex] == '\t') {
		openIndex++
	}
	if openIndex >= len(text) || text[openIndex] != '(' {
		return 0, 0, ErrPalworldOptionSettingsMissing
	}

	closeIndex := findPalworldClosingParenthesis(text, openIndex)
	if closeIndex < 0 {
		return 0, 0, errors.New("palworld OptionSettings entry has no closing parenthesis")
	}
	return openIndex, closeIndex, nil
}

func findPalworldClosingParenthesis(text string, openIndex int) int {
	depth := 0
	inQuote := false
	escaped := false
	for index := openIndex; index < len(text); index++ {
		character := text[index]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inQuote = false
			}
			continue
		}
		switch character {
		case '"':
			inQuote = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitPalworldFields(settings string) ([]string, error) {
	fields := make([]string, 0, 64)
	start := 0
	depth := 0
	inQuote := false
	escaped := false
	for index := 0; index < len(settings); index++ {
		character := settings[index]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inQuote = false
			}
			continue
		}
		switch character {
		case '"':
			inQuote = true
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, errors.New("palworld OptionSettings contains an unexpected closing parenthesis")
			}
			depth--
		case ',':
			if depth == 0 {
				field := strings.TrimSpace(settings[start:index])
				if field != "" {
					fields = append(fields, field)
				}
				start = index + 1
			}
		}
	}
	if inQuote || depth != 0 {
		return nil, errors.New("palworld OptionSettings contains an unterminated value")
	}
	lastField := strings.TrimSpace(settings[start:])
	if lastField != "" {
		fields = append(fields, lastField)
	}
	return fields, nil
}

func parsePalworldValue(key string, rawValue string) (string, error) {
	if palworldStringKeys[key] {
		return unquotePalworldString(rawValue)
	}
	if palworldArrayKeys[key] {
		return parsePalworldArray(key, rawValue)
	}
	if strings.EqualFold(rawValue, "true") {
		return "true", nil
	}
	if strings.EqualFold(rawValue, "false") {
		return "false", nil
	}
	return rawValue, nil
}

func formatPalworldValue(key string, value string) (string, error) {
	if palworldStringKeys[key] {
		return quotePalworldString(value)
	}
	if palworldArrayKeys[key] {
		return formatPalworldArray(key, value)
	}
	if strings.EqualFold(value, "true") {
		return "True", nil
	}
	if strings.EqualFold(value, "false") {
		return "False", nil
	}
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("value contains unsupported control characters")
	}
	return strings.TrimSpace(value), nil
}

func quotePalworldString(value string) (string, error) {
	if strings.ContainsAny(value, "\x00\r\n") {
		return "", errors.New("string contains unsupported control characters")
	}
	escaped := strings.NewReplacer("\\", "\\\\", "\"", "\\\"").Replace(value)
	return "\"" + escaped + "\"", nil
}

func unquotePalworldString(value string) (string, error) {
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return strings.TrimSpace(value), nil
	}
	var result strings.Builder
	escaped := false
	for index := 1; index < len(value)-1; index++ {
		character := value[index]
		if escaped {
			result.WriteByte(character)
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		result.WriteByte(character)
	}
	if escaped {
		return "", errors.New("quoted string has an incomplete escape")
	}
	return result.String(), nil
}

func parsePalworldArray(key string, rawValue string) (string, error) {
	trimmed := strings.TrimSpace(rawValue)
	if len(trimmed) < 2 || trimmed[0] != '(' || trimmed[len(trimmed)-1] != ')' {
		return trimmed, nil
	}
	items, errItems := splitPalworldFields(trimmed[1 : len(trimmed)-1])
	if errItems != nil {
		return "", errItems
	}
	for index := range items {
		items[index] = strings.TrimSpace(items[index])
		if key == "DenyTechnologyList" {
			item, errItem := unquotePalworldString(items[index])
			if errItem != nil {
				return "", errItem
			}
			items[index] = item
		}
	}
	return strings.Join(items, ","), nil
}

func formatPalworldArray(key string, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "()" {
		return "()", nil
	}
	if strings.HasPrefix(trimmed, "(") && strings.HasSuffix(trimmed, ")") {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	items, errItems := splitPalworldFields(trimmed)
	if errItems != nil {
		return "", errItems
	}
	for index := range items {
		items[index] = strings.TrimSpace(items[index])
		if key == "DenyTechnologyList" {
			quoted, errQuote := quotePalworldString(strings.Trim(items[index], "\""))
			if errQuote != nil {
				return "", errQuote
			}
			items[index] = quoted
		}
	}
	return "(" + strings.Join(items, ",") + ")", nil
}

var palworldStringKeys = map[string]bool{
	"AdditionalDropItemWhenPlayerKillingInPvPMode": true,
	"AdminPassword":     true,
	"BanListURL":        true,
	"PublicIP":          true,
	"RandomizerSeed":    true,
	"Region":            true,
	"ServerDescription": true,
	"ServerName":        true,
	"ServerPassword":    true,
}

var palworldArrayKeys = map[string]bool{
	"CrossplayPlatforms": true,
	"DenyTechnologyList": true,
}
