import { create } from '@bufbuild/protobuf'

import {
  ModProfileSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
  VariantSchema,
  type Game,
  type ModProfile,
  type UpdateProviderConfig,
  type Variant,
} from '@/proto/shared_pb'

export interface GamePreset {
  id: string
  label: string
  description: string
  updateProvider?: UpdateProviderConfig
  defaultTarget?: string
  modProfile?: ModProfile
  variants?: Variant[]
}

function buildModProfile(
  installPath: string,
  sources: Array<{ id: string; searchParamsJson: string }>,
) {
  return create(ModProfileSchema, {
    installPath,
    sources: sources.map((source) => ({
      id: source.id,
      searchParamsJson: source.searchParamsJson,
    })),
  })
}

function buildVariant(init: {
  id: string
  name: string
  updateProvider?: UpdateProviderConfig
  defaultTarget?: string
  modProfile?: ModProfile
}): Variant {
  return create(VariantSchema, init)
}

function buildUpdateProvider(kind: UpdateProviderKind, sourceId = ''): UpdateProviderConfig {
  return create(UpdateProviderConfigSchema, {
    kind,
    sourceId,
  })
}

export const gamePresets: GamePreset[] = [
  {
    id: 'minecraft',
    label: 'Minecraft: Java Edition',
    description: 'Vanilla, Paper, Purpur, Fabric, and Folia variants with built-in mod support.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    variants: [
      buildVariant({
        id: 'vanilla',
        name: 'Vanilla',
        updateProvider: buildUpdateProvider(UpdateProviderKind.MOJANG, 'vanilla'),
      }),
      buildVariant({
        id: 'paper',
        name: 'Paper',
        updateProvider: buildUpdateProvider(UpdateProviderKind.PAPERMC, 'paper'),
        modProfile: buildModProfile('plugins/', [
          {
            id: 'modrinth',
            searchParamsJson:
              '{"facets":{"project_type":"plugin","categories":["paper","spigot","bukkit"]}}',
          },
          {
            id: 'hangar',
            searchParamsJson: '{"platform":"PAPER"}',
          },
        ]),
      }),
      buildVariant({
        id: 'purpur',
        name: 'Purpur',
        updateProvider: buildUpdateProvider(UpdateProviderKind.PAPERMC, 'purpur'),
        modProfile: buildModProfile('plugins/', [
          {
            id: 'modrinth',
            searchParamsJson:
              '{"facets":{"project_type":"plugin","categories":["purpur","paper","spigot","bukkit"]}}',
          },
          {
            id: 'hangar',
            searchParamsJson: '{"platform":"PAPER"}',
          },
        ]),
      }),
      buildVariant({
        id: 'fabric',
        name: 'Fabric',
        updateProvider: buildUpdateProvider(UpdateProviderKind.COMMAND),
        modProfile: buildModProfile('mods/', [
          {
            id: 'modrinth',
            searchParamsJson: '{"facets":{"project_type":"mod","categories":["fabric"]}}',
          },
        ]),
      }),
      buildVariant({
        id: 'folia',
        name: 'Folia',
        updateProvider: buildUpdateProvider(UpdateProviderKind.PAPERMC, 'folia'),
        modProfile: buildModProfile('plugins/', [
          {
            id: 'modrinth',
            searchParamsJson: '{"facets":{"project_type":"plugin","categories":["folia"]}}',
          },
          {
            id: 'hangar',
            searchParamsJson: '{"platform":"PAPER"}',
          },
        ]),
      }),
    ],
  },
  {
    id: 'valheim',
    label: 'Valheim',
    description: 'Thunderstore mod support with BepInEx.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    modProfile: buildModProfile('BepInEx/plugins/', [
      {
        id: 'thunderstore',
        searchParamsJson: '{"community":"valheim"}',
      },
    ]),
  },
  {
    id: 'ark',
    label: 'ARK: Survival Evolved',
    description: 'Steam Workshop mod support.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    modProfile: buildModProfile('ShooterGame/Content/Mods/', [
      {
        id: 'steam_workshop',
        searchParamsJson: '{"app_id":"346110"}',
      },
    ]),
  },
  {
    id: 'garrysmod',
    label: "Garry's Mod",
    description: 'Steam Workshop addon support.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    modProfile: buildModProfile('garrysmod/addons/', [
      {
        id: 'steam_workshop',
        searchParamsJson: '{"app_id":"4000"}',
      },
    ]),
  },
  {
    id: 'rust',
    label: 'Rust',
    description: 'Steam Workshop mod support.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    modProfile: buildModProfile('', [
      {
        id: 'steam_workshop',
        searchParamsJson: '{"app_id":"252490"}',
      },
    ]),
  },
  {
    id: 'project_zomboid',
    label: 'Project Zomboid',
    description: 'Steam Workshop mod support.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    modProfile: buildModProfile('', [
      {
        id: 'steam_workshop',
        searchParamsJson: '{"app_id":"108600"}',
      },
    ]),
  },
  {
    id: 'lethal_company',
    label: 'Lethal Company',
    description: 'Thunderstore mod support with BepInEx.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
    modProfile: buildModProfile('BepInEx/plugins/', [
      {
        id: 'thunderstore',
        searchParamsJson: '{"community":"lethal-company"}',
      },
    ]),
  },
  {
    id: 'none',
    label: 'No Variants / Mods',
    description: 'Clear variants and mod support for this game.',
    updateProvider: buildUpdateProvider(UpdateProviderKind.NONE),
  },
]

function plainProvider(provider?: UpdateProviderConfig): Record<string, string> {
  if (!provider) {
    return {}
  }

  return {
    kind: String(provider.kind),
    sourceId: provider.sourceId,
  }
}

function plainModProfile(profile?: ModProfile): object | null {
  if (!profile) {
    return null
  }

  return {
    installPath: profile.installPath,
    sources: profile.sources.map((source) => ({
      id: source.id,
      searchParamsJson: source.searchParamsJson,
    })),
  }
}

function plainVariants(variants: Variant[]): object[] {
  return variants.map((variant) => ({
    id: variant.id,
    name: variant.name,
    updateProvider: plainProvider(variant.updateProvider),
    defaultTarget: variant.defaultTarget,
    modProfile: plainModProfile(variant.modProfile),
  }))
}

function serializePresetConfig(
  config: Pick<GamePreset, 'updateProvider' | 'defaultTarget' | 'modProfile' | 'variants'>,
): string {
  return JSON.stringify({
    updateProvider: plainProvider(config.updateProvider),
    defaultTarget: config.defaultTarget ?? '',
    modProfile: plainModProfile(config.modProfile),
    variants: plainVariants(config.variants ?? []),
  })
}

function serializeGameConfig(
  game: Pick<Game, 'updateProvider' | 'defaultTarget' | 'modProfile' | 'variants'>,
): string {
  return JSON.stringify({
    updateProvider: plainProvider(game.updateProvider),
    defaultTarget: game.defaultTarget,
    modProfile: plainModProfile(game.modProfile),
    variants: plainVariants(game.variants),
  })
}

export function detectPreset(
  game: Pick<Game, 'updateProvider' | 'defaultTarget' | 'modProfile' | 'variants'>,
): string {
  const serializedGame = serializeGameConfig(game)

  for (const preset of gamePresets) {
    if (serializePresetConfig(preset) === serializedGame) {
      return preset.id
    }
  }

  return serializedGame === serializePresetConfig(gamePresets[gamePresets.length - 1])
    ? 'none'
    : 'custom'
}

export function applyPreset(game: Game, presetId: string): void {
  const preset = gamePresets.find((candidate) => candidate.id === presetId)
  if (!preset) {
    return
  }

  game.updateProvider = preset.updateProvider
    ? create(UpdateProviderConfigSchema, preset.updateProvider)
    : undefined
  game.defaultTarget = preset.defaultTarget ?? ''
  game.modProfile = preset.modProfile ? create(ModProfileSchema, preset.modProfile) : undefined
  game.variants = (preset.variants ?? []).map((variant) => create(VariantSchema, variant))
}
