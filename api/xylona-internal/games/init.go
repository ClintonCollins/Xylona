package games

import internal "github.com/ClintonCollins/Xylona/api/xylona-internal"

func init() {
	internal.RegisterGame("minecraft", &Minecraft{})
}

func RegisterInternalGames() {
	internal.RegisterGame("minecraft", &Minecraft{})
}
