package rpc

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/actions"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGameServerFilesCompressAllowsRemoteNodeByStagingAndUploading(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		ReadFileResult: []byte("remote-log-data"),
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	request := connect.NewRequest(&xylona.GameServerFilesCompressionRequest{
		GameServerId:            "server-remote-files",
		FullFilePaths:           []string{"logs/latest.log"},
		FullDestinationFilePath: "archives/latest-log",
		CompressionType:         xylona.GameServerFilesCompressionType_ZIP,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errCompress := fixture.service.GameServerFilesCompress(context.Background(), request)
	if errCompress != nil {
		t.Fatalf("GameServerFilesCompress() error = %v", errCompress)
	}
	if response.Msg.GetFullFilePath() != "archives/latest-log.zip" {
		t.Fatalf("GameServerFilesCompress().FullFilePath = %q, want %q", response.Msg.GetFullFilePath(), "archives/latest-log.zip")
	}
	if len(remoteClient.ReadFileCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(remoteClient.ReadFileCalls))
	}
	if remoteClient.ReadFileCalls[0].RelativePath != "logs/latest.log" {
		t.Fatalf("ReadFile relative path = %q, want %q", remoteClient.ReadFileCalls[0].RelativePath, "logs/latest.log")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	if remoteClient.WriteFileCalls[0].RelativePath != "archives/latest-log.zip" {
		t.Fatalf("WriteFile relative path = %q, want %q", remoteClient.WriteFileCalls[0].RelativePath, "archives/latest-log.zip")
	}

	archiveEntries := readZipEntries(t, remoteClient.WriteFileCalls[0].Content)
	if archiveEntries["latest.log"] != "remote-log-data" {
		t.Fatalf("archive latest.log contents = %q, want %q", archiveEntries["latest.log"], "remote-log-data")
	}
}

func TestGameServerFilesDecompressAllowsRemoteNodeByStagingAndUploading(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		ReadFileResult: buildZipArchive(t, map[string]string{"plugins/test.txt": "hello remote"}),
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	request := connect.NewRequest(&xylona.GameServerFilesDecompressionRequest{
		GameServerId:        "server-remote-files",
		FullFilePath:        "imports/bundle.zip",
		DestinationBasePath: "restored",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errDecompress := fixture.service.GameServerFilesDecompress(context.Background(), request)
	if errDecompress != nil {
		t.Fatalf("GameServerFilesDecompress() error = %v", errDecompress)
	}
	if len(response.Msg.GetFullFilePaths()) != 1 || response.Msg.GetFullFilePaths()[0] != "plugins/test.txt" {
		t.Fatalf("GameServerFilesDecompress().FullFilePaths = %+v, want [plugins/test.txt]", response.Msg.GetFullFilePaths())
	}
	if len(remoteClient.ReadFileCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(remoteClient.ReadFileCalls))
	}
	if remoteClient.ReadFileCalls[0].RelativePath != "imports/bundle.zip" {
		t.Fatalf("ReadFile relative path = %q, want %q", remoteClient.ReadFileCalls[0].RelativePath, "imports/bundle.zip")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	if remoteClient.WriteFileCalls[0].RelativePath != "restored/plugins/test.txt" {
		t.Fatalf("WriteFile relative path = %q, want %q", remoteClient.WriteFileCalls[0].RelativePath, "restored/plugins/test.txt")
	}
	if got := string(remoteClient.WriteFileCalls[0].Content); got != "hello remote" {
		t.Fatalf("WriteFile content = %q, want %q", got, "hello remote")
	}
}

func TestGameServerFilesDecompressToServerRootDoesNotReuploadSourceArchive(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:         "node-remote",
		ReadFileResult: buildZipArchive(t, map[string]string{"plugins/test.txt": "hello remote"}),
	}
	registry := testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.nodeRegistry = registry
	fixture.service.actionsInst = actions.NewInstance(
		context.Background(),
		fixture.conn,
		nil,
		registry,
		nil,
		versiontracker.NewVersionStateMap(),
		versiontracker.ResolverConfig{},
	)

	request := connect.NewRequest(&xylona.GameServerFilesDecompressionRequest{
		GameServerId: "server-remote-files",
		FullFilePath: "imports/bundle.zip",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errDecompress := fixture.service.GameServerFilesDecompress(context.Background(), request)
	if errDecompress != nil {
		t.Fatalf("GameServerFilesDecompress() error = %v", errDecompress)
	}
	if len(response.Msg.GetFullFilePaths()) != 1 || response.Msg.GetFullFilePaths()[0] != "plugins/test.txt" {
		t.Fatalf("GameServerFilesDecompress().FullFilePaths = %+v, want [plugins/test.txt]", response.Msg.GetFullFilePaths())
	}
	if len(remoteClient.ReadFileCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(remoteClient.ReadFileCalls))
	}
	if remoteClient.ReadFileCalls[0].RelativePath != "imports/bundle.zip" {
		t.Fatalf("ReadFile relative path = %q, want %q", remoteClient.ReadFileCalls[0].RelativePath, "imports/bundle.zip")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	if remoteClient.WriteFileCalls[0].RelativePath != "plugins/test.txt" {
		t.Fatalf("WriteFile relative path = %q, want %q", remoteClient.WriteFileCalls[0].RelativePath, "plugins/test.txt")
	}
	if got := string(remoteClient.WriteFileCalls[0].Content); got != "hello remote" {
		t.Fatalf("WriteFile content = %q, want %q", got, "hello remote")
	}
}

func buildZipArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range entries {
		writer, errCreate := archive.Create(name)
		if errCreate != nil {
			t.Fatalf("zip.Create(%s) error = %v", name, errCreate)
		}
		_, errWrite := writer.Write([]byte(content))
		if errWrite != nil {
			t.Fatalf("zip write %s error = %v", name, errWrite)
		}
	}
	errClose := archive.Close()
	if errClose != nil {
		t.Fatalf("zip.Close() error = %v", errClose)
	}
	return buffer.Bytes()
}

func readZipEntries(t *testing.T, archiveBytes []byte) map[string]string {
	t.Helper()

	reader, errReader := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
	if errReader != nil {
		t.Fatalf("zip.NewReader() error = %v", errReader)
	}

	entries := make(map[string]string, len(reader.File))
	for _, file := range reader.File {
		rc, errOpen := file.Open()
		if errOpen != nil {
			t.Fatalf("zip file open %s error = %v", file.Name, errOpen)
		}
		data, errRead := io.ReadAll(rc)
		errClose := rc.Close()
		if errRead != nil {
			t.Fatalf("zip read %s error = %v", file.Name, errRead)
		}
		if errClose != nil {
			t.Fatalf("zip close %s error = %v", file.Name, errClose)
		}
		entries[file.Name] = string(data)
	}
	return entries
}
