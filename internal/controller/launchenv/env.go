// Package launchenv parses, validates, and merges launch environment values.
package launchenv

import (
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	// MaxNameBytes is the maximum encoded byte length for an env var name.
	MaxNameBytes = 128
	// MaxValueBytes is the maximum encoded byte length for one env var value.
	MaxValueBytes = 4096
	// MaxVariables is the maximum count for normal env vars or secret env vars.
	MaxVariables = 64
	// MaxMergedEnvBytes is the maximum custom env payload size passed to a node.
	MaxMergedEnvBytes = 16 * 1024
)

const emptyStoredEnv = "[]"

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var deniedExactNames = map[string]struct{}{
	"PATH":              {},
	"COMSPEC":           {},
	"SYSTEMROOT":        {},
	"WINDIR":            {},
	"PATHEXT":           {},
	"JAVA_TOOL_OPTIONS": {},
	"_JAVA_OPTIONS":     {},
	"NODE_OPTIONS":      {},
	"PYTHONPATH":        {},
}

var deniedPrefixes = []string{"LD_", "DYLD_"}

// Variable is one visible launch environment variable.
type Variable struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// SecretState is the public metadata for a configured secret env var.
type SecretState struct {
	Name       string
	Configured bool
	UpdatedAt  time.Time
}

// ValidationIssue describes a launch environment validation problem.
type ValidationIssue struct {
	Name    string
	Message string
}

// ValidationError wraps one or more validation issues.
type ValidationError struct {
	Issues []ValidationIssue
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "invalid launch environment"
	}
	if len(e.Issues) == 1 {
		return e.Issues[0].Message
	}
	return fmt.Sprintf("invalid launch environment: %d issues", len(e.Issues))
}

// NewValidationError returns nil when issues is empty.
func NewValidationError(issues []ValidationIssue) error {
	if len(issues) == 0 {
		return nil
	}
	copied := make([]ValidationIssue, len(issues))
	copy(copied, issues)
	return &ValidationError{Issues: copied}
}

// ParseStored parses the JSON representation stored in the database.
func ParseStored(raw string) ([]Variable, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []Variable{}, nil
	}

	var variables []Variable
	errUnmarshal := json.Unmarshal([]byte(trimmed), &variables)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parse launch environment: %w", errUnmarshal)
	}
	if variables == nil {
		return []Variable{}, nil
	}
	return variables, nil
}

// MarshalStored validates and serializes visible env vars for database storage.
func MarshalStored(variables []Variable) (string, error) {
	issues := ValidateVariables(variables)
	if len(issues) > 0 {
		return "", NewValidationError(issues)
	}
	if len(variables) == 0 {
		return emptyStoredEnv, nil
	}

	encoded, errMarshal := json.Marshal(variables)
	if errMarshal != nil {
		return "", fmt.Errorf("marshal launch environment: %w", errMarshal)
	}
	return string(encoded), nil
}

// ValidateVariables validates a normal visible env var list.
func ValidateVariables(variables []Variable) []ValidationIssue {
	var issues []ValidationIssue
	if len(variables) > MaxVariables {
		issues = append(issues, ValidationIssue{Message: fmt.Sprintf("environment variable count exceeds %d", MaxVariables)})
	}

	seen := make(map[string]string, len(variables))
	for _, variable := range variables {
		issues = append(issues, ValidateName(variable.Name)...)
		issues = append(issues, ValidateValue(variable.Name, variable.Value)...)

		key := strings.ToUpper(variable.Name)
		previous, exists := seen[key]
		if exists {
			issues = append(issues, ValidationIssue{
				Name:    variable.Name,
				Message: fmt.Sprintf("environment variable %q duplicates %q", variable.Name, previous),
			})
			continue
		}
		seen[key] = variable.Name
	}
	return issues
}

// ValidateName validates one env var name.
func ValidateName(name string) []ValidationIssue {
	var issues []ValidationIssue
	if name == "" {
		return []ValidationIssue{{Message: "environment variable name is required"}}
	}
	if len(name) > MaxNameBytes {
		issues = append(issues, ValidationIssue{Name: name, Message: fmt.Sprintf("environment variable %q exceeds %d bytes", name, MaxNameBytes)})
	}
	if hasControlOrNUL(name) {
		issues = append(issues, ValidationIssue{Name: name, Message: fmt.Sprintf("environment variable %q contains control characters", name)})
	}
	if !envNamePattern.MatchString(name) {
		issues = append(issues, ValidationIssue{Name: name, Message: fmt.Sprintf("environment variable %q has an invalid name", name)})
	}
	if isDeniedName(name) {
		issues = append(issues, ValidationIssue{Name: name, Message: fmt.Sprintf("environment variable %q is reserved", name)})
	}
	return issues
}

// ValidateValue validates one env var value.
func ValidateValue(name string, value string) []ValidationIssue {
	var issues []ValidationIssue
	if len(value) > MaxValueBytes {
		issues = append(issues, ValidationIssue{Name: name, Message: fmt.Sprintf("environment variable %q value exceeds %d bytes", name, MaxValueBytes)})
	}
	if strings.ContainsRune(value, '\x00') {
		issues = append(issues, ValidationIssue{Name: name, Message: fmt.Sprintf("environment variable %q value contains a NUL byte", name)})
	}
	return issues
}

// MergeNormal merges game default and per-server normal env vars. Server vars
// override defaults case-insensitively while preserving a stable order.
func MergeNormal(defaults []Variable, server []Variable) ([]Variable, []ValidationIssue) {
	issues := append(ValidateVariables(defaults), ValidateVariables(server)...)
	if len(issues) > 0 {
		return nil, issues
	}

	merged := make([]Variable, 0, len(defaults)+len(server))
	indexByName := make(map[string]int, len(defaults)+len(server))
	for _, variable := range defaults {
		indexByName[strings.ToUpper(variable.Name)] = len(merged)
		merged = append(merged, variable)
	}
	for _, variable := range server {
		key := strings.ToUpper(variable.Name)
		index, exists := indexByName[key]
		if exists {
			merged[index] = variable
			continue
		}
		indexByName[key] = len(merged)
		merged = append(merged, variable)
	}
	return merged, nil
}

// ValidateSecretInput validates a secret env name and plaintext value.
func ValidateSecretInput(name string, value string) []ValidationIssue {
	issues := ValidateName(name)
	issues = append(issues, ValidateValue(name, value)...)
	return issues
}

// ValidateSecretStates validates configured secret names and conflicts against normal env.
func ValidateSecretStates(states []SecretState, normal []Variable) []ValidationIssue {
	var issues []ValidationIssue
	if len(states) > MaxVariables {
		issues = append(issues, ValidationIssue{Message: fmt.Sprintf("secret environment variable count exceeds %d", MaxVariables)})
	}

	normalNames := make(map[string]string, len(normal))
	for _, variable := range normal {
		normalNames[strings.ToUpper(variable.Name)] = variable.Name
	}

	seen := make(map[string]string, len(states))
	for _, state := range states {
		issues = append(issues, ValidateName(state.Name)...)

		key := strings.ToUpper(state.Name)
		previous, exists := seen[key]
		if exists {
			issues = append(issues, ValidationIssue{
				Name:    state.Name,
				Message: fmt.Sprintf("secret environment variable %q duplicates %q", state.Name, previous),
			})
			continue
		}
		seen[key] = state.Name

		normalName, conflicts := normalNames[key]
		if conflicts {
			issues = append(issues, ValidationIssue{
				Name:    state.Name,
				Message: fmt.Sprintf("secret environment variable %q conflicts with normal variable %q", state.Name, normalName),
			})
		}
	}
	return issues
}

// BuildLaunchEnv merges visible normal vars and decrypted secret vars.
func BuildLaunchEnv(normal []Variable, secrets map[string]string) (map[string]string, []ValidationIssue) {
	states := make([]SecretState, 0, len(secrets))
	for name := range secrets {
		states = append(states, SecretState{Name: name, Configured: true})
	}
	sort.Slice(states, func(i int, j int) bool {
		return strings.ToUpper(states[i].Name) < strings.ToUpper(states[j].Name)
	})

	issues := append(ValidateVariables(normal), ValidateSecretStates(states, normal)...)
	for name, value := range secrets {
		issues = append(issues, ValidateValue(name, value)...)
	}
	if len(issues) > 0 {
		return nil, issues
	}

	env := make(map[string]string, len(normal)+len(secrets))
	for _, variable := range normal {
		env[variable.Name] = variable.Value
	}
	maps.Copy(env, secrets)

	size := mergedEnvSize(env)
	if size > MaxMergedEnvBytes {
		return nil, []ValidationIssue{{Message: fmt.Sprintf("custom environment exceeds %d bytes", MaxMergedEnvBytes)}}
	}
	return env, nil
}

func hasControlOrNUL(value string) bool {
	for _, r := range value {
		if r == '\x00' || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func isDeniedName(name string) bool {
	upperName := strings.ToUpper(name)
	_, exact := deniedExactNames[upperName]
	if exact {
		return true
	}
	for _, prefix := range deniedPrefixes {
		if strings.HasPrefix(upperName, prefix) {
			return true
		}
	}
	return false
}

func mergedEnvSize(env map[string]string) int {
	size := 0
	for name, value := range env {
		size += len(name) + 1 + len(value) + 1
	}
	return size
}
