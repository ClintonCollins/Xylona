package xycrypt

import (
	"testing"
)

func TestGenerateEncryptionKey(t *testing.T) {
	key, errGen := GenerateEncryptionKey()
	if errGen != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGen)
	}
	if len(key) != EncryptionKeySize {
		t.Errorf("GenerateEncryptionKey() len = %d, want %d", len(key), EncryptionKeySize)
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	key, errGen := GenerateEncryptionKey()
	if errGen != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGen)
	}

	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "short string", plaintext: "sk-abc123"},
		{name: "empty string", plaintext: ""},
		{name: "long API key", plaintext: "svc_aBcDeFgHiJkLmNoPqRsTuVwXyZ0123456789abcdef"},
		{name: "special characters", plaintext: "key+with/special=chars&symbols!@#$%^"},
		{name: "unicode", plaintext: "api-key-with-unicode-\u00e9\u00e8\u00ea"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ciphertext, errEncrypt := Encrypt(key, tt.plaintext)
			if errEncrypt != nil {
				t.Fatalf("Encrypt() error = %v", errEncrypt)
			}

			if tt.plaintext != "" && ciphertext == tt.plaintext {
				t.Errorf("Encrypt() returned plaintext unchanged")
			}

			decrypted, errDecrypt := Decrypt(key, ciphertext)
			if errDecrypt != nil {
				t.Fatalf("Decrypt() error = %v", errDecrypt)
			}

			if decrypted != tt.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tt.plaintext)
			}
		})
	}
}

func TestEncryptProducesDifferentCiphertexts(t *testing.T) {
	key, errGen := GenerateEncryptionKey()
	if errGen != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGen)
	}

	ct1, errE1 := Encrypt(key, "same-input")
	if errE1 != nil {
		t.Fatalf("Encrypt(1) error = %v", errE1)
	}

	ct2, errE2 := Encrypt(key, "same-input")
	if errE2 != nil {
		t.Fatalf("Encrypt(2) error = %v", errE2)
	}

	if ct1 == ct2 {
		t.Errorf("Encrypt() produced identical ciphertexts for same input, want different (random nonce)")
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1, errGen1 := GenerateEncryptionKey()
	if errGen1 != nil {
		t.Fatalf("GenerateEncryptionKey(1) error = %v", errGen1)
	}

	key2, errGen2 := GenerateEncryptionKey()
	if errGen2 != nil {
		t.Fatalf("GenerateEncryptionKey(2) error = %v", errGen2)
	}

	ciphertext, errEncrypt := Encrypt(key1, "secret-api-key")
	if errEncrypt != nil {
		t.Fatalf("Encrypt() error = %v", errEncrypt)
	}

	_, errDecrypt := Decrypt(key2, ciphertext)
	if errDecrypt == nil {
		t.Errorf("Decrypt() with wrong key error = nil, want error")
	}
}

func TestDecryptInvalidInput(t *testing.T) {
	key, errGen := GenerateEncryptionKey()
	if errGen != nil {
		t.Fatalf("GenerateEncryptionKey() error = %v", errGen)
	}

	_, errDecrypt := Decrypt(key, "not-valid-base64-ciphertext!!!")
	if errDecrypt == nil {
		t.Errorf("Decrypt() with invalid input error = nil, want error")
	}
}

func TestEncryptWithInvalidKeySize(t *testing.T) {
	shortKey := []byte("too-short")
	_, errEncrypt := Encrypt(shortKey, "test")
	if errEncrypt == nil {
		t.Errorf("Encrypt() with invalid key size error = nil, want error")
	}
}
