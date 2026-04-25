package cfgparse

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestJSON_ParseSimple(t *testing.T) {
	input := []byte(`{"name":"test","port":25565}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if root.Type != NodeObject {
		t.Fatalf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 2 {
		t.Fatalf("root.Children count = %d, want 2", len(root.Children))
	}

	childMap := make(map[string]*ConfigNode, len(root.Children))
	for _, c := range root.Children {
		childMap[c.Key] = c
	}

	nameNode, ok := childMap["name"]
	if !ok {
		t.Fatal("missing child 'name'")
	}
	if nameNode.Type != NodeString {
		t.Errorf("name.Type = %v, want NodeString", nameNode.Type)
	}
	if nameNode.Value != "test" {
		t.Errorf("name.Value = %q, want %q", nameNode.Value, "test")
	}

	portNode, ok := childMap["port"]
	if !ok {
		t.Fatal("missing child 'port'")
	}
	if portNode.Type != NodeNumber {
		t.Errorf("port.Type = %v, want NodeNumber", portNode.Type)
	}
	if portNode.Value != "25565" {
		t.Errorf("port.Value = %q, want %q", portNode.Value, "25565")
	}
}

func TestJSON_ParseNested(t *testing.T) {
	input := []byte(`{"server":{"host":"localhost","port":8080}}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if root.Type != NodeObject {
		t.Fatalf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root.Children count = %d, want 1", len(root.Children))
	}

	server := root.Children[0]
	if server.Key != "server" {
		t.Fatalf("child.Key = %q, want %q", server.Key, "server")
	}
	if server.Type != NodeObject {
		t.Fatalf("server.Type = %v, want NodeObject", server.Type)
	}
	if len(server.Children) != 2 {
		t.Fatalf("server.Children count = %d, want 2", len(server.Children))
	}

	childMap := make(map[string]*ConfigNode, len(server.Children))
	for _, c := range server.Children {
		childMap[c.Key] = c
	}

	hostNode, ok := childMap["host"]
	if !ok {
		t.Fatal("missing child 'host'")
	}
	if hostNode.Type != NodeString {
		t.Errorf("host.Type = %v, want NodeString", hostNode.Type)
	}
	if hostNode.Value != "localhost" {
		t.Errorf("host.Value = %q, want %q", hostNode.Value, "localhost")
	}

	portNode, ok := childMap["port"]
	if !ok {
		t.Fatal("missing child 'port'")
	}
	if portNode.Type != NodeNumber {
		t.Errorf("port.Type = %v, want NodeNumber", portNode.Type)
	}
	if portNode.Value != "8080" {
		t.Errorf("port.Value = %q, want %q", portNode.Value, "8080")
	}
}

func TestJSON_ParseArray(t *testing.T) {
	input := []byte(`{"tags":["alpha","beta","gamma"]}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if len(root.Children) != 1 {
		t.Fatalf("root.Children count = %d, want 1", len(root.Children))
	}

	tags := root.Children[0]
	if tags.Key != "tags" {
		t.Fatalf("child.Key = %q, want %q", tags.Key, "tags")
	}
	if tags.Type != NodeArray {
		t.Fatalf("tags.Type = %v, want NodeArray", tags.Type)
	}
	if len(tags.Children) != 3 {
		t.Fatalf("tags.Children count = %d, want 3", len(tags.Children))
	}

	expected := []string{"alpha", "beta", "gamma"}
	for i, want := range expected {
		child := tags.Children[i]
		if child.Key != "" {
			t.Errorf("tags.Children[%d].Key = %q, want empty", i, child.Key)
		}
		if child.Type != NodeString {
			t.Errorf("tags.Children[%d].Type = %v, want NodeString", i, child.Type)
		}
		if child.Value != want {
			t.Errorf("tags.Children[%d].Value = %q, want %q", i, child.Value, want)
		}
	}
}

func TestJSON_ParseNull(t *testing.T) {
	input := []byte(`{"value":null}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if len(root.Children) != 1 {
		t.Fatalf("root.Children count = %d, want 1", len(root.Children))
	}

	child := root.Children[0]
	if child.Key != "value" {
		t.Errorf("child.Key = %q, want %q", child.Key, "value")
	}
	if child.Type != NodeNull {
		t.Errorf("child.Type = %v, want NodeNull", child.Type)
	}
	if child.Value != "" {
		t.Errorf("child.Value = %q, want empty", child.Value)
	}
}

func TestJSON_ParseBoolean(t *testing.T) {
	input := []byte(`{"enabled":true,"debug":false}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	childMap := make(map[string]*ConfigNode, len(root.Children))
	for _, c := range root.Children {
		childMap[c.Key] = c
	}

	enabledNode, ok := childMap["enabled"]
	if !ok {
		t.Fatal("missing child 'enabled'")
	}
	if enabledNode.Type != NodeBool {
		t.Errorf("enabled.Type = %v, want NodeBool", enabledNode.Type)
	}
	if enabledNode.Value != "true" {
		t.Errorf("enabled.Value = %q, want %q", enabledNode.Value, "true")
	}

	debugNode, ok := childMap["debug"]
	if !ok {
		t.Fatal("missing child 'debug'")
	}
	if debugNode.Type != NodeBool {
		t.Errorf("debug.Type = %v, want NodeBool", debugNode.Type)
	}
	if debugNode.Value != "false" {
		t.Errorf("debug.Value = %q, want %q", debugNode.Value, "false")
	}
}

func TestJSON_RoundTrip(t *testing.T) {
	input := []byte(`{"enabled":true,"name":"test","port":25565,"value":null}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	output, errWrite := p.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	// Re-parse to compare semantically (key order may differ).
	var orig any
	errOriginal := json.Unmarshal(input, &orig)
	if errOriginal != nil {
		t.Fatalf("json.Unmarshal(input) error = %v", errOriginal)
	}

	var roundTripped any
	errRoundTrip := json.Unmarshal(output, &roundTripped)
	if errRoundTrip != nil {
		t.Fatalf("json.Unmarshal(output) error = %v", errRoundTrip)
	}

	origJSON, _ := json.Marshal(orig)
	rtJSON, _ := json.Marshal(roundTripped)
	if string(origJSON) != string(rtJSON) {
		t.Errorf("round-trip mismatch:\n  original:     %s\n  round-tripped: %s", origJSON, rtJSON)
	}
}

func TestJSON_WriteIndentation(t *testing.T) {
	input := []byte(`{"server":{"port":8080}}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	output, errWrite := p.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	outputStr := string(output)

	// Check 2-space indentation is present.
	if !strings.Contains(outputStr, "  ") {
		t.Error("output does not contain 2-space indentation")
	}

	// Check trailing newline.
	if !strings.HasSuffix(outputStr, "\n") {
		t.Error("output does not end with newline")
	}

	// Check LF, not CRLF.
	if strings.Contains(outputStr, "\r\n") {
		t.Error("output contains CRLF line endings, expected LF")
	}

	// Verify nested key is indented with exactly 4 spaces (2 levels).
	if !strings.Contains(outputStr, "    \"port\"") {
		t.Errorf("expected 4-space indent for nested key, got:\n%s", outputStr)
	}
}

func TestJSON_EmptyObject(t *testing.T) {
	input := []byte(`{}`)
	p := &JSONParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if root.Type != NodeObject {
		t.Errorf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 0 {
		t.Errorf("root.Children count = %d, want 0", len(root.Children))
	}
}

func TestJSON_RegisteredInRegistry(t *testing.T) {
	parser, errGet := GetParser("json")
	if errGet != nil {
		t.Fatalf("GetParser(json) error = %v", errGet)
	}
	if parser.Structured == nil {
		t.Fatal("GetParser(json) returned nil Structured parser")
	}
	if parser.Structured.Format() != "json" {
		t.Errorf("Format() = %q, want %q", parser.Structured.Format(), "json")
	}
}
