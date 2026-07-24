package updateproviders

import "testing"

func TestResolveConfigUsesGameDefaultsWhenNoVariantSelected(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind:     ProviderKindSteamCMD,
			SourceID: "294420",
		},
		DefaultTarget: "public",
		ModProfile: &ModProfile{
			InstallPath: "Mods/",
			Sources: []ModSource{
				{ID: "steam_workshop"},
			},
		},
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Provider.Kind != ProviderKindSteamCMD {
		t.Errorf("resolved.Provider.Kind = %q, want %q", resolved.Provider.Kind, ProviderKindSteamCMD)
	}
	if resolved.Provider.SourceID != "294420" {
		t.Errorf("resolved.Provider.SourceID = %q, want %q", resolved.Provider.SourceID, "294420")
	}
	if resolved.Target != "public" {
		t.Errorf("resolved.Target = %q, want %q", resolved.Target, "public")
	}
	if resolved.VariantID != "" {
		t.Errorf("resolved.VariantID = %q, want empty", resolved.VariantID)
	}
	if resolved.ModProfile == nil {
		t.Fatal("resolved.ModProfile = nil, want non-nil")
	}
	if resolved.ModProfile.InstallPath != "Mods/" {
		t.Errorf("resolved.ModProfile.InstallPath = %q, want %q", resolved.ModProfile.InstallPath, "Mods/")
	}
}

func TestResolveConfigAppliesVariantOverrides(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind:     ProviderKindCommand,
			SourceID: "",
		},
		DefaultTarget: "release",
		ModProfile: &ModProfile{
			InstallPath: "mods/",
			Sources: []ModSource{
				{ID: "modrinth"},
			},
		},
		Variants: []Variant{
			{
				ID:   "paper",
				Name: "Paper",
				UpdateProvider: &ProviderConfig{
					Kind:     ProviderKindPaperMC,
					SourceID: "paper",
				},
				DefaultTarget: "1.21.4",
				ModProfile: &ModProfile{
					InstallPath: "plugins/",
					Sources: []ModSource{
						{ID: "hangar"},
					},
				},
			},
		},
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{VariantID: "paper"})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Provider.Kind != ProviderKindPaperMC {
		t.Errorf("resolved.Provider.Kind = %q, want %q", resolved.Provider.Kind, ProviderKindPaperMC)
	}
	if resolved.Provider.SourceID != "paper" {
		t.Errorf("resolved.Provider.SourceID = %q, want %q", resolved.Provider.SourceID, "paper")
	}
	if resolved.Target != "" {
		t.Errorf("resolved.Target = %q, want empty string", resolved.Target)
	}
	if resolved.VariantID != "paper" {
		t.Errorf("resolved.VariantID = %q, want %q", resolved.VariantID, "paper")
	}
	if resolved.VariantName != "Paper" {
		t.Errorf("resolved.VariantName = %q, want %q", resolved.VariantName, "Paper")
	}
	if resolved.ModProfile == nil {
		t.Fatal("resolved.ModProfile = nil, want non-nil")
	}
	if resolved.ModProfile.InstallPath != "plugins/" {
		t.Errorf("resolved.ModProfile.InstallPath = %q, want %q", resolved.ModProfile.InstallPath, "plugins/")
	}
}

func TestResolveConfigUsesServerTargetOverrideForSteamCMD(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind:     ProviderKindSteamCMD,
			SourceID: "294420",
		},
		DefaultTarget: "public",
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{Target: "alpha16"})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Target != "alpha16" {
		t.Errorf("resolved.Target = %q, want %q", resolved.Target, "alpha16")
	}
}

func TestResolveConfigIgnoresStoredTargetsForPaperMCVariants(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind: ProviderKindCommand,
		},
		DefaultTarget: "release",
		Variants: []Variant{
			{
				ID:   "paper",
				Name: "Paper",
				UpdateProvider: &ProviderConfig{
					Kind:     ProviderKindPaperMC,
					SourceID: "paper",
				},
				DefaultTarget: "1.21.4",
			},
		},
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{
		VariantID:    "paper",
		Target:       "1.21.5",
		TargetPinned: false,
	})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Target != "" {
		t.Errorf("resolved.Target = %q, want empty string", resolved.Target)
	}
}

func TestResolveConfigUsesPinnedTargetsForPaperMCVariants(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind: ProviderKindCommand,
		},
		Variants: []Variant{
			{
				ID:   "paper",
				Name: "Paper",
				UpdateProvider: &ProviderConfig{
					Kind:     ProviderKindPaperMC,
					SourceID: "paper",
				},
				DefaultTarget: "1.21.4",
			},
		},
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{
		VariantID:    "paper",
		Target:       "1.21.5",
		TargetPinned: true,
	})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Target != "1.21.5" {
		t.Errorf("resolved.Target = %q, want %q", resolved.Target, "1.21.5")
	}
}

func TestResolveConfigLeavesTargetBlankWhenNoDefaultsAreConfigured(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind:     ProviderKindSteamCMD,
			SourceID: "294420",
		},
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Target != "" {
		t.Errorf("resolved.Target = %q, want empty string", resolved.Target)
	}
}

func TestResolveConfigReturnsErrorForUnknownVariant(t *testing.T) {
	game := GameConfig{
		Variants: []Variant{
			{ID: "paper", Name: "Paper"},
		},
	}

	_, errResolve := ResolveConfig(game, ServerConfig{VariantID: "purpur"})
	if errResolve == nil {
		t.Fatal("ResolveConfig() error = nil, want error")
	}
}

func TestResolveConfigIgnoresStoredVariantWhenGameHasNoVariants(t *testing.T) {
	game := GameConfig{
		UpdateProvider: ProviderConfig{
			Kind:     ProviderKindMojang,
			SourceID: "vanilla",
		},
	}

	resolved, errResolve := ResolveConfig(game, ServerConfig{VariantID: "vanilla"})
	if errResolve != nil {
		t.Fatalf("ResolveConfig() error = %v", errResolve)
	}

	if resolved.Provider.Kind != ProviderKindMojang {
		t.Errorf("resolved.Provider.Kind = %q, want %q", resolved.Provider.Kind, ProviderKindMojang)
	}
	if resolved.Provider.SourceID != "vanilla" {
		t.Errorf("resolved.Provider.SourceID = %q, want %q", resolved.Provider.SourceID, "vanilla")
	}
	if resolved.VariantID != "" {
		t.Errorf("resolved.VariantID = %q, want empty", resolved.VariantID)
	}
}

func TestResetTargetForVariant(t *testing.T) {
	tests := []struct {
		name string
		game GameConfig
		want string
	}{
		{
			name: "uses variant default for non PaperMC variant",
			game: GameConfig{
				DefaultTarget: "public",
				UpdateProvider: ProviderConfig{
					Kind: ProviderKindSteamCMD,
				},
				Variants: []Variant{
					{
						ID:            "experimental",
						DefaultTarget: "beta",
					},
				},
			},
			want: "beta",
		},
		{
			name: "paper variants clear target",
			game: GameConfig{
				DefaultTarget: "public",
				UpdateProvider: ProviderConfig{
					Kind: ProviderKindPaperMC,
				},
				Variants: []Variant{
					{
						ID: "paper",
						UpdateProvider: &ProviderConfig{
							Kind: ProviderKindPaperMC,
						},
						DefaultTarget: "1.21.4",
					},
				},
			},
			want: "",
		},
		{
			name: "falls back to game default",
			game: GameConfig{
				DefaultTarget: "public",
				UpdateProvider: ProviderConfig{
					Kind: ProviderKindSteamCMD,
				},
				Variants: []Variant{
					{
						ID: "vanilla",
					},
				},
			},
			want: "public",
		},
		{
			name: "clears target when no defaults exist",
			game: GameConfig{
				Variants: []Variant{
					{
						ID: "custom",
					},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, errReset := ResetTargetForVariant(tt.game, tt.game.Variants[0].ID)
			if errReset != nil {
				t.Fatalf("ResetTargetForVariant() error = %v", errReset)
			}
			if target != tt.want {
				t.Errorf("ResetTargetForVariant() = %q, want %q", target, tt.want)
			}
		})
	}
}
