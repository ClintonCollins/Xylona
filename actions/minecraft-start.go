package actions

import (
	"fmt"
	"strings"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/pkg/versiontracker"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func (inst *Instance) ensureMinecraftServerExecutable(gameServer *models.GameServer) error {
	if gameServer == nil {
		return nil
	}
	if gameServer.GameID != "minecraft" {
		return nil
	}

	serverExecutable := strings.TrimSpace(gameServer.ServerExecutable.GetOr(""))
	if serverExecutable != "" {
		return nil
	}

	discoveredExecutable, errDiscover := versiontracker.DiscoverMinecraftExecutable(
		gameServer.Directory,
		gameServer.ServerSoftware.GetOr(""),
	)
	if errDiscover != nil {
		return fmt.Errorf("discover minecraft server executable: %w", errDiscover)
	}
	if discoveredExecutable == "" {
		return nil
	}

	gameServer.ServerExecutable = null.From(discoveredExecutable)
	if inst.db == nil {
		return nil
	}

	setter := &models.GameServerSetter{
		ID:               omit.From(gameServer.ID),
		ServerExecutable: omitnull.From(discoveredExecutable),
	}
	updated, errUpdate := inst.db.UpdateGameServer(inst.db.DB, setter)
	if errUpdate != nil {
		return fmt.Errorf("persist minecraft server executable: %w", errUpdate)
	}

	gameServer.ServerExecutable = updated.ServerExecutable
	return nil
}
