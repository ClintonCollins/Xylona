package rpc

import (
	"errors"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/controller/launchenv"
	"github.com/ClintonCollins/Xylona/internal/gameintegrations"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestPrepareCreateGameServerEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		game      *models.Game
		variables []*xylona.EnvironmentVariable
		want      []launchenv.Variable
		wantErr   error
	}{
		{
			name:      "ordinary game accepts visible environment",
			game:      &models.Game{ID: "other", DefaultEnvVars: "[]"},
			variables: []*xylona.EnvironmentVariable{{Name: "EXAMPLE", Value: "value"}},
			want:      []launchenv.Variable{{Name: "EXAMPLE", Value: "value"}},
		},
		{
			name:      "Starbound accepts owned account name",
			game:      &models.Game{ID: gameintegrations.StarboundGameID, DefaultEnvVars: "[]"},
			variables: []*xylona.EnvironmentVariable{{Name: gameintegrations.StarboundSteamUsernameEnv, Value: "owner"}},
			want:      []launchenv.Variable{{Name: gameintegrations.StarboundSteamUsernameEnv, Value: "owner"}},
		},
		{
			name: "Starbound accepts configured game default",
			game: &models.Game{
				ID:             gameintegrations.StarboundGameID,
				DefaultEnvVars: `[{"name":"STEAM_USERNAME","value":"owner"}]`,
			},
			want: []launchenv.Variable{},
		},
		{
			name:    "Starbound rejects missing account name",
			game:    &models.Game{ID: gameintegrations.StarboundGameID, DefaultEnvVars: "[]"},
			wantErr: gameintegrations.ErrStarboundSteamUsernameRequired,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			encoded, errPrepare := prepareCreateGameServerEnvironment(test.game, test.variables)
			if test.wantErr != nil {
				if !errors.Is(errPrepare, test.wantErr) {
					t.Fatalf("prepareCreateGameServerEnvironment() error = %v, want %v", errPrepare, test.wantErr)
				}
				return
			}
			if errPrepare != nil {
				t.Fatalf("prepareCreateGameServerEnvironment() error = %v", errPrepare)
			}
			got, errParse := launchenv.ParseStored(encoded)
			if errParse != nil {
				t.Fatalf("ParseStored() error = %v", errParse)
			}
			if len(got) != len(test.want) {
				t.Fatalf("stored environment = %+v, want %+v", got, test.want)
			}
			for i := range test.want {
				if got[i] != test.want[i] {
					t.Fatalf("stored environment[%d] = %+v, want %+v", i, got[i], test.want[i])
				}
			}
		})
	}
}
