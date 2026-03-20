package db

import (
	"testing"
	"time"
)

func newPairingTokenTestDB(t *testing.T) *Connection {
	t.Helper()
	return newRBACMigratedConnection(t, "pairing-token.sqlite")
}

func TestGenerateAndValidatePairingToken(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("https://remote.example.com")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}
	if plaintext == "" {
		t.Fatalf("GeneratePairingToken() returned empty plaintext")
	}

	// Valid token should succeed.
	token, errValidate := conn.ValidateAndConsumePairingToken(plaintext)
	if errValidate != nil {
		t.Fatalf("ValidateAndConsumePairingToken() error = %v", errValidate)
	}
	if token.TargetURL != "https://remote.example.com" {
		t.Errorf("TargetURL = %q, want %q", token.TargetURL, "https://remote.example.com")
	}
	// The returned token reflects the state at query time (before the update),
	// so Used is false here. The important thing is the validation succeeded.
	if token.ID == "" {
		t.Errorf("token ID should not be empty")
	}
}

func TestPairingTokenReuse(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}

	// First use succeeds.
	_, errFirst := conn.ValidateAndConsumePairingToken(plaintext)
	if errFirst != nil {
		t.Fatalf("first ValidateAndConsumePairingToken() error = %v", errFirst)
	}

	// Second use should fail (already used).
	_, errSecond := conn.ValidateAndConsumePairingToken(plaintext)
	if errSecond == nil {
		t.Fatalf("second ValidateAndConsumePairingToken() error = nil, want error for reuse")
	}
}

func TestPairingTokenInvalid(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	_, errValidate := conn.ValidateAndConsumePairingToken("nonexistent-token")
	if errValidate == nil {
		t.Fatalf("ValidateAndConsumePairingToken(nonexistent) error = nil, want error")
	}
}

func TestPairingTokenExpired(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}

	// Manually expire the token.
	_, errExpire := conn.SQLDb.Exec(
		`update federation_pairing_token set expires_at = ? where token_hash = ?`,
		time.Now().Add(-1*time.Hour),
		hashPairingToken(plaintext),
	)
	if errExpire != nil {
		t.Fatalf("failed to expire token: %v", errExpire)
	}

	_, errValidate := conn.ValidateAndConsumePairingToken(plaintext)
	if errValidate == nil {
		t.Fatalf("ValidateAndConsumePairingToken(expired) error = nil, want error")
	}
}

func TestValidateAndConsumePairingTokenForTargetMatch(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("https://remote.example.com")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}

	_, errValidate := conn.ValidateAndConsumePairingTokenForTarget(plaintext, "https://remote.example.com/")
	if errValidate != nil {
		t.Fatalf("ValidateAndConsumePairingTokenForTarget() error = %v", errValidate)
	}
}

func TestValidateAndConsumePairingTokenForTargetMismatch(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("https://remote.example.com")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}

	_, errValidate := conn.ValidateAndConsumePairingTokenForTarget(plaintext, "https://different.example.com")
	if errValidate == nil {
		t.Fatalf("ValidateAndConsumePairingTokenForTarget() error = nil, want mismatch error")
	}
}

func TestValidateAndConsumePairingTokenForTargetRequiredWhenStored(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("https://remote.example.com")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}

	_, errValidate := conn.ValidateAndConsumePairingTokenForTarget(plaintext, "")
	if errValidate == nil {
		t.Fatalf("ValidateAndConsumePairingTokenForTarget() error = nil, want target required error")
	}
}

func TestValidateAndConsumePairingTokenForTargetOptionalWhenNotStored(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	plaintext, errGenerate := conn.GeneratePairingToken("")
	if errGenerate != nil {
		t.Fatalf("GeneratePairingToken() error = %v", errGenerate)
	}

	_, errValidate := conn.ValidateAndConsumePairingTokenForTarget(plaintext, "https://remote.example.com")
	if errValidate != nil {
		t.Fatalf("ValidateAndConsumePairingTokenForTarget() error = %v", errValidate)
	}
}

func TestCleanupExpiredPairingTokens(t *testing.T) {
	conn := newPairingTokenTestDB(t)

	// Generate two tokens.
	plaintext1, _ := conn.GeneratePairingToken("")
	_, _ = conn.GeneratePairingToken("")

	// Use the first one.
	_, errValidate := conn.ValidateAndConsumePairingToken(plaintext1)
	if errValidate != nil {
		t.Fatalf("ValidateAndConsumePairingToken() error = %v", errValidate)
	}

	// Expire the second one.
	_, errExpire := conn.SQLDb.Exec(
		`update federation_pairing_token set expires_at = ? where used = false`,
		time.Now().Add(-1*time.Hour),
	)
	if errExpire != nil {
		t.Fatalf("failed to expire token: %v", errExpire)
	}

	errCleanup := conn.CleanupExpiredPairingTokens()
	if errCleanup != nil {
		t.Fatalf("CleanupExpiredPairingTokens() error = %v", errCleanup)
	}

	// Both should be gone (one used, one expired).
	var count int
	errCount := conn.SQLDb.QueryRow(`select count(*) from federation_pairing_token`).Scan(&count)
	if errCount != nil {
		t.Fatalf("count query error = %v", errCount)
	}
	if count != 0 {
		t.Errorf("expected 0 tokens after cleanup, got %d", count)
	}
}
