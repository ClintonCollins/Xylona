package gameintegrations

import (
	"strings"
	"testing"
)

func TestOperationsForGame(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		gameID    string
		wantCount int
	}{
		{name: "7 Days to Die exposes its approved operation catalog", gameID: "7_days_to_die", wantCount: 29},
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

			byID := make(map[string]OperationDescriptor, len(operations))
			for _, operation := range operations {
				if operation.ID == "" || operation.Name == "" || operation.Summary == "" || operation.Category == "" ||
					operation.PermissionID == "" || operation.Risk == 0 || operation.NativeCapability == 0 ||
					len(operation.AvailabilityRequirements) == 0 || operation.Review.Effect == "" || operation.RendererKey != "" {
					t.Fatalf("operation descriptor is incomplete or bypasses the generic renderer: %+v", operation)
				}
				_, duplicate := byID[operation.ID]
				if duplicate {
					t.Fatalf("duplicate operation ID %q", operation.ID)
				}
				byID[operation.ID] = operation
			}

			expected := []struct {
				id         string
				category   string
				permission string
				capability OperationNativeCapability
				searchTerm string
			}{
				{id: OperationIDAddAdministrator, category: "Player access", permission: "game_server.players.manage", capability: OperationNativeCapabilityGamePermissions, searchTerm: "administrator"},
				{id: OperationIDRemoveAdministrator, category: "Player access", permission: "game_server.players.manage", capability: OperationNativeCapabilityGamePermissions, searchTerm: "administrator"},
				{id: OperationIDSetCommandPermission, category: "Permissions", permission: "game_server.console", capability: OperationNativeCapabilityCommandPermissions, searchTerm: "permission"},
				{id: OperationIDResetCommandPermission, category: "Permissions", permission: "game_server.console", capability: OperationNativeCapabilityCommandPermissions, searchTerm: "permission"},
				{id: OperationIDKickPlayer, category: "Player moderation", permission: "game_server.players.manage", capability: OperationNativeCapabilityPlayerActions, searchTerm: "disconnect"},
				{id: OperationIDBanPlayer, category: "Player moderation", permission: "game_server.players.manage", capability: OperationNativeCapabilityPlayerActions, searchTerm: "block"},
				{id: OperationIDUnbanPlayer, category: "Player moderation", permission: "game_server.players.manage", capability: OperationNativeCapabilityPlayerActions, searchTerm: "ban"},
				{id: OperationIDAllowlistAdd, category: "Player access", permission: "game_server.players.manage", capability: OperationNativeCapabilityPlayerActions, searchTerm: "allow"},
				{id: OperationIDAllowlistRemove, category: "Player access", permission: "game_server.players.manage", capability: OperationNativeCapabilityPlayerActions, searchTerm: "allowlist"},
				{id: OperationIDTeleportPlayer, category: "Player assistance", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "move"},
				{id: OperationIDGiveItem, category: "Player assistance", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "item"},
				{id: OperationIDGiveExperience, category: "Player assistance", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "experience"},
				{id: OperationIDApplyBuff, category: "Player assistance", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "buff"},
				{id: OperationIDRemoveBuff, category: "Player assistance", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "buff"},
				{id: OperationIDBroadcastMessage, category: "Communication", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "announcement"},
				{id: OperationIDMessagePlayer, category: "Communication", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "private"},
				{id: OperationIDGamePreferences, category: "Server information", permission: "game_server.view", capability: OperationNativeCapabilityCommand, searchTerm: "configuration"},
				{id: OperationIDGameStatistics, category: "Server information", permission: "game_server.view", capability: OperationNativeCapabilityCommand, searchTerm: "statistics"},
				{id: OperationIDGameTime, category: "Server information", permission: "game_server.view", capability: OperationNativeCapabilityCommand, searchTerm: "time"},
				{id: OperationIDDLCStatus, category: "Server information", permission: "game_server.view", capability: OperationNativeCapabilityCommand, searchTerm: "entitlement"},
				{id: OperationIDItemSearch, category: "Server information", permission: "game_server.view", capability: OperationNativeCapabilityCommand, searchTerm: "item"},
				{id: OperationIDVersion, category: "Server information", permission: "game_server.view", capability: OperationNativeCapabilityCommand, searchTerm: "mods"},
				{id: OperationIDSpawnAirdrop, category: "World events", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "airdrop"},
				{id: OperationIDSpawnWanderingHorde, category: "World events", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "horde"},
				{id: OperationIDSetWeather, category: "World events", permission: "game_server.console", capability: OperationNativeCapabilityCommand, searchTerm: "weather"},
			}
			for _, want := range expected {
				operation, found := byID[want.id]
				if !found {
					t.Errorf("operation %q is missing", want.id)
					continue
				}
				if operation.Category != want.category || operation.PermissionID != want.permission || operation.NativeCapability != want.capability {
					t.Errorf("operation %q policy = %+v", want.id, operation)
				}
				searchCopy := strings.ToLower(operation.Name + " " + operation.Summary)
				if !strings.Contains(searchCopy, want.searchTerm) {
					t.Errorf("operation %q is not discoverable by intent %q: %q", want.id, want.searchTerm, searchCopy)
				}
			}

			addAdministrator := byID[OperationIDAddAdministrator]
			if len(addAdministrator.Fields) != 2 || addAdministrator.Concurrency.TargetField != "player" {
				t.Fatalf("Add administrator descriptor is incomplete: %+v", addAdministrator)
			}
			playerField := addAdministrator.Fields[0]
			if playerField.Type != OperationFieldPlayerIdentity || !playerField.Required || !playerField.AllowManual ||
				playerField.ValidationPattern == "" {
				t.Fatalf("Player identity field = %+v", playerField)
			}
			permissionField := addAdministrator.Fields[1]
			if permissionField.Type != OperationFieldInteger || !permissionField.AllowExactValue ||
				permissionField.MinValue == nil || *permissionField.MinValue != 0 ||
				permissionField.MaxValue == nil || *permissionField.MaxValue != 1000 || len(permissionField.Options) < 2 {
				t.Fatalf("Permission level field = %+v", permissionField)
			}
			broadcastFields := byID["communication.broadcast_message"].Fields
			if len(broadcastFields) != 1 || broadcastFields[0].ID != "message" ||
				broadcastFields[0].Type != OperationFieldText || !broadcastFields[0].Required {
				t.Fatalf("Broadcast message fields = %+v", broadcastFields)
			}
			privateMessageFields := byID["communication.message_player"].Fields
			if len(privateMessageFields) != 2 || privateMessageFields[0].Type != OperationFieldPlayerIdentity ||
				privateMessageFields[1].ID != "message" || privateMessageFields[1].Type != OperationFieldText {
				t.Fatalf("Private message fields = %+v", privateMessageFields)
			}
			itemSearchFields := byID["server_information.item_search"].Fields
			if len(itemSearchFields) != 1 || itemSearchFields[0].ID != "search" ||
				itemSearchFields[0].Type != OperationFieldText || itemSearchFields[0].DefaultValue != "*" {
				t.Fatalf("Item search fields = %+v", itemSearchFields)
			}

			setTime := byID["server_control.set_game_time"]
			if setTime.Category != "Server control" || setTime.PermissionID != "game_server.console" ||
				setTime.Risk != OperationRiskCaution || setTime.NativeCapability != OperationNativeCapabilityCommand ||
				len(setTime.Fields) != 1 || setTime.Fields[0].Type != OperationFieldText ||
				!setTime.Fields[0].AllowManual || setTime.Fields[0].ValidationPattern == "" ||
				len(setTime.Fields[0].Options) != 2 || setTime.Concurrency.Lock != "world_time" ||
				len(setTime.Concurrency.ConflictsWith) != 1 ||
				setTime.Concurrency.ConflictsWith[0] != "server_control.shutdown" {
				t.Fatalf("Set game time descriptor = %+v", setTime)
			}

			saveWorld := byID["server_control.save_world"]
			if saveWorld.Category != "Server control" || saveWorld.PermissionID != "game_server.console" ||
				saveWorld.Risk != OperationRiskRoutine ||
				saveWorld.NativeCapability != OperationNativeCapabilityCommand || len(saveWorld.Fields) != 0 ||
				saveWorld.Review.Effect == "" || saveWorld.Concurrency.Lock != "" ||
				len(saveWorld.Concurrency.ConflictsWith) != 0 {
				t.Fatalf("Save world descriptor = %+v", saveWorld)
			}

			setTemperatureUnit := byID["server_control.set_temperature_unit"]
			if setTemperatureUnit.Category != "Server control" ||
				setTemperatureUnit.PermissionID != "game_server.console" ||
				setTemperatureUnit.Risk != OperationRiskRoutine ||
				setTemperatureUnit.NativeCapability != OperationNativeCapabilityCommand ||
				len(setTemperatureUnit.Fields) != 1 ||
				setTemperatureUnit.Fields[0].Type != OperationFieldEnum ||
				len(setTemperatureUnit.Fields[0].Options) != 2 ||
				setTemperatureUnit.Review.Effect == "" || setTemperatureUnit.Concurrency.Lock != "" ||
				len(setTemperatureUnit.Concurrency.ConflictsWith) != 0 {
				t.Fatalf("Set temperature unit descriptor = %+v", setTemperatureUnit)
			}

			shutdown := byID["server_control.shutdown"]
			if shutdown.Category != "Server control" || shutdown.PermissionID != "game_server.stop" ||
				shutdown.Risk != OperationRiskIrreversible ||
				shutdown.NativeCapability != OperationNativeCapabilityCommand || len(shutdown.Fields) != 0 ||
				len(shutdown.Concurrency.ConflictsWith) != 1 ||
				shutdown.Concurrency.ConflictsWith[0] != "server_control.set_game_time" {
				t.Fatalf("Shut down server descriptor = %+v", shutdown)
			}

			operations[0].Name = "changed"
			setTime.Concurrency.ConflictsWith[0] = "changed"
			if OperationsForGame(test.gameID)[0].Name == "changed" {
				t.Fatal("OperationsForGame returned mutable shared catalog state")
			}
			for _, operation := range OperationsForGame(test.gameID) {
				if operation.ID == "server_control.set_game_time" && operation.Concurrency.ConflictsWith[0] == "changed" {
					t.Fatal("OperationsForGame returned mutable concurrency metadata")
				}
			}
		})
	}
}
