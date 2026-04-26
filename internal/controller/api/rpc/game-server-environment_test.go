package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGameServerEnvironmentRoundTripDoesNotReturnSecretValues(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	updateReq := connect.NewRequest(&xylona.UpdateGameServerEnvironmentRequest{
		ServerId: "server-local-1",
		EnvVars: []*xylona.EnvironmentVariable{
			{Name: "VISIBLE_TOKEN", Value: "visible-value"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")

	updateResp, errUpdate := fixture.service.UpdateGameServerEnvironment(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerEnvironment() error = %v", errUpdate)
	}
	if len(updateResp.Msg.GetServerEnv()) != 1 {
		t.Fatalf("UpdateGameServerEnvironment().ServerEnv length = %d, want 1", len(updateResp.Msg.GetServerEnv()))
	}

	setReq := connect.NewRequest(&xylona.SetGameServerSecretEnvRequest{
		ServerId: "server-local-1",
		Name:     "SECRET_TOKEN",
		Value:    "secret-value",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-admin")

	setResp, errSet := fixture.service.SetGameServerSecretEnv(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() error = %v", errSet)
	}
	if len(setResp.Msg.GetSecretEnv()) != 1 {
		t.Fatalf("SetGameServerSecretEnv().SecretEnv length = %d, want 1", len(setResp.Msg.GetSecretEnv()))
	}

	getReq := connect.NewRequest(&xylona.GetGameServerEnvironmentRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")

	getResp, errGet := fixture.service.GetGameServerEnvironment(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGameServerEnvironment() error = %v", errGet)
	}
	gotValue := getResp.Msg.GetServerEnv()[0].GetValue()
	if gotValue != "visible-value" {
		t.Fatalf("GetGameServerEnvironment().ServerEnv[0].Value = %q, want visible value", gotValue)
	}
	if len(getResp.Msg.GetSecretEnv()) != 1 {
		t.Fatalf("GetGameServerEnvironment().SecretEnv length = %d, want 1", len(getResp.Msg.GetSecretEnv()))
	}
	if getResp.Msg.GetSecretEnv()[0].GetName() != "SECRET_TOKEN" {
		t.Fatalf("GetGameServerEnvironment().SecretEnv[0].Name = %q, want SECRET_TOKEN", getResp.Msg.GetSecretEnv()[0].GetName())
	}
	if !getResp.Msg.GetSecretEnv()[0].GetConfigured() {
		t.Fatal("GetGameServerEnvironment().SecretEnv[0].Configured = false, want true")
	}
}

func TestGameServerEnvironmentRequiresMutationPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	_, errGame := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		"update game set allow_start_arg_editing = false where id = ?",
		"minecraft",
	)
	if errGame != nil {
		t.Fatalf("update game setup error = %v", errGame)
	}

	ownerReq := connect.NewRequest(&xylona.UpdateGameServerEnvironmentRequest{
		ServerId: "server-local-1",
		EnvVars: []*xylona.EnvironmentVariable{
			{Name: "VISIBLE_TOKEN", Value: "visible-value"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, ownerReq, "user-owner")

	_, errOwner := fixture.service.UpdateGameServerEnvironment(context.Background(), ownerReq)
	if errOwner == nil {
		t.Fatal("UpdateGameServerEnvironment(non-superuser) error = nil, want permission denied")
	}
	if connect.CodeOf(errOwner) != connect.CodePermissionDenied {
		t.Fatalf("UpdateGameServerEnvironment(non-superuser) code = %v, want %v", connect.CodeOf(errOwner), connect.CodePermissionDenied)
	}

	adminReq := connect.NewRequest(&xylona.UpdateGameServerEnvironmentRequest{
		ServerId: "server-local-1",
		EnvVars: []*xylona.EnvironmentVariable{
			{Name: "VISIBLE_TOKEN", Value: "visible-value"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, adminReq, "user-admin")

	_, errAdmin := fixture.service.UpdateGameServerEnvironment(context.Background(), adminReq)
	if errAdmin != nil {
		t.Fatalf("UpdateGameServerEnvironment(superuser) error = %v", errAdmin)
	}
}

func TestGameServerEnvironmentRejectsSecretNormalConflict(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	updateReq := connect.NewRequest(&xylona.UpdateGameServerEnvironmentRequest{
		ServerId: "server-local-1",
		EnvVars: []*xylona.EnvironmentVariable{
			{Name: "TOKEN", Value: "visible-value"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")

	_, errUpdate := fixture.service.UpdateGameServerEnvironment(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerEnvironment() setup error = %v", errUpdate)
	}

	setReq := connect.NewRequest(&xylona.SetGameServerSecretEnvRequest{
		ServerId: "server-local-1",
		Name:     "token",
		Value:    "secret-value",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-admin")

	_, errSet := fixture.service.SetGameServerSecretEnv(context.Background(), setReq)
	if errSet == nil {
		t.Fatal("SetGameServerSecretEnv(conflict) error = nil, want invalid argument")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Fatalf("SetGameServerSecretEnv(conflict) code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestGameServerEnvironmentMergesGameDefaults(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	_, errSetup := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		"update game set default_env_vars = ? where id = ?",
		`[{"name":"DEFAULT_ONLY","value":"base"},{"name":"OVERRIDE_ME","value":"base"}]`,
		"minecraft",
	)
	if errSetup != nil {
		t.Fatalf("update game setup error = %v", errSetup)
	}

	updateReq := connect.NewRequest(&xylona.UpdateGameServerEnvironmentRequest{
		ServerId: "server-local-1",
		EnvVars: []*xylona.EnvironmentVariable{
			{Name: "OVERRIDE_ME", Value: "server"},
		},
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateReq, "user-admin")
	updateResp, errUpdate := fixture.service.UpdateGameServerEnvironment(context.Background(), updateReq)
	if errUpdate != nil {
		t.Fatalf("UpdateGameServerEnvironment() error = %v", errUpdate)
	}
	if len(updateResp.Msg.GetEffectiveEnv()) != 2 {
		t.Fatalf("UpdateGameServerEnvironment().EffectiveEnv length = %d, want 2", len(updateResp.Msg.GetEffectiveEnv()))
	}
	if updateResp.Msg.GetEffectiveEnv()[0].GetName() != "DEFAULT_ONLY" {
		t.Fatalf("EffectiveEnv[0].Name = %q, want DEFAULT_ONLY", updateResp.Msg.GetEffectiveEnv()[0].GetName())
	}
	if updateResp.Msg.GetEffectiveEnv()[1].GetValue() != "server" {
		t.Fatalf("EffectiveEnv[1].Value = %q, want server override", updateResp.Msg.GetEffectiveEnv()[1].GetValue())
	}

	getReq := connect.NewRequest(&xylona.GetGameServerEnvironmentRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, getReq, "user-admin")
	getResp, errGet := fixture.service.GetGameServerEnvironment(context.Background(), getReq)
	if errGet != nil {
		t.Fatalf("GetGameServerEnvironment() error = %v", errGet)
	}
	if len(getResp.Msg.GetGameDefaultEnv()) != 2 {
		t.Fatalf("GetGameServerEnvironment().GameDefaultEnv length = %d, want 2", len(getResp.Msg.GetGameDefaultEnv()))
	}
}

func TestGameServerEnvironmentRejectsSecretConflictWithGameDefault(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))
	_, errSetup := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		"update game set default_env_vars = ? where id = ?",
		`[{"name":"TOKEN","value":"visible"}]`,
		"minecraft",
	)
	if errSetup != nil {
		t.Fatalf("update game setup error = %v", errSetup)
	}

	setReq := connect.NewRequest(&xylona.SetGameServerSecretEnvRequest{
		ServerId: "server-local-1",
		Name:     "token",
		Value:    "secret-value",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-admin")

	_, errSet := fixture.service.SetGameServerSecretEnv(context.Background(), setReq)
	if errSet == nil {
		t.Fatal("SetGameServerSecretEnv(conflict with default) error = nil, want invalid argument")
	}
	if connect.CodeOf(errSet) != connect.CodeInvalidArgument {
		t.Fatalf("SetGameServerSecretEnv(conflict with default) code = %v, want %v", connect.CodeOf(errSet), connect.CodeInvalidArgument)
	}
}

func TestGameServerEnvironmentClearSecret(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	setReq := connect.NewRequest(&xylona.SetGameServerSecretEnvRequest{
		ServerId: "server-local-1",
		Name:     "SECRET_TOKEN",
		Value:    "secret-value",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setReq, "user-admin")

	_, errSet := fixture.service.SetGameServerSecretEnv(context.Background(), setReq)
	if errSet != nil {
		t.Fatalf("SetGameServerSecretEnv() setup error = %v", errSet)
	}

	clearReq := connect.NewRequest(&xylona.ClearGameServerSecretEnvRequest{
		ServerId: "server-local-1",
		Name:     "SECRET_TOKEN",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, clearReq, "user-admin")

	clearResp, errClear := fixture.service.ClearGameServerSecretEnv(context.Background(), clearReq)
	if errClear != nil {
		t.Fatalf("ClearGameServerSecretEnv() error = %v", errClear)
	}
	if len(clearResp.Msg.GetSecretEnv()) != 0 {
		t.Fatalf("ClearGameServerSecretEnv().SecretEnv length = %d, want 0", len(clearResp.Msg.GetSecretEnv()))
	}
}
