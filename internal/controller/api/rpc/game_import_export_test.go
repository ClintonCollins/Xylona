package rpc

import (
	"context"
	"strings"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
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
	req := connect.NewRequest(&xylona.ImportGameRequest{
		GameDefinitionJson: exportResp.Msg.GetGameDefinitionJson(),
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
