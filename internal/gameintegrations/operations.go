package gameintegrations

import "slices"

// PlayerIdentityPattern is the native platform-prefixed identity shape accepted by built-in operations.
const PlayerIdentityPattern = "^[A-Za-z]+_[A-Za-z0-9_]+$"

// OperationRisk classifies the operator attention an operation requires.
type OperationRisk uint8

// OperationRisk values are the supported operator review levels.
const (
	OperationRiskRoutine OperationRisk = iota + 1
	OperationRiskCaution
	OperationRiskIrreversible
)

// OperationFieldType identifies a transport-neutral operation input.
type OperationFieldType uint8

// OperationFieldType values are the supported semantic inputs.
const (
	OperationFieldText OperationFieldType = iota + 1
	OperationFieldInteger
	OperationFieldBoolean
	OperationFieldEnum
	OperationFieldDuration
	OperationFieldPlayerIdentity
)

// OperationFieldOption is a trusted catalog or game-authoritative choice.
type OperationFieldOption struct {
	Label       string
	Value       string
	Description string
}

// OperationField describes one semantic input without exposing its native transport.
type OperationField struct {
	ID                string
	Label             string
	Description       string
	Type              OperationFieldType
	Required          bool
	DefaultValue      string
	Options           []OperationFieldOption
	AllowManual       bool
	AllowExactValue   bool
	ValidationPattern string
	MinValue          *int32
	MaxValue          *int32
}

// OperationReview describes the intended effect shown before execution.
type OperationReview struct {
	Title   string
	Effect  string
	Caution string
}

// OperationDescriptor is the integration-owned, transport-neutral operation contract.
type OperationDescriptor struct {
	ID                       string
	Name                     string
	Summary                  string
	Category                 string
	PermissionID             string
	Risk                     OperationRisk
	AvailabilityRequirements []string
	Fields                   []OperationField
	Review                   OperationReview
	RendererKey              string
}

var operationsByGame = map[string][]OperationDescriptor{
	"7_days_to_die": {
		{
			ID:           "player_access.add_administrator",
			Name:         "Add administrator",
			Summary:      "Grant a Player an explicit native permission level.",
			Category:     "Player access",
			PermissionID: "game_server.players.manage",
			Risk:         OperationRiskCaution,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native game-permission API available",
			},
			Fields: []OperationField{
				{
					ID:                "player",
					Label:             "Player",
					Description:       "Choose a known Player or enter a stable platform identity.",
					Type:              OperationFieldPlayerIdentity,
					Required:          true,
					AllowManual:       true,
					ValidationPattern: PlayerIdentityPattern,
				},
				{
					ID:              "permission_level",
					Label:           "Permission level",
					Description:     "Lower native values grant more access; 0 is maximum and 1000 is the default Player level.",
					Type:            OperationFieldInteger,
					Required:        true,
					DefaultValue:    "0",
					AllowExactValue: true,
					MinValue:        new(int32(0)),
					MaxValue:        new(int32(1000)),
					Options: []OperationFieldOption{
						{Label: "Maximum permission", Value: "0", Description: "Native level 0"},
						{Label: "Default Player level", Value: "1000", Description: "Native level 1000"},
					},
				},
			},
			Review: OperationReview{
				Title:   "Review administrator access",
				Effect:  "The selected Player will be added as an administrator at the chosen native permission level.",
				Caution: "Lower permission levels grant more access. Confirm the Player identity and exact value before execution.",
			},
		},
	},
}

// OperationsForGame returns a detached copy of the built-in catalog for a game ID.
func OperationsForGame(gameID string) []OperationDescriptor {
	operations := slices.Clone(operationsByGame[gameID])
	for index := range operations {
		operations[index].AvailabilityRequirements = slices.Clone(operations[index].AvailabilityRequirements)
		operations[index].Fields = slices.Clone(operations[index].Fields)
		for fieldIndex := range operations[index].Fields {
			field := &operations[index].Fields[fieldIndex]
			field.Options = slices.Clone(field.Options)
			field.MinValue = cloneOperationInt32(field.MinValue)
			field.MaxValue = cloneOperationInt32(field.MaxValue)
		}
	}
	return operations
}

func cloneOperationInt32(value *int32) *int32 {
	if value == nil {
		return nil
	}
	return new(*value)
}
