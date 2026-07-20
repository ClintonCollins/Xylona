package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/admininterface"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGameServerAdminInterfacePasswordRoundTrip(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	seedAdminInterfaceGameServer(t, fixture, "7_days_to_die", "192.0.2.10", 26900, 26904)

	setRequest := connect.NewRequest(&xylona.SetGameServerAdminInterfacePasswordRequest{
		ServerId: "server-local-1",
		Password: "custom-admin-password",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setRequest, "user-admin")
	setResponse, errSet := fixture.service.SetGameServerAdminInterfacePassword(t.Context(), setRequest)
	if errSet != nil {
		t.Fatalf("SetGameServerAdminInterfacePassword() error = %v", errSet)
	}
	adminInterface := setResponse.Msg.GetAdminInterface()
	if !adminInterface.GetSupported() || !adminInterface.GetPasswordConfigured() ||
		!adminInterface.GetRemoteAccess() || adminInterface.GetTransport() != "Telnet" ||
		adminInterface.GetBindAddress() != "0.0.0.0" || adminInterface.GetPort() != 26904 {
		t.Fatalf("SetGameServerAdminInterfacePassword() admin interface = %+v", adminInterface)
	}

	password, configured, errDecrypt := fixture.conn.DecryptGameServerSecret(
		"server-local-1",
		db.GameServerSecretKindAdminInterface,
		db.GameServerSecretNameAdminInterfacePassword,
	)
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecret() error = %v", errDecrypt)
	}
	if !configured || password != "custom-admin-password" {
		t.Fatalf("DecryptGameServerSecret() = %q, %t", password, configured)
	}

	getRequest := connect.NewRequest(&xylona.GetGameServerAdminInterfaceRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-admin")
	getResponse, errGet := fixture.service.GetGameServerAdminInterface(t.Context(), getRequest)
	if errGet != nil {
		t.Fatalf("GetGameServerAdminInterface() error = %v", errGet)
	}
	if !getResponse.Msg.GetAdminInterface().GetPasswordConfigured() {
		t.Fatal("GetGameServerAdminInterface().PasswordConfigured = false, want true")
	}
}

func TestSatisfactoryAdminPasswordRetainsHistory(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	seedAdminInterfaceGameServer(t, fixture, "satisfactory", "192.0.2.20", 7777, 8888)

	errInitial := fixture.conn.SetGameServerSecret(
		"server-local-1",
		db.GameServerSecretKindAdminInterface,
		db.GameServerSecretNameAdminInterfacePassword,
		"previous-admin-password",
		"user-admin",
	)
	if errInitial != nil {
		t.Fatalf("SetGameServerSecret(initial password) error = %v", errInitial)
	}
	request := connect.NewRequest(&xylona.SetGameServerAdminInterfacePasswordRequest{
		ServerId: "server-local-1",
		Password: "replacement-admin-password",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
	_, errSet := fixture.service.SetGameServerAdminInterfacePassword(t.Context(), request)
	if errSet != nil {
		t.Fatalf("SetGameServerAdminInterfacePassword() error = %v", errSet)
	}

	rawHistory, configured, errDecrypt := fixture.conn.DecryptGameServerSecret(
		"server-local-1",
		db.GameServerSecretKindAdminInterface,
		db.GameServerSecretNameAdminInterfacePasswordHistory,
	)
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecret(history) error = %v", errDecrypt)
	}
	if !configured {
		t.Fatal("password history configured = false, want true")
	}
	history, errParse := admininterface.ParsePasswordHistory(rawHistory)
	if errParse != nil {
		t.Fatalf("ParsePasswordHistory() error = %v", errParse)
	}
	if len(history) != 1 || history[0] != "previous-admin-password" {
		t.Fatalf("password history = %q", history)
	}
}

func TestSetGameServerAdminInterfacePasswordValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		gameID   string
		password string
		wantCode connect.Code
	}{
		{name: "unsupported game", gameID: "minecraft", password: "valid-password", wantCode: connect.CodeFailedPrecondition},
		{name: "short password", gameID: "factorio", password: "short", wantCode: connect.CodeInvalidArgument},
		{name: "Palworld incompatible punctuation", gameID: "palworld", password: "invalid,password", wantCode: connect.CodeInvalidArgument},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			fixture := newRBACRPCFixture(t)
			fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
			if tc.name == "unsupported game" {
				_, errDefinition := fixture.conn.SQLDb.ExecContext(
					context.Background(),
					"update game set config_schemas = '[]' where id = ?",
					tc.gameID,
				)
				if errDefinition != nil {
					t.Fatalf("remove admin interface contract error = %v", errDefinition)
				}
			} else {
				seedAdminInterfaceGameServer(t, fixture, tc.gameID, "127.0.0.1", 27015, 27016)
			}

			request := connect.NewRequest(&xylona.SetGameServerAdminInterfacePasswordRequest{
				ServerId: "server-local-1",
				Password: tc.password,
			})
			addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-admin")
			_, errSet := fixture.service.SetGameServerAdminInterfacePassword(context.Background(), request)
			if connect.CodeOf(errSet) != tc.wantCode {
				t.Fatalf("SetGameServerAdminInterfacePassword() code = %v, want %v", connect.CodeOf(errSet), tc.wantCode)
			}
		})
	}
}

func TestGameServerAdminInterfaceRequiresEffectiveDefinitionContract(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	seedAdminInterfaceGameServer(t, fixture, "factorio", "192.0.2.30", 34197, 27015)

	_, errUpdate := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game
		 set linux_start_args_template = '[]',
		     windows_start_args_template = '[]',
		     official_definition_diverged = true
		 where id = ?`,
		"factorio",
	)
	if errUpdate != nil {
		t.Fatalf("remove Factorio admin contract: %v", errUpdate)
	}

	getRequest := connect.NewRequest(&xylona.GetGameServerAdminInterfaceRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getRequest, "user-admin")
	getResponse, errGet := fixture.service.GetGameServerAdminInterface(t.Context(), getRequest)
	if errGet != nil {
		t.Fatalf("GetGameServerAdminInterface() error = %v", errGet)
	}
	if getResponse.Msg.GetAdminInterface().GetSupported() {
		t.Fatal("GetGameServerAdminInterface().Supported = true, want false")
	}

	setRequest := connect.NewRequest(&xylona.SetGameServerAdminInterfacePasswordRequest{
		ServerId: "server-local-1",
		Password: "custom-admin-password",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setRequest, "user-admin")
	_, errSet := fixture.service.SetGameServerAdminInterfacePassword(t.Context(), setRequest)
	if connect.CodeOf(errSet) != connect.CodeFailedPrecondition {
		t.Fatalf(
			"SetGameServerAdminInterfacePassword() code = %v, want %v",
			connect.CodeOf(errSet),
			connect.CodeFailedPrecondition,
		)
	}
}

func seedAdminInterfaceGameServer(
	t *testing.T,
	fixture *rbacRPCFixture,
	gameID string,
	ip string,
	port int64,
	queryPort int64,
) {
	t.Helper()

	_, errIP := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		"insert into ip (address, usable, external, node_id) values (?, ?, ?, ?) on conflict(address, node_id) do nothing",
		ip,
		true,
		false,
		"node-local",
	)
	if errIP != nil {
		t.Fatalf("insert IP setup error = %v", errIP)
	}
	_, errUpdate := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		"update game_server set game_id = ?, ip = ?, port = ?, query_port = ? where id = ?",
		gameID,
		ip,
		port,
		queryPort,
		"server-local-1",
	)
	if errUpdate != nil {
		t.Fatalf("update game server setup error = %v", errUpdate)
	}
}
