package readiness

import (
	"encoding/json"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestJoinPasswordReadiness(t *testing.T) {
	for _, test := range []struct {
		name, password string
		blocked        bool
	}{
		{name: "missing"},
		{name: "configured", password: "private value"},
		{name: "identifier accepted", password: "server-legacy"},
		{name: "invalid", password: "tiny", blocked: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			patches, errMarshal := json.Marshal([]startargs.Patch{{ID: "01JQSD00000000000000000003", Op: startargs.PatchOpEdit, Tokens: []string{"-password", test.password}}})
			if errMarshal != nil {
				t.Fatal(errMarshal)
			}
			server := &models.GameServer{ID: "server-legacy", Name: "Forest", GameID: "valheim", StartArgsPatches: string(patches)}
			state := GetJoinPasswordState(nil, server)
			if !state.Supported || state.Configured != (test.password != "") {
				t.Fatal("incorrect configured state")
			}
			items, errList := List(t.Context(), nil, server, nil)
			if errList != nil {
				t.Fatal(errList)
			}
			for _, item := range items {
				if item.Kind == "join_password" {
					t.Fatal("optional password appeared in readiness")
				}
			}
			errStart := ValidateJoinPasswordStart(nil, server)
			if (errStart != nil) != test.blocked {
				t.Fatalf("start blocked = %v, want %v", errStart != nil, test.blocked)
			}
		})
	}
}
