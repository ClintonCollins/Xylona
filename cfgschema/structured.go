package cfgschema

import (
	"strconv"
	"strings"

	"github.com/ClintonCollins/Xylona/cfgparse"
)

// MergeStructuredFields applies schema field and advanced field values to a
// structured config tree using dot-path keys.
func MergeStructuredFields(
	root *cfgparse.ConfigNode,
	updatedFields []FieldData,
	advancedFields []AdvancedFieldData,
	schema SchemaDefinition,
) {
	if root == nil {
		return
	}

	for _, f := range updatedFields {
		prop, exists := schema.Properties[f.Key]
		if !exists {
			continue
		}
		nodeType := nodeTypeForSchemaProperty(prop)
		setStructuredPath(root, f.Key, f.Value, &nodeType)
	}

	for _, af := range advancedFields {
		setStructuredPath(root, af.Key, af.Value, nil)
	}
}

func structuredDefaultRoot(entry ConfigSchemaEntry) *cfgparse.ConfigNode {
	root := &cfgparse.ConfigNode{Type: cfgparse.NodeObject}

	for _, key := range SortedPropertyKeys(entry.Schema) {
		prop := entry.Schema.Properties[key]
		nodeType := nodeTypeForSchemaProperty(prop)
		setStructuredPath(root, key, formatDefault(prop.Default), &nodeType)
	}

	return root
}

func enforceManagedStructuredFields(
	root *cfgparse.ConfigNode,
	managedFields map[string]string,
	schema SchemaDefinition,
	resolver ManagedFieldResolver,
) {
	if root == nil || len(managedFields) == 0 {
		return
	}

	for key, source := range managedFields {
		value, ok := resolver(normalizeManagedSource(source))
		if !ok {
			continue
		}
		prop := schema.Properties[key]
		nodeType := nodeTypeForSchemaProperty(prop)
		setStructuredPath(root, key, value, &nodeType)
	}
}

func nodeTypeForSchemaProperty(prop SchemaProperty) cfgparse.NodeType {
	switch prop.Type {
	case "integer", "number":
		return cfgparse.NodeNumber
	case "boolean":
		return cfgparse.NodeBool
	default:
		return cfgparse.NodeString
	}
}

func setStructuredPath(
	root *cfgparse.ConfigNode,
	key string,
	value string,
	nodeType *cfgparse.NodeType,
) {
	parts := strings.Split(strings.TrimSpace(key), ".")
	if len(parts) == 0 || parts[0] == "" {
		return
	}

	setStructuredPathParts(root, parts, value, nodeType)
}

func setStructuredPathParts(
	node *cfgparse.ConfigNode,
	parts []string,
	value string,
	nodeType *cfgparse.NodeType,
) {
	if node == nil || len(parts) == 0 {
		return
	}

	if node.Type == cfgparse.NodeArray {
		setStructuredArrayPath(node, parts, value, nodeType)
		return
	}

	if node.Type != cfgparse.NodeObject {
		node.Type = cfgparse.NodeObject
		node.Value = ""
		node.Children = nil
	}

	target := parts[0]
	child := findStructuredChild(node, target)
	if child == nil {
		childType := cfgparse.NodeObject
		if len(parts) == 1 {
			childType = cfgparse.NodeString
			if nodeType != nil {
				childType = *nodeType
			}
		}
		child = &cfgparse.ConfigNode{Key: target, Type: childType}
		node.Children = append(node.Children, child)
	}

	if len(parts) == 1 {
		if nodeType != nil {
			child.Type = *nodeType
		}
		child.Value = value
		return
	}

	setStructuredPathParts(child, parts[1:], value, nodeType)
}

func setStructuredArrayPath(
	node *cfgparse.ConfigNode,
	parts []string,
	value string,
	nodeType *cfgparse.NodeType,
) {
	index, errParse := strconv.Atoi(parts[0])
	if errParse != nil || index < 0 {
		return
	}

	for len(node.Children) <= index {
		childType := cfgparse.NodeObject
		if len(parts) == 1 {
			childType = cfgparse.NodeString
			if nodeType != nil {
				childType = *nodeType
			}
		}
		node.Children = append(node.Children, &cfgparse.ConfigNode{Type: childType})
	}

	child := node.Children[index]
	if len(parts) == 1 {
		if nodeType != nil {
			child.Type = *nodeType
		}
		child.Value = value
		return
	}

	setStructuredPathParts(child, parts[1:], value, nodeType)
}

func findStructuredChild(node *cfgparse.ConfigNode, key string) *cfgparse.ConfigNode {
	for _, child := range node.Children {
		if child.Key == key {
			return child
		}
	}

	return nil
}
