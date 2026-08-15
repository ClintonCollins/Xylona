package cfgparse

import (
	"strconv"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

func TestTOML_ParseSimple(t *testing.T) {
	input := []byte("name = \"test\"\nport = 25565\n")
	p := &TOMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error: %v", errParse)
	}
	if root.Type != NodeObject {
		t.Fatalf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 2 {
		t.Fatalf("len(root.Children) = %d, want 2", len(root.Children))
	}

	nameNode := findChild(root, "name")
	if nameNode == nil {
		t.Fatal("missing child 'name'")
	}
	if nameNode.Type != NodeString {
		t.Errorf("name.Type = %v, want NodeString", nameNode.Type)
	}
	if nameNode.Value != "test" {
		t.Errorf("name.Value = %q, want %q", nameNode.Value, "test")
	}

	portNode := findChild(root, "port")
	if portNode == nil {
		t.Fatal("missing child 'port'")
	}
	if portNode.Type != NodeNumber {
		t.Errorf("port.Type = %v, want NodeNumber", portNode.Type)
	}
	if portNode.Value != "25565" {
		t.Errorf("port.Value = %q, want %q", portNode.Value, "25565")
	}
}

func TestTOML_ParseNested(t *testing.T) {
	input := []byte("[section]\nkey = \"value\"\ncount = 42\n")
	p := &TOMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error: %v", errParse)
	}

	section := findChild(root, "section")
	if section == nil {
		t.Fatal("missing child 'section'")
	}
	if section.Type != NodeObject {
		t.Fatalf("section.Type = %v, want NodeObject", section.Type)
	}

	keyNode := findChild(section, "key")
	if keyNode == nil {
		t.Fatal("missing child 'key' in section")
	}
	if keyNode.Value != "value" {
		t.Errorf("key.Value = %q, want %q", keyNode.Value, "value")
	}

	countNode := findChild(section, "count")
	if countNode == nil {
		t.Fatal("missing child 'count' in section")
	}
	if countNode.Type != NodeNumber {
		t.Errorf("count.Type = %v, want NodeNumber", countNode.Type)
	}
	if countNode.Value != "42" {
		t.Errorf("count.Value = %q, want %q", countNode.Value, "42")
	}
}

func TestTOML_ParseArray(t *testing.T) {
	input := []byte("tags = [\"alpha\", \"beta\", \"gamma\"]\n")
	p := &TOMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error: %v", errParse)
	}

	tags := findChild(root, "tags")
	if tags == nil {
		t.Fatal("missing child 'tags'")
	}
	if tags.Type != NodeArray {
		t.Fatalf("tags.Type = %v, want NodeArray", tags.Type)
	}
	if len(tags.Children) != 3 {
		t.Fatalf("len(tags.Children) = %d, want 3", len(tags.Children))
	}

	expected := []string{"alpha", "beta", "gamma"}
	for i, want := range expected {
		child := tags.Children[i]
		if child.Type != NodeString {
			t.Errorf("tags[%d].Type = %v, want NodeString", i, child.Type)
		}
		if child.Value != want {
			t.Errorf("tags[%d].Value = %q, want %q", i, child.Value, want)
		}
	}
}

func TestTOML_ParseBoolean(t *testing.T) {
	input := []byte("enabled = true\ndebug = false\n")
	p := &TOMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error: %v", errParse)
	}

	enabled := findChild(root, "enabled")
	if enabled == nil {
		t.Fatal("missing child 'enabled'")
	}
	if enabled.Type != NodeBool {
		t.Errorf("enabled.Type = %v, want NodeBool", enabled.Type)
	}
	if enabled.Value != "true" {
		t.Errorf("enabled.Value = %q, want %q", enabled.Value, "true")
	}

	debug := findChild(root, "debug")
	if debug == nil {
		t.Fatal("missing child 'debug'")
	}
	if debug.Type != NodeBool {
		t.Errorf("debug.Type = %v, want NodeBool", debug.Type)
	}
	if debug.Value != "false" {
		t.Errorf("debug.Value = %q, want %q", debug.Value, "false")
	}
}

func TestTOML_RoundTrip(t *testing.T) {
	input := []byte("enabled = true\nname = \"test\"\nport = 25565\n")
	p := &TOMLParser{}

	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error: %v", errParse)
	}

	output, errWrite := p.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error: %v", errWrite)
	}

	// Re-parse the output and compare structure.
	var original map[string]any
	errOriginal := toml.Unmarshal(input, &original)
	if errOriginal != nil {
		t.Fatalf("Unmarshal original: %v", errOriginal)
	}

	var roundTripped map[string]any
	errRoundTrip := toml.Unmarshal(output, &roundTripped)
	if errRoundTrip != nil {
		t.Fatalf("Unmarshal round-tripped: %v", errRoundTrip)
	}

	// Check that all keys and values match.
	for k, want := range original {
		got, ok := roundTripped[k]
		if !ok {
			t.Errorf("round-tripped missing key %q", k)
			continue
		}
		if fmtV(want) != fmtV(got) {
			t.Errorf("key %q: got %v, want %v", k, got, want)
		}
	}
}

func TestTOML_EmptyDocument(t *testing.T) {
	p := &TOMLParser{}
	root, errParse := p.Parse([]byte(""))
	if errParse != nil {
		t.Fatalf("Parse() error: %v", errParse)
	}
	if root.Type != NodeObject {
		t.Errorf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 0 {
		t.Errorf("len(root.Children) = %d, want 0", len(root.Children))
	}
}

func TestTOML_RegisteredInRegistry(t *testing.T) {
	p, errGet := GetParser("toml")
	if errGet != nil {
		t.Fatalf("GetParser(toml) error: %v", errGet)
	}
	if p.Structured == nil {
		t.Fatal("expected structured parser, got nil")
	}
	if p.Flat != nil {
		t.Error("expected Flat to be nil for TOML parser")
	}

	tomlParser, errGet := GetParser("toml")
	if errGet != nil {
		t.Fatalf("GetParser(toml) error = %v", errGet)
	}
	if tomlParser.Structured == nil {
		t.Fatal("GetParser(toml) returned no structured parser")
	}
}

// findChild returns the first child with the given key, or nil.
func findChild(node *ConfigNode, key string) *ConfigNode {
	for _, c := range node.Children {
		if c.Key == key {
			return c
		}
	}
	return nil
}

// fmtV formats a value for comparison in round-trip tests.
func fmtV(v any) string {
	return toString(v)
}

func toString(v any) string {
	switch val := v.(type) {
	case string:
		return val
	case bool:
		if val {
			return "true"
		}
		return "false"
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return ""
	}
}
