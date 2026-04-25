package cfgparse

import (
	"strings"
	"testing"
)

func TestINI_ParseSimple(t *testing.T) {
	input := "[Server]\nport=25565\n"
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Section != "Server" {
		t.Errorf("Section = %q, want %q", e.Section, "Server")
	}
	if e.Key != "port" {
		t.Errorf("Key = %q, want %q", e.Key, "port")
	}
	if e.Value != "25565" {
		t.Errorf("Value = %q, want %q", e.Value, "25565")
	}
}

func TestINI_SectionlessKeys(t *testing.T) {
	input := "name=test\nversion=1\n[Section]\nkey=val\n"
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	if entries[0].Section != "" {
		t.Errorf("entries[0].Section = %q, want empty", entries[0].Section)
	}
	if entries[1].Section != "" {
		t.Errorf("entries[1].Section = %q, want empty", entries[1].Section)
	}
	if entries[2].Section != "Section" {
		t.Errorf("entries[2].Section = %q, want %q", entries[2].Section, "Section")
	}
}

func TestINI_MultipleSections(t *testing.T) {
	input := "[Database]\nhost=localhost\nport=5432\n[App]\nname=myapp\n"
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	tests := []struct {
		idx     int
		section string
		key     string
		value   string
	}{
		{0, "Database", "host", "localhost"},
		{1, "Database", "port", "5432"},
		{2, "App", "name", "myapp"},
	}

	for _, tt := range tests {
		e := entries[tt.idx]
		if e.Section != tt.section {
			t.Errorf("entries[%d].Section = %q, want %q", tt.idx, e.Section, tt.section)
		}
		if e.Key != tt.key {
			t.Errorf("entries[%d].Key = %q, want %q", tt.idx, e.Key, tt.key)
		}
		if e.Value != tt.value {
			t.Errorf("entries[%d].Value = %q, want %q", tt.idx, e.Value, tt.value)
		}
	}
}

func TestINI_CommentPreservation(t *testing.T) {
	input := "# This is a hash comment\nkey1=val1\n; This is a semicolon comment\nkey2=val2\n"
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Comment != "# This is a hash comment" {
		t.Errorf("entries[0].Comment = %q, want %q", entries[0].Comment, "# This is a hash comment")
	}
	if entries[1].Comment != "; This is a semicolon comment" {
		t.Errorf("entries[1].Comment = %q, want %q", entries[1].Comment, "; This is a semicolon comment")
	}

	// Round-trip: write and re-parse.
	output, errWrite := p.Write(entries)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	roundTripped, errRoundTrip := p.Parse(output)
	if errRoundTrip != nil {
		t.Fatalf("round-trip Parse() error = %v", errRoundTrip)
	}
	if len(roundTripped) != 2 {
		t.Fatalf("round-trip expected 2 entries, got %d", len(roundTripped))
	}

	// After round-trip, comments are normalized to # prefix but content is preserved.
	if !strings.Contains(roundTripped[0].Comment, "This is a hash comment") {
		t.Errorf("round-trip entries[0].Comment = %q, want content preserved", roundTripped[0].Comment)
	}
	if !strings.Contains(roundTripped[1].Comment, "This is a semicolon comment") {
		t.Errorf("round-trip entries[1].Comment = %q, want content preserved", roundTripped[1].Comment)
	}
}

func TestINI_DuplicateKeys(t *testing.T) {
	input := "[Server]\nmod=mod_a\nmod=mod_b\nmod=mod_c\n"
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	for i, e := range entries {
		if e.Index != i {
			t.Errorf("entries[%d].Index = %d, want %d", i, e.Index, i)
		}
		if e.Key != "mod" {
			t.Errorf("entries[%d].Key = %q, want %q", i, e.Key, "mod")
		}
	}
}

func TestINI_EmptyFile(t *testing.T) {
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(""))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestINI_WhitespaceHandling(t *testing.T) {
	input := "[Section]\n  key  =  value  \n"
	p := &INIParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	e := entries[0]
	if e.Key != "key" {
		t.Errorf("Key = %q, want %q", e.Key, "key")
	}
	if e.Value != "value" {
		t.Errorf("Value = %q, want %q", e.Value, "value")
	}
}

func TestINI_RegisteredInRegistry(t *testing.T) {
	parser, errGet := GetParser("ini")
	if errGet != nil {
		t.Fatalf("GetParser(\"ini\") error = %v", errGet)
	}
	if !parser.IsFlat() {
		t.Fatal("expected flat parser, got structured")
	}
	if parser.Flat.Format() != "ini" {
		t.Errorf("Format() = %q, want %q", parser.Flat.Format(), "ini")
	}
}
