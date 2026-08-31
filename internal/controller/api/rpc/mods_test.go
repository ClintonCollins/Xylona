package rpc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/aarondl/opt/omit"

	"github.com/ClintonCollins/Xylona/internal/modmanager"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type rpcMockModProvider struct {
	id      string
	details *modproviders.ModDetails
}

func (p *rpcMockModProvider) ID() string {
	return p.id
}

func (p *rpcMockModProvider) Search(_ context.Context, _ string, _ modproviders.SearchParams) (modproviders.SearchResult, error) {
	return modproviders.SearchResult{}, nil
}

func (p *rpcMockModProvider) GetModDetails(_ context.Context, _ string, _ modproviders.SearchParams) (*modproviders.ModDetails, error) {
	return p.details, nil
}

func (p *rpcMockModProvider) GetVersions(_ context.Context, _ string, _ string, _ modproviders.SearchParams) ([]modproviders.ModVersion, error) {
	return p.details.Versions, nil
}

func (p *rpcMockModProvider) Download(_ context.Context, _ string, _ string, _ string) ([]modproviders.DownloadedFile, error) {
	return nil, errors.New("controller-side provider download should not be called for remote mod RPCs")
}

func (p *rpcMockModProvider) CheckForUpdate(_ context.Context, _ string, _ string) (*modproviders.ModVersion, error) {
	return nil, modproviders.ErrNoUpdateAvailable
}

var rpcModProviderCounter atomic.Int64

func newRPCModProvider() *rpcMockModProvider {
	n := rpcModProviderCounter.Add(1)
	providerID := fmt.Sprintf("rpc-mod-provider-%d", n)
	return &rpcMockModProvider{
		id: providerID,
		details: &modproviders.ModDetails{
			Source:   providerID,
			SourceID: "remote-mod",
			Name:     "Remote Mod",
			Author:   "Remote Author",
			Versions: []modproviders.ModVersion{
				{
					VersionID:      "v1",
					VersionString:  "1.0.0",
					DownloadURL:    "https://example.test/remote-mod-1.0.0.jar",
					FileSize:       1024,
					FileHashSHA256: "remote-mod-v1-sha",
				},
				{
					VersionID:      "v2",
					VersionString:  "2.0.0",
					DownloadURL:    "https://example.test/remote-mod-2.0.0.jar",
					FileSize:       2048,
					FileHashSHA256: "remote-mod-v2-sha",
				},
			},
		},
	}
}

func TestClampSearchTotalCount(t *testing.T) {
	tests := []struct {
		name      string
		totalHits int
		want      int32
	}{
		{name: "unknown total preserves sentinel", totalHits: modproviders.UnknownTotalHits, want: modproviders.UnknownTotalHits},
		{name: "other negative clamps to zero", totalHits: -2, want: 0},
		{name: "zero stays zero", totalHits: 0, want: 0},
		{name: "small value passes through", totalHits: 42, want: 42},
		{name: "max int32 passes through", totalHits: math.MaxInt32, want: math.MaxInt32},
		{name: "overflow clamps", totalHits: math.MaxInt32 + 1, want: math.MaxInt32},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clampSearchTotalCount(tc.totalHits)
			if got != tc.want {
				t.Fatalf("clampSearchTotalCount(%d) = %d, want %d", tc.totalHits, got, tc.want)
			}
		})
	}
}

func TestRemoteModRPCsUseNodeFileOperations(t *testing.T) {
	fixture := newRBACRPCFixture(t)
	insertRemoteNodeForParityTests(t, fixture, "node-remote")
	insertRemoteServerForParityTests(t, fixture, "server-remote-mods")

	provider := newRPCModProvider()
	modproviders.RegisterProvider(provider)
	_, errConfig := fixture.conn.SQLDb.ExecContext(
		context.Background(),
		`update game set server_software = ? where id = ?`,
		fmt.Sprintf(`{"update_provider":{"kind":"none"},"mod_profile":{"install_path":"mods","sources":[{"id":%q}]}}`, provider.ID()),
		"minecraft",
	)
	if errConfig != nil {
		t.Fatalf("update minecraft mod profile: %v", errConfig)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:                    "node-remote",
		DownloadFileFromURLResult: node.DownloadFileResult{RelativePath: "mods/.xylona-download-install/remote-mod-1.0.0.jar", BytesWritten: 1024, SHA256: "remote-mod-v1-sha"},
		RenameFileResult:          "mods/remote-mod-1.0.0.jar",
	}
	fixture.service.nodeRegistry = testParityRegistry(&nodeclient.FakeNodeClient{NodeID: "node-local"}, remoteClient)
	fixture.service.modManager = modmanager.New(fixture.conn)

	installRequest := connect.NewRequest(&xylona.InstallModRequest{
		GameServerId: "server-remote-mods",
		Source:       provider.ID(),
		SourceId:     "remote-mod",
		VersionId:    "v1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, installRequest, "user-owner")

	installResponse, errInstall := fixture.service.InstallMod(context.Background(), installRequest)
	if errInstall != nil {
		t.Fatalf("InstallMod() error = %v", errInstall)
	}
	installedID := installResponse.Msg.GetInstalledMod().GetId()
	if len(remoteClient.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("InstallMod() DownloadFileFromURL calls = %d, want 1", len(remoteClient.DownloadFileFromURLCalls))
	}
	if remoteClient.DownloadFileFromURLCalls[0].RawURL != "https://example.test/remote-mod-1.0.0.jar" {
		t.Fatalf("InstallMod() remote URL = %q", remoteClient.DownloadFileFromURLCalls[0].RawURL)
	}
	if remoteClient.DownloadFileFromURLCalls[0].Integrity.ExpectedSize != 1024 ||
		remoteClient.DownloadFileFromURLCalls[0].Integrity.ExpectedSHA256 != "remote-mod-v1-sha" {
		t.Fatalf("InstallMod() remote integrity = %+v", remoteClient.DownloadFileFromURLCalls[0].Integrity)
	}

	remoteClient.MoveFilesCalls = nil
	disableRequest := connect.NewRequest(&xylona.SetModEnabledRequest{
		GameServerId:   "server-remote-mods",
		InstalledModId: installedID,
		Enabled:        false,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, disableRequest, "user-owner")
	_, errDisable := fixture.service.SetModEnabled(context.Background(), disableRequest)
	if errDisable != nil {
		t.Fatalf("SetModEnabled(false) error = %v", errDisable)
	}
	if len(remoteClient.MoveFilesCalls) != 1 || remoteClient.MoveFilesCalls[0].Destination != "mods/disabled" {
		t.Fatalf("SetModEnabled(false) MoveFiles calls = %+v", remoteClient.MoveFilesCalls)
	}

	remoteClient.MoveFilesCalls = nil
	enableRequest := connect.NewRequest(&xylona.SetModEnabledRequest{
		GameServerId:   "server-remote-mods",
		InstalledModId: installedID,
		Enabled:        true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, enableRequest, "user-owner")
	_, errEnable := fixture.service.SetModEnabled(context.Background(), enableRequest)
	if errEnable != nil {
		t.Fatalf("SetModEnabled(true) error = %v", errEnable)
	}
	if len(remoteClient.MoveFilesCalls) != 1 || remoteClient.MoveFilesCalls[0].Destination != "mods" {
		t.Fatalf("SetModEnabled(true) MoveFiles calls = %+v", remoteClient.MoveFilesCalls)
	}

	remoteClient.DownloadFileFromURLCalls = nil
	remoteClient.RenameFileCalls = nil
	remoteClient.DownloadFileFromURLResult = node.DownloadFileResult{RelativePath: "mods/.xylona-download-update/remote-mod-2.0.0.jar", BytesWritten: 2048, SHA256: "remote-mod-v2-sha"}
	updateRequest := connect.NewRequest(&xylona.UpdateModRequest{
		GameServerId:   "server-remote-mods",
		InstalledModId: installedID,
		VersionId:      "v2",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, updateRequest, "user-owner")
	_, errUpdate := fixture.service.UpdateMod(context.Background(), updateRequest)
	if errUpdate != nil {
		t.Fatalf("UpdateMod() error = %v", errUpdate)
	}
	if len(remoteClient.DownloadFileFromURLCalls) != 1 {
		t.Fatalf("UpdateMod() DownloadFileFromURL calls = %d, want 1", len(remoteClient.DownloadFileFromURLCalls))
	}
	if remoteClient.DownloadFileFromURLCalls[0].RawURL != "https://example.test/remote-mod-2.0.0.jar" {
		t.Fatalf("UpdateMod() remote URL = %q", remoteClient.DownloadFileFromURLCalls[0].RawURL)
	}
	if remoteClient.DownloadFileFromURLCalls[0].Integrity.ExpectedSize != 2048 ||
		remoteClient.DownloadFileFromURLCalls[0].Integrity.ExpectedSHA256 != "remote-mod-v2-sha" {
		t.Fatalf("UpdateMod() remote integrity = %+v", remoteClient.DownloadFileFromURLCalls[0].Integrity)
	}

	remoteClient.DeleteFilesCalls = nil
	uninstallRequest := connect.NewRequest(&xylona.UninstallModRequest{
		GameServerId:   "server-remote-mods",
		InstalledModId: installedID,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, uninstallRequest, "user-owner")
	_, errUninstall := fixture.service.UninstallMod(context.Background(), uninstallRequest)
	if errUninstall != nil {
		t.Fatalf("UninstallMod() error = %v", errUninstall)
	}
	if len(remoteClient.DeleteFilesCalls) != 1 {
		t.Fatalf("UninstallMod() DeleteFiles calls = %d, want 1", len(remoteClient.DeleteFilesCalls))
	}
	if remoteClient.DeleteFilesCalls[0].Files[0] != "mods/remote-mod-2.0.0.jar" {
		t.Fatalf("UninstallMod() DeleteFiles files = %v", remoteClient.DeleteFilesCalls[0].Files)
	}
}

func TestSearchModsRequiresServerPermission(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	req := connect.NewRequest(&xylona.SearchModsRequest{
		GameServerId: "server-local-1",
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, req, "user-other")

	_, errSearch := fixture.service.SearchMods(t.Context(), req)
	if errSearch == nil {
		t.Fatal("SearchMods() error = nil, want permission error")
	}
	if connect.CodeOf(errSearch) != connect.CodePermissionDenied {
		t.Fatalf("SearchMods() code = %v, want %v", connect.CodeOf(errSearch), connect.CodePermissionDenied)
	}
}

func TestSetModAutoUpdateRejectsModFromAnotherServer(t *testing.T) {
	fixture := newRBACRPCFixture(t)

	now := time.Now().UTC()
	_, errServer := fixture.conn.SQLDb.ExecContext(
		t.Context(),
		`insert into game_server
		 (id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches)
		 values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-2", "user-other", "Local Two", "minecraft", "OFFLINE",
		10, 10, "world", "127.0.0.1", 25566, 25566, "/tmp/server-local-2", "node-local", "[]",
	)
	if errServer != nil {
		t.Fatalf("insert second game server error = %v", errServer)
	}

	_, errInsert := fixture.conn.InsertInstalledMod(fixture.conn.DB, &models.InstalledModSetter{
		ID:                 omit.From("mod-foreign"),
		GameServerID:       omit.From("server-local-2"),
		Source:             omit.From("modrinth"),
		SourceID:           omit.From("src-foreign"),
		ModName:            omit.From("ForeignMod"),
		ModAuthor:          omit.From("Author"),
		InstalledVersion:   omit.From("1.0.0"),
		InstalledVersionID: omit.From("v1"),
		FileHash:           omit.From("hash"),
		AutoUpdate:         omit.From(int64(0)),
		Enabled:            omit.From(int64(1)),
		CreatedAt:          omit.From(now),
		UpdatedAt:          omit.From(now),
	})
	if errInsert != nil {
		t.Fatalf("InsertInstalledMod() error = %v", errInsert)
	}

	request := connect.NewRequest(&xylona.SetModAutoUpdateRequest{
		GameServerId:   "server-local-1",
		InstalledModId: "mod-foreign",
		Enabled:        true,
	})
	addSessionCookieHeader(t, fixture.conn, fixture.secureCookie, request, "user-owner")

	_, errUpdate := fixture.service.SetModAutoUpdate(t.Context(), request)
	if errUpdate == nil {
		t.Fatal("SetModAutoUpdate() error = nil, want not found")
	}
	if connect.CodeOf(errUpdate) != connect.CodeNotFound {
		t.Fatalf("SetModAutoUpdate() code = %v, want %v", connect.CodeOf(errUpdate), connect.CodeNotFound)
	}

	stored, errGet := fixture.conn.GetInstalledModByID("mod-foreign")
	if errGet != nil {
		t.Fatalf("GetInstalledModByID() error = %v", errGet)
	}
	if stored.AutoUpdate != 0 {
		t.Fatalf("foreign mod AutoUpdate = %d, want 0", stored.AutoUpdate)
	}
}
