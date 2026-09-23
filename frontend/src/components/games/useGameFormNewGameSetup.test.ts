import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  CommandType,
  GameSchema,
  UpdateProviderConfigSchema,
  UpdateProviderKind,
} from '@/proto/shared_pb'

import { applySimpleGameConfig } from './game-form-provider-fields'
import { useGameFormNewGameSetup } from './useGameFormNewGameSetup'

function createState() {
  const game = ref(create(GameSchema, {}))
  const downstreamImpactServers = ref<Array<{ name: string; patchCount: number }>>([
    { name: 'Existing Server', patchCount: 3 },
  ])

  return {
    game,
    downstreamImpactServers,
    ensureTypedGameConfig: vi.fn(),
    syncSimpleGameConfig: vi.fn(),
    syncStructuredStartArgsFromGame: vi.fn(),
    captureRuntimeBaselineFromCurrentState: vi.fn(),
    syncActivePlatformFromGame: vi.fn(),
    commitSnapshot: vi.fn(),
  }
}

describe('useGameFormNewGameSetup', () => {
  beforeEach(() => {
    window.history.replaceState({}, '', '/')
  })

  it('hydrates wizard-prefilled game fields and initializes a clean new form', async () => {
    window.history.replaceState(
      {
        wizardState: {
          name: 'Minecraft',
          slug: 'minecraft',
          steamAppId: '480',
          usesSteamcmd: true,
          linuxSupport: true,
          windowsSupport: false,
          linuxBaseCommand: 'java',
          windowsBaseCommand: 'javaw',
          linuxStartArgsTemplate: '[{"id":"jar"}]',
          windowsStartArgsTemplate: '[{"id":"nogui"}]',
        },
      },
      '',
      '/',
    )

    const state = createState()
    const setup = useGameFormNewGameSetup(state)

    await setup.initializeNewGameForm()

    expect(state.game.value.name).toBe('Minecraft')
    expect(state.game.value.id).toBe('minecraft')
    expect(state.game.value.steamAppid).toBe('480')
    expect(state.game.value.usesSteamcmd).toBe(true)
    expect(state.game.value.linuxSupport).toBe(true)
    expect(state.game.value.windowsSupport).toBe(false)
    expect(state.game.value.linuxInstallType).toBe(CommandType.STEAMCMD)
    expect(state.game.value.linuxUpdateType).toBe(CommandType.STEAMCMD)
    expect(state.game.value.windowsInstallType).toBe(CommandType.NONE)
    expect(state.game.value.windowsUpdateType).toBe(CommandType.NONE)
    expect(state.game.value.linuxBaseCommand).toBe('java')
    expect(state.game.value.windowsBaseCommand).toBe('javaw')
    expect(state.game.value.linuxStartArgsTemplate).toContain('"id":"jar"')
    expect(state.game.value.windowsStartArgsTemplate).toContain('"id":"nogui"')
    expect(state.ensureTypedGameConfig).toHaveBeenCalledTimes(2)
    expect(state.syncSimpleGameConfig).toHaveBeenCalledTimes(1)
    expect(state.syncStructuredStartArgsFromGame).toHaveBeenCalledTimes(1)
    expect(state.captureRuntimeBaselineFromCurrentState).toHaveBeenCalledTimes(1)
    expect(state.syncActivePlatformFromGame).toHaveBeenCalledTimes(1)
    expect(state.downstreamImpactServers.value).toEqual([])
    expect(state.commitSnapshot).toHaveBeenCalledTimes(1)
  })

  it('keeps a SteamCMD wizard game installing and updating through SteamCMD', async () => {
    window.history.replaceState(
      {
        wizardState: {
          name: 'Valheim',
          slug: 'valheim',
          steamAppId: '896660',
          usesSteamcmd: true,
          linuxSupport: true,
          windowsSupport: true,
        },
      },
      '',
      '/',
    )

    const state = createState()
    state.game.value.updateProvider = create(UpdateProviderConfigSchema, {})
    state.syncSimpleGameConfig.mockImplementation(() => applySimpleGameConfig(state.game.value))
    const setup = useGameFormNewGameSetup(state)

    await setup.initializeNewGameForm()

    const game = state.game.value
    expect(game.usesSteamcmd).toBe(true)
    expect([
      game.linuxInstallType,
      game.linuxUpdateType,
      game.windowsInstallType,
      game.windowsUpdateType,
    ]).toEqual(Array(4).fill(CommandType.STEAMCMD))
    expect(game.linuxInstallCommand).toContain('+app_update 896660')
    expect(game.windowsUpdateCommand).toContain('+app_update 896660')
    expect(game.updateProvider?.kind).toBe(UpdateProviderKind.STEAMCMD)
    expect(game.updateProvider?.sourceId).toBe('896660')
  })

  it('initializes an empty new game without wizard prefill', async () => {
    const state = createState()
    const setup = useGameFormNewGameSetup(state)

    await setup.initializeNewGameForm()

    expect(state.game.value.name).toBe('')
    expect(state.ensureTypedGameConfig).toHaveBeenCalledTimes(1)
    expect(state.syncSimpleGameConfig).not.toHaveBeenCalled()
    expect(state.syncStructuredStartArgsFromGame).toHaveBeenCalledTimes(1)
    expect(state.captureRuntimeBaselineFromCurrentState).toHaveBeenCalledTimes(1)
    expect(state.syncActivePlatformFromGame).not.toHaveBeenCalled()
    expect(state.downstreamImpactServers.value).toEqual([])
    expect(state.commitSnapshot).toHaveBeenCalledTimes(1)
  })
})
