import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'

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
          installCommand: './install.sh',
          updateCommand: './update.sh',
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
    expect(state.game.value.linuxInstallCommand).toBe('./install.sh')
    expect(state.game.value.windowsInstallCommand).toBe('')
    expect(state.game.value.linuxUpdateCommand).toBe('./update.sh')
    expect(state.game.value.windowsUpdateCommand).toBe('')
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
