export interface PlaceholderDefinition {
  key: string
  label: string
  description: string
  commandOnly?: boolean
}

export const placeholders: PlaceholderDefinition[] = [
  { key: 'IP', label: 'IP Address', description: "The game server's bound IP address" },
  { key: 'PORT', label: 'Server Port', description: "The game server's port" },
  { key: 'QUERY_PORT', label: 'Query Port', description: 'The query port' },
  { key: 'MAX_PLAYERS', label: 'Max Players', description: 'Maximum player count' },
  { key: 'SERVER_NAME', label: 'Server Name', description: "The game server's display name" },
  { key: 'RCON_PORT', label: 'RCON Port', description: 'The RCON port' },
  { key: 'RCON_PASSWORD', label: 'RCON Password', description: 'The RCON password' },
  {
    key: 'INSTALL_DIR',
    label: 'Install Directory',
    description: "The game server's installation directory",
    commandOnly: true,
  },
  {
    key: 'STEAM_APPID',
    label: 'Steam App ID',
    description: 'The Steam application ID',
    commandOnly: true,
  },
]

/** Managed source key -> placeholder key mapping */
export const managedSourceToPlaceholder: Record<string, string> = {
  ip: 'IP',
  server_port: 'PORT',
  query_port: 'QUERY_PORT',
  max_players: 'MAX_PLAYERS',
  server_name: 'SERVER_NAME',
  rcon_port: 'RCON_PORT',
  rcon_password: 'RCON_PASSWORD',
}

/** Managed source options for config schema field dropdown (excludes command-only placeholders) */
export const managedSourceOptions = placeholders
  .filter((p) => !p.commandOnly)
  .map((p) => {
    const sourceKey = Object.entries(managedSourceToPlaceholder).find(([, v]) => v === p.key)?.[0]
    return { label: p.label, value: sourceKey ?? p.key.toLowerCase() }
  })
