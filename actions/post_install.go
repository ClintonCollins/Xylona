package actions

import (
	"io"
	"os"
	"path"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func postMinecraftInstall(gameServer *models.GameServer) error {
	dir := gameServer.Directory
	f, errFile := os.Create(path.Join(dir, "eula.txt"))
	if errFile != nil {
		log.Error().Err(errFile).Msg("Failed to open eula.txt")
		return errFile
	}
	defer func() {
		errClose := f.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close eula.txt")
		}
	}()
	_, errWrite := f.WriteString("eula=true")
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to write to eula.txt")
		return errWrite
	}
	return nil
}

func post7DaysToDieInstall(gameServer *models.GameServer) error {
	log.Debug().Msg("Copying serverconfig.xml to settings.xml")
	readConfig, errRead := os.Open(path.Join(gameServer.Directory, "serverconfig.xml"))
	if errRead != nil {
		log.Error().Err(errRead).Msg("Failed to open serverconfig.xml")
		return errRead
	}
	defer func() {
		errClose := readConfig.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close serverconfig.xml")
		}
	}()
	writeConfig, errWrite := os.Create(path.Join(gameServer.Directory, "settings.xml"))
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to open settings.xml")
		return errWrite
	}
	defer func() {
		errClose := writeConfig.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close settings.xml")
		}
	}()
	_, errCopy := io.Copy(writeConfig, readConfig)
	if errCopy != nil {
		log.Error().Err(errCopy).Msg("Failed to copy serverconfig.xml to settings.xml")
		return errCopy
	}

	return nil
}
