// Package updateconfig adapts update-provider domain config to database models.
package updateconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/sql/models"
)

type legacyServerSoftwareEntry struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	JarSource string           `json:"jar_source"`
	ModConfig *legacyModConfig `json:"mod_config"`
}

type legacyModConfig struct {
	ModTypes []legacyModType      `json:"mod_types"`
	Sources  []legacySourceConfig `json:"sources"`
}

type legacyModType struct {
	InstallPath string `json:"install_path"`
}

type legacySourceConfig struct {
	ID           string         `json:"id"`
	SearchParams map[string]any `json:"search_params"`
}

// LoadGameConfigFromModel loads a game's persisted update-provider config.
func LoadGameConfigFromModel(game *models.Game) (updateproviders.GameConfig, error) {
	if game == nil {
		return updateproviders.GameConfig{}, nil
	}

	raw := strings.TrimSpace(game.ServerSoftware.GetOr(""))
	if raw == "" {
		return normalizeGameConfig(deriveGameConfigFromLegacyFields(game)), nil
	}

	if strings.HasPrefix(raw, "[") {
		cfg, errLegacy := legacyServerSoftwareToGameConfig(game, raw)
		if errLegacy != nil {
			return updateproviders.GameConfig{}, errLegacy
		}
		return normalizeGameConfig(cfg), nil
	}

	var cfg updateproviders.GameConfig
	errUnmarshal := json.Unmarshal([]byte(raw), &cfg)
	if errUnmarshal != nil {
		return updateproviders.GameConfig{}, fmt.Errorf("parse game config: %w", errUnmarshal)
	}

	return normalizeGameConfig(cfg), nil
}

// SaveGameConfigToModel stores a normalized game config on the model.
func SaveGameConfigToModel(game *models.Game, cfg updateproviders.GameConfig) error {
	if game == nil {
		return nil
	}

	normalized := normalizeGameConfig(cfg)
	if isEmptyGameConfig(normalized) {
		game.ServerSoftware = models.Game{}.ServerSoftware
		return nil
	}

	data, errMarshal := json.Marshal(normalized)
	if errMarshal != nil {
		return fmt.Errorf("marshal game config: %w", errMarshal)
	}

	game.ServerSoftware = null.From(string(data))
	return nil
}

// ResolveModelConfig loads and resolves effective config from game and server models.
func ResolveModelConfig(game *models.Game, server *models.GameServer) (updateproviders.ResolvedConfig, error) {
	cfg, errConfig := LoadGameConfigFromModel(game)
	if errConfig != nil {
		return updateproviders.ResolvedConfig{}, errConfig
	}

	resolved, errResolve := updateproviders.ResolveConfig(cfg, LoadServerConfigFromModel(server))
	if errResolve != nil {
		return updateproviders.ResolvedConfig{}, fmt.Errorf("resolve update provider config: %w", errResolve)
	}

	resolved.Target = normalizeTarget(resolved.Provider.Kind, resolved.Target)
	return resolved, nil
}

// LoadServerConfigFromModel loads per-server overrides from a server model.
func LoadServerConfigFromModel(server *models.GameServer) updateproviders.ServerConfig {
	if server == nil {
		return updateproviders.ServerConfig{}
	}

	target := strings.TrimSpace(server.Branch)
	if strings.EqualFold(target, "public") {
		target = ""
	}

	return updateproviders.ServerConfig{
		VariantID:    strings.TrimSpace(server.ServerSoftware.GetOr("")),
		Target:       target,
		TargetPinned: server.TargetPinned,
	}
}

// SaveServerConfigToModel stores per-server overrides on a server model.
func SaveServerConfigToModel(server *models.GameServer, cfg updateproviders.ServerConfig, provider updateproviders.ProviderKind) {
	if server == nil {
		return
	}

	server.Branch = normalizeTarget(provider, cfg.Target)
	server.TargetPinned = cfg.TargetPinned
	variantID := strings.TrimSpace(cfg.VariantID)
	if variantID == "" {
		server.ServerSoftware = null.Val[string]{}
		return
	}
	server.ServerSoftware = null.From(variantID)
}

func normalizeGameConfig(cfg updateproviders.GameConfig) updateproviders.GameConfig {
	cfg.UpdateProvider.Kind = normalizeProviderKind(cfg.UpdateProvider.Kind)
	cfg.DefaultTarget = normalizeTarget(cfg.UpdateProvider.Kind, cfg.DefaultTarget)

	if cfg.ModProfile != nil {
		cfg.ModProfile = normalizeModProfile(cfg.ModProfile)
	}

	for index := range cfg.Variants {
		cfg.Variants[index].ID = strings.TrimSpace(cfg.Variants[index].ID)
		cfg.Variants[index].Name = strings.TrimSpace(cfg.Variants[index].Name)
		if cfg.Variants[index].UpdateProvider != nil {
			cfg.Variants[index].UpdateProvider.Kind = normalizeProviderKind(cfg.Variants[index].UpdateProvider.Kind)
		}
		cfg.Variants[index].DefaultTarget = normalizeTarget(providerKindForVariant(cfg, cfg.Variants[index]), cfg.Variants[index].DefaultTarget)
		if cfg.Variants[index].ModProfile != nil {
			cfg.Variants[index].ModProfile = normalizeModProfile(cfg.Variants[index].ModProfile)
		}
	}

	return cfg
}

func normalizeModProfile(profile *updateproviders.ModProfile) *updateproviders.ModProfile {
	if profile == nil {
		return nil
	}

	normalized := &updateproviders.ModProfile{
		InstallPath: strings.TrimSpace(profile.InstallPath),
		Sources:     make([]updateproviders.ModSource, 0, len(profile.Sources)),
	}

	for _, source := range profile.Sources {
		normalized.Sources = append(normalized.Sources, updateproviders.ModSource{
			ID:               strings.TrimSpace(source.ID),
			SearchParamsJSON: strings.TrimSpace(source.SearchParamsJSON),
		})
	}

	return normalized
}

func normalizeProviderKind(kind updateproviders.ProviderKind) updateproviders.ProviderKind {
	switch updateproviders.ProviderKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case updateproviders.ProviderKindSteamCMD:
		return updateproviders.ProviderKindSteamCMD
	case updateproviders.ProviderKindPaperMC:
		return updateproviders.ProviderKindPaperMC
	case updateproviders.ProviderKindMojang:
		return updateproviders.ProviderKindMojang
	case updateproviders.ProviderKindCommand:
		return updateproviders.ProviderKindCommand
	default:
		return updateproviders.ProviderKindNone
	}
}

func normalizeTarget(kind updateproviders.ProviderKind, target string) string {
	if kind == updateproviders.ProviderKindSteamCMD {
		return updateproviders.NormalizeSteamTarget(target)
	}
	return strings.TrimSpace(target)
}

func providerKindForVariant(game updateproviders.GameConfig, variant updateproviders.Variant) updateproviders.ProviderKind {
	if variant.UpdateProvider != nil {
		return variant.UpdateProvider.Kind
	}
	return game.UpdateProvider.Kind
}

func deriveGameConfigFromLegacyFields(game *models.Game) updateproviders.GameConfig {
	cfg := updateproviders.GameConfig{}

	switch {
	case game.UsesSteamcmd:
		cfg.UpdateProvider = updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindSteamCMD}
		cfg.DefaultTarget = "public"
	case strings.EqualFold(game.ID, "minecraft"):
		cfg.UpdateProvider = updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindMojang, SourceID: "vanilla"}
	case hasLegacyUpdateCommand(game):
		cfg.UpdateProvider = updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindCommand}
	default:
		cfg.UpdateProvider = updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindNone}
	}

	return cfg
}

func hasLegacyUpdateCommand(game *models.Game) bool {
	if game == nil {
		return false
	}

	return strings.TrimSpace(game.WindowsUpdateCommand) != "" ||
		strings.TrimSpace(game.LinuxUpdateCommand) != ""
}

func legacyServerSoftwareToGameConfig(game *models.Game, raw string) (updateproviders.GameConfig, error) {
	var legacyEntries []legacyServerSoftwareEntry
	errParse := json.Unmarshal([]byte(raw), &legacyEntries)
	if errParse != nil {
		return updateproviders.GameConfig{}, fmt.Errorf("parse legacy server software: %w", errParse)
	}

	cfg := deriveGameConfigFromLegacyFields(game)
	if len(legacyEntries) > 0 {
		cfg.UpdateProvider = updateproviders.ProviderConfig{Kind: updateproviders.ProviderKindNone}
		cfg.DefaultTarget = ""
	}
	cfg.Variants = make([]updateproviders.Variant, 0, len(legacyEntries))

	for _, entry := range legacyEntries {
		variant := updateproviders.Variant{
			ID:   strings.TrimSpace(entry.ID),
			Name: strings.TrimSpace(entry.Name),
		}

		providerID := legacyProviderIDForGame(game.ID, entry)
		kind := providerKindFromProviderID(providerID)
		if kind != updateproviders.ProviderKindNone {
			variant.UpdateProvider = &updateproviders.ProviderConfig{
				Kind:     kind,
				SourceID: providerSourceID(kind, entry.ID, providerID),
			}
		}

		if entry.ModConfig != nil {
			variant.ModProfile = legacyModConfigToProfile(entry.ModConfig)
		}

		cfg.Variants = append(cfg.Variants, variant)
	}

	return cfg, nil
}

func legacyModConfigToProfile(cfg *legacyModConfig) *updateproviders.ModProfile {
	if cfg == nil {
		return nil
	}

	profile := &updateproviders.ModProfile{
		InstallPath: "",
		Sources:     make([]updateproviders.ModSource, 0, len(cfg.Sources)),
	}

	if len(cfg.ModTypes) > 0 {
		profile.InstallPath = strings.TrimSpace(cfg.ModTypes[0].InstallPath)
	}

	for _, source := range cfg.Sources {
		paramsJSON := ""
		if source.SearchParams != nil {
			data, errMarshal := json.Marshal(source.SearchParams)
			if errMarshal == nil {
				paramsJSON = string(data)
			}
		}
		profile.Sources = append(profile.Sources, updateproviders.ModSource{
			ID:               strings.TrimSpace(source.ID),
			SearchParamsJSON: paramsJSON,
		})
	}

	return profile
}

func legacyProviderIDForGame(gameID string, entry legacyServerSoftwareEntry) string {
	if strings.TrimSpace(entry.JarSource) != "" {
		return strings.TrimSpace(entry.JarSource)
	}
	if strings.EqualFold(gameID, "minecraft") && strings.EqualFold(entry.ID, "vanilla") {
		return "mojang"
	}
	return ""
}

func providerKindFromProviderID(providerID string) updateproviders.ProviderKind {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "papermc":
		return updateproviders.ProviderKindPaperMC
	case "mojang":
		return updateproviders.ProviderKindMojang
	case "steamcmd":
		return updateproviders.ProviderKindSteamCMD
	default:
		return updateproviders.ProviderKindNone
	}
}

func providerSourceID(kind updateproviders.ProviderKind, variantID string, providerID string) string {
	switch kind {
	case updateproviders.ProviderKindPaperMC:
		switch strings.ToLower(strings.TrimSpace(variantID)) {
		case "paper", "folia", "purpur", "velocity", "waterfall":
			return strings.ToLower(strings.TrimSpace(variantID))
		}
		return strings.ToLower(strings.TrimSpace(variantID))
	case updateproviders.ProviderKindMojang:
		return "vanilla"
	case updateproviders.ProviderKindSteamCMD:
		return strings.TrimSpace(providerID)
	default:
		return ""
	}
}

func isEmptyGameConfig(cfg updateproviders.GameConfig) bool {
	return cfg.UpdateProvider.Kind == updateproviders.ProviderKindNone &&
		cfg.DefaultTarget == "" &&
		cfg.ModProfile == nil &&
		len(cfg.Variants) == 0
}
