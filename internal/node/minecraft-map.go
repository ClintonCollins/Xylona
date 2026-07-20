package node

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node/supervisor"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

const (
	blueMapManagedDirectory = ".xylona/bluemap"
	blueMapMaximumAssetSize = 32 << 20
	blueMapRestartCooldown  = 30 * time.Second
	blueMapLiveCacheTTL     = time.Second
	blueMapStopTimeout      = 25 * time.Second
)

type blueMapRelease struct {
	version string
	url     string
	sha256  string
	size    int64
}

var (
	blueMapReleaseLegacy = blueMapRelease{
		version: "5.16",
		url:     "https://github.com/BlueMap-Minecraft/BlueMap/releases/download/v5.16/bluemap-5.16-cli.jar",
		sha256:  "7940d561890373897f8f6be91a52e765461f40e5be4e1c4401004073ee0d2580",
		size:    6563805,
	}
	blueMapReleaseCurrent = blueMapRelease{
		version: "5.20",
		url:     "https://github.com/BlueMap-Minecraft/BlueMap/releases/download/v5.20/bluemap-5.20-cli.jar",
		sha256:  "b710cdfef1b8dbd0474f4f53ad207cb9086607c8abae6bef3975f8397dce482d",
		size:    6584438,
	}
	blueMapPositionPattern  = regexp.MustCompile(`(?m)^([A-Za-z0-9_]{1,16}) has the following entity data: \[([-+0-9.eE]+)[dDfF]?,\s*([-+0-9.eE]+)[dDfF]?,\s*([-+0-9.eE]+)[dDfF]?\]`)
	blueMapDimensionPattern = regexp.MustCompile(`(?m)^([A-Za-z0-9_]{1,16}) has the following entity data: "?([a-z0-9_.-]+:[a-z0-9_./-]+|-?[0-9]+)"?`)
)

type blueMapLivePlayers struct {
	Players []blueMapLivePlayer `json:"players"`
}

type blueMapLivePlayer struct {
	UUID     string              `json:"uuid"`
	Name     string              `json:"name"`
	Foreign  bool                `json:"foreign"`
	Position blueMapLivePosition `json:"position"`
	Rotation blueMapLiveRotation `json:"rotation"`
}

type blueMapLivePosition struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

type blueMapLiveRotation struct {
	Pitch float64 `json:"pitch"`
	Yaw   float64 `json:"yaw"`
	Roll  float64 `json:"roll"`
}

type minecraftUserCacheEntry struct {
	Name string `json:"name"`
	UUID string `json:"uuid"`
}

type minecraftPlayerPosition struct {
	name      string
	dimension string
	x         float64
	y         float64
	z         float64
}

type minecraftMapLiveCacheEntry struct {
	content   []byte
	expiresAt time.Time
}

// EnsureMinecraftMap starts Xylona's verified standalone BlueMap renderer for
// vanilla and modded worlds. Server-installed web assets are never exposed
// through the controller's authenticated origin.
func (n *Node) EnsureMinecraftMap(ctx context.Context, req MinecraftMapEnsureRequest) (MinecraftMapStatus, error) {
	processID := strings.TrimSpace(req.ProcessID)
	workingDirectory := filepath.Clean(strings.TrimSpace(req.WorkingDirectory))
	worldName, errWorldName := validateLocalPath(strings.TrimSpace(req.WorldName))
	if errWorldName != nil || worldName == "" {
		return MinecraftMapStatus{}, fmt.Errorf("node: minecraft map world name: %w", ErrInvalidPath)
	}
	if processID == "" || workingDirectory == "." || workingDirectory == "" {
		return MinecraftMapStatus{}, ErrInvalidPath
	}
	lifecycleLock := n.minecraftMapLifecycleLock(processID)
	lifecycleLock.Lock()
	defer lifecycleLock.Unlock()

	worldPath, errWorldPath := resolveWithinRoot(workingDirectory, worldName)
	if errWorldPath != nil {
		return MinecraftMapStatus{}, errWorldPath
	}
	worldInfo, errWorld := os.Stat(worldPath)
	if errors.Is(errWorld, os.ErrNotExist) {
		return MinecraftMapStatus{
			Provider:      "managed",
			Status:        "waiting_for_world",
			StatusMessage: "Start the Minecraft server once so its world can be created.",
		}, nil
	}
	if errWorld != nil {
		return MinecraftMapStatus{}, fmt.Errorf("node: inspect Minecraft world: %w", errWorld)
	}
	if !worldInfo.IsDir() {
		return MinecraftMapStatus{}, errors.New("node: minecraft map world is not a directory")
	}

	release := selectBlueMapRelease(req.MinecraftVersion)
	managedRoot := filepath.Join(workingDirectory, filepath.FromSlash(blueMapManagedDirectory))
	errDirectory := os.MkdirAll(managedRoot, 0o750)
	if errDirectory != nil {
		return MinecraftMapStatus{}, fmt.Errorf("node: create BlueMap directory: %w", errDirectory)
	}

	jarName := filepath.Base(release.url)
	jarPath := filepath.Join(managedRoot, jarName)
	installed, errInstalled := blueMapArtifactMatches(jarPath, release)
	if errInstalled != nil {
		return MinecraftMapStatus{}, errInstalled
	}
	if !installed {
		_, errDownload := n.DownloadFileFromURL(ctx, workingDirectory, release.url, blueMapManagedDirectory, DownloadIntegrity{
			ExpectedSize:   release.size,
			ExpectedSHA256: release.sha256,
		}, ProtectionPolicy{})
		if errDownload != nil {
			return MinecraftMapStatus{}, fmt.Errorf("node: download verified BlueMap %s: %w", release.version, errDownload)
		}
	}

	errConfig := writeManagedBlueMapConfig(managedRoot, worldPath)
	if errConfig != nil {
		return MinecraftMapStatus{}, errConfig
	}

	managedWebRoot := filepath.Join(managedRoot, "web")
	managedIndex := filepath.Join(managedWebRoot, "index.html")
	ready := fileExists(managedIndex)
	mapProcessID := minecraftMapProcessID(processID)
	snapshot, found, errSnapshot := n.GetProcessSnapshot(mapProcessID)
	if errSnapshot != nil {
		return MinecraftMapStatus{}, fmt.Errorf("node: inspect BlueMap process: %w", errSnapshot)
	}
	running := found && snapshot != nil && snapshot.Status != xylona.Status_OFFLINE.String()
	if running {
		return managedBlueMapStatus(release.version, ready, true), nil
	}
	if found && snapshot != nil && time.Since(time.Unix(snapshot.UnixStartedAt, 0)) < blueMapRestartCooldown {
		status := managedBlueMapStatus(release.version, ready, false)
		status.Status = "failed"
		status.StatusMessage = "BlueMap stopped unexpectedly. Xylona will retry shortly."
		return status, nil
	}

	javaExecutable := strings.TrimSpace(req.JavaExecutable)
	if javaExecutable == "" {
		javaExecutable = "java"
	}
	configDirectory := filepath.Join(managedRoot, "config")
	args := []string{
		"-jar", jarPath,
		"-c", configDirectory,
		"-r", "-u", "-g",
	}
	modsDirectory := filepath.Join(workingDirectory, "mods")
	modsInfo, errMods := os.Stat(modsDirectory)
	if errMods == nil && modsInfo.IsDir() {
		args = append(args, "-n", modsDirectory)
	}
	if errMods != nil && !errors.Is(errMods, os.ErrNotExist) {
		return MinecraftMapStatus{}, fmt.Errorf("node: inspect Minecraft mods directory: %w", errMods)
	}

	_, errStart := n.StartProcess(ProcessConfig{
		ID:                   mapProcessID,
		Name:                 "Minecraft live map",
		BaseCommand:          javaExecutable,
		Args:                 args,
		WorkingDirectory:     workingDirectory,
		ServiceID:            "minecraft-map",
		StopTimeout:          20 * time.Second,
		SuppressStatusEvents: true,
	}, xylona.Status_ONLINE)
	if errors.Is(errStart, supervisor.ErrCommandAlreadyRunning) {
		return managedBlueMapStatus(release.version, ready, true), nil
	}
	if errStart != nil {
		return MinecraftMapStatus{}, fmt.Errorf("node: start BlueMap companion: %w", errStart)
	}
	return managedBlueMapStatus(release.version, ready, true), nil
}

// StopMinecraftMap stops the Xylona-managed BlueMap companion and waits until
// it has released the server directory.
func (n *Node) StopMinecraftMap(ctx context.Context, processID string) error {
	processID = strings.TrimSpace(processID)
	if processID == "" {
		return ErrInvalidPath
	}
	lifecycleLock := n.minecraftMapLifecycleLock(processID)
	lifecycleLock.Lock()
	defer lifecycleLock.Unlock()

	mapProcessID := minecraftMapProcessID(processID)
	errStop := n.StopProcess(mapProcessID, "")
	if errors.Is(errStop, ErrProcessNotFound) {
		n.clearMinecraftLivePlayerCache(processID)
		return nil
	}
	if errStop != nil {
		return fmt.Errorf("node: stop Minecraft map: %w", errStop)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx, cancel := context.WithTimeout(ctx, blueMapStopTimeout)
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot, found, errSnapshot := n.GetProcessSnapshot(mapProcessID)
		if errSnapshot != nil {
			return fmt.Errorf("node: confirm Minecraft map stopped: %w", errSnapshot)
		}
		if !found || snapshot == nil || snapshot.Status == xylona.Status_OFFLINE.String() {
			n.clearMinecraftLivePlayerCache(processID)
			return nil
		}
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("node: wait for Minecraft map to stop: %w", waitCtx.Err())
		case <-ticker.C:
		}
	}
}

// GetMinecraftMapAsset returns a bounded BlueMap web asset. Managed live
// player requests are synthesized from the server's local RCON transport.
func (n *Node) GetMinecraftMapAsset(ctx context.Context, req MinecraftMapAssetRequest) (MinecraftMapAsset, error) {
	workingDirectory := filepath.Clean(strings.TrimSpace(req.WorkingDirectory))
	if workingDirectory == "." || workingDirectory == "" {
		return MinecraftMapAsset{}, ErrInvalidPath
	}

	managedRoot := filepath.Join(workingDirectory, filepath.FromSlash(blueMapManagedDirectory), "web")
	webRoot := managedRoot

	assetPath := strings.TrimPrefix(strings.TrimSpace(req.AssetPath), "/")
	if assetPath == "" {
		assetPath = "index.html"
	}
	validatedPath, errPath := validateLocalPath(filepath.FromSlash(assetPath))
	if errPath != nil {
		return MinecraftMapAsset{}, errPath
	}

	if isBlueMapPlayersAsset(filepath.ToSlash(validatedPath)) {
		mapID := blueMapMapID(filepath.ToSlash(validatedPath))
		content, errPlayers := n.minecraftLivePlayers(ctx, req.ProcessID, workingDirectory, mapID)
		if errPlayers == nil {
			return MinecraftMapAsset{
				Content:      content,
				ContentType:  "application/json; charset=utf-8",
				CacheControl: "no-store",
			}, nil
		}
		log.Debug().Err(errPlayers).Str("process_id", req.ProcessID).Msg("node: Minecraft live players unavailable")
	}
	if isBlueMapPlayerHeadAsset(filepath.ToSlash(validatedPath)) {
		validatedPath = filepath.FromSlash("assets/steve.png")
	}

	fullPath, errResolve := resolveWithinRoot(webRoot, validatedPath)
	if errResolve != nil {
		return MinecraftMapAsset{}, errResolve
	}
	contentEncoding := ""
	info, errStat := os.Stat(fullPath)
	if errors.Is(errStat, os.ErrNotExist) {
		gzipPath := fullPath + ".gz"
		gzipInfo, errGzip := os.Stat(gzipPath)
		if errGzip == nil {
			fullPath = gzipPath
			info = gzipInfo
			contentEncoding = "gzip"
			errStat = nil
		}
	}
	if errStat != nil {
		return MinecraftMapAsset{}, fmt.Errorf("node: stat BlueMap asset: %w", errStat)
	}
	if info.IsDir() || info.Size() > blueMapMaximumAssetSize {
		return MinecraftMapAsset{}, ErrInvalidPath
	}
	content, errRead := os.ReadFile(fullPath)
	if errRead != nil {
		return MinecraftMapAsset{}, fmt.Errorf("node: read BlueMap asset: %w", errRead)
	}

	contentType := blueMapContentType(validatedPath)
	cacheControl := "public, max-age=300"
	cleanAsset := filepath.ToSlash(validatedPath)
	if cleanAsset == "index.html" || strings.HasSuffix(cleanAsset, "/settings.json") || strings.HasSuffix(cleanAsset, "/maps.json") || strings.Contains(cleanAsset, "/live/") {
		cacheControl = "no-store"
	} else if strings.HasPrefix(cleanAsset, "assets/") {
		cacheControl = "public, max-age=31536000, immutable"
	}
	return MinecraftMapAsset{
		Content:         content,
		ContentType:     contentType,
		ContentEncoding: contentEncoding,
		CacheControl:    cacheControl,
	}, nil
}

func selectBlueMapRelease(minecraftVersion string) blueMapRelease {
	version := strings.TrimSpace(strings.TrimPrefix(minecraftVersion, "v"))
	if strings.HasPrefix(version, "26.") {
		return blueMapReleaseCurrent
	}
	return blueMapReleaseLegacy
}

func blueMapArtifactMatches(path string, release blueMapRelease) (bool, error) {
	content, errRead := os.ReadFile(path)
	if errors.Is(errRead, os.ErrNotExist) {
		return false, nil
	}
	if errRead != nil {
		return false, fmt.Errorf("node: read BlueMap artifact: %w", errRead)
	}
	if int64(len(content)) != release.size {
		return false, nil
	}
	digest := sha256.Sum256(content)
	return strings.EqualFold(hex.EncodeToString(digest[:]), release.sha256), nil
}

func writeManagedBlueMapConfig(managedRoot string, worldPath string) error {
	configRoot := filepath.Join(managedRoot, "config")
	mapsRoot := filepath.Join(configRoot, "maps")
	storagesRoot := filepath.Join(configRoot, "storages")
	for _, directory := range []string{mapsRoot, storagesRoot, filepath.Join(managedRoot, "data"), filepath.Join(managedRoot, "web", "maps")} {
		errDirectory := os.MkdirAll(directory, 0o750)
		if errDirectory != nil {
			return fmt.Errorf("node: create BlueMap config directory: %w", errDirectory)
		}
	}

	quotedWorld := quoteBlueMapPath(worldPath)
	configs := map[string]string{
		filepath.Join(configRoot, "core.conf"):      fmt.Sprintf("accept-download: true\ndata: %s\nrender-thread-count: 1\nscan-for-mod-resources: true\nmetrics: false\n", quoteBlueMapPath(filepath.Join(managedRoot, "data"))),
		filepath.Join(configRoot, "webapp.conf"):    fmt.Sprintf("enabled: true\nwebroot: %s\nupdate-settings-file: true\nuse-cookies: false\n", quoteBlueMapPath(filepath.Join(managedRoot, "web"))),
		filepath.Join(configRoot, "webserver.conf"): "enabled: false\n",
		filepath.Join(storagesRoot, "file.conf"):    fmt.Sprintf("storage-type: file\nroot: %s\ncompression: gzip\n", quoteBlueMapPath(filepath.Join(managedRoot, "web", "maps"))),
		filepath.Join(mapsRoot, "overworld.conf"):   blueMapMapConfig(quotedWorld, "minecraft:overworld", "Overworld", 0),
		filepath.Join(mapsRoot, "nether.conf"):      blueMapMapConfig(quotedWorld, "minecraft:the_nether", "Nether", 1),
		filepath.Join(mapsRoot, "end.conf"):         blueMapMapConfig(quotedWorld, "minecraft:the_end", "The End", 2),
	}
	for path, content := range configs {
		errWrite := writeBlueMapFile(path, []byte(content))
		if errWrite != nil {
			return errWrite
		}
	}
	return nil
}

func blueMapMapConfig(quotedWorld string, dimension string, name string, sorting int) string {
	return fmt.Sprintf("world: %s\ndimension: %q\nname: %q\nsorting: %d\nstorage: \"file\"\n", quotedWorld, dimension, name, sorting)
}

func quoteBlueMapPath(path string) string {
	return strconv.Quote(filepath.ToSlash(path))
}

func writeBlueMapFile(path string, content []byte) error {
	tempFile, errCreate := os.CreateTemp(filepath.Dir(path), ".bluemap-config-*")
	if errCreate != nil {
		return fmt.Errorf("node: create BlueMap config: %w", errCreate)
	}
	tempPath := tempFile.Name()
	defer func() {
		errRemove := os.Remove(tempPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("path", tempPath).Msg("node: remove BlueMap config temp file")
		}
	}()
	_, errWrite := tempFile.Write(content)
	if errWrite != nil {
		errClose := tempFile.Close()
		return errors.Join(fmt.Errorf("node: write BlueMap config: %w", errWrite), errClose)
	}
	errClose := tempFile.Close()
	if errClose != nil {
		return fmt.Errorf("node: close BlueMap config: %w", errClose)
	}
	errRename := os.Rename(tempPath, path)
	if errRename == nil {
		return nil
	}
	errRemove := os.Remove(path)
	if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
		return fmt.Errorf("node: replace BlueMap config: %w", errRemove)
	}
	errRename = os.Rename(tempPath, path)
	if errRename != nil {
		return fmt.Errorf("node: promote BlueMap config: %w", errRename)
	}
	return nil
}

func managedBlueMapStatus(version string, ready bool, running bool) MinecraftMapStatus {
	status := MinecraftMapStatus{
		Installed:            true,
		Running:              running,
		Ready:                ready,
		Provider:             "managed",
		Status:               "rendering",
		StatusMessage:        "BlueMap is rendering explored chunks. The map will fill in progressively.",
		BlueMapVersion:       version,
		LivePlayersAvailable: true,
	}
	if ready && running {
		status.Status = "ready"
		status.StatusMessage = "Live map online. Newly changed chunks render automatically."
	}
	return status
}

func minecraftMapProcessID(processID string) string {
	return strings.TrimSpace(processID) + ":minecraft-map"
}

func (n *Node) minecraftMapLifecycleLock(processID string) *sync.Mutex {
	lockValue, _ := n.minecraftMapLifecycleLocks.LoadOrStore(strings.TrimSpace(processID), &sync.Mutex{})
	lock, ok := lockValue.(*sync.Mutex)
	if !ok {
		return &sync.Mutex{}
	}
	return lock
}

func fileExists(path string) bool {
	info, errStat := os.Stat(path)
	return errStat == nil && !info.IsDir()
}

func isBlueMapPlayersAsset(assetPath string) bool {
	return strings.HasPrefix(assetPath, "maps/") && strings.HasSuffix(assetPath, "/live/players.json")
}

func isBlueMapPlayerHeadAsset(assetPath string) bool {
	return strings.HasPrefix(assetPath, "maps/") && strings.Contains(assetPath, "/assets/playerheads/") && strings.HasSuffix(assetPath, ".png")
}

func blueMapMapID(assetPath string) string {
	parts := strings.Split(assetPath, "/")
	if len(parts) < 4 {
		return ""
	}
	return parts[1]
}

func blueMapContentType(assetPath string) string {
	extension := filepath.Ext(strings.TrimSuffix(assetPath, ".gz"))
	switch strings.ToLower(extension) {
	case ".json":
		return "application/json; charset=utf-8"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".html":
		return "text/html; charset=utf-8"
	case ".svg":
		return "image/svg+xml"
	case ".prbm":
		return "application/octet-stream"
	default:
		contentType := mime.TypeByExtension(extension)
		if contentType != "" {
			return contentType
		}
		return "application/octet-stream"
	}
}

func (n *Node) minecraftLivePlayers(ctx context.Context, processID string, workingDirectory string, mapID string) ([]byte, error) {
	cacheKey := strings.TrimSpace(processID) + "|" + strings.TrimSpace(mapID)
	n.minecraftMapLiveMu.Lock()
	if n.minecraftMapLiveCache == nil {
		n.minecraftMapLiveCache = make(map[string]minecraftMapLiveCacheEntry)
	}
	entry, found := n.minecraftMapLiveCache[cacheKey]
	if found && time.Now().Before(entry.expiresAt) {
		content := append([]byte(nil), entry.content...)
		n.minecraftMapLiveMu.Unlock()
		return content, nil
	}
	n.minecraftMapLiveMu.Unlock()

	result, errRequest, _ := n.minecraftMapLiveRequests.Do(cacheKey, func() (any, error) {
		n.minecraftMapLiveMu.Lock()
		cachedEntry, cached := n.minecraftMapLiveCache[cacheKey]
		if cached && time.Now().Before(cachedEntry.expiresAt) {
			content := append([]byte(nil), cachedEntry.content...)
			n.minecraftMapLiveMu.Unlock()
			return content, nil
		}
		n.minecraftMapLiveMu.Unlock()

		content, errQuery := n.queryMinecraftLivePlayers(ctx, processID, workingDirectory, mapID)
		if errQuery != nil {
			return nil, errQuery
		}
		n.minecraftMapLiveMu.Lock()
		n.minecraftMapLiveCache[cacheKey] = minecraftMapLiveCacheEntry{
			content:   append([]byte(nil), content...),
			expiresAt: time.Now().Add(blueMapLiveCacheTTL),
		}
		n.minecraftMapLiveMu.Unlock()
		return content, nil
	})
	if errRequest != nil {
		return nil, fmt.Errorf("query Minecraft live players: %w", errRequest)
	}
	content, ok := result.([]byte)
	if !ok {
		return nil, errors.New("node: invalid Minecraft live-player cache result")
	}
	return append([]byte(nil), content...), nil
}

func (n *Node) clearMinecraftLivePlayerCache(processID string) {
	prefix := strings.TrimSpace(processID) + "|"
	n.minecraftMapLiveMu.Lock()
	defer n.minecraftMapLiveMu.Unlock()
	for cacheKey := range n.minecraftMapLiveCache {
		if strings.HasPrefix(cacheKey, prefix) {
			delete(n.minecraftMapLiveCache, cacheKey)
		}
	}
}

func (n *Node) queryMinecraftLivePlayers(ctx context.Context, processID string, workingDirectory string, mapID string) ([]byte, error) {
	if n.supervisor == nil {
		return nil, errors.New("node: supervisor not configured")
	}
	command, errCommand := n.supervisor.GetCommandByID(strings.TrimSpace(processID))
	if errCommand != nil {
		return marshalBlueMapLivePlayers([]blueMapLivePlayer{})
	}

	positionResponse, errPositions := command.ExecuteInput(ctx, "execute as @a run data get entity @s Pos")
	if errPositions != nil {
		return nil, fmt.Errorf("node: query Minecraft player positions: %w", errPositions)
	}
	dimensionResponse, errDimensions := command.ExecuteInput(ctx, "execute as @a run data get entity @s Dimension")
	if errDimensions != nil {
		return nil, fmt.Errorf("node: query Minecraft player dimensions: %w", errDimensions)
	}

	positions := parseMinecraftPlayerPositions(positionResponse, dimensionResponse)
	uuidByName := readMinecraftUserCache(workingDirectory)
	players := make([]blueMapLivePlayer, 0, len(positions))
	for _, position := range positions {
		if !blueMapPositionMatchesMap(position.dimension, mapID) {
			continue
		}
		playerUUID := uuidByName[strings.ToLower(position.name)]
		if playerUUID == "" {
			playerUUID = stableMinecraftPlayerUUID(position.name)
		}
		players = append(players, blueMapLivePlayer{
			UUID:    playerUUID,
			Name:    position.name,
			Foreign: false,
			Position: blueMapLivePosition{
				X: position.x,
				Y: position.y,
				Z: position.z,
			},
			Rotation: blueMapLiveRotation{},
		})
	}
	return marshalBlueMapLivePlayers(players)
}

func marshalBlueMapLivePlayers(players []blueMapLivePlayer) ([]byte, error) {
	content, errMarshal := json.Marshal(blueMapLivePlayers{Players: players})
	if errMarshal != nil {
		return nil, fmt.Errorf("node: marshal Minecraft live players: %w", errMarshal)
	}
	return content, nil
}

func parseMinecraftPlayerPositions(positionResponse string, dimensionResponse string) []minecraftPlayerPosition {
	dimensions := make(map[string]string)
	for _, match := range blueMapDimensionPattern.FindAllStringSubmatch(dimensionResponse, -1) {
		dimensions[match[1]] = normalizeMinecraftDimension(match[2])
	}
	positions := make([]minecraftPlayerPosition, 0)
	for _, match := range blueMapPositionPattern.FindAllStringSubmatch(positionResponse, -1) {
		x, errX := strconv.ParseFloat(match[2], 64)
		y, errY := strconv.ParseFloat(match[3], 64)
		z, errZ := strconv.ParseFloat(match[4], 64)
		if errX != nil || errY != nil || errZ != nil {
			continue
		}
		positions = append(positions, minecraftPlayerPosition{
			name:      match[1],
			dimension: dimensions[match[1]],
			x:         x,
			y:         y,
			z:         z,
		})
	}
	return positions
}

func normalizeMinecraftDimension(dimension string) string {
	switch strings.TrimSpace(dimension) {
	case "0":
		return "minecraft:overworld"
	case "-1":
		return "minecraft:the_nether"
	case "1":
		return "minecraft:the_end"
	default:
		return strings.TrimSpace(dimension)
	}
}

func blueMapPositionMatchesMap(dimension string, mapID string) bool {
	switch strings.ToLower(strings.TrimSpace(mapID)) {
	case "overworld", "world":
		return dimension == "minecraft:overworld" || dimension == ""
	case "nether", "the_nether", "world_nether":
		return dimension == "minecraft:the_nether"
	case "end", "the_end", "world_the_end":
		return dimension == "minecraft:the_end"
	default:
		return true
	}
}

func readMinecraftUserCache(workingDirectory string) map[string]string {
	content, errRead := os.ReadFile(filepath.Join(workingDirectory, "usercache.json"))
	if errRead != nil {
		return nil
	}
	var entries []minecraftUserCacheEntry
	errUnmarshal := json.Unmarshal(content, &entries)
	if errUnmarshal != nil {
		return nil
	}
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		result[strings.ToLower(entry.Name)] = entry.UUID
	}
	return result
}

func stableMinecraftPlayerUUID(name string) string {
	digest := sha256.Sum256([]byte("xylona:minecraft-player:" + strings.ToLower(name)))
	hexValue := hex.EncodeToString(digest[:16])
	return hexValue[0:8] + "-" + hexValue[8:12] + "-" + hexValue[12:16] + "-" + hexValue[16:20] + "-" + hexValue[20:32]
}
