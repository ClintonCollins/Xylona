package cfgschema

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
)

func TestMergeStructuredFieldsArrays(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		fields   []FieldData
		advanced []AdvancedFieldData
		want     string
	}{
		{
			name:  "create player group array",
			input: "{}",
			fields: []FieldData{
				{Key: "userGroups.0.name", Value: "Friends", FieldType: "string"},
				{Key: "userGroups.0.canKickBan", Value: "false", FieldType: "boolean"},
				{Key: "userGroups.0.reservedSlots", Value: "2", FieldType: "integer"},
			},
			want: `{"userGroups":[{"name":"Friends","canKickBan":false,"reservedSlots":2}]}`,
		},
		{
			name:  "preserve existing passwords and sibling groups",
			input: `{"userGroups":[{"name":"Friends","password":"private-value","canKickBan":false},{"name":"Visitors","password":"another-value"}],"unknown":true}`,
			fields: []FieldData{
				{Key: "userGroups.0.reservedSlots", Value: "1", FieldType: "integer"},
			},
			want: `{"userGroups":[{"name":"Friends","password":"private-value","canKickBan":false,"reservedSlots":1},{"name":"Visitors","password":"another-value"}],"unknown":true}`,
		},
		{
			name:  "preserve numeric object keys",
			input: `{"groups":{"0":{"name":"Old"}}}`,
			fields: []FieldData{
				{Key: "groups.0.name", Value: "New", FieldType: "string"},
			},
			want: `{"groups":{"0":{"name":"New"}}}`,
		},
		{
			name:  "new numeric object IDs do not create arrays",
			input: "{}",
			fields: []FieldData{
				{Key: "players.15637.name", Value: "Player", FieldType: "string"},
			},
			want: `{"players":{"15637":{"name":"Player"}}}`,
		},
		{
			name:  "preserve sparse array positions",
			input: `{"groups":[]}`,
			fields: []FieldData{
				{Key: "groups.2.name", Value: "Third", FieldType: "string"},
				{Key: "groups.0.name", Value: "First", FieldType: "string"},
			},
			want: `{"groups":[{"name":"First"},null,{"name":"Third"}]}`,
		},
		{
			name:  "preserve unchanged advanced nulls",
			input: `{"unknown":[null,"keep"]}`,
			advanced: []AdvancedFieldData{
				{Key: "unknown.0", Value: ""},
				{Key: "unknown.1", Value: "keep"},
			},
			want: `{"unknown":[null,"keep"]}`,
		},
		{
			name:  "create advanced scalar arrays",
			input: "{}",
			advanced: []AdvancedFieldData{
				{Key: "tags.0", Value: ""},
				{Key: "tags.2", Value: "Exploration"},
				{Key: "tags.1", Value: "English"},
			},
			want: `{"tags":["","English","Exploration"]}`,
		},
		{
			name:  "create scalar arrays",
			input: "{}",
			fields: []FieldData{
				{Key: "tags.0", Value: "English", FieldType: "string"},
				{Key: "tags.1", Value: "Exploration", FieldType: "string"},
			},
			want: `{"tags":["English","Exploration"]}`,
		},
		{
			name:  "create nested arrays",
			input: "{}",
			fields: []FieldData{
				{Key: "values.0.0", Value: "1.5", FieldType: "number"},
			},
			want: `{"values":[[1.5]]}`,
		},
		{
			name:  "noncanonical numbers remain object keys",
			input: "{}",
			fields: []FieldData{
				{Key: "leading.01.name", Value: "Leading zero", FieldType: "string"},
				{Key: "signed.+1.name", Value: "Plus sign", FieldType: "string"},
				{Key: "negative.-1.name", Value: "Negative", FieldType: "string"},
			},
			want: `{"leading":{"01":{"name":"Leading zero"}},"signed":{"+1":{"name":"Plus sign"}},"negative":{"-1":{"name":"Negative"}}}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parser := &cfgparse.JSONParser{}
			root, errParse := parser.Parse([]byte(test.input))
			if errParse != nil {
				t.Fatal(errParse)
			}
			schema := SchemaDefinition{Properties: map[string]SchemaProperty{}}
			for _, field := range test.fields {
				schema.Properties[field.Key] = SchemaProperty{Type: field.FieldType}
			}
			MergeStructuredFields(root, test.fields, test.advanced, schema)
			output, errWrite := parser.Write(root)
			if errWrite != nil {
				t.Fatal(errWrite)
			}
			assertStructuredJSON(t, output, test.want)
		})
	}
}

func TestPreStartStructuredDefaults(t *testing.T) {
	tests := []struct {
		name       string
		properties map[string]SchemaProperty
		want       string
	}{
		{
			name: "generate array defaults",
			properties: map[string]SchemaProperty{
				"userGroups.0.name":          {Type: "string", Default: "Friends"},
				"userGroups.0.reservedSlots": {Type: "integer", Default: 0},
				"userGroups.0.canKickBan":    {Type: "boolean", Default: false},
			},
			want: `{"queryPort":15637,"userGroups":[{"name":"Friends","reservedSlots":0,"canKickBan":false}]}`,
		},
		{
			name: "omit unspecified defaults but retain explicit zero values",
			properties: map[string]SchemaProperty{
				"userGroups.0.name":          {Type: "string"},
				"userGroups.0.password":      {Type: "string"},
				"userGroups.0.canKickBan":    {Type: "boolean"},
				"userGroups.0.reservedSlots": {Type: "integer"},
				"empty":                      {Type: "string", Default: ""},
				"disabled":                   {Type: "boolean", Default: false},
				"zero":                       {Type: "integer", Default: 0},
			},
			want: `{"queryPort":15637,"empty":"","disabled":false,"zero":0}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.properties["queryPort"] = SchemaProperty{Type: "integer"}
			entry := ConfigSchemaEntry{
				Path:                "server.json",
				Format:              "json",
				GenerateBeforeStart: true,
				ManagedFields:       map[string]string{"queryPort": "game_server.query_port"},
				Schema:              SchemaDefinition{Properties: test.properties},
			}
			parser, errParser := ParserForEntry(entry)
			if errParser != nil {
				t.Fatal(errParser)
			}
			output, errOutput := preStartOutput(parser, entry, nil, false, ServerSettingsResolver("", 15637, 15637))
			if errOutput != nil {
				t.Fatal(errOutput)
			}
			assertStructuredJSON(t, output, test.want)
		})
	}
}

func assertStructuredJSON(t *testing.T, output []byte, expected string) {
	t.Helper()
	var got, want any
	errOutput := json.Unmarshal(output, &got)
	if errOutput != nil {
		t.Fatal(errOutput)
	}
	errExpected := json.Unmarshal([]byte(expected), &want)
	if errExpected != nil {
		t.Fatal(errExpected)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("config = %s, want %s", output, expected)
	}
}
