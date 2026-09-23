package gameintegrations

import "testing"

func TestValheimOperations(t *testing.T) {
	operations := OperationsForGame("valheim")
	if len(operations) != 9 {
		t.Fatalf("operations = %d", len(operations))
	}
	for _, operation := range operations {
		_, action, found := ValheimAccessOperation(operation.ID)
		if !found || operation.PermissionID != "game_server.players.manage" {
			t.Fatalf("invalid authorized catalog entry: %+v", operation)
		}
		if action != "list" && (len(operation.Fields) != 2 || operation.Risk != OperationRiskCaution) {
			t.Fatalf("mutation lacks review or revision: %+v", operation)
		}
		if action != "list" && (operation.Review.Title == "" || operation.Review.Effect == "" || operation.Review.Caution == "") {
			t.Fatalf("mutation lacks review copy: %+v", operation.Review)
		}
	}
}

func TestValheimOperationNames(t *testing.T) {
	names := make(map[string]string)
	for _, operation := range OperationsForGame("valheim") {
		names[operation.ID] = operation.Name
	}
	tests := []struct {
		id   string
		name string
	}{
		{id: "valheim.access.administrators.list", name: "Read administrators"},
		{id: "valheim.access.administrators.add", name: "Add administrator"},
		{id: "valheim.access.administrators.remove", name: "Remove administrator"},
		{id: "valheim.access.bans.add", name: "Ban player"},
		{id: "valheim.access.bans.remove", name: "Unban player"},
		{id: "valheim.access.permitted.add", name: "Add permitted player"},
		{id: "valheim.access.permitted.remove", name: "Remove permitted player"},
	}
	for _, test := range tests {
		if names[test.id] != test.name {
			t.Errorf("%s name = %q, want %q", test.id, names[test.id], test.name)
		}
	}
}
