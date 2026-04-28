package actions

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (inst *Instance) shouldUseRemoteNodeFiles(nodeID string) bool {
	if inst == nil || inst.nodeRegistry == nil {
		return false
	}

	trimmedNodeID := strings.TrimSpace(nodeID)
	if trimmedNodeID == "" {
		return false
	}

	selfNodeID := strings.TrimSpace(inst.nodeRegistry.SelfID())
	if selfNodeID == "" {
		return true
	}

	return trimmedNodeID != selfNodeID
}

func (inst *Instance) actionContext() context.Context {
	if inst != nil && inst.ctx != nil {
		return inst.ctx
	}
	return context.Background()
}

func (inst *Instance) post7DaysToDieInstall(gameServer *models.GameServer) error {
	if inst.shouldUseRemoteNodeFiles(gameServer.NodeID) {
		client, errClient := inst.nodeRegistry.Get(gameServer.NodeID)
		if errClient != nil {
			return fmt.Errorf("actions: resolve node client for 7 days to die post install: %w", errClient)
		}

		_, errCopy := client.CopyFiles(inst.actionContext(), gameServer.Directory, []node.CopyFileOperation{
			{
				SourceRelativePath:      "serverconfig.xml",
				DestinationRelativePath: "settings.xml",
			},
		}, node.ProtectionPolicy{})
		if errCopy != nil {
			log.Error().Err(errCopy).Msg("Failed to copy remote serverconfig.xml to settings.xml")
			return fmt.Errorf("actions: copy serverconfig.xml to settings.xml: %w", errCopy)
		}
		return nil
	}

	return post7DaysToDieInstall(gameServer)
}

func post7DaysToDieInstall(gameServer *models.GameServer) error {
	log.Debug().Msg("Copying serverconfig.xml to settings.xml")
	readConfig, errRead := os.Open(filepath.Join(gameServer.Directory, "serverconfig.xml")) // #nosec G703 -- gameServer.Directory is a controller-managed install root and the file name is fixed.
	if errRead != nil {
		log.Error().Err(errRead).Msg("Failed to open serverconfig.xml")
		return fmt.Errorf("actions: open serverconfig.xml: %w", errRead)
	}
	defer func() {
		errClose := readConfig.Close()
		if errClose != nil {
			log.Error().Err(errClose).Msg("Failed to close serverconfig.xml")
		}
	}()
	writeConfig, errWrite := os.Create(filepath.Join(gameServer.Directory, "settings.xml")) // #nosec G703 -- gameServer.Directory is a controller-managed install root and the file name is fixed.
	if errWrite != nil {
		log.Error().Err(errWrite).Msg("Failed to open settings.xml")
		return fmt.Errorf("actions: create settings.xml: %w", errWrite)
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
		return fmt.Errorf("actions: copy serverconfig.xml to settings.xml: %w", errCopy)
	}

	return nil
}
