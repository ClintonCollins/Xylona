package cfgparse

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLParser parses YAML configuration files.
type YAMLParser struct{}

func init() {
	RegisterStructured(&YAMLParser{})
}

// Format returns the format identifier for this parser.
func (p *YAMLParser) Format() string {
	return "yaml"
}

// Parse reads YAML data and returns a structured config tree.
func (p *YAMLParser) Parse(data []byte) (*ConfigNode, error) {
	var doc yaml.Node
	errUnmarshal := yaml.Unmarshal(data, &doc)
	if errUnmarshal != nil {
		return nil, fmt.Errorf("parsing yaml: %w", errUnmarshal)
	}

	root := &ConfigNode{
		Type: NodeObject,
	}

	// Empty document.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return root, nil
	}

	// The top-level yaml.Node from Unmarshal is a DocumentNode wrapping the actual content.
	content := doc.Content[0]

	// A bare document separator (---) with no content produces a null scalar.
	if content.Kind == yaml.ScalarNode && content.Tag == "!!null" {
		return root, nil
	}

	converted, errConvert := yamlNodeToConfig("", content, make(map[*yaml.Node]struct{}))
	if errConvert != nil {
		return nil, fmt.Errorf("converting yaml tree: %w", errConvert)
	}

	return converted, nil
}

// Write serializes a ConfigNode tree back to YAML.
func (p *YAMLParser) Write(root *ConfigNode) ([]byte, error) {
	yn := configToYAMLNode(root)

	doc := &yaml.Node{
		Kind:    yaml.DocumentNode,
		Content: []*yaml.Node{yn},
	}

	var buf strings.Builder
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(4)

	errEncode := enc.Encode(doc)
	if errEncode != nil {
		return nil, fmt.Errorf("encoding yaml: %w", errEncode)
	}

	errClose := enc.Close()
	if errClose != nil {
		return nil, fmt.Errorf("closing yaml encoder: %w", errClose)
	}

	// Normalize to LF line endings.
	result := strings.ReplaceAll(buf.String(), "\r\n", "\n")
	return []byte(result), nil
}

// yamlNodeToConfig converts a yaml.Node into a ConfigNode.
func yamlNodeToConfig(key string, yn *yaml.Node, aliasStack map[*yaml.Node]struct{}) (*ConfigNode, error) {
	comment := mergeComments(yn)

	switch yn.Kind {
	case yaml.MappingNode:
		node := &ConfigNode{
			Key:     key,
			Type:    NodeObject,
			Comment: comment,
		}
		for i := 0; i < len(yn.Content)-1; i += 2 {
			keyNode := yn.Content[i]
			valNode := yn.Content[i+1]

			child, errChild := yamlNodeToConfig(keyNode.Value, valNode, aliasStack)
			if errChild != nil {
				return nil, errChild
			}
			// Attach key node comments to the child if the child has none.
			keyComment := mergeComments(keyNode)
			if keyComment != "" && child.Comment == "" {
				child.Comment = keyComment
			}
			node.Children = append(node.Children, child)
		}
		return node, nil

	case yaml.SequenceNode:
		node := &ConfigNode{
			Key:     key,
			Type:    NodeArray,
			Comment: comment,
		}
		for _, item := range yn.Content {
			child, errChild := yamlNodeToConfig("", item, aliasStack)
			if errChild != nil {
				return nil, errChild
			}
			node.Children = append(node.Children, child)
		}
		return node, nil

	case yaml.ScalarNode:
		node := &ConfigNode{
			Key:     key,
			Value:   yn.Value,
			Comment: comment,
		}
		switch yn.Tag {
		case "!!int", "!!float":
			node.Type = NodeNumber
		case "!!bool":
			node.Type = NodeBool
		case "!!null":
			node.Type = NodeNull
			node.Value = ""
		default:
			node.Type = NodeString
		}
		return node, nil

	case yaml.AliasNode:
		if yn.Alias == nil {
			return nil, fmt.Errorf("yaml alias has no target")
		}
		if _, exists := aliasStack[yn.Alias]; exists {
			return nil, fmt.Errorf("yaml alias cycle detected")
		}
		aliasStack[yn.Alias] = struct{}{}
		defer delete(aliasStack, yn.Alias)
		return yamlNodeToConfig(key, yn.Alias, aliasStack)

	default:
		return nil, fmt.Errorf("unsupported yaml node kind: %d", yn.Kind)
	}
}

// configToYAMLNode converts a ConfigNode back into a yaml.Node tree.
func configToYAMLNode(cn *ConfigNode) *yaml.Node {
	switch cn.Type {
	case NodeObject:
		yn := &yaml.Node{
			Kind: yaml.MappingNode,
			Tag:  "!!map",
		}
		applyComment(yn, cn.Comment)
		for _, child := range cn.Children {
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!str",
				Value: child.Key,
			}
			valNode := configToYAMLNode(child)
			yn.Content = append(yn.Content, keyNode, valNode)
		}
		return yn

	case NodeArray:
		yn := &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
		}
		applyComment(yn, cn.Comment)
		for _, child := range cn.Children {
			yn.Content = append(yn.Content, configToYAMLNode(child))
		}
		return yn

	case NodeNumber:
		yn := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: cn.Value,
			Tag:   "!!float",
		}
		// Use int tag if value looks like an integer.
		if !strings.Contains(cn.Value, ".") {
			yn.Tag = "!!int"
		}
		applyComment(yn, cn.Comment)
		return yn

	case NodeBool:
		yn := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: cn.Value,
			Tag:   "!!bool",
		}
		applyComment(yn, cn.Comment)
		return yn

	case NodeNull:
		yn := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "null",
			Tag:   "!!null",
		}
		applyComment(yn, cn.Comment)
		return yn

	default:
		// NodeString
		yn := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: cn.Value,
			Tag:   "!!str",
		}
		applyComment(yn, cn.Comment)
		return yn
	}
}

// mergeComments combines all comment fields from a yaml.Node into a single string.
func mergeComments(yn *yaml.Node) string {
	parts := make([]string, 0, 3)
	if yn.HeadComment != "" {
		parts = append(parts, yn.HeadComment)
	}
	if yn.LineComment != "" {
		parts = append(parts, yn.LineComment)
	}
	if yn.FootComment != "" {
		parts = append(parts, yn.FootComment)
	}
	return strings.Join(parts, "\n")
}

// applyComment sets the HeadComment on a yaml.Node from a ConfigNode comment string.
func applyComment(yn *yaml.Node, comment string) {
	if comment == "" {
		return
	}
	yn.HeadComment = comment
}
