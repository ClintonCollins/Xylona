package updateproviders

import (
	"encoding/json"
	"strings"
)

// SearchParams decodes serialized search parameters for a mod source.
func SearchParams(source ModSource) map[string]any {
	raw := strings.TrimSpace(source.SearchParamsJSON)
	if raw == "" {
		return nil
	}

	var parsed map[string]any
	errUnmarshal := json.Unmarshal([]byte(raw), &parsed)
	if errUnmarshal != nil {
		return nil
	}
	return parsed
}
