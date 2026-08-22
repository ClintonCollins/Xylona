package cfgschema

import (
	"testing"
)

func TestSortFieldsBySchema_RespectsXOrder(t *testing.T) {
	fields := []FieldData{
		{Key: "c", Group: "net", Order: new(int32(3))},
		{Key: "a", Group: "net", Order: new(int32(1))},
		{Key: "b", Group: "net", Order: new(int32(2))},
	}
	schema := SchemaDefinition{
		Groups: []string{"net"},
	}

	sorted := SortFieldsBySchema(fields, schema)

	expected := []string{"a", "b", "c"}
	for i, want := range expected {
		if sorted[i].Key != want {
			t.Errorf("sorted[%d].Key = %q, want %q", i, sorted[i].Key, want)
		}
	}
}

func TestSortFieldsBySchema_RespectsXGroups(t *testing.T) {
	fields := []FieldData{
		{Key: "motd", Group: "gameplay"},
		{Key: "port", Group: "network"},
		{Key: "name", Group: "general"},
	}
	schema := SchemaDefinition{
		Groups: []string{"network", "gameplay", "general"},
	}

	sorted := SortFieldsBySchema(fields, schema)

	expected := []string{"port", "motd", "name"}
	for i, want := range expected {
		if sorted[i].Key != want {
			t.Errorf("sorted[%d].Key = %q, want %q", i, sorted[i].Key, want)
		}
	}
}

func TestSortFieldsBySchema_NilOrderSortsLast(t *testing.T) {
	fields := []FieldData{
		{Key: "z-unordered", Group: "net"},
		{Key: "a-unordered", Group: "net"},
		{Key: "ordered", Group: "net", Order: new(int32(1))},
	}
	schema := SchemaDefinition{
		Groups: []string{"net"},
	}

	sorted := SortFieldsBySchema(fields, schema)

	if sorted[0].Key != "ordered" {
		t.Errorf("sorted[0].Key = %q, want %q", sorted[0].Key, "ordered")
	}
	// Stable sort preserves relative order of nil-Order fields.
	if sorted[1].Key != "z-unordered" {
		t.Errorf("sorted[1].Key = %q, want %q", sorted[1].Key, "z-unordered")
	}
	if sorted[2].Key != "a-unordered" {
		t.Errorf("sorted[2].Key = %q, want %q", sorted[2].Key, "a-unordered")
	}
}

func TestSortFieldsBySchema_UnlistedGroupsAppendInFirstOccurrence(t *testing.T) {
	fields := []FieldData{
		{Key: "a", Group: "unlisted-b"},
		{Key: "b", Group: "listed"},
		{Key: "c", Group: "unlisted-a"},
	}
	schema := SchemaDefinition{
		Groups: []string{"listed"},
	}

	sorted := SortFieldsBySchema(fields, schema)

	expected := []string{"b", "a", "c"}
	for i, want := range expected {
		if sorted[i].Key != want {
			t.Errorf("sorted[%d].Key = %q, want %q", i, sorted[i].Key, want)
		}
	}
}

func TestSortFieldsBySchema_EmptyGroupGeneralFirst(t *testing.T) {
	fields := []FieldData{
		{Key: "grouped", Group: "network"},
		{Key: "ungrouped", Group: ""},
	}
	schema := SchemaDefinition{
		Groups: []string{"network"},
	}

	sorted := SortFieldsBySchema(fields, schema)

	if sorted[0].Key != "ungrouped" {
		t.Errorf("sorted[0].Key = %q, want %q (empty group should be first)", sorted[0].Key, "ungrouped")
	}
	if sorted[1].Key != "grouped" {
		t.Errorf("sorted[1].Key = %q, want %q", sorted[1].Key, "grouped")
	}
}

func TestSortFieldsBySchema_NoOrderNoGroups_PreservesOriginalOrder(t *testing.T) {
	fields := []FieldData{
		{Key: "z"},
		{Key: "a"},
		{Key: "m"},
	}
	schema := SchemaDefinition{}

	sorted := SortFieldsBySchema(fields, schema)

	expected := []string{"z", "a", "m"}
	for i, want := range expected {
		if sorted[i].Key != want {
			t.Errorf("sorted[%d].Key = %q, want %q", i, sorted[i].Key, want)
		}
	}
}

func TestSortedPropertyKeys_SortsByOrder(t *testing.T) {
	schema := SchemaDefinition{
		Properties: map[string]SchemaProperty{
			"c": {Order: new(int32(3))},
			"a": {Order: new(int32(1))},
			"b": {Order: new(int32(2))},
		},
	}

	keys := SortedPropertyKeys(schema)

	expected := []string{"a", "b", "c"}
	for i, want := range expected {
		if keys[i] != want {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want)
		}
	}
}

func TestSortedPropertyKeys_NilOrderFallsBackToAlphabetical(t *testing.T) {
	schema := SchemaDefinition{
		Properties: map[string]SchemaProperty{
			"zebra": {},
			"apple": {},
			"mango": {},
		},
	}

	keys := SortedPropertyKeys(schema)

	expected := []string{"apple", "mango", "zebra"}
	for i, want := range expected {
		if keys[i] != want {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want)
		}
	}
}

func TestSortedPropertyKeys_MixedOrderAndNil(t *testing.T) {
	schema := SchemaDefinition{
		Properties: map[string]SchemaProperty{
			"zebra":    {},
			"ordered1": {Order: new(int32(2))},
			"apple":    {},
			"ordered0": {Order: new(int32(1))},
		},
	}

	keys := SortedPropertyKeys(schema)

	expected := []string{"ordered0", "ordered1", "apple", "zebra"}
	for i, want := range expected {
		if keys[i] != want {
			t.Errorf("keys[%d] = %q, want %q", i, keys[i], want)
		}
	}
}

func TestSortFieldsBySchema_CombinedGroupAndFieldOrder(t *testing.T) {
	fields := []FieldData{
		// Group "gameplay" fields (should be third group).
		{Key: "pvp", Group: "gameplay", Order: new(int32(1))},
		{Key: "difficulty", Group: "gameplay", Order: new(int32(0))},
		{Key: "motd", Group: "gameplay", Order: new(int32(2))},
		// Group "network" fields (should be first group).
		{Key: "query-port", Group: "network", Order: new(int32(2))},
		{Key: "ip", Group: "network", Order: new(int32(0))},
		{Key: "port", Group: "network", Order: new(int32(1))},
		// Group "world" fields (should be second group).
		{Key: "seed", Group: "world", Order: new(int32(1))},
		{Key: "level-name", Group: "world", Order: new(int32(0))},
		{Key: "spawn-protection", Group: "world", Order: new(int32(2))},
	}
	schema := SchemaDefinition{
		Groups: []string{"network", "world", "gameplay"},
	}

	sorted := SortFieldsBySchema(fields, schema)

	wantKeys := []string{
		// network group, ordered by x-order
		"ip", "port", "query-port",
		// world group, ordered by x-order
		"level-name", "seed", "spawn-protection",
		// gameplay group, ordered by x-order
		"difficulty", "pvp", "motd",
	}

	if len(sorted) != len(wantKeys) {
		t.Fatalf("expected %d fields, got %d", len(wantKeys), len(sorted))
	}
	for i, want := range wantKeys {
		if sorted[i].Key != want {
			t.Errorf("sorted[%d].Key = %q, want %q", i, sorted[i].Key, want)
		}
	}
}

func TestSortedPropertyKeys_EmptySchema(t *testing.T) {
	schema := SchemaDefinition{
		Properties: map[string]SchemaProperty{},
	}

	keys := SortedPropertyKeys(schema)

	if len(keys) != 0 {
		t.Errorf("expected empty slice, got %v", keys)
	}
}

func TestSortFieldsBySchema_DoesNotMutateInput(t *testing.T) {
	fields := []FieldData{
		{Key: "c", Group: "net", Order: new(int32(3))},
		{Key: "a", Group: "net", Order: new(int32(1))},
		{Key: "b", Group: "net", Order: new(int32(2))},
	}
	schema := SchemaDefinition{
		Groups: []string{"net"},
	}

	// Make a copy of the original order to compare after sorting.
	originalKeys := make([]string, len(fields))
	for i, f := range fields {
		originalKeys[i] = f.Key
	}

	_ = SortFieldsBySchema(fields, schema)

	// Verify the input slice was not modified.
	for i, f := range fields {
		if f.Key != originalKeys[i] {
			t.Errorf("input fields[%d].Key = %q, was %q before sort — input was mutated",
				i, f.Key, originalKeys[i])
		}
	}
}
