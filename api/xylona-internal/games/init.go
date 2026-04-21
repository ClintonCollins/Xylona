// Package games registers the built-in internal game implementations.
package games

import internal "github.com/ClintonCollins/Xylona/api/xylona-internal"

func init() {
	registerMinecraft()
}

// RegisterInternalGames registers all built-in internal game implementations.
func RegisterInternalGames() {
	registerMinecraft()
}

func registerMinecraft() {
	internal.RegisterGame("minecraft", &Minecraft{})
}
