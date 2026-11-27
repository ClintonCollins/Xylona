package gamedefinitions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/cfgschema"
)

// Starbound's queryServerPort is independent of gameServerPort. See the
// developer-linked wiki: https://starbounder.org/Guide:LinuxServerSetup#Server_Configuration.
func TestStarboundDefinitionQueryPort(t *testing.T) {
	data, errRead := FS.ReadFile("official/starbound.json")
	if errRead != nil {
		t.Fatal(errRead)
	}
	definition, errParse := Parse(data)
	if errParse != nil {
		t.Fatal(errParse)
	}
	if len(definition.Warnings) != 0 {
		t.Fatalf("definition warnings = %v", definition.Warnings)
	}

	tests := []struct {
		name     string
		existing string
	}{
		{name: "new config"},
		{name: "existing config without query port", existing: `{"runQueryServer":false,"maxTeamSize":6}`},
		{name: "existing config with stale query port", existing: `{"queryServerPort":21025,"runQueryServer":false,"maxTeamSize":6}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			configPath := filepath.Join(dir, "storage", "starbound_server.config")
			if test.existing != "" {
				errMkdir := os.MkdirAll(filepath.Dir(configPath), 0o700)
				if errMkdir != nil {
					t.Fatal(errMkdir)
				}
				errWrite := os.WriteFile(configPath, []byte(test.existing), 0o600)
				if errWrite != nil {
					t.Fatal(errWrite)
				}
			}

			// Repeated starts must follow later changes to the server's query
			// port without coupling it to the game port or resetting opt-outs.
			for _, queryPort := range []int64{22026, 23026} {
				resolver := cfgschema.GameServerSettingsResolver(cfgschema.GameServerSettings{
					Name: "Test Starbound", Port: 22025, QueryPort: queryPort, MaxPlayers: 8,
				})
				errStart := cfgschema.RunPreStartStrict(dir, string(definition.Document.ConfigSchemas), resolver)
				if errStart != nil {
					t.Fatal(errStart)
				}
				generated, errGenerated := os.ReadFile(configPath)
				if errGenerated != nil {
					t.Fatal(errGenerated)
				}
				var config map[string]any
				errJSON := json.Unmarshal(generated, &config)
				if errJSON != nil {
					t.Fatal(errJSON)
				}
				if config["queryServerPort"] != float64(queryPort) {
					t.Errorf("queryServerPort = %v, want %d", config["queryServerPort"], queryPort)
				}
				if config["gameServerPort"] != float64(22025) {
					t.Errorf("gameServerPort = %v, want 22025", config["gameServerPort"])
				}
				wantQueryEnabled := test.existing == ""
				if config["runQueryServer"] != wantQueryEnabled {
					t.Errorf("runQueryServer = %v, want %t", config["runQueryServer"], wantQueryEnabled)
				}
				if test.existing != "" && config["maxTeamSize"] != float64(6) {
					t.Errorf("maxTeamSize = %v, want 6", config["maxTeamSize"])
				}
			}
		})
	}
}
