package actions

import (
	"fmt"
	"path"
	"strings"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"

	"github.com/ClintonCollins/Xylona/internal/node"
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

	discoveredExecutable, errDiscover := inst.discoverMinecraftExecutable(gameServer)
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

func (inst *Instance) discoverMinecraftExecutable(gameServer *models.GameServer) (string, error) {
	if inst.shouldUseRemoteNodeFiles(gameServer.NodeID) {
		return inst.discoverRemoteMinecraftExecutable(gameServer)
	}

	discoveredExecutable, errDiscover := versiontracker.DiscoverMinecraftExecutable(
		gameServer.Directory,
		gameServer.ServerSoftware.GetOr(""),
	)
	if errDiscover != nil {
		return "", fmt.Errorf("discover local minecraft executable: %w", errDiscover)
	}
	return discoveredExecutable, nil
}

func (inst *Instance) discoverRemoteMinecraftExecutable(gameServer *models.GameServer) (string, error) {
	client, errClient := inst.nodeRegistry.Get(gameServer.NodeID)
	if errClient != nil {
		return "", fmt.Errorf("resolve node client for minecraft executable discovery: %w", errClient)
	}

	entries, errList := client.ListFiles(inst.actionContext(), gameServer.Directory, "")
	if errList != nil {
		return "", fmt.Errorf("list minecraft server files: %w", errList)
	}

	fallbackExecutable := ""
	candidates := []string{}
	for _, entry := range entries {
		if entry.IsDirectory {
			continue
		}

		entryName := strings.TrimSpace(entry.Name)
		if !isRemoteRootJarName(entryName) {
			continue
		}
		if fallbackExecutable == "" {
			fallbackExecutable = entryName
		}
		candidates = append(candidates, entryName)
	}

	if len(candidates) == 0 {
		return "", nil
	}

	result, errProbe := client.ProbeInstalledVersion(inst.actionContext(), node.InstalledVersionProbeRequest{
		Directory:     gameServer.Directory,
		Kind:          node.InstalledVersionProbeKindMinecraftJar,
		RelativePaths: candidates,
	})
	if errProbe != nil {
		return "", fmt.Errorf("probe remote minecraft executable candidates: %w", errProbe)
	}
	if result.Found && strings.TrimSpace(result.SourcePath) != "" {
		return strings.TrimSpace(result.SourcePath), nil
	}

	return fallbackExecutable, nil
}

func isRemoteRootJarName(name string) bool {
	if name == "" || strings.Contains(name, "/") || strings.Contains(name, `\`) {
		return false
	}
	cleaned := path.Clean(name)
	if cleaned != name || cleaned == "." || cleaned == ".." || hasWindowsDrivePrefix(cleaned) {
		return false
	}
	return strings.EqualFold(path.Ext(cleaned), ".jar")
}
