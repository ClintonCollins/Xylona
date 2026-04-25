package updater

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"unicode"
)

var (
	// ErrChecksumNotFound is returned when checksums.txt has no entry for an asset.
	ErrChecksumNotFound = errors.New("updater: checksum not found")
	// ErrChecksumMismatch is returned when a downloaded artifact does not match.
	ErrChecksumMismatch = errors.New("updater: checksum mismatch")
	// ErrArtifactSizeMismatch is returned when a downloaded artifact has an unexpected size.
	ErrArtifactSizeMismatch = errors.New("updater: artifact size mismatch")
)

// ParseChecksums parses a GoReleaser-style checksums file.
func ParseChecksums(content string) map[string]string {
	out := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		sum := strings.ToLower(strings.TrimSpace(fields[0]))
		name := strings.TrimLeft(strings.TrimSpace(fields[1]), "*")
		if len(sum) != sha256.Size*2 {
			continue
		}
		out[name] = sum
	}
	return out
}

// SHA256Hex streams r and returns its SHA-256 digest and byte count.
func SHA256Hex(r io.Reader) (string, int64, error) {
	hasher := sha256.New()
	written, errCopy := io.Copy(hasher, r)
	if errCopy != nil {
		return "", written, fmt.Errorf("updater: hash stream: %w", errCopy)
	}
	return hex.EncodeToString(hasher.Sum(nil)), written, nil
}

// VerifySHA256 compares the expected SHA-256 hex digest against r.
func VerifySHA256(r io.Reader, expected string) (int64, error) {
	expected = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(expected, "sha256:")))
	got, written, errHash := SHA256Hex(r)
	if errHash != nil {
		return written, errHash
	}
	if expected == "" {
		return written, ErrChecksumNotFound
	}
	if got != expected {
		return written, fmt.Errorf("%w: got %s, want %s", ErrChecksumMismatch, got, expected)
	}
	return written, nil
}

// VerifyDetachedSignatureWithGPG verifies a detached signature using the
// system gpg binary. keyringPath and trustedFingerprint are trust boundaries:
// at least one is required, and a supplied keyring disables default keyrings.
func VerifyDetachedSignatureWithGPG(ctx context.Context, artifactPath string, signaturePath string, keyringPath string, trustedFingerprint string) error {
	args, errArgs := gpgVerifyArgs(artifactPath, signaturePath, keyringPath, trustedFingerprint)
	if errArgs != nil {
		return errArgs
	}
	cmd := exec.CommandContext(ctx, "gpg", args...)
	output, errRun := cmd.CombinedOutput()
	if errRun != nil {
		return fmt.Errorf("updater: verify checksum signature: %w: %s", errRun, strings.TrimSpace(string(output)))
	}
	if strings.TrimSpace(trustedFingerprint) != "" && !gpgOutputHasValidSignature(output, trustedFingerprint) {
		return errors.New("updater: checksum signature was not made by the trusted GPG fingerprint")
	}
	return nil
}

func gpgVerifyArgs(artifactPath string, signaturePath string, keyringPath string, trustedFingerprint string) ([]string, error) {
	keyringPath = strings.TrimSpace(keyringPath)
	trustedFingerprint = normalizeGPGFingerprint(trustedFingerprint)
	if keyringPath == "" && trustedFingerprint == "" {
		return nil, errors.New("updater: trusted GPG keyring or fingerprint is required")
	}

	args := []string{"--batch", "--status-fd", "1"}
	if keyringPath != "" {
		args = append(args, "--no-default-keyring", "--keyring", keyringPath)
	}
	args = append(args, "--verify", signaturePath, artifactPath)
	return args, nil
}

func gpgOutputHasValidSignature(output []byte, trustedFingerprint string) bool {
	trustedFingerprint = normalizeGPGFingerprint(trustedFingerprint)
	if trustedFingerprint == "" {
		return false
	}

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}
		if fields[0] == "[GNUPG:]" && fields[1] == "VALIDSIG" && normalizeGPGFingerprint(fields[2]) == trustedFingerprint {
			return true
		}
	}
	return false
}

func normalizeGPGFingerprint(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if unicode.IsSpace(r) {
			continue
		}
		builder.WriteRune(unicode.ToUpper(r))
	}
	return builder.String()
}
