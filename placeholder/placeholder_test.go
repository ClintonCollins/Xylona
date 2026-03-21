package placeholder

import (
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		template string
		vars     map[string]string
		want     string
	}{
		{
			name:     "single placeholder",
			template: "server -port {{PORT}}",
			vars:     map[string]string{"PORT": "27015"},
			want:     "server -port 27015",
		},
		{
			name:     "multiple placeholders",
			template: "-ip {{IP}} -port {{PORT}}",
			vars:     map[string]string{"IP": "192.168.1.1", "PORT": "27015"},
			want:     "-ip 192.168.1.1 -port 27015",
		},
		{
			name:     "no placeholders",
			template: "just a plain string",
			vars:     map[string]string{"PORT": "27015"},
			want:     "just a plain string",
		},
		{
			name:     "unresolved placeholder resolves to empty",
			template: "server -port {{RCON_PORT}}",
			vars:     map[string]string{},
			want:     "server -port ",
		},
		{
			name:     "backward compat with legacy format",
			template: "server -dir %GAMESERVER_DIRECTORY%",
			vars:     map[string]string{"INSTALL_DIR": "/opt/game"},
			want:     "server -dir /opt/game",
		},
		{
			name:     "mixed formats both resolved",
			template: "{{IP}} and %GAMESERVER_PORT%",
			vars:     map[string]string{"IP": "127.0.0.1", "PORT": "27015"},
			want:     "127.0.0.1 and 27015",
		},
		{
			name:     "empty template",
			template: "",
			vars:     map[string]string{"PORT": "27015"},
			want:     "",
		},
		{
			name:     "nil vars",
			template: "{{PORT}}",
			vars:     nil,
			want:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Resolve(tt.template, tt.vars)
			if got != tt.want {
				t.Errorf("Resolve() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildVarsFromGameServer(t *testing.T) {
	gs := &models.GameServer{
		ID:              "test-server-1",
		Name:            "My Server",
		IP:              "192.168.1.100",
		Port:            27015,
		QueryPort:       27016,
		MaxPlayers:      32,
		SetPlayers:      24,
		Directory:       "/opt/gameservers/test",
		BackupDirectory: "/opt/backups/test",
		MaxMemoryMB:     4096,
	}
	vars := BuildVarsFromGameServer(gs)

	expected := map[string]string{
		"IP":            "192.168.1.100",
		"PORT":          "27015",
		"QUERY_PORT":    "27016",
		"MAX_PLAYERS":   "32",
		"SERVER_NAME":   "My Server",
		"INSTALL_DIR":   "/opt/gameservers/test",
		"SERVER_ID":     "test-server-1",
		"BACKUP_DIR":    "/opt/backups/test",
		"MAX_MEMORY_MB": "4096",
		"SET_PLAYERS":   "24",
	}

	for key, want := range expected {
		got, ok := vars[key]
		if !ok {
			t.Errorf("BuildVarsFromGameServer() missing key %q", key)
			continue
		}
		if got != want {
			t.Errorf("BuildVarsFromGameServer()[%q] = %q, want %q", key, got, want)
		}
	}
}

func TestLegacyMapping(t *testing.T) {
	tests := []struct {
		name   string
		legacy string
		newKey string
	}{
		{"directory", "%GAMESERVER_DIRECTORY%", "INSTALL_DIR"},
		{"ip", "%GAMESERVER_IP%", "IP"},
		{"port", "%GAMESERVER_PORT%", "PORT"},
		{"query port", "%GAMESERVER_QUERY_PORT%", "QUERY_PORT"},
		{"max players", "%GAMESERVER_MAX_PLAYERS%", "MAX_PLAYERS"},
		{"name", "%GAMESERVER_NAME%", "SERVER_NAME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LegacyToNewKey(tt.legacy)
			if !ok {
				t.Fatalf("LegacyToNewKey(%q) returned not found", tt.legacy)
			}
			if got != tt.newKey {
				t.Errorf("LegacyToNewKey(%q) = %q, want %q", tt.legacy, got, tt.newKey)
			}
		})
	}
}
