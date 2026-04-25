package updateconfig

import (
	"testing"

	"github.com/aarondl/opt/null"

	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/sql/models"
)

func TestLoadGameConfigFromModel_LegacyServerSoftwareJSON(t *testing.T) {
	game := &models.Game{
		ID: "minecraft",
		ServerSoftware: null.From(`[
			{
				"id":"paper",
				"name":"Paper",
				"jar_source":"papermc",
				"mod_config":{
					"mod_types":[{"type":"plugin","label":"Plugins","install_path":"plugins/"}],
					"sources":[{"id":"modrinth","search_params":{"facets":{"project_type":"plugin"}}}]
				}
			}
		]`),
	}

	cfg, errLoad := LoadGameConfigFromModel(game)
	if errLoad != nil {
		t.Fatalf("LoadGameConfigFromModel() error = %v", errLoad)
	}

	if len(cfg.Variants) != 1 {
		t.Fatalf("LoadGameConfigFromModel() variants = %d, want 1", len(cfg.Variants))
	}

	variant := cfg.Variants[0]
	if variant.ID != "paper" {
		t.Fatalf("variant.ID = %q, want %q", variant.ID, "paper")
	}
	if variant.UpdateProvider == nil || variant.UpdateProvider.Kind != updateproviders.ProviderKindPaperMC {
		t.Fatalf("variant.UpdateProvider.Kind = %v, want %v", variant.UpdateProvider, updateproviders.ProviderKindPaperMC)
	}
	if variant.ModProfile == nil {
		t.Fatalf("variant.ModProfile = nil, want value")
	}
	if variant.ModProfile.InstallPath != "plugins/" {
		t.Fatalf("variant.ModProfile.InstallPath = %q, want %q", variant.ModProfile.InstallPath, "plugins/")
	}
	if len(variant.ModProfile.Sources) != 1 {
		t.Fatalf("variant.ModProfile.Sources = %d, want 1", len(variant.ModProfile.Sources))
	}
	if variant.ModProfile.Sources[0].SearchParamsJSON == "" {
		t.Fatalf("variant.ModProfile.Sources[0].SearchParamsJSON is empty, want serialized params")
	}
}

func TestLoadGameConfigFromModel_DerivesSteamCMDDefaults(t *testing.T) {
	game := &models.Game{
		ID:           "7_days_to_die",
		UsesSteamcmd: true,
	}

	cfg, errLoad := LoadGameConfigFromModel(game)
	if errLoad != nil {
		t.Fatalf("LoadGameConfigFromModel() error = %v", errLoad)
	}

	if cfg.UpdateProvider.Kind != updateproviders.ProviderKindSteamCMD {
		t.Fatalf("cfg.UpdateProvider.Kind = %q, want %q", cfg.UpdateProvider.Kind, updateproviders.ProviderKindSteamCMD)
	}
	if cfg.DefaultTarget != "public" {
		t.Fatalf("cfg.DefaultTarget = %q, want %q", cfg.DefaultTarget, "public")
	}
}

func TestResolveModelConfig_UsesStoredVariantAndTarget(t *testing.T) {
	game := &models.Game{
		ServerSoftware: null.From(`{
			"variants":[
				{
					"id":"paper",
					"name":"Paper",
					"update_provider":{"kind":"papermc","source_id":"paper"},
					"default_target":"1.21.4"
				}
			]
		}`),
	}
	server := &models.GameServer{
		ServerSoftware: null.From("paper"),
		Branch:         "1.21.5",
	}

	resolved, errResolve := ResolveModelConfig(game, server)
	if errResolve != nil {
		t.Fatalf("ResolveModelConfig() error = %v", errResolve)
	}

	if resolved.VariantID != "paper" {
		t.Fatalf("resolved.VariantID = %q, want %q", resolved.VariantID, "paper")
	}
	if resolved.Provider.Kind != updateproviders.ProviderKindPaperMC {
		t.Fatalf("resolved.Provider.Kind = %q, want %q", resolved.Provider.Kind, updateproviders.ProviderKindPaperMC)
	}
	if resolved.Provider.SourceID != "paper" {
		t.Fatalf("resolved.Provider.SourceID = %q, want %q", resolved.Provider.SourceID, "paper")
	}
	if resolved.Target != "" {
		t.Fatalf("resolved.Target = %q, want empty string", resolved.Target)
	}
}

func TestResolveModelConfig_UsesPinnedTargetWhenExplicitlyPinned(t *testing.T) {
	game := &models.Game{
		ServerSoftware: null.From(`{
			"variants":[
				{
					"id":"paper",
					"name":"Paper",
					"update_provider":{"kind":"papermc","source_id":"paper"},
					"default_target":"1.21.4"
				}
			]
		}`),
	}
	server := &models.GameServer{
		ServerSoftware: null.From("paper"),
		Branch:         "1.21.5",
		TargetPinned:   true,
	}

	resolved, errResolve := ResolveModelConfig(game, server)
	if errResolve != nil {
		t.Fatalf("ResolveModelConfig() error = %v", errResolve)
	}

	if resolved.Target != "1.21.5" {
		t.Fatalf("resolved.Target = %q, want %q", resolved.Target, "1.21.5")
	}
}
