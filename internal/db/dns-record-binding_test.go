package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestDNSRecordBindingPersistence(t *testing.T) {
	conn := newRBACMigratedConnection(t, "dns-record-binding.sqlite")
	seedRBACFixture(t, conn)
	insertDNSRecordBindingTestServer(t, conn, "server-local-2", 25566)

	t.Run("creates and updates one binding per server", func(t *testing.T) {
		errUpsert := conn.UpsertDNSRecordBinding("server-local-1", "play")
		if errUpsert != nil {
			t.Fatalf("UpsertDNSRecordBinding() error = %v", errUpsert)
		}

		binding, errGet := conn.GetDNSRecordBinding("server-local-1")
		if errGet != nil {
			t.Fatalf("GetDNSRecordBinding() error = %v", errGet)
		}
		if binding.GameServerID != "server-local-1" || binding.RelativeName != "play" || binding.Ownership != nil {
			t.Fatalf("GetDNSRecordBinding() = %+v", binding)
		}

		errUpdate := conn.UpsertDNSRecordBinding("server-local-1", "new-play")
		if errUpdate != nil {
			t.Fatalf("UpsertDNSRecordBinding(update) error = %v", errUpdate)
		}
		binding, errGet = conn.GetDNSRecordBinding("server-local-1")
		if errGet != nil {
			t.Fatalf("GetDNSRecordBinding(updated) error = %v", errGet)
		}
		if binding.RelativeName != "new-play" {
			t.Fatalf("updated relative name = %q, want new-play", binding.RelativeName)
		}

		count, errCount := conn.CountDNSRecordBindings()
		if errCount != nil {
			t.Fatalf("CountDNSRecordBindings() error = %v", errCount)
		}
		if count != 1 {
			t.Fatalf("CountDNSRecordBindings() = %d, want 1", count)
		}
	})

	t.Run("replaces preserves and clears ownership atomically", func(t *testing.T) {
		providerRecordID := "provider-record-1"
		ownership := DNSRecordOwnership{
			ProviderRecordID: &providerRecordID,
			FQDN:             "new-play.example.com",
			RecordType:       "A",
			Value:            "192.0.2.10",
			TTL:              300,
		}
		errReplace := conn.ReplaceDNSRecordBindingOwnership("server-local-1", ownership)
		if errReplace != nil {
			t.Fatalf("ReplaceDNSRecordBindingOwnership() error = %v", errReplace)
		}

		errRename := conn.UpsertDNSRecordBinding("server-local-1", "renamed")
		if errRename != nil {
			t.Fatalf("UpsertDNSRecordBinding(rename) error = %v", errRename)
		}
		binding, errGet := conn.GetDNSRecordBinding("server-local-1")
		if errGet != nil {
			t.Fatalf("GetDNSRecordBinding(owned) error = %v", errGet)
		}
		assertDNSRecordOwnership(t, binding.Ownership, ownership)

		errClear := conn.ClearDNSRecordBindingOwnership("server-local-1")
		if errClear != nil {
			t.Fatalf("ClearDNSRecordBindingOwnership() error = %v", errClear)
		}
		binding, errGet = conn.GetDNSRecordBinding("server-local-1")
		if errGet != nil {
			t.Fatalf("GetDNSRecordBinding(cleared) error = %v", errGet)
		}
		if binding.Ownership != nil {
			t.Fatalf("ownership after clear = %+v, want nil", binding.Ownership)
		}

		ownership.ProviderRecordID = nil
		ownership.RecordType = "AAAA"
		ownership.FQDN = "renamed.example.com"
		ownership.Value = "2001:db8::10"
		errReplace = conn.ReplaceDNSRecordBindingOwnership("server-local-1", ownership)
		if errReplace != nil {
			t.Fatalf("ReplaceDNSRecordBindingOwnership(no provider ID) error = %v", errReplace)
		}
		binding, errGet = conn.GetDNSRecordBinding("server-local-1")
		if errGet != nil {
			t.Fatalf("GetDNSRecordBinding(no provider ID) error = %v", errGet)
		}
		assertDNSRecordOwnership(t, binding.Ownership, ownership)
	})

	t.Run("rejects a target already owned by another binding", func(t *testing.T) {
		errUpsert := conn.UpsertDNSRecordBinding("server-local-2", "other")
		if errUpsert != nil {
			t.Fatalf("UpsertDNSRecordBinding(second server) error = %v", errUpsert)
		}
		conflicting := DNSRecordOwnership{
			FQDN:       "renamed.example.com",
			RecordType: "AAAA",
			Value:      "2001:db8::20",
			TTL:        300,
		}
		targetOwned, errTargetOwned := conn.DNSRecordBindingTargetOwned("server-local-2", conflicting.FQDN, conflicting.RecordType)
		if errTargetOwned != nil || !targetOwned {
			t.Fatalf("DNSRecordBindingTargetOwned() = %t, error = %v, want true", targetOwned, errTargetOwned)
		}
		ownTargetOwned, errOwnTargetOwned := conn.DNSRecordBindingTargetOwned("server-local-1", conflicting.FQDN, conflicting.RecordType)
		if errOwnTargetOwned != nil || ownTargetOwned {
			t.Fatalf("DNSRecordBindingTargetOwned(own target) = %t, error = %v, want false", ownTargetOwned, errOwnTargetOwned)
		}
		errReplace := conn.ReplaceDNSRecordBindingOwnership("server-local-2", conflicting)
		if !errors.Is(errReplace, ErrDNSRecordBindingTargetConflict) {
			t.Fatalf("ReplaceDNSRecordBindingOwnership(conflict) error = %v, want target conflict", errReplace)
		}

		binding, errGet := conn.GetDNSRecordBinding("server-local-2")
		if errGet != nil {
			t.Fatalf("GetDNSRecordBinding(after conflict) error = %v", errGet)
		}
		if binding.Ownership != nil {
			t.Fatalf("ownership after conflict = %+v, want nil", binding.Ownership)
		}
	})

	t.Run("removes local state", func(t *testing.T) {
		errRemove := conn.RemoveDNSRecordBinding("server-local-1")
		if errRemove != nil {
			t.Fatalf("RemoveDNSRecordBinding() error = %v", errRemove)
		}
		_, errGet := conn.GetDNSRecordBinding("server-local-1")
		if !errors.Is(errGet, sql.ErrNoRows) {
			t.Fatalf("GetDNSRecordBinding(removed) error = %v, want sql.ErrNoRows", errGet)
		}
	})

	t.Run("deleting a game server cascades its binding", func(t *testing.T) {
		_, errDelete := conn.SQLDb.ExecContext(t.Context(), `delete from game_server where id = ?`, "server-local-2")
		if errDelete != nil {
			t.Fatalf("delete game server error = %v", errDelete)
		}
		_, errGet := conn.GetDNSRecordBinding("server-local-2")
		if !errors.Is(errGet, sql.ErrNoRows) {
			t.Fatalf("GetDNSRecordBinding(after server delete) error = %v, want sql.ErrNoRows", errGet)
		}
		count, errCount := conn.CountDNSRecordBindings()
		if errCount != nil {
			t.Fatalf("CountDNSRecordBindings(after cascade) error = %v", errCount)
		}
		if count != 0 {
			t.Fatalf("CountDNSRecordBindings(after cascade) = %d, want 0", count)
		}
	})
}

func insertDNSRecordBindingTestServer(t *testing.T, conn *Connection, id string, port int) {
	t.Helper()
	_, errInsert := conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, "user-other", id, "minecraft", "OFFLINE", 10, 10, "world", "127.0.0.1", port, port,
		"/tmp/"+id, "node-local", "[]",
	)
	if errInsert != nil {
		t.Fatalf("insert DNS record binding test server: %v", errInsert)
	}
}

func assertDNSRecordOwnership(t *testing.T, got *DNSRecordOwnership, want DNSRecordOwnership) {
	t.Helper()
	if got == nil {
		t.Fatal("ownership = nil, want owned snapshot")
	}
	if (got.ProviderRecordID == nil) != (want.ProviderRecordID == nil) {
		t.Fatalf("provider record ID = %v, want %v", got.ProviderRecordID, want.ProviderRecordID)
	}
	if got.ProviderRecordID != nil && *got.ProviderRecordID != *want.ProviderRecordID {
		t.Fatalf("provider record ID = %q, want %q", *got.ProviderRecordID, *want.ProviderRecordID)
	}
	if got.FQDN != want.FQDN || got.RecordType != want.RecordType || got.Value != want.Value || got.TTL != want.TTL {
		t.Fatalf("ownership = %+v, want %+v", got, want)
	}
}
