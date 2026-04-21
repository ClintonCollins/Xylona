package updater

import "testing"

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

func TestParseChecksums(t *testing.T) {
	t.Parallel()

	checksums := ParseChecksums("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa  xylona_linux_amd64\n")
	got := checksums["xylona_linux_amd64"]
	if got != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("checksum = %q", got)
	}
}

func TestGPGVerifyArgsRequiresPinnedTrust(t *testing.T) {
	t.Parallel()

	_, errArgs := gpgVerifyArgs("checksums.txt", "checksums.txt.sig", "", "")
	if errArgs == nil {
		t.Fatal("gpgVerifyArgs() error = nil, want trust requirement error")
	}
}

func TestGPGVerifyArgsDisablesDefaultKeyring(t *testing.T) {
	t.Parallel()

	args, errArgs := gpgVerifyArgs("checksums.txt", "checksums.txt.sig", "trusted.gpg", "")
	if errArgs != nil {
		t.Fatalf("gpgVerifyArgs() error = %v, want nil", errArgs)
	}

	want := []string{"--batch", "--status-fd", "1", "--no-default-keyring", "--keyring", "trusted.gpg", "--verify", "checksums.txt.sig", "checksums.txt"}
	if len(args) != len(want) {
		t.Fatalf("gpgVerifyArgs() = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("gpgVerifyArgs()[%d] = %q, want %q; args = %#v", i, args[i], want[i], args)
		}
	}
}

func TestGPGOutputHasValidSignature(t *testing.T) {
	t.Parallel()

	output := []byte("[GNUPG:] NEWSIG\n[GNUPG:] VALIDSIG ABCD1234EF 2026-04-21 0 4 0 1 10 00 ABCD1234EF\n")
	if !gpgOutputHasValidSignature(output, "abcd 1234 ef") {
		t.Fatal("gpgOutputHasValidSignature() = false, want true")
	}
	if gpgOutputHasValidSignature(output, "BADF00D") {
		t.Fatal("gpgOutputHasValidSignature() = true for wrong fingerprint, want false")
	}
}
