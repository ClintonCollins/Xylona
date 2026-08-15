// Package xycrypt provides AES-GCM helpers for stored secrets.
// User passwords use the dedicated passwordhash Argon2id helpers.
package xycrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// EncryptionKeySize is the required length for AES-256 keys.
const EncryptionKeySize = 32

// GenerateEncryptionKey generates a cryptographically random 256-bit key
// suitable for use with Encrypt and Decrypt.
func GenerateEncryptionKey() ([]byte, error) {
	return generateRandomBytes(EncryptionKeySize)
}

// Encrypt encrypts plaintext using AES-256-GCM with the given key.
// The key must be exactly EncryptionKeySize (32) bytes.
// Returns a base64-encoded string containing the nonce prepended to the ciphertext.
func Encrypt(key []byte, plaintext string) (string, error) {
	block, errBlock := aes.NewCipher(key)
	if errBlock != nil {
		return "", fmt.Errorf("xycrypt: create cipher: %w", errBlock)
	}

	gcm, errGCM := cipher.NewGCM(block)
	if errGCM != nil {
		return "", fmt.Errorf("xycrypt: create GCM: %w", errGCM)
	}

	nonce, errNonce := generateRandomBytes(gcm.NonceSize())
	if errNonce != nil {
		return "", fmt.Errorf("xycrypt: generate nonce: %w", errNonce)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.RawStdEncoding.EncodeToString(sealed), nil
}

// Decrypt decrypts a base64-encoded ciphertext produced by Encrypt.
// The key must be the same key used for encryption.
func Decrypt(key []byte, ciphertextB64 string) (string, error) {
	data, errDecode := base64.RawStdEncoding.DecodeString(ciphertextB64)
	if errDecode != nil {
		return "", fmt.Errorf("xycrypt: decode ciphertext: %w", errDecode)
	}

	block, errBlock := aes.NewCipher(key)
	if errBlock != nil {
		return "", fmt.Errorf("xycrypt: create cipher: %w", errBlock)
	}

	gcm, errGCM := cipher.NewGCM(block)
	if errGCM != nil {
		return "", fmt.Errorf("xycrypt: create GCM: %w", errGCM)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("xycrypt: ciphertext too short")
	}

	nonce := data[:nonceSize]
	ciphertext := data[nonceSize:]

	plaintext, errOpen := gcm.Open(nil, nonce, ciphertext, nil)
	if errOpen != nil {
		return "", fmt.Errorf("xycrypt: decrypt: %w", errOpen)
	}

	return string(plaintext), nil
}

func generateRandomBytes(length int) ([]byte, error) {
	b := make([]byte, length)
	_, errRead := rand.Read(b)
	if errRead != nil {
		return nil, fmt.Errorf("xycrypt: generate random bytes: %w", errRead)
	}
	return b, nil
}
