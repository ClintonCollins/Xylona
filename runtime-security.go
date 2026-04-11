package main

import (
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/actions"
	dbpkg "github.com/ClintonCollins/Xylona/db"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func validateStartupRuntimeSecurity(config Configuration, database *dbpkg.Connection) error {
	runtimeSecurityContext := actions.DetectRuntimeSecurityContext(config.DBFilePath)

	localNodeID, errLocalNodeID := database.GetLocalNodeID()
	if errLocalNodeID != nil {
		return fmt.Errorf(`get local node ID for runtime security validation: %w`, errLocalNodeID)
	}

	localNodeID = strings.TrimSpace(localNodeID)

	var (
		gameServers       []*models.GameServer
		errGetGameServers error
	)

	if localNodeID == `` {
		gameServers, errGetGameServers = database.GetAllGameServers()
	} else {
		gameServers, errGetGameServers = database.GetGameServersByNodeID(localNodeID)
	}
	if errGetGameServers != nil {
		return fmt.Errorf(`list game servers for runtime security validation: %w`, errGetGameServers)
	}

	assessment := actions.AssessRuntimeSecurity(actions.RuntimeSecurityAssessmentInput{
		DBFilePath:  runtimeSecurityContext.DBFilePath,
		Servers:     gameServers,
		CurrentUser: runtimeSecurityContext.CurrentUser,
		Elevated:    runtimeSecurityContext.Elevated,
	})
	for _, warning := range assessment.Warnings {
		log.Warn().Msg(warning)
	}

	errBlocking := assessment.BlockingError()
	if errBlocking != nil {
		return fmt.Errorf(`evaluate runtime security: %w`, errBlocking)
	}

	return nil
}
