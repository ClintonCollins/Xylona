package firstsetup

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"sync"
)

const setupTokenBytes = 32

// Token is a single-use first-run setup token stored as a SHA-256 hash.
type Token struct {
	hash     [sha256.Size]byte
	mu       sync.Mutex
	consumed bool
}

// IssueToken creates a new setup token. The plaintext is returned once for
// logging and the URL; only the hash is retained.
func IssueToken() (string, *Token, error) {
	buf := make([]byte, setupTokenBytes)
	_, errRead := rand.Read(buf)
	if errRead != nil {
		return "", nil, fmt.Errorf("firstsetup: issue token: %w", errRead)
	}
	plaintext := base64.RawURLEncoding.EncodeToString(buf)
	return plaintext, &Token{hash: sha256.Sum256([]byte(plaintext))}, nil
}

// Valid reports whether plaintext matches and has not been consumed.
func (t *Token) Valid(plaintext string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumed {
		return false
	}
	return tokenHashMatches(t.hash, plaintext)
}

// Consume validates and spends the token. A wrong guess does not spend it.
func (t *Token) Consume(plaintext string) bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.consumed {
		return false
	}
	if !tokenHashMatches(t.hash, plaintext) {
		return false
	}
	t.consumed = true
	return true
}

func tokenHashMatches(stored [sha256.Size]byte, plaintext string) bool {
	if plaintext == "" {
		return false
	}
	sum := sha256.Sum256([]byte(plaintext))
	return subtle.ConstantTimeCompare(stored[:], sum[:]) == 1
}
