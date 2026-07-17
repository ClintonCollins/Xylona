// Package admininterface describes the password-protected management endpoint
// exposed by each officially supported game.
package admininterface

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/ClintonCollins/Xylona/internal/db"
)

// Supported administration interface transport labels.
const (
	TransportTelnet = "Telnet"
	TransportRCON   = "RCON"
	TransportREST   = "REST"
)

// Profile describes one game's management endpoint and encrypted password
// storage location. Password values are never included in this structure.
type Profile struct {
	Transport             string
	Port                  int64
	Username              string
	BindToGameServerIP    bool
	SecretKind            string
	SecretName            string
	RemoteAccessNote      string
	TransportSecurityNote string
}

// Lookup returns the management endpoint profile for an officially supported
// game that uses a password-protected network administration interface.
func Lookup(gameID string, port int64, queryPort int64) (Profile, bool) {
	profile := Profile{
		SecretKind: db.GameServerSecretKindAdminInterface,
		SecretName: db.GameServerSecretNameAdminInterfacePassword,
	}

	switch strings.TrimSpace(gameID) {
	case "7_days_to_die":
		profile.Transport = TransportTelnet
		profile.Port = queryPort
		profile.RemoteAccessNote = "7 Days to Die accepts remote Telnet connections when a password is configured."
		profile.TransportSecurityNote = "Telnet traffic, including the password and commands, is not encrypted."
	case "counter_strike_2", "garrys_mod", "team_fortress_2":
		profile.Transport = TransportRCON
		profile.Port = port
		profile.BindToGameServerIP = true
		profile.RemoteAccessNote = "RCON shares the game server's configured bind address and port."
		profile.TransportSecurityNote = "Source RCON traffic is not encrypted."
	case "factorio":
		profile.Transport = TransportRCON
		profile.Port = queryPort
		profile.BindToGameServerIP = true
		profile.RemoteAccessNote = "RCON is explicitly bound to the game server IP on its query port."
		profile.TransportSecurityNote = "RCON traffic is not encrypted."
	case "rust":
		profile.Transport = TransportRCON
		profile.Port = queryPort
		profile.BindToGameServerIP = true
		profile.RemoteAccessNote = "WebRCON is explicitly bound to the game server IP on its configured RCON port."
		profile.TransportSecurityNote = "Use a firewall or private network when exposing WebRCON."
	case "v_rising":
		profile.Transport = TransportRCON
		profile.Port = queryPort
		profile.BindToGameServerIP = true
		profile.RemoteAccessNote = "RCON is explicitly bound to the game server IP on its query port."
		profile.TransportSecurityNote = "RCON traffic is not encrypted."
	case "conan_exiles":
		profile.Transport = TransportRCON
		profile.Port = queryPort
		profile.BindToGameServerIP = true
		profile.RemoteAccessNote = "RCON uses the game server's configured multihome address."
		profile.TransportSecurityNote = "RCON traffic is not encrypted."
	case "palworld":
		profile.Transport = TransportREST
		profile.Port = queryPort
		profile.Username = "admin"
		profile.SecretKind = db.GameServerSecretKindPalworldREST
		profile.SecretName = db.GameServerSecretNamePalworldRESTPassword
		profile.RemoteAccessNote = "The Palworld REST API listens on the configured REST port."
		profile.TransportSecurityNote = "Palworld's REST API uses HTTP Basic authentication without TLS."
	case "satisfactory":
		profile.Transport = TransportREST
		profile.Port = port
		profile.BindToGameServerIP = true
		profile.RemoteAccessNote = "The HTTPS API shares the game server's configured address and main port."
		profile.TransportSecurityNote = "Satisfactory uses HTTPS and may present a self-signed certificate."
	default:
		return Profile{}, false
	}

	return profile, true
}

// ValidatePassword applies the common subset accepted safely by every managed
// config format and command line used by the supported games.
func ValidatePassword(gameID string, password string) error {
	if !utf8.ValidString(password) {
		return errors.New("admin interface password must be valid UTF-8")
	}
	if len(password) < 8 || len(password) > 128 {
		return errors.New("admin interface password must be between 8 and 128 characters")
	}
	for _, character := range password {
		if character < 0x21 || character > 0x7e || character == '"' || character == '\\' {
			return errors.New("admin interface password must use printable ASCII without spaces, double quotes, or backslashes")
		}
	}
	if gameID == "palworld" && strings.ContainsAny(password, ",()") {
		return errors.New("admin interface password for Palworld cannot contain commas or parentheses")
	}
	return nil
}
