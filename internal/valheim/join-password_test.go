package valheim

import "testing"

func TestValidateJoinPassword(t *testing.T) {
	for _, test := range []struct {
		name, password, serverName string
		valid                      bool
	}{
		{name: "minimum", password: "abcde", serverName: "My server", valid: true},
		{name: "one argument with spaces", password: "two words", serverName: "My server", valid: true},
		{name: "Unicode characters", password: "冬夜森林家", serverName: "My server", valid: true}, // #nosec G101 -- Synthetic Unicode validation fixture.
		{name: "short Unicode", password: "冬夜", serverName: "My server"},
		{name: "embedded password", password: "abcde", serverName: "my abcde server"},
		{name: "NUL", password: "abcde\x00"},
		{name: "newline", password: "abcde\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			errValidate := ValidateJoinPassword(test.password, test.serverName)
			if (errValidate == nil) != test.valid {
				t.Fatalf("valid = %v, want %v", errValidate == nil, test.valid)
			}
		})
	}
}
