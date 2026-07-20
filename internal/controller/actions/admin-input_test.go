package actions

import (
	"errors"
	"strconv"
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestGameServerAdminInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		gameID          string
		wantTelnet      bool
		wantRCON        bool
		wantREST        bool
		wantPort        int
		wantProtocol    node.RCONProtocol
		wantManaged     bool
		wantCapability  string
		managedContract bool
		diverged        bool
	}{
		{name: "7 Days to Die uses password-protected Telnet", gameID: sevenDaysToDieGameID, wantTelnet: true, wantPort: 27016, wantManaged: true, wantCapability: "telnet", managedContract: true},
		{name: "Source engine RCON uses game port", gameID: counterStrikeTwoGameID, wantRCON: true, wantPort: 27015, wantProtocol: node.RCONProtocolSource, wantManaged: true, wantCapability: "rcon", managedContract: true},
		{name: "Factorio RCON uses query port", gameID: factorioGameID, wantRCON: true, wantPort: 27016, wantProtocol: node.RCONProtocolSource, wantCapability: "rcon", managedContract: true},
		{name: "diverged Factorio definition retains managed RCON", gameID: factorioGameID, wantRCON: true, wantPort: 27016, wantProtocol: node.RCONProtocolSource, wantCapability: "rcon", managedContract: true, diverged: true},
		{name: "legacy Factorio definition preserves stdin", gameID: factorioGameID},
		{name: "V Rising RCON uses query port", gameID: "v_rising", wantRCON: true, wantPort: 27016, wantProtocol: node.RCONProtocolSource, wantCapability: "rcon", managedContract: true},
		{name: "Rust uses WebRCON", gameID: rustGameID, wantRCON: true, wantPort: 27016, wantProtocol: node.RCONProtocolRustWeb, wantCapability: "rcon", managedContract: true},
		{name: "Conan uses Minecraft RCON", gameID: "conan_exiles", wantRCON: true, wantPort: 27016, wantProtocol: node.RCONProtocolMinecraft, wantCapability: "rcon", managedContract: true},
		{name: "Satisfactory uses authenticated REST", gameID: "satisfactory", wantREST: true, wantPort: 27015, wantCapability: "rest", managedContract: true},
		{name: "Minecraft uses managed RCON", gameID: minecraftGameID, wantRCON: true, wantPort: 27017, wantProtocol: node.RCONProtocolMinecraft, wantManaged: true, wantCapability: "rcon", managedContract: true},
		{name: "legacy Minecraft definition preserves stdin", gameID: minecraftGameID},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gameServer := &models.GameServer{
				ID:        "server-admin-input",
				GameID:    tc.gameID,
				IP:        "0.0.0.0",
				Port:      27015,
				QueryPort: 27016,
			}
			if tc.managedContract {
				gameServer.R.Game = adminInputTestGameDefinition(tc.gameID)
				gameServer.R.Game.OfficialDefinitionDiverged = tc.diverged
			} else {
				gameServer.R.Game = &models.Game{ID: tc.gameID, LinuxSupport: true}
			}
			input, errInput := newGameServerAdminInput(
				gameServer,
				"custom-admin-password",
				[]string{"previous-admin-password"},
			)
			if errInput != nil {
				t.Fatalf("newGameServerAdminInput() error = %v", errInput)
			}
			if tc.wantTelnet != (input.telnet != nil) || tc.wantRCON != (input.rcon != nil) ||
				tc.wantREST != (input.rest != nil) ||
				tc.wantManaged != input.managedConfigRequired {
				t.Fatalf("admin input = %+v", input)
			}
			if input.telnet != nil {
				if input.telnet.Port != tc.wantPort || input.telnet.Password != "custom-admin-password" ||
					input.localConsolePassword != input.telnet.Password {
					t.Fatalf("Telnet input = %+v", input.telnet)
				}
				if tc.gameID == sevenDaysToDieGameID &&
					(input.placeholderVars[sevenDaysToDieWebAPITokenNamePlaceholder] != sevenDaysToDieWebAPITokenName ||
						input.placeholderVars[sevenDaysToDieWebAPITokenSecretPlaceholder] == "") {
					t.Fatalf("7 Days to Die WebAPI vars = %+v", input.placeholderVars)
				}
			}
			if input.rcon != nil {
				if input.rcon.Host != "127.0.0.1" || input.rcon.Port != tc.wantPort ||
					input.rcon.Protocol != tc.wantProtocol || input.rcon.Password != "custom-admin-password" ||
					input.placeholderVars["RCON_PASSWORD"] != input.rcon.Password ||
					input.placeholderVars["RCON_BIND"] != "0.0.0.0:"+strconv.Itoa(tc.wantPort) {
					t.Fatalf("RCON input = %+v, vars = %+v", input.rcon, input.placeholderVars)
				}
			}
			if input.rest != nil {
				if input.rest.Host != "127.0.0.1" || input.rest.Port != tc.wantPort ||
					input.rest.Kind != node.RESTInputKindSatisfactory || input.rest.Password != "custom-admin-password" ||
					len(input.rest.PreviousPasswords) != 1 ||
					input.rest.PreviousPasswords[0] != "previous-admin-password" {
					t.Fatalf("REST input = %+v", input.rest)
				}
			}

			client := &nodeclient.FakeNodeClient{}
			errSupported := (&Instance{}).ensureAdminInputSupported(client, input)
			if tc.wantCapability == "" {
				if errSupported != nil || client.RuntimeCapabilitiesCalls != 0 {
					t.Fatalf("capability check = %v, calls = %d", errSupported, client.RuntimeCapabilitiesCalls)
				}
				return
			}
			var startError *StartGameServerError
			if !errors.As(errSupported, &startError) || startError.Kind != StartFailureConfiguration {
				t.Fatalf("capability check error = %v, want start configuration error", errSupported)
			}
		})
	}
}

func adminInputTestGameDefinition(gameID string) *models.Game {
	game := &models.Game{ID: gameID}
	switch gameID {
	case minecraftGameID, sevenDaysToDieGameID, counterStrikeTwoGameID, garrysModGameID, teamFortressTwoGameID:
		game.ConfigSchemas = null.From(`[{"managed_fields":{"rcon_password":"xylona.local_console_password"}}]`)
	case factorioGameID:
		game.LinuxSupport = true
		game.LinuxStartArgsTemplate = null.From(`[{"ownership":"system","tokens":["--rcon-bind","{{RCON_BIND}}","--rcon-password","{{RCON_PASSWORD}}"]}]`)
	case "v_rising":
		game.WindowsSupport = true
		game.WindowsStartArgsTemplate = null.From(`[{"ownership":"system","tokens":["-rconEnabled","true","-rconPort","{{RCON_PORT}}","-rconPassword","{{RCON_PASSWORD}}","-rconBindAddress","{{IP}}"]}]`)
	case rustGameID:
		game.LinuxSupport = true
		game.LinuxStartArgsTemplate = null.From(`[{"ownership":"system","tokens":["+server.ip","{{IP}}"]},{"ownership":"system","tokens":["+rcon.ip","{{IP}}","+rcon.port","{{RCON_PORT}}","+rcon.password","{{RCON_PASSWORD}}","+rcon.web","1"]}]`)
	case "conan_exiles":
		game.LinuxSupport = true
		game.LinuxStartArgsTemplate = null.From(`[{"ownership":"system","tokens":["-RconEnabled=1","-RconPort={{RCON_PORT}}","-RconPassword={{RCON_PASSWORD}}"]}]`)
	case "satisfactory":
		game.LinuxSupport = true
		game.LinuxStartArgsTemplate = null.From(`[{"ownership":"system","tokens":["-multihome={{IP}}","-ini:Engine:[HTTPServer.Listeners]:DefaultBindAddress={{IP}}"]}]`)
	}
	return game
}
