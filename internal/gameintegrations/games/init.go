// Package games registers the built-in internal game implementations.
package games

import "github.com/ClintonCollins/Xylona/internal/gameintegrations"

func init() {
	registerMinecraft()
}

// RegisterInternalGames registers all built-in internal game implementations.
func RegisterInternalGames() {
	registerMinecraft()
}

func registerMinecraft() {
	gameintegrations.RegisterGame("minecraft", &Minecraft{})
}
