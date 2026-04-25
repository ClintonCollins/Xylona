package rpc

import (
	"fmt"

	"github.com/ClintonCollins/Xylona/sql/models"
)

func validateGameServerPort(port int64, label string) error {
	if port < 1 || port > 65535 {
		return invalidArg(fmt.Sprintf("%s must be between 1 and 65535", label))
	}

	return nil
}

func validateGameServerPlayerCount(value int64, label string, minimum int64) error {
	if value < minimum {
		return invalidArg(fmt.Sprintf("%s must be %d or greater", label, minimum))
	}

	return nil
}

func validateGameServerPlayerCountAtMost(value int64, label string, maximum *int64, maximumLabel string) error {
	if maximum == nil {
		return nil
	}

	if value > *maximum {
		return invalidArg(fmt.Sprintf("%s cannot exceed %s", label, maximumLabel))
	}

	return nil
}

func validateGameServerMaxMemory(maxMemoryMB int64) error {
	if maxMemoryMB < 128 {
		return invalidArg(`Max Memory MB must be at least 128`)
	}

	return nil
}

func validateGameServerSubmission(game *models.Game, gameServer *models.GameServer, validateProvisioning bool) error {
	if errSetPlayers := validateGameServerPlayerCount(gameServer.SetPlayers, `Set Players`, 0); errSetPlayers != nil {
		return errSetPlayers
	}

	if errSetPlayersRange := validateGameServerPlayerCountAtMost(
		gameServer.SetPlayers,
		`Set Players`,
		&gameServer.MaxPlayers,
		`Max Players`,
	); errSetPlayersRange != nil {
		return errSetPlayersRange
	}

	if !validateProvisioning {
		return nil
	}

	if errPort := validateGameServerPort(gameServer.Port, `Port`); errPort != nil {
		return errPort
	}

	if errQueryPort := validateGameServerPort(gameServer.QueryPort, `Query Port`); errQueryPort != nil {
		return errQueryPort
	}

	if errMaxPlayers := validateGameServerPlayerCount(gameServer.MaxPlayers, `Max Players`, 1); errMaxPlayers != nil {
		return errMaxPlayers
	}

	if game != nil && game.ID == `minecraft` {
		if errMaxMemory := validateGameServerMaxMemory(gameServer.MaxMemoryMB); errMaxMemory != nil {
			return errMaxMemory
		}
	}

	return nil
}
