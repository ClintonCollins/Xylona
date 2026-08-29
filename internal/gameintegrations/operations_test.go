package gameintegrations

import "testing"

func TestOperationsForGame(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		gameID    string
		wantCount int
	}{
		{name: "7 Days to Die exposes Add administrator", gameID: "7_days_to_die", wantCount: 1},
		{name: "unknown games have no operations", gameID: "unknown", wantCount: 0},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			operations := OperationsForGame(test.gameID)
			if len(operations) != test.wantCount {
				t.Fatalf("OperationsForGame(%q) count = %d, want %d", test.gameID, len(operations), test.wantCount)
			}
			if test.wantCount == 0 {
				return
			}

			operation := operations[0]
			if operation.ID != "player_access.add_administrator" || operation.Category != "Player access" ||
				operation.PermissionID != "game_server.players.manage" || operation.Risk != OperationRiskCaution {
				t.Fatalf("Add administrator identity and policy = %+v", operation)
			}
			if len(operation.AvailabilityRequirements) != 3 || len(operation.Fields) != 2 || operation.Review.Effect == "" {
				t.Fatalf("Add administrator descriptor is incomplete: %+v", operation)
			}
			playerField := operation.Fields[0]
			if playerField.Type != OperationFieldPlayerIdentity || !playerField.Required || !playerField.AllowManual ||
				playerField.ValidationPattern == "" {
				t.Fatalf("Player identity field = %+v", playerField)
			}
			permissionField := operation.Fields[1]
			if permissionField.Type != OperationFieldInteger || !permissionField.AllowExactValue ||
				permissionField.MinValue == nil || *permissionField.MinValue != 0 ||
				permissionField.MaxValue == nil || *permissionField.MaxValue != 1000 || len(permissionField.Options) < 2 {
				t.Fatalf("Permission level field = %+v", permissionField)
			}

			operations[0].Name = "changed"
			if OperationsForGame(test.gameID)[0].Name == "changed" {
				t.Fatal("OperationsForGame returned mutable shared catalog state")
			}
		})
	}
}
