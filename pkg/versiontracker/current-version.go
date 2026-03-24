package versiontracker

import "github.com/ClintonCollins/Xylona/sql/models"

// ResolveCurrentVersion returns the best-effort currently installed version for display.
// For Minecraft it prefers reading the active jar directly; all other games fall back to
// the persisted game server version field.
func ResolveCurrentVersion(gs *models.GameServer) string {
	switch gs.GameID {
	case "minecraft":
		version, errGetVersion := ReadMinecraftJarVersion(gs.Directory, gs.ServerExecutable.GetOr(""))
		if errGetVersion != nil {
			return gs.Version
		}
		return version
	default:
		return gs.Version
	}
}
