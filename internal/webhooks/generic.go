package webhooks

// GenericPayload is a plain JSON payload for generic HTTP webhooks.
type GenericPayload struct {
	EventType    string            `json:"event_type"`
	Severity     string            `json:"severity"`
	Message      string            `json:"message"`
	ServerName   string            `json:"server_name,omitempty"`
	ServerID     string            `json:"server_id,omitempty"`
	ServerNodeID string            `json:"server_node_id,omitempty"`
	NodeID       string            `json:"node_id,omitempty"`
	Timestamp    string            `json:"timestamp"`
	Fields       map[string]string `json:"fields,omitempty"`
}

// FormatGeneric formats an AlertEvent as a generic JSON payload.
func FormatGeneric(event AlertEvent) GenericPayload {
	return GenericPayload{
		EventType:    event.EventType,
		Severity:     event.Severity.String(),
		Message:      event.Message,
		ServerName:   event.ServerName,
		ServerID:     event.ServerID,
		ServerNodeID: event.ServerNodeID,
		NodeID:       event.NodeID,
		Timestamp:    event.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		Fields:       event.Fields,
	}
}
