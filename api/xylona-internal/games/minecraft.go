package games

import (
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/helpers"
	"github.com/ClintonCollins/Xylona/pkg/minecraft"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type Minecraft struct {
}

func (m *Minecraft) Install(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error {
	latestServerURL, errGetURL := minecraft.GetLatestServerDownloadURL()
	if errGetURL != nil {
		log.Error().Err(errGetURL).Msg("Failed to get latest server URL")
		return errGetURL
	}
	httpClient := helpers.GetXylonaHTTPClient()
	response, errGet := httpClient.Get(latestServerURL)
	if errGet != nil {
		log.Error().Err(errGet).Msg("Failed to get latest server")
		return errGet
	}
	defer func() {
		_ = response.Body.Close()
	}()
	_, _ = stdOutWriter.Write([]byte("Downloading latest server\n"))

	f, errCreate := os.Create(filepath.Join(gameServer.Directory, "minecraft_server.jar"))
	if errCreate != nil {
		log.Error().Err(errCreate).Msg("Failed to create server file")
		return errCreate
	}
	defer func() {
		_ = f.Close()
	}()

	_, errCopy := io.Copy(f, response.Body)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy server file")
		return errCopy
	}

	return nil
}

func (m *Minecraft) Update(gameServer *models.GameServer, stdOutWriter, stdErrWriter io.Writer) error {
	return nil
}
