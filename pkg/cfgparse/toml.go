package cfgparse

import (
	"bytes"
	"fmt"
	"strconv"

	toml "github.com/pelletier/go-toml/v2"
)

// TOMLParser implements StructuredConfigParser for TOML files.
type TOMLParser struct{}

func init() {
	RegisterStructured(&TOMLParser{})
}

// Format returns the format identifier for TOML.
func (p *TOMLParser) Format() string {
	return "toml"
}

// Parse decodes TOML data into a ConfigNode tree.
func (p *TOMLParser) Parse(data []byte) (*ConfigNode, error) {
	var raw map[string]any
	errUnmarshal := toml.Unmarshal(data, &raw)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing TOML: %w", errUnmarshal)
	}
	root := &ConfigNode{
		Type: NodeObject,
	}
	for k, v := range raw {
		root.Children = append(root.Children, tomlBuildNode(k, v))
	}
	return root, nil
}

func tomlBuildNode(key string, val any) *ConfigNode {
	switch v := val.(type) {
	case map[string]any:
		node := &ConfigNode{
			Key:  key,
			Type: NodeObject,
		}
		for k, child := range v {
			node.Children = append(node.Children, tomlBuildNode(k, child))
		}
		return node
	case []any:
		node := &ConfigNode{
			Key:  key,
			Type: NodeArray,
		}
		for _, child := range v {
			node.Children = append(node.Children, tomlBuildNode("", child))
		}
		return node
	case string:
		return &ConfigNode{Key: key, Type: NodeString, Value: v}
	case int64:
		return &ConfigNode{Key: key, Type: NodeNumber, Value: strconv.FormatInt(v, 10)}
	case float64:
		return &ConfigNode{Key: key, Type: NodeNumber, Value: strconv.FormatFloat(v, 'f', -1, 64)}
	case bool:
		return &ConfigNode{Key: key, Type: NodeBool, Value: strconv.FormatBool(v)}
	default:
		return &ConfigNode{Key: key, Type: NodeString, Value: fmt.Sprintf("%v", v)}
	}
}

// Write serializes a ConfigNode tree back to TOML.
func (p *TOMLParser) Write(root *ConfigNode) ([]byte, error) {
	m := tomlNodeToMap(root)
	out, errMarshal := toml.Marshal(m)
	if errMarshal != nil {
		return nil, fmt.Errorf("writing TOML: %w", errMarshal)
	}
	// Normalize to LF line endings.
	out = bytes.ReplaceAll(out, []byte("\r\n"), []byte("\n"))
	return out, nil
}

func tomlNodeToMap(n *ConfigNode) map[string]any {
	m := make(map[string]any, len(n.Children))
	for _, child := range n.Children {
		m[child.Key] = tomlNodeToValue(child)
	}
	return m
}

func tomlNodeToValue(n *ConfigNode) any {
	switch n.Type {
	case NodeObject:
		return tomlNodeToMap(n)
	case NodeArray:
		arr := make([]any, 0, len(n.Children))
		for _, child := range n.Children {
			arr = append(arr, tomlNodeToValue(child))
		}
		return arr
	case NodeString:
		return n.Value
	case NodeNumber:
		// Try integer first, then float.
		i, errInt := strconv.ParseInt(n.Value, 10, 64)
		if errInt == nil {
			return i
		}
		f, errFloat := strconv.ParseFloat(n.Value, 64)
		if errFloat == nil {
			return f
		}
		return n.Value
	case NodeBool:
		return n.Value == "true"
	case NodeNull:
		return nil
	default:
		return n.Value
	}
}
