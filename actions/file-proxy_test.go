package actions

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aarondl/opt/null"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestResolveFileRequestTargetWithLookups(t *testing.T) {
	errDBUnavailable := errors.New("db unavailable")

	tests := []struct {
		name              string
		localLookup       func(string) (*models.GameServer, error)
		remoteCacheLookup func(string) (*models.RemoteServerCache, error)
		remoteNodeLookup  func(string) (*models.Node, error)
		wantLocal         bool
		wantRemoteID      string
		wantErr           error
	}{
		{
			name: "uses local game server when present",
			localLookup: func(string) (*models.GameServer, error) {
				return &models.GameServer{ID: "local-server"}, nil
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				t.Fatalf("remote cache lookup should not be called when local server exists")
				return nil, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				t.Fatalf("remote node lookup should not be called when local server exists")
				return nil, nil
			},
			wantLocal: true,
		},
		{
			name: "falls back to remote server when local lookup is not found",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, sql.ErrNoRows
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				return &models.RemoteServerCache{RemoteServerID: "remote-server", NodeID: "node-1"}, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				return &models.Node{ID: "node-1", Enabled: true}, nil
			},
			wantRemoteID: "remote-server",
		},
		{
			name: "returns not found when remote node is disabled",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, sql.ErrNoRows
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				return &models.RemoteServerCache{RemoteServerID: "remote-server", NodeID: "node-1"}, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				return &models.Node{ID: "node-1", Enabled: false}, nil
			},
			wantErr: sql.ErrNoRows,
		},
		{
			name: "returns local lookup error",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, errDBUnavailable
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				t.Fatalf("remote cache lookup should not be called when local lookup fails")
				return nil, nil
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				t.Fatalf("remote node lookup should not be called when local lookup fails")
				return nil, nil
			},
			wantErr: errDBUnavailable,
		},
		{
			name: "returns not found when remote cache is missing",
			localLookup: func(string) (*models.GameServer, error) {
				return nil, sql.ErrNoRows
			},
			remoteCacheLookup: func(string) (*models.RemoteServerCache, error) {
				return nil, sql.ErrNoRows
			},
			remoteNodeLookup: func(string) (*models.Node, error) {
				t.Fatalf("remote node lookup should not be called when remote cache is missing")
				return nil, nil
			},
			wantErr: sql.ErrNoRows,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			target, errResolve := resolveFileRequestTargetWithLookups("server-id", tt.localLookup, tt.remoteCacheLookup, tt.remoteNodeLookup)
			if tt.wantErr != nil {
				if errResolve == nil {
					t.Fatalf("resolveFileRequestTargetWithLookups() error = nil, want non-nil")
				}
				if !errors.Is(errResolve, tt.wantErr) {
					t.Fatalf("resolveFileRequestTargetWithLookups() error = %v, want %v", errResolve, tt.wantErr)
				}
				return
			}

			if errResolve != nil {
				t.Fatalf("resolveFileRequestTargetWithLookups() error = %v, want nil", errResolve)
			}
			if target.isLocal() != tt.wantLocal {
				t.Errorf("target.isLocal() = %t, want %t", target.isLocal(), tt.wantLocal)
			}
			if target.remoteServerID != tt.wantRemoteID {
				t.Errorf("target.remoteServerID = %q, want %q", target.remoteServerID, tt.wantRemoteID)
			}
		})
	}
}

func TestProxyRemoteFileGet(t *testing.T) {
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fileGetPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, fileGetPath)
		}
		if gotHeader := r.Header.Get("X-Federation-Key"); gotHeader != "secret-key" {
			t.Fatalf("X-Federation-Key = %q, want %q", gotHeader, "secret-key")
		}

		bodyBytes, errRead := io.ReadAll(r.Body)
		if errRead != nil {
			t.Fatalf("failed to read request body: %v", errRead)
		}

		request := xylona.DownloadFileRequest{}
		errDecode := protojson.Unmarshal(bodyBytes, &request)
		if errDecode != nil {
			t.Fatalf("failed to decode request body: %v", errDecode)
		}

		if request.GameServerId != "remote-server-id" {
			t.Fatalf("request.GameServerId = %q, want %q", request.GameServerId, "remote-server-id")
		}
		if request.Path != "server.properties" {
			t.Fatalf("request.Path = %q, want %q", request.Path, "server.properties")
		}

		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("remote-content"))
	}))
	t.Cleanup(remoteServer.Close)

	inst := &Instance{}
	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode: &models.Node{
			BaseURL:   remoteServer.URL,
			SecretKey: null.From("secret-key"),
		},
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileGet(context.Background(), target, "server.properties", responseRecorder)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileGet() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusOK)
	}
	if responseRecorder.Body.String() != "remote-content" {
		t.Errorf("response body = %q, want %q", responseRecorder.Body.String(), "remote-content")
	}
}

func TestProxyRemoteFileDownload(t *testing.T) {
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fileDownloadPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, fileDownloadPath)
		}

		errParse := r.ParseMultipartForm(1024)
		if errParse != nil {
			t.Fatalf("failed to parse multipart form: %v", errParse)
		}

		if gotServerID := r.FormValue("gameServerId"); gotServerID != "remote-server-id" {
			t.Fatalf("gameServerId = %q, want %q", gotServerID, "remote-server-id")
		}
		if gotPath := r.FormValue("path"); gotPath != "logs/latest.log" {
			t.Fatalf("path = %q, want %q", gotPath, "logs/latest.log")
		}

		_, _ = w.Write([]byte("download-content"))
	}))
	t.Cleanup(remoteServer.Close)

	inst := &Instance{}
	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode: &models.Node{
			BaseURL:   remoteServer.URL,
			SecretKey: null.From("secret-key"),
		},
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileDownload(context.Background(), target, "logs/latest.log", responseRecorder)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileDownload() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusOK)
	}
	if responseRecorder.Body.String() != "download-content" {
		t.Errorf("response body = %q, want %q", responseRecorder.Body.String(), "download-content")
	}
}

func TestProxyRemoteFileUpload(t *testing.T) {
	remoteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fileUploadPath {
			t.Fatalf("request path = %q, want %q", r.URL.Path, fileUploadPath)
		}

		reader, errReader := r.MultipartReader()
		if errReader != nil {
			t.Fatalf("failed to create multipart reader: %v", errReader)
		}

		foundFile := false
		for {
			part, errPart := reader.NextPart()
			if errors.Is(errPart, io.EOF) {
				break
			}
			if errPart != nil {
				t.Fatalf("failed to get next multipart part: %v", errPart)
			}

			switch part.FormName() {
			case "gameServerId":
				bodyBytes, errRead := io.ReadAll(part)
				if errRead != nil {
					t.Fatalf("failed to read gameServerId part: %v", errRead)
				}
				if string(bodyBytes) != "remote-server-id" {
					t.Fatalf("gameServerId part = %q, want %q", string(bodyBytes), "remote-server-id")
				}
			case "path":
				bodyBytes, errRead := io.ReadAll(part)
				if errRead != nil {
					t.Fatalf("failed to read path part: %v", errRead)
				}
				if string(bodyBytes) != "uploads" {
					t.Fatalf("path part = %q, want %q", string(bodyBytes), "uploads")
				}
			case "file":
				fileBytes, errRead := io.ReadAll(part)
				if errRead != nil {
					t.Fatalf("failed to read file part: %v", errRead)
				}
				if string(fileBytes) != "file-content" {
					t.Fatalf("file content = %q, want %q", string(fileBytes), "file-content")
				}
				if part.FileName() != "test.txt" {
					t.Fatalf("file name = %q, want %q", part.FileName(), "test.txt")
				}
				foundFile = true
			}
		}

		if !foundFile {
			t.Fatalf("expected multipart file part")
		}

		_, _ = w.Write([]byte("uploaded"))
	}))
	t.Cleanup(remoteServer.Close)

	inst := &Instance{}
	target := fileRequestTarget{
		remoteServerID: "remote-server-id",
		remoteNode: &models.Node{
			BaseURL:   remoteServer.URL,
			SecretKey: null.From("secret-key"),
		},
	}

	responseRecorder := httptest.NewRecorder()
	errProxy := inst.proxyRemoteFileUpload(
		context.Background(),
		target,
		"uploads",
		"test.txt",
		strings.NewReader("file-content"),
		responseRecorder,
	)
	if errProxy != nil {
		t.Fatalf("proxyRemoteFileUpload() error = %v", errProxy)
	}

	if responseRecorder.Code != http.StatusOK {
		t.Fatalf("response status = %d, want %d", responseRecorder.Code, http.StatusOK)
	}
	if responseRecorder.Body.String() != "uploaded" {
		t.Errorf("response body = %q, want %q", responseRecorder.Body.String(), "uploaded")
	}
}

