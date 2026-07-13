package rpc

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestCreateGameServerPersistsRuntimeOnlyNodeIP(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	seedTestGame(t, fixture)

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-alt",
		SnapshotResult: &node.NodeSnapshot{OS: "windows"},
		BindableIPsResult: []node.BindableIP{
			{Address: "203.0.113.42", Usable: true, External: true},
		},
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.installGameServerFn = func(_ *models.Game, gameServer *models.GameServer, _ *models.User) (*models.GameServer, error) {
		return gameServer, nil
	}

	request := connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			UserId:        "user-owner",
			Name:          "Runtime Remote",
			GameId:        "test-game",
			SetMaxPlayers: 20,
			MaxPlayers:    20,
			Ip:            &xylona.IP{Address: "203.0.113.42"},
			Port:          28000,
			QueryPort:     28001,
			Directory:     "C:/servers/runtime-remote",
			NodeId:        "node-alt",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	response, errCreate := fixture.service.CreateGameServer(context.Background(), request)
	if errCreate != nil {
		t.Fatalf("CreateGameServer() error = %v", errCreate)
	}
	if response.Msg.GetGameServer().GetIp().GetAddress() != "203.0.113.42" {
		t.Fatalf("CreateGameServer().GameServer.Ip.Address = %q, want %q", response.Msg.GetGameServer().GetIp().GetAddress(), "203.0.113.42")
	}

	persistedIP, errGetIP := fixture.conn.GetIPByNodeIDAndAddress("node-alt", "203.0.113.42")
	if errGetIP != nil {
		t.Fatalf("GetIPByNodeIDAndAddress() error = %v", errGetIP)
	}
	if !persistedIP.Usable {
		t.Errorf("persisted IP usable = %v, want %v", persistedIP.Usable, true)
	}
	if !persistedIP.External {
		t.Errorf("persisted IP external = %v, want %v", persistedIP.External, true)
	}
	if !persistedIP.AutomaticallyAdded {
		t.Errorf("persisted IP automatically_added = %v, want %v", persistedIP.AutomaticallyAdded, true)
	}
	if remoteClient.BindableIPsCalls != 1 {
		t.Fatalf("ListBindableIPs() call count = %d, want %d", remoteClient.BindableIPsCalls, 1)
	}
}

func TestCreateGameServerDefaultsBackupsFromPlatformCapability(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedTestGame(t, fixture)

	_, errDisableBackups := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		"update game set linux_allow_backups = false, windows_allow_backups = false where id = ?",
		"test-game",
	)
	if errDisableBackups != nil {
		t.Fatalf("disable game backup capability: %v", errDisableBackups)
	}

	var installedServer *models.GameServer
	fixture.service.installGameServerFn = func(_ *models.Game, gameServer *models.GameServer, _ *models.User) (*models.GameServer, error) {
		installedServer = gameServer
		return gameServer, nil
	}

	request := connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			UserId:         "user-owner",
			Name:           "Unsupported Backups",
			GameId:         "test-game",
			SetMaxPlayers:  20,
			MaxPlayers:     20,
			Ip:             &xylona.IP{Address: "127.0.0.1"},
			Port:           28000,
			QueryPort:      28001,
			Directory:      "C:/servers/unsupported-backups",
			NodeId:         "node-local",
			BackupsEnabled: true,
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errCreate := fixture.service.CreateGameServer(context.Background(), request)
	if errCreate != nil {
		t.Fatalf("CreateGameServer() error = %v", errCreate)
	}
	if installedServer == nil {
		t.Fatal("installGameServerFn was not called")
	}
	if installedServer.BackupsEnabled {
		t.Fatal("installed game server BackupsEnabled = true, want false")
	}
}

func TestCreateGameServerRejectsRemoteNodeWithoutAuthoritativePlatform(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	seedTestGame(t, fixture)

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID: "node-alt",
		BindableIPsResult: []node.BindableIP{
			{Address: "203.0.113.42", Usable: true, External: true},
		},
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	fixture.service.nodeRegistry = registry

	installCalled := false
	fixture.service.installGameServerFn = func(_ *models.Game, gameServer *models.GameServer, _ *models.User) (*models.GameServer, error) {
		installCalled = true
		return gameServer, nil
	}

	request := connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			UserId:        "user-owner",
			Name:          "Unknown Remote Platform",
			GameId:        "test-game",
			SetMaxPlayers: 20,
			MaxPlayers:    20,
			Ip:            &xylona.IP{Address: "203.0.113.42"},
			Port:          28000,
			QueryPort:     28001,
			Directory:     "C:/servers/unknown-platform",
			NodeId:        "node-alt",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errCreate := fixture.service.CreateGameServer(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateGameServer() error = nil, want failed precondition")
	}
	if connect.CodeOf(errCreate) != connect.CodeFailedPrecondition {
		t.Fatalf("CreateGameServer() code = %v, want %v", connect.CodeOf(errCreate), connect.CodeFailedPrecondition)
	}
	if installCalled {
		t.Fatal("installGameServerFn should not be called without authoritative node platform")
	}
}

func TestCreateGameServerRejectsIPNotConfiguredForNode(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	seedAlternateNodeAndIP(t, fixture)
	seedTestGame(t, fixture)

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-alt",
		SnapshotResult: &node.NodeSnapshot{OS: "windows"},
		BindableIPsResult: []node.BindableIP{
			{Address: "203.0.113.42", Usable: true, External: true},
		},
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	fixture.service.nodeRegistry = registry

	installCalled := false
	fixture.service.installGameServerFn = func(_ *models.Game, gameServer *models.GameServer, _ *models.User) (*models.GameServer, error) {
		installCalled = true
		return gameServer, nil
	}

	request := connect.NewRequest(&xylona.CreateGameServerRequest{
		GameServer: &xylona.GameServer{
			UserId:        "user-owner",
			Name:          "Unconfigured Remote",
			GameId:        "test-game",
			SetMaxPlayers: 20,
			MaxPlayers:    20,
			Ip:            &xylona.IP{Address: "198.51.100.12"},
			Port:          28000,
			QueryPort:     28001,
			Directory:     "C:/servers/unconfigured-remote",
			NodeId:        "node-alt",
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")

	_, errCreate := fixture.service.CreateGameServer(context.Background(), request)
	if errCreate == nil {
		t.Fatal("CreateGameServer() error = nil, want invalid argument")
	}
	if connect.CodeOf(errCreate) != connect.CodeInvalidArgument {
		t.Fatalf("CreateGameServer() code = %v, want %v", connect.CodeOf(errCreate), connect.CodeInvalidArgument)
	}
	if installCalled {
		t.Fatal("installGameServerFn should not be called for an unconfigured IP")
	}

	_, errGetIP := fixture.conn.GetIPByNodeIDAndAddress("node-alt", "198.51.100.12")
	if !errors.Is(errGetIP, sql.ErrNoRows) {
		t.Fatalf("GetIPByNodeIDAndAddress() error = %v, want %v", errGetIP, sql.ErrNoRows)
	}
}
