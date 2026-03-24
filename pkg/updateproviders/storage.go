package updateproviders

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/aarondl/opt/null"

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

func LoadGameConfigFromModel(game *models.Game) (GameConfig, error) {
	if game == nil {
		return GameConfig{}, nil
	}

	raw := strings.TrimSpace(game.ServerSoftware.GetOr(""))
	if raw == "" {
		return normalizeGameConfig(deriveGameConfigFromLegacyFields(game)), nil
	}

	if strings.HasPrefix(raw, "[") {
		cfg, errLegacy := legacyServerSoftwareToGameConfig(game, raw)
		if errLegacy != nil {
			return GameConfig{}, errLegacy
		}
		return normalizeGameConfig(cfg), nil
	}

	var cfg GameConfig
	errUnmarshal := json.Unmarshal([]byte(raw), &cfg)
	if errUnmarshal != nil {
		return GameConfig{}, fmt.Errorf("parse game config: %w", errUnmarshal)
	}

	return normalizeGameConfig(cfg), nil
}

func SaveGameConfigToModel(game *models.Game, cfg GameConfig) error {
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

func ResolveModelConfig(game *models.Game, server *models.GameServer) (ResolvedConfig, error) {
	cfg, errConfig := LoadGameConfigFromModel(game)
	if errConfig != nil {
		return ResolvedConfig{}, errConfig
	}

	resolved, errResolve := ResolveConfig(cfg, LoadServerConfigFromModel(server))
	if errResolve != nil {
		return ResolvedConfig{}, errResolve
	}

	resolved.Target = normalizeTarget(resolved.Provider.Kind, resolved.Target)
	return resolved, nil
}

func LoadServerConfigFromModel(server *models.GameServer) ServerConfig {
	if server == nil {
		return ServerConfig{}
	}

	target := strings.TrimSpace(server.Branch)
	if strings.EqualFold(target, "public") {
		target = ""
	}

	return ServerConfig{
		VariantID:    strings.TrimSpace(server.ServerSoftware.GetOr("")),
		Target:       target,
		TargetPinned: server.TargetPinned,
	}
}

func SaveServerConfigToModel(server *models.GameServer, cfg ServerConfig, provider ProviderKind) {
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

func normalizeGameConfig(cfg GameConfig) GameConfig {
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

func normalizeModProfile(profile *ModProfile) *ModProfile {
	if profile == nil {
		return nil
	}

	normalized := &ModProfile{
		InstallPath: strings.TrimSpace(profile.InstallPath),
		Sources:     make([]ModSource, 0, len(profile.Sources)),
	}

	for _, source := range profile.Sources {
		normalized.Sources = append(normalized.Sources, ModSource{
			ID:               strings.TrimSpace(source.ID),
			SearchParamsJSON: strings.TrimSpace(source.SearchParamsJSON),
		})
	}

	return normalized
}

func normalizeProviderKind(kind ProviderKind) ProviderKind {
	switch ProviderKind(strings.ToLower(strings.TrimSpace(string(kind)))) {
	case ProviderKindSteamCMD:
		return ProviderKindSteamCMD
	case ProviderKindPaperMC:
		return ProviderKindPaperMC
	case ProviderKindMojang:
		return ProviderKindMojang
	case ProviderKindCommand:
		return ProviderKindCommand
	default:
		return ProviderKindNone
	}
}

func normalizeTarget(kind ProviderKind, target string) string {
	trimmed := strings.TrimSpace(target)
	if kind == ProviderKindSteamCMD && trimmed == "" {
		return "public"
	}
	return trimmed
}

func NormalizeSteamTarget(target string) string {
	return normalizeTarget(ProviderKindSteamCMD, target)
}

func providerKindForVariant(game GameConfig, variant Variant) ProviderKind {
	if variant.UpdateProvider != nil {
		return variant.UpdateProvider.Kind
	}
	return game.UpdateProvider.Kind
}

func deriveGameConfigFromLegacyFields(game *models.Game) GameConfig {
	cfg := GameConfig{}

	switch {
	case game.UsesSteamcmd:
		cfg.UpdateProvider = ProviderConfig{Kind: ProviderKindSteamCMD}
		cfg.DefaultTarget = "public"
	case strings.EqualFold(game.ID, "minecraft"):
		cfg.UpdateProvider = ProviderConfig{Kind: ProviderKindMojang, SourceID: "vanilla"}
	case hasLegacyUpdateCommand(game):
		cfg.UpdateProvider = ProviderConfig{Kind: ProviderKindCommand}
	default:
		cfg.UpdateProvider = ProviderConfig{Kind: ProviderKindNone}
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

func legacyServerSoftwareToGameConfig(game *models.Game, raw string) (GameConfig, error) {
	var legacyEntries []legacyServerSoftwareEntry
	errParse := json.Unmarshal([]byte(raw), &legacyEntries)
	if errParse != nil {
		return GameConfig{}, fmt.Errorf("parse legacy server software: %w", errParse)
	}

	cfg := deriveGameConfigFromLegacyFields(game)
	if len(legacyEntries) > 0 {
		cfg.UpdateProvider = ProviderConfig{Kind: ProviderKindNone}
		cfg.DefaultTarget = ""
	}
	cfg.Variants = make([]Variant, 0, len(legacyEntries))

	for _, entry := range legacyEntries {
		variant := Variant{
			ID:   strings.TrimSpace(entry.ID),
			Name: strings.TrimSpace(entry.Name),
		}

		providerID := legacyProviderIDForGame(game.ID, entry)
		if kind := providerKindFromProviderID(providerID); kind != ProviderKindNone {
			variant.UpdateProvider = &ProviderConfig{
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

func legacyModConfigToProfile(cfg *legacyModConfig) *ModProfile {
	if cfg == nil {
		return nil
	}

	profile := &ModProfile{
		InstallPath: "",
		Sources:     make([]ModSource, 0, len(cfg.Sources)),
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
		profile.Sources = append(profile.Sources, ModSource{
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

func providerKindFromProviderID(providerID string) ProviderKind {
	switch strings.ToLower(strings.TrimSpace(providerID)) {
	case "papermc":
		return ProviderKindPaperMC
	case "mojang":
		return ProviderKindMojang
	case "steamcmd":
		return ProviderKindSteamCMD
	default:
		return ProviderKindNone
	}
}

func providerSourceID(kind ProviderKind, variantID string, providerID string) string {
	switch kind {
	case ProviderKindPaperMC:
		switch strings.ToLower(strings.TrimSpace(variantID)) {
		case "paper", "folia", "purpur", "velocity", "waterfall":
			return strings.ToLower(strings.TrimSpace(variantID))
		}
		return strings.ToLower(strings.TrimSpace(variantID))
	case ProviderKindMojang:
		return "vanilla"
	case ProviderKindSteamCMD:
		return strings.TrimSpace(providerID)
	default:
		return ""
	}
}

func isEmptyGameConfig(cfg GameConfig) bool {
	return cfg.UpdateProvider.Kind == ProviderKindNone &&
		cfg.DefaultTarget == "" &&
		cfg.ModProfile == nil &&
		len(cfg.Variants) == 0
}
