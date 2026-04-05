package db

import (
	"database/sql"
	"errors"
	"testing"
)

func TestInsertNotificationChannel(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nc-insert.sqlite")
	seedRBACFixture(t, conn)

	channel, errInsert := conn.InsertNotificationChannel("user-owner", "My Discord", "discord", `{"webhook":"https://example.com/hook"}`, true)
	if errInsert != nil {
		t.Fatalf("InsertNotificationChannel() error = %v", errInsert)
	}
	if channel.ID == "" {
		t.Error("InsertNotificationChannel() returned empty ID")
	}
	if channel.UserID != "user-owner" {
		t.Errorf("InsertNotificationChannel().UserID = %q, want %q", channel.UserID, "user-owner")
	}
	if channel.Name != "My Discord" {
		t.Errorf("InsertNotificationChannel().Name = %q, want %q", channel.Name, "My Discord")
	}
	if channel.ChannelType != "discord" {
		t.Errorf("InsertNotificationChannel().ChannelType = %q, want %q", channel.ChannelType, "discord")
	}
	if channel.Config != `{"webhook":"https://example.com/hook"}` {
		t.Errorf("InsertNotificationChannel().Config = %q, want original plaintext", channel.Config)
	}
	if channel.Enabled != 1 {
		t.Errorf("InsertNotificationChannel().Enabled = %d, want 1", channel.Enabled)
	}
}

func TestGetNotificationChannelsByUserID(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nc-list.sqlite")
	seedRBACFixture(t, conn)

	_, errFirst := conn.InsertNotificationChannel("user-owner", "Channel A", "discord", `{"url":"a"}`, true)
	if errFirst != nil {
		t.Fatalf("InsertNotificationChannel(A) error = %v", errFirst)
	}
	_, errSecond := conn.InsertNotificationChannel("user-owner", "Channel B", "slack", `{"url":"b"}`, false)
	if errSecond != nil {
		t.Fatalf("InsertNotificationChannel(B) error = %v", errSecond)
	}
	_, errOther := conn.InsertNotificationChannel("user-other", "Channel C", "discord", `{"url":"c"}`, true)
	if errOther != nil {
		t.Fatalf("InsertNotificationChannel(C) error = %v", errOther)
	}

	channels, errGet := conn.GetNotificationChannelsByUserID("user-owner")
	if errGet != nil {
		t.Fatalf("GetNotificationChannelsByUserID() error = %v", errGet)
	}
	if len(channels) != 2 {
		t.Errorf("GetNotificationChannelsByUserID() len = %d, want 2", len(channels))
	}
}

func TestUpdateNotificationChannel(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nc-update.sqlite")
	seedRBACFixture(t, conn)

	channel, errInsert := conn.InsertNotificationChannel("user-owner", "Old Name", "discord", `{"old":true}`, true)
	if errInsert != nil {
		t.Fatalf("InsertNotificationChannel() error = %v", errInsert)
	}

	errUpdate := conn.UpdateNotificationChannel(channel.ID, "user-owner", "New Name", `{"new":true}`, false)
	if errUpdate != nil {
		t.Fatalf("UpdateNotificationChannel() error = %v", errUpdate)
	}

	updated, errGet := conn.GetNotificationChannelByID(channel.ID)
	if errGet != nil {
		t.Fatalf("GetNotificationChannelByID() after update error = %v", errGet)
	}
	if updated.Name != "New Name" {
		t.Errorf("UpdateNotificationChannel() Name = %q, want %q", updated.Name, "New Name")
	}
	if updated.Config != `{"new":true}` {
		t.Errorf("UpdateNotificationChannel() Config = %q, want updated value", updated.Config)
	}
	if updated.Enabled != 0 {
		t.Errorf("UpdateNotificationChannel() Enabled = %d, want 0", updated.Enabled)
	}
}

func TestDeleteNotificationChannel(t *testing.T) {
	conn := newRBACMigratedConnection(t, "nc-delete.sqlite")
	seedRBACFixture(t, conn)

	channel, errInsert := conn.InsertNotificationChannel("user-owner", "To Delete", "slack", `{}`, true)
	if errInsert != nil {
		t.Fatalf("InsertNotificationChannel() error = %v", errInsert)
	}

	errDelete := conn.DeleteNotificationChannel(channel.ID, "user-owner")
	if errDelete != nil {
		t.Fatalf("DeleteNotificationChannel() error = %v", errDelete)
	}

	_, errGet := conn.GetNotificationChannelByID(channel.ID)
	if !errors.Is(errGet, sql.ErrNoRows) {
		t.Errorf("GetNotificationChannelByID() after delete error = %v, want %v", errGet, sql.ErrNoRows)
	}
}

func TestNotificationChannel_ConfigEncryption(t *testing.T) {
	conn := newEncryptedConnection(t, "nc-encrypt.sqlite")
	seedRBACFixture(t, conn)

	plaintext := `{"webhook":"https://hooks.example.com/secret-token"}`

	channel, errInsert := conn.InsertNotificationChannel("user-owner", "Encrypted Channel", "discord", plaintext, true)
	if errInsert != nil {
		t.Fatalf("InsertNotificationChannel() error = %v", errInsert)
	}
	// Returned config must be the plaintext (decrypted for caller).
	if channel.Config != plaintext {
		t.Errorf("InsertNotificationChannel().Config = %q, want plaintext %q", channel.Config, plaintext)
	}

	// Read the raw stored value directly — it must NOT equal the plaintext.
	var storedConfig string
	errScan := conn.SQLDb.QueryRowContext(conn.ctx, `SELECT config FROM notification_channel WHERE id = ?`, channel.ID).Scan(&storedConfig)
	if errScan != nil {
		t.Fatalf("QueryRow() error = %v", errScan)
	}
	if storedConfig == plaintext {
		t.Errorf("Stored config matches plaintext — expected encrypted value to be stored")
	}

	// Fetch via GetNotificationChannelByID — must return the decrypted plaintext.
	fetched, errGet := conn.GetNotificationChannelByID(channel.ID)
	if errGet != nil {
		t.Fatalf("GetNotificationChannelByID() error = %v", errGet)
	}
	if fetched.Config != plaintext {
		t.Errorf("GetNotificationChannelByID().Config = %q, want plaintext %q", fetched.Config, plaintext)
	}

	// Update with new config and verify round-trip.
	newPlaintext := `{"webhook":"https://hooks.example.com/new-secret"}`
	errUpdate := conn.UpdateNotificationChannel(channel.ID, "user-owner", "Encrypted Channel", newPlaintext, true)
	if errUpdate != nil {
		t.Fatalf("UpdateNotificationChannel() error = %v", errUpdate)
	}

	updated, errGetUpdated := conn.GetNotificationChannelByID(channel.ID)
	if errGetUpdated != nil {
		t.Fatalf("GetNotificationChannelByID() after update error = %v", errGetUpdated)
	}
	if updated.Config != newPlaintext {
		t.Errorf("GetNotificationChannelByID() after update Config = %q, want %q", updated.Config, newPlaintext)
	}

	// Fetch via GetNotificationChannelsByUserID — all configs must be decrypted.
	all, errAll := conn.GetNotificationChannelsByUserID("user-owner")
	if errAll != nil {
		t.Fatalf("GetNotificationChannelsByUserID() error = %v", errAll)
	}
	if len(all) != 1 {
		t.Fatalf("GetNotificationChannelsByUserID() len = %d, want 1", len(all))
	}
	if all[0].Config != newPlaintext {
		t.Errorf("GetNotificationChannelsByUserID()[0].Config = %q, want plaintext %q", all[0].Config, newPlaintext)
	}
}
