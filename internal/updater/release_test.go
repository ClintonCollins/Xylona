package updater

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sigstore/sigstore-go/pkg/fulcio/certificate"
	"github.com/sigstore/sigstore-go/pkg/verify"
)

func TestFindArtifact(t *testing.T) {
	t.Parallel()

	release := &Release{
		Version: "1.2.3",
		Assets: []Asset{
			{Name: "xylona_linux_amd64", BrowserDownloadURL: "https://example.test/xylona"},
			{Name: "xylona-node_linux_amd64", BrowserDownloadURL: "https://example.test/node"},
			{Name: "xylona_windows_amd64.exe", BrowserDownloadURL: "https://example.test/xylona.exe"},
		},
	}

	controller, okController := FindArtifact(release, ComponentController, "linux", "amd64")
	if !okController {
		t.Fatal("FindArtifact(controller linux/amd64) ok = false, want true")
	}
	if controller.Name != "xylona_linux_amd64" {
		t.Fatalf("controller artifact = %q, want xylona_linux_amd64", controller.Name)
	}

	node, okNode := FindArtifact(release, ComponentNode, "linux", "x86_64")
	if !okNode {
		t.Fatal("FindArtifact(node linux/x86_64) ok = false, want true")
	}
	if node.Name != "xylona-node_linux_amd64" {
		t.Fatalf("node artifact = %q, want xylona-node_linux_amd64", node.Name)
	}
}

func TestLatestReleaseReturnsHTTPStatusError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "missing release", http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := NewGitHubClient("owner", "repo")
	client.BaseURL = server.URL

	_, errLatest := client.LatestRelease(t.Context())
	if errLatest == nil {
		t.Fatal("LatestRelease() error = nil, want HTTP status error")
	}
	var statusErr *HTTPStatusError
	if !errors.As(errLatest, &statusErr) {
		t.Fatalf("LatestRelease() error = %T, want HTTPStatusError", errLatest)
	}
	if statusErr.StatusCode != http.StatusNotFound {
		t.Fatalf("LatestRelease() status = %d, want %d", statusErr.StatusCode, http.StatusNotFound)
	}
}

func TestParseChecksums(t *testing.T) {
	t.Parallel()

	checksums := ParseChecksums("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  xylona_linux_amd64\n")
	got := checksums["xylona_linux_amd64"]
	if got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("checksum = %q", got)
	}
}

func TestFindChecksumBundleAsset(t *testing.T) {
	t.Parallel()

	release := &Release{Assets: []Asset{
		{Name: "checksums.txt"},
		{Name: "checksums.txt.sig"},
		{Name: "checksums.txt.sigstore.json", BrowserDownloadURL: "https://example.test/bundle"},
	}}

	asset, ok := FindChecksumBundleAsset(release)
	if !ok {
		t.Fatal("FindChecksumBundleAsset() ok = false, want true")
	}
	if asset.BrowserDownloadURL != "https://example.test/bundle" {
		t.Fatalf("FindChecksumBundleAsset() URL = %q, want bundle URL", asset.BrowserDownloadURL)
	}
}

func TestSigstoreReleaseIdentity(t *testing.T) {
	t.Parallel()

	identity, errIdentity := verify.NewShortCertificateIdentity(sigstoreGitHubActionsIssuer, "", "", sigstoreReleaseIdentity)
	if errIdentity != nil {
		t.Fatalf("NewShortCertificateIdentity() error = %v", errIdentity)
	}

	tests := []struct {
		name    string
		issuer  string
		subject string
		wantErr bool
	}{
		{
			name:    "tag release workflow",
			issuer:  sigstoreGitHubActionsIssuer,
			subject: "https://github.com/ClintonCollins/Xylona/.github/workflows/release.yml@refs/tags/v1.2.3",
		},
		{
			name:    "branch workflow",
			issuer:  sigstoreGitHubActionsIssuer,
			subject: "https://github.com/ClintonCollins/Xylona/.github/workflows/release.yml@refs/heads/main",
			wantErr: true,
		},
		{
			name:    "different repository",
			issuer:  sigstoreGitHubActionsIssuer,
			subject: "https://github.com/example/Xylona/.github/workflows/release.yml@refs/tags/v1.2.3",
			wantErr: true,
		},
		{
			name:    "different issuer",
			issuer:  "https://example.test",
			subject: "https://github.com/ClintonCollins/Xylona/.github/workflows/release.yml@refs/tags/v1.2.3",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			errVerify := identity.Verify(certificate.Summary{
				SubjectAlternativeName: tt.subject,
				Extensions: certificate.Extensions{
					Issuer: tt.issuer,
				},
			})
			if (errVerify != nil) != tt.wantErr {
				t.Fatalf("identity.Verify() error = %v, wantErr %t", errVerify, tt.wantErr)
			}
		})
	}
}

func TestVerifySigstoreBundleRejectsMalformedBundle(t *testing.T) {
	t.Parallel()

	errVerify := VerifySigstoreBundle(t.Context(), []byte("checksums"), []byte("not a bundle"))
	if errVerify == nil {
		t.Fatal("VerifySigstoreBundle() error = nil, want malformed bundle error")
	}
	if !strings.Contains(errVerify.Error(), "parse Sigstore bundle") {
		t.Fatalf("VerifySigstoreBundle() error = %v, want bundle parsing error", errVerify)
	}
}
