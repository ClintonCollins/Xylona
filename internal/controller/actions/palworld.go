package actions

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	palworldGameID       = "palworld"
	palworldRESTUsername = "admin"
	palworldDefaultFile  = "DefaultPalWorldSettings.ini"
)

var errPalworldOptionSettingsMissing = errors.New("palworld OptionSettings entry is missing")

type palworldSettingUpdate struct {
	key   string
	value string
}

func (inst *Instance) ensurePalworldQueryConfig(gameServer *models.GameServer, client nodeclient.NodeClient) error {
	if gameServer == nil || gameServer.GameID != palworldGameID {
		return nil
	}
	if client == nil {
		return errors.New("node client is nil")
	}
	if gameServer.QueryPort < 1 || gameServer.QueryPort > 65535 {
		return fmt.Errorf("query port %d is invalid", gameServer.QueryPort)
	}

	password, errPassword := inst.loadOrCreatePalworldRESTPassword(gameServer)
	if errPassword != nil {
		return errPassword
	}
	settingsPath := palworldSettingsPath(inst.resolveNodeOS(inst.ctx, gameServer.NodeID))
	settingsData, errSettings := inst.readPalworldSettingsSource(client, gameServer, settingsPath)
	if errSettings != nil {
		return errSettings
	}
	patchedSettings, errPatch := patchPalworldSettings(
		settingsData,
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

func (inst *Instance) loadOrCreatePalworldRESTPassword(gameServer *models.GameServer) (string, error) {
	password, configured, errDecrypt := inst.db.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindPalworldREST,
		db.GameServerSecretNamePalworldRESTPassword,
	)
	if errDecrypt != nil {
		return "", fmt.Errorf("load Palworld REST password: %w", errDecrypt)
	}
	if configured && password != "" {
		return password, nil
	}

	passwordBytes := make([]byte, 32)
	_, errRandom := rand.Read(passwordBytes)
	if errRandom != nil {
		return "", fmt.Errorf("generate Palworld REST password: %w", errRandom)
	}
	password = base64.RawURLEncoding.EncodeToString(passwordBytes)
	errStore := inst.db.SetGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindPalworldREST,
		db.GameServerSecretNamePalworldRESTPassword,
		password,
		gameServer.UserID,
	)
	if errStore != nil {
		return "", fmt.Errorf("store Palworld REST password: %w", errStore)
	}
	return password, nil
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

func patchPalworldSettings(data []byte, password string, queryPort int64, maxPlayers int64) ([]byte, error) {
	if strings.ContainsAny(password, "\"\\\r\n,()") {
		return nil, errors.New("palworld REST password contains unsupported characters")
	}
	text := string(data)
	lowerText := strings.ToLower(text)
	settingsIndex := strings.Index(lowerText, "optionsettings")
	if settingsIndex < 0 {
		return nil, errPalworldOptionSettingsMissing
	}
	equalsOffset := strings.Index(text[settingsIndex:], "=")
	if equalsOffset < 0 {
		return nil, errPalworldOptionSettingsMissing
	}
	openIndex := settingsIndex + equalsOffset + 1
	for openIndex < len(text) && (text[openIndex] == ' ' || text[openIndex] == '\t') {
		openIndex++
	}
	if openIndex >= len(text) || text[openIndex] != '(' {
		return nil, errPalworldOptionSettingsMissing
	}
	closeIndex := findPalworldClosingParenthesis(text, openIndex)
	if closeIndex < 0 {
		return nil, errors.New("palworld OptionSettings entry has no closing parenthesis")
	}

	fields, errFields := splitPalworldSettings(text[openIndex+1 : closeIndex])
	if errFields != nil {
		return nil, errFields
	}
	updates := []palworldSettingUpdate{
		{key: "AdminPassword", value: "\"" + password + "\""},
		{key: "RESTAPIEnabled", value: "True"},
		{key: "RESTAPIPort", value: strconv.FormatInt(queryPort, 10)},
		{key: "ServerPlayerMaxNum", value: strconv.FormatInt(maxPlayers, 10)},
	}
	for _, update := range updates {
		fields = setPalworldSetting(fields, update)
	}

	patched := text[:openIndex+1] + strings.Join(fields, ",") + text[closeIndex:]
	return []byte(patched), nil
}

func findPalworldClosingParenthesis(text string, openIndex int) int {
	depth := 0
	inQuote := false
	escaped := false
	for index := openIndex; index < len(text); index++ {
		character := text[index]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inQuote = false
			}
			continue
		}
		switch character {
		case '"':
			inQuote = true
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitPalworldSettings(settings string) ([]string, error) {
	fields := make([]string, 0, 64)
	start := 0
	depth := 0
	inQuote := false
	escaped := false
	for index := 0; index < len(settings); index++ {
		character := settings[index]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == '"' {
				inQuote = false
			}
			continue
		}
		switch character {
		case '"':
			inQuote = true
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, errors.New("palworld OptionSettings contains an unexpected closing parenthesis")
			}
			depth--
		case ',':
			if depth == 0 {
				fields = append(fields, strings.TrimSpace(settings[start:index]))
				start = index + 1
			}
		}
	}
	if inQuote || depth != 0 {
		return nil, errors.New("palworld OptionSettings contains an unterminated value")
	}
	lastField := strings.TrimSpace(settings[start:])
	if lastField != "" {
		fields = append(fields, lastField)
	}
	return fields, nil
}

func setPalworldSetting(fields []string, update palworldSettingUpdate) []string {
	for index, field := range fields {
		keyPart, _, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		key := strings.TrimSpace(keyPart)
		if strings.EqualFold(key, update.key) {
			fields[index] = key + "=" + update.value
			return fields
		}
	}
	return append(fields, update.key+"="+update.value)
}
