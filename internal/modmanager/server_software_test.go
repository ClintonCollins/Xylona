package modmanager

import (
	"testing"
)

func TestParseServerSoftware(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantLen   int
		wantErr   bool
		wantFirst string
	}{
		{
			name: "valid JSON with multiple options",
			input: `[
				{"id":"paper","name":"Paper","jar_source":"papermc","mod_config":{"mod_types":[{"type":"plugin","label":"Plugins","install_path":"plugins"}],"sources":[{"id":"modrinth","search_params":{"facets":"categories:paper"}}]}},
				{"id":"fabric","name":"Fabric","jar_source":"fabric-meta","mod_config":{"mod_types":[{"type":"mod","label":"Mods","install_path":"mods"}],"sources":[{"id":"modrinth","search_params":{"facets":"categories:fabric"}}]}}
			]`,
			wantLen:   2,
			wantErr:   false,
			wantFirst: "paper",
		},
		{
			name:    "empty string returns nil",
			input:   "",
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "null string returns nil",
			input:   "null",
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "whitespace-only returns nil",
			input:   "   ",
			wantLen: 0,
			wantErr: false,
		},
		{
			name:    "malformed JSON returns error",
			input:   `[{"id": "broken"`,
			wantLen: 0,
			wantErr: true,
		},
		{
			name:    "empty array",
			input:   `[]`,
			wantLen: 0,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseServerSoftware(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseServerSoftware() error = %v, wantErr %v", err, tt.wantErr)
			}
			if len(got) != tt.wantLen {
				t.Errorf("ParseServerSoftware() len = %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantFirst != "" && len(got) > 0 && got[0].ID != tt.wantFirst {
				t.Errorf("ParseServerSoftware()[0].ID = %q, want %q", got[0].ID, tt.wantFirst)
			}
		})
	}
}

func TestParseServerSoftwareModConfig(t *testing.T) {
	input := `[{"id":"paper","name":"Paper","jar_source":"papermc","mod_config":{"mod_types":[{"type":"plugin","label":"Plugins","install_path":"plugins"}],"sources":[{"id":"modrinth","search_params":{"facets":"categories:paper"}}]}}]`

	software, errParse := ParseServerSoftware(input)
	if errParse != nil {
		t.Fatalf("ParseServerSoftware() error = %v", errParse)
	}
	if len(software) != 1 {
		t.Fatalf("ParseServerSoftware() len = %d, want 1", len(software))
	}

	sw := software[0]
	if sw.ModConfig == nil {
		t.Fatal("ModConfig is nil")
	}
	if len(sw.ModConfig.ModTypes) != 1 {
		t.Fatalf("ModTypes len = %d, want 1", len(sw.ModConfig.ModTypes))
	}
	if sw.ModConfig.ModTypes[0].Type != "plugin" {
		t.Errorf("ModTypes[0].Type = %q, want %q", sw.ModConfig.ModTypes[0].Type, "plugin")
	}
	if sw.ModConfig.ModTypes[0].InstallPath != "plugins" {
		t.Errorf("ModTypes[0].InstallPath = %q, want %q", sw.ModConfig.ModTypes[0].InstallPath, "plugins")
	}
	if len(sw.ModConfig.Sources) != 1 {
		t.Fatalf("Sources len = %d, want 1", len(sw.ModConfig.Sources))
	}
	if sw.ModConfig.Sources[0].ID != "modrinth" {
		t.Errorf("Sources[0].ID = %q, want %q", sw.ModConfig.Sources[0].ID, "modrinth")
	}
}

func TestParseServerSoftwareNilModConfig(t *testing.T) {
	input := `[{"id":"vanilla","name":"Vanilla","jar_source":"mojang"}]`

	software, errParse := ParseServerSoftware(input)
	if errParse != nil {
		t.Fatalf("ParseServerSoftware() error = %v", errParse)
	}
	if len(software) != 1 {
		t.Fatalf("ParseServerSoftware() len = %d, want 1", len(software))
	}
	if software[0].ModConfig != nil {
		t.Errorf("expected nil ModConfig for vanilla, got %+v", software[0].ModConfig)
	}
}

func TestGetSoftwareByID(t *testing.T) {
	software := []ServerSoftware{
		{ID: "paper", Name: "Paper"},
		{ID: "fabric", Name: "Fabric"},
		{ID: "forge", Name: "Forge"},
	}

	tests := []struct {
		name     string
		software []ServerSoftware
		id       string
		wantName string
		wantOK   bool
	}{
		{name: "find first", software: software, id: "paper", wantName: "Paper", wantOK: true},
		{name: "find middle", software: software, id: "fabric", wantName: "Fabric", wantOK: true},
		{name: "find last", software: software, id: "forge", wantName: "Forge", wantOK: true},
		{name: "not found", software: software, id: "vanilla", wantName: "", wantOK: false},
		{name: "empty id", software: software, id: "", wantName: "", wantOK: false},
		{name: "nil slice", software: nil, id: "paper", wantName: "", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := GetSoftwareByID(tt.software, tt.id)
			if ok != tt.wantOK {
				t.Fatalf("GetSoftwareByID() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && got.Name != tt.wantName {
				t.Errorf("GetSoftwareByID().Name = %q, want %q", got.Name, tt.wantName)
			}
		})
	}
}

func TestProviderIDForGame(t *testing.T) {
	tests := []struct {
		name   string
		gameID string
		input  ServerSoftware
		want   string
	}{
		{
			name:   "uses explicit jar source when present",
			gameID: "minecraft",
			input:  ServerSoftware{ID: "paper", JarSource: "papermc"},
			want:   "papermc",
		},
		{
			name:   "maps legacy minecraft vanilla to mojang",
			gameID: "minecraft",
			input:  ServerSoftware{ID: "vanilla"},
			want:   "mojang",
		},
		{
			name:   "leaves non minecraft blank source empty",
			gameID: "terraria",
			input:  ServerSoftware{ID: "vanilla"},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ProviderIDForGame(tt.gameID, tt.input)
			if got != tt.want {
				t.Errorf("ProviderIDForGame() = %q, want %q", got, tt.want)
			}
		})
	}
}
