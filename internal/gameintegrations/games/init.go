// Package games registers the built-in internal game implementations.
package games

import "github.com/ClintonCollins/Xylona/internal/gameintegrations"

func init() {
	registerFactorio()
	registerHytale()
	registerMinecraft()
	registerTerraria()
}

// RegisterInternalGames registers all built-in internal game implementations.
func RegisterInternalGames() {
	registerFactorio()
	registerHytale()
	registerMinecraft()
	registerTerraria()
}

func registerFactorio() {
	gameintegrations.RegisterGame("factorio", &Factorio{})
}

func registerMinecraft() {
	gameintegrations.RegisterGame("minecraft", &Minecraft{})
}

func registerHytale() {
	gameintegrations.RegisterGame("hytale", &Hytale{})
}

func registerTerraria() {
	gameintegrations.RegisterGame("terraria", &Terraria{})
}
