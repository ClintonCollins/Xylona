package cfgparse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// JSONParser implements StructuredConfigParser for JSON files.
type JSONParser struct{}

func init() {
	RegisterStructured(&JSONParser{})
}

// Format returns the format identifier for JSON.
func (p *JSONParser) Format() string {
	return "json"
}

// Parse decodes JSON data into a ConfigNode tree.
func (p *JSONParser) Parse(data []byte) (*ConfigNode, error) {
	var raw any
	errUnmarshal := json.Unmarshal(data, &raw)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing JSON: %w", errUnmarshal)
	}
	return buildNode("", raw), nil
}

func buildNode(key string, val any) *ConfigNode {
	switch v := val.(type) {
	case map[string]any:
		node := &ConfigNode{
			Key:  key,
			Type: NodeObject,
		}
		for k, child := range v {
			node.Children = append(node.Children, buildNode(k, child))
		}
		return node
	case []any:
		node := &ConfigNode{
			Key:  key,
			Type: NodeArray,
		}
		for _, child := range v {
			node.Children = append(node.Children, buildNode("", child))
		}
		return node
	case string:
		return &ConfigNode{Key: key, Type: NodeString, Value: v}
	case float64:
		s := strconv.FormatFloat(v, 'f', -1, 64)
		return &ConfigNode{Key: key, Type: NodeNumber, Value: s}
	case bool:
		return &ConfigNode{Key: key, Type: NodeBool, Value: strconv.FormatBool(v)}
	case nil:
		return &ConfigNode{Key: key, Type: NodeNull, Value: ""}
	default:
		return &ConfigNode{Key: key, Type: NodeString, Value: fmt.Sprintf("%v", v)}
	}
}

// Write serializes a ConfigNode tree back to JSON with 2-space indentation and a trailing newline.
func (p *JSONParser) Write(root *ConfigNode) ([]byte, error) {
	v := nodeToValue(root)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	errEncode := enc.Encode(v)
	if errEncode != nil {
		return nil, fmt.Errorf("writing JSON: %w", errEncode)
	}
	// json.Encoder.Encode already appends a newline.
	// Normalize to LF line endings.
	out := bytes.ReplaceAll(buf.Bytes(), []byte("\r\n"), []byte("\n"))
	return out, nil
}

func nodeToValue(n *ConfigNode) any {
	switch n.Type {
	case NodeObject:
		m := make(map[string]any, len(n.Children))
		for _, child := range n.Children {
			m[child.Key] = nodeToValue(child)
		}
		return m
	case NodeArray:
		arr := make([]any, 0, len(n.Children))
		for _, child := range n.Children {
			arr = append(arr, nodeToValue(child))
		}
		return arr
	case NodeString:
		return n.Value
	case NodeNumber:
		f, errParse := strconv.ParseFloat(n.Value, 64)
		if errParse != nil {
			return n.Value
		}
		return f
	case NodeBool:
		return n.Value == "true"
	case NodeNull:
		return nil
	default:
		return n.Value
	}
}
