package actions

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/pkg/modproviders"
	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/sql/models"
)

var minecraftUpdateProviderLookup = func(kind updateproviders.ProviderKind) (modproviders.ModProvider, bool) {
	switch kind {
	case updateproviders.ProviderKindPaperMC:
		return modproviders.GetProvider("papermc")
	case updateproviders.ProviderKindMojang:
		return modproviders.GetProvider("mojang")
	default:
		return nil, false
	}
}

type minecraftUpdatePlan struct {
	softwareID          string
	softwareName        string
	provider            modproviders.ModProvider
	targetVersion       string
	downloadVersionID   string
	downloadVersionName string
	plannedFileName     string
}

func (inst *Instance) resolveMinecraftUpdatePlan(
	gameServer *models.GameServer,
) (*minecraftUpdatePlan, error) {
	if gameServer.GameID != "minecraft" {
		return nil, ErrNotMinecraftServer
	}
	if gameServer.R.Game == nil {
		return nil, errMinecraftGameRelationNotLoaded
	}

	resolved, errResolve := updateproviders.ResolveModelConfig(gameServer.R.Game, gameServer)
	if errResolve != nil {
		return nil, fmt.Errorf("resolve minecraft update config: %w", errResolve)
	}

	softwareID := strings.TrimSpace(resolved.VariantID)
	if softwareID == "" {
		softwareID = "vanilla"
	}

	if resolved.Provider.Kind != updateproviders.ProviderKindPaperMC &&
		resolved.Provider.Kind != updateproviders.ProviderKindMojang {
		return nil, ErrMinecraftVariantUpdateNotSupported
	}

	provider, ok := minecraftUpdateProviderLookup(resolved.Provider.Kind)
	if !ok {
		return nil, fmt.Errorf("jar source provider not found for %s", resolved.Provider.Kind)
	}

	updateCtx, cancel := context.WithTimeout(inst.ctx, 10*time.Minute)
	defer cancel()

	sourceID := strings.TrimSpace(resolved.Provider.SourceID)
	if sourceID == "" {
		switch resolved.Provider.Kind {
		case updateproviders.ProviderKindPaperMC:
			sourceID = softwareID
		case updateproviders.ProviderKindMojang:
			sourceID = "vanilla"
		}
	}

	details, errDetails := provider.GetModDetails(updateCtx, sourceID, nil)
	if errDetails != nil {
		return nil, fmt.Errorf("get minecraft variant versions: %w", errDetails)
	}
	if details == nil || len(details.Versions) == 0 {
		return nil, fmt.Errorf("no versions available for minecraft variant %s", sourceID)
	}

	targetVersion := resolvePreferredMinecraftTarget(details.Versions, resolved.Target)
	if targetVersion == "" {
		return nil, fmt.Errorf("no usable target version for minecraft variant %s", sourceID)
	}

	downloadVersionID := targetVersion
	downloadVersionName := targetVersion
	plannedFileName := ""

	builds, errBuilds := provider.GetVersions(updateCtx, sourceID, targetVersion, nil)
	if errBuilds == nil && len(builds) > 0 {
		selectedBuild := builds[len(builds)-1]
		if selectedBuild.VersionID != "" {
			downloadVersionID = selectedBuild.VersionID
		}
		if selectedBuild.VersionString != "" {
			downloadVersionName = selectedBuild.VersionString
		}
		if fileName := plannedDownloadFileName(selectedBuild.DownloadURL); fileName != "" {
			plannedFileName = fileName
		}
	}
	if downloadVersionName == "" {
		downloadVersionName = downloadVersionID
	}

	softwareName := strings.TrimSpace(resolved.VariantName)
	if softwareName == "" {
		softwareName = strings.TrimSpace(sourceID)
	}

	return &minecraftUpdatePlan{
		softwareID:          softwareID,
		softwareName:        softwareName,
		provider:            provider,
		targetVersion:       targetVersion,
		downloadVersionID:   downloadVersionID,
		downloadVersionName: downloadVersionName,
		plannedFileName:     plannedFileName,
	}, nil
}

func resolvePreferredMinecraftTarget(versions []modproviders.ModVersion, preferred string) string {
	normalizedPreferred := strings.TrimSpace(preferred)
	if normalizedPreferred != "" {
		for _, version := range versions {
			if strings.TrimSpace(version.VersionID) == normalizedPreferred {
				return normalizedPreferred
			}
			if strings.TrimSpace(version.VersionString) == normalizedPreferred {
				return normalizedPreferred
			}
		}
	}

	if len(versions) == 0 {
		return ""
	}

	latestVersion := versions[0]
	targetVersion := strings.TrimSpace(latestVersion.VersionString)
	if targetVersion == "" {
		targetVersion = strings.TrimSpace(latestVersion.VersionID)
	}
	return targetVersion
}

func plannedDownloadFileName(downloadURL string) string {
	trimmed := strings.TrimSpace(downloadURL)
	if trimmed == "" {
		return ""
	}

	parsedURL, errParse := url.Parse(trimmed)
	if errParse != nil {
		return path.Base(trimmed)
	}
	if parsedURL.Path == "" {
		return ""
	}
	return path.Base(parsedURL.Path)
}

func (inst *Instance) tryUpdateMinecraftServerSoftware(gameServer *models.GameServer) (bool, error) {
	plan, errPlan := inst.resolveMinecraftUpdatePlan(gameServer)
	if errors.Is(errPlan, ErrNotMinecraftServer) {
		return false, nil
	}
	if errPlan != nil {
		return true, errPlan
	}
	if plan == nil {
		return false, nil
	}

	updateCtx, cancel := context.WithTimeout(inst.ctx, 10*time.Minute)
	defer cancel()

	files, errDownload := plan.provider.Download(
		updateCtx,
		plan.softwareID,
		plan.downloadVersionID,
		gameServer.Directory,
	)
	if errDownload != nil {
		return true, fmt.Errorf("download minecraft update: %w", errDownload)
	}

	newExecutable := primaryDownloadedFile(files)
	if oldExecutable := gameServer.ServerExecutable.GetOr(""); oldExecutable != "" && oldExecutable != newExecutable {
		oldPath := filepath.Join(gameServer.Directory, oldExecutable)
		if errRemove := os.Remove(oldPath); errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("game_server_id", gameServer.ID).Str("path", oldPath).
				Msg("Failed to remove superseded game server executable")
		}
	}

	if inst.db == nil {
		if newExecutable == "" {
			gameServer.ServerExecutable = null.FromPtr[string](nil)
		} else {
			gameServer.ServerExecutable = null.From(newExecutable)
		}
		gameServer.ServerSoftware = null.From(plan.softwareID)
		return true, nil
	}

	setter := &models.GameServerSetter{
		ID:             omit.From(gameServer.ID),
		ServerSoftware: omitnull.From(plan.softwareID),
	}
	if newExecutable == "" {
		setter.ServerExecutable = omitnull.FromNull(null.Val[string]{})
	} else {
		setter.ServerExecutable = omitnull.From(newExecutable)
	}

	updated, errUpdate := inst.db.UpdateGameServer(inst.db.DB, setter)
	if errUpdate != nil {
		return true, fmt.Errorf("persist minecraft update: %w", errUpdate)
	}
	gameServer.ServerExecutable = updated.ServerExecutable
	gameServer.ServerSoftware = updated.ServerSoftware
	return true, nil
}

func primaryDownloadedFile(files []modproviders.DownloadedFile) string {
	for _, f := range files {
		if f.IsPrimary {
			return f.Path
		}
	}
	return ""
}
