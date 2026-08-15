package cfgparse

import (
	"testing"
)

func TestFlatten_SimpleObject(t *testing.T) {
	root := &ConfigNode{
		Type: NodeObject,
		Children: []*ConfigNode{
			{Key: "name", Value: "test-server", Type: NodeString},
			{Key: "port", Value: "25565", Type: NodeString},
		},
	}

	entries := Flatten(root)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Key != "name" || entries[0].Value != "test-server" {
		t.Errorf("entry[0] = %q:%q, want name:test-server", entries[0].Key, entries[0].Value)
	}
	if entries[1].Key != "port" || entries[1].Value != "25565" {
		t.Errorf("entry[1] = %q:%q, want port:25565", entries[1].Key, entries[1].Value)
	}
	if entries[0].Index != 0 || entries[1].Index != 0 {
		t.Errorf("indexes = %d,%d, want 0,0", entries[0].Index, entries[1].Index)
	}
}

func TestFlatten_NestedObject(t *testing.T) {
	root := &ConfigNode{
		Type: NodeObject,
		Children: []*ConfigNode{
			{
				Key:  "settings",
				Type: NodeObject,
				Children: []*ConfigNode{
					{
						Key:  "world",
						Type: NodeObject,
						Children: []*ConfigNode{
							{Key: "difficulty", Value: "hard", Type: NodeString},
						},
					},
				},
			},
		},
	}

	entries := Flatten(root)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Key != "settings.world.difficulty" {
		t.Errorf("key = %q, want settings.world.difficulty", entries[0].Key)
	}
	if entries[0].Value != "hard" {
		t.Errorf("value = %q, want hard", entries[0].Value)
	}
}

func TestFlatten_ArrayNodes(t *testing.T) {
	root := &ConfigNode{
		Type: NodeObject,
		Children: []*ConfigNode{
			{
				Key:  "players",
				Type: NodeArray,
				Children: []*ConfigNode{
					{Value: "Alice", Type: NodeString},
					{Value: "Bob", Type: NodeString},
				},
			},
		},
	}

	entries := Flatten(root)
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Key != "players.0" || entries[0].Value != "Alice" {
		t.Errorf("entry[0] = %q:%q, want players.0:Alice", entries[0].Key, entries[0].Value)
	}
	if entries[1].Key != "players.1" || entries[1].Value != "Bob" {
		t.Errorf("entry[1] = %q:%q, want players.1:Bob", entries[1].Key, entries[1].Value)
	}
}

func TestFlatten_EmptyTree(t *testing.T) {
	tests := []struct {
		name string
		root *ConfigNode
	}{
		{name: "nil root", root: nil},
		{name: "no children", root: &ConfigNode{Type: NodeObject}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries := Flatten(tt.root)
			if len(entries) != 0 {
				t.Errorf("expected 0 entries, got %d", len(entries))
			}
		})
	}
}

func TestFlatten_MixedTypes(t *testing.T) {
	root := &ConfigNode{
		Type: NodeObject,
		Children: []*ConfigNode{
			{Key: "name", Value: "server", Type: NodeString},
			{Key: "port", Value: "8080", Type: NodeNumber},
			{Key: "debug", Value: "true", Type: NodeBool},
			{Key: "description", Value: "", Type: NodeNull},
		},
	}

	entries := Flatten(root)
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	wantKeys := []string{"name", "port", "debug", "description"}
	wantVals := []string{"server", "8080", "true", ""}

	for i, entry := range entries {
		if entry.Key != wantKeys[i] {
			t.Errorf("entry[%d].Key = %q, want %q", i, entry.Key, wantKeys[i])
		}
		if entry.Value != wantVals[i] {
			t.Errorf("entry[%d].Value = %q, want %q", i, entry.Value, wantVals[i])
		}
	}
}
