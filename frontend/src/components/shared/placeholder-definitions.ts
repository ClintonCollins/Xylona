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

const frontendManagedSourceToBackendSourceMap: Record<string, string> = {
  ip: 'game_server.ip',
  server_port: 'game_server.port',
  query_port: 'game_server.query_port',
  max_players: 'game_server.max_players',
  server_name: 'game_server.server_name',
  rcon_port: 'game_server.rcon_port',
  rcon_password: 'game_server.rcon_password',
}

const backendManagedSourceToFrontendSourceMap = Object.fromEntries(
  Object.entries(frontendManagedSourceToBackendSourceMap).map(([frontendSource, backendSource]) => [
    backendSource,
    frontendSource,
  ]),
) as Record<string, string>

/** Maps full backend managed source keys (e.g. "game_server.port") to human-readable labels */
export const managedSourceLabels: Record<string, string> = {
  'game_server.ip': 'IP Address',
  'game_server.port': 'Server Port',
  'game_server.query_port': 'Query Port',
  'game_server.max_players': 'Max Players',
  'game_server.server_name': 'Server Name',
  'game_server.rcon_port': 'RCON Port',
  'game_server.rcon_password': 'RCON Password',
}

/** Returns a human-readable label for a managed source key, or the raw key if unknown */
export function getManagedSourceLabel(source: string): string {
  return managedSourceLabels[source] ?? source
}

export function toBackendManagedSource(source: string): string {
  return frontendManagedSourceToBackendSourceMap[source] ?? source
}

export function toFrontendManagedSource(source: string): string {
  return backendManagedSourceToFrontendSourceMap[source] ?? source
}

/** Managed source options for config schema field dropdown (excludes command-only placeholders) */
export const managedSourceOptions = placeholders
  .filter((p) => !p.commandOnly)
  .map((p) => {
    const sourceKey = Object.entries(managedSourceToPlaceholder).find(([, v]) => v === p.key)?.[0]
    return { label: p.label, value: sourceKey ?? p.key.toLowerCase() }
  })
