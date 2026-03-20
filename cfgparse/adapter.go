package cfgparse

import (
	"strconv"
	"strings"
)

// Flatten converts a ConfigNode tree into a flat list of dot-path ConfigEntry values.
// Only leaf nodes (NodeString, NodeNumber, NodeBool, NodeNull) produce entries.
// The root node's Key is not included in the path.
func Flatten(root *ConfigNode) []ConfigEntry {
	if root == nil || len(root.Children) == 0 {
		return nil
	}

	var entries []ConfigEntry
	flattenNode(root, "", &entries)

	for i := range entries {
		entries[i].Index = i
	}

	return entries
}

func flattenNode(node *ConfigNode, prefix string, entries *[]ConfigEntry) {
	for _, child := range node.Children {
		key := child.Key
		if prefix != "" {
			key = prefix + "." + child.Key
		}

		switch child.Type {
		case NodeObject:
			flattenNode(child, key, entries)
		case NodeArray:
			for i, elem := range child.Children {
				elemKey := key + "." + strconv.Itoa(i)
				if isLeaf(elem.Type) {
					*entries = append(*entries, ConfigEntry{
						Key:   elemKey,
						Value: elem.Value,
					})
				} else {
					flattenNode(elem, elemKey, entries)
				}
			}
		default:
			*entries = append(*entries, ConfigEntry{
				Key:   key,
				Value: child.Value,
			})
		}
	}
}

func isLeaf(t NodeType) bool {
	return t == NodeString || t == NodeNumber || t == NodeBool || t == NodeNull
}

// MergeDotPaths applies dot-path updates back into an existing ConfigNode tree.
// For existing paths, it updates the Value of the matching leaf node.
// For new paths, it creates intermediate NodeObject nodes as needed and adds the leaf.
func MergeDotPaths(root *ConfigNode, updates []ConfigEntry) error {
	for _, entry := range updates {
		parts := strings.Split(entry.Key, ".")
		mergePath(root, parts, entry.Value)
	}
	return nil
}

func mergePath(node *ConfigNode, parts []string, value string) {
	if len(parts) == 0 {
		return
	}

	target := parts[0]
	remaining := parts[1:]

	// Search for existing child with matching key.
	for _, child := range node.Children {
		if child.Key == target {
			if len(remaining) == 0 {
				child.Value = value
				return
			}
			mergePath(child, remaining, value)
			return
		}
	}

	// Child not found — create it.
	if len(remaining) == 0 {
		node.Children = append(node.Children, &ConfigNode{
			Key:   target,
			Value: value,
			Type:  NodeString,
		})
		return
	}

	intermediate := &ConfigNode{
		Key:  target,
		Type: NodeObject,
	}
	node.Children = append(node.Children, intermediate)
	mergePath(intermediate, remaining, value)
}
