package games

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/helpers"
	"github.com/ClintonCollins/Xylona/pkg/minecraft"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// Minecraft implements the internal installer and updater for Minecraft servers.
type Minecraft struct {
}

// Install downloads the latest Minecraft server jar into the game server directory.
func (m *Minecraft) Install(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	return downloadLatestMinecraftServerJar(gameServer, stdOutWriter)
}

// Update downloads the latest Minecraft server jar into the game server directory.
func (m *Minecraft) Update(gameServer *models.GameServer, stdOutWriter, _ io.Writer) error {
	return downloadLatestMinecraftServerJar(gameServer, stdOutWriter)
}

func downloadLatestMinecraftServerJar(gameServer *models.GameServer, stdOutWriter io.Writer) error {
	latestServerURL, errGetURL := minecraft.GetLatestServerDownloadURL()
	if errGetURL != nil {
		log.Error().Err(errGetURL).Msg("Failed to get latest server URL")
		return fmt.Errorf("get latest minecraft server URL: %w", errGetURL)
	}
	httpClient := helpers.GetXylonaHTTPClient()
	req, errReq := http.NewRequestWithContext(context.Background(), http.MethodGet, latestServerURL, nil)
	if errReq != nil {
		log.Error().Err(errReq).Msg("Failed to create latest server request")
		return fmt.Errorf("create minecraft server download request: %w", errReq)
	}
	response, errGet := httpClient.Do(req)
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get latest server")
		return fmt.Errorf("download latest minecraft server: %w", errGet)
	}
	defer func() {
		errClose := response.Body.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close Minecraft server download response body")
		}
	}()
	_, _ = stdOutWriter.Write([]byte("Downloading latest server\n"))

	f, errCreate := os.Create(filepath.Join(gameServer.Directory, "minecraft_server.jar"))
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create server file")
		return fmt.Errorf("create minecraft server jar: %w", errCreate)
	}
	defer func() {
		errClose := f.Close()
		if errClose != nil {
			log.Warn().Err(errClose).Msg("Failed to close Minecraft server jar file")
		}
	}()

	_, errCopy := io.Copy(f, response.Body)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy server file")
		return fmt.Errorf("write minecraft server jar: %w", errCopy)
	}

	return nil
}
