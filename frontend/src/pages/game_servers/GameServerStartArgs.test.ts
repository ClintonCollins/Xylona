import { create } from '@bufbuild/protobuf'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  GameSchema,
  GameServerSchema,
  NodeSchema,
  ServerQuery_Type,
  ServerQuerySchema,
  SourceQueryInfoSchema,
  Status,
} from '@/proto/shared_pb'
import {
  GetGameServerResponseSchema,
  ListNodesResponseSchema,
  QueryGameServerResponseSchema,
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
    superUser: false,
    dialog: vi.fn(),
    leaveGuard: undefined as undefined | (() => boolean | Promise<boolean>),
    getGameServer: vi.fn(),
    listNodes: vi.fn(),
    notify: vi.fn(),
    queryGameServer: vi.fn(),
    restartGameServer: vi.fn(),
    updateGameServerStartArgs: vi.fn(),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ dialog: mocks.dialog, notify: mocks.notify }),
  }
})

// Every toast goes through the shared helpers; record them on one mock.
vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notify,
  notifyError: mocks.notify,
  notifyWarning: mocks.notify,
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
  useRouter: () => ({ replace: vi.fn() }),
  onBeforeRouteLeave: (guard: () => boolean | Promise<boolean>) => {
    mocks.leaveGuard = guard
  },
  onBeforeRouteUpdate: vi.fn(),
}))

vi.mock('@/stores/xylona', () => ({
  useUserAuthStore: () => ({
    checkUserAuthenticated: vi.fn().mockResolvedValue({
      user: { superUser: mocks.superUser },
    }),
    user: { superUser: mocks.superUser },
  }),
}))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    listNodes: mocks.listNodes,
    queryGameServer: mocks.queryGameServer,
    restartGameServer: mocks.restartGameServer,
    updateGameServerStartArgs: mocks.updateGameServerStartArgs,
  }),
  XylonaEventBus: mocks.eventBus,
}))

function buildGameServer(status: Status, baseCommandOverride = '', startArgsPatches = '') {
  return create(GameServerSchema, {
    id: 'server-1',
    nodeId: 'node-1',
    status,
    baseCommandOverride,
    startArgsPatches,
    name: 'Survival',
    effectivePermissions: ['game_server.settings', 'game_server.restart'],
    game: create(GameSchema, {
      allowStartArgEditing: true,
      linuxBaseCommand: '{{INSTALL_DIR}}/server',
      linuxStartArgsTemplate:
        '[{"id":"locked","order":1,"ownership":"locked","tokens":["--safe"]},{"id":"port","order":2,"ownership":"editable","tokens":["--port","25565"]}]',
    }),
    directory: '/srv/game',
  })
}

describe('GameServerStartArgs', () => {
  beforeEach(() => {
    mocks.superUser = false
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
    mocks.queryGameServer.mockReset()
    mocks.restartGameServer.mockReset()
    mocks.updateGameServerStartArgs.mockReset()
    mocks.dialog.mockReset()
  })

  it('asks before leaving with an unsaved draft', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
    )
    mocks.dialog.mockReturnValue({
      onOk() {
        return this
      },
      onCancel(handler: () => void) {
        handler()
        return this
      },
      onDismiss() {
        return this
      },
    })

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    expect(await mocks.leaveGuard?.()).toBe(true)

    wrapper
      .getComponent({ name: 'StartArgsEditor' })
      .vm.$emit('update:base-command-override', './draft.sh')
    await flushPromises()

    expect(await mocks.leaveGuard?.()).toBe(false)
    expect(mocks.dialog).toHaveBeenCalledWith(expect.objectContaining({ title: 'Unsaved Changes' }))
  })

  function queryWithPlayers(players: number) {
    return create(QueryGameServerResponseSchema, {
      queryInfo: create(ServerQuerySchema, {
        type: ServerQuery_Type.Source,
        source: create(SourceQueryInfoSchema, { players }),
      }),
    })
  }

  function answerRestartDialog(confirm: boolean) {
    mocks.dialog.mockReturnValue({
      onOk(handler: () => void) {
        if (confirm) handler()
        return this
      },
      onDismiss(handler: () => void) {
        handler()
        return this
      },
    })
  }

  it.each([
    { name: 'restarts at once with nobody online', players: 0, confirm: true, asked: false },
    { name: 'restarts once the operator confirms', players: 2, confirm: true, asked: true },
    { name: 'saves nothing when the operator cancels', players: 2, confirm: false, asked: true },
  ])('Save & Restart $name', async ({ players, confirm, asked }) => {
    const onlineServer = buildGameServer(Status.ONLINE, './custom-start.sh')
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: onlineServer }),
    )
    mocks.queryGameServer.mockResolvedValue(queryWithPlayers(players))
    mocks.updateGameServerStartArgs.mockResolvedValue(
      create(UpdateGameServerStartArgsResponseSchema, { gameServer: onlineServer }),
    )
    mocks.restartGameServer.mockResolvedValue({})
    answerRestartDialog(confirm)

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    await (wrapper.vm as unknown as { saveAndRestart: () => Promise<void> }).saveAndRestart()

    expect(mocks.dialog).toHaveBeenCalledTimes(asked ? 1 : 0)
    if (asked) {
      expect(mocks.dialog).toHaveBeenCalledWith(
        expect.objectContaining({
          title: 'Restart Survival?',
          message: '2 players are online and will be disconnected while the server restarts.',
        }),
      )
    }
    if (!confirm) {
      expect(mocks.updateGameServerStartArgs).not.toHaveBeenCalled()
      expect(mocks.restartGameServer).not.toHaveBeenCalled()
      return
    }
    expect(mocks.updateGameServerStartArgs).toHaveBeenCalledWith(
      expect.objectContaining({ serverId: 'server-1', baseCommandOverride: './custom-start.sh' }),
    )
    expect(mocks.restartGameServer).toHaveBeenCalledWith(
      expect.objectContaining({ serverId: 'server-1' }),
    )
    expect(mocks.updateGameServerStartArgs.mock.invocationCallOrder[0]).toBeLessThan(
      mocks.restartGameServer.mock.invocationCallOrder[0] ?? 0,
    )
    expect(mocks.notify).toHaveBeenCalledWith('Start command saved and server restarted.')
  })

  it('shows the effective command read-only to ordinary users', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildGameServer(Status.OFFLINE, './custom-start.sh'),
      }),
    )

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()

    const editor = wrapper.getComponent({ name: 'StartArgsEditor' })
    expect(editor.props('allowProtectedEditing')).toBe(false)
    expect(editor.props('baseCommand')).toBe('./custom-start.sh')
  })

  it('resets only editable patches for ordinary users', async () => {
    const patches = JSON.stringify([
      { id: 'locked', op: 'edit', tokens: ['--custom'] },
      { id: 'port', op: 'edit', tokens: ['--port', '28010'] },
      { id: 'added', op: 'add', tokens: ['--extra'], afterId: 'port' },
    ])
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildGameServer(Status.OFFLINE, './custom-start.sh', patches),
      }),
    )

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      draftBaseCommandOverride: string
      draftPatches: Array<{ id: string }>
      resetAll: () => void
    }
    viewModel.resetAll()

    expect(viewModel.draftPatches.map((patch) => patch.id)).toEqual(['locked'])
    expect(viewModel.draftBaseCommandOverride).toBe('./custom-start.sh')
  })

  it('lets superusers reset every server override', async () => {
    mocks.superUser = true
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildGameServer(
          Status.OFFLINE,
          './custom-start.sh',
          '[{"id":"locked","op":"remove"}]',
        ),
      }),
    )

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const editor = wrapper.getComponent({ name: 'StartArgsEditor' })
    const viewModel = wrapper.vm as unknown as {
      draftBaseCommandOverride: string
      draftPatches: Array<{ id: string }>
      resetAll: () => void
    }

    expect(editor.props('allowProtectedEditing')).toBe(true)
    viewModel.resetAll()
    expect(viewModel.draftPatches).toEqual([])
    expect(viewModel.draftBaseCommandOverride).toBe('')
  })

  it('updates preview and dirty state when the base override changes', async () => {
    mocks.superUser = true
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: buildGameServer(Status.OFFLINE),
      }),
    )

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      draftBaseCommandOverride: string
      draftChangeCount: number
      isDirty: boolean
      resolvedBaseCommand: string
    }

    expect(viewModel.resolvedBaseCommand).toBe('/srv/game/server')
    viewModel.draftBaseCommandOverride = '{{INSTALL_DIR}}/custom-start.sh'
    await wrapper.vm.$nextTick()
    expect(viewModel.resolvedBaseCommand).toBe('/srv/game/custom-start.sh')
    expect(viewModel.draftChangeCount).toBe(1)
    expect(viewModel.isDirty).toBe(true)
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
