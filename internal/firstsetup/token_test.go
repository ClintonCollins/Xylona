package firstsetup

import (
	"testing"
)

func TestToken(t *testing.T) {
	t.Parallel()

	t.Run("issued token verifies and consumes once", func(t *testing.T) {
		t.Parallel()

		plaintext, token, errIssue := IssueToken()
		if errIssue != nil {
			t.Fatalf("IssueToken() error = %v", errIssue)
		}
		if plaintext == "" {
			t.Fatal("IssueToken() plaintext is empty")
		}
		if !token.Valid(plaintext) {
			t.Fatal("Valid() = false, want true for issued token")
		}
		if token.Valid("not-the-token") {
			t.Fatal("Valid() = true, want false for a different token")
		}
		if token.Valid("") {
			t.Fatal("Valid() = true, want false for empty token")
		}
		if !token.Consume(plaintext) {
			t.Fatal("Consume() = false, want true for issued token")
		}
		if token.Consume(plaintext) {
			t.Fatal("Consume() = true, want false after the token is used")
		}
		if token.Valid(plaintext) {
			t.Fatal("Valid() = true, want false after the token is consumed")
		}
	})

	t.Run("wrong token does not consume the issued token", func(t *testing.T) {
		t.Parallel()

		plaintext, token, errIssue := IssueToken()
		if errIssue != nil {
			t.Fatalf("IssueToken() error = %v", errIssue)
		}
		if token.Consume("wrong") {
			t.Fatal("Consume() = true, want false for a wrong token")
		}
		if !token.Consume(plaintext) {
			t.Fatal("Consume() = false, want true after a wrong guess")
		}
	})
}
