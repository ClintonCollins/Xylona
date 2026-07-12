// Package readiness evaluates per-server setup requirements before launch.
package readiness

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	// KindMinecraftEULA tracks whether a Minecraft server owner accepted the EULA.
	KindMinecraftEULA = "minecraft_eula"
	// KindSteamGSLT tracks whether a Steam Game Server Login Token is configured.
	KindSteamGSLT = "steam_gslt"
	// KindHytaleAccount tracks whether a Hytale account/profile is linked.
	KindHytaleAccount = "hytale_account"
	// KindSunkenlandWorld tracks whether a client-created Sunkenland world was imported.
	KindSunkenlandWorld = "sunkenland_world"
	// KindDragonwildsConfig tracks the mandatory owner/admin configuration.
	KindDragonwildsConfig = "dragonwilds_config"
)

const minecraftEULAFileName = "eula.txt"

var sunkenlandWorldNamePattern = regexp.MustCompile(`(?i)^.+~([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`)

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
	if requiresSteamGSLT(gameServer) {
		item, errItem := steamGSLTItem(database, gameServer)
		if errItem != nil {
			item = Item{
				Kind:     KindSteamGSLT,
				Required: true,
				Complete: false,
				Blocking: true,
				Message:  "Steam GSLT status could not be read: " + errItem.Error(),
			}
		}
		items = append(items, item)
	}
	if RequiresHytaleAccount(gameServer) {
		item, errItem := HytaleAccountItem(ctx, database, gameServer, client, false)
		if errItem != nil {
			item = Item{
				Kind:     KindHytaleAccount,
				Required: true,
				Complete: false,
				Blocking: true,
				Message:  "Hytale account status could not be read: " + errItem.Error(),
			}
		}
		items = append(items, item)
	}
	if gameServer.GameID == "sunkenland" {
		item, errItem := sunkenlandWorldItem(ctx, gameServer, client)
		if errItem != nil {
			item = Item{
				Kind:     KindSunkenlandWorld,
				Required: true,
				Complete: false,
				Blocking: true,
				Message:  "Sunkenland world status could not be read: " + errItem.Error(),
			}
		}
		items = append(items, item)
	}
	if gameServer.GameID == "runescape_dragonwilds" {
		item, errItem := dragonwildsConfigItem(ctx, gameServer, client)
		if errItem != nil {
			item = Item{
				Kind:     KindDragonwildsConfig,
				Required: true,
				Complete: false,
				Blocking: true,
				Message:  "Dragonwilds configuration could not be read: " + errItem.Error(),
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
	if gameServer.GameID == "minecraft" {
		item, errItem := minecraftEULAItem(ctx, database, gameServer, client, true)
		if errItem != nil {
			return errItem
		}
		if item.Blocking {
			return errors.New(item.Message)
		}
	}

	if requiresSteamGSLT(gameServer) {
		item, errItem := steamGSLTItem(database, gameServer)
		if errItem != nil {
			return errItem
		}
		if item.Blocking {
			return errors.New(item.Message)
		}
	}

	if RequiresHytaleAccount(gameServer) {
		item, errItem := HytaleAccountItem(ctx, database, gameServer, client, true)
		if errItem != nil {
			return errItem
		}
		if item.Blocking {
			return errors.New(item.Message)
		}
	}

	if gameServer.GameID == "sunkenland" {
		item, errItem := sunkenlandWorldItem(ctx, gameServer, client)
		if errItem != nil {
			return errItem
		}
		if item.Blocking {
			return errors.New(item.Message)
		}
	}
	if gameServer.GameID == "runescape_dragonwilds" {
		item, errItem := dragonwildsConfigItem(ctx, gameServer, client)
		if errItem != nil {
			return errItem
		}
		if item.Blocking {
			return errors.New(item.Message)
		}
	}
	return nil
}

func dragonwildsConfigItem(ctx context.Context, gameServer *models.GameServer, client nodeclient.NodeClient) (Item, error) {
	item := Item{
		Kind:     KindDragonwildsConfig,
		Required: true,
		Complete: false,
		Blocking: true,
		Message: "Configure the Dragonwilds Owner ID and Admin Password in the server's " +
			"DedicatedServer.ini before starting it.",
	}
	if client == nil {
		return item, errors.New("dragonwilds node client is unavailable")
	}

	snapshot, errSnapshot := client.GetNodeSnapshot(ctx)
	if errSnapshot != nil {
		return item, fmt.Errorf("get Dragonwilds node platform: %w", errSnapshot)
	}
	if snapshot == nil {
		return item, errors.New("get Dragonwilds node platform: snapshot is missing")
	}
	var configPath string
	switch strings.ToLower(strings.TrimSpace(snapshot.OS)) {
	case "windows":
		configPath = "RSDragonwilds/Saved/Config/WindowsServer/DedicatedServer.ini"
	case "linux":
		configPath = "RSDragonwilds/Saved/Config/Linux/DedicatedServer.ini"
	default:
		return item, fmt.Errorf("dragonwilds node operating system %q is unsupported", snapshot.OS)
	}

	contents, errRead := client.ReadFile(ctx, gameServer.Directory, configPath)
	if errors.Is(errRead, os.ErrNotExist) {
		return item, nil
	}
	if errRead != nil {
		return item, fmt.Errorf("read Dragonwilds configuration: %w", errRead)
	}
	entries, errParse := (&cfgparse.INIParser{}).Parse(contents)
	if errParse != nil {
		return item, fmt.Errorf("parse Dragonwilds configuration: %w", errParse)
	}
	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		values[strings.ToLower(strings.TrimSpace(entry.Key))] = strings.TrimSpace(entry.Value)
	}
	missing := make([]string, 0, 4)
	for key, label := range map[string]string{
		"ownerid":          "Owner ID",
		"servername":       "Server Name",
		"defaultworldname": "Default World Name",
		"adminpassword":    "Admin Password",
	} {
		if values[key] == "" {
			missing = append(missing, label)
		}
	}
	if len(missing) > 0 {
		slices.Sort(missing)
		item.Message = "Dragonwilds requires these DedicatedServer.ini values before start: " + strings.Join(missing, ", ") + "."
		return item, nil
	}

	publicData, errPublicData := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: configPath})
	if errPublicData != nil {
		return item, fmt.Errorf("encode Dragonwilds readiness: %w", errPublicData)
	}
	item.Complete = true
	item.Blocking = false
	item.Message = "Dragonwilds dedicated-server configuration is ready."
	item.PublicData = string(publicData)
	return item, nil
}

func sunkenlandWorldItem(ctx context.Context, gameServer *models.GameServer, client nodeclient.NodeClient) (Item, error) {
	item := Item{
		Kind:     KindSunkenlandWorld,
		Required: true,
		Complete: false,
		Blocking: true,
		Message: "Sunkenland requires a client-created world. Upload its complete " +
			"WorldName~GUID folder to the server's worlds directory and extract it there.",
	}
	if client == nil {
		return item, errors.New("sunkenland node client is unavailable")
	}

	entries, errList := client.ListFiles(ctx, gameServer.Directory, "worlds")
	if errors.Is(errList, os.ErrNotExist) {
		return item, nil
	}
	if errList != nil {
		return item, fmt.Errorf("list Sunkenland worlds: %w", errList)
	}

	type worldCandidate struct {
		folder string
		guid   string
	}
	candidates := make([]worldCandidate, 0)
	for _, entry := range entries {
		if !entry.IsDirectory {
			continue
		}
		matches := sunkenlandWorldNamePattern.FindStringSubmatch(strings.TrimSpace(entry.Name))
		if len(matches) != 2 {
			continue
		}
		candidates = append(candidates, worldCandidate{folder: entry.Name, guid: strings.ToLower(matches[1])})
	}
	if len(candidates) == 0 {
		return item, nil
	}
	if len(candidates) > 1 {
		item.Message = "Sunkenland found multiple valid world folders under worlds. Keep exactly one world in this server directory."
		return item, nil
	}

	candidate := candidates[0]
	worldEntries, errWorld := client.ListFiles(ctx, gameServer.Directory, filepath.ToSlash(filepath.Join("worlds", candidate.folder)))
	if errWorld != nil {
		return item, fmt.Errorf("inspect Sunkenland world %q: %w", candidate.folder, errWorld)
	}
	if len(worldEntries) == 0 {
		item.Message = "Sunkenland world folder " + candidate.folder + " is empty. Upload the complete client-created world."
		return item, nil
	}

	publicData, errPublicData := json.Marshal(struct {
		Folder string `json:"folder"`
		GUID   string `json:"guid"`
	}{Folder: candidate.folder, GUID: candidate.guid})
	if errPublicData != nil {
		return item, fmt.Errorf("encode Sunkenland world readiness: %w", errPublicData)
	}
	item.Complete = true
	item.Blocking = false
	item.Message = "Sunkenland world ready: " + candidate.folder
	item.PublicData = string(publicData)
	return item, nil
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
	errPersist := database.UpsertGameServerReadiness(gameServerID, KindMinecraftEULA, data, userID)
	if errPersist != nil {
		return fmt.Errorf("persist minecraft EULA readiness: %w", errPersist)
	}
	return nil
}

// SetSteamGSLT stores a Steam Game Server Login Token as an encrypted readiness secret.
func SetSteamGSLT(database *db.Connection, gameServerID string, token string, userID string) error {
	if database == nil {
		return errors.New("database is missing")
	}
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return errors.New("steam GSLT is required")
	}
	errPersist := database.SetGameServerSecret(
		gameServerID,
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
		trimmedToken,
		userID,
	)
	if errPersist != nil {
		return fmt.Errorf("store steam GSLT: %w", errPersist)
	}
	return nil
}

// ClearSteamGSLT removes a configured Steam Game Server Login Token.
func ClearSteamGSLT(database *db.Connection, gameServerID string) error {
	if database == nil {
		return errors.New("database is missing")
	}
	errClear := database.ClearGameServerSecret(
		gameServerID,
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	)
	if errClear != nil {
		return fmt.Errorf("clear steam GSLT: %w", errClear)
	}
	return nil
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

func steamGSLTItem(database *db.Connection, gameServer *models.GameServer) (Item, error) {
	item := Item{
		Kind:     KindSteamGSLT,
		Required: true,
		Complete: false,
		Blocking: true,
		Message:  "Steam GSLT required",
	}
	if database == nil {
		return item, errors.New("database is missing")
	}

	configured, errConfigured := database.HasGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindSteamGSLT,
		db.GameServerSecretNameSteamGSLT,
	)
	if errConfigured != nil {
		return item, fmt.Errorf("check steam GSLT: %w", errConfigured)
	}
	if configured {
		item.Complete = true
		item.Blocking = false
		item.Message = "Steam GSLT configured"
	}
	return item, nil
}

func requiresSteamGSLT(gameServer *models.GameServer) bool {
	if gameServer == nil {
		return false
	}
	if gameServer.R.Game == nil {
		return false
	}
	return gameServer.R.Game.RequiresSteamGameServerLoginToken
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
		return false, false, fmt.Errorf("load minecraft EULA readiness: %w", errGet)
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
		return false, fmt.Errorf("read minecraft EULA file: %w", errRead)
	}

	for line := range strings.SplitSeq(string(data), "\n") {
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
