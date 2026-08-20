package actions

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// MaxRequestBodySize caps file action request bodies at 1 MiB.
const (
	MaxRequestBodySize          = 1024 * 1024 * 1 // 1 MB
	maxMultipartUploadBodyBytes = 100 << 30       // 100 GiB
)

// StreamFileToUser streams a server file to an HTTP response.
func (inst *Instance) StreamFileToUser(w http.ResponseWriter, r *http.Request) {
	fileRequest := xylona.DownloadFileRequest{}
	bodyBytes, errReadBody := io.ReadAll(io.LimitReader(r.Body, MaxRequestBodySize))
	if errReadBody != nil {
		log.Error().Err(errReadBody).Msg("Failed to read file request body")
		http.Error(w, "Failed to read file request body", http.StatusBadRequest)
		return
	}
	errDecode := protojson.Unmarshal(bodyBytes, &fileRequest)
	if errDecode != nil {
		log.Error().Err(errDecode).Msg("Failed to decode file request")
		http.Error(w, "Failed to decode file request", http.StatusBadRequest)
		return
	}

	inst.serveLocalFileRequest(
		w,
		r,
		fileRequest.GetGameServerId(),
		"game_server.files.view",
		"Failed to get file",
		func(gameServer *models.GameServer) error {
			return inst.GetGameServerFile(gameServer, fileRequest.GetPath(), w, true, false)
		},
	)
}

// UploadFileToUserGET streams a file download requested via query parameters.
func (inst *Instance) UploadFileToUserGET(w http.ResponseWriter, r *http.Request) {
	gameServerID, errGameServerID := url.QueryUnescape(chi.URLParam(r, "gameServerId"))
	filePath, errFilePath := url.QueryUnescape(chi.URLParam(r, "path"))
	if errGameServerID != nil || errFilePath != nil {
		log.Error().Err(errGameServerID).Err(errFilePath).Msg("Failed to get game server ID or file path")
		http.Error(w, "Failed to get game server ID or file path", http.StatusBadRequest)
		return
	}

	inst.serveLocalFileRequest(
		w,
		r,
		gameServerID,
		"game_server.files.view",
		"Failed to get file",
		func(gameServer *models.GameServer) error {
			return inst.GetGameServerFile(gameServer, filePath, w, true, true)
		},
	)
}

// UploadFileToUserPOST streams a file download requested via form fields.
func (inst *Instance) UploadFileToUserPOST(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	errParseForm := r.ParseForm() // #nosec G120 -- request body is capped by MaxBytesReader above.
	if errParseForm != nil {
		log.Error().Err(errParseForm).Msg("Failed to parse form")
		http.Error(w, "Failed to parse form", http.StatusBadRequest)
		return
	}
	gameServerID := r.PostForm.Get("gameServerId")
	filePath := r.PostForm.Get("path")

	inst.serveLocalFileRequest(
		w,
		r,
		gameServerID,
		"game_server.files.view",
		"Failed to get file",
		func(gameServer *models.GameServer) error {
			return inst.GetGameServerFile(gameServer, filePath, w, true, true)
		},
	)
}

// ListGameServerFiles lists files and directories for a relative server path.
func (inst *Instance) ListGameServerFiles(gameServer *models.GameServer, relativePath string) ([]*xylona.File, error) {
	fullPath, errResolve := node.ResolveExistingWithinRoot(gameServer.Directory, relativePath)
	if errResolve != nil {
		if errors.Is(errResolve, node.ErrInvalidPath) {
			return nil, ErrInvalidPath
		}
		return nil, fmt.Errorf("actions: resolve list path: %w", errResolve)
	}
	files, errReadDir := os.ReadDir(fullPath)
	if errReadDir != nil {
		if errors.Is(errReadDir, os.ErrNotExist) {
			log.Error().Err(errReadDir).Msg("Path does not exist")
			return nil, fmt.Errorf("actions: read server directory: %w", errReadDir)
		}
		log.Error().Err(errReadDir).Msg("Failed to read directory")
		return nil, fmt.Errorf("actions: read server directory: %w", errReadDir)
	}

	xylonaFiles := make([]*xylona.File, 0, len(files))
	for _, file := range files {
		fileInfo, errFileInfo := file.Info()
		if errFileInfo != nil {
			log.Error().Err(errFileInfo).Msg("Failed to get file info")
			return nil, fmt.Errorf("actions: stat directory entry: %w", errFileInfo)
		}
		size := fileInfo.Size()
		if fileInfo.IsDir() {
			var totalSize int64

			err := filepath.WalkDir(filepath.Join(fullPath, file.Name()), func(_ string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if !d.IsDir() {
					entryInfo, errInfo := d.Info()
					if errInfo != nil {
						return fmt.Errorf("actions: stat nested directory entry: %w", errInfo)
					}
					totalSize += entryInfo.Size()
				}
				return nil
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to walk through directory")
				return nil, fmt.Errorf("actions: walk directory: %w", err)
			}
			size = totalSize
		}
		xylonaFiles = append(xylonaFiles, &xylona.File{
			Name:         fileInfo.Name(),
			Size:         size,
			IsDirectory:  fileInfo.IsDir(),
			LastModified: timestamppb.New(fileInfo.ModTime()),
		})
	}

	return xylonaFiles, nil
}

// DownloadGameServerFile handles multipart uploads into a game server directory.
func (inst *Instance) DownloadGameServerFile(w http.ResponseWriter, r *http.Request) {
	inst.downloadGameServerFileWithMaxBytes(w, r, maxMultipartUploadBodyBytes)
}

func (inst *Instance) downloadGameServerFileWithMaxBytes(w http.ResponseWriter, r *http.Request, maxBodyBytes int64) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	multiReader, err := r.MultipartReader()
	if err != nil {
		writeMultipartUploadBodyError(w, err, "Error creating multipart reader")
		return
	}
	foundGameServerID := false
	foundPath := false
	gameServerID := ""
	relativePath := ""
	for {
		part, errNext := multiReader.NextPart()
		if errNext == io.EOF {
			break
		} else if errNext != nil {
			writeMultipartUploadBodyError(w, errNext, "Error reading next part")
			return
		}
		switch part.FormName() {
		case "gameServerId":
			gameServerIDBytes, errRead := io.ReadAll(io.LimitReader(part, 10<<10))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read game server ID")
				writeMultipartUploadBodyError(w, errRead, "Error reading game server ID")
				return
			}
			gameServerID = string(gameServerIDBytes)
			foundGameServerID = true
		case "path":
			pathBytes, errRead := io.ReadAll(io.LimitReader(part, 1<<20))
			if errRead != nil {
				log.Error().Err(errRead).Msg("Failed to read path")
				writeMultipartUploadBodyError(w, errRead, "Error reading path")
				return
			}
			relativePath = string(pathBytes)
			foundPath = true
		case "file":
			if !foundGameServerID || !foundPath {
				log.Error().Msg("Game server ID and path must be specified")
				http.Error(w, "Game server ID and path must be specified", http.StatusBadRequest)
				return
			}
			filename := part.FileName()
			if !inst.serveLocalFileRequest(
				w,
				r,
				gameServerID,
				"game_server.files.edit",
				"Failed to upload file",
				func(gameServer *models.GameServer) error {
					return inst.saveUploadedGameServerFile(gameServer, relativePath, filename, part)
				},
			) {
				return
			}
		}
	}
}

func writeMultipartUploadBodyError(w http.ResponseWriter, err error, fallbackMessage string) {
	if isRequestBodyTooLarge(err) {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	http.Error(w, fallbackMessage, http.StatusBadRequest)
}

func isRequestBodyTooLarge(err error) bool {
	if err == nil {
		return false
	}

	var maxBytesErr *http.MaxBytesError
	if errors.As(err, &maxBytesErr) {
		return true
	}

	errText := err.Error()
	return strings.Contains(errText, "request body too large") || strings.Contains(errText, "message too large")
}

func (inst *Instance) saveUploadedGameServerFile(gameServer *models.GameServer, relativePath, fileName string, fileSource io.Reader) error {
	validatedPath, errPath := validateRemoteServerPath(relativePath)
	if errPath != nil {
		return errPath
	}

	sanitizedFileName, errFileName := sanitizeRemoteUploadFileName(fileName)
	if errFileName != nil {
		return errFileName
	}

	protectedRelativePath := path.Join(validatedPath, sanitizedFileName)
	policy := remoteFileProtectionPolicy(gameServer)
	if policy.IsConfigured() && startargs.IsProtectedServerPath(protectedRelativePath, policy.BaseCommand, policy.ServerExecutable) {
		log.Warn().Str("Game Server ID", gameServer.ID).Str("path", protectedRelativePath).Msg("Blocked mutation of protected server path")
		return ErrProtectedPath
	}

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve node client for upload: %w", errClient)
	}

	if validatedPath != "" {
		errCreateDir := client.CreateFileOrDirectory(inst.ctx, gameServer.Directory, validatedPath, "", true, policy)
		if errCreateDir != nil {
			return fmt.Errorf("actions: create upload directory: %w", errCreateDir)
		}
	}

	_, errWrite := client.StreamWriteFile(inst.ctx, gameServer.Directory, protectedRelativePath, fileSource, policy)
	if errWrite != nil {
		return fmt.Errorf("actions: stream uploaded file: %w", errWrite)
	}

	return nil
}

func validateRemoteServerPath(relativePath string) (string, error) {
	normalizedPath := strings.ReplaceAll(strings.TrimSpace(relativePath), `\`, "/")
	if strings.HasPrefix(normalizedPath, "//") {
		return "", ErrInvalidPath
	}

	normalizedPath = strings.TrimPrefix(normalizedPath, "/")
	cleanedPath := path.Clean(normalizedPath)
	if cleanedPath == "." {
		return "", nil
	}
	if cleanedPath == ".." || strings.HasPrefix(cleanedPath, "../") {
		return "", ErrInvalidPath
	}
	if strings.HasPrefix(cleanedPath, "/") {
		return "", ErrInvalidPath
	}
	if hasWindowsDrivePrefix(cleanedPath) {
		return "", ErrInvalidPath
	}

	return cleanedPath, nil
}

func sanitizeRemoteUploadFileName(fileName string) (string, error) {
	normalizedName := strings.ReplaceAll(strings.TrimSpace(fileName), `\`, "/")
	sanitizedFileName := path.Base(normalizedName)
	if sanitizedFileName == "." || sanitizedFileName == ".." || sanitizedFileName == "" {
		log.Error().Str("fileName", fileName).Msg("Invalid file name")
		return "", ErrInvalidPath
	}
	if hasWindowsDrivePrefix(sanitizedFileName) {
		return "", ErrInvalidPath
	}
	return sanitizedFileName, nil
}

func remoteFileProtectionPolicy(gameServer *models.GameServer) node.ProtectionPolicy {
	return node.ProtectionPolicy{
		ServerExecutable: gameServer.ServerExecutable.GetOr(""),
		BaseCommand:      baseCommandForProtectedPath(gameServer),
	}
}

// GetGameServerFile streams a game server file to the provided writer.
func (inst *Instance) GetGameServerFile(gameServer *models.GameServer, relativePath string, writer io.Writer, setHeaders, setAsAttachment bool) error {
	if inst.isRemoteGameServer(gameServer) {
		return inst.getRemoteGameServerFile(gameServer, relativePath, writer, setHeaders, setAsAttachment)
	}

	fullPath, errResolve := node.ResolveExistingWithinRoot(gameServer.Directory, relativePath)
	if errResolve != nil {
		if errors.Is(errResolve, node.ErrInvalidPath) {
			return ErrInvalidPath
		}
		return fmt.Errorf("actions: resolve game server file: %w", errResolve)
	}

	file, errReadFile := os.Open(fullPath) // #nosec G703 -- ResolveExistingWithinRoot rejects traversal and escaping symlinks.
	if errReadFile != nil {
		if errors.Is(errReadFile, os.ErrNotExist) {
			log.Error().Err(errReadFile).Msg("File does not exist")
			return fmt.Errorf("actions: open game server file: %w", errReadFile)
		}
		log.Error().Err(errReadFile).Msg("Failed to read file")
		return fmt.Errorf("actions: open game server file: %w", errReadFile)
	}
	defer func() { _ = file.Close() }()

	fileInfo, errFileInfo := file.Stat()
	if errFileInfo != nil {
		log.Error().Err(errFileInfo).Msg("Failed to get file info")
		return fmt.Errorf("actions: stat game server file: %w", errFileInfo)
	}

	if setHeaders {
		w, ok := writer.(http.ResponseWriter)
		if !ok {
			log.Error().Msg("Writer is not an http.ResponseWriter")
			return errors.New("writer is not an http.ResponseWriter")
		}

		sniffBuf := make([]byte, 512)
		sniffed, errSniff := file.Read(sniffBuf)
		_, errSeek := file.Seek(0, io.SeekStart)
		if (errSniff != nil && !errors.Is(errSniff, io.EOF)) || errSeek != nil {
			w.Header().Set("Content-Type", "application/octet-stream")
		} else {
			w.Header().Set("Content-Type", http.DetectContentType(sniffBuf[:sniffed]))
		}

		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size(), 10))
		if setAsAttachment {
			// Sanitize filename to prevent header injection via quotes or newlines.
			safeName := strings.Map(func(r rune) rune {
				if r == '"' || r == '\\' || r == '\n' || r == '\r' {
					return '_'
				}
				return r
			}, fileInfo.Name())
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		}
	}

	_, errCopy := io.Copy(writer, file)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy file")
		return fmt.Errorf("actions: stream game server file: %w", errCopy)
	}
	return nil
}

func (inst *Instance) getRemoteGameServerFile(gameServer *models.GameServer, relativePath string, writer io.Writer, setHeaders, setAsAttachment bool) error {
	validatedPath, errPath := validateRemoteServerPath(relativePath)
	if errPath != nil {
		return errPath
	}

	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve node client for game server file: %w", errClient)
	}

	fileInfo, errStat := client.StatFile(inst.ctx, gameServer.Directory, validatedPath)
	if errStat != nil {
		return fmt.Errorf("actions: stat remote game server file: %w", errStat)
	}

	fileReader, errOpen := client.StreamFile(inst.ctx, gameServer.Directory, validatedPath)
	if errOpen != nil {
		return fmt.Errorf("actions: stream remote game server file: %w", errOpen)
	}
	defer func() {
		errClose := fileReader.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close remote game server file stream")
		}
	}()

	if setHeaders {
		w, ok := writer.(http.ResponseWriter)
		if !ok {
			log.Error().Msg("Writer is not an http.ResponseWriter")
			return errors.New("writer is not an http.ResponseWriter")
		}

		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Length", strconv.FormatInt(fileInfo.Size, 10))
		if setAsAttachment {
			safeName := sanitizeDownloadFileName(fileInfo.Name)
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safeName))
		}
	}

	_, errCopy := io.Copy(writer, fileReader)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy remote file")
		return fmt.Errorf("actions: stream remote game server file: %w", errCopy)
	}
	return nil
}

func sanitizeDownloadFileName(fileName string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r == '\n' || r == '\r' {
			return '_'
		}
		return r
	}, fileName)
}

func slugifyName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	var builder strings.Builder
	prevHyphen := false
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			builder.WriteRune(r)
			prevHyphen = false
		case r == ' ' || r == '_' || r == '-':
			if !prevHyphen && builder.Len() > 0 {
				builder.WriteByte('-')
				prevHyphen = true
			}
		}
	}
	return strings.Trim(builder.String(), "-")
}

func (inst *Instance) createGameServerDirectory(gameServer *models.GameServer, owner *models.User) (string, error) {
	gsNameSlug := slugifyName(gameServer.Name)

	// In a hub-spoke deployment the target node may run a different OS and
	// different env from the controller, so the install root and path
	// separator must be resolved on the node. For self / in-process nodes
	// this still works: the in-process client's GetNodeSnapshot also reports
	// DefaultInstallPath via the shared internal/node resolver.
	client, errClient := inst.resolveNodeClient(gameServer.NodeID)
	if errClient != nil {
		return "", fmt.Errorf("actions: resolve node client for install dir: %w", errClient)
	}

	snap, errSnap := client.GetNodeSnapshot(inst.ctx)
	if errSnap != nil || snap == nil {
		return "", fmt.Errorf("actions: get node snapshot for install dir: %w", errSnap)
	}

	installRoot := strings.TrimSpace(snap.DefaultInstallPath)
	if installRoot == "" {
		log.Error().Str("node_id", gameServer.NodeID).Str("node_os", snap.OS).
			Msg("target node did not report a default install path; check HOME / USERPROFILE on the node")
		return "", errors.New("actions: target node has no default install path — set HOME (Linux) or USERPROFILE (Windows) for the xylona-node process")
	}

	nodeOS, _ := detectOperatingSystem(strings.ToLower(strings.TrimSpace(snap.OS)))

	// Build the relative path with forward slashes — the node's
	// CreateFileOrDirectory handler accepts them on both Linux and Windows
	// (filepath.IsLocal allows them) and normalizes to the host separator.
	relativePath := path.Join(owner.UserName, gsNameSlug)

	errCreate := client.CreateFileOrDirectory(inst.ctx, installRoot, relativePath, "", true, node.ProtectionPolicy{})
	if errCreate != nil {
		log.Error().Err(errCreate).Str("node_id", gameServer.NodeID).
			Str("install_root", installRoot).Str("relative_path", relativePath).
			Msg("Failed to create game server directory on node")
		return "", fmt.Errorf("actions: create game server directory on node: %w", errCreate)
	}

	// Compose the full directory string with the target node's separator so
	// everything downstream (StartProcess.working_directory, protected-path
	// checks, post-install file writes) matches the node's native path
	// format. The controller's own filepath.Join uses the controller OS, so
	// join by hand for the heterogeneous case.
	return joinForNodeOS(nodeOS, installRoot, owner.UserName, gsNameSlug), nil
}

// joinForNodeOS composes a path with the target-node's native separator.
// Unlike filepath.Join (which uses the controller's separator), this lets a
// Windows controller produce a Linux-style path when the target node is
// Linux, and vice versa. nodeOS being unknown falls back to forward slashes
// (safe for Linux/Darwin and tolerated by Windows).
func joinForNodeOS(nodeOS OSType, parts ...string) string {
	sep := "/"
	if nodeOS == Windows {
		sep = "\\"
	}
	out := make([]string, 0, len(parts))
	for i, p := range parts {
		if i == 0 {
			// Keep the root as-is (preserves leading / on Unix or C:\ on Windows);
			// only trim trailing separators so we don't double-up when joining.
			p = strings.TrimRight(p, "\\/")
			if p == "" {
				continue
			}
			out = append(out, p)
			continue
		}
		p = strings.Trim(p, "\\/")
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, sep)
}

// PurgeAllGameServerFiles deletes the server's working directory.
func (inst *Instance) PurgeAllGameServerFiles(ctx context.Context, gameServer *models.GameServer) error {
	err := inst.deleteGameServerDirectory(ctx, gameServer.NodeID, gameServer.Directory)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete game server files")
		return fmt.Errorf("actions: delete game server files: %w", err)
	}
	return nil
}

func (inst *Instance) deleteGameServerDirectory(ctx context.Context, nodeID string, directory string) error {
	if directory == "" {
		return nil
	}
	if inst == nil {
		return errors.New("actions: instance is not configured")
	}
	if ctx == nil {
		ctx = inst.actionContext()
	}

	client, errClient := inst.resolveNodeClient(nodeID)
	if errClient != nil {
		return fmt.Errorf("actions: resolve node client for directory delete: %w", errClient)
	}
	_, errDelete := client.DeleteFiles(ctx, directory, []string{""}, node.ProtectionPolicy{})
	if errDelete != nil {
		return fmt.Errorf("actions: delete game server directory on node: %w", errDelete)
	}

	return nil
}
