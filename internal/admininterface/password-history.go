package admininterface

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParsePasswordHistory validates and decodes an encrypted history payload.
// The returned order is oldest to newest. History is intentionally retained
// until Xylona can authenticate the live Satisfactory server: discarding an
// older value can make offline password changes impossible to apply.
func ParsePasswordHistory(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var history []string
	errDecode := json.Unmarshal([]byte(raw), &history)
	if errDecode != nil {
		return nil, fmt.Errorf("decode admin interface password history: %w", errDecode)
	}
	for _, password := range history {
		errPassword := ValidatePassword("satisfactory", password)
		if errPassword != nil {
			return nil, fmt.Errorf("validate admin interface password history: %w", errPassword)
		}
	}
	return history, nil
}

// AppendPasswordHistory adds one prior password and removes duplicates without
// discarding a password that may still be active on the live server.
func AppendPasswordHistory(raw string, password string) (string, error) {
	errPassword := ValidatePassword("satisfactory", password)
	if errPassword != nil {
		return "", errPassword
	}
	history, errParse := ParsePasswordHistory(raw)
	if errParse != nil {
		return "", errParse
	}
	filtered := make([]string, 0, len(history)+1)
	for _, existing := range history {
		if existing != password {
			filtered = append(filtered, existing)
		}
	}
	filtered = append(filtered, password)
	encoded, errEncode := json.Marshal(filtered)
	if errEncode != nil {
		return "", fmt.Errorf("encode admin interface password history: %w", errEncode)
	}
	return string(encoded), nil
}
