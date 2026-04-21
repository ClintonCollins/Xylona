package cfgparse

import (
	"strings"
	"testing"
)

func TestCommandCFG_Parse(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []ConfigEntry
	}{
		{
			name:  "source style quoted values",
			input: "// identity\nhostname \"Xylona Server\"\nsv_password \"secret\"\n",
			want: []ConfigEntry{
				{Key: "hostname", Value: "Xylona Server", Index: 0, Comment: "identity"},
				{Key: "sv_password", Value: "secret", Index: 0},
			},
		},
		{
			name:  "set commands use target key",
			input: "set sv_hostname \"FiveM Server\"\nsets sv_projectName \"Roleplay\"\nsetr locale en-US\n",
			want: []ConfigEntry{
				{Section: "set", Key: "sv_hostname", Value: "FiveM Server", Index: 0},
				{Section: "sets", Key: "sv_projectName", Value: "Roleplay", Index: 0},
				{Section: "setr", Key: "locale", Value: "en-US", Index: 0},
			},
		},
		{
			name:  "equals and repeated commands",
			input: "port=7777\nensure mapmanager\nensure chat\n# trailing\n",
			want: []ConfigEntry{
				{Section: "=", Key: "port", Value: "7777", Index: 0},
				{Key: "ensure", Value: "mapmanager", Index: 0},
				{Key: "ensure", Value: "chat", Index: 1},
				{Comment: "trailing"},
			},
		},
		{
			name:  "blank lines and semicolon comments",
			input: "\n; access\nMaxPlayers 24\n\nPassword \"\"\n",
			want: []ConfigEntry{
				{Key: "MaxPlayers", Value: "24", Index: 0, Comment: "access"},
				{Key: "Password", Value: "", Index: 0},
			},
		},
	}

	parser := &CommandCFGParser{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, errParse := parser.Parse([]byte(tt.input))
			if errParse != nil {
				t.Fatalf("Parse() error = %v", errParse)
			}
			if len(entries) != len(tt.want) {
				t.Fatalf("Parse() entries = %#v, want %#v", entries, tt.want)
			}
			for i := range entries {
				if entries[i] != tt.want[i] {
					t.Fatalf("entries[%d] = %#v, want %#v", i, entries[i], tt.want[i])
				}
			}
		})
	}
}

func TestCommandCFG_WriteRoundTrip(t *testing.T) {
	parser := &CommandCFGParser{}
	entries := []ConfigEntry{
		{Comment: "identity", Key: "hostname", Value: "Xylona Server"},
		{Section: "set", Key: "sv_maxclients", Value: "48"},
		{Section: "=", Key: "port", Value: "30120"},
		{Key: "ensure", Value: "mapmanager", Index: 0},
		{Key: "ensure", Value: "chat", Index: 1},
	}

	output, errWrite := parser.Write(entries)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}
	outputString := string(output)
	for _, want := range []string{
		"// identity",
		"hostname \"Xylona Server\"",
		"set sv_maxclients 48",
		"port=30120",
		"ensure mapmanager",
		"ensure chat",
	} {
		if !strings.Contains(outputString, want) {
			t.Fatalf("Write() output missing %q:\n%s", want, outputString)
		}
	}

	roundTrip, errParse := parser.Parse(output)
	if errParse != nil {
		t.Fatalf("round-trip Parse() error = %v", errParse)
	}
	if len(roundTrip) != len(entries) {
		t.Fatalf("round-trip entries = %#v, want len %d", roundTrip, len(entries))
	}
	if roundTrip[0].Comment != "identity" || roundTrip[0].Value != "Xylona Server" {
		t.Fatalf("round-trip first entry = %#v", roundTrip[0])
	}
	if roundTrip[3].Index != 0 || roundTrip[4].Index != 1 {
		t.Fatalf("round-trip repeated indexes = %d, %d", roundTrip[3].Index, roundTrip[4].Index)
	}
}

func TestCommandCFG_RegisteredInRegistry(t *testing.T) {
	parser, errGet := GetParser("commandcfg")
	if errGet != nil {
		t.Fatalf("GetParser(\"commandcfg\") error = %v", errGet)
	}
	if !parser.IsFlat() {
		t.Fatal("expected flat parser, got structured")
	}
	if parser.Flat.Format() != "commandcfg" {
		t.Errorf("Format() = %q, want %q", parser.Flat.Format(), "commandcfg")
	}
}
