package games

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders/mojang"
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
	provider := mojang.New()
	ctx := context.Background()
	latest, errLatest := provider.CheckForUpdate(ctx, "", "")
	if errLatest != nil {
		log.Error().Err(errLatest).Msg("Failed to get latest server URL")
		return fmt.Errorf("get latest minecraft server URL: %w", errLatest)
	}
	if latest == nil {
		return errors.New("get latest minecraft server URL: no release available")
	}
	_, _ = stdOutWriter.Write([]byte("Downloading latest server\n"))
	_, errDownload := provider.Download(ctx, "", latest.VersionID, gameServer.Directory)
	if errDownload != nil {
		log.Error().Err(errDownload).Msg("Failed to get latest server")
		return fmt.Errorf("download latest minecraft server: %w", errDownload)
	}
	return nil
}
