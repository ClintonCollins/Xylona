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
	}
}
