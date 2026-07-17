package actions

import (
	"errors"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/pkg/cfgparse"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	palworldGameID       = "palworld"
	palworldRESTUsername = "admin"
	palworldDefaultFile  = "DefaultPalWorldSettings.ini"
)

var errPalworldOptionSettingsMissing = cfgparse.ErrPalworldOptionSettingsMissing

func (inst *Instance) ensurePalworldQueryConfig(
	gameServer *models.GameServer,
	client nodeclient.NodeClient,
	password string,
) error {
	if gameServer == nil || gameServer.GameID != palworldGameID {
		return nil
	}
	if client == nil {
		return errors.New("node client is nil")
	}
	if gameServer.QueryPort < 1 || gameServer.QueryPort > 65535 {
		return fmt.Errorf("query port %d is invalid", gameServer.QueryPort)
	}

	if password == "" {
		return errors.New("palworld REST password is empty")
	}
	settingsPath := palworldSettingsPath(inst.resolveNodeOS(inst.ctx, gameServer.NodeID))
	settingsData, errSettings := inst.readPalworldSettingsSource(client, gameServer, settingsPath)
	if errSettings != nil {
		return errSettings
	}
	patchedSettings, errPatch := patchPalworldSettings(
		settingsData,
		gameServer.Name,
		password,
		gameServer.QueryPort,
		gameServer.MaxPlayers,
	)
	if errPatch != nil {
		return errPatch
	}

	settingsDirectory := path.Dir(settingsPath)
	errDirectory := client.CreateFileOrDirectory(
		inst.ctx,
		gameServer.Directory,
		settingsDirectory,
		"",
		true,
		node.ProtectionPolicy{},
	)
	if errDirectory != nil {
		return fmt.Errorf("create Palworld settings directory: %w", errDirectory)
	}
	errWrite := client.WriteFile(
		inst.ctx,
		gameServer.Directory,
		settingsPath,
		patchedSettings,
		node.ProtectionPolicy{},
	)
	if errWrite != nil {
		return fmt.Errorf("write Palworld settings: %w", errWrite)
	}
	return nil
}

func (inst *Instance) palworldQueryCredentials(gameServer *models.GameServer) (string, string, error) {
	if gameServer == nil {
		return "", "", errors.New("game server is nil")
	}
	password, configured, errDecrypt := inst.db.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindPalworldREST,
		db.GameServerSecretNamePalworldRESTPassword,
	)
	if errDecrypt != nil {
		return "", "", fmt.Errorf("decrypt Palworld REST password: %w", errDecrypt)
	}
	if !configured || password == "" {
		return "", "", errors.New("palworld REST password is not configured")
	}
	return palworldRESTUsername, password, nil
}

func (inst *Instance) readPalworldSettingsSource(
	client nodeclient.NodeClient,
	gameServer *models.GameServer,
	settingsPath string,
) ([]byte, error) {
	settingsData, errReadSettings := client.ReadFile(inst.ctx, gameServer.Directory, settingsPath)
	if errReadSettings == nil && strings.TrimSpace(string(settingsData)) != "" {
		return settingsData, nil
	}
	if errReadSettings != nil && !errors.Is(errReadSettings, os.ErrNotExist) {
		return nil, fmt.Errorf("read Palworld settings: %w", errReadSettings)
	}

	defaultData, errReadDefault := client.ReadFile(inst.ctx, gameServer.Directory, palworldDefaultFile)
	if errReadDefault != nil {
		return nil, fmt.Errorf("read default Palworld settings: %w", errReadDefault)
	}
	return defaultData, nil
}

func palworldSettingsPath(nodeOS OSType) string {
	serverDirectory := "LinuxServer"
	if nodeOS == Windows {
		serverDirectory = "WindowsServer"
	}
	return path.Join("Pal", "Saved", "Config", serverDirectory, "PalWorldSettings.ini")
}

func patchPalworldSettings(
	data []byte,
	serverName string,
	password string,
	queryPort int64,
	maxPlayers int64,
) ([]byte, error) {
	if strings.ContainsAny(password, "\"\\\r\n,()") {
		return nil, errors.New("palworld REST password contains unsupported characters")
	}
	if strings.ContainsAny(serverName, "\x00\r\n") {
		return nil, errors.New("palworld server name contains unsupported characters")
	}

	parser := &cfgparse.PalworldParser{}
	entries, errParse := parser.Parse(data)
	if errParse != nil {
		return nil, fmt.Errorf("parse Palworld settings: %w", errParse)
	}
	updates := []cfgparse.ConfigEntry{
		{Key: "ServerName", Value: serverName},
		{Key: "AdminPassword", Value: password},
		{Key: "RESTAPIEnabled", Value: "true"},
		{Key: "RESTAPIPort", Value: fmt.Sprintf("%d", queryPort)},
		{Key: "ServerPlayerMaxNum", Value: fmt.Sprintf("%d", maxPlayers)},
	}
	for _, update := range updates {
		entries = setPalworldConfigEntry(entries, update)
	}
	output, errWrite := parser.Write(entries)
	if errWrite != nil {
		return nil, fmt.Errorf("write Palworld settings: %w", errWrite)
	}
	return output, nil
}

func setPalworldConfigEntry(entries []cfgparse.ConfigEntry, update cfgparse.ConfigEntry) []cfgparse.ConfigEntry {
	for index := range entries {
		if strings.EqualFold(entries[index].Key, update.Key) {
			entries[index].Value = update.Value
			return entries
		}
	}
	return append(entries, update)
}
