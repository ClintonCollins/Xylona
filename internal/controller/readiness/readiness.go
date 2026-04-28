// Package readiness evaluates per-server setup requirements before launch.
package readiness

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// KindMinecraftEULA tracks whether a Minecraft server owner accepted the EULA.
	KindMinecraftEULA = "minecraft_eula"
	// KindSteamGSLT tracks whether a Steam Game Server Login Token is configured.
	KindSteamGSLT = "steam_gslt"
	// KindHytaleAccount tracks whether a Hytale account/profile is linked.
	KindHytaleAccount = "hytale_account"
)

const minecraftEULAFileName = "eula.txt"

// Item is the public readiness state returned to the UI.
type Item struct {
	Kind       string
	Required   bool
	Complete   bool
	Blocking   bool
	Message    string
	PublicData string
}

type minecraftEULAPublicData struct {
	Accepted bool `json:"accepted"`
}

// List returns the current public readiness state for a server.
func List(ctx context.Context, database *db.Connection, gameServer *models.GameServer, client nodeclient.NodeClient) ([]Item, error) {
	if gameServer == nil {
		return nil, errors.New("readiness: game server is nil")
	}

	items := []Item{}
	if gameServer.GameID == "minecraft" {
		item, errItem := minecraftEULAItem(ctx, database, gameServer, client, false)
		if errItem != nil {
			item = Item{
				Kind:     KindMinecraftEULA,
				Required: true,
				Complete: false,
				Blocking: true,
				Message:  "Minecraft EULA status could not be read: " + errItem.Error(),
			}
		}
		items = append(items, item)
	}
	return items, nil
}

// CheckStart blocks launch when required setup is missing.
func CheckStart(ctx context.Context, database *db.Connection, gameServer *models.GameServer, client nodeclient.NodeClient) error {
	if gameServer == nil {
		return errors.New("game server is missing")
	}
	if gameServer.GameID != "minecraft" {
		return nil
	}

	item, errItem := minecraftEULAItem(ctx, database, gameServer, client, true)
	if errItem != nil {
		return errItem
	}
	if item.Blocking {
		return errors.New(item.Message)
	}
	return nil
}

// AcceptMinecraftEULA records EULA acceptance and writes eula.txt when possible.
func AcceptMinecraftEULA(ctx context.Context, database *db.Connection, gameServer *models.GameServer, client nodeclient.NodeClient, userID string) error {
	if gameServer == nil {
		return errors.New("game server is missing")
	}
	if gameServer.GameID != "minecraft" {
		return errors.New("game server is not a minecraft server")
	}

	errWrite := writeMinecraftEULA(ctx, gameServer, client)
	if errWrite != nil {
		return errWrite
	}
	return PersistMinecraftEULAAccepted(database, gameServer.ID, userID)
}

// PersistMinecraftEULAAccepted records that a user accepted the Minecraft EULA.
func PersistMinecraftEULAAccepted(database *db.Connection, gameServerID string, userID string) error {
	if database == nil {
		return nil
	}
	data, errData := minecraftEULAData(true)
	if errData != nil {
		return errData
	}
	return database.UpsertGameServerReadiness(gameServerID, KindMinecraftEULA, data, userID)
}

func minecraftEULAItem(ctx context.Context, database *db.Connection, gameServer *models.GameServer, client nodeclient.NodeClient, repairFile bool) (Item, error) {
	item := Item{
		Kind:     KindMinecraftEULA,
		Required: true,
		Complete: false,
		Blocking: true,
		Message:  "Minecraft EULA required",
	}

	accepted, hasRow, errRow := minecraftEULAAcceptedFromDB(database, gameServer.ID)
	if errRow != nil {
		return item, errRow
	}
	if accepted {
		if repairFile {
			errWrite := writeMinecraftEULA(ctx, gameServer, client)
			if errWrite != nil {
				return item, errWrite
			}
		}
		data, errData := minecraftEULAData(true)
		if errData != nil {
			return item, errData
		}
		item.Complete = true
		item.Blocking = false
		item.Message = "Minecraft EULA accepted"
		item.PublicData = data
		return item, nil
	}

	fileAccepted, errFile := readMinecraftEULA(ctx, gameServer, client)
	if errFile != nil {
		if errors.Is(errFile, os.ErrNotExist) {
			return item, nil
		}
		return item, errFile
	}
	if fileAccepted {
		errPersist := PersistMinecraftEULAAccepted(database, gameServer.ID, "")
		if errPersist != nil {
			return item, errPersist
		}
		data, errData := minecraftEULAData(true)
		if errData != nil {
			return item, errData
		}
		item.Complete = true
		item.Blocking = false
		item.Message = "Minecraft EULA accepted"
		item.PublicData = data
		return item, nil
	}
	if hasRow {
		data, errData := minecraftEULAData(false)
		if errData != nil {
			return item, errData
		}
		item.PublicData = data
	}
	return item, nil
}

func minecraftEULAAcceptedFromDB(database *db.Connection, gameServerID string) (bool, bool, error) {
	if database == nil {
		return false, false, nil
	}
	state, errGet := database.GetGameServerReadiness(gameServerID, KindMinecraftEULA)
	if errGet != nil {
		if errors.Is(errGet, sql.ErrNoRows) {
			return false, false, nil
		}
		return false, false, errGet
	}

	var data minecraftEULAPublicData
	errUnmarshal := json.Unmarshal([]byte(state.PublicData), &data)
	if errUnmarshal != nil {
		return false, true, fmt.Errorf("parse minecraft EULA readiness data: %w", errUnmarshal)
	}
	return data.Accepted, true, nil
}

func readMinecraftEULA(ctx context.Context, gameServer *models.GameServer, client nodeclient.NodeClient) (bool, error) {
	if client == nil {
		return false, errors.New("target node client is unavailable")
	}

	data, errRead := client.ReadFile(ctx, gameServer.Directory, minecraftEULAFileName)
	if errRead != nil {
		return false, errRead
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "eula=true") {
			return true, nil
		}
	}
	return false, nil
}

func writeMinecraftEULA(ctx context.Context, gameServer *models.GameServer, client nodeclient.NodeClient) error {
	if client == nil {
		return errors.New("target node client is unavailable")
	}

	errWrite := client.WriteFile(ctx, gameServer.Directory, minecraftEULAFileName, []byte("eula=true\n"), node.ProtectionPolicy{})
	if errWrite != nil {
		return fmt.Errorf("write minecraft EULA file: %w", errWrite)
	}
	return nil
}

func minecraftEULAData(accepted bool) (string, error) {
	data, errMarshal := json.Marshal(minecraftEULAPublicData{Accepted: accepted})
	if errMarshal != nil {
		return "", fmt.Errorf("marshal minecraft EULA readiness data: %w", errMarshal)
	}
	return string(data), nil
}
