import { create } from '@bufbuild/protobuf'
import { ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema } from '@/proto/shared_pb'

import { useGameFormPersistence } from './useGameFormPersistence'

const mocks = vi.hoisted(() => ({
  addGame: vi.fn(),
  editGame: vi.fn(),
  getGame: vi.fn(),
  listGameServers: vi.fn(),
  notify: vi.fn(),
  push: vi.fn(),
  updateGameConfigSchemas: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      addGame: mocks.addGame,
      editGame: mocks.editGame,
      getGame: mocks.getGame,
      listGameServers: mocks.listGameServers,
      updateGameConfigSchemas: mocks.updateGameConfigSchemas,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      back: vi.fn(),
      push: mocks.push,
    }),
  }
})

function createState() {
  const formRef = ref({
    validate: vi.fn().mockResolvedValue(true),
  })
  const game = ref(create(GameSchema, { id: 'minecraft', name: 'Minecraft' }))
  const gameID = ref('minecraft')
  const existingGame = ref(true)
  const copyGame = ref(false)
  const defaultPort = ref<number | null>(25565)
  const defaultQueryPort = ref<number | null>(25566)
  const configSchemas = ref<
    Array<{
      path: string
      format: string
      category: string
      generate_before_start: boolean
    }>
  >([])
  const downstreamImpactServers = ref<Array<{ name: string; patchCount: number }>>([])
  const savedSuccessfully = ref(false)
  const ensureTypedGameConfig = vi.fn()
  const syncSimpleGameConfig = vi.fn()
  const syncStructuredStartArgsFromGame = vi.fn()
  const captureRuntimeBaselineFromCurrentState = vi.fn()
  const syncStructuredStartArgsToGame = vi.fn()
  const syncActivePlatformFromGame = vi.fn()
  const commitFormSnapshot = vi.fn()

  return {
    formRef,
    game,
    gameID,
    existingGame,
    copyGame,
    defaultPort,
    defaultQueryPort,
    configSchemas,
    downstreamImpactServers,
    savedSuccessfully,
    ensureTypedGameConfig,
    syncSimpleGameConfig,
    syncStructuredStartArgsFromGame,
    captureRuntimeBaselineFromCurrentState,
    syncStructuredStartArgsToGame,
    syncActivePlatformFromGame,
    commitFormSnapshot,
  }
}

describe('useGameFormPersistence', () => {
  beforeEach(() => {
    mocks.addGame.mockReset()
    mocks.editGame.mockReset()
    mocks.getGame.mockReset()
    mocks.listGameServers.mockReset()
    mocks.notify.mockReset()
    mocks.push.mockReset()
    mocks.updateGameConfigSchemas.mockReset()
    mocks.listGameServers.mockResolvedValue({ gameServers: [] })
  })

  it('hydrates loaded games, parsed schemas, and downstream impact', async () => {
    const state = createState()
    const persistence = useGameFormPersistence(state)

    mocks.getGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
        defaultPort: 25565n,
        defaultQueryPort: 25566n,
        configSchemas:
          '[{"path":"server.properties","format":"properties","category":"server","generate_before_start":false}]',
      }),
    })
    mocks.listGameServers.mockResolvedValue({
      gameServers: [
        { gameId: 'minecraft', name: 'Alpha', startArgsPatches: '[{"id":"nogui","op":"add"}]' },
        { gameId: 'valheim', name: 'Beta', startArgsPatches: '' },
      ],
    })

    await persistence.loadGameDetails()

    expect(state.ensureTypedGameConfig).toHaveBeenCalledTimes(1)
    expect(state.defaultPort.value).toBe(25565)
    expect(state.defaultQueryPort.value).toBe(25566)
    expect(state.configSchemas.value).toEqual([
      {
        path: 'server.properties',
        format: 'properties',
        category: 'server',
        generate_before_start: false,
      },
    ])
    expect(state.syncStructuredStartArgsFromGame).toHaveBeenCalledTimes(1)
    expect(state.captureRuntimeBaselineFromCurrentState).toHaveBeenCalledTimes(1)
    expect(state.syncActivePlatformFromGame).toHaveBeenCalledTimes(1)
    expect(state.downstreamImpactServers.value).toEqual([{ name: 'Alpha', patchCount: 1 }])
    expect(state.commitFormSnapshot).toHaveBeenCalledTimes(1)
  })

  it('normalizes and submits new games through addGame, then redirects to edit', async () => {
    const state = createState()
    state.existingGame.value = false
    state.game.value.steamAppid = ' 480 '
    state.configSchemas.value = [
      {
        path: 'server.properties',
        format: 'properties',
        category: 'server',
        generate_before_start: false,
      },
    ]
    const persistence = useGameFormPersistence(state)

    mocks.addGame.mockResolvedValue({
      game: create(GameSchema, {
        id: 'minecraft',
        name: 'Minecraft',
      }),
    })

    await persistence.submit()

    expect(state.formRef.value?.validate).toHaveBeenCalledTimes(1)
    expect(state.syncSimpleGameConfig).toHaveBeenCalledTimes(1)
    expect(state.syncStructuredStartArgsToGame).toHaveBeenCalledTimes(1)
    expect(mocks.addGame).toHaveBeenCalledTimes(1)
    expect(state.game.value.configSchemas).toContain('"path":"server.properties"')
    expect(state.game.value.steamAppid).toBe('480')
    expect(state.game.value.defaultPort).toBe(25565n)
    expect(state.game.value.defaultQueryPort).toBe(25566n)
    expect(state.savedSuccessfully.value).toBe(true)
    expect(state.captureRuntimeBaselineFromCurrentState).toHaveBeenCalledTimes(1)
    expect(state.commitFormSnapshot).toHaveBeenCalledTimes(1)
    expect(mocks.push).toHaveBeenCalledWith({ path: '/games/minecraft/edit' })
  })

  it('saves schemas before navigating to the config schema editor', async () => {
    const state = createState()
    state.configSchemas.value = [
      {
        path: 'server.properties',
        format: 'properties',
        category: 'server',
        generate_before_start: false,
      },
    ]
    const persistence = useGameFormPersistence(state)

    mocks.updateGameConfigSchemas.mockResolvedValue({})

    await persistence.navigateToSchemaEditor(2)

    expect(mocks.updateGameConfigSchemas).toHaveBeenCalledTimes(1)
    expect(mocks.updateGameConfigSchemas.mock.calls[0]?.[0]).toMatchObject({
      gameId: 'minecraft',
    })
    expect(mocks.updateGameConfigSchemas.mock.calls[0]?.[0]?.configSchemasJson).toContain(
      '"path":"server.properties"',
    )
    expect(state.commitFormSnapshot).toHaveBeenCalledTimes(1)
    expect(mocks.push).toHaveBeenCalledWith({ path: '/games/minecraft/config-schema/2' })
  })
})
