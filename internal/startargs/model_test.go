package startargs

import "testing"

func TestIsValidManagedSource(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want bool
	}{
		{name: "game server port", key: "game_server.port", want: true},
		{name: "game server port plus one", key: "game_server.port_plus_1", want: true},
		{name: "game server port plus two", key: "game_server.port_plus_2", want: true},
		{name: "game server query port plus one", key: "game_server.query_port_plus_1", want: true},
		{name: "game server query port", key: "game_server.query_port", want: true},
		{name: "game server ip", key: "game_server.ip", want: true},
		{name: "game server memory", key: "game_server.max_memory_mb", want: true},
		{name: "game server max players", key: "game_server.max_players", want: true},
		{name: "game server name", key: "game_server.server_name", want: true},
		{name: "server executable", key: "server_executable", want: true},
		{name: "Steam GSLT", key: "steam_gslt", want: true},
		{name: "empty key", key: "", want: false},
		{name: "unsupported key", key: "game_server.server_id", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidManagedSource(tt.key)
			if got != tt.want {
				t.Errorf("IsValidManagedSource() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestParseTemplate(t *testing.T) {
	t.Run("empty template returns nil", func(t *testing.T) {
		got, errParse := ParseTemplate("")
		if errParse != nil {
			t.Fatalf("ParseTemplate() error = %v", errParse)
		}
		if got != nil {
			t.Errorf("ParseTemplate() = %#v, want nil", got)
		}
	})

	t.Run("valid template parses", func(t *testing.T) {
		templateJSON := `[{"id":"01TEST","order":1,"ownership":"editable","tokens":["-Xmx2G"],"label":"Max heap"}]`

		got, errParse := ParseTemplate(templateJSON)
		if errParse != nil {
			t.Fatalf("ParseTemplate() error = %v", errParse)
		}
		if len(got) != 1 {
			t.Fatalf("ParseTemplate() len = %d, want 1", len(got))
		}
		if got[0].ID != "01TEST" {
			t.Errorf("ParseTemplate()[0].ID = %q, want %q", got[0].ID, "01TEST")
		}
		if got[0].Ownership != OwnershipEditable {
			t.Errorf("ParseTemplate()[0].Ownership = %q, want %q", got[0].Ownership, OwnershipEditable)
		}
	})
}

func TestParsePatches(t *testing.T) {
	t.Run("empty patches returns nil", func(t *testing.T) {
		got, errParse := ParsePatches("")
		if errParse != nil {
			t.Fatalf("ParsePatches() error = %v", errParse)
		}
		if got != nil {
			t.Errorf("ParsePatches() = %#v, want nil", got)
		}
	})

	t.Run("invalid json returns error", func(t *testing.T) {
		_, errParse := ParsePatches(`[{`)
		if errParse == nil {
			t.Fatal("ParsePatches() error = nil, want error")
		}
	})
}

func TestParseBlocklist(t *testing.T) {
	t.Run("empty blocklist returns nil", func(t *testing.T) {
		got, errParse := ParseBlocklist("")
		if errParse != nil {
			t.Fatalf("ParseBlocklist() error = %v", errParse)
		}
		if got != nil {
			t.Errorf("ParseBlocklist() = %#v, want nil", got)
		}
	})

	t.Run("valid blocklist parses", func(t *testing.T) {
		blocklistJSON := `[{"pattern":"-agentlib:","reason":"debug agent"}]`

		got, errParse := ParseBlocklist(blocklistJSON)
		if errParse != nil {
			t.Fatalf("ParseBlocklist() error = %v", errParse)
		}
		if len(got) != 1 {
			t.Fatalf("ParseBlocklist() len = %d, want 1", len(got))
		}
		if got[0].Pattern != "-agentlib:" {
			t.Errorf("ParseBlocklist()[0].Pattern = %q, want %q", got[0].Pattern, "-agentlib:")
		}
	})
}
