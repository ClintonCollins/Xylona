package gameintegrations

// ValheimIdentityPattern is the identity shape captured in the stored-file fixtures.
const ValheimIdentityPattern = "^Steam_[0-9]{17}$"

// ValheimAccessOperation resolves only the nine built-in stored access intents.
func ValheimAccessOperation(id string) (kind, action string, found bool) {
	for _, list := range []string{"administrators", "bans", "permitted"} {
		for _, intent := range []string{"list", "add", "remove"} {
			if id == "valheim.access."+list+"."+intent {
				return list, intent, true
			}
		}
	}
	return "", "", false
}

func valheimOperations() []OperationDescriptor {
	var operations []OperationDescriptor
	for _, kind := range []string{"administrators", "bans", "permitted"} {
		for _, action := range []string{"list", "add", "remove"} {
			operation := OperationDescriptor{
				ID:       "valheim.access." + kind + "." + action,
				Name:     action + " stored " + kind,
				Summary:  "Inspect or change stored access identities; in-game enforcement is not verified.",
				Category: "Stored access", PermissionID: "game_server.players.manage",
				Risk: OperationRiskRoutine, RendererKey: "valheim_access",
				AvailabilityRequirements: []string{"Node supports Valheim stored access"},
				Review:                   OperationReview{Title: "Review stored access", Effect: "Changes apply on next start; in-game enforcement is not verified.", Caution: "Permitted-list changes can exclude other players. Empty-list behavior is not verified."},
				Concurrency:              OperationConcurrency{Lock: "valheim_stored_access"},
			}
			if action != "list" {
				operation.Risk = OperationRiskCaution
				operation.AvailabilityRequirements = append(operation.AvailabilityRequirements, "Server stopped")
				operation.Fields = []OperationField{
					{ID: "player", Label: "Exact identity", Description: "Captured supported form: Steam_ followed by 17 digits. Preserve case exactly.", Type: OperationFieldPlayerIdentity, Required: true, AllowManual: true, ValidationPattern: ValheimIdentityPattern},
					{ID: "expected_revision", Label: "Stored list revision", Description: "Read the list before changing it.", Type: OperationFieldText, Required: true, ValidationPattern: "^[a-f0-9]{64}$"},
				}
			}
			operations = append(operations, operation)
		}
	}
	return operations
}
