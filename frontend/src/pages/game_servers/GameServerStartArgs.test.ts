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
  type UpdateGameServerStartArgsResponse,
  UpdateGameServerStartArgsResponseSchema,
} from '@/proto/xylona_pb'
import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'
import GameServerStartArgs from './GameServerStartArgs.vue'
import type { StartArgPatch } from '@/components/game_servers/start-args'

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
  notifyConnectError: mocks.notify,
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

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })
  return { promise, resolve, reject }
}

type StartArgsViewModel = {
  draftBaseCommandOverride: string
  draftPatches: StartArgPatch[]
  isDirty: boolean
  saving: boolean
  restarting: boolean
  saveOnly: () => Promise<void>
  saveAndRestart: () => Promise<void>
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

  function queryWithPlayers(players: number, responded = true) {
    return create(QueryGameServerResponseSchema, {
      queryInfo: create(ServerQuerySchema, {
        type: ServerQuery_Type.Source,
        source: create(SourceQueryInfoSchema, { players, responded }),
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

  const twoOnline = '2 players are online and will be disconnected while the server restarts.'
  const unknownOnline =
    'Player count unknown — anyone connected will be disconnected while the server restarts.'
  it.each([
    {
      name: 'restarts at once with nobody online',
      players: 0,
      responded: true,
      confirm: true,
      message: null,
    },
    {
      name: 'restarts once the operator confirms',
      players: 2,
      responded: true,
      confirm: true,
      message: twoOnline,
    },
    {
      name: 'saves nothing when the operator cancels',
      players: 2,
      responded: true,
      confirm: false,
      message: twoOnline,
    },
    {
      name: 'asks first when the query did not answer',
      players: 0,
      responded: false,
      confirm: true,
      message: unknownOnline,
    },
  ])('Save & Restart $name', async ({ players, responded, confirm, message }) => {
    const onlineServer = buildGameServer(Status.ONLINE, './custom-start.sh')
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: onlineServer }),
    )
    mocks.queryGameServer.mockResolvedValue(queryWithPlayers(players, responded))
    mocks.updateGameServerStartArgs.mockResolvedValue(
      create(UpdateGameServerStartArgsResponseSchema, { gameServer: onlineServer }),
    )
    mocks.restartGameServer.mockResolvedValue({})
    answerRestartDialog(confirm)

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    await (wrapper.vm as unknown as { saveAndRestart: () => Promise<void> }).saveAndRestart()

    expect(mocks.dialog).toHaveBeenCalledTimes(message === null ? 0 : 1)
    if (message !== null) {
      expect(mocks.dialog).toHaveBeenCalledWith(
        expect.objectContaining({ title: 'Restart Survival?', message }),
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

  it('offers Retry instead of the editor when the server fails to load', async () => {
    mocks.getGameServer.mockRejectedValueOnce(new Error('controller offline'))

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()

    expect(wrapper.find('[data-test="start-args-load-error"]').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'StartArgsEditor' }).exists()).toBe(false)

    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
    )
    await (wrapper.vm as unknown as { initialize: () => Promise<void> }).initialize()
    await flushPromises()

    expect(wrapper.find('[data-test="start-args-load-error"]').exists()).toBe(false)
    expect(wrapper.findComponent({ name: 'StartArgsEditor' }).exists()).toBe(true)
  })

  it('asks before Reset All discards the draft', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
    )
    mocks.dialog.mockReturnValue({ onOk: vi.fn() })

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      draftBaseCommandOverride: string
      confirmResetAll: () => void
    }
    viewModel.draftBaseCommandOverride = './draft.sh'
    viewModel.confirmResetAll()

    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Reset all start arguments?', focus: 'cancel' }),
    )
    expect(viewModel.draftBaseCommandOverride).toBe('./draft.sh')
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

    mocks.dialog.mockReturnValue({ onOk: (handler: () => void) => handler() })

    const wrapper = shallowMount(GameServerStartArgs, {
      global: { stubs: { PageHeader: false } },
    })
    await flushPromises()
    const editor = wrapper.getComponent({ name: 'StartArgsEditor' })
    const viewModel = wrapper.vm as unknown as {
      draftBaseCommandOverride: string
      draftPatches: Array<{ id: string }>
    }
    // The overrides are saved, so the draft is clean; Reset All must still be available.
    const resetButton = wrapper
      .findAllComponents({ name: 'QBtn' })
      .find((button) => button.props('label') === 'Reset All')

    expect(editor.props('allowProtectedEditing')).toBe(true)
    expect(resetButton?.props('disable')).toBe(false)
    await resetButton?.trigger('click')
    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({ title: 'Reset all start arguments?' }),
    )
    expect(viewModel.draftPatches).toEqual([])
    expect(viewModel.draftBaseCommandOverride).toBe('')
  })

  it('offers Retry when the node list fails to load', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
    )
    mocks.listNodes.mockRejectedValueOnce(new Error('nodes unavailable'))

    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()

    expect(wrapper.find('[data-test="start-args-load-error"]').exists()).toBe(true)
    expect(wrapper.findComponent({ name: 'StartArgsEditor' }).exists()).toBe(false)
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

  it.each([false, true])(
    'keeps newer base-command and argument edits after saving (restart: %s)',
    async (restart) => {
      mocks.superUser = true
      mocks.getGameServer.mockResolvedValue(
        create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
      )
      mocks.queryGameServer.mockResolvedValue(queryWithPlayers(0))
      mocks.restartGameServer.mockResolvedValue({})
      const response = deferred<UpdateGameServerStartArgsResponse>()
      mocks.updateGameServerStartArgs.mockReturnValueOnce(response.promise)
      const wrapper = shallowMount(GameServerStartArgs)
      await flushPromises()
      const viewModel = wrapper.vm as unknown as StartArgsViewModel
      const editor = wrapper.getComponent({ name: 'StartArgsEditor' })
      const submittedPatches: StartArgPatch[] = [
        { id: 'port', op: 'edit', tokens: ['--port', '28010'] },
      ]
      editor.vm.$emit('update:base-command-override', './submitted.sh')
      editor.vm.$emit('update:patches', submittedPatches)
      const saving = restart ? viewModel.saveAndRestart() : viewModel.saveOnly()
      await flushPromises()
      expect(mocks.updateGameServerStartArgs).toHaveBeenCalledWith(
        expect.objectContaining({
          baseCommandOverride: './submitted.sh',
          startArgsPatches: JSON.stringify(submittedPatches),
        }),
      )

      const newerPatches: StartArgPatch[] = [
        { id: 'port', op: 'edit', tokens: ['--port', '28020'] },
      ]
      editor.vm.$emit('update:base-command-override', './newer.sh')
      editor.vm.$emit('update:patches', newerPatches)
      response.resolve(
        create(UpdateGameServerStartArgsResponseSchema, {
          gameServer: buildGameServer(
            Status.UNKNOWN,
            './submitted.sh',
            JSON.stringify(submittedPatches),
          ),
        }),
      )
      await saving
      await flushPromises()

      expect(editor.props('baseCommandOverride')).toBe('./newer.sh')
      expect(editor.props('patches')).toEqual(newerPatches)
      expect(viewModel.isDirty).toBe(true)
      expect(mocks.notify).toHaveBeenCalledWith(
        expect.stringContaining('Newer edits are still unsaved.'),
      )
      expect(mocks.restartGameServer).toHaveBeenCalledTimes(Number(restart))

      mocks.updateGameServerStartArgs.mockResolvedValueOnce(
        create(UpdateGameServerStartArgsResponseSchema, {
          gameServer: buildGameServer(Status.UNKNOWN, './newer.sh', JSON.stringify(newerPatches)),
        }),
      )
      await viewModel.saveOnly()
      expect(viewModel.isDirty).toBe(false)
      expect(mocks.updateGameServerStartArgs).toHaveBeenLastCalledWith(
        expect.objectContaining({
          baseCommandOverride: './newer.sh',
          startArgsPatches: JSON.stringify(newerPatches),
        }),
      )
      wrapper.unmount()
    },
  )

  it('rejects repeated save events while a submission is pending', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
    )
    const response = deferred<UpdateGameServerStartArgsResponse>()
    mocks.updateGameServerStartArgs.mockReturnValue(response.promise)
    mocks.queryGameServer.mockResolvedValue(queryWithPlayers(0))
    mocks.restartGameServer.mockResolvedValue({})
    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as StartArgsViewModel
    viewModel.draftBaseCommandOverride = './submitted.sh'
    const saving = viewModel.saveOnly()
    const repeatedSave = viewModel.saveOnly()
    const repeatedRestart = viewModel.saveAndRestart()
    const callsWhilePending = mocks.updateGameServerStartArgs.mock.calls.length
    response.resolve(
      create(UpdateGameServerStartArgsResponseSchema, {
        gameServer: buildGameServer(Status.UNKNOWN, './submitted.sh'),
      }),
    )
    await Promise.all([saving, repeatedSave, repeatedRestart])
    expect(callsWhilePending).toBe(1)
    expect(mocks.queryGameServer).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('still warns before leaving when a pending save draft is reverted to the old value', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
    )
    const response = deferred<UpdateGameServerStartArgsResponse>()
    mocks.updateGameServerStartArgs.mockReturnValueOnce(response.promise)
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
    const viewModel = wrapper.vm as unknown as StartArgsViewModel
    viewModel.draftBaseCommandOverride = './submitted.sh'
    const saving = viewModel.saveOnly()
    viewModel.draftBaseCommandOverride = ''
    expect(viewModel.isDirty).toBe(false)
    const canLeaveWhilePending = await mocks.leaveGuard?.()
    response.resolve(
      create(UpdateGameServerStartArgsResponseSchema, {
        gameServer: buildGameServer(Status.UNKNOWN, './submitted.sh'),
      }),
    )
    await saving
    expect(canLeaveWhilePending).toBe(false)
    expect(viewModel.draftBaseCommandOverride).toBe('')
    expect(viewModel.isDirty).toBe(true)
    wrapper.unmount()
  })

  it.each(['base', 'patches'] as const)(
    'preserves newer %s edits while accepting normalization of the unchanged field',
    async (changedField) => {
      mocks.getGameServer.mockResolvedValue(
        create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
      )
      const response = deferred<UpdateGameServerStartArgsResponse>()
      mocks.updateGameServerStartArgs.mockReturnValueOnce(response.promise)
      const wrapper = shallowMount(GameServerStartArgs)
      await flushPromises()
      const viewModel = wrapper.vm as unknown as StartArgsViewModel
      viewModel.draftBaseCommandOverride = ' ./submitted.sh '
      viewModel.draftPatches = [{ id: 'port', op: 'edit', tokens: ['--port', '28010'] }]
      const saving = viewModel.saveOnly()
      if (changedField === 'base') viewModel.draftBaseCommandOverride = './newer.sh'
      else viewModel.draftPatches = []
      const savedPatches: StartArgPatch[] = [
        { id: 'port', op: 'edit', tokens: ['--port', '28010'], label: 'Port' },
      ]
      response.resolve(
        create(UpdateGameServerStartArgsResponseSchema, {
          gameServer: buildGameServer(
            Status.OFFLINE,
            './submitted.sh',
            JSON.stringify(savedPatches),
          ),
        }),
      )
      await saving
      expect(viewModel.draftBaseCommandOverride).toBe(
        changedField === 'base' ? './newer.sh' : './submitted.sh',
      )
      expect(viewModel.draftPatches).toEqual(changedField === 'patches' ? [] : savedPatches)
      expect(viewModel.isDirty).toBe(true)
      wrapper.unmount()
    },
  )

  it('preserves the draft after a failed save and permits a retry', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.OFFLINE) }),
    )
    const response = deferred<UpdateGameServerStartArgsResponse>()
    mocks.updateGameServerStartArgs.mockReturnValueOnce(response.promise)
    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as StartArgsViewModel
    viewModel.draftBaseCommandOverride = './submitted.sh'
    const saving = viewModel.saveOnly()
    viewModel.draftBaseCommandOverride = './newer.sh'
    const error = new Error('node unavailable')
    response.reject(error)
    await saving
    expect(viewModel.draftBaseCommandOverride).toBe('./newer.sh')
    expect(viewModel.isDirty).toBe(true)
    expect(viewModel.saving).toBe(false)
    expect(mocks.notify).toHaveBeenCalledWith(
      error,
      'Failed to save start command',
      expect.anything(),
    )
    mocks.updateGameServerStartArgs.mockResolvedValueOnce(
      create(UpdateGameServerStartArgsResponseSchema, {
        gameServer: buildGameServer(Status.OFFLINE, './newer.sh'),
      }),
    )
    await viewModel.saveOnly()
    expect(viewModel.isDirty).toBe(false)
    wrapper.unmount()
  })

  it.each([false, true])(
    'keeps submission controls locked through restart confirmation (confirm: %s)',
    async (confirm) => {
      mocks.getGameServer.mockResolvedValue(
        create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
      )
      mocks.queryGameServer.mockResolvedValue(queryWithPlayers(2))
      mocks.updateGameServerStartArgs.mockResolvedValue(
        create(UpdateGameServerStartArgsResponseSchema, {
          gameServer: buildGameServer(Status.ONLINE, './submitted.sh'),
        }),
      )
      mocks.restartGameServer.mockResolvedValue({})
      let accept!: () => void
      let dismiss!: () => void
      mocks.dialog.mockReturnValue({
        onOk(handler: () => void) {
          accept = handler
          return this
        },
        onDismiss(handler: () => void) {
          dismiss = handler
          return this
        },
      })
      const wrapper = shallowMount(GameServerStartArgs, {
        global: { stubs: { PageHeader: false } },
      })
      await flushPromises()
      const viewModel = wrapper.vm as unknown as StartArgsViewModel
      viewModel.draftBaseCommandOverride = './submitted.sh'
      const restarting = viewModel.saveAndRestart()
      await flushPromises()
      expect(viewModel.restarting).toBe(true)
      for (const button of wrapper.findAllComponents({ name: 'QBtn' })) {
        expect(button.props('disable')).toBe(true)
      }
      await viewModel.saveOnly()
      await viewModel.saveAndRestart()
      expect(mocks.queryGameServer).toHaveBeenCalledTimes(1)
      expect(mocks.updateGameServerStartArgs).not.toHaveBeenCalled()
      if (confirm) accept()
      dismiss()
      await restarting
      expect(viewModel.restarting).toBe(false)
      expect(mocks.updateGameServerStartArgs).toHaveBeenCalledTimes(Number(confirm))
      expect(mocks.restartGameServer).toHaveBeenCalledTimes(Number(confirm))
      wrapper.unmount()
    },
  )

  it.each(['query', 'confirmation', 'save'] as const)(
    'after leaving during %s, finishes and reports only a restart whose save was sent',
    async (stage) => {
      mocks.getGameServer.mockResolvedValue(
        create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
      )
      const query = deferred<ReturnType<typeof queryWithPlayers>>()
      const save = deferred<UpdateGameServerStartArgsResponse>()
      mocks.queryGameServer.mockReturnValue(query.promise)
      mocks.updateGameServerStartArgs.mockReturnValue(save.promise)
      let accept!: () => void
      mocks.dialog.mockReturnValue({
        onOk(handler: () => void) {
          accept = handler
          return this
        },
        onDismiss() {
          return this
        },
      })
      const wrapper = shallowMount(GameServerStartArgs)
      await flushPromises()
      const viewModel = wrapper.vm as unknown as StartArgsViewModel
      viewModel.draftBaseCommandOverride = './submitted.sh'
      const restarting = viewModel.saveAndRestart()
      if (stage === 'query') wrapper.unmount()
      query.resolve(queryWithPlayers(stage === 'confirmation' ? 2 : 0))
      await flushPromises()
      if (stage !== 'query') wrapper.unmount()
      if (stage === 'confirmation') accept()
      save.resolve(
        create(UpdateGameServerStartArgsResponseSchema, {
          gameServer: buildGameServer(Status.ONLINE, './submitted.sh'),
        }),
      )
      await restarting
      expect(mocks.updateGameServerStartArgs).toHaveBeenCalledTimes(Number(stage === 'save'))
      expect(mocks.restartGameServer).toHaveBeenCalledTimes(Number(stage === 'save'))
      if (stage === 'save') {
        expect(mocks.notify).toHaveBeenCalledWith('Start command saved and server restarted.')
      } else {
        expect(mocks.notify).not.toHaveBeenCalled()
      }
    },
  )

  it('lets the operator leave during the restart and still reports its failure', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, { gameServer: buildGameServer(Status.ONLINE) }),
    )
    mocks.queryGameServer.mockResolvedValue(queryWithPlayers(0))
    mocks.updateGameServerStartArgs.mockResolvedValue(
      create(UpdateGameServerStartArgsResponseSchema, {
        gameServer: buildGameServer(Status.ONLINE, './submitted.sh'),
      }),
    )
    const restart = deferred<unknown>()
    mocks.restartGameServer.mockReturnValue(restart.promise)
    const wrapper = shallowMount(GameServerStartArgs)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as StartArgsViewModel
    viewModel.draftBaseCommandOverride = './submitted.sh'
    const restarting = viewModel.saveAndRestart()
    await flushPromises()
    expect(mocks.restartGameServer).toHaveBeenCalledTimes(1)
    expect(viewModel.isDirty).toBe(false)
    expect(await mocks.leaveGuard?.()).toBe(true)

    wrapper.unmount()
    const error = new Error('server did not start')
    restart.reject(error)
    await restarting
    expect(mocks.notify).toHaveBeenCalledWith(
      error,
      'Start command was saved, but the server could not restart',
      expect.anything(),
    )
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
