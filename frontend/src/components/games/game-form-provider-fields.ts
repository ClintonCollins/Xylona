import {
  CommandProcessor,
  CommandType,
  type Game,
  type ModProfile,
  type ModSource,
  UpdateProviderKind,
} from '@/proto/shared_pb'

type Platform = 'linux' | 'windows'
type ModSourceMode = 'community' | 'app_id' | 'platform' | 'json'

interface ModSourceConfig {
  mode: ModSourceMode
  primaryLabel: string
  primaryHint: string
  placeholder: string
}

const commandTypeOptions = [
  { label: 'None', value: CommandType.NONE },
  { label: 'Command', value: CommandType.COMMAND },
  { label: 'SteamCMD', value: CommandType.STEAMCMD },
  { label: 'PaperMC', value: CommandType.PAPERMC },
  { label: 'Mojang', value: CommandType.MOJANG },
]

const linuxCommandProcessorOptions = [
  { label: 'Direct', value: CommandProcessor.DIRECT },
  { label: 'Bash', value: CommandProcessor.BASH },
]

const windowsCommandProcessorOptions = [
  { label: 'Direct', value: CommandProcessor.DIRECT },
  { label: 'CMD', value: CommandProcessor.CMD },
  { label: 'PowerShell', value: CommandProcessor.POWERSHELL },
]

const modSourceOptions = [
  { label: 'Modrinth', value: 'modrinth' },
  { label: 'Hangar', value: 'hangar' },
  { label: 'Thunderstore', value: 'thunderstore' },
  { label: 'Steam Workshop', value: 'steam_workshop' },
]

export function getCommandTypeOptions(): { label: string; value: CommandType }[] {
  return commandTypeOptions
}

export function getCommandProcessorOptions(
  platform: Platform,
): { label: string; value: CommandProcessor }[] {
  if (platform === 'windows') {
    return windowsCommandProcessorOptions
  }
  return linuxCommandProcessorOptions
}

export function isCommandTypeCommand(commandType: CommandType): boolean {
  return commandType === CommandType.COMMAND
}

export function applySimpleGameConfig(game: Game): void {
  const updateProvider = game.updateProvider
  if (!updateProvider) {
    return
  }

  game.usesSteamcmd = hasSteamCommandType(game)

  const updateProviderKind = commandTypeToUpdateProviderKind(primaryUpdateType(game))
  updateProvider.kind = updateProviderKind
  updateProvider.sourceId = updateProviderSourceID(updateProviderKind, game.steamAppid)

  game.linuxInstallCommand = commandValueForType(
    game.linuxInstallType,
    game.linuxInstallCommand,
    game.steamAppid,
  )
  game.linuxUpdateCommand = commandValueForType(
    game.linuxUpdateType,
    game.linuxUpdateCommand,
    game.steamAppid,
  )
  game.windowsInstallCommand = commandValueForType(
    game.windowsInstallType,
    game.windowsInstallCommand,
    game.steamAppid,
  )
  game.windowsUpdateCommand = commandValueForType(
    game.windowsUpdateType,
    game.windowsUpdateCommand,
    game.steamAppid,
  )

  game.linuxInstallCommandProcessor = commandProcessorForType(
    game.linuxInstallType,
    game.linuxInstallCommandProcessor,
    'linux',
  )
  game.linuxUpdateCommandProcessor = commandProcessorForType(
    game.linuxUpdateType,
    game.linuxUpdateCommandProcessor,
    'linux',
  )
  game.windowsInstallCommandProcessor = commandProcessorForType(
    game.windowsInstallType,
    game.windowsInstallCommandProcessor,
    'windows',
  )
  game.windowsUpdateCommandProcessor = commandProcessorForType(
    game.windowsUpdateType,
    game.windowsUpdateCommandProcessor,
    'windows',
  )
}

export function isManagedGameConfig(
  game: Pick<Game, 'variants' | 'updateProvider' | 'defaultTarget' | 'modProfile'>,
): boolean {
  if ((game.variants?.length ?? 0) > 0) {
    return true
  }
  if (game.defaultTarget.trim() !== '') {
    return true
  }

  const providerKind = game.updateProvider?.kind ?? UpdateProviderKind.NONE
  const providerSourceID = game.updateProvider?.sourceId?.trim() ?? ''
  if (providerKind === UpdateProviderKind.PAPERMC || providerKind === UpdateProviderKind.MOJANG) {
    return true
  }
  if (providerKind === UpdateProviderKind.COMMAND && providerSourceID !== '') {
    return true
  }
  if (providerKind === UpdateProviderKind.STEAMCMD && providerSourceID !== '') {
    return false
  }
  if (providerSourceID !== '') {
    return true
  }

  return !isSimpleModProfile(game.modProfile)
}

export function isManagedModConfig(game: Pick<Game, 'modProfile'>): boolean {
  return !isSimpleModProfile(game.modProfile)
}

export function getModSourceOptions(): { label: string; value: string }[] {
  return modSourceOptions
}

export function getModSourceConfig(providerID: string): ModSourceConfig {
  switch (providerID) {
    case 'thunderstore':
      return {
        mode: 'community',
        primaryLabel: 'Community',
        primaryHint: 'Examples: valheim, lethal-company',
        placeholder: 'valheim',
      }
    case 'steam_workshop':
      return {
        mode: 'app_id',
        primaryLabel: 'Workshop App ID',
        primaryHint: 'Examples: 346110, 108600',
        placeholder: '346110',
      }
    case 'hangar':
      return {
        mode: 'platform',
        primaryLabel: 'Platform',
        primaryHint: 'Examples: PAPER, VELOCITY, WATERFALL',
        placeholder: 'PAPER',
      }
    case 'modrinth':
    default:
      return {
        mode: 'json',
        primaryLabel: 'Filters JSON (Advanced)',
        primaryHint: 'Examples: {"facets":{"project_type":"plugin","categories":["paper"]}}',
        placeholder: '{"facets":{"project_type":"plugin"}}',
      }
  }
}

export function readModSourcePrimaryValue(providerID: string, searchParamsJSON: string): string {
  const config = getModSourceConfig(providerID)
  if (config.mode === 'json') {
    return searchParamsJSON
  }

  const parsed = parseSearchParams(searchParamsJSON)
  if (!parsed) {
    return ''
  }

  switch (config.mode) {
    case 'community':
      return readStringField(parsed, 'community')
    case 'app_id':
      return readStringField(parsed, 'app_id')
    case 'platform':
      return readStringField(parsed, 'platform')
    default:
      return ''
  }
}

export function writeModSourcePrimaryValue(
  providerID: string,
  currentJSON: string,
  value: string,
): string {
  const config = getModSourceConfig(providerID)
  const trimmed = value.trim()

  if (config.mode === 'json') {
    return value
  }
  if (trimmed === '') {
    return ''
  }

  const parsed = parseSearchParams(currentJSON) ?? {}
  const normalized = { ...parsed }

  switch (config.mode) {
    case 'community':
      delete normalized['app_id']
      delete normalized['platform']
      normalized['community'] = trimmed
      break
    case 'app_id':
      delete normalized['community']
      delete normalized['platform']
      normalized['app_id'] = trimmed
      break
    case 'platform':
      delete normalized['community']
      delete normalized['app_id']
      normalized['platform'] = trimmed
      break
    default:
      return value
  }

  return JSON.stringify(normalized)
}

function primaryUpdateType(game: Game): CommandType {
  if (game.linuxSupport && game.linuxUpdateType !== CommandType.NONE) {
    return game.linuxUpdateType
  }
  if (game.windowsSupport && game.windowsUpdateType !== CommandType.NONE) {
    return game.windowsUpdateType
  }
  if (game.linuxSupport) {
    return game.linuxInstallType
  }
  if (game.windowsSupport) {
    return game.windowsInstallType
  }
  return CommandType.NONE
}

function commandTypeToUpdateProviderKind(commandType: CommandType): UpdateProviderKind {
  switch (commandType) {
    case CommandType.COMMAND:
      return UpdateProviderKind.COMMAND
    case CommandType.STEAMCMD:
      return UpdateProviderKind.STEAMCMD
    case CommandType.PAPERMC:
      return UpdateProviderKind.PAPERMC
    case CommandType.MOJANG:
      return UpdateProviderKind.MOJANG
    default:
      return UpdateProviderKind.NONE
  }
}

function updateProviderSourceID(kind: UpdateProviderKind, steamAppID: string): string {
  switch (kind) {
    case UpdateProviderKind.STEAMCMD:
      return steamAppID.trim()
    case UpdateProviderKind.PAPERMC:
      return 'paper'
    case UpdateProviderKind.MOJANG:
      return 'vanilla'
    default:
      return ''
  }
}

function commandValueForType(
  commandType: CommandType,
  currentCommand: string,
  steamAppID: string,
): string {
  switch (commandType) {
    case CommandType.NONE:
      return ''
    case CommandType.STEAMCMD:
      return buildSteamCMDCommand(steamAppID)
    default:
      return currentCommand
  }
}

function commandProcessorForType(
  commandType: CommandType,
  current: CommandProcessor,
  platform: Platform,
): CommandProcessor {
  if (commandType === CommandType.COMMAND) {
    if (platform === 'windows') {
      if (current === CommandProcessor.CMD || current === CommandProcessor.POWERSHELL) {
        return current
      }
      return CommandProcessor.DIRECT
    }

    if (current === CommandProcessor.BASH) {
      return current
    }
    return CommandProcessor.DIRECT
  }

  if (commandType === CommandType.PAPERMC || commandType === CommandType.MOJANG) {
    return CommandProcessor.XYLONA_INTERNAL
  }

  return CommandProcessor.DIRECT
}

function hasSteamCommandType(game: Game): boolean {
  return (
    game.linuxInstallType === CommandType.STEAMCMD ||
    game.linuxUpdateType === CommandType.STEAMCMD ||
    game.windowsInstallType === CommandType.STEAMCMD ||
    game.windowsUpdateType === CommandType.STEAMCMD
  )
}

function isSimpleModProfile(profile: ModProfile | undefined): boolean {
  if (!profile) {
    return true
  }
  if (profile.sources.length > 1) {
    return false
  }

  return profile.sources.every((source) => isSimpleModSource(source))
}

function isSimpleModSource(source: ModSource): boolean {
  switch (source.id) {
    case 'hangar':
    case 'steam_workshop':
    case 'thunderstore':
      return true
    case 'modrinth':
      return source.searchParamsJson.trim() === ''
    default:
      return false
  }
}

function buildSteamCMDCommand(steamAppID: string): string {
  const normalized = steamAppID.trim()
  if (normalized === '') {
    return ''
  }
  return `steamcmd +force_install_dir %GAMESERVER_DIRECTORY% +login anonymous +app_update ${normalized} validate +quit`
}

function parseSearchParams(searchParamsJSON: string): Record<string, unknown> | null {
  if (searchParamsJSON.trim() === '') {
    return {}
  }

  try {
    const parsed: unknown = JSON.parse(searchParamsJSON)
    if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
      return parsed as Record<string, unknown>
    }
  } catch {
    return null
  }

  return null
}

function readStringField(record: Record<string, unknown>, field: string): string {
  const value = record[field]
  if (typeof value === 'string') {
    return value
  }
  if (typeof value === 'number') {
    return String(value)
  }
  return ''
}
