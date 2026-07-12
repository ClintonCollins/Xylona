package gameintegrations

import (
	"errors"
	"strings"
)

const (
	// StarboundGameID identifies the official Starbound integration.
	StarboundGameID = "starbound"
	// StarboundSteamUsernameEnv is the visible per-server environment value
	// used to select the Steam account that owns Starbound.
	StarboundSteamUsernameEnv = "STEAM_USERNAME"
)

// ErrStarboundSteamUsernameRequired indicates that the owning Steam account was not configured.
var ErrStarboundSteamUsernameRequired = errors.New(
	"a Steam account name for an account that owns Starbound is required to install Starbound",
)

// StarboundSteamUsername returns the configured account name without ever
// accepting a password or Steam Guard code as durable configuration.
func StarboundSteamUsername(environment map[string]string) (string, error) {
	for name, value := range environment {
		if strings.EqualFold(name, StarboundSteamUsernameEnv) {
			username := strings.TrimSpace(value)
			if username == "" {
				return "", ErrStarboundSteamUsernameRequired
			}
			return username, nil
		}
	}
	return "", ErrStarboundSteamUsernameRequired
}
