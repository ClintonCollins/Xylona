package db

import (
	"database/sql"
	"encoding/json"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestJoinPasswordSetAndClear(t *testing.T) {
	sqlDB, errOpen := sql.Open("sqlite", ":memory:")
	if errOpen != nil {
		t.Fatal(errOpen)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() {
		errClose := sqlDB.Close()
		if errClose != nil {
			t.Error(errClose)
		}
	})
	conn := &Connection{ctx: t.Context(), SQLDb: sqlDB}
	_, errSeed := sqlDB.ExecContext(t.Context(), `create table game_server (id text primary key, start_args_patches text not null);
		insert into game_server values ('server', '[]')`)
	if errSeed != nil {
		t.Fatal(errSeed)
	}
	server := &models.GameServer{ID: "server", GameID: "valheim"}
	server.StartArgsPatches = `[{"id":"world","op":"edit","tokens":["-world","keep-world"]}]`
	for _, password := range []string{"two words ' with quotes", "replacement"} {
		errSet := conn.SetValheimJoinPassword(server, password)
		if errSet != nil {
			t.Fatal(errSet)
		}
		stored := &models.GameServer{GameID: "valheim"}
		errGet := sqlDB.QueryRowContext(t.Context(), "select start_args_patches from game_server where id = ?", server.ID).Scan(&stored.StartArgsPatches)
		if errGet != nil {
			t.Fatal(errGet)
		}
		value, errPassword := GetValheimJoinPassword(stored)
		if errPassword != nil || value != password {
			t.Fatalf("round trip = %q, %v", value, errPassword)
		}
		if !strings.Contains(stored.StartArgsPatches, password) || !strings.Contains(stored.StartArgsPatches, "keep-world") {
			t.Fatal("password or unrelated patch was not retained")
		}
	}
	errClear := conn.ClearValheimJoinPassword(server)
	if errClear != nil {
		t.Fatal(errClear)
	}
	stored := &models.GameServer{GameID: "valheim"}
	errGet := sqlDB.QueryRowContext(t.Context(), "select start_args_patches from game_server where id = ?", server.ID).Scan(&stored.StartArgsPatches)
	if errGet != nil {
		t.Fatal(errGet)
	}
	value, errPassword := GetValheimJoinPassword(stored)
	if errPassword != nil || value != "" {
		t.Fatalf("clear = %q, %v", value, errPassword)
	}
	var patches []startargs.Patch
	errDecode := json.Unmarshal([]byte(stored.StartArgsPatches), &patches)
	if errDecode != nil {
		t.Fatal(errDecode)
	}
	if len(patches) != 2 || patches[1].ID != valheimPasswordBlockID || patches[1].Op != startargs.PatchOpRemove {
		t.Fatal("clear must remove the password argument and retain unrelated patches")
	}
}

func TestGetValheimJoinPasswordTemplateAndPatches(t *testing.T) {
	server := &models.GameServer{GameID: "valheim"}
	server.R.Game = &models.Game{LinuxStartArgsTemplate: null.From(`[{"id":"01JQSD00000000000000000003","tokens":["-password","template password"]}]`)}
	for _, test := range []struct {
		name, patches, want string
		invalid             bool
	}{
		{name: "template", want: "template password"},
		{name: "cleared", patches: `[{"id":"01JQSD00000000000000000003","op":"edit","tokens":["-password",""]}]`},
		{name: "removed", patches: `[{"id":"01JQSD00000000000000000003","op":"remove"}]`},
		{name: "malformed", patches: `[`, invalid: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server.StartArgsPatches = test.patches
			password, errPassword := GetValheimJoinPassword(server)
			if password != test.want || (errPassword != nil) != test.invalid {
				t.Fatalf("password = %q, error = %v", password, errPassword)
			}
		})
	}
}
