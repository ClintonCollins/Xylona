package cfgschema

import (
	"cmp"
	"math"
	"slices"
	"strings"
)

// SortFieldsBySchema sorts fields by group order (from schema.Groups), then by
// field order (from Order) within each group. Fields in the empty group (General)
// are placed first. Groups not listed in schema.Groups are appended in
// first-occurrence order. Fields with nil Order sort after explicitly ordered
// fields, preserving their relative order (stable sort).
func SortFieldsBySchema(fields []FieldData, schema SchemaDefinition) []FieldData {
	if len(fields) == 0 {
		return fields
	}

	groupPos := map[string]int{}
	groupPos[""] = -1
	for i, g := range schema.Groups {
		groupPos[g] = i
	}

	nextPos := len(schema.Groups)
	for _, f := range fields {
		if _, exists := groupPos[f.Group]; !exists {
			groupPos[f.Group] = nextPos
			nextPos++
		}
	}

	sorted := make([]FieldData, len(fields))
	copy(sorted, fields)

	slices.SortStableFunc(sorted, func(a, b FieldData) int {
		gCmp := cmp.Compare(groupPos[a.Group], groupPos[b.Group])
		if gCmp != 0 {
			return gCmp
		}

		aOrd := int32(math.MaxInt32)
		if a.Order != nil {
			aOrd = *a.Order
		}
		bOrd := int32(math.MaxInt32)
		if b.Order != nil {
			bOrd = *b.Order
		}
		return cmp.Compare(aOrd, bOrd)
	})

	return sorted
}

// SortedPropertyKeys returns property keys sorted by x-order ascending, with
// nil-order keys falling back to alphabetical order at the end.
func SortedPropertyKeys(schema SchemaDefinition) []string {
	keys := make([]string, 0, len(schema.Properties))
	for k := range schema.Properties {
		keys = append(keys, k)
	}

	slices.SortStableFunc(keys, func(a, b string) int {
		aOrd := int32(math.MaxInt32)
		if v := schema.Properties[a].Order; v != nil {
			aOrd = *v
		}
		bOrd := int32(math.MaxInt32)
		if v := schema.Properties[b].Order; v != nil {
			bOrd = *v
		}
		if aOrd != bOrd {
			return cmp.Compare(aOrd, bOrd)
		}
		return strings.Compare(a, b)
	})

	return keys
}
