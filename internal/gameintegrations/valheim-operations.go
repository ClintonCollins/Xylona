package gameintegrations

import "strings"

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

// valheimAccessCopy is the operator-facing wording for one stored list and its three intents.
type valheimAccessCopy struct {
	readName   string
	listLabel  string
	addName    string
	removeName string
	caution    string
}

var valheimAccessCopyByKind = map[string]valheimAccessCopy{
	"administrators": {
		readName:   "Read administrators",
		listLabel:  "administrator list",
		addName:    "Add administrator",
		removeName: "Remove administrator",
		caution:    "In-game enforcement is not verified.",
	},
	"bans": {
		readName:   "Read bans",
		listLabel:  "ban list",
		addName:    "Ban player",
		removeName: "Unban player",
		caution:    "In-game enforcement is not verified.",
	},
	"permitted": {
		readName:   "Read permitted players",
		listLabel:  "permitted list",
		addName:    "Add permitted player",
		removeName: "Remove permitted player",
		caution:    "While the permitted list has entries, players not on it may be unable to join. Empty-list behavior and in-game enforcement are not verified.",
	},
}

func valheimOperations() []OperationDescriptor {
	var operations []OperationDescriptor
	for _, kind := range []string{"administrators", "bans", "permitted"} {
		copyText := valheimAccessCopyByKind[kind]
		for _, action := range []string{"list", "add", "remove"} {
			operation := OperationDescriptor{
				ID:       "valheim.access." + kind + "." + action,
				Name:     copyText.readName,
				Summary:  "Read the stored " + copyText.listLabel + ".",
				Category: "Stored access", PermissionID: "game_server.players.manage",
				Risk: OperationRiskRoutine, RendererKey: "valheim_access",
				AvailabilityRequirements: []string{"Node supports Valheim stored access"},
				Review:                   OperationReview{Title: "Review " + copyText.listLabel, Effect: "The stored " + copyText.listLabel + " will be read."},
				Concurrency:              OperationConcurrency{Lock: "valheim_stored_access"},
			}
			if action != "list" {
				direction := "added to"
				operation.Name = copyText.addName
				if action == "remove" {
					direction = "removed from"
					operation.Name = copyText.removeName
				}
				operation.Summary = "The identity is " + direction + " the stored " + copyText.listLabel + " and applies on the next start."
				operation.Review = OperationReview{
					Title:   "Review " + strings.ToLower(operation.Name),
					Effect:  "The identity will be " + direction + " the stored " + copyText.listLabel + ". The change applies the next time the server starts.",
					Caution: copyText.caution,
				}
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
