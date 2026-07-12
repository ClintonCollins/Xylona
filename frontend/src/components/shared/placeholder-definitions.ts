export interface PlaceholderDefinition {
  key: string
  label: string
  description: string
  commandOnly?: boolean
}

export const placeholders: PlaceholderDefinition[] = [
  { key: 'IP', label: 'IP Address', description: "The game server's bound IP address" },
  { key: 'PORT', label: 'Server Port', description: "The game server's port" },
  {
    key: 'PORT_PLUS_1',
    label: 'Server Port + 1',
    description: "The port immediately after the game server's port",
  },
  {
    key: 'PORT_PLUS_2',
    label: 'Server Port + 2',
    description: "The second port after the game server's port",
  },
  { key: 'QUERY_PORT', label: 'Query Port', description: 'The query port' },
  {
    key: 'QUERY_PORT_PLUS_1',
    label: 'Query Port + 1',
    description: 'The port immediately after the query port',
  },
  {
    key: 'MAX_MEMORY_MB',
    label: 'Game Server Memory (MB)',
    description: "The game server's configured memory limit in megabytes",
  },
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
  {
    key: 'SERVER_EXECUTABLE',
    label: 'Server Executable',
    description: 'The server software executable filename (for example, paper-1.21.4-100.jar)',
    commandOnly: true,
  },
]

/** Managed source key -> placeholder key mapping */
export const managedSourceToPlaceholder: Record<string, string> = {
  ip: 'IP',
  server_port: 'PORT',
  server_port_plus_1: 'PORT_PLUS_1',
  server_port_plus_2: 'PORT_PLUS_2',
  query_port: 'QUERY_PORT',
  query_port_plus_1: 'QUERY_PORT_PLUS_1',
  max_memory_mb: 'MAX_MEMORY_MB',
  max_players: 'MAX_PLAYERS',
  server_name: 'SERVER_NAME',
  rcon_port: 'RCON_PORT',
  rcon_password: 'RCON_PASSWORD',
}

const frontendManagedSourceToBackendSourceMap: Record<string, string> = {
  ip: 'game_server.ip',
  server_port: 'game_server.port',
  server_port_plus_1: 'game_server.port_plus_1',
  server_port_plus_2: 'game_server.port_plus_2',
  query_port: 'game_server.query_port',
  query_port_plus_1: 'game_server.query_port_plus_1',
  max_memory_mb: 'game_server.max_memory_mb',
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
  'game_server.port_plus_1': 'Server Port + 1',
  'game_server.port_plus_2': 'Server Port + 2',
  'game_server.query_port': 'Query Port',
  'game_server.query_port_plus_1': 'Query Port + 1',
  'game_server.max_memory_mb': 'Game Server Memory (MB)',
  'game_server.max_players': 'Max Players',
  'game_server.server_name': 'Server Name',
  'game_server.rcon_port': 'RCON Port',
  'game_server.rcon_password': 'RCON Password',
  server_executable: 'Server Executable',
  steam_gslt: 'Steam GSLT',
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

const startArgManagedSourceToPlaceholder: Record<string, string> = {
  'game_server.ip': 'IP',
  'game_server.port': 'PORT',
  'game_server.port_plus_1': 'PORT_PLUS_1',
  'game_server.port_plus_2': 'PORT_PLUS_2',
  'game_server.query_port': 'QUERY_PORT',
  'game_server.query_port_plus_1': 'QUERY_PORT_PLUS_1',
  'game_server.max_memory_mb': 'MAX_MEMORY_MB',
  'game_server.max_players': 'MAX_PLAYERS',
  'game_server.server_name': 'SERVER_NAME',
  server_executable: 'SERVER_EXECUTABLE',
}

const placeholderByKey = new Map(placeholders.map((placeholder) => [placeholder.key, placeholder]))

export const startArgManagedSourceOptions = [
  { label: 'Not set', value: '' },
  ...Object.entries(startArgManagedSourceToPlaceholder).map(([sourceKey, placeholderKey]) => ({
    label: placeholderByKey.get(placeholderKey)?.label ?? sourceKey,
    value: sourceKey,
  })),
]
