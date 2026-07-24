package gamedefinitions_test

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/gamedefinitions"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	internalgames "github.com/ClintonCollins/Xylona/internal/gameintegrations/games"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/cfgschema"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var officialGameDefinitionIDs = []string{
	"7_days_to_die",
	"abiotic_factor",
	"aska",
	"conan_exiles",
	"core_keeper",
	"counter_strike_2",
	"enshrouded",
	"factorio",
	"garrys_mod",
	"hytale",
	"minecraft",
	"palworld",
	"project_zomboid",
	"runescape_dragonwilds",
	"rust",
	"satisfactory",
	"sons_of_the_forest",
	"starbound",
	"sunkenland",
	"survive_the_nights",
	"team_fortress_2",
	"terraria",
	"v_rising",
	"valheim",
	"windrose",
}

var officialFixedMaxPlayers = map[string]int64{
	"aska":    4,
	"valheim": 10,
}

func TestSevenDaysToDieDefinitionConfigSchema(t *testing.T) {
	definitions, errLoad := gamedefinitions.LoadBundled()
	if errLoad != nil {
		t.Fatalf("LoadBundled() error = %v", errLoad)
	}

	var schemaJSON string
	for _, definition := range definitions {
		if definition.Model.ID == "7_days_to_die" {
			schemaJSON = definition.Model.ConfigSchemas.GetOr("")
			break
		}
	}
	if schemaJSON == "" {
		t.Fatal("7 Days to Die config schema is unavailable")
	}

	entries, errParse := cfgschema.ParseConfigSchemas(schemaJSON)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("config schema count = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.ManagedFields["TelnetEnabled"] != "xylona.local_console_enabled" {
		t.Fatalf("TelnetEnabled source = %q", entry.ManagedFields["TelnetEnabled"])
	}
	if entry.ManagedFields["TelnetPassword"] != "xylona.local_console_password" {
		t.Fatalf("TelnetPassword source = %q", entry.ManagedFields["TelnetPassword"])
	}

	wantProperties := []string{
		"AdminFileName",
		"AllowSpawnNearFriend",
		"BedrollDeadZoneSize",
		"BedrollExpiryTime",
		"BuildCreate",
		"CameraRestrictionMode",
		"DynamicMeshEnabled",
		"DynamicMeshLandClaimBuffer",
		"DynamicMeshLandClaimOnly",
		"DynamicMeshMaxItemCache",
		"EACEnabled",
		"EnableMapRendering",
		"GameMode",
		"GameName",
		"GameWorld",
		"HideCommandExecutionLog",
		"IgnoreEOSSanctions",
		"LandClaimCount",
		"LandClaimDeadZone",
		"LandClaimDecayMode",
		"LandClaimExpiryTime",
		"LandClaimOfflineDelay",
		"LandClaimOfflineDurabilityModifier",
		"LandClaimOnlineDurabilityModifier",
		"LandClaimSize",
		"Language",
		"MaxChunkAge",
		"MaxQueuedMeshLayers",
		"MaxSpawnedAnimals",
		"MaxSpawnedZombies",
		"MaxUncoveredMapChunksPerPlayer",
		"PartySharedKillRange",
		"PersistentPlayerProfiles",
		"PlayerKillingMode",
		"PlayerSafeZoneHours",
		"PlayerSafeZoneLevel",
		"Region",
		"SandboxCode",
		"SaveDataLimit",
		"ServerAdminSlots",
		"ServerAdminSlotsPermission",
		"ServerAllowCrossplay",
		"ServerDescription",
		"ServerDisabledNetworkProtocols",
		"ServerLoginConfirmationText",
		"ServerMaxAllowedViewDistance",
		"ServerMaxPlayerCount",
		"ServerMaxWorldTransferSpeedKiBs",
		"ServerName",
		"ServerPassword",
		"ServerPort",
		"ServerReservedSlots",
		"ServerReservedSlotsPermission",
		"ServerVisibility",
		"ServerWebsiteURL",
		"TelnetEnabled",
		"TelnetFailedLoginLimit",
		"TelnetFailedLoginsBlocktime",
		"TelnetPassword",
		"TelnetPort",
		"TerminalWindowEnabled",
		"TwitchBloodMoonAllowed",
		"TwitchServerPermission",
		"UserDataFolder",
		"WebDashboardEnabled",
		"WebDashboardPort",
		"WebDashboardUrl",
		"WorldGenSeed",
		"WorldGenSize",
	}
	gotProperties := slices.Sorted(maps.Keys(entry.Schema.Properties))
	if !slices.Equal(gotProperties, wantProperties) {
		t.Fatalf("config schema properties = %v, want %v", gotProperties, wantProperties)
	}
}

func TestPalworldDefinitionConfigSchema(t *testing.T) {
	definitions, errLoad := gamedefinitions.LoadBundled()
	if errLoad != nil {
		t.Fatalf("LoadBundled() error = %v", errLoad)
	}

	var schemaJSON string
	for _, definition := range definitions {
		if definition.Model.ID == "palworld" {
			schemaJSON = definition.Model.ConfigSchemas.GetOr("")
			break
		}
	}
	if schemaJSON == "" {
		t.Fatal("Palworld config schema is unavailable")
	}

	entries, errParse := cfgschema.ParseConfigSchemas(schemaJSON)
	if errParse != nil {
		t.Fatalf("ParseConfigSchemas() error = %v", errParse)
	}
	if len(entries) != 1 {
		t.Fatalf("config schema count = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Format != "palworld" {
		t.Fatalf("format = %q, want palworld", entry.Format)
	}
	if !entry.GenerateBeforeStart {
		t.Fatal("GenerateBeforeStart = false, want true")
	}
	if entry.Path != "Pal/Saved/Config/LinuxServer/PalWorldSettings.ini" {
		t.Fatalf("default path = %q", entry.Path)
	}
	windowsPath := cfgschema.ResolvePlatformPath(entry, "windows")
	if windowsPath != "Pal/Saved/Config/WindowsServer/PalWorldSettings.ini" {
		t.Fatalf("windows path = %q", windowsPath)
	}
	for field, wantSource := range map[string]string{
		"ServerName":         "game_server.server_name",
		"ServerPlayerMaxNum": "game_server.max_players",
		"RESTAPIPort":        "game_server.query_port",
	} {
		if entry.ManagedFields[field] != wantSource {
			t.Errorf("managed source for %s = %q, want %q", field, entry.ManagedFields[field], wantSource)
		}
	}
	if len(entry.Schema.Properties) != 119 {
		t.Fatalf("config property count = %d, want 119", len(entry.Schema.Properties))
	}
	for _, property := range []string{
		"AdminPassword",
		"BaseCampMaxNumInGuild",
		"CrossplayPlatforms",
		"DeathPenalty",
		"DenyTechnologyList",
		"ItemContainerForceMarkDirtyInterval",
		"RandomizerType",
		"ServerReplicatePawnCullDistance",
		"VoiceChatZeroVolumeDistance",
	} {
		if _, exists := entry.Schema.Properties[property]; !exists {
			t.Errorf("config property %q is missing", property)
		}
	}
	baseCampLimit := entry.Schema.Properties["BaseCampMaxNumInGuild"]
	if baseCampLimit.Default != float64(4) || baseCampLimit.Maximum == nil || *baseCampLimit.Maximum != 50 {
		t.Fatalf("BaseCampMaxNumInGuild schema = %#v, want default 4 and maximum 50", baseCampLimit)
	}
}

func TestExportParseRoundTripPreservesStructuredSections(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-roundtrip.sqlite")
	game, errGame := conn.GetGameByID("minecraft")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}

	definitionJSON, hash, errExport := gamedefinitions.ExportModel(game, "test", fixedZeroTime())
	if errExport != nil {
		t.Fatalf("ExportModel() error = %v", errExport)
	}
	if hash == "" {
		t.Fatal("ExportModel() hash is empty")
	}
	if !strings.Contains(definitionJSON, `"config_schemas": [`) {
		t.Fatal("ExportModel() did not expose config_schemas as structured JSON")
	}
	if strings.Contains(definitionJSON, `\"path\"`) {
		t.Fatal("ExportModel() contains escaped config schema JSON")
	}

	parsed, errParse := gamedefinitions.Parse([]byte(definitionJSON))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if parsed.Hash != hash {
		t.Fatalf("Parse().Hash = %q, want %q", parsed.Hash, hash)
	}
	if parsed.Model.ConfigSchemas.GetOr("") == "" {
		t.Fatal("Parse().Model.ConfigSchemas is empty")
	}
	if parsed.Model.LinuxStartArgsTemplate.GetOr("") == "" {
		t.Fatal("Parse().Model.LinuxStartArgsTemplate is empty")
	}
	if parsed.Model.StartArgBlocklist == "" {
		t.Fatal("Parse().Model.StartArgBlocklist is empty")
	}
}

func TestExportParseRoundTripPreservesDefaultEnvVars(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-default-env-roundtrip.sqlite")
	game, errGame := conn.GetGameByID("hytale")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}
	game.DefaultEnvVars = `[{"name":"HYTALE_AUTH_MODE","value":"refresh_token"}]`

	definitionJSON, hash, errExport := gamedefinitions.ExportModel(game, "test", fixedZeroTime())
	if errExport != nil {
		t.Fatalf("ExportModel() error = %v", errExport)
	}
	if !strings.Contains(definitionJSON, `"default_env_vars": [`) {
		t.Fatal("ExportModel() did not expose default_env_vars as structured JSON")
	}

	parsed, errParse := gamedefinitions.Parse([]byte(definitionJSON))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if parsed.Hash != hash {
		t.Fatalf("Parse().Hash = %q, want %q", parsed.Hash, hash)
	}
	if parsed.Model.DefaultEnvVars != game.DefaultEnvVars {
		t.Fatalf("Parse().Model.DefaultEnvVars = %q, want %q", parsed.Model.DefaultEnvVars, game.DefaultEnvVars)
	}
}

func TestDefaultEnvHashTreatsMissingAndEmptyAsEquivalent(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-default-env-empty-hash.sqlite")
	game, errGame := conn.GetGameByID("minecraft")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}

	definitionJSON, hashMissing, errExport := gamedefinitions.ExportModel(game, "test", fixedZeroTime())
	if errExport != nil {
		t.Fatalf("ExportModel() error = %v", errExport)
	}
	withEmptyEnv := strings.Replace(definitionJSON, `"update_config": {`, `"default_env_vars": [],`+"\n  "+`"update_config": {`, 1)

	parsed, errParse := gamedefinitions.Parse([]byte(withEmptyEnv))
	if errParse != nil {
		t.Fatalf("Parse() with empty default_env_vars error = %v", errParse)
	}
	if parsed.Hash != hashMissing {
		t.Fatalf("Parse().Hash = %q, want %q", parsed.Hash, hashMissing)
	}
}

func TestParseReportsHashMismatchAsWarning(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-hash-warning.sqlite")
	game, errGame := conn.GetGameByID("valheim")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}

	definitionJSON, _, errExport := gamedefinitions.ExportModel(game, "test", fixedZeroTime())
	if errExport != nil {
		t.Fatalf("ExportModel() error = %v", errExport)
	}
	modified := strings.Replace(definitionJSON, `"content_hash": "sha256:`, `"content_hash": "sha256:0000`, 1)

	parsed, errParse := gamedefinitions.Parse([]byte(modified))
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}
	if len(parsed.Warnings) != 1 {
		t.Fatalf("Parse().Warnings = %d, want 1", len(parsed.Warnings))
	}
}

func TestParsePreservesExplicitUpdateConfig(t *testing.T) {
	data, errRead := gamedefinitions.FS.ReadFile("official/hytale.json")
	if errRead != nil {
		t.Fatalf("ReadFile() error = %v", errRead)
	}

	parsed, errParse := gamedefinitions.Parse(data)
	if errParse != nil {
		t.Fatalf("Parse() error = %v", errParse)
	}

	gameConfig, errConfig := updateconfig.LoadGameConfigFromModel(parsed.Model)
	if errConfig != nil {
		t.Fatalf("LoadGameConfigFromModel() error = %v", errConfig)
	}
	if gameConfig.UpdateProvider.Kind != updateproviders.ProviderKindNone {
		t.Fatalf("UpdateProvider.Kind = %q, want %q", gameConfig.UpdateProvider.Kind, updateproviders.ProviderKindNone)
	}
}

func TestMinecraftDefinitionOffersServerSoftwareVariants(t *testing.T) {
	definitions, errLoad := gamedefinitions.LoadBundled()
	if errLoad != nil {
		t.Fatalf("LoadBundled() error = %v", errLoad)
	}

	var minecraft *models.Game
	for _, definition := range definitions {
		if definition.Model.ID == "minecraft" {
			minecraft = definition.Model
			break
		}
	}
	if minecraft == nil {
		t.Fatal("bundled minecraft definition not found")
	}

	gameConfig, errConfig := updateconfig.LoadGameConfigFromModel(minecraft)
	if errConfig != nil {
		t.Fatalf("LoadGameConfigFromModel() error = %v", errConfig)
	}

	if gameConfig.UpdateProvider.Kind != updateproviders.ProviderKindMojang {
		t.Errorf("game UpdateProvider.Kind = %q, want %q", gameConfig.UpdateProvider.Kind, updateproviders.ProviderKindMojang)
	}

	wantVariants := []struct {
		id           string
		providerKind updateproviders.ProviderKind
		sourceID     string
		modInstall   string
	}{
		{id: "vanilla", providerKind: updateproviders.ProviderKindMojang, sourceID: "vanilla"},
		{id: "paper", providerKind: updateproviders.ProviderKindPaperMC, sourceID: "paper", modInstall: "plugins/"},
		{id: "purpur", providerKind: updateproviders.ProviderKindPaperMC, sourceID: "purpur", modInstall: "plugins/"},
		{id: "fabric", providerKind: updateproviders.ProviderKindCommand, modInstall: "mods/"},
		{id: "folia", providerKind: updateproviders.ProviderKindPaperMC, sourceID: "folia", modInstall: "plugins/"},
	}

	if len(gameConfig.Variants) != len(wantVariants) {
		t.Fatalf("len(Variants) = %d, want %d", len(gameConfig.Variants), len(wantVariants))
	}
	for index, want := range wantVariants {
		variant := gameConfig.Variants[index]
		if variant.ID != want.id {
			t.Errorf("Variants[%d].ID = %q, want %q", index, variant.ID, want.id)
			continue
		}
		if variant.UpdateProvider == nil {
			t.Errorf("Variants[%d] (%s) UpdateProvider = nil, want %q", index, want.id, want.providerKind)
			continue
		}
		if variant.UpdateProvider.Kind != want.providerKind {
			t.Errorf("Variants[%d] (%s) UpdateProvider.Kind = %q, want %q", index, want.id, variant.UpdateProvider.Kind, want.providerKind)
		}
		if variant.UpdateProvider.SourceID != want.sourceID {
			t.Errorf("Variants[%d] (%s) UpdateProvider.SourceID = %q, want %q", index, want.id, variant.UpdateProvider.SourceID, want.sourceID)
		}
		if want.modInstall == "" {
			continue
		}
		if variant.ModProfile == nil {
			t.Errorf("Variants[%d] (%s) ModProfile = nil, want install path %q", index, want.id, want.modInstall)
			continue
		}
		if variant.ModProfile.InstallPath != want.modInstall {
			t.Errorf("Variants[%d] (%s) ModProfile.InstallPath = %q, want %q", index, want.id, variant.ModProfile.InstallPath, want.modInstall)
		}
	}
}

func TestLoadBundledDefinitions(t *testing.T) {
	definitions, errLoad := gamedefinitions.LoadBundled()
	if errLoad != nil {
		t.Fatalf("LoadBundled() error = %v", errLoad)
	}
	definitionIDs := make([]string, 0, len(definitions))
	for _, definition := range definitions {
		definitionIDs = append(definitionIDs, definition.Model.ID)
	}
	if !slices.Equal(definitionIDs, officialGameDefinitionIDs) {
		t.Fatalf("LoadBundled() IDs = %v, want %v", definitionIDs, officialGameDefinitionIDs)
	}
	for _, definition := range definitions {
		if len(definition.Warnings) > 0 {
			t.Errorf("definition %q warnings = %v", definition.Model.ID, definition.Warnings)
		}
		validationErrors := gamedefinitions.ValidateModel(definition.Model)
		if len(validationErrors) > 0 {
			t.Fatalf("ValidateModel(%s) errors = %v", definition.Model.ID, validationErrors)
		}
	}
}

func TestOfficialDefinitionsHaveTurnkeyRuntimeContracts(t *testing.T) {
	internalgames.RegisterInternalGames()

	definitions, errLoad := gamedefinitions.LoadBundled()
	if errLoad != nil {
		t.Fatalf("LoadBundled() error = %v", errLoad)
	}
	for _, definition := range definitions {
		t.Run(definition.Model.ID, func(t *testing.T) {
			game := definition.Game
			model := definition.Model
			wantSource := model.ID + ".json"
			if model.OfficialDefinitionSource != wantSource {
				t.Errorf("OfficialDefinitionSource = %q, want %q", model.OfficialDefinitionSource, wantSource)
			}
			if !game.GetLinuxSupport() && !game.GetWindowsSupport() {
				t.Error("definition does not support any operating system")
			}
			validateOfficialDefinitionRange(t, "default port", game.GetDefaultPort(), 1, 65535)
			validateOfficialDefinitionRange(t, "default query port", game.GetDefaultQueryPort(), 1, 65535)
			validateOfficialDefinitionRange(t, "default max players", game.GetDefaultMaxPlayers(), 1, 65535)
			fixedMaxPlayers, hasFixedMaxPlayers := officialFixedMaxPlayers[game.GetId()]
			if hasFixedMaxPlayers && game.GetDefaultMaxPlayers() != fixedMaxPlayers {
				t.Errorf("default max players = %d, want fixed game capacity %d", game.GetDefaultMaxPlayers(), fixedMaxPlayers)
			}

			gameConfig, errConfig := updateconfig.LoadGameConfigFromModel(model)
			if errConfig != nil {
				t.Fatalf("LoadGameConfigFromModel() error = %v", errConfig)
			}
			validateOfficialPlatform(t, officialPlatform{
				name:             "linux",
				supported:        game.GetLinuxSupport(),
				installType:      game.GetLinuxInstallType(),
				updateType:       game.GetLinuxUpdateType(),
				installProcessor: game.GetLinuxInstallCommandProcessor(),
				updateProcessor:  game.GetLinuxUpdateCommandProcessor(),
				installCommand:   game.GetLinuxInstallCommand(),
				updateCommand:    game.GetLinuxUpdateCommand(),
				workingDirectory: game.GetLinuxWorkingDirectory(),
				baseCommand:      game.GetLinuxBaseCommand(),
				startArgs:        model.LinuxStartArgsTemplate.GetOr(""),
			}, game, model, gameConfig)
			validateOfficialPlatform(t, officialPlatform{
				name:             "windows",
				supported:        game.GetWindowsSupport(),
				installType:      game.GetWindowsInstallType(),
				updateType:       game.GetWindowsUpdateType(),
				installProcessor: game.GetWindowsInstallCommandProcessor(),
				updateProcessor:  game.GetWindowsUpdateCommandProcessor(),
				installCommand:   game.GetWindowsInstallCommand(),
				updateCommand:    game.GetWindowsUpdateCommand(),
				workingDirectory: game.GetWindowsWorkingDirectory(),
				baseCommand:      game.GetWindowsBaseCommand(),
				startArgs:        model.WindowsStartArgsTemplate.GetOr(""),
			}, game, model, gameConfig)
		})
	}
}

type officialPlatform struct {
	name             string
	supported        bool
	installType      xylona.CommandType
	updateType       xylona.CommandType
	installProcessor xylona.CommandProcessor
	updateProcessor  xylona.CommandProcessor
	installCommand   string
	updateCommand    string
	workingDirectory string
	baseCommand      string
	startArgs        string
}

func validateOfficialPlatform(
	t *testing.T,
	platform officialPlatform,
	game *xylona.Game,
	model *models.Game,
	gameConfig updateproviders.GameConfig,
) {
	t.Helper()
	if !platform.supported {
		return
	}

	if platform.installType == xylona.CommandType_NONE {
		t.Errorf("%s install type is NONE", platform.name)
	}
	if strings.TrimSpace(platform.baseCommand) == "" {
		t.Errorf("%s base command is empty", platform.name)
	}
	validateOfficialCommand(t, platform.name+" install", platform.installType, platform.installProcessor, platform.installCommand, game)

	if platform.updateType == xylona.CommandType_NONE && gameConfig.UpdateProvider.Kind == updateproviders.ProviderKindNone {
		t.Errorf("%s update mechanism is not configured", platform.name)
	} else {
		validateOfficialCommand(t, platform.name+" update", platform.updateType, platform.updateProcessor, platform.updateCommand, game)
	}

	workingDirectory := strings.TrimSpace(platform.workingDirectory)
	if workingDirectory == ".." || strings.HasPrefix(workingDirectory, "../") || strings.HasPrefix(workingDirectory, `..\`) {
		t.Errorf("%s working directory escapes the server root: %q", platform.name, platform.workingDirectory)
	}

	runtimeConfig := platform.startArgs + "\n" + model.ConfigSchemas.GetOr("")
	hasGamePort := containsAny(runtimeConfig, "{{PORT}}", "%GAMESERVER_PORT%", "game_server.port")
	hasQueryPort := containsAny(runtimeConfig, "{{QUERY_PORT}}", "%GAMESERVER_QUERY_PORT%", "game_server.query_port")
	if game.GetDefaultPort() == game.GetDefaultQueryPort() {
		if !hasGamePort && !hasQueryPort {
			t.Errorf("%s runtime does not consume the configured shared game/query port", platform.name)
		}
	} else if !hasGamePort {
		t.Errorf("%s runtime does not consume the configured game port", platform.name)
	}

	queryPortIsDerived := game.GetDefaultQueryPort() == game.GetDefaultPort()+1
	queryPortHandledOutsideDefinition := game.GetId() == "palworld"
	if game.GetDefaultQueryPort() != game.GetDefaultPort() &&
		!queryPortIsDerived && !queryPortHandledOutsideDefinition && !hasQueryPort {
		t.Errorf("%s runtime does not consume the configured query port", platform.name)
	}

	_, maxPlayersFixedByGame := officialFixedMaxPlayers[game.GetId()]
	maxPlayersHandledOutsideDefinition := game.GetId() == "palworld" || maxPlayersFixedByGame
	if !maxPlayersHandledOutsideDefinition &&
		!containsAny(runtimeConfig, "{{MAX_PLAYERS}}", "%GAMESERVER_MAX_PLAYERS%", "game_server.max_players") {
		t.Errorf("%s runtime does not consume the configured max players", platform.name)
	}
}

func validateOfficialCommand(
	t *testing.T,
	label string,
	commandType xylona.CommandType,
	processor xylona.CommandProcessor,
	command string,
	game *xylona.Game,
) {
	t.Helper()
	trimmedCommand := strings.TrimSpace(command)
	switch commandType {
	case xylona.CommandType_NONE:
		return
	case xylona.CommandType_STEAMCMD:
		appID := strings.TrimSpace(game.GetSteamAppid())
		parsedAppID, errAppID := strconv.ParseUint(appID, 10, 64)
		if errAppID != nil || parsedAppID == 0 {
			t.Errorf("%s Steam app ID = %q, want a positive integer", label, appID)
		}
		if !game.GetUsesSteamcmd() {
			t.Errorf("%s uses STEAMCMD but usesSteamcmd is false", label)
		}
		loginFragment := "+login anonymous"
		if game.GetId() == gameintegrations.StarboundGameID {
			loginFragment = `+login "{{STEAM_USERNAME}}"`
		}
		for _, fragment := range []string{
			"steamcmd",
			loginFragment,
			"+app_update " + appID,
			"validate",
			"+quit",
		} {
			if !strings.Contains(strings.ToLower(trimmedCommand), strings.ToLower(fragment)) {
				t.Errorf("%s command does not contain %q", label, fragment)
			}
		}
		forceInstallDirectory := "+force_install_dir %GAMESERVER_DIRECTORY%"
		quotedForceInstallDirectory := `+force_install_dir "%GAMESERVER_DIRECTORY%"`
		if !strings.Contains(strings.ToLower(trimmedCommand), strings.ToLower(forceInstallDirectory)) &&
			!strings.Contains(strings.ToLower(trimmedCommand), strings.ToLower(quotedForceInstallDirectory)) {
			t.Errorf("%s command does not contain a force-install directory", label)
		}
	case xylona.CommandType_COMMAND:
		if processor == xylona.CommandProcessor_XYLONA_INTERNAL {
			_, registered := gameintegrations.GetGame(game.GetId())
			if !registered {
				t.Errorf("%s uses XYLONA_INTERNAL but game integration %q is not registered", label, game.GetId())
			}
			return
		}
		if trimmedCommand == "" {
			t.Errorf("%s command is empty", label)
		}
	case xylona.CommandType_PAPERMC, xylona.CommandType_MOJANG:
		if processor != xylona.CommandProcessor_XYLONA_INTERNAL {
			t.Errorf("%s type %s must use XYLONA_INTERNAL", label, commandType)
		}
		_, registered := gameintegrations.GetGame(game.GetId())
		if !registered {
			t.Errorf("%s internal game integration %q is not registered", label, game.GetId())
		}
	default:
		t.Errorf("%s command type %s is unsupported", label, commandType)
	}
}

func validateOfficialDefinitionRange(t *testing.T, label string, value int64, minimum int64, maximum int64) {
	t.Helper()
	if value < minimum || value > maximum {
		t.Errorf("%s = %d, want %d..%d", label, value, minimum, maximum)
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func TestSyncOfficialDefinitionsInsertsDefinitionsForEmptyDatabase(t *testing.T) {
	conn := dbtest.NewMigratedSchemaConnection(t, "definition-sync-empty.sqlite")

	result, errSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSync != nil {
		t.Fatalf("SyncOfficialDefinitions() error = %v", errSync)
	}
	if result.Inserted == 0 {
		t.Fatalf("SyncOfficialDefinitions().Inserted = %d, want > 0", result.Inserted)
	}

	game, errGame := conn.GetGameByID("minecraft")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}
	if game.OfficialDefinitionHash == "" {
		t.Fatal("OfficialDefinitionHash is empty")
	}
	if game.OfficialDefinitionSource != "minecraft.json" {
		t.Fatalf("OfficialDefinitionSource = %q, want minecraft.json", game.OfficialDefinitionSource)
	}
	if game.OfficialDefinitionSchemaVersion != gamedefinitions.SchemaVersion {
		t.Fatalf("OfficialDefinitionSchemaVersion = %d, want %d", game.OfficialDefinitionSchemaVersion, gamedefinitions.SchemaVersion)
	}
	if game.OfficialDefinitionDiverged {
		t.Fatal("OfficialDefinitionDiverged = true, want false")
	}
}

func TestSyncOfficialDefinitionsSkipsUnchangedOfficialRows(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-sync-unchanged.sqlite")

	_, errSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSync != nil {
		t.Fatalf("SyncOfficialDefinitions() setup error = %v", errSync)
	}

	result, errSecondSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSecondSync != nil {
		t.Fatalf("SyncOfficialDefinitions() error = %v", errSecondSync)
	}
	if result.Updated != 0 {
		t.Fatalf("SyncOfficialDefinitions().Updated = %d, want 0", result.Updated)
	}
	if result.Diverged != 0 {
		t.Fatalf("SyncOfficialDefinitions().Diverged = %d, want 0", result.Diverged)
	}
}

func TestSyncOfficialDefinitionsPreservesLocallyEditedOfficialRows(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-sync-diverged.sqlite")

	_, errSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSync != nil {
		t.Fatalf("SyncOfficialDefinitions() setup error = %v", errSync)
	}
	game, errGame := conn.GetGameByID("minecraft")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}
	game.Name = "Minecraft Local Edit"
	_, errUpdate := conn.UpdateGame(conn.DB, game, &models.GameSetter{
		ID:        omit.From(game.ID),
		Name:      omit.From(game.Name),
		UpdatedAt: omit.From(game.UpdatedAt),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdate)
	}

	result, errSecondSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSecondSync != nil {
		t.Fatalf("SyncOfficialDefinitions() error = %v", errSecondSync)
	}
	if result.Diverged == 0 {
		t.Fatalf("SyncOfficialDefinitions().Diverged = %d, want > 0", result.Diverged)
	}
	updated, errUpdated := conn.GetGameByID("minecraft")
	if errUpdated != nil {
		t.Fatalf("GetGameByID() after sync error = %v", errUpdated)
	}
	if updated.Name != "Minecraft Local Edit" {
		t.Fatalf("Name = %q, want local edit preserved", updated.Name)
	}
	if !updated.OfficialDefinitionDiverged {
		t.Fatal("OfficialDefinitionDiverged = false, want true")
	}
}

func TestSyncOfficialDefinitionsSkipsCustomIDCollision(t *testing.T) {
	conn := dbtest.NewMigratedSchemaConnection(t, "definition-sync-custom-collision.sqlite")
	_, errInsert := conn.InsertGame(conn.DB, &models.GameSetter{
		ID:                omit.From("minecraft"),
		Name:              omit.From("Custom Minecraft"),
		DefaultPort:       omit.From(int64(25565)),
		DefaultQueryPort:  omit.From(int64(25565)),
		DefaultMaxPlayers: omit.From(int64(20)),
		XylonaOfficial:    omit.From(false),
	})
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}

	result, errSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSync != nil {
		t.Fatalf("SyncOfficialDefinitions() error = %v", errSync)
	}
	if result.Skipped == 0 {
		t.Fatalf("SyncOfficialDefinitions().Skipped = %d, want > 0", result.Skipped)
	}
	updated, errUpdated := conn.GetGameByID("minecraft")
	if errUpdated != nil {
		t.Fatalf("GetGameByID() after sync error = %v", errUpdated)
	}
	if updated.XylonaOfficial {
		t.Fatal("XylonaOfficial = true, want custom row preserved")
	}
	if updated.Name != "Custom Minecraft" {
		t.Fatalf("Name = %q, want custom row preserved", updated.Name)
	}
}

func TestSyncOfficialDefinitionsInsertsMissingOfficialDefinition(t *testing.T) {
	conn := dbtest.NewMigratedConnection(t, "definition-sync-missing.sqlite")
	errDelete := conn.DeleteGameByID("windrose")
	if errDelete != nil {
		t.Fatalf("DeleteGameByID() error = %v", errDelete)
	}

	result, errSync := gamedefinitions.SyncOfficialDefinitions(conn)
	if errSync != nil {
		t.Fatalf("SyncOfficialDefinitions() error = %v", errSync)
	}
	if result.Inserted == 0 {
		t.Fatalf("SyncOfficialDefinitions().Inserted = %d, want > 0", result.Inserted)
	}
	game, errGame := conn.GetGameByID("windrose")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}
	if !game.XylonaOfficial {
		t.Fatal("XylonaOfficial = false, want true")
	}
}

func fixedZeroTime() time.Time {
	return time.Time{}
}
