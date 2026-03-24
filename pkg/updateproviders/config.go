package updateproviders

import "fmt"

type ProviderKind string

const (
	ProviderKindNone     ProviderKind = "none"
	ProviderKindSteamCMD ProviderKind = "steamcmd"
	ProviderKindPaperMC  ProviderKind = "papermc"
	ProviderKindMojang   ProviderKind = "mojang"
	ProviderKindCommand  ProviderKind = "command"
)

type ProviderConfig struct {
	Kind     ProviderKind `json:"kind"`
	SourceID string       `json:"source_id,omitempty"`
}

type ModSource struct {
	ID               string `json:"id"`
	SearchParamsJSON string `json:"search_params_json,omitempty"`
}

type ModProfile struct {
	InstallPath string      `json:"install_path"`
	Sources     []ModSource `json:"sources,omitempty"`
}

type Variant struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	UpdateProvider *ProviderConfig `json:"update_provider,omitempty"`
	DefaultTarget  string          `json:"default_target,omitempty"`
	ModProfile     *ModProfile     `json:"mod_profile,omitempty"`
}

type GameConfig struct {
	UpdateProvider ProviderConfig `json:"update_provider"`
	DefaultTarget  string         `json:"default_target,omitempty"`
	ModProfile     *ModProfile    `json:"mod_profile,omitempty"`
	Variants       []Variant      `json:"variants,omitempty"`
}

type ServerConfig struct {
	VariantID    string `json:"variant_id,omitempty"`
	Target       string `json:"target,omitempty"`
	TargetPinned bool   `json:"target_pinned,omitempty"`
}

type ResolvedConfig struct {
	VariantID   string
	VariantName string
	Provider    ProviderConfig
	Target      string
	ModProfile  *ModProfile
}

func ResolveConfig(game GameConfig, server ServerConfig) (ResolvedConfig, error) {
	resolved := ResolvedConfig{
		Provider:   game.UpdateProvider,
		Target:     game.DefaultTarget,
		ModProfile: game.ModProfile,
	}

	selectedVariantID := server.VariantID
	if selectedVariantID == "" && len(game.Variants) == 1 {
		selectedVariantID = game.Variants[0].ID
	}

	if selectedVariantID != "" {
		variant, ok := findVariant(game.Variants, selectedVariantID)
		if !ok {
			return ResolvedConfig{}, fmt.Errorf("variant %q not found", selectedVariantID)
		}

		resolved.VariantID = variant.ID
		resolved.VariantName = variant.Name

		if variant.UpdateProvider != nil {
			resolved.Provider = *variant.UpdateProvider
		}
		if variant.DefaultTarget != "" {
			resolved.Target = variant.DefaultTarget
		}
		if variant.ModProfile != nil {
			resolved.ModProfile = variant.ModProfile
		}
	}

	if providerUsesExplicitPinning(resolved.Provider.Kind) {
		resolved.Target = ""
	}

	if shouldUseServerTarget(resolved.Provider.Kind, server) {
		resolved.Target = server.Target
	}

	return resolved, nil
}

func ResetTargetForVariant(game GameConfig, variantID string) (string, error) {
	if variantID == "" && len(game.Variants) == 1 {
		variantID = game.Variants[0].ID
	}
	if variantID == "" {
		if providerUsesExplicitPinning(game.UpdateProvider.Kind) {
			return "", nil
		}
		return game.DefaultTarget, nil
	}

	variant, ok := findVariant(game.Variants, variantID)
	if !ok {
		return "", fmt.Errorf("variant %q not found", variantID)
	}

	providerKind := providerKindForVariant(game, variant)
	if providerUsesExplicitPinning(providerKind) {
		return "", nil
	}

	if variant.DefaultTarget != "" {
		return variant.DefaultTarget, nil
	}

	return game.DefaultTarget, nil
}

func providerUsesExplicitPinning(kind ProviderKind) bool {
	return kind == ProviderKindPaperMC || kind == ProviderKindMojang
}

func shouldUseServerTarget(kind ProviderKind, server ServerConfig) bool {
	if server.Target == "" {
		return false
	}
	if providerUsesExplicitPinning(kind) {
		return server.TargetPinned
	}
	return true
}

func findVariant(variants []Variant, variantID string) (Variant, bool) {
	for _, variant := range variants {
		if variant.ID == variantID {
			return variant, true
		}
	}

	return Variant{}, false
}
