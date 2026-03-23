package modmanager

import (
	"encoding/json"
	"strings"
)

// ServerSoftware represents one server software option in a game definition.
type ServerSoftware struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	JarSource string     `json:"jar_source"`
	ModConfig *ModConfig `json:"mod_config"`
}

// ModConfig defines what mods are supported and where they come from.
type ModConfig struct {
	ModTypes []ModType      `json:"mod_types"`
	Sources  []SourceConfig `json:"sources"`
}

// ModType defines a type of mod and where it installs.
type ModType struct {
	Type        string `json:"type"`
	Label       string `json:"label"`
	InstallPath string `json:"install_path"`
}

// SourceConfig defines a mod source and its search parameters.
type SourceConfig struct {
	ID           string         `json:"id"`
	SearchParams map[string]any `json:"search_params"`
}

// ParseServerSoftware parses the server_software JSON column.
func ParseServerSoftware(jsonStr string) ([]ServerSoftware, error) {
	trimmed := strings.TrimSpace(jsonStr)
	if trimmed == "" || trimmed == "null" {
		return nil, nil
	}

	var software []ServerSoftware
	errUnmarshal := json.Unmarshal([]byte(trimmed), &software)
	if errUnmarshal != nil {
		return nil, errUnmarshal
	}
	return software, nil
}

// GetSoftwareByID finds a software option by ID.
func GetSoftwareByID(software []ServerSoftware, id string) (*ServerSoftware, bool) {
	for i := range software {
		if software[i].ID == id {
			return &software[i], true
		}
	}
	return nil, false
}

// ProviderIDForGame resolves the effective provider ID for a server software option.
// This preserves legacy Minecraft Vanilla definitions that omitted jar_source.
func ProviderIDForGame(gameID string, sw ServerSoftware) string {
	if sw.JarSource != "" {
		return sw.JarSource
	}
	if strings.EqualFold(gameID, "minecraft") && strings.EqualFold(sw.ID, "vanilla") {
		return "mojang"
	}
	return ""
}
