package games

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestMinecraftUpdateReplacesServerJar(t *testing.T) {
	originalTransport := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.String() {
		case "https://launchermeta.mojang.com/mc/game/version_manifest.json":
			return jsonResponse(`{
				"latest": {"release": "1.21.5", "snapshot": "24w01a"},
				"versions": [
					{"id": "1.21.5", "type": "release", "url": "https://downloads.example.test/version/1.21.5.json"}
				]
			}`), nil
		case "https://downloads.example.test/version/1.21.5.json":
			return jsonResponse(`{
				"downloads": {
					"server": {"url": "https://downloads.example.test/server/1.21.5.jar"}
				}
			}`), nil
		case "https://downloads.example.test/server/1.21.5.jar":
			return binaryResponse([]byte("new server jar")), nil
		default:
			t.Fatalf("unexpected URL requested: %s", req.URL.String())
			return nil, errors.New("unexpected URL requested")
		}
	})
	t.Cleanup(func() {
		http.DefaultTransport = originalTransport
	})

	serverDir := t.TempDir()
	jarPath := filepath.Join(serverDir, "minecraft_server.jar")
	errWrite := os.WriteFile(jarPath, []byte("old server jar"), 0o600)
	if errWrite != nil {
		t.Fatalf("os.WriteFile(%q) error = %v", jarPath, errWrite)
	}

	gameServer := &models.GameServer{Directory: serverDir}

	minecraftGame := &Minecraft{}
	errUpdate := minecraftGame.Update(gameServer, io.Discard, io.Discard)
	if errUpdate != nil {
		t.Fatalf("Minecraft.Update() error = %v", errUpdate)
	}

	got, errRead := os.ReadFile(jarPath)
	if errRead != nil {
		t.Fatalf("os.ReadFile(%q) error = %v", jarPath, errRead)
	}
	if string(got) != "new server jar" {
		t.Fatalf("updated jar contents = %q, want %q", string(got), "new server jar")
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func binaryResponse(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
	}
}
