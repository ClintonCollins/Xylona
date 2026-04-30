package gamedefinitions_test

import (
	"strings"
	"testing"
	"time"

	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/db/dbtest"
	"github.com/ClintonCollins/Xylona/internal/gamedefinitions"
	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/sql/models"
)

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

func TestLoadBundledDefinitions(t *testing.T) {
	definitions, errLoad := gamedefinitions.LoadBundled()
	if errLoad != nil {
		t.Fatalf("LoadBundled() error = %v", errLoad)
	}
	if len(definitions) != 53 {
		t.Fatalf("LoadBundled() = %d definitions, want 53", len(definitions))
	}
	for _, definition := range definitions {
		validationErrors := gamedefinitions.ValidateModel(definition.Model)
		if len(validationErrors) > 0 {
			t.Fatalf("ValidateModel(%s) errors = %v", definition.Model.ID, validationErrors)
		}
	}
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
