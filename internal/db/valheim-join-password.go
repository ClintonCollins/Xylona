package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const valheimPasswordBlockID = "01JQSD00000000000000000003"

// GetValheimJoinPassword reads the password from the server's start arguments.
func GetValheimJoinPassword(server *models.GameServer) (string, error) {
	if server == nil || server.GameID != "valheim" {
		return "", errors.New("join passwords are supported only for Valheim")
	}
	var patches []startargs.Patch
	if server.StartArgsPatches != "" {
		errParse := json.Unmarshal([]byte(server.StartArgsPatches), &patches)
		if errParse != nil {
			return "", fmt.Errorf("parse start arguments: %w", errParse)
		}
	}
	for _, patch := range slices.Backward(patches) {
		if patch.ID == valheimPasswordBlockID {
			if patch.Op == startargs.PatchOpRemove || len(patch.Tokens) < 2 {
				return "", nil
			}
			return patch.Tokens[1], nil
		}
	}
	if server.R.Game == nil {
		return "", nil
	}
	template := server.R.Game.LinuxStartArgsTemplate.GetOr("")
	if template == "" {
		template = server.R.Game.WindowsStartArgsTemplate.GetOr("")
	}
	blocks, errParse := startargs.ParseTemplate(template)
	if errParse != nil {
		return "", fmt.Errorf("parse password template: %w", errParse)
	}
	for _, block := range blocks {
		if block.ID == valheimPasswordBlockID && len(block.Tokens) > 1 {
			return block.Tokens[1], nil
		}
	}
	return "", nil
}

// SetValheimJoinPassword updates the ordinary password argument patch.
func (c *Connection) SetValheimJoinPassword(server *models.GameServer, password string) error {
	if server == nil || server.GameID != "valheim" {
		return errors.New("join passwords are supported only for Valheim")
	}
	var patches []startargs.Patch
	if server.StartArgsPatches != "" {
		errParse := json.Unmarshal([]byte(server.StartArgsPatches), &patches)
		if errParse != nil {
			return fmt.Errorf("parse start arguments: %w", errParse)
		}
	}
	patches = slices.DeleteFunc(patches, func(patch startargs.Patch) bool { return patch.ID == valheimPasswordBlockID })
	patch := startargs.Patch{ID: valheimPasswordBlockID, Op: startargs.PatchOpRemove}
	if password != "" {
		patch.Op = startargs.PatchOpEdit
		patch.Tokens = []string{"-password", password}
	}
	patches = append(patches, patch)
	encoded, errEncode := json.Marshal(patches)
	if errEncode != nil {
		return fmt.Errorf("encode start arguments: %w", errEncode)
	}
	_, errUpdate := c.SQLDb.ExecContext(c.ctx, "update game_server set start_args_patches = ? where id = ?", string(encoded), server.ID)
	if errUpdate != nil {
		return fmt.Errorf("store join password: %w", errUpdate)
	}
	server.StartArgsPatches = string(encoded)
	return nil
}

// ClearValheimJoinPassword clears the password argument for the next start.
func (c *Connection) ClearValheimJoinPassword(server *models.GameServer) error {
	return c.SetValheimJoinPassword(server, "")
}
