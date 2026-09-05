package webhooks

import (
	"fmt"
	"maps"
	"slices"
)

// SlackPayload is the top-level payload for Slack incoming webhooks using Block Kit.
type SlackPayload struct {
	Blocks []SlackBlock `json:"blocks"`
}

// SlackBlock represents a single block in a Slack Block Kit message.
type SlackBlock struct {
	Type     string      `json:"type"`
	Text     *SlackText  `json:"text,omitempty"`
	Fields   []SlackText `json:"fields,omitempty"`
	Elements []SlackText `json:"elements,omitempty"`
}

// SlackText represents a text composition object in Slack Block Kit.
type SlackText struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// FormatSlack formats an AlertEvent as a Slack Block Kit payload.
func FormatSlack(event AlertEvent) SlackPayload {
	blocks := make([]SlackBlock, 0, 4)

	// Header block with the event type title.
	blocks = append(blocks, SlackBlock{
		Type: "header",
		Text: &SlackText{
			Type: "plain_text",
			Text: EventTypeTitle(event.EventType),
		},
	})

	// Section block with the event message.
	blocks = append(blocks, SlackBlock{
		Type: "section",
		Text: &SlackText{
			Type: "mrkdwn",
			Text: event.Message,
		},
	})

	// Section block with fields if present.
	if len(event.Fields) > 0 {
		// Sort keys for deterministic output.
		keys := slices.Sorted(maps.Keys(event.Fields))

		fields := make([]SlackText, 0, len(event.Fields))
		for _, k := range keys {
			fields = append(fields, SlackText{
				Type: "mrkdwn",
				Text: fmt.Sprintf("*%s*\n%s", k, event.Fields[k]),
			})
		}
		blocks = append(blocks, SlackBlock{
			Type:   "section",
			Fields: fields,
		})
	}

	// Context block with timestamp.
	blocks = append(blocks, SlackBlock{
		Type: "context",
		Elements: []SlackText{
			{
				Type: "mrkdwn",
				Text: fmt.Sprintf("<!date^%d^{date_num} {time_secs}|%s>",
					event.Timestamp.Unix(),
					event.Timestamp.UTC().Format("2006-01-02 15:04:05 UTC")),
			},
		},
	})

	return SlackPayload{Blocks: blocks}
}
