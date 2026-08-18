// Package passwordhash provides Argon2id helpers for user password storage.
package passwordhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
)

const (
	algorithmName = "argon2id"
	saltLength    = 16
	keyLength     = 32
	memoryKiB     = 64 * 1024
	iterations    = 3
	parallelism   = 4
)

var (
	// ErrInvalidHashFormat indicates a malformed encoded hash string.
	ErrInvalidHashFormat = errors.New("invalid hash format")
	// ErrUnsupportedHashAlgorithm indicates a non-Argon2id encoded hash.
	ErrUnsupportedHashAlgorithm = errors.New("unsupported hash algorithm")
	// ErrUnsupportedHashVersion indicates an Argon2 version this package does not support.
	ErrUnsupportedHashVersion = errors.New("unsupported hash version")

	dummyPasswordHash string
)

func init() {
	hashed, errHash := Hash("xylona-unknown-user-timing-pad")
	if errHash != nil {
		panic("passwordhash: initialize dummy hash: " + errHash.Error())
	}
	dummyPasswordHash = hashed
}

// VerifyDummy spends the same Argon2 work as Verify so unknown-user logins
// do not return faster than a password mismatch.
func VerifyDummy(password string) {
	_, _ = Verify(dummyPasswordHash, password)
}

// Hash hashes a password using Argon2id and returns a PHC-formatted string.
func Hash(password string) (string, error) {
	salt := make([]byte, saltLength)
	_, errRead := rand.Read(salt)
	if errRead != nil {
		return "", fmt.Errorf("passwordhash: generate salt: %w", errRead)
	}

	hashBytes := argon2.IDKey(
		[]byte(password),
		salt,
		iterations,
		memoryKiB,
		parallelism,
		keyLength,
	)

	encodedSalt := base64.RawStdEncoding.EncodeToString(salt)
	encodedHash := base64.RawStdEncoding.EncodeToString(hashBytes)

	return fmt.Sprintf(
		"$%s$v=%d$m=%d,t=%d,p=%d$%s$%s",
		algorithmName,
		argon2.Version,
		memoryKiB,
		iterations,
		parallelism,
		encodedSalt,
		encodedHash,
	), nil
}

// Verify verifies an Argon2id PHC-formatted hash against a plaintext password.
func Verify(encodedHash string, password string) (bool, error) {
	hashInfo, errParse := parseHash(encodedHash)
	if errParse != nil {
		return false, errParse
	}

	computedHash := argon2.IDKey(
		[]byte(password),
		hashInfo.salt,
		hashInfo.iterations,
		hashInfo.memoryKiB,
		hashInfo.parallelism,
		helpers.ClampUint32FromInt(len(hashInfo.hash)),
	)

	return subtle.ConstantTimeCompare(hashInfo.hash, computedHash) == 1, nil
}

type parsedHash struct {
	memoryKiB   uint32
	iterations  uint32
	parallelism uint8
	salt        []byte
	hash        []byte
}

func parseHash(encodedHash string) (*parsedHash, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" {
		return nil, ErrInvalidHashFormat
	}

	if parts[1] != algorithmName {
		return nil, ErrUnsupportedHashAlgorithm
	}

	versionPart := parts[2]
	if !strings.HasPrefix(versionPart, "v=") {
		return nil, ErrInvalidHashFormat
	}

	version, errVersion := strconv.Atoi(strings.TrimPrefix(versionPart, "v="))
	if errVersion != nil {
		return nil, ErrInvalidHashFormat
	}
	if version != argon2.Version {
		return nil, ErrUnsupportedHashVersion
	}

	memoryValue, iterationsValue, parallelismValue, errParams := parseParams(parts[3])
	if errParams != nil {
		return nil, errParams
	}

	salt, errSalt := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if errSalt != nil {
		return nil, ErrInvalidHashFormat
	}
	if len(salt) == 0 {
		return nil, ErrInvalidHashFormat
	}

	hashBytes, errHash := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if errHash != nil {
		return nil, ErrInvalidHashFormat
	}
	if len(hashBytes) == 0 {
		return nil, ErrInvalidHashFormat
	}

	return &parsedHash{
		memoryKiB:   memoryValue,
		iterations:  iterationsValue,
		parallelism: parallelismValue,
		salt:        salt,
		hash:        hashBytes,
	}, nil
}

func parseParams(params string) (uint32, uint32, uint8, error) {
	paramParts := strings.Split(params, ",")
	if len(paramParts) != 3 {
		return 0, 0, 0, ErrInvalidHashFormat
	}

	values := map[string]string{}
	for _, part := range paramParts {
		key, value, found := strings.Cut(part, "=")
		if !found || key == "" || value == "" {
			return 0, 0, 0, ErrInvalidHashFormat
		}
		values[key] = value
	}

	memoryParsed, errMemory := strconv.ParseUint(values["m"], 10, 32)
	if errMemory != nil || memoryParsed == 0 {
		return 0, 0, 0, ErrInvalidHashFormat
	}

	iterationsParsed, errIterations := strconv.ParseUint(values["t"], 10, 32)
	if errIterations != nil || iterationsParsed == 0 {
		return 0, 0, 0, ErrInvalidHashFormat
	}

	parallelismParsed, errParallelism := strconv.ParseUint(values["p"], 10, 8)
	if errParallelism != nil || parallelismParsed == 0 {
		return 0, 0, 0, ErrInvalidHashFormat
	}

	return uint32(memoryParsed), uint32(iterationsParsed), uint8(parallelismParsed), nil
}
