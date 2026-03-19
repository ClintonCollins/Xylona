package actions

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	fileGetPath      = "/api/file/get"
	fileDownloadPath = "/api/file/download"
	fileUploadPath   = "/api/file/upload"
)

var hopByHopHeaderNames = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
	"Proxy-Connection":    {},
}

type fileRequestTarget struct {
	gameServer     *models.GameServer
	remoteServerID string
	remoteNode     *models.Node
}

func (frt fileRequestTarget) isLocal() bool {
	return frt.gameServer != nil
}

func (inst *Instance) resolveFileRequestTarget(gameServerID string) (fileRequestTarget, error) {
	return resolveFileRequestTargetWithLookups(
		gameServerID,
		inst.db.GetGameServerByID,
		inst.db.GetRemoteServerCacheByRemoteServerID,
		inst.db.GetRemoteNodeByID,
	)
}

func resolveFileRequestTargetWithLookups(
	gameServerID string,
	localLookup func(string) (*models.GameServer, error),
	remoteCacheLookup func(string) (*models.RemoteServerCache, error),
	remoteNodeLookup func(string) (*models.Node, error),
) (fileRequestTarget, error) {
	gameServer, errGetGameServer := localLookup(gameServerID)
	if errGetGameServer == nil {
		return fileRequestTarget{gameServer: gameServer}, nil
	}
	if !errors.Is(errGetGameServer, sql.ErrNoRows) {
		return fileRequestTarget{}, errGetGameServer
	}

	remoteServer, errGetRemoteServer := remoteCacheLookup(gameServerID)
	if errGetRemoteServer != nil {
		if errors.Is(errGetRemoteServer, sql.ErrNoRows) {
			return fileRequestTarget{}, sql.ErrNoRows
		}
		return fileRequestTarget{}, errGetRemoteServer
	}

	remoteNode, errGetRemoteNode := remoteNodeLookup(remoteServer.NodeID)
	if errGetRemoteNode != nil {
		if errors.Is(errGetRemoteNode, sql.ErrNoRows) {
			return fileRequestTarget{}, sql.ErrNoRows
		}
		return fileRequestTarget{}, errGetRemoteNode
	}

	if !remoteNode.Enabled {
		return fileRequestTarget{}, sql.ErrNoRows
	}

	return fileRequestTarget{
		remoteServerID: remoteServer.RemoteServerID,
		remoteNode:     remoteNode,
	}, nil
}

func (inst *Instance) proxyRemoteFileGet(ctx context.Context, target fileRequestTarget, filePath string, w http.ResponseWriter) error {
	payload := xylona.DownloadFileRequest{
		GameServerId: target.remoteServerID,
		Path:         filePath,
	}

	bodyBytes, errMarshal := protojson.Marshal(&payload)
	if errMarshal != nil {
		return errMarshal
	}

	remoteURL, errRemoteURL := inst.remoteFederationFileURL(target.remoteNode, fileGetPath)
	if errRemoteURL != nil {
		return errRemoteURL
	}
	req, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, remoteURL, bytes.NewReader(bodyBytes))
	if errRequest != nil {
		return errRequest
	}
	req.Header.Set("Content-Type", "application/json")

	return inst.proxyRemoteFileRequest(req, target.remoteNode, w)
}

func (inst *Instance) proxyRemoteFileDownload(ctx context.Context, target fileRequestTarget, filePath string, w http.ResponseWriter) error {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	errGameServerID := writer.WriteField("gameServerId", target.remoteServerID)
	if errGameServerID != nil {
		return errGameServerID
	}
	errPath := writer.WriteField("path", filePath)
	if errPath != nil {
		return errPath
	}
	errClose := writer.Close()
	if errClose != nil {
		return errClose
	}

	remoteURL, errRemoteURL := inst.remoteFederationFileURL(target.remoteNode, fileDownloadPath)
	if errRemoteURL != nil {
		return errRemoteURL
	}
	req, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, remoteURL, body)
	if errRequest != nil {
		return errRequest
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return inst.proxyRemoteFileRequest(req, target.remoteNode, w)
}

func (inst *Instance) proxyRemoteFileUpload(ctx context.Context, target fileRequestTarget, destinationPath string, fileName string, fileSource io.Reader, w http.ResponseWriter) error {
	pipedReader, pipedWriter := io.Pipe()
	writer := multipart.NewWriter(pipedWriter)

	go func() {
		defer func() {
			errClose := writer.Close()
			if errClose != nil {
				errPipe := pipedWriter.CloseWithError(errClose)
				if errPipe != nil {
					log.Error().Err(errPipe).Msg("Failed to close multipart writer for remote file upload proxy")
				}
				return
			}
			errPipe := pipedWriter.Close()
			if errPipe != nil {
				log.Error().Err(errPipe).Msg("Failed to close pipe writer for remote file upload proxy")
			}
		}()

		errGameServerID := writer.WriteField("gameServerId", target.remoteServerID)
		if errGameServerID != nil {
			errPipe := pipedWriter.CloseWithError(errGameServerID)
			if errPipe != nil {
				log.Error().Err(errPipe).Msg("Failed to close pipe writer after remote game server ID write error")
			}
			return
		}

		errPath := writer.WriteField("path", destinationPath)
		if errPath != nil {
			errPipe := pipedWriter.CloseWithError(errPath)
			if errPipe != nil {
				log.Error().Err(errPipe).Msg("Failed to close pipe writer after remote path write error")
			}
			return
		}

		filePart, errCreatePart := writer.CreateFormFile("file", fileName)
		if errCreatePart != nil {
			errPipe := pipedWriter.CloseWithError(errCreatePart)
			if errPipe != nil {
				log.Error().Err(errPipe).Msg("Failed to close pipe writer after remote file part creation error")
			}
			return
		}

		_, errCopy := io.Copy(filePart, fileSource)
		if errCopy != nil {
			errPipe := pipedWriter.CloseWithError(errCopy)
			if errPipe != nil {
				log.Error().Err(errPipe).Msg("Failed to close pipe writer after remote file upload copy error")
			}
		}
	}()

	remoteURL, errRemoteURL := inst.remoteFederationFileURL(target.remoteNode, fileUploadPath)
	if errRemoteURL != nil {
		return errRemoteURL
	}
	req, errRequest := http.NewRequestWithContext(ctx, http.MethodPost, remoteURL, pipedReader)
	if errRequest != nil {
		return errRequest
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return inst.proxyRemoteFileRequest(req, target.remoteNode, w)
}

func (inst *Instance) proxyRemoteFileRequest(req *http.Request, remoteNode *models.Node, w http.ResponseWriter) error {
	httpClient, errClient := inst.remoteFederationHTTPClient(remoteNode)
	if errClient != nil {
		return errClient
	}

	resp, errDo := httpClient.Do(req)
	if errDo != nil {
		return errDo
	}
	defer func() {
		errClose := resp.Body.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close remote proxy response body")
		}
	}()

	connectionSpecificHeaders := make(map[string]struct{})
	for _, connectionValue := range resp.Header.Values("Connection") {
		for token := range strings.SplitSeq(connectionValue, ",") {
			headerName := http.CanonicalHeaderKey(strings.TrimSpace(token))
			if headerName == "" {
				continue
			}
			connectionSpecificHeaders[headerName] = struct{}{}
		}
	}

	for headerName, headerValues := range resp.Header {
		canonicalHeaderName := http.CanonicalHeaderKey(headerName)
		if _, isHopByHop := hopByHopHeaderNames[canonicalHeaderName]; isHopByHop {
			continue
		}
		if _, isConnectionSpecific := connectionSpecificHeaders[canonicalHeaderName]; isConnectionSpecific {
			continue
		}
		for _, headerValue := range headerValues {
			w.Header().Add(headerName, headerValue)
		}
	}
	w.WriteHeader(resp.StatusCode)

	_, errCopy := io.Copy(w, resp.Body)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy remote proxy response body")
	}
	return nil
}

func (inst *Instance) remoteFederationFileURL(remoteNode *models.Node, path string) (string, error) {
	if inst.federationMTLS == nil {
		return "", errors.New("federation mTLS is not configured")
	}

	federationBaseURL, errFederationURL := inst.federationMTLS.FederationBaseURLWithPort(remoteNode.BaseURL, inst.remoteFederationPort(remoteNode))
	if errFederationURL != nil {
		return "", errFederationURL
	}
	return strings.TrimSuffix(federationBaseURL, "/") + path, nil
}

func (inst *Instance) remoteFederationHTTPClient(remoteNode *models.Node) (*http.Client, error) {
	if inst.federationMTLS == nil {
		return nil, errors.New("federation mTLS is not configured")
	}

	httpClient, _, errClient := inst.federationMTLS.NewTrustedPeerHTTPClientWithPort(
		federationFileTransferTimeout,
		remoteNode.ID,
		remoteNode.BaseURL,
		inst.remoteFederationPort(remoteNode),
		inst.db,
	)
	if errClient != nil {
		return nil, errClient
	}
	return httpClient, nil
}

func (inst *Instance) remoteFederationPort(remoteNode *models.Node) int {
	if remoteNode != nil && remoteNode.Port > 0 {
		return int(remoteNode.Port)
	}
	if inst.federationMTLS != nil {
		return inst.federationMTLS.FederationPort()
	}
	return 0
}

func writeGameServerLookupError(w http.ResponseWriter, lookupErr error) {
	if errors.Is(lookupErr, sql.ErrNoRows) {
		http.Error(w, "Game server not found", http.StatusNotFound)
		return
	}
	http.Error(w, "Failed to get game server", http.StatusInternalServerError)
}
