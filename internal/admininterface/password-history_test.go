package admininterface

import (
	"slices"
	"testing"
)

func TestAppendPasswordHistory(t *testing.T) {
	t.Parallel()

	raw := ""
	passwords := []string{
		"password-one",
		"password-two",
		"password-three",
		"password-four",
		"password-five",
		"password-three",
	}
	for _, password := range passwords {
		var errAppend error
		raw, errAppend = AppendPasswordHistory(raw, password)
		if errAppend != nil {
			t.Fatalf("AppendPasswordHistory(%q) error = %v", password, errAppend)
		}
	}
	history, errParse := ParsePasswordHistory(raw)
	if errParse != nil {
		t.Fatalf("ParsePasswordHistory() error = %v", errParse)
	}
	want := []string{"password-one", "password-two", "password-four", "password-five", "password-three"}
	if !slices.Equal(history, want) {
		t.Fatalf("history = %q, want %q", history, want)
	}
}
