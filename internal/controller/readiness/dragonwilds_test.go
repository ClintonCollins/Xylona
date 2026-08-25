package readiness

import (
	"context"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestDragonwildsConfigItem(t *testing.T) {
	tests := []struct {
		name         string
		operatingSys string
		contents     string
		wantComplete bool
		wantMessage  string
		wantPath     string
	}{
		{
			name:         "Windows complete",
			operatingSys: "windows",
			contents:     "OwnerId=owner-id\nServerName=Server\nDefaultWorldName=World\nAdminPassword=secret\n",
			wantComplete: true,
			wantMessage:  "configuration is ready",
			wantPath:     "RSDragonwilds/Saved/Config/WindowsServer/DedicatedServer.ini",
		},
		{
			name:         "Linux reports missing required values",
			operatingSys: "linux",
			contents:     "OwnerId=\nServerName=Server\nDefaultWorldName=World\nAdminPassword=\n",
			wantComplete: false,
			wantMessage:  "Admin Password, Owner ID",
			wantPath:     "RSDragonwilds/Saved/Config/Linux/DedicatedServer.ini",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &readinessNodeClientFake{
				snapshotResult: &node.NodeSnapshot{OS: test.operatingSys},
				readFileResult: []byte(test.contents),
			}
			gameServer := &models.GameServer{
				GameID:    "runescape_dragonwilds",
				Directory: "/srv/dragonwilds",
			}

			item, errItem := dragonwildsConfigItem(context.Background(), gameServer, client)
			if errItem != nil {
				t.Fatalf("dragonwildsConfigItem() error = %v", errItem)
			}
			if item.Complete != test.wantComplete || item.Blocking == test.wantComplete {
				t.Fatalf("dragonwildsConfigItem() = %+v, want complete %t", item, test.wantComplete)
			}
			if !strings.Contains(item.Message, test.wantMessage) {
				t.Fatalf("message = %q, want substring %q", item.Message, test.wantMessage)
			}
			if len(client.readFileCalls) != 1 || client.readFileCalls[0].relativePath != test.wantPath {
				t.Fatalf("ReadFile calls = %+v, want path %q", client.readFileCalls, test.wantPath)
			}
		})
	}
}

func TestCheckStartBlocksIncompleteDragonwildsConfig(t *testing.T) {
	client := &readinessNodeClientFake{
		snapshotResult: &node.NodeSnapshot{OS: "linux"},
		readFileResult: []byte("ServerName=Server\nDefaultWorldName=World\n"),
	}
	gameServer := &models.GameServer{
		GameID:    "runescape_dragonwilds",
		Directory: "/srv/dragonwilds",
	}

	errCheck := CheckStart(context.Background(), nil, gameServer, client)
	if errCheck == nil {
		t.Fatal("CheckStart() error = nil, want incomplete configuration error")
	}
	if !strings.Contains(errCheck.Error(), "Admin Password") || !strings.Contains(errCheck.Error(), "Owner ID") {
		t.Fatalf("CheckStart() error = %v, want missing Dragonwilds values", errCheck)
	}
}
