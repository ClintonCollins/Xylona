package xycrypt

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

var DefaultHashParameters = HashParameters{
	keyLength:       48,
	saltLength:      24,
	memoryBytes:     96 * 1024,
	iterations:      4,
	parallelization: 1,
}

var (
	ErrInvalidHashFormat = errors.New("invalid hash format")
	ErrInvalidHashVersion  = errors.New("invalid hash version")
)

type HashParameters struct {
	keyLength       uint32
	saltLength      uint32
	memoryBytes     uint32
	iterations      uint32
	parallelization uint8
}

func GenerateRandomBytes(length uint32) ([]byte, error) {
	b := make([]byte, length)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

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

func CompareHashAndString(encodedHash []byte, input string) (bool, error) {
	fmt.Printf("%s", string(encodedHash))
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

	newHash := argon2.IDKey([]byte(input), salt, uint32(iterations), uint32(memoryBytes), uint8(parallelization), uint32(len(originalHash)))

	return subtle.ConstantTimeCompare(originalHash, newHash) == 1, nil
}
