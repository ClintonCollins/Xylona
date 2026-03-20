package cfgparse

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestYAML_ParseSimple(t *testing.T) {
	input := []byte("name: test\nport: 25565\n")

	p := &YAMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if root.Type != NodeObject {
		t.Fatalf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 2 {
		t.Fatalf("len(root.Children) = %d, want 2", len(root.Children))
	}

	nameNode := root.Children[0]
	if nameNode.Key != "name" {
		t.Errorf("Children[0].Key = %q, want %q", nameNode.Key, "name")
	}
	if nameNode.Value != "test" {
		t.Errorf("Children[0].Value = %q, want %q", nameNode.Value, "test")
	}
	if nameNode.Type != NodeString {
		t.Errorf("Children[0].Type = %v, want NodeString", nameNode.Type)
	}

	portNode := root.Children[1]
	if portNode.Key != "port" {
		t.Errorf("Children[1].Key = %q, want %q", portNode.Key, "port")
	}
	if portNode.Value != "25565" {
		t.Errorf("Children[1].Value = %q, want %q", portNode.Value, "25565")
	}
	if portNode.Type != NodeNumber {
		t.Errorf("Children[1].Type = %v, want NodeNumber", portNode.Type)
	}
}

func TestYAML_ParseNested(t *testing.T) {
	input := []byte("server:\n  host: localhost\n  port: 8080\n")

	p := &YAMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if len(root.Children) != 1 {
		t.Fatalf("len(root.Children) = %d, want 1", len(root.Children))
	}

	server := root.Children[0]
	if server.Key != "server" {
		t.Errorf("server.Key = %q, want %q", server.Key, "server")
	}
	if server.Type != NodeObject {
		t.Fatalf("server.Type = %v, want NodeObject", server.Type)
	}
	if len(server.Children) != 2 {
		t.Fatalf("len(server.Children) = %d, want 2", len(server.Children))
	}

	host := server.Children[0]
	if host.Key != "host" || host.Value != "localhost" || host.Type != NodeString {
		t.Errorf("host = {Key:%q Value:%q Type:%v}, want {Key:host Value:localhost Type:NodeString}",
			host.Key, host.Value, host.Type)
	}

	port := server.Children[1]
	if port.Key != "port" || port.Value != "8080" || port.Type != NodeNumber {
		t.Errorf("port = {Key:%q Value:%q Type:%v}, want {Key:port Value:8080 Type:NodeNumber}",
			port.Key, port.Value, port.Type)
	}
}

func TestYAML_ParseList(t *testing.T) {
	input := []byte("plugins:\n  - WorldEdit\n  - Essentials\n  - Vault\n")

	p := &YAMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if len(root.Children) != 1 {
		t.Fatalf("len(root.Children) = %d, want 1", len(root.Children))
	}

	plugins := root.Children[0]
	if plugins.Key != "plugins" {
		t.Errorf("plugins.Key = %q, want %q", plugins.Key, "plugins")
	}
	if plugins.Type != NodeArray {
		t.Fatalf("plugins.Type = %v, want NodeArray", plugins.Type)
	}
	if len(plugins.Children) != 3 {
		t.Fatalf("len(plugins.Children) = %d, want 3", len(plugins.Children))
	}

	wantValues := []string{"WorldEdit", "Essentials", "Vault"}
	for i, want := range wantValues {
		child := plugins.Children[i]
		if child.Key != "" {
			t.Errorf("plugins.Children[%d].Key = %q, want empty", i, child.Key)
		}
		if child.Value != want {
			t.Errorf("plugins.Children[%d].Value = %q, want %q", i, child.Value, want)
		}
		if child.Type != NodeString {
			t.Errorf("plugins.Children[%d].Type = %v, want NodeString", i, child.Type)
		}
	}
}

func TestYAML_ParseBoolNull(t *testing.T) {
	input := []byte("enabled: true\ndisabled: false\nnothing: null\n")

	p := &YAMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if len(root.Children) != 3 {
		t.Fatalf("len(root.Children) = %d, want 3", len(root.Children))
	}

	tests := []struct {
		name     string
		wantKey  string
		wantVal  string
		wantType NodeType
	}{
		{name: "true bool", wantKey: "enabled", wantVal: "true", wantType: NodeBool},
		{name: "false bool", wantKey: "disabled", wantVal: "false", wantType: NodeBool},
		{name: "null value", wantKey: "nothing", wantVal: "", wantType: NodeNull},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			child := root.Children[i]
			if child.Key != tt.wantKey {
				t.Errorf("Key = %q, want %q", child.Key, tt.wantKey)
			}
			if child.Value != tt.wantVal {
				t.Errorf("Value = %q, want %q", child.Value, tt.wantVal)
			}
			if child.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", child.Type, tt.wantType)
			}
		})
	}
}

func TestYAML_CommentPreservation(t *testing.T) {
	input := []byte("# server config\nhost: localhost\nport: 8080 # default port\n")

	p := &YAMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	// Check that at least one node captured a comment.
	foundComment := false
	for _, child := range root.Children {
		if child.Comment != "" {
			foundComment = true
			break
		}
	}
	if !foundComment {
		t.Error("no comments preserved in any child node")
	}

	// Round-trip and verify comments survive.
	output, errWrite := p.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	// Re-parse to verify comments are still present.
	var doc yaml.Node
	errRe := yaml.Unmarshal(output, &doc)
	if errRe != nil {
		t.Fatalf("re-parse error = %v", errRe)
	}

	foundInOutput := false
	walkYAMLNode(&doc, func(yn *yaml.Node) {
		if yn.HeadComment != "" || yn.LineComment != "" || yn.FootComment != "" {
			foundInOutput = true
		}
	})
	if !foundInOutput {
		t.Error("comments not preserved in round-trip output")
	}
}

// walkYAMLNode recursively visits all yaml.Nodes.
func walkYAMLNode(yn *yaml.Node, fn func(*yaml.Node)) {
	fn(yn)
	for _, child := range yn.Content {
		walkYAMLNode(child, fn)
	}
}

func TestYAML_RoundTrip(t *testing.T) {
	input := []byte("name: test\nport: 25565\nenabled: true\nitems:\n    - one\n    - two\n")

	p := &YAMLParser{}
	root, errParse := p.Parse(input)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	output, errWrite := p.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	// Re-parse the output and compare structure.
	root2, errReparse := p.Parse(output)
	if errReparse != nil {
		t.Fatalf("re-Parse() error = %v", errReparse)
	}

	assertConfigNodesEqual(t, "root", root, root2)
}

func assertConfigNodesEqual(t *testing.T, path string, a, b *ConfigNode) {
	t.Helper()
	if a.Key != b.Key {
		t.Errorf("%s: Key mismatch: %q vs %q", path, a.Key, b.Key)
	}
	if a.Value != b.Value {
		t.Errorf("%s: Value mismatch: %q vs %q", path, a.Value, b.Value)
	}
	if a.Type != b.Type {
		t.Errorf("%s: Type mismatch: %v vs %v", path, a.Type, b.Type)
	}
	if len(a.Children) != len(b.Children) {
		t.Fatalf("%s: Children length mismatch: %d vs %d", path, len(a.Children), len(b.Children))
	}
	for i := range a.Children {
		childPath := path + "." + a.Children[i].Key
		if childPath == path+"." {
			childPath = path + "[" + string(rune('0'+i)) + "]"
		}
		assertConfigNodesEqual(t, childPath, a.Children[i], b.Children[i])
	}
}

func TestYAML_EmptyDocument(t *testing.T) {
	inputs := [][]byte{
		{},
		[]byte(""),
		[]byte("\n"),
		[]byte("---\n"),
	}

	p := &YAMLParser{}
	for _, input := range inputs {
		root, errParse := p.Parse(input)
		if errParse != nil {
			t.Fatalf("Parse(%q) error = %v", input, errParse)
		}
		if root == nil {
			t.Fatalf("Parse(%q) returned nil root", input)
		}
		if root.Type != NodeObject {
			t.Errorf("Parse(%q) root.Type = %v, want NodeObject", input, root.Type)
		}
	}
}

func TestYAML_RegisteredInRegistry(t *testing.T) {
	parser, errGet := GetParser("yaml")
	if errGet != nil {
		t.Fatalf("GetParser(\"yaml\") error = %v", errGet)
	}
	if parser.Structured == nil {
		t.Fatal("GetParser(\"yaml\") returned nil Structured parser")
	}
	if parser.IsFlat() {
		t.Error("YAML parser should not be flat")
	}
	if parser.Structured.Format() != "yaml" {
		t.Errorf("Format() = %q, want %q", parser.Structured.Format(), "yaml")
	}
}
