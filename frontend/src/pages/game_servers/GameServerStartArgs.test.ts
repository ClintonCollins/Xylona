import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema, GameServerSchema, NodeSchema, Status } from '@/proto/shared_pb'
import {
  GetGameServerResponseSchema,
  ListNodesResponseSchema,
  UpdateGameServerStartArgsResponseSchema,
} from '@/proto/xylona_pb'
import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'
import GameServerStartArgs from './GameServerStartArgs.vue'

const mocks = vi.hoisted(() => {
  type EventHandler = (...args: unknown[]) => void
  const listeners = new Map<string, Set<EventHandler>>()
  const eventBus = {
    on(event: string, handler: EventHandler) {
      if (!listeners.has(event)) {
        listeners.set(event, new Set())
      }
      listeners.get(event)?.add(handler)
      return eventBus
    },
    off(event: string, handler: EventHandler) {
      listeners.get(event)?.delete(handler)
      return eventBus
    },
    emit(event: string, ...args: unknown[]) {
      for (const handler of listeners.get(event) ?? []) {
        handler(...args)
      }
      return eventBus
    },
    reset() {
      listeners.clear()
    },
  }

  return {
    eventBus,
    getGameServer: vi.fn(),
    listNodes: vi.fn(),
    notify: vi.fn(),
    startGameServer: vi.fn(),
    stopGameServer: vi.fn(),
    updateGameServerStartArgs: vi.fn(),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ notify: mocks.notify }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
  useRouter: () => ({ replace: vi.fn() }),
}))

vi.mock('@/stores/xylona', () => ({
  useUserAuthStore: () => ({
    checkUserAuthenticated: vi.fn().mockResolvedValue({ user: { superUser: false } }),
    user: { superUser: false },
  }),
}))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    listNodes: mocks.listNodes,
    startGameServer: mocks.startGameServer,
    stopGameServer: mocks.stopGameServer,
    updateGameServerStartArgs: mocks.updateGameServerStartArgs,
  }),
  XylonaEventBus: mocks.eventBus,
}))

function buildGameServer(status: Status) {
  return create(GameServerSchema, {
    id: 'server-1',
    nodeId: 'node-1',
    status,
    effectivePermissions: ['game_server.settings', 'game_server.start', 'game_server.stop'],
    game: create(GameSchema, {
      allowStartArgEditing: true,
      linuxBaseCommand: './server',
      linuxStartArgsTemplate: '[{"id":"port","tokens":["--port","25565"]}]',
    }),
  })
}

describe('GameServerStartArgs', () => {
  beforeEach(() => {
    setWebsocketConnectionStatus('connected')
    mocks.listNodes.mockResolvedValue(
      create(ListNodesResponseSchema, {
        nodes: [create(NodeSchema, { id: 'node-1', os: 'linux' })],
      }),
    )
  })

  afterEach(() => {
    mocks.eventBus.reset()
    setWebsocketConnectionStatus('connecting')
    mocks.getGameServer.mockReset()
    mocks.listNodes.mockReset()
    mocks.notify.mockReset()
    mocks.startGameServer.mockReset()
    mocks.stopGameServer.mockReset()
    mocks.updateGameServerStartArgs.mockReset()
  })

  it('saves, stops, waits for offline, and starts through supported RPCs', async () => {
    const onlineServer = buildGameServer(Status.ONLINE)
    const offlineServer = buildGameServer(Status.OFFLINE)
    mocks.getGameServer
      .mockResolvedValueOnce(create(GetGameServerResponseSchema, { gameServer: onlineServer }))
      .mockResolvedValueOnce(create(GetGameServerResponseSchema, { gameServer: offlineServer }))
    mocks.updateGameServerStartArgs.mockResolvedValue(
      create(UpdateGameServerStartArgsResponseSchema, { gameServer: onlineServer }),
    )
    mocks.stopGameServer.mockResolvedValue({})
    mocks.startGameServer.mockResolvedValue({})

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      saveAndRestart: () => Promise<void>
    }
    await viewModel.saveAndRestart()

    expect(mocks.updateGameServerStartArgs).toHaveBeenCalledTimes(1)
    expect(mocks.stopGameServer).toHaveBeenCalledWith(
      expect.objectContaining({ serverId: 'server-1' }),
    )
    expect(mocks.getGameServer).toHaveBeenCalledTimes(2)
    expect(mocks.startGameServer).toHaveBeenCalledWith(
      expect.objectContaining({ serverId: 'server-1' }),
    )
    expect(mocks.updateGameServerStartArgs.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.stopGameServer.mock.invocationCallOrder[0] ?? 0,
    )
    expect(mocks.stopGameServer.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.startGameServer.mock.invocationCallOrder[0] ?? 0,
    )
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ caption: 'Start arguments saved and server restarted.' }),
    )
  })

  it('preserves the authoritative runtime status after saving start arguments', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
    )
    mocks.updateGameServerStartArgs.mockResolvedValue(
      create(UpdateGameServerStartArgsResponseSchema, {
        gameServer: buildGameServer(Status.UNKNOWN),
      }),
    )

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      gameServer: { status: Status }
      restartStateAuthoritative: boolean
      saveOnly: () => Promise<void>
    }
    await viewModel.saveOnly()

    expect(viewModel.gameServer.status).toBe(Status.ONLINE)
    expect(viewModel.restartStateAuthoritative).toBe(true)
    expect(mocks.updateGameServerStartArgs).toHaveBeenCalledTimes(1)
  })

  it('keeps restart disabled while live status is stale or unknown', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
    )

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as { restartStateAuthoritative: boolean }
    expect(viewModel.restartStateAuthoritative).toBe(true)

    setWebsocketConnectionStatus('reconnecting')
    mocks.eventBus.emit('websocketDisconnected')
    expect(viewModel.restartStateAuthoritative).toBe(false)

    setWebsocketConnectionStatus('connected')
    mocks.eventBus.emit('gameServerStatus', 'server-1', 'Server', Status.UNKNOWN)
    expect(viewModel.restartStateAuthoritative).toBe(false)

    mocks.eventBus.emit('gameServerStatus', 'server-1', 'Server', Status.ONLINE)
    expect(viewModel.restartStateAuthoritative).toBe(true)
  })
})
