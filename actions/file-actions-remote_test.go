package actions

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/api/gatekeeper"
	"github.com/ClintonCollins/Xylona/pkg/node"
	"github.com/ClintonCollins/Xylona/pkg/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/noderegistry"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func configureRemoteFileActionFixture(t *testing.T) (*Instance, *models.GameServer, *models.User, *nodeclient.FakeNodeClient) {
	t.Helper()

	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)
	remoteClient := &nodeclient.FakeNodeClient{NodeID: fixture.nodeID}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	inst.nodeRegistry = registry

	user := &models.User{ID: fixture.userID}
	return inst, fixture.gameServer, user, remoteClient
}

func TestDownloadGameServerFileWritesRemoteUploadThroughNodeClient(t *testing.T) {
	inst, gameServer, user, remoteClient := configureRemoteFileActionFixture(t)

	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	errWriteField := writer.WriteField("gameServerId", gameServer.ID)
	if errWriteField != nil {
		t.Fatalf("WriteField(gameServerId) error = %v", errWriteField)
	}
	errWriteField = writer.WriteField("path", `configs\active`)
	if errWriteField != nil {
		t.Fatalf("WriteField(path) error = %v", errWriteField)
	}
	fileWriter, errCreateFile := writer.CreateFormFile("file", `..\server.properties`)
	if errCreateFile != nil {
		t.Fatalf("CreateFormFile() error = %v", errCreateFile)
	}
	_, errWriteFile := fileWriter.Write([]byte("motd=Remote\n"))
	if errWriteFile != nil {
		t.Fatalf("Write(file) error = %v", errWriteFile)
	}
	errCloseWriter := writer.Close()
	if errCloseWriter != nil {
		t.Fatalf("multipart writer close error = %v", errCloseWriter)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/files/upload", &requestBody)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request = request.WithContext(gatekeeper.WithUser(request.Context(), user))

	responseRecorder := httptest.NewRecorder()
	inst.DownloadGameServerFile(responseRecorder, request)

	response := responseRecorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("DownloadGameServerFile() status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if len(remoteClient.WriteFileCalls) != 0 {
		t.Fatalf("WriteFile call count = %d, want 0", len(remoteClient.WriteFileCalls))
	}
	if len(remoteClient.StreamWriteFileCalls) != 1 {
		t.Fatalf("StreamWriteFile call count = %d, want 1", len(remoteClient.StreamWriteFileCalls))
	}
	call := remoteClient.StreamWriteFileCalls[0]
	if call.Directory != gameServer.Directory {
		t.Fatalf("StreamWriteFile directory = %q, want %q", call.Directory, gameServer.Directory)
	}
	if call.RelativePath != "configs/active/server.properties" {
		t.Fatalf("StreamWriteFile relative path = %q, want %q", call.RelativePath, "configs/active/server.properties")
	}
	if string(call.Content) != "motd=Remote\n" {
		t.Fatalf("StreamWriteFile content = %q, want uploaded content", string(call.Content))
	}
	if len(remoteClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(remoteClient.CreateFileOrDirectoryCalls))
	}
	if remoteClient.CreateFileOrDirectoryCalls[0].RelativePath != "configs/active" {
		t.Fatalf("CreateFileOrDirectory relative path = %q, want %q", remoteClient.CreateFileOrDirectoryCalls[0].RelativePath, "configs/active")
	}

	localUploadPath := filepath.Join(gameServer.Directory, "configs", "active", "server.properties")
	_, errStat := os.Stat(localUploadPath)
	if !errorsIsNotExist(errStat) {
		t.Fatalf("Stat(local upload path) error = %v, want not exist", errStat)
	}
}

func TestStreamFileToUserReadsRemoteFileThroughNodeClient(t *testing.T) {
	inst, gameServer, user, remoteClient := configureRemoteFileActionFixture(t)
	remoteClient.StatFileResult = node.NewFileEntry("server.properties", 17, false, time.Now())
	remoteClient.StreamFileReader = io.NopCloser(strings.NewReader("remote file bytes"))

	fileRequest := &xylona.DownloadFileRequest{
		GameServerId: gameServer.ID,
		Path:         `configs\server.properties`,
	}
	requestBytes, errMarshal := protojson.Marshal(fileRequest)
	if errMarshal != nil {
		t.Fatalf("protojson.Marshal() error = %v", errMarshal)
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/api/files/download", bytes.NewReader(requestBytes))
	request = request.WithContext(gatekeeper.WithUser(request.Context(), user))
	responseRecorder := httptest.NewRecorder()

	inst.StreamFileToUser(responseRecorder, request)

	response := responseRecorder.Result()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("StreamFileToUser() status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if responseRecorder.Body.String() != "remote file bytes" {
		t.Fatalf("response body = %q, want remote bytes", responseRecorder.Body.String())
	}
	if response.Header.Get("Content-Length") != "17" {
		t.Fatalf("Content-Length = %q, want %q", response.Header.Get("Content-Length"), "17")
	}
	if len(remoteClient.StatFileCalls) != 1 {
		t.Fatalf("StatFile call count = %d, want 1", len(remoteClient.StatFileCalls))
	}
	if len(remoteClient.StreamFileCalls) != 1 {
		t.Fatalf("StreamFile call count = %d, want 1", len(remoteClient.StreamFileCalls))
	}
	call := remoteClient.StreamFileCalls[0]
	if call.Directory != gameServer.Directory {
		t.Fatalf("StreamFile directory = %q, want %q", call.Directory, gameServer.Directory)
	}
	if call.RelativePath != "configs/server.properties" {
		t.Fatalf("StreamFile relative path = %q, want %q", call.RelativePath, "configs/server.properties")
	}
}

func TestRunConfigPreStartWritesRemoteConfigThroughNodeClient(t *testing.T) {
	inst := newTestInstance(t)
	fixture := newBackupServiceFixture(t, inst)

	schemasJSON := `[{"path":"config\\server.properties","format":"properties","generate_before_start":true,"schema":{"type":"object","properties":{"motd":{"type":"string","default":"Generated"}}},"managed_fields":{"server-port":"game_server.port","query.port":"game_server.query_port"}}]`
	errUpdateSchemas := inst.db.UpdateGameConfigSchemas(fixture.gameID, schemasJSON)
	if errUpdateSchemas != nil {
		t.Fatalf("UpdateGameConfigSchemas() error = %v", errUpdateSchemas)
	}

	remoteClient := &nodeclient.FakeNodeClient{
		NodeID:      fixture.nodeID,
		ReadFileErr: os.ErrNotExist,
	}
	registry := noderegistry.New("node-local", &nodeclient.FakeNodeClient{NodeID: "node-local"})
	registry.Register(remoteClient)
	inst.nodeRegistry = registry
	fixture.gameServer.Directory = "/srv/xylona/remote-server"

	inst.runConfigPreStart(fixture.gameServer)

	if len(remoteClient.ReadFileCalls) != 1 {
		t.Fatalf("ReadFile call count = %d, want 1", len(remoteClient.ReadFileCalls))
	}
	if remoteClient.ReadFileCalls[0].RelativePath != "config/server.properties" {
		t.Fatalf("ReadFile relative path = %q, want %q", remoteClient.ReadFileCalls[0].RelativePath, "config/server.properties")
	}
	if len(remoteClient.CreateFileOrDirectoryCalls) != 1 {
		t.Fatalf("CreateFileOrDirectory call count = %d, want 1", len(remoteClient.CreateFileOrDirectoryCalls))
	}
	if remoteClient.CreateFileOrDirectoryCalls[0].RelativePath != "config" {
		t.Fatalf("CreateFileOrDirectory relative path = %q, want %q", remoteClient.CreateFileOrDirectoryCalls[0].RelativePath, "config")
	}
	if len(remoteClient.WriteFileCalls) != 1 {
		t.Fatalf("WriteFile call count = %d, want 1", len(remoteClient.WriteFileCalls))
	}
	call := remoteClient.WriteFileCalls[0]
	if call.RelativePath != "config/server.properties" {
		t.Fatalf("WriteFile relative path = %q, want %q", call.RelativePath, "config/server.properties")
	}
	content := string(call.Content)
	if !strings.Contains(content, "server-port=25565\n") {
		t.Fatalf("WriteFile content = %q, want managed server-port", content)
	}
	if !strings.Contains(content, "query.port=25565\n") {
		t.Fatalf("WriteFile content = %q, want managed query.port", content)
	}
}

func errorsIsNotExist(err error) bool {
	return os.IsNotExist(err)
}
