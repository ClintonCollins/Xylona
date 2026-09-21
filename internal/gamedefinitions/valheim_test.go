package gamedefinitions

import (
	"encoding/json"
	"testing"
)

func TestValheimDefinition(t *testing.T) {
	data, errRead := FS.ReadFile("official/valheim.json")
	if errRead != nil {
		t.Fatal(errRead)
	}
	var document Document
	errDecode := json.Unmarshal(data, &document)
	if errDecode != nil {
		t.Fatal(errDecode)
	}
	hash, errHash := HashDocument(document)
	if errHash != nil {
		t.Fatal(errHash)
	}
	if document.ContentHash != hash {
		t.Fatalf("Valheim definition hash must be %s", hash)
	}
	parsed, errParse := Parse(data)
	if errParse != nil {
		t.Fatal(errParse)
	}
	issues := ValidateModel(parsed.Model)
	if len(issues) != 0 {
		t.Fatalf("invalid Valheim definition: %v", issues)
	}
}
