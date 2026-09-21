package readiness

import (
	"errors"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/valheim"
	"github.com/ClintonCollins/Xylona/sql/models"
)

// JoinPasswordState contains join password setup metadata.
type JoinPasswordState struct {
	Supported        bool
	Configured       bool
	ValidationIssues []string
}

// GetJoinPasswordState checks the configured join password.
func GetJoinPasswordState(_ *db.Connection, server *models.GameServer) JoinPasswordState {
	state := JoinPasswordState{Supported: server != nil && server.GameID == "valheim"}
	if !state.Supported {
		return state
	}
	password, errPassword := db.GetValheimJoinPassword(server)
	if errPassword != nil {
		state.ValidationIssues = []string{"Join password could not be read from start arguments"}
		return state
	}
	state.Configured = password != ""
	if !state.Configured {
		return state
	}
	errValidate := valheim.ValidateJoinPassword(password, server.Name)
	if errValidate != nil {
		state.ValidationIssues = append(state.ValidationIssues, errValidate.Error())
	}

	return state
}

// ValidateJoinPasswordStart blocks launch when the join password is invalid.
func ValidateJoinPasswordStart(database *db.Connection, server *models.GameServer) error {
	if server == nil || server.GameID != "valheim" {
		return nil
	}
	state := GetJoinPasswordState(database, server)
	if len(state.ValidationIssues) != 0 {
		return errors.New(state.ValidationIssues[0])
	}
	return nil
}
