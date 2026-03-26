package rpc

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCreateGameServerRequiresSuperUserForAllCreates(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)

	request := connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			UserId:        "user-owner",
			Name:          "Created Server",
			GameId:        "minecraft",
			SetMaxPlayers: 12,
			MaxPlayers:    24,
			Ip:            &xylona.IP{Address: "127.0.0.2"},
			Port:          25570,
			QueryPort:     25571,
			NodeId:        "node-local",
			MaxMemoryMb:   1024,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errCreate := fixture.service.CreateGameServer(context.Background(), request)
	if errCreate == nil {
		t.Fatalf("CreateGameServer(non-super) expected error, got nil")
	}
	if connect.CodeOf(errCreate) != connect.CodePermissionDenied {
		t.Fatalf("CreateGameServer(non-super) code = %v, want %v", connect.CodeOf(errCreate), connect.CodePermissionDenied)
	}
}

func TestCreateGameServerAllowsSuperUser(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)

	var gotGameID string
	var gotOwnerID string
	var gotModel *models.GameServer

	fixture.service.installGameServerFn = func(game *models.Game, gameServer *models.GameServer, owner *models.User) (*models.GameServer, error) {
		gotGameID = game.ID
		gotOwnerID = owner.ID
		copied := *gameServer
		copied.ID = "server-created-1"
		gotModel = &copied
		return &copied, nil
	}

	request := connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			UserId:        "user-owner",
			Name:          "Created Server",
			GameId:        "minecraft",
			SetMaxPlayers: 18,
			MaxPlayers:    32,
			Ip:            &xylona.IP{Address: "127.0.0.2"},
			Port:          25570,
			QueryPort:     25571,
			NodeId:        "node-local",
			MaxMemoryMb:   1024,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errCreate := fixture.service.CreateGameServer(context.Background(), request)
	if errCreate != nil {
		t.Fatalf("CreateGameServer(superuser) error = %v", errCreate)
	}
	if gotModel == nil {
		t.Fatalf("CreateGameServer(superuser) did not invoke installer")
	}
	if gotGameID != "minecraft" {
		t.Fatalf("CreateGameServer(superuser) game id = %q, want %q", gotGameID, "minecraft")
	}
	if gotOwnerID != "user-owner" {
		t.Fatalf("CreateGameServer(superuser) owner id = %q, want %q", gotOwnerID, "user-owner")
	}
	if response.Msg.GetGameServer() == nil {
		t.Fatalf("CreateGameServer(superuser) returned nil game server")
	}
	if response.Msg.GetGameServer().GetId() != "server-created-1" {
		t.Fatalf("CreateGameServer(superuser) response id = %q, want %q", response.Msg.GetGameServer().GetId(), "server-created-1")
	}
}

func TestEditGameServerRestrictsProvisioningFieldsForNonSuperUsers(t *testing.T) {
	tests := []struct {
		name     string
		editorID string
		setup    func(t *testing.T, fixture *rbacRPCFixture)
	}{
		{
			name:     "owner keeps provisioning locked",
			editorID: "user-owner",
			setup:    func(_ *testing.T, _ *rbacRPCFixture) {},
		},
		{
			name:     "role granted admin keeps provisioning locked",
			editorID: "user-other",
			setup: func(t *testing.T, fixture *rbacRPCFixture) {
				t.Helper()
				errAssign := fixture.conn.CreateUserRoleAssignment(
					"assignment-settings-admin",
					"user-other",
					"admin",
					"server-local-1",
					"user-owner",
				)
				if errAssign != nil {
					t.Fatalf("CreateUserRoleAssignment() error = %v", errAssign)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newRBACRPCFixture(t)
			tt.setup(t, fixture)

			existing, errGet := fixture.conn.GetGameServerByID("server-local-1")
			if errGet != nil {
				t.Fatalf("GetGameServerByID() error = %v", errGet)
			}

			requestServer := helpers.GameServerModelToProto(existing, nil)
			requestServer.Name = "Renamed Server"
			requestServer.SetMaxPlayers = 14
			requestServer.UserId = "user-admin"
			requestServer.GameId = "missing-game"
			requestServer.NodeId = "missing-node"
			requestServer.Ip = &xylona.IP{Address: "203.0.113.10"}
			requestServer.Port = 28000
			requestServer.QueryPort = 28001
			requestServer.MaxPlayers = 64
			requestServer.MaxMemoryMb = 4096

			request := connect.NewRequest(&xylona.EditGameServerRequest{
				ServerId:   existing.ID,
				GameServer: requestServer,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, tt.editorID)

			response, errEdit := fixture.service.EditGameServer(context.Background(), request)
			if errEdit != nil {
				t.Fatalf("EditGameServer() error = %v", errEdit)
			}

			if response.Msg.GetGame_Server() == nil {
				t.Fatalf("EditGameServer() returned nil game server")
			}
			if response.Msg.GetGame_Server().GetName() != "Renamed Server" {
				t.Fatalf("EditGameServer().GameServer.Name = %q, want %q", response.Msg.GetGame_Server().GetName(), "Renamed Server")
			}
			if response.Msg.GetGame_Server().GetSetMaxPlayers() != 14 {
				t.Fatalf("EditGameServer().GameServer.SetMaxPlayers = %d, want %d", response.Msg.GetGame_Server().GetSetMaxPlayers(), 14)
			}
			if response.Msg.GetGame_Server().GetGameId() != existing.GameID {
				t.Fatalf("EditGameServer().GameServer.GameId = %q, want existing %q", response.Msg.GetGame_Server().GetGameId(), existing.GameID)
			}

			stored, errStored := fixture.conn.GetGameServerByID(existing.ID)
			if errStored != nil {
				t.Fatalf("GetGameServerByID(updated) error = %v", errStored)
			}
			if stored.Name != "Renamed Server" {
				t.Errorf("stored.Name = %q, want %q", stored.Name, "Renamed Server")
			}
			if stored.SetPlayers != 14 {
				t.Errorf("stored.SetPlayers = %d, want %d", stored.SetPlayers, 14)
			}
			if stored.UserID != existing.UserID {
				t.Errorf("stored.UserID = %q, want %q", stored.UserID, existing.UserID)
			}
			if stored.GameID != existing.GameID {
				t.Errorf("stored.GameID = %q, want %q", stored.GameID, existing.GameID)
			}
			if stored.NodeID != existing.NodeID {
				t.Errorf("stored.NodeID = %q, want %q", stored.NodeID, existing.NodeID)
			}
			if stored.IP != existing.IP {
				t.Errorf("stored.IP = %q, want %q", stored.IP, existing.IP)
			}
			if stored.Port != existing.Port {
				t.Errorf("stored.Port = %d, want %d", stored.Port, existing.Port)
			}
			if stored.QueryPort != existing.QueryPort {
				t.Errorf("stored.QueryPort = %d, want %d", stored.QueryPort, existing.QueryPort)
			}
			if stored.MaxPlayers != existing.MaxPlayers {
				t.Errorf("stored.MaxPlayers = %d, want %d", stored.MaxPlayers, existing.MaxPlayers)
			}
			if stored.MaxMemoryMB != existing.MaxMemoryMB {
				t.Errorf("stored.MaxMemoryMB = %d, want %d", stored.MaxMemoryMB, existing.MaxMemoryMB)
			}
		})
	}
}

func TestEditGameServerAllowsSuperUserToChangeProvisioningFields(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	seedTestGame(t, fixture, "test-game")

	existing, errGet := fixture.conn.GetGameServerByID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGet)
	}

	requestServer := helpers.GameServerModelToProto(existing, nil)
	requestServer.Name = "Superuser Updated"
	requestServer.UserId = "user-other"
	requestServer.GameId = "test-game"
	requestServer.NodeId = "node-alt"
	requestServer.Ip = &xylona.IP{Address: "127.0.0.2"}
	requestServer.Port = 28000
	requestServer.QueryPort = 28001
	requestServer.SetMaxPlayers = 22
	requestServer.MaxPlayers = 48
	requestServer.MaxMemoryMb = 3072

	request := connect.NewRequest(&xylona.EditGameServerRequest{
		ServerId:   existing.ID,
		GameServer: requestServer,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errEdit := fixture.service.EditGameServer(context.Background(), request)
	if errEdit != nil {
		t.Fatalf("EditGameServer(superuser) error = %v", errEdit)
	}
	if response.Msg.GetGame_Server() == nil {
		t.Fatalf("EditGameServer(superuser) returned nil game server")
	}
	if response.Msg.GetGame_Server().GetGameId() != "test-game" {
		t.Fatalf("EditGameServer(superuser) game id = %q, want %q", response.Msg.GetGame_Server().GetGameId(), "test-game")
	}
	if response.Msg.GetGame_Server().GetNodeId() != "node-alt" {
		t.Fatalf("EditGameServer(superuser) node id = %q, want %q", response.Msg.GetGame_Server().GetNodeId(), "node-alt")
	}
	if response.Msg.GetGame_Server().GetUserId() != "user-other" {
		t.Fatalf("EditGameServer(superuser) user id = %q, want %q", response.Msg.GetGame_Server().GetUserId(), "user-other")
	}

	stored, errStored := fixture.conn.GetGameServerByID(existing.ID)
	if errStored != nil {
		t.Fatalf("GetGameServerByID(updated) error = %v", errStored)
	}
	if stored.GameID != "test-game" {
		t.Errorf("stored.GameID = %q, want %q", stored.GameID, "test-game")
	}
	if stored.NodeID != "node-alt" {
		t.Errorf("stored.NodeID = %q, want %q", stored.NodeID, "node-alt")
	}
	if stored.UserID != "user-other" {
		t.Errorf("stored.UserID = %q, want %q", stored.UserID, "user-other")
	}
	if stored.IP != "127.0.0.2" {
		t.Errorf("stored.IP = %q, want %q", stored.IP, "127.0.0.2")
	}
	if stored.Port != 28000 {
		t.Errorf("stored.Port = %d, want %d", stored.Port, 28000)
	}
	if stored.QueryPort != 28001 {
		t.Errorf("stored.QueryPort = %d, want %d", stored.QueryPort, 28001)
	}
	if stored.SetPlayers != 22 {
		t.Errorf("stored.SetPlayers = %d, want %d", stored.SetPlayers, 22)
	}
	if stored.MaxPlayers != 48 {
		t.Errorf("stored.MaxPlayers = %d, want %d", stored.MaxPlayers, 48)
	}
	if stored.MaxMemoryMB != 3072 {
		t.Errorf("stored.MaxMemoryMB = %d, want %d", stored.MaxMemoryMB, 3072)
	}
}

func TestFederationEditRemoteServerRestrictsProvisioningFieldsForNonSuperUsers(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedRemoteNodeForRBACRPCTests(t, fixture.conn, "node-remote")

	errGrant := fixture.conn.CreateFederatedAccessGrant(
		"fed-grant-settings-admin",
		"server-local-1",
		"node-remote",
		"user-owner",
		"owner",
		"admin",
		"user-admin",
	)
	if errGrant != nil {
		t.Fatalf("CreateFederatedAccessGrant() error = %v", errGrant)
	}

	service := FederationService{db: fixture.conn}
	peerCtx := context.WithValue(context.Background(), federationPeerIdentityKey, FederationPeerIdentity{
		NodeID:     "node-remote",
		PeerNodeID: "peer-node-id",
	})

	existing, errGet := fixture.conn.GetGameServerByID("server-local-1")
	if errGet != nil {
		t.Fatalf("GetGameServerByID() error = %v", errGet)
	}

	requestServer := helpers.GameServerModelToProto(existing, nil)
	requestServer.Name = "Federated Rename"
	requestServer.SetMaxPlayers = 16
	requestServer.UserId = "user-admin"
	requestServer.GameId = "missing-game"
	requestServer.NodeId = "missing-node"
	requestServer.Ip = &xylona.IP{Address: "203.0.113.20"}
	requestServer.Port = 29000
	requestServer.QueryPort = 29001
	requestServer.MaxPlayers = 80
	requestServer.MaxMemoryMb = 6144

	request := connect.NewRequest(&xylona.FederationEditServerRequest{
		ServerId:   existing.ID,
		GameServer: requestServer,
	})
	request.Header().Set(helpers.FederationActingUserIDHeader, "user-owner")
	request.Header().Set(helpers.FederationOriginNodeIDHeader, "node-remote")

	response, errEdit := service.EditRemoteServer(peerCtx, request)
	if errEdit != nil {
		t.Fatalf("EditRemoteServer() error = %v", errEdit)
	}
	if !response.Msg.GetSuccess() {
		t.Fatalf("EditRemoteServer() success = false, want true")
	}
	if response.Msg.GetGameServer().GetGameId() != existing.GameID {
		t.Fatalf("EditRemoteServer().GameServer.GameId = %q, want existing %q", response.Msg.GetGameServer().GetGameId(), existing.GameID)
	}

	stored, errStored := fixture.conn.GetGameServerByID(existing.ID)
	if errStored != nil {
		t.Fatalf("GetGameServerByID(updated) error = %v", errStored)
	}
	if stored.Name != "Federated Rename" {
		t.Errorf("stored.Name = %q, want %q", stored.Name, "Federated Rename")
	}
	if stored.SetPlayers != 16 {
		t.Errorf("stored.SetPlayers = %d, want %d", stored.SetPlayers, 16)
	}
	if stored.GameID != existing.GameID {
		t.Errorf("stored.GameID = %q, want %q", stored.GameID, existing.GameID)
	}
	if stored.NodeID != existing.NodeID {
		t.Errorf("stored.NodeID = %q, want %q", stored.NodeID, existing.NodeID)
	}
	if stored.IP != existing.IP {
		t.Errorf("stored.IP = %q, want %q", stored.IP, existing.IP)
	}
	if stored.Port != existing.Port {
		t.Errorf("stored.Port = %d, want %d", stored.Port, existing.Port)
	}
	if stored.QueryPort != existing.QueryPort {
		t.Errorf("stored.QueryPort = %d, want %d", stored.QueryPort, existing.QueryPort)
	}
	if stored.MaxPlayers != existing.MaxPlayers {
		t.Errorf("stored.MaxPlayers = %d, want %d", stored.MaxPlayers, existing.MaxPlayers)
	}
	if stored.MaxMemoryMB != existing.MaxMemoryMB {
		t.Errorf("stored.MaxMemoryMB = %d, want %d", stored.MaxMemoryMB, existing.MaxMemoryMB)
	}
}

func seedAlternateNodeAndIP(t *testing.T, fixture *rbacRPCFixture) {
	t.Helper()

	_, errNode := fixture.conn.InsertNode(&models.NodeSetter{
		ID:      omit.From("node-alt"),
		Name:    omit.From("Alternate Node"),
		IsLocal: omit.From(false),
		Host:    omit.From("node-alt.local"),
		Port:    omit.From(int64(8081)),
		BaseURL: omit.From("http://node-alt.local:8081"),
		Enabled: omit.From(true),
	})
	if errNode != nil {
		t.Fatalf("InsertNode() error = %v", errNode)
	}

	_, errIP := fixture.conn.UpsertIP(&models.IPSetter{
		Address:            omit.From("127.0.0.2"),
		Usable:             omit.From(true),
		External:           omit.From(false),
		AutomaticallyAdded: omit.From(false),
	})
	if errIP != nil {
		t.Fatalf("UpsertIP() error = %v", errIP)
	}
}

func seedTestGame(t *testing.T, fixture *rbacRPCFixture, gameID string) {
	t.Helper()

	now := time.Now().UTC()
	_, errInsert := fixture.conn.InsertGame(fixture.conn.DB, &models.GameSetter{
		ID:                omit.From(gameID),
		Name:              omit.From("Test Game"),
		DefaultPort:       omit.From(int64(28000)),
		DefaultQueryPort:  omit.From(int64(28001)),
		DefaultMaxPlayers: omit.From(int64(48)),
		WindowsSupport:    omit.From(true),
		CreatedAt:         omit.From(now),
		UpdatedAt:         omit.From(now),
	})
	if errInsert != nil {
		t.Fatalf("InsertGame() error = %v", errInsert)
	}
}
