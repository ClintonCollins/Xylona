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

func TestMerge_UpdateExisting(t *testing.T) {
	root := &ConfigNode{
		Type: NodeObject,
		Children: []*ConfigNode{
			{
				Key:  "settings",
				Type: NodeObject,
				Children: []*ConfigNode{
					{Key: "difficulty", Value: "easy", Type: NodeString},
				},
			},
		},
	}

	errMerge := MergeDotPaths(root, []ConfigEntry{
		{Key: "settings.difficulty", Value: "hard"},
	})
	if errMerge != nil {
		t.Fatalf("MergeDotPaths() error = %v", errMerge)
	}

	leaf := root.Children[0].Children[0]
	if leaf.Value != "hard" {
		t.Errorf("value = %q, want hard", leaf.Value)
	}
}

func TestMerge_AddNewPath(t *testing.T) {
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

	errMerge := MergeDotPaths(root, []ConfigEntry{
		{Key: "settings.world.seed", Value: "12345"},
	})
	if errMerge != nil {
		t.Fatalf("MergeDotPaths() error = %v", errMerge)
	}

	world := root.Children[0].Children[0]
	if len(world.Children) != 2 {
		t.Fatalf("expected 2 children under world, got %d", len(world.Children))
	}

	added := world.Children[1]
	if added.Key != "seed" || added.Value != "12345" {
		t.Errorf("added node = %q:%q, want seed:12345", added.Key, added.Value)
	}
}

func TestMerge_CreateIntermediateNodes(t *testing.T) {
	root := &ConfigNode{Type: NodeObject}

	errMerge := MergeDotPaths(root, []ConfigEntry{
		{Key: "a.b.c", Value: "deep"},
	})
	if errMerge != nil {
		t.Fatalf("MergeDotPaths() error = %v", errMerge)
	}

	if len(root.Children) != 1 {
		t.Fatalf("root has %d children, want 1", len(root.Children))
	}

	a := root.Children[0]
	if a.Key != "a" || a.Type != NodeObject {
		t.Fatalf("a = %q (type %d), want a (NodeObject)", a.Key, a.Type)
	}

	if len(a.Children) != 1 {
		t.Fatalf("a has %d children, want 1", len(a.Children))
	}

	b := a.Children[0]
	if b.Key != "b" || b.Type != NodeObject {
		t.Fatalf("b = %q (type %d), want b (NodeObject)", b.Key, b.Type)
	}

	if len(b.Children) != 1 {
		t.Fatalf("b has %d children, want 1", len(b.Children))
	}

	c := b.Children[0]
	if c.Key != "c" || c.Value != "deep" {
		t.Errorf("c = %q:%q, want c:deep", c.Key, c.Value)
	}
}

func TestFlattenAndMerge_RoundTrip(t *testing.T) {
	root := &ConfigNode{
		Type: NodeObject,
		Children: []*ConfigNode{
			{Key: "name", Value: "my-server", Type: NodeString},
			{
				Key:  "settings",
				Type: NodeObject,
				Children: []*ConfigNode{
					{Key: "difficulty", Value: "normal", Type: NodeString},
					{Key: "maxPlayers", Value: "20", Type: NodeNumber},
				},
			},
		},
	}

	entries := Flatten(root)
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Modify one value in the flattened entries.
	for i := range entries {
		if entries[i].Key == "settings.difficulty" {
			entries[i].Value = "hard"
		}
	}

	errMerge := MergeDotPaths(root, entries)
	if errMerge != nil {
		t.Fatalf("MergeDotPaths() error = %v", errMerge)
	}

	// Verify the updated value.
	difficulty := root.Children[1].Children[0]
	if difficulty.Value != "hard" {
		t.Errorf("difficulty = %q, want hard", difficulty.Value)
	}

	// Verify unchanged values are preserved.
	name := root.Children[0]
	if name.Value != "my-server" {
		t.Errorf("name = %q, want my-server", name.Value)
	}

	maxPlayers := root.Children[1].Children[1]
	if maxPlayers.Value != "20" {
		t.Errorf("maxPlayers = %q, want 20", maxPlayers.Value)
	}
}
