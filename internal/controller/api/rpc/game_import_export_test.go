package rpc

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/internal/updateconfig"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestExportGameRequiresSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	req := connect.NewRequest(&xylona.ExportGameRequest{
		GameId: "minecraft",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-owner")

	_, errExport := fixture.service.ExportGame(context.Background(), req)
	if errExport == nil {
		t.Fatal("ExportGame(non-superuser) error = nil, want permission denied")
	}
	if connect.CodeOf(errExport) != connect.CodePermissionDenied {
		t.Fatalf("ExportGame(non-superuser) code = %v, want %v", connect.CodeOf(errExport), connect.CodePermissionDenied)
	}
}

func TestExportGameReturnsDefinitionJSON(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	resp := exportGameForTest(t, fixture, "minecraft")

	if resp.Msg.GetFileName() != "minecraft.json" {
		t.Fatalf("ExportGame().FileName = %q, want minecraft.json", resp.Msg.GetFileName())
	}
	definitionJSON := resp.Msg.GetGameDefinitionJson()
	if !strings.Contains(definitionJSON, `"document_type": "xylona.game_definition"`) {
		t.Fatal("ExportGame() missing definition envelope")
	}
	if !strings.Contains(definitionJSON, `"config_schemas": [`) {
		t.Fatal("ExportGame() missing structured config_schemas")
	}
	if strings.Contains(definitionJSON, `\"path\"`) {
		t.Fatal("ExportGame() contains escaped config schema JSON")
	}
}

func TestImportGamePreviewReportsConflictAndImpact(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, "minecraft")
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: exportResp.Msg.GetGameDefinitionJson(),
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(PREVIEW) error = %v", errImport)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("ImportGame(PREVIEW).Success = false, errors = %v", resp.Msg.GetValidationErrors())
	}
	if !resp.Msg.GetIdConflict() {
		t.Fatal("ImportGame(PREVIEW).IdConflict = false, want true")
	}
	if resp.Msg.GetAffectedGameServerCount() != 1 {
		t.Fatalf("ImportGame(PREVIEW).AffectedGameServerCount = %d, want 1", resp.Msg.GetAffectedGameServerCount())
	}
	if len(resp.Msg.GetAffectedGameServerNames()) != 1 || resp.Msg.GetAffectedGameServerNames()[0] != "Local One" {
		t.Fatalf("ImportGame(PREVIEW).AffectedGameServerNames = %v, want [Local One]", resp.Msg.GetAffectedGameServerNames())
	}
	if len(resp.Msg.GetChanges()) != 0 {
		t.Fatalf("ImportGame(PREVIEW).Changes = %d, want 0 for unchanged export", len(resp.Msg.GetChanges()))
	}
}

func TestImportGameRejectsDefaultEnvConflictWithExistingServerSecret(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	errSet := fixture.conn.SetGameServerSecretEnv("server-local-1", "TOKEN", "secret-value", "user-admin")
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() setup error = %v", errSet)
	}

	exportResp := exportGameForTest(t, fixture, "minecraft")
	definitionJSON := mutateImportDefinitionJSON(t, exportResp.Msg.GetGameDefinitionJson(), func(t *testing.T, document map[string]any) {
		t.Helper()

		document["default_env_vars"] = []any{
			map[string]any{
				"name":  "token",
				"value": "visible-value",
			},
		}
	})

	previewReq := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, previewReq, "user-admin")
	previewResp, errPreview := fixture.service.ImportGame(context.Background(), previewReq)
	if errPreview != nil {
		t.Fatalf("ImportGame(PREVIEW) error = %v", errPreview)
	}
	if !previewResp.Msg.GetSuccess() {
		t.Fatalf("ImportGame(PREVIEW).Success = false, errors = %v", previewResp.Msg.GetValidationErrors())
	}
	if !validationErrorsContain(previewResp.Msg.GetWarnings(), "Local One") {
		t.Fatalf("ImportGame(PREVIEW).Warnings = %v, want affected server warning", previewResp.Msg.GetWarnings())
	}

	applyReq := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_APPLY,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, applyReq, "user-admin")
	applyResp, errApply := fixture.service.ImportGame(context.Background(), applyReq)
	if errApply != nil {
		t.Fatalf("ImportGame(APPLY) error = %v", errApply)
	}
	if applyResp.Msg.GetSuccess() {
		t.Fatal("ImportGame(APPLY).Success = true, want false")
	}
	if !validationErrorsContain(applyResp.Msg.GetValidationErrors(), "Local One") {
		t.Fatalf("ImportGame(APPLY).ValidationErrors = %v, want affected server", applyResp.Msg.GetValidationErrors())
	}

	copyReq := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_IMPORT_COPY,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, copyReq, "user-admin")
	copyResp, errCopy := fixture.service.ImportGame(context.Background(), copyReq)
	if errCopy != nil {
		t.Fatalf("ImportGame(IMPORT_COPY) error = %v", errCopy)
	}
	if !copyResp.Msg.GetSuccess() {
		t.Fatalf("ImportGame(IMPORT_COPY).Success = false, errors = %v", copyResp.Msg.GetValidationErrors())
	}
	if copyResp.Msg.GetImportedGameId() == "minecraft" || copyResp.Msg.GetImportedGameId() == "" {
		t.Fatalf("ImportGame(IMPORT_COPY).ImportedGameId = %q, want copied ID", copyResp.Msg.GetImportedGameId())
	}

	game, errGame := fixture.conn.GetGameByID("minecraft")
	if errGame != nil {
		t.Fatalf("GetGameByID(minecraft) error = %v", errGame)
	}
	if strings.Contains(game.DefaultEnvVars, "token") {
		t.Fatalf("ImportGame(APPLY) wrote conflicting default env vars: %s", game.DefaultEnvVars)
	}
}

func TestImportGamePreviewReportsChangedFieldsForEditedConflict(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, "valheim")
	definitionJSON := strings.Replace(exportResp.Msg.GetGameDefinitionJson(), `"name": "Valheim"`, `"name": "Valheim Imported"`, 1)
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(PREVIEW edited) error = %v", errImport)
	}
	if !resp.Msg.GetIdConflict() {
		t.Fatal("ImportGame(PREVIEW edited).IdConflict = false, want true")
	}
	if len(resp.Msg.GetWarnings()) == 0 {
		t.Fatal("ImportGame(PREVIEW edited).Warnings is empty, want stale hash warning")
	}
	change := findImportChange(resp.Msg.GetChanges(), "game.name")
	if change == nil {
		t.Fatalf("ImportGame(PREVIEW edited).Changes missing game.name: %+v", resp.Msg.GetChanges())
	}
	if change.GetPreviousValue() != "Valheim" || change.GetImportedValue() != "Valheim Imported" {
		t.Fatalf("game.name change = %q -> %q, want Valheim -> Valheim Imported", change.GetPreviousValue(), change.GetImportedValue())
	}
}

func TestImportGamePreviewReportsAddedConfigSchemaFile(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, "valheim")
	configSchemas := `"config_schemas": [
    {
      "path": "server.properties",
      "format": "properties",
      "category": "Core",
      "schema": {
        "type": "object",
        "properties": {
          "motd": {
            "type": "string"
          }
        }
      }
    }
  ],`
	definitionJSON := strings.Replace(exportResp.Msg.GetGameDefinitionJson(), `"config_schemas": [],`, configSchemas, 1)
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(PREVIEW complex) error = %v", errImport)
	}
	change := findImportChange(resp.Msg.GetChanges(), "config_schemas.server.properties")
	if change == nil {
		t.Fatalf("ImportGame(PREVIEW complex).Changes missing added config file: %+v", resp.Msg.GetChanges())
	}
	if change.GetPreviousValue() != "Missing" || !strings.Contains(change.GetImportedValue(), "1 field") {
		t.Fatalf("config file change = %q -> %q, want added file summary", change.GetPreviousValue(), change.GetImportedValue())
	}
}

func TestImportGamePreviewReportsSameSummaryComplexChanges(t *testing.T) {
	tests := []struct {
		name                 string
		gameID               string
		path                 string
		wantPrevious         string
		wantImported         string
		wantImportedContains string
		setup                func(t *testing.T, fixture *rbacRPCFixture)
		mutate               func(t *testing.T, definitionJSON string) string
	}{
		{
			name:         "config schema detail",
			gameID:       "minecraft",
			path:         "config_schemas.server.properties.schema.properties.motd.default",
			wantPrevious: "A Minecraft Server",
			wantImported: "Imported Minecraft Server",
			mutate: func(t *testing.T, definitionJSON string) string {
				t.Helper()
				return replaceOnce(t, definitionJSON, `"default": "A Minecraft Server"`, `"default": "Imported Minecraft Server"`)
			},
		},
		{
			name:         "start arg token",
			gameID:       "valheim",
			path:         "linux_start_args_template.01JQSD00000000000000000001.tokens",
			wantPrevious: `-name {{SERVER_NAME}}`,
			wantImported: `-name "Imported Valheim Server"`,
			mutate: func(t *testing.T, definitionJSON string) string {
				t.Helper()
				return replaceOnce(t, definitionJSON, `"{{SERVER_NAME}}"`, `"Imported Valheim Server"`)
			},
		},
		{
			name:                 "blocklist reason",
			gameID:               "minecraft",
			path:                 "start_arg_blocklist",
			wantImportedContains: "(details changed)",
			mutate: func(t *testing.T, definitionJSON string) string {
				t.Helper()
				return replaceOnce(t, definitionJSON, `"Java agents can modify server behavior unpredictably"`, `"Imported Java agent reason"`)
			},
		},
		{
			name:                 "variant target",
			gameID:               "valheim",
			path:                 "update_config.variants",
			setup:                seedValheimVariantForImportTest,
			wantImportedContains: "(details changed)",
			mutate: func(t *testing.T, definitionJSON string) string {
				t.Helper()
				return replaceOnce(t, definitionJSON, `"default_target": "stable"`, `"default_target": "beta"`)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			if test.setup != nil {
				test.setup(t, fixture)
			}
			exportResp := exportGameForTest(t, fixture, test.gameID)
			definitionJSON := test.mutate(t, exportResp.Msg.GetGameDefinitionJson())
			req := connect.NewRequest(&xylona.ImportGameRequest{
				GameDefinitionJson: definitionJSON,
				Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

			resp, errImport := fixture.service.ImportGame(context.Background(), req)
			if errImport != nil {
				t.Fatalf("ImportGame(PREVIEW %s) error = %v", test.name, errImport)
			}
			change := findImportChange(resp.Msg.GetChanges(), test.path)
			if change == nil {
				t.Fatalf("ImportGame(PREVIEW %s).Changes missing %s: %+v", test.name, test.path, resp.Msg.GetChanges())
			}
			if change.GetPreviousValue() == change.GetImportedValue() {
				t.Fatalf("ImportGame(PREVIEW %s) identical display values %q", test.name, change.GetPreviousValue())
			}
			if test.wantPrevious != "" && change.GetPreviousValue() != test.wantPrevious {
				t.Fatalf("ImportGame(PREVIEW %s).PreviousValue = %q, want %q", test.name, change.GetPreviousValue(), test.wantPrevious)
			}
			if test.wantImported != "" && change.GetImportedValue() != test.wantImported {
				t.Fatalf("ImportGame(PREVIEW %s).ImportedValue = %q, want %q", test.name, change.GetImportedValue(), test.wantImported)
			}
			if test.wantImportedContains != "" && !strings.Contains(change.GetImportedValue(), test.wantImportedContains) {
				t.Fatalf("ImportGame(PREVIEW %s).ImportedValue = %q, want to contain %q", test.name, change.GetImportedValue(), test.wantImportedContains)
			}
		})
	}
}

func TestImportGamePreviewReportsConfigSchemaGranularChanges(t *testing.T) {
	tests := []struct {
		name                 string
		gameID               string
		path                 string
		wantPrevious         string
		wantImported         string
		wantPreviousContains string
		wantImportedContains string
		mutate               func(t *testing.T, document map[string]any)
	}{
		{
			name:         "property default",
			gameID:       "minecraft",
			path:         "config_schemas.server.properties.schema.properties.motd.default",
			wantPrevious: "A Minecraft Server",
			wantImported: "Imported Minecraft Server",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				property := configSchemaPropertyForTest(t, document, 0, "motd")
				property["default"] = "Imported Minecraft Server"
			},
		},
		{
			name:         "property title",
			gameID:       "minecraft",
			path:         "config_schemas.server.properties.schema.properties.motd.title",
			wantPrevious: "MOTD",
			wantImported: "Server MOTD",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				property := configSchemaPropertyForTest(t, document, 0, "motd")
				property["title"] = "Server MOTD"
			},
		},
		{
			name:         "property enum",
			gameID:       "minecraft",
			path:         "config_schemas.server.properties.schema.properties.gamemode.enum",
			wantPrevious: "survival, creative, adventure, spectator",
			wantImported: "survival, creative, adventure, spectator, hardcore",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				property := configSchemaPropertyForTest(t, document, 0, "gamemode")
				enumValues := documentArrayForTest(t, property, "enum")
				property["enum"] = append(enumValues, "hardcore")
			},
		},
		{
			name:                 "file added",
			gameID:               "valheim",
			path:                 "config_schemas.server.properties",
			wantPrevious:         "Missing",
			wantImportedContains: "1 field",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				document["config_schemas"] = []any{
					map[string]any{
						"path":                  "server.properties",
						"format":                "properties",
						"category":              "Core",
						"generate_before_start": true,
						"schema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"motd": map[string]any{
									"type":    "string",
									"default": "Imported MOTD",
								},
							},
						},
					},
				}
			},
		},
		{
			name:                 "file removed",
			gameID:               "minecraft",
			path:                 "config_schemas.server.properties",
			wantPreviousContains: "properties",
			wantImported:         "Missing",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				document["config_schemas"] = []any{}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changes := previewMutatedImportChangesForTest(t, test.gameID, test.mutate)
			change := findImportChange(changes, test.path)
			if change == nil {
				t.Fatalf("ImportGame(PREVIEW %s).Changes missing %s: %+v", test.name, test.path, changes)
			}
			assertImportChangeValues(t, test.name, change, test.wantPrevious, test.wantImported, test.wantPreviousContains, test.wantImportedContains)
		})
	}
}

func TestImportGamePreviewReportsConfigSchemaReorder(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	configSchemas := `[
		{
			"path": "alpha.properties",
			"format": "properties",
			"category": "Core",
			"schema": {
				"type": "object",
				"properties": {
					"alpha": {
						"type": "string",
						"default": "one"
					}
				}
			}
		},
		{
			"path": "beta.properties",
			"format": "properties",
			"category": "Core",
			"schema": {
				"type": "object",
				"properties": {
					"beta": {
						"type": "string",
						"default": "two"
					}
				}
			}
		}
	]`
	seedGameConfigSchemasForImportTest(t, fixture, "minecraft", configSchemas)

	exportResp := exportGameForTest(t, fixture, "minecraft")
	definitionJSON := mutateImportDefinitionJSON(t, exportResp.Msg.GetGameDefinitionJson(), func(t *testing.T, document map[string]any) {
		t.Helper()
		schemas := documentArrayForTest(t, document, "config_schemas")
		document["config_schemas"] = []any{schemas[1], schemas[0]}
	})
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(PREVIEW reorder) error = %v", errImport)
	}
	alphaOrder := findImportChange(resp.Msg.GetChanges(), "config_schemas.alpha.properties.order")
	if alphaOrder == nil {
		t.Fatalf("ImportGame(PREVIEW reorder).Changes missing alpha order: %+v", resp.Msg.GetChanges())
	}
	assertImportChangeValues(t, "config schema reorder alpha", alphaOrder, "1", "2", "", "")

	betaOrder := findImportChange(resp.Msg.GetChanges(), "config_schemas.beta.properties.order")
	if betaOrder == nil {
		t.Fatalf("ImportGame(PREVIEW reorder).Changes missing beta order: %+v", resp.Msg.GetChanges())
	}
	assertImportChangeValues(t, "config schema reorder beta", betaOrder, "2", "1", "", "")
}

func TestImportGamePreviewReportsStartArgGranularChanges(t *testing.T) {
	tests := []struct {
		name                 string
		path                 string
		wantPrevious         string
		wantImported         string
		wantPreviousContains string
		wantImportedContains string
		mutate               func(t *testing.T, document map[string]any)
	}{
		{
			name:         "tokens",
			path:         "linux_start_args_template.01JQSD00000000000000000001.tokens",
			wantPrevious: `-name {{SERVER_NAME}}`,
			wantImported: `-name "Imported Valheim Server"`,
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				block := linuxStartArgBlockForTest(t, document, 0)
				block["tokens"] = []any{"-name", "Imported Valheim Server"}
			},
		},
		{
			name:         "label",
			path:         "linux_start_args_template.01JQSD00000000000000000001.label",
			wantPrevious: "Server name",
			wantImported: "Imported server name",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				linuxBlock := linuxStartArgBlockForTest(t, document, 0)
				linuxBlock["label"] = "Imported server name"
				windowsBlock := windowsStartArgBlockForTest(t, document, 0)
				windowsBlock["label"] = "Imported server name"
			},
		},
		{
			name:         "order",
			path:         "linux_start_args_template.01JQSD00000000000000000001.order",
			wantPrevious: "1",
			wantImported: "42",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				block := linuxStartArgBlockForTest(t, document, 0)
				block["order"] = 42
			},
		},
		{
			name:         "ownership",
			path:         "linux_start_args_template.01JQSD00000000000000000001.ownership",
			wantPrevious: "editable",
			wantImported: "system",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				linuxBlock := linuxStartArgBlockForTest(t, document, 0)
				linuxBlock["ownership"] = "system"
				windowsBlock := windowsStartArgBlockForTest(t, document, 0)
				windowsBlock["ownership"] = "system"
			},
		},
		{
			name:         "managed source",
			path:         "linux_start_args_template.01JQSD00000000000000000004.managed_source",
			wantPrevious: "game_server.port",
			wantImported: "game_server.query_port",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				linuxBlock := linuxStartArgBlockForTest(t, document, 3)
				linuxBlock["managed_source"] = "game_server.query_port"
				windowsBlock := windowsStartArgBlockForTest(t, document, 3)
				windowsBlock["managed_source"] = "game_server.query_port"
			},
		},
		{
			name:                 "block added",
			path:                 "linux_start_args_template.01JQSD00000000000000000099",
			wantPrevious:         "Missing",
			wantImportedContains: "-public 1",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				blocks := documentArrayForTest(t, document, "linux_start_args_template")
				document["linux_start_args_template"] = append(blocks, map[string]any{
					"id":        "01JQSD00000000000000000099",
					"order":     99,
					"ownership": "editable",
					"tokens":    []any{"-public", "1"},
					"label":     "Public server",
				})
			},
		},
		{
			name:                 "block removed",
			path:                 "linux_start_args_template.01JQSD00000000000000000001",
			wantPreviousContains: `-name {{SERVER_NAME}}`,
			wantImported:         "Missing",
			mutate: func(t *testing.T, document map[string]any) {
				t.Helper()
				blocks := documentArrayForTest(t, document, "linux_start_args_template")
				document["linux_start_args_template"] = blocks[1:]
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			changes := previewMutatedImportChangesForTest(t, "valheim", test.mutate)
			change := findImportChange(changes, test.path)
			if change == nil {
				t.Fatalf("ImportGame(PREVIEW %s).Changes missing %s: %+v", test.name, test.path, changes)
			}
			assertImportChangeValues(t, test.name, change, test.wantPrevious, test.wantImported, test.wantPreviousContains, test.wantImportedContains)
		})
	}
}

func TestImportGameApplyUpdatesExistingGame(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, "valheim")
	definitionJSON := strings.Replace(exportResp.Msg.GetGameDefinitionJson(), `"name": "Valheim"`, `"name": "Valheim Imported"`, 1)
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_APPLY,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(APPLY) error = %v", errImport)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("ImportGame(APPLY).Success = false, errors = %v", resp.Msg.GetValidationErrors())
	}
	if resp.Msg.GetImportedGameId() != "valheim" {
		t.Fatalf("ImportGame(APPLY).ImportedGameId = %q, want valheim", resp.Msg.GetImportedGameId())
	}
	if len(resp.Msg.GetWarnings()) == 0 {
		t.Fatal("ImportGame(APPLY).Warnings is empty, want hash mismatch warning")
	}
	updated, errGame := fixture.conn.GetGameByID("valheim")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}
	if updated.Name != "Valheim Imported" {
		t.Fatalf("updated game name = %q, want Valheim Imported", updated.Name)
	}
	if !updated.OfficialDefinitionDiverged {
		t.Fatal("OfficialDefinitionDiverged = false, want true after importing over official row")
	}
}

func TestImportGameCopyCreatesCustomGame(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, "valheim")
	definitionJSON := strings.Replace(exportResp.Msg.GetGameDefinitionJson(), `"name": "Valheim"`, `"name": "Valheim Copy Import"`, 1)
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_IMPORT_COPY,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(IMPORT_COPY) error = %v", errImport)
	}
	if !resp.Msg.GetSuccess() {
		t.Fatalf("ImportGame(IMPORT_COPY).Success = false, errors = %v", resp.Msg.GetValidationErrors())
	}
	if resp.Msg.GetImportedGameId() != "valheim_import" {
		t.Fatalf("ImportGame(IMPORT_COPY).ImportedGameId = %q, want valheim_import", resp.Msg.GetImportedGameId())
	}
	if len(resp.Msg.GetChanges()) != 0 {
		t.Fatalf("ImportGame(IMPORT_COPY).Changes = %d, want 0", len(resp.Msg.GetChanges()))
	}
	imported, errGame := fixture.conn.GetGameByID("valheim_import")
	if errGame != nil {
		t.Fatalf("GetGameByID() error = %v", errGame)
	}
	if imported.XylonaOfficial {
		t.Fatal("imported copy XylonaOfficial = true, want false")
	}
}

func TestImportGameReturnsValidationErrorsForInvalidJSON(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: `{"document_type":"xylona.game_definition","schema_version":1,"game":`,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(invalid JSON) error = %v", errImport)
	}
	if resp.Msg.GetSuccess() {
		t.Fatal("ImportGame(invalid JSON).Success = true, want false")
	}
	if len(resp.Msg.GetValidationErrors()) == 0 {
		t.Fatal("ImportGame(invalid JSON).ValidationErrors is empty")
	}
}

func TestImportGameRejectsRouteUnsafeIDs(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, "valheim")

	tests := []struct {
		name string
		id   string
	}{
		{
			name: "slash",
			id:   "bad/id",
		},
		{
			name: "query marker",
			id:   "bad?id",
		},
		{
			name: "space",
			id:   "bad id",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			definitionJSON := strings.Replace(exportResp.Msg.GetGameDefinitionJson(), `"id": "valheim"`, `"id": "`+test.id+`"`, 1)
			req := connect.NewRequest(&xylona.ImportGameRequest{
				GameDefinitionJson: definitionJSON,
				Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

			resp, errImport := fixture.service.ImportGame(context.Background(), req)
			if errImport != nil {
				t.Fatalf("ImportGame(PREVIEW) error = %v", errImport)
			}
			if resp.Msg.GetSuccess() {
				t.Fatal("ImportGame(PREVIEW).Success = true, want false")
			}
			if len(resp.Msg.GetValidationErrors()) == 0 {
				t.Fatal("ImportGame(PREVIEW).ValidationErrors is empty")
			}
		})
	}
}

func previewMutatedImportChangesForTest(t *testing.T, gameID string, mutate func(t *testing.T, document map[string]any)) []*xylona.GameImportChange {
	t.Helper()

	fixture := newRBACRPCFixture(t)
	exportResp := exportGameForTest(t, fixture, gameID)
	definitionJSON := mutateImportDefinitionJSON(t, exportResp.Msg.GetGameDefinitionJson(), mutate)
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: definitionJSON,
		Mode:               xylona.GameImportMode_GAME_IMPORT_MODE_PREVIEW,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")

	resp, errImport := fixture.service.ImportGame(context.Background(), req)
	if errImport != nil {
		t.Fatalf("ImportGame(PREVIEW %s) error = %v", gameID, errImport)
	}
	return resp.Msg.GetChanges()
}

func mutateImportDefinitionJSON(t *testing.T, definitionJSON string, mutate func(t *testing.T, document map[string]any)) string {
	t.Helper()

	var document map[string]any
	decoder := json.NewDecoder(strings.NewReader(definitionJSON))
	decoder.UseNumber()
	errDecode := decoder.Decode(&document)
	if errDecode != nil {
		t.Fatalf("decode exported game definition: %v", errDecode)
	}

	mutate(t, document)

	data, errMarshal := json.MarshalIndent(document, "", "  ")
	if errMarshal != nil {
		t.Fatalf("marshal mutated game definition: %v", errMarshal)
	}
	return string(data)
}

func configSchemaPropertyForTest(t *testing.T, document map[string]any, schemaIndex int, propertyKey string) map[string]any {
	t.Helper()

	schemas := documentArrayForTest(t, document, "config_schemas")
	if schemaIndex >= len(schemas) {
		t.Fatalf("config_schemas[%d] missing", schemaIndex)
	}
	schemaEntry := objectForTest(t, schemas[schemaIndex], "config schema")
	schema := objectFieldForTest(t, schemaEntry, "schema")
	properties := objectFieldForTest(t, schema, "properties")
	return objectFieldForTest(t, properties, propertyKey)
}

func linuxStartArgBlockForTest(t *testing.T, document map[string]any, blockIndex int) map[string]any {
	t.Helper()

	blocks := documentArrayForTest(t, document, "linux_start_args_template")
	if blockIndex >= len(blocks) {
		t.Fatalf("linux_start_args_template[%d] missing", blockIndex)
	}
	return objectForTest(t, blocks[blockIndex], "start arg block")
}

func windowsStartArgBlockForTest(t *testing.T, document map[string]any, blockIndex int) map[string]any {
	t.Helper()

	blocks := documentArrayForTest(t, document, "windows_start_args_template")
	if blockIndex >= len(blocks) {
		t.Fatalf("windows_start_args_template[%d] missing", blockIndex)
	}
	return objectForTest(t, blocks[blockIndex], "start arg block")
}

func documentArrayForTest(t *testing.T, document map[string]any, key string) []any {
	t.Helper()

	value, exists := document[key]
	if !exists {
		t.Fatalf("%s missing", key)
	}
	array, ok := value.([]any)
	if !ok {
		t.Fatalf("%s = %T, want []any", key, value)
	}
	return array
}

func objectFieldForTest(t *testing.T, object map[string]any, key string) map[string]any {
	t.Helper()

	value, exists := object[key]
	if !exists {
		t.Fatalf("%s missing", key)
	}
	return objectForTest(t, value, key)
}

func objectForTest(t *testing.T, value any, label string) map[string]any {
	t.Helper()

	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("%s = %T, want object", label, value)
	}
	return object
}

func assertImportChangeValues(
	t *testing.T,
	name string,
	change *xylona.GameImportChange,
	wantPrevious string,
	wantImported string,
	wantPreviousContains string,
	wantImportedContains string,
) {
	t.Helper()

	if wantPrevious != "" && change.GetPreviousValue() != wantPrevious {
		t.Fatalf("ImportGame(PREVIEW %s).PreviousValue = %q, want %q", name, change.GetPreviousValue(), wantPrevious)
	}
	if wantImported != "" && change.GetImportedValue() != wantImported {
		t.Fatalf("ImportGame(PREVIEW %s).ImportedValue = %q, want %q", name, change.GetImportedValue(), wantImported)
	}
	if wantPreviousContains != "" && !strings.Contains(change.GetPreviousValue(), wantPreviousContains) {
		t.Fatalf("ImportGame(PREVIEW %s).PreviousValue = %q, want to contain %q", name, change.GetPreviousValue(), wantPreviousContains)
	}
	if wantImportedContains != "" && !strings.Contains(change.GetImportedValue(), wantImportedContains) {
		t.Fatalf("ImportGame(PREVIEW %s).ImportedValue = %q, want to contain %q", name, change.GetImportedValue(), wantImportedContains)
	}
}

func exportGameForTest(t *testing.T, fixture *rbacRPCFixture, gameID string) *connect.Response[xylona.ExportGameResponse] {
	t.Helper()

	req := connect.NewRequest(&xylona.ExportGameRequest{
		GameId: gameID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-admin")
	resp, errExport := fixture.service.ExportGame(context.Background(), req)
	if errExport != nil {
		t.Fatalf("ExportGame() error = %v", errExport)
	}
	return resp
}

func replaceOnce(t *testing.T, input string, old string, replacement string) string {
	t.Helper()
	output := strings.Replace(input, old, replacement, 1)
	if output == input {
		t.Fatalf("replacement target %q not found", old)
	}
	return output
}

func validationErrorsContain(validationErrors []string, substring string) bool {
	for _, validationError := range validationErrors {
		if strings.Contains(validationError, substring) {
			return true
		}
	}
	return false
}

func seedValheimVariantForImportTest(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()

	game, errGame := fixture.conn.GetGameByID("valheim")
	if errGame != nil {
		t.Fatalf("GetGameByID(valheim) error = %v", errGame)
	}
	gameConfig, errConfig := updateconfig.LoadGameConfigFromModel(game)
	if errConfig != nil {
		t.Fatalf("LoadGameConfigFromModel() error = %v", errConfig)
	}
	gameConfig.Variants = []updateproviders.Variant{
		{
			ID:            "stable",
			Name:          "Stable",
			DefaultTarget: "stable",
			UpdateProvider: &updateproviders.ProviderConfig{
				Kind: updateproviders.ProviderKindSteamCMD,
			},
		},
	}
	errSave := updateconfig.SaveGameConfigToModel(game, gameConfig)
	if errSave != nil {
		t.Fatalf("SaveGameConfigToModel() error = %v", errSave)
	}
	_, errUpdate := fixture.conn.UpdateGame(fixture.conn.DB, game, &models.GameSetter{
		ID:             omit.From(game.ID),
		ServerSoftware: omitnull.FromNull(game.ServerSoftware),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdate)
	}
}

func seedGameConfigSchemasForImportTest(t *testing.T, fixture *rbacRPCFixture, gameID string, configSchemas string) {
	t.Helper()

	game, errGame := fixture.conn.GetGameByID(gameID)
	if errGame != nil {
		t.Fatalf("GetGameByID(%s) error = %v", gameID, errGame)
	}
	_, errUpdate := fixture.conn.UpdateGame(fixture.conn.DB, game, &models.GameSetter{
		ID:            omit.From(game.ID),
		ConfigSchemas: omitnull.From(configSchemas),
	})
	if errUpdate != nil {
		t.Fatalf("UpdateGame() error = %v", errUpdate)
	}
}

func findImportChange(changes []*xylona.GameImportChange, path string) *xylona.GameImportChange {
	for _, change := range changes {
		if change.GetPath() == path {
			return change
		}
	}
	return nil
}
