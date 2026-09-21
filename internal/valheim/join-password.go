package valheim

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ValidateJoinPassword validates the documented minimum and name restriction.
// The byte ceiling is a panel request limit, not a native game limit.
func ValidateJoinPassword(value, name string) error {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) < 5 {
		return errors.New("join password must contain at least five characters")
	}
	if len(value) > 1024 {
		return errors.New("join password exceeds the panel limit of 1024 bytes")
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("join password must not contain control characters")
		}
	}
	if strings.Contains(name, value) {
		return errors.New("server name must not contain the join password")
	}
	return nil
}
