// Package palworldmap downloads and serves optional Palworld map imagery.
package palworldmap

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"golang.org/x/sync/errgroup"
)

const (
	// TilePathPrefix is the public same-origin path under which installed tiles
	// are served by the controller.
	TilePathPrefix = "/palworld-map-tiles"

	maxTileBytes        = 8 << 20
	downloadConcurrency = 4
	requestTimeout      = 30 * time.Second
)

// Layer describes one locally installed Palworld map tile pyramid.
type Layer struct {
	ID          string
	Label       string
	BaseURL     string
	Attribution string
	MinZoom     int32
	MaxZoom     int32
	TileSize    int32
	TransformA  float64
	TransformB  float64
	TransformC  float64
	TransformD  float64
	MinX        float64
	MinY        float64
	MaxX        float64
	MaxY        float64
}

var builtInLayers = []Layer{
	{
		ID:          "default",
		Label:       "Palpagos",
		BaseURL:     "https://cdn.th.gl/palworld/map-tiles/default-733001e0986faa3f88b0a970412d7fb9",
		Attribution: "Palworld © Pocketpair; map tiles sourced from th.gl",
		MinZoom:     0,
		MaxZoom:     4,
		TileSize:    512,
		TransformA:  0.000353395913859746,
		TransformB:  256,
		TransformC:  -0.000353395913859746,
		TransformD:  123.47653230259525,
		MinX:        -1099399,
		MinY:        -724399,
		MaxX:        349399,
		MaxY:        724399,
	},
	{
		ID:          "tree",
		Label:       "World Tree",
		BaseURL:     "https://cdn.th.gl/palworld/map-tiles/tree-bd046c3cfb06ee41b25a111f912d407f",
		Attribution: "Palworld © Pocketpair; map tiles sourced from th.gl",
		MinZoom:     0,
		MaxZoom:     4,
		TileSize:    512,
		TransformA:  0.0014979651664584533,
		TransformB:  1225.6306053008072,
		TransformC:  -0.0014979651664584533,
		TransformD:  1032.3204475170935,
		MinX:        347352.5,
		MinY:        -818196,
		MaxX:        689147.5,
		MaxY:        -476401,
	},
}

// Store owns the controller-local tile directory and its download lifecycle.
type Store struct {
	root   string
	client *http.Client
	layers []Layer
	mu     sync.Mutex
}

// NewStore creates a store rooted in the controller's persistent directory.
func NewStore(root string) (*Store, error) {
	client := &http.Client{Timeout: requestTimeout}
	return newStoreWithConfig(root, client, builtInLayers)
}

func newStoreWithConfig(root string, client *http.Client, layers []Layer) (*Store, error) {
	trimmedRoot := strings.TrimSpace(root)
	if trimmedRoot == "" {
		return nil, errors.New("palworld map tile directory is required")
	}
	absRoot, errAbs := filepath.Abs(trimmedRoot)
	if errAbs != nil {
		return nil, fmt.Errorf("palworld map: resolve tile directory: %w", errAbs)
	}
	if client == nil {
		return nil, errors.New("palworld map HTTP client is required")
	}
	if len(layers) == 0 {
		return nil, errors.New("at least one Palworld map layer is required")
	}

	storeLayers := make([]Layer, len(layers))
	copy(storeLayers, layers)
	return &Store{
		root:   filepath.Clean(absRoot),
		client: client,
		layers: storeLayers,
	}, nil
}

// Root returns the resolved local tile directory for startup diagnostics.
func (s *Store) Root() string {
	return s.root
}

// Layers returns the fixed local layer definitions used by the map renderer.
func (s *Store) Layers() []Layer {
	layers := make([]Layer, len(s.layers))
	copy(layers, s.layers)
	return layers
}

// TileURLTemplate returns the same-origin URL template for an installed layer.
func TileURLTemplate(layerID string) string {
	return TilePathPrefix + "/" + layerID + "/{z}/{x}/{y}.webp"
}

// Install downloads missing or invalid tiles for every built-in layer. Remote
// THGL paths use z/y/x; files are stored locally in the conventional z/x/y
// layout used by TileURLTemplate.
func (s *Store) Install(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	errMkdir := os.MkdirAll(s.root, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("palworld map: create tile directory: %w", errMkdir)
	}

	group, groupContext := errgroup.WithContext(ctx)
	group.SetLimit(downloadConcurrency)
	for _, layer := range s.layers {
		for zoom := layer.MinZoom; zoom <= layer.MaxZoom; zoom++ {
			tileCount := 1 << zoom
			for tileX := range tileCount {
				for tileY := range tileCount {
					group.Go(func() error {
						return s.installTile(groupContext, layer, zoom, tileX, tileY)
					})
				}
			}
		}
	}
	errInstall := group.Wait()
	if errInstall != nil {
		return fmt.Errorf("palworld map: install tiles: %w", errInstall)
	}
	return nil
}

func (s *Store) installTile(ctx context.Context, layer Layer, zoom int32, tileX int, tileY int) error {
	destinationDirectory := filepath.Join(s.root, layer.ID, strconv.FormatInt(int64(zoom), 10), strconv.Itoa(tileX))
	destinationPath := filepath.Join(destinationDirectory, strconv.Itoa(tileY)+".webp")
	valid, errValid := validWebPFile(destinationPath)
	if errValid != nil {
		return fmt.Errorf("check %s tile %d/%d/%d: %w", layer.ID, zoom, tileX, tileY, errValid)
	}
	if valid {
		return nil
	}

	// The source serves row before column, unlike the local XYZ layout.
	sourceURL := fmt.Sprintf("%s/%d/%d/%d.webp", strings.TrimRight(layer.BaseURL, "/"), zoom, tileY, tileX)
	request, errRequest := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if errRequest != nil {
		return fmt.Errorf("create %s tile request: %w", layer.ID, errRequest)
	}
	request.Header.Set("User-Agent", "Xylona-Palworld-Map/1.0")

	response, errResponse := s.client.Do(request)
	if errResponse != nil {
		return fmt.Errorf("download %s tile %d/%d/%d: %w", layer.ID, zoom, tileX, tileY, errResponse)
	}
	body, errRead := io.ReadAll(io.LimitReader(response.Body, maxTileBytes+1))
	errCloseBody := response.Body.Close()
	if response.StatusCode != http.StatusOK {
		if errCloseBody != nil {
			return fmt.Errorf("download %s tile %d/%d/%d: unexpected HTTP status %s; close response: %w", layer.ID, zoom, tileX, tileY, response.Status, errCloseBody)
		}
		return fmt.Errorf("download %s tile %d/%d/%d: unexpected HTTP status %s", layer.ID, zoom, tileX, tileY, response.Status)
	}
	if errRead != nil {
		return fmt.Errorf("read %s tile %d/%d/%d: %w", layer.ID, zoom, tileX, tileY, errRead)
	}
	if errCloseBody != nil {
		return fmt.Errorf("close %s tile %d/%d/%d response: %w", layer.ID, zoom, tileX, tileY, errCloseBody)
	}
	if len(body) > maxTileBytes {
		return fmt.Errorf("download %s tile %d/%d/%d: response exceeds %d bytes", layer.ID, zoom, tileX, tileY, maxTileBytes)
	}
	if !isWebP(body) {
		return fmt.Errorf("download %s tile %d/%d/%d: response is not WebP", layer.ID, zoom, tileX, tileY)
	}

	errMkdir := os.MkdirAll(destinationDirectory, 0o750)
	if errMkdir != nil {
		return fmt.Errorf("create %s tile directory: %w", layer.ID, errMkdir)
	}
	temporary, errCreate := os.CreateTemp(destinationDirectory, ".tile-*.part")
	if errCreate != nil {
		return fmt.Errorf("create %s tile temporary file: %w", layer.ID, errCreate)
	}
	temporaryPath := temporary.Name()
	keepTemporary := false
	defer func() {
		if keepTemporary {
			return
		}
		errRemove := os.Remove(temporaryPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("path", temporaryPath).Msg("Failed to remove incomplete Palworld map tile")
		}
	}()

	errChmod := temporary.Chmod(0o640)
	if errChmod != nil {
		errClose := temporary.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("path", temporaryPath).Msg("Failed to close Palworld map tile after chmod failure")
		}
		return fmt.Errorf("set %s tile permissions: %w", layer.ID, errChmod)
	}
	bytesWritten, errWrite := temporary.Write(body)
	if errWrite == nil && bytesWritten != len(body) {
		errWrite = io.ErrShortWrite
	}
	if errWrite != nil {
		errClose := temporary.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("path", temporaryPath).Msg("Failed to close incomplete Palworld map tile")
		}
		return fmt.Errorf("write %s tile: %w", layer.ID, errWrite)
	}
	errClose := temporary.Close()
	if errClose != nil {
		return fmt.Errorf("close %s tile: %w", layer.ID, errClose)
	}

	errRename := os.Rename(temporaryPath, destinationPath)
	if errRename != nil {
		errRemoveDestination := os.Remove(destinationPath)
		if errRemoveDestination != nil && !errors.Is(errRemoveDestination, os.ErrNotExist) {
			return fmt.Errorf("replace %s tile: %w", layer.ID, errRemoveDestination)
		}
		errRename = os.Rename(temporaryPath, destinationPath)
		if errRename != nil {
			return fmt.Errorf("promote %s tile: %w", layer.ID, errRename)
		}
	}
	keepTemporary = true
	return nil
}

func validWebPFile(path string) (bool, error) {
	file, errOpen := os.Open(path)
	if errors.Is(errOpen, os.ErrNotExist) {
		return false, nil
	}
	if errOpen != nil {
		return false, fmt.Errorf("open tile for validation: %w", errOpen)
	}
	fileInfo, errStat := file.Stat()
	if errStat != nil {
		errClose := file.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("path", path).Msg("Failed to close Palworld map tile after stat failure")
		}
		return false, fmt.Errorf("stat tile for validation: %w", errStat)
	}
	header := make([]byte, 12)
	_, errRead := io.ReadFull(file, header)
	errClose := file.Close()
	if errRead != nil && !errors.Is(errRead, io.ErrUnexpectedEOF) {
		return false, fmt.Errorf("read tile header: %w", errRead)
	}
	if errClose != nil {
		return false, fmt.Errorf("close validated tile: %w", errClose)
	}
	validSize := int64(binary.LittleEndian.Uint32(header[4:8]))+8 == fileInfo.Size()
	return hasWebPHeader(header) && validSize, nil
}

func isWebP(data []byte) bool {
	if !hasWebPHeader(data) {
		return false
	}
	return uint64(binary.LittleEndian.Uint32(data[4:8]))+8 == uint64(len(data))
}

func hasWebPHeader(data []byte) bool {
	return len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP"
}

// Handler serves only valid built-in tile coordinates and never exposes a
// directory listing or temporary download files.
func (s *Store) Handler() http.Handler {
	return http.HandlerFunc(func(responseWriter http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet && request.Method != http.MethodHead {
			responseWriter.Header().Set("Allow", "GET, HEAD")
			http.Error(responseWriter, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		layer, zoom, tileX, tileY, valid := s.parseTilePath(request.URL.Path)
		if !valid {
			http.NotFound(responseWriter, request)
			return
		}
		tilePath := filepath.Join(s.root, layer.ID, strconv.FormatInt(zoom, 10), strconv.FormatInt(tileX, 10), strconv.FormatInt(tileY, 10)+".webp")
		file, errOpen := os.Open(tilePath) // #nosec G703 -- parseTilePath returns only configured layer IDs and bounded integer coordinates.
		if errors.Is(errOpen, os.ErrNotExist) {
			http.NotFound(responseWriter, request)
			return
		}
		if errOpen != nil {
			log.Error().Err(errOpen).Str("path", tilePath).Msg("Failed to open Palworld map tile")
			http.Error(responseWriter, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		fileInfo, errStat := file.Stat()
		if errStat != nil || !fileInfo.Mode().IsRegular() {
			errClose := file.Close()
			if errClose != nil {
				log.Warn().Err(errClose).Str("path", tilePath).Msg("Failed to close invalid Palworld map tile")
			}
			http.NotFound(responseWriter, request)
			return
		}

		responseWriter.Header().Set("Cache-Control", "public, max-age=86400")
		responseWriter.Header().Set("Content-Type", "image/webp")
		http.ServeContent(responseWriter, request, fileInfo.Name(), fileInfo.ModTime(), file)
		errClose := file.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Str("path", tilePath).Msg("Failed to close served Palworld map tile")
		}
	})
}

func (s *Store) parseTilePath(requestPath string) (Layer, int64, int64, int64, bool) {
	segments := strings.Split(strings.Trim(requestPath, "/"), "/")
	if len(segments) != 4 || !strings.HasSuffix(segments[3], ".webp") {
		return Layer{}, 0, 0, 0, false
	}
	layer, found := s.layerByID(segments[0])
	if !found {
		return Layer{}, 0, 0, 0, false
	}
	zoom, errZoom := strconv.ParseInt(segments[1], 10, 32)
	tileX, errTileX := strconv.ParseInt(segments[2], 10, 32)
	tileY, errTileY := strconv.ParseInt(strings.TrimSuffix(segments[3], ".webp"), 10, 32)
	if errZoom != nil || errTileX != nil || errTileY != nil || zoom < int64(layer.MinZoom) || zoom > int64(layer.MaxZoom) {
		return Layer{}, 0, 0, 0, false
	}
	tileCount := int64(1) << zoom
	if tileX < 0 || tileX >= tileCount || tileY < 0 || tileY >= tileCount {
		return Layer{}, 0, 0, 0, false
	}
	return layer, zoom, tileX, tileY, true
}

func (s *Store) layerByID(layerID string) (Layer, bool) {
	for _, layer := range s.layers {
		if layer.ID == layerID {
			return layer, true
		}
	}
	return Layer{}, false
}
