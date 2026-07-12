package placeholder

import (
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestRegistryIncludesMaxMemoryMB(t *testing.T) {
	found := false
	for _, placeholder := range Registry {
		if placeholder.Key != "MAX_MEMORY_MB" {
			continue
		}
		found = true
		if placeholder.Label != "Game Server Memory (MB)" {
			t.Errorf("MAX_MEMORY_MB label = %q, want %q", placeholder.Label, "Game Server Memory (MB)")
		}
	}

	if !found {
		t.Fatal("MAX_MEMORY_MB missing from Registry")
	}
}

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
		"IP":                "192.168.1.100",
		"PORT":              "27015",
		"PORT_PLUS_1":       "27016",
		"PORT_PLUS_2":       "27017",
		"QUERY_PORT":        "27016",
		"QUERY_PORT_PLUS_1": "27017",
		"MAX_PLAYERS":       "32",
		"SERVER_NAME":       "My Server",
		"INSTALL_DIR":       "/opt/gameservers/test",
		"SERVER_ID":         "test-server-1",
		"BACKUP_DIR":        "/opt/backups/test",
		"MAX_MEMORY_MB":     "4096",
		"SET_PLAYERS":       "24",
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

func TestBuildVarsFromGameServer_ServerExecutable(t *testing.T) {
	gs := &models.GameServer{
		ID:              "test-id",
		IP:              "127.0.0.1",
		Port:            25565,
		QueryPort:       25565,
		MaxPlayers:      20,
		Name:            "Test Server",
		Directory:       "/srv/mc",
		BackupDirectory: "/backups",
		MaxMemoryMB:     4096,
		SetPlayers:      10,
	}
	gs.ServerExecutable.Set("paper-1.21.4-100.jar")

	vars := BuildVarsFromGameServer(gs)

	expected := "paper-1.21.4-100.jar"
	actual, ok := vars["SERVER_EXECUTABLE"]
	if !ok {
		t.Fatal("SERVER_EXECUTABLE key missing from vars")
	}
	if actual != expected {
		t.Errorf("SERVER_EXECUTABLE = %q, want %q", actual, expected)
	}
}

func TestBuildVarsFromGameServer_ServerExecutableEmpty(t *testing.T) {
	gs := &models.GameServer{
		ID:              "test-id",
		IP:              "127.0.0.1",
		Port:            25565,
		QueryPort:       25565,
		MaxPlayers:      20,
		Name:            "Test Server",
		Directory:       "/srv/mc",
		BackupDirectory: "/backups",
		MaxMemoryMB:     4096,
		SetPlayers:      10,
	}

	vars := BuildVarsFromGameServer(gs)

	actual := vars["SERVER_EXECUTABLE"]
	if actual != "" {
		t.Errorf("SERVER_EXECUTABLE = %q, want empty string", actual)
	}
}

func TestResolve_ServerExecutable(t *testing.T) {
	vars := map[string]string{
		"MAX_MEMORY_MB":     "4096",
		"SERVER_EXECUTABLE": "paper-1.21.4-100.jar",
	}

	template := "java -Xmx{{MAX_MEMORY_MB}}M -jar {{SERVER_EXECUTABLE}}"
	result := Resolve(template, vars)

	expected := "java -Xmx4096M -jar paper-1.21.4-100.jar"
	if result != expected {
		t.Errorf("Resolve() = %q, want %q", result, expected)
	}
}

func TestResolveToken(t *testing.T) {
	tests := []struct {
		name  string
		input string
		vars  map[string]string
		want  string
	}{
		{
			name:  "standard replacement",
			input: "{{PORT}}",
			vars:  map[string]string{"PORT": "25565"},
			want:  "25565",
		},
		{
			name:  "embedded replacement",
			input: "server-{{PORT}}.log",
			vars:  map[string]string{"PORT": "25565"},
			want:  "server-25565.log",
		},
		{
			name:  "missing placeholder resolves to empty string",
			input: "server-{{RCON_PORT}}.log",
			vars:  map[string]string{},
			want:  "server-.log",
		},
		{
			name:  "multiple placeholders in one token",
			input: "{{IP}}:{{PORT}}",
			vars: map[string]string{
				"IP":   "127.0.0.1",
				"PORT": "25565",
			},
			want: "127.0.0.1:25565",
		},
		{
			name:  "legacy placeholder still resolves",
			input: "%GAMESERVER_PORT%",
			vars:  map[string]string{"PORT": "25565"},
			want:  "25565",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveToken(tt.input, tt.vars)
			if got != tt.want {
				t.Errorf("ResolveToken() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveTokens(t *testing.T) {
	tokens := []string{"-jar", "{{SERVER_EXECUTABLE}}", "--name=server-{{PORT}}"}
	vars := map[string]string{
		"SERVER_EXECUTABLE": "paper.jar",
		"PORT":              "25565",
	}

	got := ResolveTokens(tokens, vars)
	want := []string{"-jar", "paper.jar", "--name=server-25565"}

	if len(got) != len(want) {
		t.Fatalf("ResolveTokens() length = %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ResolveTokens()[%d] = %q, want %q", i, got[i], want[i])
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
