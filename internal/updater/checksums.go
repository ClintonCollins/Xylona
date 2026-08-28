package updater

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/sigstore/sigstore-go/pkg/bundle"
	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

const (
	sigstoreGitHubActionsIssuer = "https://token.actions.githubusercontent.com"
	sigstoreReleaseIdentity     = `^https://github\.com/ClintonCollins/Xylona/\.github/workflows/release\.yml@refs/tags/v[^/]*$`
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

// VerifySigstoreBundle verifies an artifact against a keyless Sigstore bundle
// issued to Xylona's tag-triggered GitHub release workflow.
func VerifySigstoreBundle(ctx context.Context, artifact []byte, bundleJSON []byte) error {
	signedBundle := &bundle.Bundle{}
	errBundle := signedBundle.UnmarshalJSON(bundleJSON)
	if errBundle != nil {
		return fmt.Errorf("updater: parse Sigstore bundle: %w", errBundle)
	}

	options := tuf.DefaultOptions().WithContext(ctx).WithDisableLocalCache()
	trustedRoot, errRoot := root.FetchTrustedRootWithOptions(options)
	if errRoot != nil {
		return fmt.Errorf("updater: fetch Sigstore trusted root: %w", errRoot)
	}

	identity, errIdentity := verify.NewShortCertificateIdentity(sigstoreGitHubActionsIssuer, "", "", sigstoreReleaseIdentity)
	if errIdentity != nil {
		return fmt.Errorf("updater: configure Sigstore release identity: %w", errIdentity)
	}
	verifier, errVerifier := verify.NewVerifier(
		trustedRoot,
		verify.WithSignedCertificateTimestamps(1),
		verify.WithObserverTimestamps(1),
		verify.WithTransparencyLog(1),
	)
	if errVerifier != nil {
		return fmt.Errorf("updater: configure Sigstore verifier: %w", errVerifier)
	}
	policy := verify.NewPolicy(
		verify.WithArtifact(bytes.NewReader(artifact)),
		verify.WithCertificateIdentity(identity),
	)
	_, errVerify := verifier.Verify(signedBundle, policy)
	if errVerify != nil {
		return fmt.Errorf("updater: verify Sigstore bundle: %w", errVerify)
	}
	return nil
}
