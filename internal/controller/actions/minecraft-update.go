package actions

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aarondl/opt/null"
	"github.com/aarondl/opt/omit"
	"github.com/aarondl/opt/omitnull"
	"github.com/rs/zerolog/log"

	"github.com/ClintonCollins/Xylona/internal/node"
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
	providerKind        updateproviders.ProviderKind
	sourceID            string
	provider            modproviders.ModProvider
	targetVersion       string
	downloadVersionID   string
	downloadVersionName string
	downloadURL         string
	downloadSize        int64
	downloadSHA256      string
	downloadSHA1        string
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
	downloadURL := ""
	downloadSize := int64(0)
	downloadSHA256 := ""
	downloadSHA1 := ""
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
		downloadURL = strings.TrimSpace(selectedBuild.DownloadURL)
		downloadSize = selectedBuild.FileSize
		downloadSHA256 = selectedBuild.FileHashSHA256
		downloadSHA1 = selectedBuild.FileHashSHA1
		fileName := plannedDownloadFileName(selectedBuild.DownloadURL)
		if fileName != "" {
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
		providerKind:        resolved.Provider.Kind,
		sourceID:            sourceID,
		provider:            provider,
		targetVersion:       targetVersion,
		downloadVersionID:   downloadVersionID,
		downloadVersionName: downloadVersionName,
		downloadURL:         downloadURL,
		downloadSize:        downloadSize,
		downloadSHA256:      downloadSHA256,
		downloadSHA1:        downloadSHA1,
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

	if inst.shouldUseRemoteNodeFiles(gameServer.NodeID) {
		errRemoteUpdate := inst.updateRemoteMinecraftServerSoftware(updateCtx, gameServer, plan)
		return true, errRemoteUpdate
	}

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
	oldExecutable := gameServer.ServerExecutable.GetOr("")
	if oldExecutable != "" && oldExecutable != newExecutable {
		oldPath := filepath.Join(gameServer.Directory, oldExecutable)
		errRemove := os.Remove(oldPath)
		if errRemove != nil && !errors.Is(errRemove, os.ErrNotExist) {
			log.Warn().Err(errRemove).Str("game_server_id", gameServer.ID).Str("path", oldPath).
				Msg("Failed to remove superseded game server executable")
		}
	}

	errPersist := inst.persistMinecraftUpdateResult(gameServer, plan.softwareID, newExecutable)
	if errPersist != nil {
		return true, errPersist
	}
	return true, nil
}

func (inst *Instance) updateRemoteMinecraftServerSoftware(
	ctx context.Context,
	gameServer *models.GameServer,
	plan *minecraftUpdatePlan,
) error {
	downloadURL := plan.downloadURL
	if downloadURL == "" {
		downloadURL = remoteMinecraftDownloadURL(plan)
	}
	if downloadURL == "" {
		return fmt.Errorf("download minecraft update: remote download URL unavailable for %s %s", plan.softwareID, plan.downloadVersionID)
	}

	client, errClient := inst.nodeRegistry.Get(gameServer.NodeID)
	if errClient != nil {
		return fmt.Errorf("resolve node client for minecraft update: %w", errClient)
	}

	integrity := node.DownloadIntegrity{
		ExpectedSize:   plan.downloadSize,
		ExpectedSHA256: plan.downloadSHA256,
		ExpectedSHA1:   plan.downloadSHA1,
	}
	if !integrity.HasExpectedMetadata() {
		return fmt.Errorf("download remote minecraft update: integrity metadata unavailable for %s %s", plan.softwareID, plan.downloadVersionID)
	}

	downloadResult, errDownload := client.DownloadFileFromURL(ctx, gameServer.Directory, downloadURL, "", integrity, node.ProtectionPolicy{})
	if errDownload != nil {
		return fmt.Errorf("download remote minecraft update: %w", errDownload)
	}

	newExecutable := path.Base(downloadResult.RelativePath)
	oldExecutable := gameServer.ServerExecutable.GetOr("")
	if oldExecutable != "" && oldExecutable != newExecutable {
		_, errRemove := client.DeleteFiles(ctx, gameServer.Directory, []string{oldExecutable}, node.ProtectionPolicy{})
		if errRemove != nil {
			log.Warn().Err(errRemove).Str("game_server_id", gameServer.ID).Str("path", oldExecutable).
				Msg("Failed to remove superseded remote game server executable")
		}
	}

	errPersist := inst.persistMinecraftUpdateResult(gameServer, plan.softwareID, newExecutable)
	if errPersist != nil {
		return errPersist
	}
	return nil
}

func remoteMinecraftDownloadURL(plan *minecraftUpdatePlan) string {
	if plan == nil || plan.providerKind != updateproviders.ProviderKindPaperMC {
		return ""
	}

	version, buildNumber, ok := splitPaperMCVersionID(plan.downloadVersionID)
	if !ok {
		return ""
	}

	sourceID := strings.TrimSpace(plan.sourceID)
	if sourceID == "" {
		sourceID = strings.TrimSpace(plan.softwareID)
	}
	if sourceID == "" {
		return ""
	}

	baseURL := "https://api.papermc.io/v2"
	if sourceID == "purpur" {
		baseURL = "https://api.purpurmc.org/v2"
	}

	fileName := fmt.Sprintf("%s-%s-%d.jar", sourceID, version, buildNumber)
	return fmt.Sprintf("%s/projects/%s/versions/%s/builds/%d/downloads/%s", baseURL, sourceID, version, buildNumber, fileName)
}

func splitPaperMCVersionID(versionID string) (string, int, bool) {
	trimmed := strings.TrimSpace(versionID)
	lastDash := strings.LastIndex(trimmed, "-")
	if lastDash < 0 || lastDash == len(trimmed)-1 {
		return "", 0, false
	}

	buildNumber, errParse := strconv.Atoi(trimmed[lastDash+1:])
	if errParse != nil {
		return "", 0, false
	}

	version := strings.TrimSpace(trimmed[:lastDash])
	if version == "" {
		return "", 0, false
	}
	return version, buildNumber, true
}

func (inst *Instance) persistMinecraftUpdateResult(gameServer *models.GameServer, softwareID string, newExecutable string) error {
	if inst.db == nil {
		if newExecutable == "" {
			gameServer.ServerExecutable = null.FromPtr[string](nil)
		} else {
			gameServer.ServerExecutable = null.From(newExecutable)
		}
		gameServer.ServerSoftware = null.From(softwareID)
		return nil
	}

	setter := &models.GameServerSetter{
		ID:             omit.From(gameServer.ID),
		ServerSoftware: omitnull.From(softwareID),
	}
	if newExecutable == "" {
		setter.ServerExecutable = omitnull.FromNull(null.Val[string]{})
	} else {
		setter.ServerExecutable = omitnull.From(newExecutable)
	}

	updated, errUpdate := inst.db.UpdateGameServer(inst.db.DB, setter)
	if errUpdate != nil {
		return fmt.Errorf("persist minecraft update: %w", errUpdate)
	}
	gameServer.ServerExecutable = updated.ServerExecutable
	gameServer.ServerSoftware = updated.ServerSoftware
	return nil
}

func primaryDownloadedFile(files []modproviders.DownloadedFile) string {
	for _, f := range files {
		if f.IsPrimary {
			return f.Path
		}
	}
	return ""
}
