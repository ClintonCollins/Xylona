package cfgparse

import (
	"errors"
	"strings"
	"testing"
)

func TestPalworldParser(t *testing.T) {
	t.Parallel()

	parser := &PalworldParser{}
	tests := []struct {
		name    string
		input   string
		want    map[string]string
		wantErr error
	}{
		{
			name: "parses quoted and nested settings",
			input: "[/Script/Pal.PalGameWorldSettings]\n" +
				`OptionSettings=(ServerName="Pal, World",CrossplayPlatforms=(Steam,Xbox),` +
				`DenyTechnologyList=("PALBOX","RepairBench"),RESTAPIEnabled=False,ExpRate=1.000000)`,
			want: map[string]string{
				"ServerName":         "Pal, World",
				"CrossplayPlatforms": "Steam,Xbox",
				"DenyTechnologyList": "PALBOX,RepairBench",
				"RESTAPIEnabled":     "false",
				"ExpRate":            "1.000000",
			},
		},
		{
			name:    "requires option settings",
			input:   "[/Script/Pal.PalGameWorldSettings]\n",
			wantErr: ErrPalworldOptionSettingsMissing,
		},
		{
			name:    "rejects unterminated tuple",
			input:   "OptionSettings=(ServerName=\"Palworld\"",
			wantErr: errors.New("no closing parenthesis"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			entries, errParse := parser.Parse([]byte(test.input))
			if test.wantErr != nil {
				if errParse == nil {
					t.Fatalf("Parse() error = nil, want %v", test.wantErr)
				}
				if !errors.Is(errParse, test.wantErr) && !strings.Contains(errParse.Error(), test.wantErr.Error()) {
					t.Fatalf("Parse() error = %v, want %v", errParse, test.wantErr)
				}
				return
			}
			if errParse != nil {
				t.Fatalf("Parse() error = %v", errParse)
			}
			got := make(map[string]string, len(entries))
			for _, entry := range entries {
				got[entry.Key] = entry.Value
			}
			for key, want := range test.want {
				if got[key] != want {
					t.Errorf("Parse() value for %q = %q, want %q", key, got[key], want)
				}
			}
		})
	}
}

func TestPalworldParserWriteRoundTrip(t *testing.T) {
	t.Parallel()

	parser := &PalworldParser{}
	entries := []ConfigEntry{
		{Key: "ServerName", Value: `Xylona "Palworld", Server`},
		{Key: "CrossplayPlatforms", Value: "Steam,Xbox,PS5,Mac"},
		{Key: "DenyTechnologyList", Value: "PALBOX,RepairBench"},
		{Key: "RESTAPIEnabled", Value: "true"},
		{Key: "RESTAPIPort", Value: "8212"},
		{Key: "FutureNestedSetting", Value: "(Alpha,Beta)"},
	}

	output, errWrite := parser.Write(entries)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}
	outputText := string(output)
	for _, want := range []string{
		"[/Script/Pal.PalGameWorldSettings]\n",
		`ServerName="Xylona \"Palworld\", Server"`,
		"CrossplayPlatforms=(Steam,Xbox,PS5,Mac)",
		`DenyTechnologyList=("PALBOX","RepairBench")`,
		"RESTAPIEnabled=True",
		"FutureNestedSetting=(Alpha,Beta)",
	} {
		if !strings.Contains(outputText, want) {
			t.Errorf("Write() output missing %q:\n%s", want, outputText)
		}
	}

	roundTrip, errParse := parser.Parse(output)
	if errParse != nil {
		t.Fatalf("Parse(Write()) error = %v", errParse)
	}
	if len(roundTrip) != len(entries) {
		t.Fatalf("Parse(Write()) entry count = %d, want %d", len(roundTrip), len(entries))
	}
	for index := range entries {
		if roundTrip[index].Key != entries[index].Key || roundTrip[index].Value != entries[index].Value {
			t.Errorf("Parse(Write()) entry[%d] = %#v, want %#v", index, roundTrip[index], entries[index])
		}
	}
}

func TestPalworldParserRegistered(t *testing.T) {
	t.Parallel()

	parser, errGet := GetParser("palworld")
	if errGet != nil {
		t.Fatalf("GetParser() error = %v", errGet)
	}
	if !parser.IsFlat() {
		t.Fatal("GetParser() returned a structured parser, want flat")
	}
	if parser.Flat.Format() != "palworld" {
		t.Fatalf("Format() = %q, want %q", parser.Flat.Format(), "palworld")
	}
}
