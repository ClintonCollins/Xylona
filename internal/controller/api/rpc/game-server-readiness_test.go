package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestSteamGSLTReadinessRPCStoresEncryptedSecret(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	fixture.conn.SetEncryptionKey([]byte("01234567890123456789012345678901"))

	_, errRequireSteam := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game set requires_steam_game_server_login_token = true where id = ?`,
		"minecraft",
	)
	if errRequireSteam != nil {
		t.Fatalf("failed to mark game as requiring Steam GSLT: %v", errRequireSteam)
	}

	setRequest := connect.NewRequest(&xylona.SetSteamGSLTRequest{
		ServerId: "server-local-1",
		Token:    "  steam-token  ",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, setRequest, "user-admin")

	setResponse, errSet := fixture.service.SetSteamGSLT(context.Background(), setRequest)
	if errSet != nil {
		t.Fatalf("SetSteamGSLT() error = %v", errSet)
	}
	setItem := findReadinessItem(t, setResponse.Msg.GetItems(), "steam_gslt")
	if !setItem.GetComplete() || setItem.GetBlocking() {
		t.Fatalf("SetSteamGSLT().Items steam_gslt = %+v, want complete non-blocking", setItem)
	}

	var encryptedValue string
	errScan := fixture.conn.SQLDb.QueryRowContext(
		context.Background(),
		`select value_encrypted
		 from game_server_secret
		 where game_server_id = ? and kind = ? and name = ?`,
		"server-local-1",
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	).Scan(&encryptedValue)
	if errScan != nil {
		t.Fatalf("query raw Steam GSLT secret error = %v", errScan)
	}
	if encryptedValue == "steam-token" {
		t.Fatal("Steam GSLT stored as plaintext, want encrypted ciphertext")
	}

	decrypted, ok, errDecrypt := fixture.conn.DecryptGameServerSecret(
		"server-local-1",
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	)
	if errDecrypt != nil {
		t.Fatalf("DecryptGameServerSecret(steam_gslt) error = %v", errDecrypt)
	}
	if !ok {
		t.Fatal("DecryptGameServerSecret(steam_gslt) ok = false, want true")
	}
	if decrypted != "steam-token" {
		t.Fatalf("DecryptGameServerSecret(steam_gslt) = %q, want %q", decrypted, "steam-token")
	}

	clearRequest := connect.NewRequest(&xylona.ClearSteamGSLTRequest{
		ServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, clearRequest, "user-admin")

	clearResponse, errClear := fixture.service.ClearSteamGSLT(context.Background(), clearRequest)
	if errClear != nil {
		t.Fatalf("ClearSteamGSLT() error = %v", errClear)
	}
	clearItem := findReadinessItem(t, clearResponse.Msg.GetItems(), "steam_gslt")
	if clearItem.GetComplete() || !clearItem.GetBlocking() {
		t.Fatalf("ClearSteamGSLT().Items steam_gslt = %+v, want incomplete blocking", clearItem)
	}
}

func findReadinessItem(t *testing.T, items []*xylona.GameServerReadinessItem, kind string) *xylona.GameServerReadinessItem {
	t.Helper()
	for _, item := range items {
		if item.GetKind() == kind {
			return item
		}
	}
	t.Fatalf("readiness item %q not found", kind)
	return nil
}
