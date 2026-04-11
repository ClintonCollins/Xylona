package actions

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	apifederation "github.com/ClintonCollins/Xylona/api/federation"
	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/db/dbtest"
	fedheaders "github.com/ClintonCollins/Xylona/helpers/federation"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestLegacyFileReadHandlersRejectUnauthorizedLocalUser(t *testing.T) {
	t.Parallel()

	inst, gameServer, _ := newLegacyFileAuthTestSetup(t)

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		request func(t *testing.T, gameServerID string) *http.Request
	}{
		{
			name:    "stream file to user",
			handler: inst.StreamFileToUser,
			request: func(t *testing.T, gameServerID string) *http.Request {
				t.Helper()
				requestBody, errMarshal := protojson.Marshal(&xylona.DownloadFileRequest{
					GameServerId: gameServerID,
					Path:         "server.properties",
				})
				if errMarshal != nil {
					t.Fatalf("protojson.Marshal() error = %v", errMarshal)
				}
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/get", bytes.NewReader(requestBody))
				request.Header.Set("Content-Type", "application/json")
				return request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-other"}))
			},
		},
		{
			name:    "download file to user post",
			handler: inst.UploadFileToUserPOST,
			request: func(t *testing.T, gameServerID string) *http.Request {
				t.Helper()
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/download", strings.NewReader("gameServerId="+gameServerID+"&path=server.properties"))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-other"}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			responseRecorder := httptest.NewRecorder()
			tt.handler(responseRecorder, tt.request(t, gameServer.ID))

			if responseRecorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusForbidden)
			}
		})
	}
}

func TestDownloadGameServerFileRejectsUnauthorizedLocalUser(t *testing.T) {
	t.Parallel()

	inst, gameServer, gameServerDir := newLegacyFileAuthTestSetup(t)

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)
	errWriteField := writer.WriteField("gameServerId", gameServer.ID)
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	errWriteField = writer.WriteField("path", "uploads")
	if errWriteField != nil {
		t.Fatalf("WriteField(path) error = %v", errWriteField)
	}
	filePart, errCreatePart := writer.CreateFormFile("file", "unauthorized.txt")
	if errCreatePart != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreatePart)
	}
	_, errWriteFile := filePart.Write([]byte("payload"))
	if errWriteFile != nil {
		t.Fatalf("Write(file) error = %v", errWriteFile)
	}
	errClose := writer.Close()
	if errClose != nil {
		t.Fatalf("writer.Close() error = %v", errClose)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/upload", requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-other"}))

	responseRecorder := httptest.NewRecorder()
	inst.DownloadGameServerFile(responseRecorder, request)

	if responseRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusForbidden)
	}

	uploadedPath := filepath.Join(gameServerDir, "uploads", "unauthorized.txt")
	_, errStat := os.Stat(uploadedPath)
	if errStat == nil || !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("uploaded file exists, want not exist")
	}
}

func TestLegacyFileHandlersRejectFederatedRequestsWithoutActingIdentity(t *testing.T) {
	t.Parallel()

	inst, gameServer, _ := newLegacyFileAuthTestSetup(t)

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		request func(t *testing.T, gameServerID string) *http.Request
	}{
		{
			name:    "stream file to user",
			handler: inst.StreamFileToUser,
			request: func(t *testing.T, gameServerID string) *http.Request {
				t.Helper()
				requestBody, errMarshal := protojson.Marshal(&xylona.DownloadFileRequest{
					GameServerId: gameServerID,
					Path:         "server.properties",
				})
				if errMarshal != nil {
					t.Fatalf("protojson.Marshal() error = %v", errMarshal)
				}
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/get", bytes.NewReader(requestBody))
				request.Header.Set("Content-Type", "application/json")
				request = request.WithContext(apifederation.WithPeerIdentity(request.Context(), apifederation.PeerIdentity{
					NodeID:     "node-remote",
					PeerNodeID: "peer-node",
				}))
				return request
			},
		},
		{
			name:    "download file to user post",
			handler: inst.UploadFileToUserPOST,
			request: func(t *testing.T, gameServerID string) *http.Request {
				t.Helper()
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/download", strings.NewReader("gameServerId="+gameServerID+"&path=server.properties"))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				request = request.WithContext(apifederation.WithPeerIdentity(request.Context(), apifederation.PeerIdentity{
					NodeID:     "node-remote",
					PeerNodeID: "peer-node",
				}))
				return request
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			responseRecorder := httptest.NewRecorder()
			tt.handler(responseRecorder, tt.request(t, gameServer.ID))

			if responseRecorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusForbidden)
			}
		})
	}
}

func TestDownloadGameServerFileRejectsFederatedUserWithoutPermission(t *testing.T) {
	t.Parallel()

	inst, gameServer, gameServerDir := newLegacyFileAuthTestSetup(t)

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)
	errWriteField := writer.WriteField("gameServerId", gameServer.ID)
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	errWriteField = writer.WriteField("path", "uploads")
	if errWriteField != nil {
		t.Fatalf("WriteField(path) error = %v", errWriteField)
	}
	filePart, errCreatePart := writer.CreateFormFile("file", "unauthorized.txt")
	if errCreatePart != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreatePart)
	}
	_, errWriteFile := filePart.Write([]byte("payload"))
	if errWriteFile != nil {
		t.Fatalf("Write(file) error = %v", errWriteFile)
	}
	errClose := writer.Close()
	if errClose != nil {
		t.Fatalf("writer.Close() error = %v", errClose)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/upload", requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(apifederation.WithPeerIdentity(request.Context(), apifederation.PeerIdentity{
		NodeID:     "node-remote",
		PeerNodeID: "peer-node",
	}))
	fedheaders.ApplyActingIdentityHeaders(request.Header, &models.User{ID: "user-other"}, "node-remote")

	responseRecorder := httptest.NewRecorder()
	inst.DownloadGameServerFile(responseRecorder, request)

	if responseRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusForbidden)
	}

	uploadedPath := filepath.Join(gameServerDir, "uploads", "unauthorized.txt")
	_, errStat := os.Stat(uploadedPath)
	if errStat == nil || !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("uploaded file exists, want not exist")
	}
}

func TestDownloadGameServerFileAbortsAfterFirstFailedFilePart(t *testing.T) {
	t.Parallel()

	inst, gameServer, gameServerDir := newLegacyFileAuthTestSetup(t)

	requestBody := &bytes.Buffer{}
	writer := multipart.NewWriter(requestBody)
	errWriteField := writer.WriteField("gameServerId", gameServer.ID)
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	errWriteField = writer.WriteField("path", "uploads")
	if errWriteField != nil {
		t.Fatalf("WriteField(path) error = %v", errWriteField)
	}

	firstPart, errCreatePart := writer.CreateFormFile("file", ".")
	if errCreatePart != nil {
		t.Fatalf("CreateFormFile(first) error = %v", errCreatePart)
	}
	_, errWriteFile := firstPart.Write([]byte("bad-payload"))
	if errWriteFile != nil {
		t.Fatalf("Write(first file) error = %v", errWriteFile)
	}

	secondPart, errCreateSecondPart := writer.CreateFormFile("file", "second.txt")
	if errCreateSecondPart != nil {
		t.Fatalf("CreateFormFile(second) error = %v", errCreateSecondPart)
	}
	_, errWriteSecondFile := secondPart.Write([]byte("good-payload"))
	if errWriteSecondFile != nil {
		t.Fatalf("Write(second file) error = %v", errWriteSecondFile)
	}

	errClose := writer.Close()
	if errClose != nil {
		t.Fatalf("writer.Close() error = %v", errClose)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/upload", requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-owner"}))

	responseRecorder := httptest.NewRecorder()
	inst.DownloadGameServerFile(responseRecorder, request)

	if responseRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusInternalServerError)
	}

	abortedPath := filepath.Join(gameServerDir, "uploads", "second.txt")
	_, errStat := os.Stat(abortedPath)
	if errStat == nil || !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("second file exists, want not exist")
	}
}

func TestHandleLegacyFileTransferTargetDispatchesByTargetType(t *testing.T) {
	t.Parallel()

	inst, gameServer, _ := newLegacyFileAuthTestSetup(t)

	tests := []struct {
		name         string
		target       fileRequestTarget
		permissionID string
		request      func(t *testing.T) *http.Request
		wantLocal    bool
		wantRemote   bool
	}{
		{
			name: "local target uses local handler",
			target: fileRequestTarget{
				gameServer: gameServer,
			},
			permissionID: "game_server.files.view",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/file/get", nil)
				return request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-owner"}))
			},
			wantLocal:  true,
			wantRemote: false,
		},
		{
			name: "local target accepts the edit permission path too",
			target: fileRequestTarget{
				gameServer: gameServer,
			},
			permissionID: "game_server.files.edit",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/upload", nil)
				return request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-owner"}))
			},
			wantLocal:  true,
			wantRemote: false,
		},
		{
			name: "remote target uses remote handler without local auth",
			target: fileRequestTarget{
				remoteServerID: "remote-server-id",
				remoteNode: &models.Node{
					ID:      "remote-node",
					Enabled: true,
				},
			},
			permissionID: "game_server.files.view",
			request: func(t *testing.T) *http.Request {
				t.Helper()
				return httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/file/get", nil)
			},
			wantLocal:  false,
			wantRemote: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			localCalled := false
			remoteCalled := false

			responseRecorder := httptest.NewRecorder()
			inst.handleLegacyFileTransferTarget(
				responseRecorder,
				tt.request(t),
				tt.target,
				tt.permissionID,
				"Failed to get file",
				func(gameServer *models.GameServer) error {
					localCalled = true
					if gameServer != nil && gameServer.ID != tt.target.gameServer.ID {
						t.Fatalf("local handler received server %q, want %q", gameServer.ID, tt.target.gameServer.ID)
					}
					return nil
				},
				func(target fileRequestTarget) error {
					remoteCalled = true
					if target.remoteServerID != tt.target.remoteServerID {
						t.Fatalf("remote handler received server ID %q, want %q", target.remoteServerID, tt.target.remoteServerID)
					}
					return nil
				},
			)

			if responseRecorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusOK)
			}
			if localCalled != tt.wantLocal {
				t.Fatalf("local handler called = %t, want %t", localCalled, tt.wantLocal)
			}
			if remoteCalled != tt.wantRemote {
				t.Fatalf("remote handler called = %t, want %t", remoteCalled, tt.wantRemote)
			}
		})
	}
}

func TestLegacyFileHandlersReturnForbiddenForRemoteFederatedRequestsWithoutActingIdentity(t *testing.T) {
	t.Parallel()

	inst, gameServerID := newLegacyRemoteFileAuthTestSetup(t)

	tests := []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		request func(t *testing.T) *http.Request
	}{
		{
			name:    "stream file to user",
			handler: inst.StreamFileToUser,
			request: func(t *testing.T) *http.Request {
				t.Helper()
				requestBody, errMarshal := protojson.Marshal(&xylona.DownloadFileRequest{
					GameServerId: gameServerID,
					Path:         "server.properties",
				})
				if errMarshal != nil {
					t.Fatalf("protojson.Marshal() error = %v", errMarshal)
				}
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/get", bytes.NewReader(requestBody))
				request.Header.Set("Content-Type", "application/json")
				return request.WithContext(apifederation.WithPeerIdentity(request.Context(), apifederation.PeerIdentity{
					NodeID:     "remote-node-1",
					PeerNodeID: "peer-node-1",
				}))
			},
		},
		{
			name:    "download file to user post",
			handler: inst.UploadFileToUserPOST,
			request: func(t *testing.T) *http.Request {
				t.Helper()
				request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/download", strings.NewReader("gameServerId="+gameServerID+"&path=server.properties"))
				request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return request.WithContext(apifederation.WithPeerIdentity(request.Context(), apifederation.PeerIdentity{
					NodeID:     "remote-node-1",
					PeerNodeID: "peer-node-1",
				}))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			responseRecorder := httptest.NewRecorder()
			tt.handler(responseRecorder, tt.request(t))

			if responseRecorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusForbidden)
			}
		})
	}
}

func TestDownloadGameServerFileWithMaxBytesRejectsOversizedMultipartBody(t *testing.T) {
	t.Parallel()

	inst, gameServer, gameServerDir := newLegacyFileAuthTestSetup(t)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	errWriteField := writer.WriteField("gameServerId", gameServer.ID)
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	errWriteField = writer.WriteField("path", "uploads")
	if errWriteField != nil {
		t.Fatalf("WriteField(path) error = %v", errWriteField)
	}
	filePart, errCreatePart := writer.CreateFormFile("file", "oversized.txt")
	if errCreatePart != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreatePart)
	}
	_, errWriteFile := filePart.Write([]byte(strings.Repeat("b", 2048)))
	if errWriteFile != nil {
		t.Fatalf("Write(file) error = %v", errWriteFile)
	}
	errClose := writer.Close()
	if errClose != nil {
		t.Fatalf("writer.Close() error = %v", errClose)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/file/upload", body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(gatekeeper.WithUser(request.Context(), &models.User{ID: "user-owner"}))

	responseRecorder := httptest.NewRecorder()
	inst.downloadGameServerFileWithMaxBytes(responseRecorder, request, 1024)

	if responseRecorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", responseRecorder.Code, http.StatusRequestEntityTooLarge)
	}

	uploadedPath := filepath.Join(gameServerDir, "uploads", "oversized.txt")
	_, errStat := os.Stat(uploadedPath)
	if errStat == nil || !errors.Is(errStat, os.ErrNotExist) {
		t.Fatalf("uploaded file exists, want not exist")
	}
}

func newLegacyFileAuthTestSetup(t *testing.T) (*Instance, *models.GameServer, string) {
	t.Helper()

	conn := dbtest.NewMigratedConnection(t, "legacy-file-auth.sqlite")

	_, errOwner := conn.SQLDb.ExecContext(context.Background(), `insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		values ('user-owner', 'owner', 'owner@example.com', 'Owner', 'User', 'hash', false, current_timestamp, current_timestamp, current_timestamp)`)
	if errOwner != nil {
		t.Fatalf("failed to insert owner user: %v", errOwner)
	}
	_, errOther := conn.SQLDb.ExecContext(context.Background(), `insert into user (id, user_name, email, first_name, last_name, password_hash, super_user, last_login_at, created_at, updated_at)
		values ('user-other', 'other', 'other@example.com', 'Other', 'User', 'hash', false, current_timestamp, current_timestamp, current_timestamp)`)
	if errOther != nil {
		t.Fatalf("failed to insert other user: %v", errOther)
	}
	_, errNode := conn.SQLDb.ExecContext(context.Background(), `insert into node (id, name, is_local, host, port, base_url, enabled, health_status, last_sync_status, version, protocol_version, capabilities, sync_interval_seconds, allow_insecure_tls)
		values ('node-local', 'Local Node', true, '127.0.0.1', 0, 'http://127.0.0.1', true, 'healthy', '', '', 0, '', 60, false)`)
	if errNode != nil {
		t.Fatalf("failed to insert local node: %v", errNode)
	}
	_, errIP := conn.SQLDb.ExecContext(context.Background(), `insert into ip (address, usable, external) values ('127.0.0.1', true, false)`)
	if errIP != nil {
		t.Fatalf("failed to insert ip: %v", errIP)
	}
	gameServerDir := filepath.Join(t.TempDir(), "server-local-1")
	errMkdir := os.MkdirAll(gameServerDir, 0o750)
	if errMkdir != nil {
		t.Fatalf("MkdirAll(serverDir) error = %v", errMkdir)
	}
	errWrite := os.WriteFile(filepath.Join(gameServerDir, "server.properties"), []byte("motd=legacy"), 0o600)
	if errWrite != nil {
		t.Fatalf("WriteFile(server.properties) error = %v", errWrite)
	}

	now := time.Now().UTC()
	_, errServer := conn.SQLDb.ExecContext(context.Background(), `insert into game_server
		(id, user_id, name, game_id, status, set_players, max_players, map, ip, port, query_port, directory, node_id, start_args_patches, created_at, updated_at)
		values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"server-local-1", "user-owner", "Local One", "minecraft", "OFFLINE",
		20, 20, "world", "127.0.0.1", 25565, 25565, gameServerDir, "node-local", "[]", now, now,
	)
	if errServer != nil {
		t.Fatalf("failed to insert game server: %v", errServer)
	}
	gameServer, errGetServer := conn.GetGameServerByID("server-local-1")
	if errGetServer != nil {
		t.Fatalf("failed to load inserted game server: %v", errGetServer)
	}

	return &Instance{db: conn}, gameServer, gameServerDir
}

func newLegacyRemoteFileAuthTestSetup(t *testing.T) (*Instance, string) {
	t.Helper()

	inst, remoteNode := newMTLSFileProxyTestSetup(t, http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Fatalf("remote federation server should not be called for acting-identity validation failures")
	}))

	_, errUpdate := inst.db.SQLDb.ExecContext(context.Background(), `update node set base_url = ? where id = 'remote-node-1'`, remoteNode.BaseURL)
	if errUpdate != nil {
		t.Fatalf("failed to update remote node base URL: %v", errUpdate)
	}

	_, errCache := inst.db.SQLDb.ExecContext(context.Background(), `insert into remote_server_cache (id, source_node_id, node_id, remote_server_id)
		values ('cache-remote-1', 'node-local', 'remote-node-1', 'remote-server-1')`)
	if errCache != nil {
		t.Fatalf("failed to insert remote server cache: %v", errCache)
	}

	return inst, "remote-server-1"
}
