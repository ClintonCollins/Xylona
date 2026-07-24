package protomap

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const emptyConsoleCommands = "[]"

// GameConsoleCommandsFromStored parses the JSON persisted on a game definition.
func GameConsoleCommandsFromStored(stored string) ([]*xylona.GameConsoleCommand, error) {
	trimmed := strings.TrimSpace(stored)
	if trimmed == "" || trimmed == "null" || trimmed == emptyConsoleCommands {
		return nil, nil
	}

	var rawCommands []json.RawMessage
	errUnmarshal := json.Unmarshal([]byte(trimmed), &rawCommands)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parse console commands JSON: %w", errUnmarshal)
	}
	if rawCommands == nil {
		return nil, errors.New("console commands must be a JSON array")
	}

	commands := make([]*xylona.GameConsoleCommand, 0, len(rawCommands))
	for index, rawCommand := range rawCommands {
		command := &xylona.GameConsoleCommand{}
		errCommand := protojson.Unmarshal(rawCommand, command)
		if errCommand != nil {
			return nil, fmt.Errorf("parse console command %d: %w", index, errCommand)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

// GameConsoleCommandsToStored serializes game console commands for SQLite.
func GameConsoleCommandsToStored(commands []*xylona.GameConsoleCommand) (string, error) {
	if len(commands) == 0 {
		return emptyConsoleCommands, nil
	}

	rawCommands := make([]json.RawMessage, 0, len(commands))
	for index, command := range commands {
		if command == nil {
			return "", fmt.Errorf("console command %d is nil", index)
		}
		rawCommand, errMarshal := protojson.Marshal(command)
		if errMarshal != nil {
			return "", fmt.Errorf("marshal console command %d: %w", index, errMarshal)
		}
		rawCommands = append(rawCommands, rawCommand)
	}

	encoded, errMarshal := json.Marshal(rawCommands)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal console commands JSON: %w", errMarshal)
	}
	return string(encoded), nil
}
