package actions

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/ClintonCollins/Xylona/internal/admininterface"
	"github.com/ClintonCollins/Xylona/internal/db"
	"github.com/ClintonCollins/Xylona/internal/node"
	"github.com/ClintonCollins/Xylona/internal/nodeclient"
	"github.com/ClintonCollins/Xylona/internal/startargs"
	"github.com/ClintonCollins/Xylona/pkg/cfgschema"
	"github.com/ClintonCollins/Xylona/sql/models"
)

const (
	sevenDaysToDieWebAPITokenNamePlaceholder   = "SEVEN_DAYS_TO_DIE_WEB_API_TOKEN_NAME"   //nolint:gosec // Placeholder identifier, not a credential.
	sevenDaysToDieWebAPITokenSecretPlaceholder = "SEVEN_DAYS_TO_DIE_WEB_API_TOKEN_SECRET" //nolint:gosec // Placeholder identifier, not a credential.
	sevenDaysToDieWebAPITokenName              = "xylona"
	sevenDaysToDieWebAPITokenDomain            = "xylona:7dtd-map:v1" //nolint:gosec // Public domain-separation label, not a credential.
)

type gameServerAdminInput struct {
	telnet                *node.TelnetInput
	rcon                  *node.RCONInput
	rest                  *node.RESTInput
	placeholderVars       map[string]string
	localConsolePassword  string
	managedConfigRequired bool
}

func newGameServerAdminInput(
	gameServer *models.GameServer,
	password string,
	previousPasswords []string,
) (gameServerAdminInput, error) {
	if gameServer == nil {
		return gameServerAdminInput{}, errors.New("actions: game server is nil")
	}
	if !GameServerDefinitionSupportsAdminInput(gameServer) {
		return gameServerAdminInput{}, nil
	}
	if password == "" {
		return gameServerAdminInput{}, errors.New("actions: admin interface password is empty")
	}

	switch gameServer.GameID {
	case sevenDaysToDieGameID:
		if gameServer.QueryPort <= 0 || gameServer.QueryPort > 65535 {
			return gameServerAdminInput{}, errors.New("actions: Telnet port is invalid")
		}
		webAPITokenSecret, errWebAPITokenSecret := deriveSevenDaysToDieWebAPITokenSecret(password, gameServer.ID)
		if errWebAPITokenSecret != nil {
			return gameServerAdminInput{}, errWebAPITokenSecret
		}
		return gameServerAdminInput{
			telnet: &node.TelnetInput{
				Port:     int(gameServer.QueryPort),
				Password: password,
			},
			placeholderVars: map[string]string{
				sevenDaysToDieWebAPITokenNamePlaceholder:   sevenDaysToDieWebAPITokenName,
				sevenDaysToDieWebAPITokenSecretPlaceholder: webAPITokenSecret,
			},
			localConsolePassword:  password,
			managedConfigRequired: true,
		}, nil
	case counterStrikeTwoGameID, garrysModGameID, teamFortressTwoGameID:
		return newRCONAdminInput(gameServer, gameServer.Port, password, node.RCONProtocolSource, true)
	case factorioGameID, "v_rising":
		return newRCONAdminInput(gameServer, gameServer.QueryPort, password, node.RCONProtocolSource, false)
	case rustGameID:
		return newRCONAdminInput(gameServer, gameServer.QueryPort, password, node.RCONProtocolRustWeb, false)
	case "conan_exiles":
		return newRCONAdminInput(gameServer, gameServer.QueryPort, password, node.RCONProtocolMinecraft, false)
	case "satisfactory":
		if gameServer.Port <= 0 || gameServer.Port > 65535 {
			return gameServerAdminInput{}, errors.New("actions: Satisfactory REST port is invalid")
		}
		return gameServerAdminInput{
			rest: &node.RESTInput{
				Host:              localAdminInputHost(gameServer.IP),
				Port:              int(gameServer.Port),
				Kind:              node.RESTInputKindSatisfactory,
				Password:          password,
				PreviousPasswords: slices.Clone(previousPasswords),
			},
		}, nil
	default:
		return gameServerAdminInput{}, nil
	}
}

func deriveSevenDaysToDieWebAPITokenSecret(adminSecret string, gameServerID string) (string, error) {
	if adminSecret == "" {
		return "", errors.New("actions: 7 Days to Die admin secret is empty")
	}
	trimmedGameServerID := strings.TrimSpace(gameServerID)
	if trimmedGameServerID == "" {
		return "", errors.New("actions: game server ID is empty")
	}

	mac := hmac.New(sha256.New, []byte(adminSecret))
	_, errDomain := mac.Write([]byte(sevenDaysToDieWebAPITokenDomain))
	if errDomain != nil {
		return "", fmt.Errorf("actions: derive 7 Days to Die WebAPI token domain: %w", errDomain)
	}
	_, errSeparator := mac.Write([]byte{0})
	if errSeparator != nil {
		return "", fmt.Errorf("actions: derive 7 Days to Die WebAPI token separator: %w", errSeparator)
	}
	_, errGameServerID := mac.Write([]byte(trimmedGameServerID))
	if errGameServerID != nil {
		return "", fmt.Errorf("actions: derive 7 Days to Die WebAPI token server ID: %w", errGameServerID)
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

// SevenDaysToDieMapCredentials returns the deterministic native WebAPI token
// injected into the server's locked launch arguments.
func (inst *Instance) SevenDaysToDieMapCredentials(gameServer *models.GameServer) (string, string, error) {
	if gameServer == nil || gameServer.GameID != sevenDaysToDieGameID {
		return "", "", errors.New("actions: game server is not 7 Days to Die")
	}
	adminSecret, errSecret := inst.loadOrCreateAdminInterfacePassword(gameServer)
	if errSecret != nil {
		return "", "", errSecret
	}
	tokenSecret, errToken := deriveSevenDaysToDieWebAPITokenSecret(adminSecret, gameServer.ID)
	if errToken != nil {
		return "", "", errToken
	}
	return sevenDaysToDieWebAPITokenName, tokenSecret, nil
}

// GameServerDefinitionSupportsAdminInput reports whether the effective game
// definition still contains Xylona's non-editable administration contract.
func GameServerDefinitionSupportsAdminInput(gameServer *models.GameServer) bool {
	if gameServer == nil || gameServer.R.Game == nil {
		return false
	}
	game := gameServer.R.Game

	switch gameServer.GameID {
	case sevenDaysToDieGameID, counterStrikeTwoGameID, garrysModGameID, teamFortressTwoGameID:
		return gameDefinitionHasManagedConsolePassword(game)
	case factorioGameID:
		return gameDefinitionTemplatesContainNonEditableBlock(
			game,
			"--rcon-bind",
			"{{RCON_BIND}}",
			"--rcon-password",
			"{{RCON_PASSWORD}}",
		)
	case "v_rising":
		return gameDefinitionTemplatesContainNonEditableBlock(
			game,
			"-rconEnabled",
			"true",
			"-rconPort",
			"{{RCON_PORT}}",
			"-rconPassword",
			"{{RCON_PASSWORD}}",
			"-rconBindAddress",
			"{{IP}}",
		)
	case rustGameID:
		return gameDefinitionTemplatesContainNonEditableBlock(
			game,
			"+server.ip",
			"{{IP}}",
		) && gameDefinitionTemplatesContainNonEditableBlock(
			game,
			"+rcon.ip",
			"{{IP}}",
			"+rcon.port",
			"{{RCON_PORT}}",
			"+rcon.password",
			"{{RCON_PASSWORD}}",
			"+rcon.web",
			"1",
		)
	case "conan_exiles":
		return gameDefinitionTemplatesContainNonEditableBlock(
			game,
			"-RconEnabled=1",
			"-RconPort={{RCON_PORT}}",
			"-RconPassword={{RCON_PASSWORD}}",
		)
	case "satisfactory":
		return gameDefinitionTemplatesContainNonEditableBlock(
			game,
			"-multihome={{IP}}",
			"-ini:Engine:[HTTPServer.Listeners]:DefaultBindAddress={{IP}}",
		)
	default:
		return false
	}
}

func gameDefinitionHasManagedConsolePassword(game *models.Game) bool {
	entries, errParse := cfgschema.ParseConfigSchemas(game.ConfigSchemas.GetOr(""))
	if errParse != nil {
		return false
	}
	for _, entry := range entries {
		for _, source := range entry.ManagedFields {
			if source == "xylona.local_console_password" {
				return true
			}
		}
	}
	return false
}

func gameDefinitionTemplatesContainNonEditableBlock(game *models.Game, requiredTokens ...string) bool {
	templates := make([]string, 0, 2)
	if game.LinuxSupport {
		templates = append(templates, game.LinuxStartArgsTemplate.GetOr(""))
	}
	if game.WindowsSupport {
		templates = append(templates, game.WindowsStartArgsTemplate.GetOr(""))
	}
	if len(templates) == 0 {
		return false
	}

	for _, templateJSON := range templates {
		blocks, errParse := startargs.ParseTemplate(strings.TrimSpace(templateJSON))
		if errParse != nil {
			return false
		}
		contractFound := false
		for _, block := range blocks {
			if block.Ownership != startargs.OwnershipSystem && block.Ownership != startargs.OwnershipLocked {
				continue
			}
			if blockContainsAllTokens(block.Tokens, requiredTokens) {
				contractFound = true
				break
			}
		}
		if !contractFound {
			return false
		}
	}
	return true
}

func blockContainsAllTokens(tokens []string, required []string) bool {
	for _, requiredToken := range required {
		if !slices.Contains(tokens, requiredToken) {
			return false
		}
	}
	return true
}

func newRCONAdminInput(
	gameServer *models.GameServer,
	port int64,
	password string,
	protocol node.RCONProtocol,
	managedConfigRequired bool,
) (gameServerAdminInput, error) {
	if port <= 0 || port > 65535 {
		return gameServerAdminInput{}, errors.New("actions: RCON port is invalid")
	}
	bindHost := strings.TrimSpace(gameServer.IP)
	if bindHost == "" {
		bindHost = "0.0.0.0"
	}
	return gameServerAdminInput{
		rcon: &node.RCONInput{
			Host:     localAdminInputHost(gameServer.IP),
			Port:     int(port),
			Password: password,
			Protocol: protocol,
		},
		placeholderVars: map[string]string{
			"RCON_PORT":     strconv.FormatInt(port, 10),
			"RCON_PASSWORD": password,
			"RCON_BIND":     net.JoinHostPort(bindHost, strconv.FormatInt(port, 10)),
		},
		localConsolePassword:  password,
		managedConfigRequired: managedConfigRequired,
	}, nil
}

func localAdminInputHost(host string) string {
	host = strings.TrimSpace(host)
	ip := net.ParseIP(host)
	if host == "" || (ip != nil && ip.IsUnspecified()) {
		return "127.0.0.1"
	}
	return host
}

func (input gameServerAdminInput) apply(config *node.ProcessConfig) {
	if config == nil {
		return
	}
	config.InputTelnet = input.telnet
	config.InputRCON = input.rcon
	config.InputREST = input.rest
}

func (input gameServerAdminInput) mergePlaceholderVars(values map[string]string) {
	maps.Copy(values, input.placeholderVars)
}

func (inst *Instance) ensureAdminInputSupported(
	client nodeclient.NodeClient,
	input gameServerAdminInput,
) error {
	if input.telnet == nil && input.rcon == nil && input.rest == nil {
		return nil
	}
	caps, errCaps := client.GetRuntimeCapabilities(inst.ctx)
	if errCaps != nil {
		return startUnavailableError("target node runtime capabilities unavailable", errCaps)
	}
	if input.telnet != nil && !caps.TelnetInput {
		return startConfigurationError("target node does not support the required Telnet input; upgrade the node before starting this server", nil)
	}
	if input.rcon != nil && !caps.RCONInput {
		return startConfigurationError("target node does not support the required RCON input; upgrade the node before starting this server", nil)
	}
	if input.rest != nil && !caps.RESTInput {
		return startConfigurationError("target node does not support the required REST input; upgrade the node before starting this server", nil)
	}
	return nil
}

func (inst *Instance) loadOrCreateAdminInterfacePassword(gameServer *models.GameServer) (string, error) {
	if gameServer == nil {
		return "", errors.New("actions: game server is nil")
	}
	profile, supported := admininterface.Lookup(gameServer.GameID, gameServer.Port, gameServer.QueryPort)
	if !supported {
		return "", nil
	}
	if gameServer.GameID != palworldGameID && !GameServerDefinitionSupportsAdminInput(gameServer) {
		return "", nil
	}

	password, configured, errDecrypt := inst.db.DecryptGameServerSecret(
		gameServer.ID,
		profile.SecretKind,
		profile.SecretName,
	)
	if errDecrypt != nil {
		return "", fmt.Errorf("load admin interface password: %w", errDecrypt)
	}
	if configured && password != "" {
		return password, nil
	}

	passwordBytes := make([]byte, 32)
	_, errRandom := rand.Read(passwordBytes)
	if errRandom != nil {
		return "", fmt.Errorf("generate admin interface password: %w", errRandom)
	}
	password = base64.RawURLEncoding.EncodeToString(passwordBytes)
	errStore := inst.db.SetGameServerSecret(
		gameServer.ID,
		profile.SecretKind,
		profile.SecretName,
		password,
		gameServer.UserID,
	)
	if errStore != nil {
		return "", fmt.Errorf("store admin interface password: %w", errStore)
	}
	return password, nil
}

func (inst *Instance) loadAdminInterfacePasswordHistory(gameServer *models.GameServer) ([]string, error) {
	if gameServer == nil || gameServer.GameID != "satisfactory" {
		return nil, nil
	}
	rawHistory, configured, errDecrypt := inst.db.DecryptGameServerSecret(
		gameServer.ID,
		db.GameServerSecretKindAdminInterface,
		db.GameServerSecretNameAdminInterfacePasswordHistory,
	)
	if errDecrypt != nil {
		return nil, fmt.Errorf("load admin interface password history: %w", errDecrypt)
	}
	if !configured {
		return nil, nil
	}
	history, errParse := admininterface.ParsePasswordHistory(rawHistory)
	if errParse != nil {
		return nil, fmt.Errorf("parse admin interface password history: %w", errParse)
	}
	return history, nil
}
