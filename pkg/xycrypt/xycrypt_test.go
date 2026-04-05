package xycrypt

import (
	"bytes"
	"io"
	"os"
	"testing"
)

func TestCompareHashAndString(t *testing.T) {
	testHash1, errGenerate := GenerateHashFromString("SomeTestString", DefaultHashParameters)
	if errGenerate != nil {
		t.Errorf("Error generating hash: %v", errGenerate)
		return
	}
	testHash2, errGenerate := GenerateHashFromString("77%F!^(f@4babl0*3&7v8o7@lpy^2)#3^Nz+^oIgzp8)73Qltscw6nNG5QE$z05z", DefaultHashParameters)
	if errGenerate != nil {
		t.Errorf("Error generating hash: %v", errGenerate)
		return
	}
	type args struct {
		encodedHash []byte
		input       string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name: "match 1",
			args: args{
				encodedHash: testHash1,
				input:       "SomeTestString",
			},
			want:    true,
			wantErr: false,
		},
		{
			name: "match 2",
			args: args{
				encodedHash: testHash2,
				input:       "77%F!^(f@4babl0*3&7v8o7@lpy^2)#3^Nz+^oIgzp8)73Qltscw6nNG5QE$z05z",
			},
			want:    true,
			wantErr: false,
		},

		{
			name: "not match 1",
			args: args{
				encodedHash: testHash1,
				input:       "swercv#!@#sdfsdz",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "not match 2",
			args: args{
				encodedHash: testHash1,
				input:       "someteststring",
			},
			want:    false,
			wantErr: false,
		},
		{
			name: "invalid version",
			args: args{
				encodedHash: []byte("$argon2id$17$98304$4$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid algorithm",
			args: args{
				encodedHash: []byte("$bcrypt$17$98304$4$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid format",
			args: args{
				encodedHash: []byte("$bcrypt$17$98304$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
		{
			name: "invalid cost",
			args: args{
				encodedHash: []byte("$bcrypt$17$98304$3$1$WrTctqxkMVovOVMpkfUYVhZePgdyA9lI$KJOB2iZQMa0w0l+KpYSkvT0K8SoTrPz1mL2Yz0KPnjgt3RMhH2XLVKeDz5O/wbHp"),
				input:       "SomeTestString",
			},
			want:    false,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareHashAndString(tt.args.encodedHash, tt.args.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("CompareHashAndString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("CompareHashAndString() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCompareHashAndStringDoesNotWriteToStdout(t *testing.T) {
	testHash, errGenerate := GenerateHashFromString("SomeTestString", DefaultHashParameters)
	if errGenerate != nil {
		t.Fatalf("GenerateHashFromString() error = %v", errGenerate)
	}

	stdoutOutput := captureStdout(t, func() {
		_, _ = CompareHashAndString(testHash, "SomeTestString")
	})

	if stdoutOutput != "" {
		t.Fatalf("CompareHashAndString() wrote to stdout: %q", stdoutOutput)
	}
}

func captureStdout(t *testing.T, run func()) string {
	t.Helper()

	oldStdout := os.Stdout
	stdoutReader, stdoutWriter, errPipe := os.Pipe()
	if errPipe != nil {
		t.Fatalf("os.Pipe() error = %v", errPipe)
	}

	os.Stdout = stdoutWriter
	defer func() {
		os.Stdout = oldStdout
	}()

	outputChan := make(chan string, 1)
	go func() {
		var buffer bytes.Buffer
		_, _ = io.Copy(&buffer, stdoutReader)
		outputChan <- buffer.String()
	}()

	run()

	errCloseWriter := stdoutWriter.Close()
	if errCloseWriter != nil {
		t.Fatalf("stdoutWriter.Close() error = %v", errCloseWriter)
	}
	output := <-outputChan

	errCloseReader := stdoutReader.Close()
	if errCloseReader != nil {
		t.Fatalf("stdoutReader.Close() error = %v", errCloseReader)
	}

	return output
}

// --------------------------------------------------------------------------
// AES-GCM encrypt/decrypt
// --------------------------------------------------------------------------

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

			// Ciphertext must differ from plaintext (except possibly empty).
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
