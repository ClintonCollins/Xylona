package launchenv

import (
	"strings"
	"testing"
)

func TestValidateVariables(t *testing.T) {
	tests := []struct {
		name      string
		variables []Variable
		wantIssue string
	}{
		{
			name:      "valid",
			variables: []Variable{{Name: "HYLE_AUTH_TOKEN", Value: "token"}},
		},
		{
			name:      "empty name",
			variables: []Variable{{Name: "", Value: "value"}},
			wantIssue: "environment variable name is required",
		},
		{
			name:      "invalid name",
			variables: []Variable{{Name: "1TOKEN", Value: "value"}},
			wantIssue: "has an invalid name",
		},
		{
			name:      "reserved exact name",
			variables: []Variable{{Name: "Path", Value: "value"}},
			wantIssue: "is reserved",
		},
		{
			name:      "reserved prefix",
			variables: []Variable{{Name: "ld_preload", Value: "value"}},
			wantIssue: "is reserved",
		},
		{
			name: "duplicate case insensitive",
			variables: []Variable{
				{Name: "TOKEN", Value: "one"},
				{Name: "token", Value: "two"},
			},
			wantIssue: "duplicates",
		},
		{
			name:      "nul value",
			variables: []Variable{{Name: "TOKEN", Value: "a\x00b"}},
			wantIssue: "NUL byte",
		},
		{
			name:      "too long value",
			variables: []Variable{{Name: "TOKEN", Value: strings.Repeat("a", MaxValueBytes+1)}},
			wantIssue: "value exceeds",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			issues := ValidateVariables(test.variables)
			if test.wantIssue == "" {
				if len(issues) != 0 {
					t.Fatalf("ValidateVariables() issues = %+v, want none", issues)
				}
				return
			}
			if !hasIssueContaining(issues, test.wantIssue) {
				t.Fatalf("ValidateVariables() issues = %+v, want issue containing %q", issues, test.wantIssue)
			}
		})
	}
}

func TestMarshalStoredRoundTripsStableJSON(t *testing.T) {
	variables := []Variable{
		{Name: "TOKEN", Value: "abc"},
		{Name: "EMPTY", Value: ""},
	}

	encoded, errMarshal := MarshalStored(variables)
	if errMarshal != nil {
		t.Fatalf("MarshalStored() error = %v", errMarshal)
	}

	decoded, errParse := ParseStored(encoded)
	if errParse != nil {
		t.Fatalf("ParseStored() error = %v", errParse)
	}
	if len(decoded) != len(variables) {
		t.Fatalf("ParseStored() length = %d, want %d", len(decoded), len(variables))
	}
	for i, variable := range variables {
		if decoded[i] != variable {
			t.Fatalf("ParseStored()[%d] = %+v, want %+v", i, decoded[i], variable)
		}
	}
}

func TestMergeNormal(t *testing.T) {
	defaults := []Variable{
		{Name: "TOKEN", Value: "default"},
		{Name: "REGION", Value: "us"},
	}
	server := []Variable{
		{Name: "token", Value: "server"},
		{Name: "SERVER_ONLY", Value: "yes"},
	}

	merged, issues := MergeNormal(defaults, server)
	if len(issues) != 0 {
		t.Fatalf("MergeNormal() issues = %+v, want none", issues)
	}

	want := []Variable{
		{Name: "token", Value: "server"},
		{Name: "REGION", Value: "us"},
		{Name: "SERVER_ONLY", Value: "yes"},
	}
	if len(merged) != len(want) {
		t.Fatalf("MergeNormal() length = %d, want %d", len(merged), len(want))
	}
	for i := range want {
		if merged[i] != want[i] {
			t.Fatalf("MergeNormal()[%d] = %+v, want %+v", i, merged[i], want[i])
		}
	}
}

func TestValidateSecretStatesRejectsNormalConflicts(t *testing.T) {
	issues := ValidateSecretStates(
		[]SecretState{{Name: "TOKEN", Configured: true}},
		[]Variable{{Name: "token", Value: "visible"}},
	)
	if !hasIssueContaining(issues, "conflicts with normal variable") {
		t.Fatalf("ValidateSecretStates() issues = %+v, want normal conflict", issues)
	}
}

func TestBuildLaunchEnv(t *testing.T) {
	t.Run("merges normal and secret values", func(t *testing.T) {
		env, issues := BuildLaunchEnv(
			[]Variable{{Name: "VISIBLE", Value: "one"}},
			map[string]string{"SECRET": "two"},
		)
		if len(issues) != 0 {
			t.Fatalf("BuildLaunchEnv() issues = %+v, want none", issues)
		}
		if env["VISIBLE"] != "one" || env["SECRET"] != "two" {
			t.Fatalf("BuildLaunchEnv() = %+v, want visible and secret values", env)
		}
	})

	t.Run("rejects secret conflict", func(t *testing.T) {
		_, issues := BuildLaunchEnv(
			[]Variable{{Name: "TOKEN", Value: "visible"}},
			map[string]string{"token": "secret"},
		)
		if !hasIssueContaining(issues, "conflicts with normal variable") {
			t.Fatalf("BuildLaunchEnv() issues = %+v, want conflict", issues)
		}
	})

	t.Run("rejects oversized merged env", func(t *testing.T) {
		_, issues := BuildLaunchEnv(
			[]Variable{
				{Name: "VALUE_1", Value: strings.Repeat("a", MaxValueBytes)},
				{Name: "VALUE_2", Value: strings.Repeat("b", MaxValueBytes)},
				{Name: "VALUE_3", Value: strings.Repeat("c", MaxValueBytes)},
				{Name: "VALUE_4", Value: strings.Repeat("d", MaxValueBytes)},
				{Name: "VALUE_5", Value: strings.Repeat("e", MaxValueBytes)},
			},
			nil,
		)
		if !hasIssueContaining(issues, "custom environment exceeds") {
			t.Fatalf("BuildLaunchEnv() issues = %+v, want size issue", issues)
		}
	})
}

func hasIssueContaining(issues []ValidationIssue, value string) bool {
	for _, issue := range issues {
		if strings.Contains(issue.Message, value) {
			return true
		}
	}
	return false
}
