// Package xycrypt provides AES-GCM helpers for stored secrets and Argon2id
// hashing for non-password secret tokens. User passwords use the dedicated
// passwordhash Argon2id helpers in the auth and user RPC paths.
package xycrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/ClintonCollins/Xylona/helpers"
)

// EncryptionKeySize is the required length for AES-256 keys.
const EncryptionKeySize = 32

// GenerateEncryptionKey generates a cryptographically random 256-bit key
// suitable for use with Encrypt and Decrypt.
func GenerateEncryptionKey() ([]byte, error) {
	return GenerateRandomBytes(EncryptionKeySize)
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

	nonce, errNonce := GenerateRandomBytes(helpers.ClampUint32FromInt(gcm.NonceSize()))
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

// DefaultHashParameters contains the default Argon2id settings used by Xylona.
var DefaultHashParameters = HashParameters{
	keyLength:       48,
	saltLength:      24,
	memoryBytes:     96 * 1024,
	iterations:      4,
	parallelization: 1,
}

var (
	// ErrInvalidHashFormat indicates a malformed encoded hash string.
	ErrInvalidHashFormat = errors.New("invalid hash format")
	// ErrInvalidHashVersion indicates the encoded hash uses an unsupported format version.
	ErrInvalidHashVersion = errors.New("invalid hash version")
)

// HashParameters defines the Argon2id settings used for non-password secret
// token hashing.
type HashParameters struct {
	keyLength       uint32
	saltLength      uint32
	memoryBytes     uint32
	iterations      uint32
	parallelization uint8
}

// GenerateRandomBytes returns cryptographically random bytes of the requested length.
func GenerateRandomBytes(length uint32) ([]byte, error) {
	b := make([]byte, length)
	_, errRead := rand.Read(b)
	if errRead != nil {
		return nil, fmt.Errorf("xycrypt: generate random bytes: %w", errRead)
	}
	return b, nil
}

// GenerateHashFromString hashes the input using Argon2id and returns the encoded form.
func GenerateHashFromString(input string, hashParameters HashParameters) ([]byte, error) {
	salt, errGetSalt := GenerateRandomBytes(hashParameters.saltLength)
	if errGetSalt != nil {
		return nil, errGetSalt
	}

	argon2Hash := argon2.IDKey([]byte(input), salt, hashParameters.iterations, hashParameters.memoryBytes, hashParameters.parallelization, hashParameters.keyLength)

	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(argon2Hash)

	encodedHash := fmt.Sprintf("$argon2id$%d$%d$%d$%d$%s$%s", argon2.Version, hashParameters.memoryBytes, hashParameters.iterations, hashParameters.parallelization, saltB64, hashB64)
	return []byte(encodedHash), nil
}

// CompareHashAndString verifies whether the input matches the encoded Argon2id hash.
func CompareHashAndString(encodedHash []byte, input string) (bool, error) {
	splitEncodedHash := strings.Split(string(encodedHash), "$")
	if len(splitEncodedHash) != 8 {
		return false, ErrInvalidHashFormat
	}
	// Remove empty first element...
	splitEncodedHash = splitEncodedHash[1:]

	hashAlgo := splitEncodedHash[0]
	if hashAlgo != "argon2id" {
		return false, ErrInvalidHashVersion
	}

	version, errVersion := strconv.Atoi(splitEncodedHash[1])
	if errVersion != nil {
		return false, ErrInvalidHashFormat
	}

	if version != argon2.Version {
		return false, ErrInvalidHashVersion
	}

	memoryBytes, errMemory := strconv.ParseUint(splitEncodedHash[2], 10, 32)
	if errMemory != nil {
		return false, ErrInvalidHashFormat
	}
	iterations, errIterations := strconv.ParseUint(splitEncodedHash[3], 10, 32)
	if errIterations != nil {
		return false, ErrInvalidHashFormat
	}
	parallelization, errParallelization := strconv.ParseUint(splitEncodedHash[4], 10, 8)
	if errParallelization != nil {
		return false, ErrInvalidHashFormat
	}
	salt, errSalt := base64.RawStdEncoding.Strict().DecodeString(splitEncodedHash[5])
	if errSalt != nil {
		return false, ErrInvalidHashFormat
	}
	originalHash, errHash := base64.RawStdEncoding.Strict().DecodeString(splitEncodedHash[6])
	if errHash != nil {
		return false, ErrInvalidHashFormat
	}

	newHash := argon2.IDKey(
		[]byte(input),
		salt,
		helpers.ClampUint32FromUint64(iterations),
		helpers.ClampUint32FromUint64(memoryBytes),
		uint8(parallelization),
		helpers.ClampUint32FromInt(len(originalHash)),
	)

	return subtle.ConstantTimeCompare(originalHash, newHash) == 1, nil
}
