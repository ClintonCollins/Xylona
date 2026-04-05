// Package games registers the built-in internal game implementations.
package games

import internal "github.com/ClintonCollins/Xylona/api/xylona-internal"

func init() {
	internal.RegisterGame("minecraft", &Minecraft{})
}

// RegisterInternalGames registers all built-in internal game implementations.
func RegisterInternalGames() {
	internal.RegisterGame("minecraft", &Minecraft{})
}
