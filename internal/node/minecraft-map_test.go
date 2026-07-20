package node

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMinecraftPlayerPositions(t *testing.T) {
	t.Parallel()
	positions := parseMinecraftPlayerPositions(
		"Steve has the following entity data: [12.5d, 64.0d, -2.25d]\nAlex has the following entity data: [-1.0d, 70.5d, 8.0d]",
		"Steve has the following entity data: \"minecraft:overworld\"\nAlex has the following entity data: \"minecraft:the_nether\"",
	)
	if len(positions) != 2 {
		t.Fatalf("parseMinecraftPlayerPositions() count = %d, want 2", len(positions))
	}
	if positions[0].name != "Steve" || positions[0].dimension != "minecraft:overworld" || positions[0].x != 12.5 || positions[0].y != 64 || positions[0].z != -2.25 {
		t.Errorf("Steve position = %+v", positions[0])
	}
	if positions[1].name != "Alex" || positions[1].dimension != "minecraft:the_nether" {
		t.Errorf("Alex position = %+v", positions[1])
	}
}

func TestBlueMapReleaseSelection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		version string
		want    string
	}{
		{name: "Minecraft 26 uses current BlueMap", version: "26.1", want: "5.20"},
		{name: "modern legacy world uses Java 21 BlueMap", version: "1.21.11", want: "5.16"},
		{name: "unknown defaults to widest Java compatibility", want: "5.16"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := selectBlueMapRelease(test.version).version; got != test.want {
				t.Fatalf("selectBlueMapRelease(%q) = %q, want %q", test.version, got, test.want)
			}
		})
	}
}

func TestWriteManagedBlueMapConfig(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	managedRoot := filepath.Join(root, ".xylona", "bluemap")
	worldRoot := filepath.Join(root, "world")
	errWorld := os.MkdirAll(worldRoot, 0o750)
	if errWorld != nil {
		t.Fatalf("create world: %v", errWorld)
	}
	errWrite := writeManagedBlueMapConfig(managedRoot, worldRoot)
	if errWrite != nil {
		t.Fatalf("writeManagedBlueMapConfig() error = %v", errWrite)
	}

	tests := []struct {
		name     string
		path     string
		contains []string
	}{
		{name: "core accepts resources and scans mods", path: "core.conf", contains: []string{"accept-download: true", "scan-for-mod-resources: true", "render-thread-count: 1"}},
		{name: "web app disables browser storage", path: "webapp.conf", contains: []string{"enabled: true", "use-cookies: false"}},
		{name: "overworld dimension", path: filepath.Join("maps", "overworld.conf"), contains: []string{`dimension: "minecraft:overworld"`, `name: "Overworld"`}},
		{name: "nether dimension", path: filepath.Join("maps", "nether.conf"), contains: []string{`dimension: "minecraft:the_nether"`}},
		{name: "end dimension", path: filepath.Join("maps", "end.conf"), contains: []string{`dimension: "minecraft:the_end"`}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content, errRead := os.ReadFile(filepath.Join(managedRoot, "config", test.path))
			if errRead != nil {
				t.Fatalf("read generated config: %v", errRead)
			}
			for _, expected := range test.contains {
				if !strings.Contains(string(content), expected) {
					t.Errorf("config %q does not contain %q", test.path, expected)
				}
			}
		})
	}
}

func TestGetMinecraftMapAssetBoundsPathsAndUsesManagedBlueMap(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	webRoot := filepath.Join(root, ".xylona", "bluemap", "web")
	errDirectory := os.MkdirAll(webRoot, 0o750)
	if errDirectory != nil {
		t.Fatalf("create BlueMap webroot: %v", errDirectory)
	}
	errIndex := os.WriteFile(filepath.Join(webRoot, "index.html"), []byte("<html>BlueMap</html>"), 0o600)
	if errIndex != nil {
		t.Fatalf("write BlueMap index: %v", errIndex)
	}
	existingRoot := filepath.Join(root, "bluemap", "web")
	errExistingDirectory := os.MkdirAll(existingRoot, 0o750)
	if errExistingDirectory != nil {
		t.Fatalf("create existing plugin webroot: %v", errExistingDirectory)
	}
	errExistingIndex := os.WriteFile(filepath.Join(existingRoot, "index.html"), []byte("<html>Untrusted plugin</html>"), 0o600)
	if errExistingIndex != nil {
		t.Fatalf("write existing plugin index: %v", errExistingIndex)
	}

	nodeInst := &Node{}
	asset, errAsset := nodeInst.GetMinecraftMapAsset(t.Context(), MinecraftMapAssetRequest{
		ProcessID: "server-1", WorkingDirectory: root,
	})
	if errAsset != nil {
		t.Fatalf("GetMinecraftMapAsset(index) error = %v", errAsset)
	}
	if string(asset.Content) != "<html>BlueMap</html>" || asset.ContentType != "text/html; charset=utf-8" || asset.CacheControl != "no-store" {
		t.Fatalf("index asset = %+v", asset)
	}

	_, errTraversal := nodeInst.GetMinecraftMapAsset(t.Context(), MinecraftMapAssetRequest{
		ProcessID: "server-1", WorkingDirectory: root, AssetPath: "../server.properties",
	})
	if errTraversal == nil {
		t.Fatal("GetMinecraftMapAsset(traversal) error = nil")
	}
}
