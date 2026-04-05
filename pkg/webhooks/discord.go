// Package webhooks formats and delivers alert notifications to webhook targets.
package webhooks

import (
	"sort"
)

// Discord embed color constants matching the project's design tokens.
const (
	ColorInfo     = 0x3B82F6 // blue
	ColorWarning  = 0xF59E0B // amber
	ColorCritical = 0xEF4444 // red
)

// DiscordPayload is the top-level payload for Discord webhook execution.
type DiscordPayload struct {
	Embeds []DiscordEmbed `json:"embeds"`
}

// DiscordEmbed represents a single Discord embed object.
type DiscordEmbed struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Color       int            `json:"color"`
	Fields      []DiscordField `json:"fields,omitempty"`
	Timestamp   string         `json:"timestamp,omitempty"`
}

// DiscordField represents a field within a Discord embed.
type DiscordField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// FormatDiscord formats an AlertEvent as a Discord webhook payload.
func FormatDiscord(event AlertEvent) DiscordPayload {
	embed := DiscordEmbed{
		Title:       EventTypeTitle(event.EventType),
		Description: event.Message,
		Color:       severityColor(event.Severity),
		Timestamp:   event.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
	}

	if len(event.Fields) > 0 {
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(event.Fields))
		for k := range event.Fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		fields := make([]DiscordField, 0, len(event.Fields))
		for _, k := range keys {
			fields = append(fields, DiscordField{
				Name:   k,
				Value:  event.Fields[k],
				Inline: true,
			})
		}
		embed.Fields = fields
	}

	return DiscordPayload{
		Embeds: []DiscordEmbed{embed},
	}
}

// severityColor returns the Discord embed color for the given severity.
func severityColor(s Severity) int {
	switch s {
	case SeverityInfo:
		return ColorInfo
	case SeverityWarning:
		return ColorWarning
	case SeverityCritical:
		return ColorCritical
	default:
		return ColorInfo
	}
}
