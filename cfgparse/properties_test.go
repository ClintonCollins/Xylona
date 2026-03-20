package cfgparse

import (
	"strings"
	"testing"
)

func TestProperties_ParseSimple(t *testing.T) {
	input := "motd=Hello World\nport=25565\n"
	p := &PropertiesParser{}
	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 2 {
		t.Fatalf("Parse() returned %d entries, want 2", len(entries))
	}
	if entries[0].Key != "motd" || entries[0].Value != "Hello World" {
		t.Errorf("entry[0] = %q=%q, want motd=Hello World", entries[0].Key, entries[0].Value)
	}
	if entries[1].Key != "port" || entries[1].Value != "25565" {
		t.Errorf("entry[1] = %q=%q, want port=25565", entries[1].Key, entries[1].Value)
	}
}

func TestProperties_CommentPreservation(t *testing.T) {
	input := "# Server MOTD\nmotd=Hello\n"
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("Parse() returned %d entries, want 1", len(entries))
	}
	if entries[0].Comment != "Server MOTD" {
		t.Errorf("entry[0].Comment = %q, want %q", entries[0].Comment, "Server MOTD")
	}

	output, errWrite := p.Write(entries)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}
	result := string(output)
	if !strings.Contains(result, "# Server MOTD\n") {
		t.Errorf("Write() output missing comment, got:\n%s", result)
	}
	if !strings.Contains(result, "motd=Hello\n") {
		t.Errorf("Write() output missing key=value, got:\n%s", result)
	}
}

func TestProperties_KeyOrdering(t *testing.T) {
	input := "z-key=last\na-key=first\nm-key=middle\n"
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	output, errWrite := p.Write(entries)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	want := "z-key=last\na-key=first\nm-key=middle\n"
	if string(output) != want {
		t.Errorf("Write() output = %q, want %q", string(output), want)
	}
}

func TestProperties_DuplicateKeys(t *testing.T) {
	input := "key=value1\nkey=value2\n"
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 2 {
		t.Fatalf("Parse() returned %d entries, want 2", len(entries))
	}
	if entries[0].Index != 0 {
		t.Errorf("entry[0].Index = %d, want 0", entries[0].Index)
	}
	if entries[1].Index != 1 {
		t.Errorf("entry[1].Index = %d, want 1", entries[1].Index)
	}
}

func TestProperties_EmptyFile(t *testing.T) {
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(""))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 0 {
		t.Fatalf("Parse() returned %d entries, want 0", len(entries))
	}
}

func TestProperties_CommentsOnlyFile(t *testing.T) {
	input := "# Just a comment\n! Another comment\n"
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("Parse() returned %d entries, want 1", len(entries))
	}
	if entries[0].Key != "" {
		t.Errorf("entry[0].Key = %q, want empty", entries[0].Key)
	}
	if entries[0].Comment != "Just a comment\nAnother comment" {
		t.Errorf("entry[0].Comment = %q, want %q", entries[0].Comment, "Just a comment\nAnother comment")
	}
}

func TestProperties_WhitespaceHandling(t *testing.T) {
	input := "key = value\n"
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("Parse() returned %d entries, want 1", len(entries))
	}
	if entries[0].Key != "key" {
		t.Errorf("entry[0].Key = %q, want %q", entries[0].Key, "key")
	}
	if entries[0].Value != "value" {
		t.Errorf("entry[0].Value = %q, want %q", entries[0].Value, "value")
	}
}

func TestProperties_ColonSeparator(t *testing.T) {
	input := "key: value\n"
	p := &PropertiesParser{}

	entries, errParse := p.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("Parse() returned %d entries, want 1", len(entries))
	}
	if entries[0].Key != "key" {
		t.Errorf("entry[0].Key = %q, want %q", entries[0].Key, "key")
	}
	if entries[0].Value != "value" {
		t.Errorf("entry[0].Value = %q, want %q", entries[0].Value, "value")
	}
}

func TestProperties_RegisteredInRegistry(t *testing.T) {
	parser, errGet := GetParser("properties")
	if errGet != nil {
		t.Fatalf("GetParser(properties) error = %v", errGet)
	}
	if !parser.IsFlat() {
		t.Fatal("expected flat parser, got structured")
	}
	if parser.Flat.Format() != "properties" {
		t.Errorf("Format() = %q, want %q", parser.Flat.Format(), "properties")
	}
}
