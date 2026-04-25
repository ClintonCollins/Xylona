package cfgparse

import (
	"strings"
	"testing"
)

func TestXML_Elements_ParseSimple(t *testing.T) {
	input := `<server><port>25565</port></server>`

	parser := NewXMLParser(XMLKeyMode{Mode: "elements"})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if root.Key != "server" {
		t.Errorf("root.Key = %q, want %q", root.Key, "server")
	}
	if root.Type != NodeObject {
		t.Errorf("root.Type = %v, want NodeObject", root.Type)
	}
	if len(root.Children) != 1 {
		t.Fatalf("root.Children count = %d, want 1", len(root.Children))
	}

	port := root.Children[0]
	if port.Key != "port" {
		t.Errorf("child.Key = %q, want %q", port.Key, "port")
	}
	if port.Value != "25565" {
		t.Errorf("child.Value = %q, want %q", port.Value, "25565")
	}
	if port.Type != NodeString {
		t.Errorf("child.Type = %v, want NodeString", port.Type)
	}
}

func TestXML_Elements_ParseAttributes(t *testing.T) {
	input := `<server id="main"><port>25565</port></server>`

	parser := NewXMLParser(XMLKeyMode{Mode: "elements"})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if root.Key != "server" {
		t.Errorf("root.Key = %q, want %q", root.Key, "server")
	}
	if len(root.Children) != 2 {
		t.Fatalf("root.Children count = %d, want 2", len(root.Children))
	}

	attrNode := root.Children[0]
	if attrNode.Key != "@id" {
		t.Errorf("attr.Key = %q, want %q", attrNode.Key, "@id")
	}
	if attrNode.Value != "main" {
		t.Errorf("attr.Value = %q, want %q", attrNode.Value, "main")
	}
	if attrNode.Type != NodeString {
		t.Errorf("attr.Type = %v, want NodeString", attrNode.Type)
	}

	portNode := root.Children[1]
	if portNode.Key != "port" {
		t.Errorf("port.Key = %q, want %q", portNode.Key, "port")
	}
	if portNode.Value != "25565" {
		t.Errorf("port.Value = %q, want %q", portNode.Value, "25565")
	}
}

func TestXML_Elements_ParseNested(t *testing.T) {
	input := `<root><level1><level2><level3>deep</level3></level2></level1></root>`

	parser := NewXMLParser(XMLKeyMode{Mode: "elements"})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if root.Key != "root" {
		t.Fatalf("root.Key = %q, want %q", root.Key, "root")
	}
	if len(root.Children) != 1 {
		t.Fatalf("root.Children count = %d, want 1", len(root.Children))
	}

	l1 := root.Children[0]
	if l1.Key != "level1" {
		t.Errorf("level1.Key = %q, want %q", l1.Key, "level1")
	}
	if len(l1.Children) != 1 {
		t.Fatalf("level1.Children count = %d, want 1", len(l1.Children))
	}

	l2 := l1.Children[0]
	if l2.Key != "level2" {
		t.Errorf("level2.Key = %q, want %q", l2.Key, "level2")
	}
	if len(l2.Children) != 1 {
		t.Fatalf("level2.Children count = %d, want 1", len(l2.Children))
	}

	l3 := l2.Children[0]
	if l3.Key != "level3" {
		t.Errorf("level3.Key = %q, want %q", l3.Key, "level3")
	}
	if l3.Value != "deep" {
		t.Errorf("level3.Value = %q, want %q", l3.Value, "deep")
	}
	if l3.Type != NodeString {
		t.Errorf("level3.Type = %v, want NodeString", l3.Type)
	}
}

func TestXML_Elements_SelfClosing(t *testing.T) {
	input := `<root><empty/></root>`

	parser := NewXMLParser(XMLKeyMode{Mode: "elements"})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if len(root.Children) != 1 {
		t.Fatalf("root.Children count = %d, want 1", len(root.Children))
	}

	empty := root.Children[0]
	if empty.Key != "empty" {
		t.Errorf("empty.Key = %q, want %q", empty.Key, "empty")
	}
	if empty.Value != "" {
		t.Errorf("empty.Value = %q, want empty string", empty.Value)
	}
	if empty.Type != NodeString {
		t.Errorf("empty.Type = %v, want NodeString", empty.Type)
	}
}

func TestXML_Elements_RoundTrip(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<server>
  <port>25565</port>
  <motd>Hello World</motd>
  <empty/>
</server>
`

	parser := NewXMLParser(XMLKeyMode{Mode: "elements"})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	output, errWrite := parser.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	// Parse the output again to verify structural equivalence.
	root2, errReparse := parser.Parse(output)
	if errReparse != nil {
		t.Fatalf("re-Parse() error = %v", errReparse)
	}

	if root2.Key != root.Key {
		t.Errorf("round-trip root.Key = %q, want %q", root2.Key, root.Key)
	}
	if len(root2.Children) != len(root.Children) {
		t.Fatalf("round-trip children count = %d, want %d", len(root2.Children), len(root.Children))
	}
	for i, child := range root.Children {
		child2 := root2.Children[i]
		if child2.Key != child.Key {
			t.Errorf("round-trip child[%d].Key = %q, want %q", i, child2.Key, child.Key)
		}
		if child2.Value != child.Value {
			t.Errorf("round-trip child[%d].Value = %q, want %q", i, child2.Value, child.Value)
		}
	}

	// Verify output has XML declaration and LF line endings.
	outputStr := string(output)
	if !strings.HasPrefix(outputStr, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("output missing XML declaration")
	}
	if strings.Contains(outputStr, "\r\n") {
		t.Error("output contains CRLF line endings")
	}
}

func TestXML_Elements_RegisteredInRegistry(t *testing.T) {
	p, errGet := GetParser("xml")
	if errGet != nil {
		t.Fatalf("GetParser(\"xml\") error = %v", errGet)
	}
	if p.Structured == nil {
		t.Fatal("GetParser(\"xml\") returned nil Structured parser")
	}
	if p.Structured.Format() != "xml" {
		t.Errorf("Format() = %q, want %q", p.Structured.Format(), "xml")
	}
}

func TestXML_Attributes_ParseProperties(t *testing.T) {
	input := `<properties>
  <property name="motd" value="Hello"/>
  <property name="port" value="25565"/>
</properties>`

	parser := NewXMLParser(XMLKeyMode{
		Mode:      "attributes",
		Element:   "property",
		KeyAttr:   "name",
		ValueAttr: "value",
	})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	if root.Key != "properties" {
		t.Errorf("root.Key = %q, want %q", root.Key, "properties")
	}
	if len(root.Children) != 2 {
		t.Fatalf("root.Children count = %d, want 2", len(root.Children))
	}

	motd := root.Children[0]
	if motd.Key != "motd" {
		t.Errorf("child[0].Key = %q, want %q", motd.Key, "motd")
	}
	if motd.Value != "Hello" {
		t.Errorf("child[0].Value = %q, want %q", motd.Value, "Hello")
	}

	port := root.Children[1]
	if port.Key != "port" {
		t.Errorf("child[1].Key = %q, want %q", port.Key, "port")
	}
	if port.Value != "25565" {
		t.Errorf("child[1].Value = %q, want %q", port.Value, "25565")
	}
}

func TestXML_Attributes_RoundTrip(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties>
  <property name="motd" value="Hello"/>
  <property name="port" value="25565"/>
</properties>
`

	parser := NewXMLParser(XMLKeyMode{
		Mode:      "attributes",
		Element:   "property",
		KeyAttr:   "name",
		ValueAttr: "value",
	})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	output, errWrite := parser.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	// Re-parse and compare.
	root2, errReparse := parser.Parse(output)
	if errReparse != nil {
		t.Fatalf("re-Parse() error = %v", errReparse)
	}

	if len(root2.Children) != len(root.Children) {
		t.Fatalf("round-trip children count = %d, want %d", len(root2.Children), len(root.Children))
	}
	for i, child := range root.Children {
		child2 := root2.Children[i]
		if child2.Key != child.Key {
			t.Errorf("round-trip child[%d].Key = %q, want %q", i, child2.Key, child.Key)
		}
		if child2.Value != child.Value {
			t.Errorf("round-trip child[%d].Value = %q, want %q", i, child2.Value, child.Value)
		}
	}

	outputStr := string(output)
	if !strings.HasPrefix(outputStr, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("output missing XML declaration")
	}
	if strings.Contains(outputStr, "\r\n") {
		t.Error("output contains CRLF line endings")
	}
}

func TestXML_Attributes_CommentPreservation(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties>
  <!-- Server MOTD -->
  <property name="motd" value="Hello"/>
  <!-- Server port -->
  <property name="port" value="25565"/>
</properties>
`

	parser := NewXMLParser(XMLKeyMode{
		Mode:      "attributes",
		Element:   "property",
		KeyAttr:   "name",
		ValueAttr: "value",
	})
	root, errParse := parser.Parse([]byte(input))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	// Verify comments are captured on the property nodes.
	if len(root.Children) != 2 {
		t.Fatalf("root.Children count = %d, want 2", len(root.Children))
	}

	motd := root.Children[0]
	if motd.Comment != "Server MOTD" {
		t.Errorf("child[0].Comment = %q, want %q", motd.Comment, "Server MOTD")
	}

	port := root.Children[1]
	if port.Comment != "Server port" {
		t.Errorf("child[1].Comment = %q, want %q", port.Comment, "Server port")
	}

	// Write and verify comments survive.
	output, errWrite := parser.Write(root)
	if errWrite != nil {
		t.Fatalf("Write() error = %v", errWrite)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "<!-- Server MOTD -->") {
		t.Error("output missing 'Server MOTD' comment")
	}
	if !strings.Contains(outputStr, "<!-- Server port -->") {
		t.Error("output missing 'Server port' comment")
	}

	// Re-parse to verify full round-trip.
	root2, errReparse := parser.Parse(output)
	if errReparse != nil {
		t.Fatalf("re-Parse() error = %v", errReparse)
	}

	for i, child := range root.Children {
		child2 := root2.Children[i]
		if child2.Comment != child.Comment {
			t.Errorf("round-trip child[%d].Comment = %q, want %q", i, child2.Comment, child.Comment)
		}
	}
}
