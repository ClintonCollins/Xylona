import { create } from '@bufbuild/protobuf'
import { describe, expect, it } from 'vitest'

import {
  CommandProcessor,
  CommandType,
  GameSchema,
  ModProfileSchema,
  ModSourceSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
} from '@/proto/shared_pb'
import {
  applySimpleGameConfig,
  getCommandProcessorOptions,
  getCommandTypeOptions,
  getModSourceConfig,
  getModSourceOptions,
  isCommandTypeCommand,
  isManagedGameConfig,
  isManagedModConfig,
  readModSourcePrimaryValue,
  writeModSourcePrimaryValue,
} from './game-form-provider-fields'

describe('getCommandTypeOptions', () => {
  it('returns the supported public install and update types', () => {
    expect(getCommandTypeOptions()).toEqual([
      { label: 'None', value: CommandType.NONE },
      { label: 'Command', value: CommandType.COMMAND },
      { label: 'SteamCMD', value: CommandType.STEAMCMD },
      { label: 'PaperMC', value: CommandType.PAPERMC },
      { label: 'Mojang', value: CommandType.MOJANG },
    ])
  })
})

describe('getCommandProcessorOptions', () => {
  it('returns shell-only processor options for linux', () => {
    expect(getCommandProcessorOptions('linux')).toEqual([
      { label: 'Direct', value: CommandProcessor.DIRECT },
      { label: 'Bash', value: CommandProcessor.BASH },
    ])
  })

  it('returns shell-only processor options for windows', () => {
    expect(getCommandProcessorOptions('windows')).toEqual([
      { label: 'Direct', value: CommandProcessor.DIRECT },
      { label: 'CMD', value: CommandProcessor.CMD },
      { label: 'PowerShell', value: CommandProcessor.POWERSHELL },
    ])
  })
})

describe('isCommandTypeCommand', () => {
  it('only returns true for command-backed fields', () => {
    expect(isCommandTypeCommand(CommandType.COMMAND)).toBe(true)
    expect(isCommandTypeCommand(CommandType.STEAMCMD)).toBe(false)
  })
})

describe('applySimpleGameConfig', () => {
  it('derives hidden SteamCMD config and generated commands from the simple editor state', () => {
    const game = create(GameSchema, {
      linuxSupport: true,
      steamAppid: '294420',
      linuxInstallType: CommandType.STEAMCMD,
      linuxUpdateType: CommandType.STEAMCMD,
      updateProvider: create(UpdateProviderConfigSchema, {}),
    })

    applySimpleGameConfig(game)

    expect(game.usesSteamcmd).toBe(true)
    expect(game.updateProvider?.kind).toBe(UpdateProviderKind.STEAMCMD)
    expect(game.updateProvider?.sourceId).toBe('294420')
    expect(game.defaultTarget).toBe('')
    expect(game.linuxInstallCommand).toContain('app_update 294420')
    expect(game.linuxUpdateCommand).toContain('app_update 294420')
    expect(game.linuxInstallCommandProcessor).toBe(CommandProcessor.DIRECT)
    expect(game.linuxUpdateCommandProcessor).toBe(CommandProcessor.DIRECT)
  })

  it('preserves command text while keeping command processors shell-only', () => {
    const game = create(GameSchema, {
      windowsSupport: true,
      windowsInstallType: CommandType.COMMAND,
      windowsUpdateType: CommandType.COMMAND,
      windowsInstallCommand: 'installer.exe /S',
      windowsUpdateCommand: 'updater.exe /silent',
      windowsInstallCommandProcessor: CommandProcessor.POWERSHELL,
      windowsUpdateCommandProcessor: CommandProcessor.CMD,
      updateProvider: create(UpdateProviderConfigSchema, {}),
    })

    applySimpleGameConfig(game)

    expect(game.updateProvider?.kind).toBe(UpdateProviderKind.COMMAND)
    expect(game.windowsInstallCommand).toBe('installer.exe /S')
    expect(game.windowsUpdateCommand).toBe('updater.exe /silent')
    expect(game.windowsInstallCommandProcessor).toBe(CommandProcessor.POWERSHELL)
    expect(game.windowsUpdateCommandProcessor).toBe(CommandProcessor.CMD)
  })
})

describe('isManagedGameConfig', () => {
  it('treats variant-backed games as managed config', () => {
    const game = create(GameSchema, {
      variants: [{ id: 'paper', name: 'Paper' }],
      updateProvider: create(UpdateProviderConfigSchema, {}),
    })

    expect(isManagedGameConfig(game)).toBe(true)
  })

  it('keeps simple SteamCMD and single-source mod support editable', () => {
    const game = create(GameSchema, {
      updateProvider: create(UpdateProviderConfigSchema, {
        kind: UpdateProviderKind.STEAMCMD,
        sourceId: '294420',
      }),
      modProfile: create(ModProfileSchema, {
        installPath: 'BepInEx/plugins/',
        sources: [
          create(ModSourceSchema, {
            id: 'thunderstore',
            searchParamsJson: '{"community":"valheim"}',
          }),
        ],
      }),
    })

    expect(isManagedGameConfig(game)).toBe(false)
  })

  it('treats advanced modrinth filters as managed config', () => {
    const game = create(GameSchema, {
      updateProvider: create(UpdateProviderConfigSchema, {}),
      modProfile: create(ModProfileSchema, {
        sources: [
          create(ModSourceSchema, {
            id: 'modrinth',
            searchParamsJson: '{"facets":{"project_type":"plugin"}}',
          }),
        ],
      }),
    })

    expect(isManagedGameConfig(game)).toBe(true)
  })
})

describe('isManagedModConfig', () => {
  it('keeps simple single-source mod support editable even for variant-backed games', () => {
    const game = create(GameSchema, {
      variants: [{ id: 'paper', name: 'Paper' }],
      updateProvider: create(UpdateProviderConfigSchema, {
        kind: UpdateProviderKind.MOJANG,
        sourceId: 'vanilla',
      }),
      modProfile: create(ModProfileSchema, {
        installPath: 'plugins/',
        sources: [
          create(ModSourceSchema, {
            id: 'hangar',
            searchParamsJson: '{"platform":"PAPER"}',
          }),
        ],
      }),
    })

    expect(isManagedModConfig(game)).toBe(false)
  })

  it('treats multi-source mod profiles as managed', () => {
    const game = create(GameSchema, {
      modProfile: create(ModProfileSchema, {
        installPath: 'mods/',
        sources: [
          create(ModSourceSchema, {
            id: 'thunderstore',
            searchParamsJson: '{"community":"valheim"}',
          }),
          create(ModSourceSchema, {
            id: 'steam_workshop',
            searchParamsJson: '{"app_id":"346110"}',
          }),
        ],
      }),
    })

    expect(isManagedModConfig(game)).toBe(true)
  })
})

describe('getModSourceOptions', () => {
  it('returns labeled mod source choices', () => {
    expect(getModSourceOptions()).toEqual([
      { label: 'Modrinth', value: 'modrinth' },
      { label: 'Hangar', value: 'hangar' },
      { label: 'Thunderstore', value: 'thunderstore' },
      { label: 'Steam Workshop', value: 'steam_workshop' },
    ])
  })
})

describe('getModSourceConfig', () => {
  it('returns a community field for thunderstore', () => {
    expect(getModSourceConfig('thunderstore')).toEqual(
      expect.objectContaining({
        mode: 'community',
        primaryLabel: 'Community',
      }),
    )
  })

  it('returns a workshop app id field for steam workshop', () => {
    expect(getModSourceConfig('steam_workshop')).toEqual(
      expect.objectContaining({
        mode: 'app_id',
        primaryLabel: 'Workshop App ID',
      }),
    )
  })

  it('keeps modrinth on advanced filters json', () => {
    expect(getModSourceConfig('modrinth')).toEqual(
      expect.objectContaining({
        mode: 'json',
        primaryLabel: 'Filters JSON (Advanced)',
      }),
    )
  })
})

describe('readModSourcePrimaryValue', () => {
  it('extracts a thunderstore community from search params', () => {
    expect(readModSourcePrimaryValue('thunderstore', '{"community":"valheim"}')).toBe('valheim')
  })

  it('extracts a steam workshop app id from search params', () => {
    expect(readModSourcePrimaryValue('steam_workshop', '{"app_id":"346110"}')).toBe('346110')
  })

  it('returns the raw json for advanced providers', () => {
    const raw = '{"facets":{"project_type":"plugin"}}'
    expect(readModSourcePrimaryValue('modrinth', raw)).toBe(raw)
  })

  it('falls back to blank for malformed structured json', () => {
    expect(readModSourcePrimaryValue('thunderstore', '{')).toBe('')
  })
})

describe('writeModSourcePrimaryValue', () => {
  it('writes a thunderstore community into json', () => {
    expect(writeModSourcePrimaryValue('thunderstore', '', 'valheim')).toBe(
      '{"community":"valheim"}',
    )
  })

  it('writes a steam workshop app id into json', () => {
    expect(writeModSourcePrimaryValue('steam_workshop', '', '346110')).toBe('{"app_id":"346110"}')
  })

  it('preserves json mode as raw text', () => {
    const raw = '{"facets":{"project_type":"plugin"}}'
    expect(writeModSourcePrimaryValue('modrinth', '', raw)).toBe(raw)
  })

  it('clears structured json when the primary field is blank', () => {
    expect(writeModSourcePrimaryValue('thunderstore', '{"community":"valheim"}', '')).toBe('')
  })
})
