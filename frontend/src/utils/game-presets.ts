export interface GamePreset {
  id: string
  label: string
  description: string
  serverSoftware: object[]
}

export const gamePresets: GamePreset[] = [
  {
    id: 'minecraft',
    label: 'Minecraft: Java Edition',
    description:
      'Vanilla, Paper, Purpur, Fabric server software with Modrinth and Hangar mod sources',
    serverSoftware: [
      {
        id: 'vanilla',
        name: 'Vanilla',
        jar_source: null,
        mod_config: null,
      },
      {
        id: 'paper',
        name: 'Paper',
        jar_source: 'papermc',
        mod_config: {
          mod_types: [{ type: 'plugin', label: 'Plugins', install_path: 'plugins/' }],
          sources: [
            {
              id: 'modrinth',
              search_params: {
                facets: { project_type: 'plugin', categories: ['paper', 'spigot', 'bukkit'] },
              },
            },
            { id: 'hangar', search_params: { platform: 'PAPER' } },
          ],
        },
      },
      {
        id: 'purpur',
        name: 'Purpur',
        jar_source: 'papermc',
        mod_config: {
          mod_types: [{ type: 'plugin', label: 'Plugins', install_path: 'plugins/' }],
          sources: [
            {
              id: 'modrinth',
              search_params: {
                facets: {
                  project_type: 'plugin',
                  categories: ['purpur', 'paper', 'spigot', 'bukkit'],
                },
              },
            },
            { id: 'hangar', search_params: { platform: 'PAPER' } },
          ],
        },
      },
      {
        id: 'fabric',
        name: 'Fabric',
        jar_source: null,
        mod_config: {
          mod_types: [{ type: 'mod', label: 'Mods', install_path: 'mods/' }],
          sources: [
            {
              id: 'modrinth',
              search_params: { facets: { project_type: 'mod', categories: ['fabric'] } },
            },
          ],
        },
      },
      {
        id: 'folia',
        name: 'Folia',
        jar_source: 'papermc',
        mod_config: {
          mod_types: [{ type: 'plugin', label: 'Plugins', install_path: 'plugins/' }],
          sources: [
            {
              id: 'modrinth',
              search_params: { facets: { project_type: 'plugin', categories: ['folia'] } },
            },
            { id: 'hangar', search_params: { platform: 'PAPER' } },
          ],
        },
      },
    ],
  },
  {
    id: 'valheim',
    label: 'Valheim',
    description: 'Thunderstore mod support with BepInEx',
    serverSoftware: [
      {
        id: 'default',
        name: 'Valheim Dedicated Server',
        jar_source: null,
        mod_config: {
          mod_types: [{ type: 'bepinex_mod', label: 'Mods', install_path: 'BepInEx/plugins/' }],
          sources: [{ id: 'thunderstore', search_params: { community: 'valheim' } }],
        },
      },
    ],
  },
  {
    id: 'ark',
    label: 'ARK: Survival Evolved',
    description: 'Steam Workshop mod support',
    serverSoftware: [
      {
        id: 'default',
        name: 'ARK Dedicated Server',
        jar_source: null,
        mod_config: {
          mod_types: [
            {
              type: 'workshop_item',
              label: 'Mods',
              install_path: 'ShooterGame/Content/Mods/',
            },
          ],
          sources: [{ id: 'steam_workshop', search_params: { app_id: '346110' } }],
        },
      },
    ],
  },
  {
    id: 'garrysmod',
    label: "Garry's Mod",
    description: 'Steam Workshop addon support',
    serverSoftware: [
      {
        id: 'default',
        name: "Garry's Mod Server",
        jar_source: null,
        mod_config: {
          mod_types: [
            { type: 'workshop_item', label: 'Addons', install_path: 'garrysmod/addons/' },
          ],
          sources: [{ id: 'steam_workshop', search_params: { app_id: '4000' } }],
        },
      },
    ],
  },
  {
    id: 'rust',
    label: 'Rust',
    description: 'Steam Workshop mod support',
    serverSoftware: [
      {
        id: 'default',
        name: 'Rust Dedicated Server',
        jar_source: null,
        mod_config: {
          mod_types: [{ type: 'workshop_item', label: 'Mods', install_path: '' }],
          sources: [{ id: 'steam_workshop', search_params: { app_id: '252490' } }],
        },
      },
    ],
  },
  {
    id: 'project_zomboid',
    label: 'Project Zomboid',
    description: 'Steam Workshop mod support',
    serverSoftware: [
      {
        id: 'default',
        name: 'Project Zomboid Server',
        jar_source: null,
        mod_config: {
          mod_types: [{ type: 'workshop_item', label: 'Mods', install_path: '' }],
          sources: [{ id: 'steam_workshop', search_params: { app_id: '108600' } }],
        },
      },
    ],
  },
  {
    id: 'lethal_company',
    label: 'Lethal Company',
    description: 'Thunderstore mod support with BepInEx',
    serverSoftware: [
      {
        id: 'default',
        name: 'Lethal Company Server',
        jar_source: null,
        mod_config: {
          mod_types: [{ type: 'bepinex_mod', label: 'Mods', install_path: 'BepInEx/plugins/' }],
          sources: [{ id: 'thunderstore', search_params: { community: 'lethal-company' } }],
        },
      },
    ],
  },
  {
    id: 'none',
    label: 'No Mod Support',
    description: 'No server software variants or mod management',
    serverSoftware: [],
  },
]

/**
 * Attempts to match a JSON string against known presets.
 * Returns the matching preset ID, or 'custom' if no match is found.
 */
export function detectPreset(jsonString: string): string {
  const trimmed = jsonString.trim()
  if (!trimmed) return 'none'

  let parsed: unknown
  try {
    parsed = JSON.parse(trimmed)
  } catch {
    return 'custom'
  }

  if (!Array.isArray(parsed)) return 'custom'
  if (parsed.length === 0) return 'none'

  // Normalize and compare against each preset
  const normalizedInput = JSON.stringify(parsed)
  for (const preset of gamePresets) {
    if (preset.id === 'none') continue
    const normalizedPreset = JSON.stringify(preset.serverSoftware)
    if (normalizedInput === normalizedPreset) {
      return preset.id
    }
  }

  return 'custom'
}

/**
 * Formats a preset's server software array as pretty-printed JSON.
 */
export function presetToJson(preset: GamePreset): string {
  if (preset.serverSoftware.length === 0) return ''
  return JSON.stringify(preset.serverSoftware, null, 2)
}
