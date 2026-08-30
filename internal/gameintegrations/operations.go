package gameintegrations

import "slices"

const (
	// PlayerIdentityPattern is the native platform-prefixed identity shape accepted by built-in operations.
	PlayerIdentityPattern = "^[A-Za-z]+_[A-Za-z0-9_]+$"
	// PlayerActionIdentityPattern preserves the bounded, quoted identifier accepted by typed 7 Days to Die actions.
	PlayerActionIdentityPattern = `^[^"\\\x00-\x1F\x7F]{1,256}$`
	// PlayerActionReasonPattern preserves the optional bounded reason accepted by typed 7 Days to Die actions.
	PlayerActionReasonPattern = `^[^\x00-\x1F\x7F]{0,256}$`
	// CommandArgumentPattern accepts one bounded quoted native command argument.
	CommandArgumentPattern = `^[^"\\\x00-\x1F\x7F]{1,128}$`
	// CommandNamePattern is the bounded native command identifier accepted by permission operations.
	CommandNamePattern = "^[A-Za-z0-9_-][A-Za-z0-9 _-]{0,127}$"
	// GameTimePattern accepts the captured presets or an exact day, hour, and minute.
	GameTimePattern = `^(?:day|night|[1-9][0-9]{0,9} (?:[0-9]|1[0-9]|2[0-3]) (?:[0-9]|[1-5][0-9]))$`
)

// Stable built-in operation IDs.
const (
	OperationIDAddAdministrator       = "player_access.add_administrator"
	OperationIDRemoveAdministrator    = "player_access.remove_administrator"
	OperationIDSetCommandPermission   = "permissions.set_command_permission"
	OperationIDResetCommandPermission = "permissions.reset_command_permission"
	OperationIDKickPlayer             = "player_moderation.kick"
	OperationIDBanPlayer              = "player_moderation.ban"
	OperationIDUnbanPlayer            = "player_moderation.unban"
	OperationIDAllowlistAdd           = "player_access.allowlist_add"
	OperationIDAllowlistRemove        = "player_access.allowlist_remove"
	OperationIDTeleportPlayer         = "player_assistance.teleport_to_player"
	OperationIDGiveItem               = "player_assistance.give_item"
	OperationIDGiveExperience         = "player_assistance.give_experience"
	OperationIDApplyBuff              = "player_assistance.apply_buff"
	OperationIDRemoveBuff             = "player_assistance.remove_buff"
	OperationIDBroadcastMessage       = "communication.broadcast_message"
	OperationIDMessagePlayer          = "communication.message_player"
	OperationIDGamePreferences        = "server_information.game_preferences"
	OperationIDGameStatistics         = "server_information.game_statistics"
	OperationIDGameTime               = "server_information.game_time"
	OperationIDDLCStatus              = "server_information.dlc_status"
	OperationIDItemSearch             = "server_information.item_search"
	OperationIDVersion                = "server_information.version"
	OperationIDSaveWorld              = "server_control.save_world"
	OperationIDSetGameTime            = "server_control.set_game_time"
	OperationIDSetTemperatureUnit     = "server_control.set_temperature_unit"
	OperationIDShutdown               = "server_control.shutdown"
	OperationIDSpawnAirdrop           = "world_events.spawn_airdrop"
	OperationIDSpawnWanderingHorde    = "world_events.spawn_wandering_horde"
	OperationIDSetWeather             = "world_events.set_weather"
)

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

// OperationNativeCapability identifies the native contract required at execution time.
type OperationNativeCapability uint8

// OperationNativeCapability values are the supported native execution contracts.
const (
	OperationNativeCapabilityGamePermissions OperationNativeCapability = iota + 1
	OperationNativeCapabilityCommand
	OperationNativeCapabilityCommandPermissions
	OperationNativeCapabilityPlayerActions
)

// OperationConcurrency declares only demonstrated conflicts and same-target races.
type OperationConcurrency struct {
	Lock          string
	TargetField   string
	ConflictsWith []string
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
	NativeCapability         OperationNativeCapability
	Concurrency              OperationConcurrency
}

var commandOperationAvailabilityRequirements = []string{
	"Server online",
	"Node supports game operations",
	"Native command API available",
}

var operationsByGame = map[string][]OperationDescriptor{
	"7_days_to_die": {
		{
			ID:           OperationIDAddAdministrator,
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
			NativeCapability: OperationNativeCapabilityGamePermissions,
			Concurrency: OperationConcurrency{
				Lock:        "administrator_access",
				TargetField: "player",
			},
		},
		{
			ID:           OperationIDRemoveAdministrator,
			Name:         "Remove administrator",
			Summary:      "Remove a Player's explicit native permission level.",
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
			},
			Review: OperationReview{
				Title:   "Review administrator removal",
				Effect:  "The selected Player's explicit administrator permission will be removed.",
				Caution: "Confirm the stable Player identity before removing administrator access.",
			},
			NativeCapability: OperationNativeCapabilityGamePermissions,
			Concurrency: OperationConcurrency{
				Lock:        "administrator_access",
				TargetField: "player",
			},
		},
		{
			ID:           OperationIDSetCommandPermission,
			Name:         "Set command permission",
			Summary:      "Set the exact native permission level required for a server command.",
			Category:     "Permissions",
			PermissionID: "game_server.console",
			Risk:         OperationRiskCaution,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native command-permission API available",
			},
			Fields: []OperationField{
				{
					ID:                "command",
					Label:             "Command",
					Description:       "Enter the exact native command name reported by the game server.",
					Type:              OperationFieldText,
					Required:          true,
					AllowManual:       true,
					ValidationPattern: CommandNamePattern,
				},
				{
					ID:              "permission_level",
					Label:           "Permission level",
					Description:     "Lower native values grant more access; enter an exact value from 0 through 1000.",
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
				Title:   "Review command permission",
				Effect:  "The selected command will require the chosen exact native permission level.",
				Caution: "Lower permission levels grant more access. Confirm the command name and exact value before execution.",
			},
			NativeCapability: OperationNativeCapabilityCommandPermissions,
			Concurrency: OperationConcurrency{
				Lock:        "command_permission",
				TargetField: "command",
			},
		},
		{
			ID:           OperationIDResetCommandPermission,
			Name:         "Reset command permission",
			Summary:      "Remove an explicit command override and restore the game's native default.",
			Category:     "Permissions",
			PermissionID: "game_server.console",
			Risk:         OperationRiskCaution,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native command-permission API available",
			},
			Fields: []OperationField{
				{
					ID:                "command",
					Label:             "Command",
					Description:       "Enter the exact native command name reported by the game server.",
					Type:              OperationFieldText,
					Required:          true,
					AllowManual:       true,
					ValidationPattern: CommandNamePattern,
				},
			},
			Review: OperationReview{
				Title:   "Review command permission reset",
				Effect:  "The selected command's explicit permission override will be removed.",
				Caution: "The command will return to the permission level chosen by the game server.",
			},
			NativeCapability: OperationNativeCapabilityCommandPermissions,
			Concurrency: OperationConcurrency{
				Lock:        "command_permission",
				TargetField: "command",
			},
		},
		playerActionOperation(
			OperationIDKickPlayer,
			"Kick Player",
			"Disconnect a Player from the game server.",
			"Player moderation",
			OperationRiskCaution,
			"The selected Player will be disconnected from the game server.",
			"The game accepts an optional reason, but does not provide authoritative state read-back.",
			true,
		),
		playerActionOperation(
			OperationIDBanPlayer,
			"Ban Player",
			"Permanently block a Player from joining the game server.",
			"Player moderation",
			OperationRiskCaution,
			"The selected Player will be permanently banned using the existing 0-minute native duration.",
			"Confirm the Player identity. The game accepts an optional reason but does not provide authoritative state read-back.",
			true,
		),
		playerActionOperation(
			OperationIDUnbanPlayer,
			"Unban Player",
			"Remove a Player ban.",
			"Player moderation",
			OperationRiskRoutine,
			"The selected Player will be removed from the game server ban list.",
			"The game does not provide authoritative state read-back for this action.",
			false,
		),
		playerActionOperation(
			OperationIDAllowlistAdd,
			"Add to allowlist",
			"Allow a Player to join an allowlisted game server.",
			"Player access",
			OperationRiskRoutine,
			"The selected Player will be added to the game server allowlist.",
			"The game does not provide authoritative state read-back for this action.",
			false,
		),
		playerActionOperation(
			OperationIDAllowlistRemove,
			"Remove from allowlist",
			"Remove a Player from the game server allowlist.",
			"Player access",
			OperationRiskCaution,
			"The selected Player will be removed from the game server allowlist.",
			"Confirm the Player identity. The game does not provide authoritative state read-back for this action.",
			false,
		),
		commandOperation(
			OperationIDTeleportPlayer,
			"Teleport Player",
			"Move one online Player to another online Player.",
			"Player assistance",
			[]OperationField{
				playerIdentityOperationField("player", "Player", "Choose the Player to move."),
				playerIdentityOperationField("destination", "Destination Player", "Choose the online Player to use as the destination."),
			},
			OperationReview{
				Title:   "Review Player teleport",
				Effect:  "The selected Player will be moved to the destination Player's current location.",
				Caution: "Both Players must be online. Confirm the source and destination before execution.",
			},
			OperationConcurrency{Lock: "player_assistance", TargetField: "player"},
		),
		commandOperation(
			OperationIDGiveItem,
			"Give item",
			"Drop a stack of an exact item definition in front of a Player.",
			"Player assistance",
			[]OperationField{
				playerIdentityOperationField("player", "Player", "Choose the Player who should receive the item."),
				{
					ID:                "item",
					Label:             "Item",
					Description:       "Enter the exact item name reported by the game server.",
					Type:              OperationFieldText,
					Required:          true,
					AllowManual:       true,
					ValidationPattern: CommandArgumentPattern,
				},
				{
					ID:              "amount",
					Label:           "Amount",
					Description:     "Choose how many items to drop as one stack.",
					Type:            OperationFieldInteger,
					Required:        true,
					DefaultValue:    "1",
					AllowExactValue: true,
					MinValue:        new(int32(1)),
					MaxValue:        new(int32(1000)),
				},
			},
			OperationReview{
				Title:   "Review item grant",
				Effect:  "The chosen item stack will be dropped in front of the selected Player.",
				Caution: "The item name must exactly match a server item definition.",
			},
			OperationConcurrency{Lock: "player_assistance", TargetField: "player"},
		),
		commandOperation(
			OperationIDGiveExperience,
			"Give experience",
			"Grant experience points to a Player.",
			"Player assistance",
			[]OperationField{
				playerIdentityOperationField("player", "Player", "Choose the Player who should receive experience."),
				{
					ID:              "experience",
					Label:           "Experience",
					Description:     "Enter the number of experience points to grant.",
					Type:            OperationFieldInteger,
					Required:        true,
					DefaultValue:    "1000",
					AllowExactValue: true,
					MinValue:        new(int32(1)),
					MaxValue:        new(int32(1000000)),
				},
			},
			OperationReview{
				Title:   "Review experience grant",
				Effect:  "The selected Player will receive the chosen amount of experience.",
				Caution: "Experience changes Player progression and cannot be automatically reversed.",
			},
			OperationConcurrency{Lock: "player_assistance", TargetField: "player"},
		),
		commandOperation(
			OperationIDApplyBuff,
			"Apply buff",
			"Apply an exact game buff to a Player.",
			"Player assistance",
			playerBuffOperationFields("Apply the buff to this Player."),
			OperationReview{
				Title:   "Review buff application",
				Effect:  "The exact buff will be applied to the selected Player.",
				Caution: "The buff name must exactly match a server buff definition.",
			},
			OperationConcurrency{Lock: "player_assistance", TargetField: "player"},
		),
		commandOperation(
			OperationIDRemoveBuff,
			"Remove buff",
			"Remove an exact game buff from a Player.",
			"Player assistance",
			playerBuffOperationFields("Remove the buff from this Player."),
			OperationReview{
				Title:   "Review buff removal",
				Effect:  "The exact buff will be removed from the selected Player.",
				Caution: "The buff name must exactly match an active server buff definition.",
			},
			OperationConcurrency{Lock: "player_assistance", TargetField: "player"},
		),
		commandOperation(
			OperationIDSpawnAirdrop,
			"Spawn airdrop",
			"Trigger one server airdrop event.",
			"World events",
			nil,
			OperationReview{
				Title:   "Review airdrop event",
				Effect:  "The game server will spawn one airdrop event.",
				Caution: "This adds supplies to the active world and cannot be automatically reversed.",
			},
			OperationConcurrency{Lock: "world_events"},
		),
		commandOperation(
			OperationIDSpawnWanderingHorde,
			"Spawn wandering horde",
			"Trigger one wandering horde event.",
			"World events",
			nil,
			OperationReview{
				Title:   "Review wandering horde",
				Effect:  "The game server will spawn one wandering horde.",
				Caution: "This can immediately endanger online Players and cannot be automatically reversed.",
			},
			OperationConcurrency{Lock: "world_events"},
		),
		commandOperation(
			OperationIDSetWeather,
			"Set weather",
			"Restore natural weather or start rain or snowfall.",
			"World events",
			[]OperationField{
				{
					ID:           "weather",
					Label:        "Weather",
					Description:  "Choose one bounded weather change from the captured server command.",
					Type:         OperationFieldEnum,
					Required:     true,
					DefaultValue: "natural",
					Options: []OperationFieldOption{
						{Label: "Natural", Value: "natural", Description: "Restore the game's default weather simulation"},
						{Label: "Rain", Value: "rain", Description: "Set rain intensity to maximum"},
						{Label: "Snow", Value: "snow", Description: "Set snowfall intensity to maximum"},
					},
				},
			},
			OperationReview{
				Title:   "Review weather change",
				Effect:  "The active world weather will change to the selected state.",
				Caution: "Weather changes affect every online Player.",
			},
			OperationConcurrency{Lock: "world_events"},
		),
		{
			ID:           OperationIDSaveWorld,
			Name:         "Save world",
			Summary:      "Save the current world state immediately.",
			Category:     "Server control",
			PermissionID: "game_server.console",
			Risk:         OperationRiskRoutine,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native command API available",
			},
			Review: OperationReview{
				Title:  "Review world save",
				Effect: "The selected game server will save its current world state.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:           OperationIDSetTemperatureUnit,
			Name:         "Set temperature unit",
			Summary:      "Choose how the game reports temperatures.",
			Category:     "Server control",
			PermissionID: "game_server.console",
			Risk:         OperationRiskRoutine,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native command API available",
			},
			Fields: []OperationField{
				{
					ID:           "unit",
					Label:        "Temperature unit",
					Description:  "Choose the temperature unit supported by the captured server command.",
					Type:         OperationFieldEnum,
					Required:     true,
					DefaultValue: "F",
					Options: []OperationFieldOption{
						{Label: "Fahrenheit", Value: "F"},
						{Label: "Celsius", Value: "C"},
					},
				},
			},
			Review: OperationReview{
				Title:  "Review temperature unit change",
				Effect: "The selected game server will report temperatures using the chosen unit.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:           OperationIDSetGameTime,
			Name:         "Set game time",
			Summary:      "Move the current world clock to a preset or an exact day and time.",
			Category:     "Server control",
			PermissionID: "game_server.console",
			Risk:         OperationRiskCaution,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native command API available",
			},
			Fields: []OperationField{
				{
					ID:                "time",
					Label:             "World time",
					Description:       "Choose a preset or enter an exact day, hour, and minute.",
					Type:              OperationFieldText,
					Required:          true,
					DefaultValue:      "day",
					AllowManual:       true,
					ValidationPattern: GameTimePattern,
					Options: []OperationFieldOption{
						{Label: "Day", Value: "day", Description: "Day 1 at 12:00 PM"},
						{Label: "Night", Value: "night", Description: "Day 2 at 12:00 AM"},
					},
				},
			},
			Review: OperationReview{
				Title:   "Review world time change",
				Effect:  "The selected game server's world clock will move to the chosen time.",
				Caution: "Changing world time can affect active Players and time-dependent game events.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
			Concurrency: OperationConcurrency{
				Lock:          "world_time",
				ConflictsWith: []string{OperationIDShutdown},
			},
		},
		{
			ID:           OperationIDShutdown,
			Name:         "Shut down server",
			Summary:      "Stop the running 7 Days to Die server process.",
			Category:     "Server control",
			PermissionID: "game_server.stop",
			Risk:         OperationRiskIrreversible,
			AvailabilityRequirements: []string{
				"Server online",
				"Node supports game operations",
				"Native command API available",
			},
			Review: OperationReview{
				Title:   "Review server shutdown",
				Effect:  "The selected game server will stop and disconnect all active Players.",
				Caution: "This immediately ends the running server process. Start it again from Xylona when you are ready.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
			Concurrency: OperationConcurrency{
				ConflictsWith: []string{OperationIDSetGameTime},
			},
		},
		{
			ID:                       OperationIDBroadcastMessage,
			Name:                     "Send server announcement",
			Summary:                  "Broadcast an announcement to every connected Player.",
			Category:                 "Communication",
			PermissionID:             "game_server.console",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Fields: []OperationField{
				{
					ID:          "message",
					Label:       "Announcement",
					Description: "Enter the message every connected Player should receive.",
					Type:        OperationFieldText,
					Required:    true,
				},
			},
			Review: OperationReview{
				Title:  "Review server announcement",
				Effect: "The announcement will be sent to every connected Player.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDMessagePlayer,
			Name:                     "Send private Player message",
			Summary:                  "Deliver a private message to one Player.",
			Category:                 "Communication",
			PermissionID:             "game_server.console",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
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
					ID:          "message",
					Label:       "Message",
					Description: "Enter the private message this Player should receive.",
					Type:        OperationFieldText,
					Required:    true,
				},
			},
			Review: OperationReview{
				Title:  "Review private message",
				Effect: "The message will be sent only to the selected Player.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDGamePreferences,
			Name:                     "Inspect game preferences",
			Summary:                  "Read current game configuration preferences, optionally filtered by name.",
			Category:                 "Server information",
			PermissionID:             "game_server.view",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Fields: []OperationField{
				{
					ID:          "filter",
					Label:       "Name filter",
					Description: "Optionally show only preference names containing this text.",
					Type:        OperationFieldText,
				},
			},
			Review: OperationReview{
				Title:  "Review preference query",
				Effect: "Current game preferences matching the optional name filter will be read.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDGameStatistics,
			Name:                     "Inspect game statistics",
			Summary:                  "Read current game statistics, optionally filtered by name.",
			Category:                 "Server information",
			PermissionID:             "game_server.view",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Fields: []OperationField{
				{
					ID:          "filter",
					Label:       "Name filter",
					Description: "Optionally show only statistic names containing this text.",
					Type:        OperationFieldText,
				},
			},
			Review: OperationReview{
				Title:  "Review statistics query",
				Effect: "Current game statistics matching the optional name filter will be read.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDGameTime,
			Name:                     "Inspect game time",
			Summary:                  "Read the current in-game day and time.",
			Category:                 "Server information",
			PermissionID:             "game_server.view",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Review: OperationReview{
				Title:  "Review game-time query",
				Effect: "The current in-game day and time will be read.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDDLCStatus,
			Name:                     "Inspect DLC status",
			Summary:                  "List available DLC and current entitlement status.",
			Category:                 "Server information",
			PermissionID:             "game_server.view",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Review: OperationReview{
				Title:  "Review DLC query",
				Effect: "Available DLC and current entitlement states will be read.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDItemSearch,
			Name:                     "Search item definitions",
			Summary:                  "Find available item names by a bounded text search.",
			Category:                 "Server information",
			PermissionID:             "game_server.view",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Fields: []OperationField{
				{
					ID:           "search",
					Label:        "Item search",
					Description:  "Enter part of an item name, or use * to list every item.",
					Type:         OperationFieldText,
					Required:     true,
					DefaultValue: "*",
				},
			},
			Review: OperationReview{
				Title:  "Review item search",
				Effect: "Available item names matching the search text will be read.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
		{
			ID:                       OperationIDVersion,
			Name:                     "Inspect game version",
			Summary:                  "Read the running game version and loaded mods.",
			Category:                 "Server information",
			PermissionID:             "game_server.view",
			Risk:                     OperationRiskRoutine,
			AvailabilityRequirements: commandOperationAvailabilityRequirements,
			Review: OperationReview{
				Title:  "Review version query",
				Effect: "The running game version and loaded mods will be read.",
			},
			NativeCapability: OperationNativeCapabilityCommand,
		},
	},
}

func commandOperation(
	id string,
	name string,
	summary string,
	category string,
	fields []OperationField,
	review OperationReview,
	concurrency OperationConcurrency,
) OperationDescriptor {
	return OperationDescriptor{
		ID:                       id,
		Name:                     name,
		Summary:                  summary,
		Category:                 category,
		PermissionID:             "game_server.console",
		Risk:                     OperationRiskCaution,
		AvailabilityRequirements: commandOperationAvailabilityRequirements,
		Fields:                   fields,
		Review:                   review,
		NativeCapability:         OperationNativeCapabilityCommand,
		Concurrency:              concurrency,
	}
}

func playerIdentityOperationField(id string, label string, description string) OperationField {
	return OperationField{
		ID:                id,
		Label:             label,
		Description:       description,
		Type:              OperationFieldPlayerIdentity,
		Required:          true,
		AllowManual:       true,
		ValidationPattern: PlayerIdentityPattern,
	}
}

func playerBuffOperationFields(playerDescription string) []OperationField {
	return []OperationField{
		playerIdentityOperationField("player", "Player", playerDescription),
		{
			ID:                "buff",
			Label:             "Buff",
			Description:       "Enter the exact buff name reported by the game server.",
			Type:              OperationFieldText,
			Required:          true,
			AllowManual:       true,
			ValidationPattern: CommandArgumentPattern,
		},
	}
}

func playerActionOperation(
	id string,
	name string,
	summary string,
	category string,
	risk OperationRisk,
	effect string,
	caution string,
	allowReason bool,
) OperationDescriptor {
	fields := []OperationField{
		{
			ID:                "player",
			Label:             "Player",
			Description:       "Choose a known Player or enter the platform, cross-platform, or entity ID accepted by the game.",
			Type:              OperationFieldPlayerIdentity,
			Required:          true,
			AllowManual:       true,
			ValidationPattern: PlayerActionIdentityPattern,
		},
	}
	if allowReason {
		fields = append(fields, OperationField{
			ID:                "reason",
			Label:             "Reason",
			Description:       "Optional. Control characters are rejected and the value is limited to 256 characters.",
			Type:              OperationFieldText,
			ValidationPattern: PlayerActionReasonPattern,
		})
	}
	return OperationDescriptor{
		ID:           id,
		Name:         name,
		Summary:      summary,
		Category:     category,
		PermissionID: "game_server.players.manage",
		Risk:         risk,
		AvailabilityRequirements: []string{
			"Server online",
			"Node supports game operations",
			"Node supports typed 7 Days to Die Player actions",
		},
		Fields: fields,
		Review: OperationReview{
			Title:   "Review " + name,
			Effect:  effect,
			Caution: caution,
		},
		NativeCapability: OperationNativeCapabilityPlayerActions,
		Concurrency: OperationConcurrency{
			Lock:        id,
			TargetField: "player",
		},
	}
}

// OperationsForGame returns a detached copy of the built-in catalog for a game ID.
func OperationsForGame(gameID string) []OperationDescriptor {
	operations := slices.Clone(operationsByGame[gameID])
	for index := range operations {
		operations[index].AvailabilityRequirements = slices.Clone(operations[index].AvailabilityRequirements)
		operations[index].Concurrency.ConflictsWith = slices.Clone(operations[index].Concurrency.ConflictsWith)
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
