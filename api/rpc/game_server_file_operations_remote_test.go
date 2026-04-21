package rpc

import (
	"context"
	"testing"

	"connectrpc.com/connect"

	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func TestGameServerFilesCompressRoutesToRemoteNodeWithoutControllerStaging(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-remote",
		CreateFileArchiveResult: "archives/latest-log.zip",
		CreateFileArchiveProgress: node.ArchiveProgress{
			TotalFiles:      1,
			FilesCompressed: 1,
			TotalBytes:      10,
			BytesCompressed: 10,
			CurrentFile:     "latest.log",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GameServerFilesCompressionRequest{
		GameServerId:            "server-remote-files",
		FullFilePaths:           []string{"logs/latest.log"},
		FullDestinationFilePath: "archives/latest-log",
		CompressionType:         xylona.GameServerFilesCompressionType_ZIP,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errCompress := fixture.service.GameServerFilesCompress(context.Background(), request)
	if errCompress != nil {
		t.Fatalf("GameServerFilesCompress() error = %v, want nil", errCompress)
	}
	if response.Msg.GetFullFilePath() != "archives/latest-log.zip" {
		t.Fatalf("GameServerFilesCompress() path = %q, want %q", response.Msg.GetFullFilePath(), "archives/latest-log.zip")
	}
	if len(remoteClient.CreateFileArchiveCalls) != 1 {
		t.Fatalf("CreateFileArchive call count = %d, want 1", len(remoteClient.CreateFileArchiveCalls))
	}
	call := remoteClient.CreateFileArchiveCalls[0]
	if call.Directory != "/srv/remote-server" {
		t.Fatalf("CreateFileArchive directory = %q, want %q", call.Directory, "/srv/remote-server")
	}
	if call.DestinationArchivePath != "archives/latest-log" {
		t.Fatalf("CreateFileArchive destination = %q, want %q", call.DestinationArchivePath, "archives/latest-log")
	}
	if call.Compression != node.ArchiveCompressionZIP {
		t.Fatalf("CreateFileArchive compression = %v, want %v", call.Compression, node.ArchiveCompressionZIP)
	}
	if len(call.IncludePaths) != 1 || call.IncludePaths[0] != "logs/latest.log" {
		t.Fatalf("CreateFileArchive include paths = %v, want [logs/latest.log]", call.IncludePaths)
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
}

func TestGameServerFileArchiveOperationsRouteLocalNodeThroughNodeClient(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	selfClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-local",
		CreateFileArchiveResult: "archives/latest-log.zip",
		CreateFileArchiveProgress: node.ArchiveProgress{
			TotalFiles:      1,
			FilesCompressed: 1,
			TotalBytes:      10,
			BytesCompressed: 10,
			CurrentFile:     "latest.log",
		},
		ExtractFileArchiveResult: []string{"server.properties"},
		ExtractFileArchiveProgress: node.ExtractProgress{
			TotalFiles:     1,
			FilesExtracted: 1,
			TotalBytes:     42,
			BytesExtracted: 42,
			CurrentFile:    "server.properties",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(selfClient, nil)

	compressRequest := connect.NewRequest(&xylona.GameServerFilesCompressionRequest{
		GameServerId:            "server-local-1",
		FullFilePaths:           []string{"logs/latest.log"},
		FullDestinationFilePath: "archives/latest-log",
		CompressionType:         xylona.GameServerFilesCompressionType_ZIP,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, compressRequest, "user-owner")

	compressResponse, errCompress := fixture.service.GameServerFilesCompress(context.Background(), compressRequest)
	if errCompress != nil {
		t.Fatalf("GameServerFilesCompress() error = %v, want nil", errCompress)
	}
	if compressResponse.Msg.GetFullFilePath() != "archives/latest-log.zip" {
		t.Fatalf("GameServerFilesCompress() path = %q, want %q", compressResponse.Msg.GetFullFilePath(), "archives/latest-log.zip")
	}

	archiveRequest := connect.NewRequest(&xylona.GameServerFilesCompressionRequest{
		GameServerId:            "server-local-1",
		FullFilePaths:           []string{"logs/latest.log"},
		FullDestinationFilePath: "archives/latest-log",
		CompressionType:         xylona.GameServerFilesCompressionType_ZIP,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, archiveRequest, "user-owner")

	errArchive := fixture.service.GameServerFilesArchive(context.Background(), archiveRequest, nil)
	if errArchive != nil {
		t.Fatalf("GameServerFilesArchive() error = %v, want nil", errArchive)
	}

	decompressRequest := connect.NewRequest(&xylona.GameServerFilesDecompressionRequest{
		GameServerId:        "server-local-1",
		FullFilePath:        "imports/bundle.zip",
		DestinationBasePath: "restored",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, decompressRequest, "user-owner")

	decompressResponse, errDecompress := fixture.service.GameServerFilesDecompress(context.Background(), decompressRequest)
	if errDecompress != nil {
		t.Fatalf("GameServerFilesDecompress() error = %v, want nil", errDecompress)
	}
	if len(decompressResponse.Msg.GetFullFilePaths()) != 1 || decompressResponse.Msg.GetFullFilePaths()[0] != "server.properties" {
		t.Fatalf("GameServerFilesDecompress() paths = %v, want [server.properties]", decompressResponse.Msg.GetFullFilePaths())
	}

	extractRequest := connect.NewRequest(&xylona.GameServerFilesDecompressionRequest{
		GameServerId:        "server-local-1",
		FullFilePath:        "imports/bundle.zip",
		DestinationBasePath: "restored",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, extractRequest, "user-owner")

	errExtract := fixture.service.GameServerFilesExtract(context.Background(), extractRequest, nil)
	if errExtract != nil {
		t.Fatalf("GameServerFilesExtract() error = %v, want nil", errExtract)
	}

	if len(selfClient.CreateFileArchiveCalls) != 2 {
		t.Fatalf("CreateFileArchive call count = %d, want 2", len(selfClient.CreateFileArchiveCalls))
	}
	if len(selfClient.ExtractFileArchiveCalls) != 2 {
		t.Fatalf("ExtractFileArchive call count = %d, want 2", len(selfClient.ExtractFileArchiveCalls))
	}
	if len(selfClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(selfClient.ReadFileCalls))
	}
	if len(selfClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(selfClient.WriteFileCalls))
	}
}

func TestGameServerFilesCompressRejectsRemoteTraversal(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{NodeID: "node-remote"}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GameServerFilesCompressionRequest{
		GameServerId:            "server-remote-files",
		FullFilePaths:           []string{`..\latest.log`},
		FullDestinationFilePath: "archives/latest-log",
		CompressionType:         xylona.GameServerFilesCompressionType_ZIP,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errCompress := fixture.service.GameServerFilesCompress(context.Background(), request)
	if errCompress == nil {
		t.Fatal("GameServerFilesCompress() error = nil, want invalid path error")
	}
	if connect.CodeOf(errCompress) != connect.CodeInvalidArgument {
		t.Fatalf("GameServerFilesCompress() code = %v, want %v", connect.CodeOf(errCompress), connect.CodeInvalidArgument)
	}
	if len(remoteClient.CreateFileArchiveCalls) != 0 {
		t.Fatalf("CreateFileArchive call count = %d, want 0", len(remoteClient.CreateFileArchiveCalls))
	}
}

func TestGameServerFilesDecompressRoutesToRemoteNodeWithoutControllerStaging(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                   "node-remote",
		ExtractFileArchiveResult: []string{"server.properties"},
		ExtractFileArchiveProgress: node.ExtractProgress{
			TotalFiles:     1,
			FilesExtracted: 1,
			TotalBytes:     42,
			BytesExtracted: 42,
			CurrentFile:    "server.properties",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GameServerFilesDecompressionRequest{
		GameServerId:        "server-remote-files",
		FullFilePath:        "imports/bundle.zip",
		DestinationBasePath: "restored",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	response, errDecompress := fixture.service.GameServerFilesDecompress(context.Background(), request)
	if errDecompress != nil {
		t.Fatalf("GameServerFilesDecompress() error = %v, want nil", errDecompress)
	}
	if len(response.Msg.GetFullFilePaths()) != 1 || response.Msg.GetFullFilePaths()[0] != "server.properties" {
		t.Fatalf("GameServerFilesDecompress() paths = %v, want [server.properties]", response.Msg.GetFullFilePaths())
	}
	if len(remoteClient.ExtractFileArchiveCalls) != 1 {
		t.Fatalf("ExtractFileArchive call count = %d, want 1", len(remoteClient.ExtractFileArchiveCalls))
	}
	call := remoteClient.ExtractFileArchiveCalls[0]
	if call.Directory != "/srv/remote-server" {
		t.Fatalf("ExtractFileArchive directory = %q, want %q", call.Directory, "/srv/remote-server")
	}
	if call.ArchivePath != "imports/bundle.zip" {
		t.Fatalf("ExtractFileArchive archive path = %q, want %q", call.ArchivePath, "imports/bundle.zip")
	}
	if call.DestinationDirectoryPath != "restored" {
		t.Fatalf("ExtractFileArchive destination = %q, want %q", call.DestinationDirectoryPath, "restored")
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
}

func TestGameServerFilesArchiveRoutesToRemoteNodeWithoutControllerStaging(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                  "node-remote",
		CreateFileArchiveResult: "archives/latest-log.zip",
		CreateFileArchiveProgress: node.ArchiveProgress{
			TotalFiles:      1,
			FilesCompressed: 1,
			TotalBytes:      10,
			BytesCompressed: 10,
			CurrentFile:     "latest.log",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GameServerFilesCompressionRequest{
		GameServerId:            "server-remote-files",
		FullFilePaths:           []string{"logs/latest.log"},
		FullDestinationFilePath: "archives/latest-log",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	errArchive := fixture.service.GameServerFilesArchive(context.Background(), request, nil)
	if errArchive != nil {
		t.Fatalf("GameServerFilesArchive() error = %v, want nil", errArchive)
	}
	if len(remoteClient.CreateFileArchiveCalls) != 1 {
		t.Fatalf("CreateFileArchive call count = %d, want 1", len(remoteClient.CreateFileArchiveCalls))
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
}

func TestGameServerFilesExtractRoutesToRemoteNodeWithoutControllerStaging(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-files")

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                   "node-remote",
		ExtractFileArchiveResult: []string{"server.properties"},
		ExtractFileArchiveProgress: node.ExtractProgress{
			TotalFiles:     1,
			FilesExtracted: 1,
			TotalBytes:     42,
			BytesExtracted: 42,
			CurrentFile:    "server.properties",
		},
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)

	request := connect.NewRequest(&xylona.GameServerFilesDecompressionRequest{
		GameServerId:        "server-remote-files",
		FullFilePath:        "imports/bundle.zip",
		DestinationBasePath: "restored",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	errExtract := fixture.service.GameServerFilesExtract(context.Background(), request, nil)
	if errExtract != nil {
		t.Fatalf("GameServerFilesExtract() error = %v, want nil", errExtract)
	}
	if len(remoteClient.ExtractFileArchiveCalls) != 1 {
		t.Fatalf("ExtractFileArchive call count = %d, want 1", len(remoteClient.ExtractFileArchiveCalls))
	}
	if len(remoteClient.ReadFileCalls) != 0 {
		t.Fatalf("ReadFile call count = %d, want 0", len(remoteClient.ReadFileCalls))
	}
}

func TestSanitizeRemoteFileActionPathIsSlashNeutral(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		want      string
		wantError bool
	}{
		{
			name:  "normalizes windows separators",
			input: `configs\server.properties`,
			want:  "configs/server.properties",
		},
		{
			name:  "trims a browser-leading slash",
			input: `/configs\server.properties`,
			want:  "configs/server.properties",
		},
		{
			name:      "rejects parent traversal",
			input:     `..\server.properties`,
			wantError: true,
		},
		{
			name:      "rejects windows drive absolute path",
			input:     `C:\servers\server.properties`,
			wantError: true,
		},
		{
			name:      "rejects unc path",
			input:     `\\server\share\server.properties`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, errSanitize := sanitizeRemoteFileActionPath(tt.input)
			if tt.wantError {
				if errSanitize == nil {
					t.Fatalf("sanitizeRemoteFileActionPath(%q) error = nil, want error", tt.input)
				}
				return
			}
			if errSanitize != nil {
				t.Fatalf("sanitizeRemoteFileActionPath(%q) error = %v", tt.input, errSanitize)
			}
			if got != tt.want {
				t.Fatalf("sanitizeRemoteFileActionPath(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
