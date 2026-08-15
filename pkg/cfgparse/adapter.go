// Package cfgparse parses and serializes supported configuration file formats.
package cfgparse

import (
	"strconv"
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

	keyCounts := make(map[string]int)
	for i := range entries {
		entries[i].Index = keyCounts[entries[i].Key]
		keyCounts[entries[i].Key]++
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
