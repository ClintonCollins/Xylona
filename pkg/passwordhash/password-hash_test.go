package passwordhash

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestHashAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()

	password := "CorrectHorseBatteryStaple123!"

	hash, errHash := Hash(password)
	if errHash != nil {
		t.Fatalf("Hash() error = %v", errHash)
	}

	if !strings.HasPrefix(hash, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("Hash() = %q, want Argon2id PHC prefix with configured params", hash)
	}

	match, errVerify := Verify(hash, password)
	if errVerify != nil {
		t.Fatalf("Verify(correct password) error = %v", errVerify)
	}
	if !match {
		t.Fatal("Verify(correct password) = false, want true")
	}

	wrongMatch, errWrong := Verify(hash, "wrong-password")
	if errWrong != nil {
		t.Fatalf("Verify(wrong password) error = %v", errWrong)
	}
	if wrongMatch {
		t.Fatal("Verify(wrong password) = true, want false")
	}
}

func TestHashUsesUniqueSalt(t *testing.T) {
	t.Parallel()

	password := "RepeatedPassword123!"

	firstHash, errFirstHash := Hash(password)
	if errFirstHash != nil {
		t.Fatalf("Hash(first) error = %v", errFirstHash)
	}

	secondHash, errSecondHash := Hash(password)
	if errSecondHash != nil {
		t.Fatalf("Hash(second) error = %v", errSecondHash)
	}

	if firstHash == secondHash {
		t.Fatal("Hash() returned identical hashes for the same password, want unique salts")
	}

	firstMatch, errFirstMatch := Verify(firstHash, password)
	if errFirstMatch != nil {
		t.Fatalf("Verify(first hash) error = %v", errFirstMatch)
	}
	if !firstMatch {
		t.Fatal("Verify(first hash) = false, want true")
	}

	secondMatch, errSecondMatch := Verify(secondHash, password)
	if errSecondMatch != nil {
		t.Fatalf("Verify(second hash) error = %v", errSecondMatch)
	}
	if !secondMatch {
		t.Fatal("Verify(second hash) = false, want true")
	}
}

func TestVerifyRejectsMalformedHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		hash string
	}{
		{
			name: "wrong algorithm",
			hash: "$bcrypt$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA",
		},
		{
			name: "missing fields",
			hash: "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA",
		},
		{
			name: "bad params",
			hash: "$argon2id$v=19$m=abc,t=3,p=4$c2FsdA$aGFzaA",
		},
		{
			name: "bad salt",
			hash: "$argon2id$v=19$m=65536,t=3,p=4$***$aGFzaA",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			match, errVerify := Verify(tt.hash, "password123")
			if errVerify == nil {
				t.Fatalf("Verify() error = nil, want malformed hash error for %q", tt.hash)
			}
			if match {
				t.Fatalf("Verify() = true, want false for malformed hash %q", tt.hash)
			}
		})
	}
}

func TestVerifyRejectsLegacyBcryptHashes(t *testing.T) {
	t.Parallel()

	bcryptHash, errBcrypt := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	if errBcrypt != nil {
		t.Fatalf("GenerateFromPassword() error = %v", errBcrypt)
	}

	match, errVerify := Verify(string(bcryptHash), "password123")
	if errVerify == nil {
		t.Fatalf("Verify() error = nil, want unsupported-format error for bcrypt hash")
	}
	if match {
		t.Fatal("Verify() = true, want false for bcrypt hash")
	}
}
