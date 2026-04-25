package protomap

import (
	"strings"

	"github.com/ClintonCollins/Xylona/pkg/updateproviders"
	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

// ProviderConfigToProto converts a provider config to its protobuf form.
func ProviderConfigToProto(cfg updateproviders.ProviderConfig) *xylona.UpdateProviderConfig {
	return &xylona.UpdateProviderConfig{
		Kind:     providerKindToProto(cfg.Kind),
		SourceId: cfg.SourceID,
	}
}

// ProviderConfigFromProto converts a protobuf provider config to its local form.
func ProviderConfigFromProto(proto *xylona.UpdateProviderConfig) updateproviders.ProviderConfig {
	if proto == nil {
		return updateproviders.ProviderConfig{}
	}

	return updateproviders.ProviderConfig{
		Kind:     providerKindFromProto(proto.GetKind()),
		SourceID: strings.TrimSpace(proto.GetSourceId()),
	}
}

// ModProfileToProto converts a mod profile to protobuf form.
func ModProfileToProto(profile *updateproviders.ModProfile) *xylona.ModProfile {
	if profile == nil {
		return nil
	}

	sources := make([]*xylona.ModSource, 0, len(profile.Sources))
	for _, source := range profile.Sources {
		sources = append(sources, &xylona.ModSource{
			Id:               source.ID,
			SearchParamsJson: source.SearchParamsJSON,
		})
	}

	return &xylona.ModProfile{
		InstallPath: profile.InstallPath,
		Sources:     sources,
	}
}

// ModProfileFromProto converts a protobuf mod profile to local form.
func ModProfileFromProto(proto *xylona.ModProfile) *updateproviders.ModProfile {
	if proto == nil {
		return nil
	}

	sources := make([]updateproviders.ModSource, 0, len(proto.GetSources()))
	for _, source := range proto.GetSources() {
		sources = append(sources, updateproviders.ModSource{
			ID:               strings.TrimSpace(source.GetId()),
			SearchParamsJSON: strings.TrimSpace(source.GetSearchParamsJson()),
		})
	}

	return &updateproviders.ModProfile{
		InstallPath: strings.TrimSpace(proto.GetInstallPath()),
		Sources:     sources,
	}
}

// VariantsToProto converts variants to protobuf form.
func VariantsToProto(variants []updateproviders.Variant) []*xylona.Variant {
	protoVariants := make([]*xylona.Variant, 0, len(variants))
	for _, variant := range variants {
		var updateProvider *xylona.UpdateProviderConfig
		if variant.UpdateProvider != nil {
			updateProvider = ProviderConfigToProto(*variant.UpdateProvider)
		}
		protoVariants = append(protoVariants, &xylona.Variant{
			Id:             variant.ID,
			Name:           variant.Name,
			UpdateProvider: updateProvider,
			DefaultTarget:  variant.DefaultTarget,
			ModProfile:     ModProfileToProto(variant.ModProfile),
		})
	}
	return protoVariants
}

// VariantsFromProto converts protobuf variants to local form.
func VariantsFromProto(protoVariants []*xylona.Variant) []updateproviders.Variant {
	variants := make([]updateproviders.Variant, 0, len(protoVariants))
	for _, protoVariant := range protoVariants {
		var updateProvider *updateproviders.ProviderConfig
		if protoVariant.GetUpdateProvider() != nil {
			cfg := ProviderConfigFromProto(protoVariant.GetUpdateProvider())
			updateProvider = &cfg
		}
		variants = append(variants, updateproviders.Variant{
			ID:             strings.TrimSpace(protoVariant.GetId()),
			Name:           strings.TrimSpace(protoVariant.GetName()),
			UpdateProvider: updateProvider,
			DefaultTarget:  strings.TrimSpace(protoVariant.GetDefaultTarget()),
			ModProfile:     ModProfileFromProto(protoVariant.GetModProfile()),
		})
	}
	return variants
}

func providerKindToProto(kind updateproviders.ProviderKind) xylona.UpdateProviderKind {
	switch normalizeProviderKind(kind) {
	case updateproviders.ProviderKindSteamCMD:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_STEAMCMD
	case updateproviders.ProviderKindPaperMC:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_PAPERMC
	case updateproviders.ProviderKindMojang:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_MOJANG
	case updateproviders.ProviderKindCommand:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND
	default:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_NONE
	}
}

func providerKindFromProto(kind xylona.UpdateProviderKind) updateproviders.ProviderKind {
	switch kind {
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_STEAMCMD:
		return updateproviders.ProviderKindSteamCMD
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_PAPERMC:
		return updateproviders.ProviderKindPaperMC
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_MOJANG:
		return updateproviders.ProviderKindMojang
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND:
		return updateproviders.ProviderKindCommand
	default:
		return updateproviders.ProviderKindNone
	}
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
