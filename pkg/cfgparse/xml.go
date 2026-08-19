package cfgparse

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// XMLKeyMode controls how the XML parser maps XML structures to ConfigNode trees.
type XMLKeyMode struct {
	Mode      string // "elements" or "attributes"
	Element   string // e.g., "property" (attributes mode only)
	KeyAttr   string // e.g., "name" (attributes mode only)
	ValueAttr string // e.g., "value" (attributes mode only)
}

// XMLParser implements StructuredConfigParser for XML files.
type XMLParser struct {
	KeyMode XMLKeyMode
}

// NewXMLParser creates an XMLParser with the given key mode.
func NewXMLParser(mode XMLKeyMode) *XMLParser {
	return &XMLParser{KeyMode: mode}
}

func init() {
	RegisterStructured(NewXMLParser(XMLKeyMode{Mode: "elements"}))
}

// Format returns the format identifier for XML.
func (p *XMLParser) Format() string {
	return "xml"
}

// Parse decodes XML data into a ConfigNode tree.
func (p *XMLParser) Parse(data []byte) (*ConfigNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	root, errParse := p.parseElement(decoder)
	if errParse != nil {
		return nil, fmt.Errorf("parsing XML: %w", errParse)
	}
	return root, nil
}

// parseElement reads tokens until it finds the first start element and parses it into a tree.
func (p *XMLParser) parseElement(decoder *xml.Decoder) (*ConfigNode, error) {
	for {
		tok, errToken := decoder.Token()
		if errToken != nil {
			return nil, fmt.Errorf("reading XML token: %w", errToken)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		return p.parseStartElement(decoder, start, "")
	}
}

// parseStartElement recursively parses an XML element and its children.
func (p *XMLParser) parseStartElement(decoder *xml.Decoder, start xml.StartElement, pendingComment string) (*ConfigNode, error) {
	node := &ConfigNode{
		Key:     start.Name.Local,
		Type:    NodeObject,
		Comment: pendingComment,
	}

	// In attributes mode, check if this element is a property element.
	if p.KeyMode.Mode == "attributes" && start.Name.Local == p.KeyMode.Element {
		return p.parsePropertyElement(start, decoder, pendingComment)
	}

	// Store XML attributes as @-prefixed children.
	for _, attr := range start.Attr {
		attrNode := &ConfigNode{
			Key:   "@" + attr.Name.Local,
			Type:  NodeString,
			Value: attr.Value,
		}
		node.Children = append(node.Children, attrNode)
	}

	// Parse child tokens until the matching end element.
	var textContent strings.Builder
	var comment string
	for {
		tok, errToken := decoder.Token()
		if errToken != nil {
			if errors.Is(errToken, io.EOF) {
				break
			}
			return nil, fmt.Errorf("reading child token: %w", errToken)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			child, errChild := p.parseStartElement(decoder, t, comment)
			if errChild != nil {
				return nil, errChild
			}
			comment = ""
			node.Children = append(node.Children, child)
		case xml.EndElement:
			// If this element has no children, it's a leaf with text content.
			text := strings.TrimSpace(textContent.String())
			if len(node.Children) == 0 {
				node.Type = NodeString
				node.Value = text
			}
			return node, nil
		case xml.CharData:
			textContent.Write(t)
		case xml.Comment:
			comment = strings.TrimSpace(string(t))
		}
	}

	return node, nil
}

// parsePropertyElement handles attribute-mode property elements like <property name="X" value="Y"/>.
func (p *XMLParser) parsePropertyElement(start xml.StartElement, decoder *xml.Decoder, comment string) (*ConfigNode, error) {
	var key, value string
	for _, attr := range start.Attr {
		if attr.Name.Local == p.KeyMode.KeyAttr {
			key = attr.Value
		}
		if attr.Name.Local == p.KeyMode.ValueAttr {
			value = attr.Value
		}
	}

	// Skip to end of this element.
	errSkip := decoder.Skip()
	if errSkip != nil {
		return nil, fmt.Errorf("skipping property element: %w", errSkip)
	}

	return &ConfigNode{
		Key:     key,
		Type:    NodeString,
		Value:   value,
		Comment: comment,
	}, nil
}

// Write serializes a ConfigNode tree back to XML.
func (p *XMLParser) Write(root *ConfigNode) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")

	errWrite := p.writeNode(&buf, root, 0)
	if errWrite != nil {
		return nil, fmt.Errorf("writing XML: %w", errWrite)
	}

	out := bytes.ReplaceAll(buf.Bytes(), []byte("\r\n"), []byte("\n"))
	return out, nil
}

// writeNode writes a single ConfigNode as XML with the given indentation level.
func (p *XMLParser) writeNode(buf *bytes.Buffer, node *ConfigNode, indent int) error {
	prefix := strings.Repeat("  ", indent)

	if node.Comment != "" {
		buf.WriteString(prefix)
		buf.WriteString("<!-- ")
		buf.WriteString(node.Comment)
		buf.WriteString(" -->\n")
	}

	// In attributes mode, write property elements for leaf nodes inside a container.
	if p.KeyMode.Mode == "attributes" && isLeaf(node.Type) && !strings.HasPrefix(node.Key, "@") {
		buf.WriteString(prefix)
		buf.WriteString("<")
		buf.WriteString(p.KeyMode.Element)
		buf.WriteString(" ")
		buf.WriteString(p.KeyMode.KeyAttr)
		buf.WriteString("=\"")
		buf.WriteString(escapeXMLAttr(node.Key))
		buf.WriteString("\" ")
		buf.WriteString(p.KeyMode.ValueAttr)
		buf.WriteString("=\"")
		buf.WriteString(escapeXMLAttr(node.Value))
		buf.WriteString("\"/>\n")
		return nil
	}

	// Collect attributes (@-prefixed children).
	var attrs []*ConfigNode
	var children []*ConfigNode
	for _, child := range node.Children {
		if strings.HasPrefix(child.Key, "@") {
			attrs = append(attrs, child)
		} else {
			children = append(children, child)
		}
	}

	// Open tag.
	buf.WriteString(prefix)
	buf.WriteString("<")
	buf.WriteString(node.Key)
	for _, attr := range attrs {
		buf.WriteString(" ")
		buf.WriteString(strings.TrimPrefix(attr.Key, "@"))
		buf.WriteString("=\"")
		buf.WriteString(escapeXMLAttr(attr.Value))
		buf.WriteString("\"")
	}

	// Self-closing if leaf with empty value and no children.
	if node.Type == NodeString && node.Value == "" && len(children) == 0 {
		buf.WriteString("/>\n")
		return nil
	}

	// Leaf with text content.
	if node.Type == NodeString && len(children) == 0 {
		buf.WriteString(">")
		buf.WriteString(escapeXMLText(node.Value))
		buf.WriteString("</")
		buf.WriteString(node.Key)
		buf.WriteString(">\n")
		return nil
	}

	// Container element with children.
	buf.WriteString(">\n")
	for _, child := range children {
		errChild := p.writeNode(buf, child, indent+1)
		if errChild != nil {
			return errChild
		}
	}
	buf.WriteString(prefix)
	buf.WriteString("</")
	buf.WriteString(node.Key)
	buf.WriteString(">\n")

	return nil
}

// escapeXMLAttr escapes special characters for use in XML attribute values.
func escapeXMLAttr(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// escapeXMLText escapes special characters for use in XML text content.
func escapeXMLText(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
