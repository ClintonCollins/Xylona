package updateproviders

import (
	"encoding/json"
	"strings"

	"github.com/ClintonCollins/Xylona/proto/go/xylona"
)

func ProviderConfigToProto(cfg ProviderConfig) *xylona.UpdateProviderConfig {
	return &xylona.UpdateProviderConfig{
		Kind:     providerKindToProto(cfg.Kind),
		SourceId: cfg.SourceID,
	}
}

func ProviderConfigFromProto(proto *xylona.UpdateProviderConfig) ProviderConfig {
	if proto == nil {
		return ProviderConfig{}
	}

	return ProviderConfig{
		Kind:     providerKindFromProto(proto.GetKind()),
		SourceID: strings.TrimSpace(proto.GetSourceId()),
	}
}

func ModProfileToProto(profile *ModProfile) *xylona.ModProfile {
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

func ModProfileFromProto(proto *xylona.ModProfile) *ModProfile {
	if proto == nil {
		return nil
	}

	sources := make([]ModSource, 0, len(proto.GetSources()))
	for _, source := range proto.GetSources() {
		sources = append(sources, ModSource{
			ID:               strings.TrimSpace(source.GetId()),
			SearchParamsJSON: strings.TrimSpace(source.GetSearchParamsJson()),
		})
	}

	return &ModProfile{
		InstallPath: strings.TrimSpace(proto.GetInstallPath()),
		Sources:     sources,
	}
}

func VariantsToProto(variants []Variant) []*xylona.Variant {
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

func VariantsFromProto(protoVariants []*xylona.Variant) []Variant {
	variants := make([]Variant, 0, len(protoVariants))
	for _, protoVariant := range protoVariants {
		var updateProvider *ProviderConfig
		if protoVariant.GetUpdateProvider() != nil {
			cfg := ProviderConfigFromProto(protoVariant.GetUpdateProvider())
			updateProvider = &cfg
		}
		variants = append(variants, Variant{
			ID:             strings.TrimSpace(protoVariant.GetId()),
			Name:           strings.TrimSpace(protoVariant.GetName()),
			UpdateProvider: updateProvider,
			DefaultTarget:  strings.TrimSpace(protoVariant.GetDefaultTarget()),
			ModProfile:     ModProfileFromProto(protoVariant.GetModProfile()),
		})
	}
	return variants
}

func SearchParams(source ModSource) map[string]any {
	raw := strings.TrimSpace(source.SearchParamsJSON)
	if raw == "" {
		return nil
	}

	var parsed map[string]any
	errUnmarshal := json.Unmarshal([]byte(raw), &parsed)
	if errUnmarshal != nil {
		return nil
	}
	return parsed
}

func providerKindToProto(kind ProviderKind) xylona.UpdateProviderKind {
	switch normalizeProviderKind(kind) {
	case ProviderKindSteamCMD:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_STEAMCMD
	case ProviderKindPaperMC:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_PAPERMC
	case ProviderKindMojang:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_MOJANG
	case ProviderKindCommand:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND
	default:
		return xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_NONE
	}
}

func providerKindFromProto(kind xylona.UpdateProviderKind) ProviderKind {
	switch kind {
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_STEAMCMD:
		return ProviderKindSteamCMD
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_PAPERMC:
		return ProviderKindPaperMC
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_MOJANG:
		return ProviderKindMojang
	case xylona.UpdateProviderKind_UPDATE_PROVIDER_KIND_COMMAND:
		return ProviderKindCommand
	default:
		return ProviderKindNone
	}
}
