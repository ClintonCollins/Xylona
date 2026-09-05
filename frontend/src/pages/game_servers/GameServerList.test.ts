import { createPinia, setActivePinia } from 'pinia'
import { create as createProto } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  GameServerSchema,
  NodeSchema,
  ServerQuery_Type,
  Status,
  VersionInfoSchema,
  VersionStatus,
} from '@/proto/shared_pb'
import {
  AggregatedGameServerSchema,
  RemoteServerSummarySchema,
  StepStatus,
  UpdateProgressSchema,
  UpdateStep,
  UserSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import GameServerList from './GameServerList.vue'
import type { DisplayRow } from './server-list-cache'
import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'

const mocks = vi.hoisted(() => {
  const dialogChoice = { value: 'ok' as 'ok' | 'dismiss' }
  const dialog = vi.fn(() => {
    const chain = {
      onOk(callback: (value?: unknown) => void) {
        if (dialogChoice.value === 'ok') {
          callback()
        }
        return chain
      },
      onDismiss(callback: () => void) {
        callback()
        return chain
      },
    }
    return chain
  })

  return {
    dialog,
    dialogChoice,
    notify: vi.fn(),
    listAggregatedGameServers: vi.fn(),
    listGameServers: vi.fn(),
    listNodes: vi.fn(),
    startGameServer: vi.fn(),
    stopGameServer: vi.fn(),
    restartGameServer: vi.fn(),
    updateGameServer: vi.fn(),
    websocketClient: {
      isOpen: vi.fn(() => true),
      send: vi.fn(),
    },
    eventBus: (() => {
      const listeners = new Map<string, Set<(...args: unknown[]) => void>>()

      return {
        on: vi.fn((eventName: string, handler: (...args: unknown[]) => void) => {
          const handlers = listeners.get(eventName) ?? new Set<(...args: unknown[]) => void>()
          handlers.add(handler)
          listeners.set(eventName, handlers)
        }),
        off: vi.fn((eventName: string, handler?: (...args: unknown[]) => void) => {
          if (handler === undefined) {
            listeners.delete(eventName)
            return
          }

          const handlers = listeners.get(eventName)
          if (!handlers) {
            return
          }

          handlers.delete(handler)
          if (handlers.size === 0) {
            listeners.delete(eventName)
          }
        }),
        emit: (eventName: string, ...args: unknown[]) => {
          const handlers = listeners.get(eventName)
          if (!handlers) {
            return
          }

          for (const handler of handlers) {
            handler(...args)
          }
        },
        reset: () => {
          listeners.clear()
        },
      }
    })(),
  }
})

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      listAggregatedGameServers: mocks.listAggregatedGameServers,
      listGameServers: mocks.listGameServers,
      listNodes: mocks.listNodes,
      startGameServer: mocks.startGameServer,
      stopGameServer: mocks.stopGameServer,
      restartGameServer: mocks.restartGameServer,
      updateGameServer: mocks.updateGameServer,
    }),
    GetOrCreateXylonaWebsocketClient: () => mocks.websocketClient,
    XylonaEventBus: mocks.eventBus,
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      dialog: mocks.dialog,
      notify: mocks.notify,
      screen: { lt: { md: false }, xs: false },
    }),
  }
})

vi.mock('@/utils/persisted-ref', () => ({
  usePersistedRef: <T>(_key: string, initialValue: T) => ref(initialValue),
}))

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<button>{{ label }}</button>',
})

function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })

  return { promise, resolve, reject }
}

function buildLocalNode(overrides: Record<string, unknown> = {}) {
  return createProto(NodeSchema, {
    id: 'node-local',
    name: 'Local Node',
    host: 'localhost',
    port: 8080n,
    local: true,
    secretKey: '',
    baseUrl: 'http://localhost:8080',
    enabled: true,
    lastSyncStatus: '',
    healthStatus: '',
    version: '',
    protocolVersion: 0n,
    capabilities: '',
    allowInsecureTls: false,
    departed: false,
    autoPaired: false,
    ...overrides,
  })
}

function buildRemoteNode(overrides: Record<string, unknown> = {}) {
  return createProto(NodeSchema, {
    id: 'node-remote',
    name: 'Remote Node',
    host: 'remote-host',
    port: 9443n,
    local: false,
    secretKey: '',
    baseUrl: 'https://remote.example.com',
    enabled: true,
    lastSyncStatus: '',
    healthStatus: 'healthy',
    version: '',
    protocolVersion: 0n,
    capabilities: '',
    allowInsecureTls: false,
    departed: false,
    autoPaired: false,
    ...overrides,
  })
}

function buildLocalServer(overrides: Record<string, unknown> = {}) {
  return createProto(GameServerSchema, {
    id: 'server-local',
    name: 'Local Server',
    gameName: 'Minecraft',
    nodeId: 'node-local',
    status: Status.OFFLINE,
    version: '1.20.4',
    ...overrides,
  })
}

function buildLocalAggregatedServer(name = 'Local Server') {
  return createProto(AggregatedGameServerSchema, {
    isLocal: true,
    localServer: buildLocalServer({ name }),
  })
}

function buildRemoteAggregatedServer(name = 'Remote Server') {
  return createProto(AggregatedGameServerSchema, {
    isLocal: false,
    remoteServer: createProto(RemoteServerSummarySchema, {
      sourceNodeId: 'node-remote',
      nodeId: 'node-remote',
      remoteServerId: 'server-remote',
      displayName: name,
      status: Status.OFFLINE,
      gameName: 'Minecraft',
      gameId: 'minecraft',
      nodeName: 'Remote Node',
      nodeHost: 'remote-host',
      version: '1.20.4',
    }),
  })
}

describe('GameServerList', () => {
  beforeEach(() => {
    setWebsocketConnectionStatus('connected')
    setActivePinia(createPinia())
    mocks.listAggregatedGameServers.mockReset()
    mocks.listGameServers.mockReset()
    mocks.listNodes.mockReset()
    mocks.startGameServer.mockReset()
    mocks.stopGameServer.mockReset()
    mocks.restartGameServer.mockReset()
    mocks.updateGameServer.mockReset()
    mocks.websocketClient.isOpen.mockReset()
    mocks.websocketClient.isOpen.mockReturnValue(true)
    mocks.websocketClient.send.mockReset()
    mocks.eventBus.reset()
    mocks.eventBus.on.mockClear()
    mocks.eventBus.off.mockClear()
    mocks.dialog.mockClear()
    mocks.dialogChoice.value = 'ok'
    mocks.notify.mockClear()
    mocks.listAggregatedGameServers.mockResolvedValue({ servers: [] })
    mocks.listGameServers.mockResolvedValue({
      gameServers: [buildLocalServer()],
    })
    mocks.listNodes.mockResolvedValue({
      nodes: [buildLocalNode()],
    })
    mocks.startGameServer.mockResolvedValue({})
    mocks.stopGameServer.mockResolvedValue({})
    mocks.restartGameServer.mockResolvedValue({})
    mocks.updateGameServer.mockResolvedValue({})
  })

  afterEach(() => {
    setWebsocketConnectionStatus('connecting')
  })

  function mountList(superUser: boolean, rowSlot?: 'item' | 'body-cell-actions') {
    const store = useUserAuthStore()
    store.user = createProto(UserSchema, {
      id: superUser ? 'user-admin' : 'user-owner',
      userName: superUser ? 'admin' : 'owner',
      superUser,
    })

    return mount(GameServerList, {
      global: {
        stubs: {
          'q-page': { template: '<div><slot /></div>' },
          'q-input': { template: '<div><slot /></div>' },
          'q-icon': true,
          'q-badge': true,
          'q-card': { template: '<div><slot /></div>' },
          'q-card-section': { template: '<div><slot /></div>' },
          'q-card-actions': { template: '<div><slot /></div>' },
          'q-checkbox': true,
          'q-space': true,
          'q-td': { template: '<div><slot /></div>' },
          'q-tooltip': true,
          'q-spinner': true,
          'q-skeleton': true,
          'q-separator': { template: '<span />' },
          'q-toolbar': { template: '<div><slot /></div>' },
          'q-table': defineComponent({
            name: 'QTableStub',
            setup: () => ({ rowSlot }),
            props: {
              rows: { type: Array, default: () => [] },
              loading: { type: Boolean, default: false },
            },
            template:
              '<div>' +
              '<div data-test="q-table-row-count">{{ rows.length }}</div>' +
              '<div data-test="q-table-loading">{{ loading }}</div>' +
              '<slot />' +
              '<slot name="bottom-row" />' +
              '<slot name="no-data" />' +
              '<template v-if="rowSlot"><slot v-for="row in rows" :name="rowSlot" :row="row" :selected="false" /></template>' +
              '</div>',
          }),
          'router-link': { template: '<a><slot /></a>' },
          'delete-game-server-dialog': true,
          StatusBadge: true,
          'q-btn': QBtnStub,
        },
      },
    })
  }

  it.each(['item', 'body-cell-actions'] as const)(
    'renders lifecycle controls in order and starts the server from the %s layout',
    async (rowSlot) => {
      mocks.listAggregatedGameServers.mockResolvedValue({
        servers: [buildLocalAggregatedServer()],
      })
      const startRequest = createDeferred<Record<string, never>>()
      mocks.startGameServer.mockReturnValueOnce(startRequest.promise)
      const wrapper = mountList(true, rowSlot)
      await flushPromises()

      const buttons = wrapper.find('.server-lifecycle-actions').findAll('button')
      expect(
        buttons.map((button) => [
          button.attributes('aria-label'),
          button.attributes('color'),
          button.attributes('icon'),
          button.attributes('disable'),
        ]),
      ).toEqual([
        ['Start Local Server', 'positive', 'play_arrow', 'false'],
        ['Restart Local Server', 'warning', 'restart_alt', 'true'],
        ['Stop Local Server', 'negative', 'stop', 'true'],
        ['Update Local Server', 'accent', 'system_update_alt', 'true'],
      ])

      const startButton = buttons[0]
      if (!startButton) throw new Error('expected start button')
      await startButton.trigger('click')
      expect(mocks.startGameServer).toHaveBeenCalledWith(
        expect.objectContaining({ serverId: 'server-local' }),
      )
      expect(startButton.attributes('loading')).toBe('true')
      expect(buttons.every((button) => button.attributes('disable') === 'true')).toBe(true)

      startRequest.resolve({})
      await flushPromises()
      const refreshedStartButton = wrapper.get('button[aria-label="Start Local Server"]')
      expect(refreshedStartButton.attributes('loading')).toBe('false')
      expect(refreshedStartButton.attributes('disable')).toBe('false')
    },
  )

  it('hides the create button for non-superusers', async () => {
    const wrapper = mountList(false)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Create Game Server')
  })

  it('shows the create button for superusers', async () => {
    const wrapper = mountList(true)
    await flushPromises()

    const createButtons = wrapper
      .findAll('button')
      .filter((button) => button.text() === 'Create Game Server')
    expect(createButtons).toHaveLength(1)
  })

  it('distinguishes an empty fleet from search with no matches', async () => {
    const wrapper = mountList(true)
    await flushPromises()

    expect(wrapper.text()).toContain('No game servers')
    expect(wrapper.text()).toContain('Create a game server to get started.')

    const viewModel = wrapper.vm as unknown as { search: string }
    viewModel.search = 'Minecraft'
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('No matching game servers')
    expect(wrapper.text()).toContain('No game servers match “Minecraft”.')
    expect(wrapper.text()).toContain('Clear search')
    expect(wrapper.text()).not.toContain('Create a game server to get started.')
  })

  it('does not call the local game server list endpoint', async () => {
    mountList(true)
    await flushPromises()

    expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(1)
    expect(mocks.listGameServers).not.toHaveBeenCalled()
  })

  it('starts selected servers concurrently', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ id: 'server-a', name: 'Server A' }),
        }),
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ id: 'server-b', name: 'Server B' }),
        }),
      ],
    })
    const firstStart = createDeferred<Record<string, never>>()
    const secondStart = createDeferred<Record<string, never>>()
    mocks.startGameServer.mockImplementation((request: { serverId: string }) => {
      return request.serverId === 'server-a' ? firstStart.promise : secondStart.promise
    })

    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      selectedGameServers: DisplayRow[]
      startSelectedGameServers: () => Promise<void>
    }
    vm.selectedGameServers = [...vm.displayRows]

    const action = vm.startSelectedGameServers()
    await vi.waitFor(() => {
      expect(mocks.startGameServer).toHaveBeenCalledTimes(2)
    })
    expect(
      mocks.startGameServer.mock.calls.map(
        ([request]) => (request as { serverId: string }).serverId,
      ),
    ).toEqual(['server-a', 'server-b'])

    firstStart.resolve({})
    secondStart.resolve({})
    await action
  })

  it('uses the atomic restart endpoint', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ status: Status.ONLINE }),
        }),
      ],
    })
    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      runServerAction: (action: 'restart', server: DisplayRow) => Promise<void>
    }
    const server = vm.displayRows[0]
    if (!server) throw new Error('expected online server')

    await vm.runServerAction('restart', server)

    expect(mocks.restartGameServer).toHaveBeenCalledWith(
      expect.objectContaining({ serverId: 'server-local' }),
    )
    expect(mocks.stopGameServer).not.toHaveBeenCalled()
    expect(mocks.startGameServer).not.toHaveBeenCalled()
  })

  it.each([
    {
      label: 'confirms a row stop when players are online',
      action: 'stop' as const,
      playerCount: 3n,
      dialogChoice: 'ok' as const,
      wantDialog: {
        title: 'Stop Server A?',
        message: '3 players are online and will be disconnected.',
        confirmColor: 'negative',
      },
      wantCalls: 1,
    },
    {
      label: 'aborts a row stop when the player confirm is cancelled',
      action: 'stop' as const,
      playerCount: 3n,
      dialogChoice: 'dismiss' as const,
      wantDialog: {
        title: 'Stop Server A?',
        message: '3 players are online and will be disconnected.',
        confirmColor: 'negative',
      },
      wantCalls: 0,
    },
    {
      label: 'stops a row immediately when no players are online',
      action: 'stop' as const,
      playerCount: 0n,
      dialogChoice: 'ok' as const,
      wantDialog: null,
      wantCalls: 1,
    },
    {
      label: 'confirms a row restart when players are online',
      action: 'restart' as const,
      playerCount: 2n,
      dialogChoice: 'ok' as const,
      wantDialog: {
        title: 'Restart Server A?',
        message: '2 players are online and will be disconnected while the server restarts.',
        confirmColor: 'warning',
      },
      wantCalls: 1,
    },
    {
      label: 'restarts a row immediately when no players are online',
      action: 'restart' as const,
      playerCount: 0n,
      dialogChoice: 'ok' as const,
      wantDialog: null,
      wantCalls: 1,
    },
  ])('$label', async ({ action, playerCount, dialogChoice, wantDialog, wantCalls }) => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            id: 'server-a',
            name: 'Server A',
            status: Status.ONLINE,
            currentPlayerCount: playerCount,
            maxPlayers: 10n,
          }),
        }),
      ],
    })
    mocks.dialogChoice.value = dialogChoice

    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      runServerAction: (action: 'stop' | 'restart', server: DisplayRow) => Promise<void>
    }
    const server = vm.displayRows[0]
    if (!server) throw new Error('expected online server')

    await vm.runServerAction(action, server)

    if (wantDialog === null) {
      expect(mocks.dialog).not.toHaveBeenCalled()
    } else {
      expect(mocks.dialog).toHaveBeenCalledWith(
        expect.objectContaining({
          title: wantDialog.title,
          message: wantDialog.message,
          ok: expect.objectContaining({ color: wantDialog.confirmColor }),
        }),
      )
    }
    const actionMock = action === 'stop' ? mocks.stopGameServer : mocks.restartGameServer
    expect(actionMock).toHaveBeenCalledTimes(wantCalls)
  })

  it('confirms a bulk stop once, naming total players and server count', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            id: 'server-a',
            name: 'Server A',
            status: Status.ONLINE,
            currentPlayerCount: 2n,
            maxPlayers: 10n,
          }),
        }),
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            id: 'server-b',
            name: 'Server B',
            status: Status.ONLINE,
            currentPlayerCount: 3n,
            maxPlayers: 10n,
          }),
        }),
      ],
    })

    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      selectedGameServers: DisplayRow[]
      stopSelectedGameServers: () => Promise<void>
    }
    vm.selectedGameServers = [...vm.displayRows]

    await vm.stopSelectedGameServers()

    expect(mocks.dialog).toHaveBeenCalledTimes(1)
    expect(mocks.dialog).toHaveBeenCalledWith(
      expect.objectContaining({
        title: 'Stop 2 servers?',
        message: '5 players are online across 2 servers and will be disconnected.',
        ok: expect.objectContaining({ color: 'negative' }),
      }),
    )
    expect(mocks.stopGameServer).toHaveBeenCalledTimes(2)
  })

  it('keeps page tools and bulk actions in the same action row', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ status: Status.ONLINE }),
        }),
      ],
    })

    const wrapper = mountList(true)
    await flushPromises()
    const pageActions = wrapper.find('.xy-page-actions')
    const selectionRegion = wrapper.find('.server-selection-region')
    expect(selectionRegion.exists()).toBe(true)
    expect(selectionRegion.element.parentElement).toBe(pageActions.element)
    expect(selectionRegion.find('.server-selection-toolbar').exists()).toBe(false)
    expect(wrapper.find('.xy-search-input').exists()).toBe(true)
    expect(
      wrapper
        .find('.xy-page-actions')
        .findAll('button')
        .some((button) => button.text() === 'Create Game Server'),
    ).toBe(true)

    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      selectedGameServers: DisplayRow[]
    }
    vm.selectedGameServers = [...vm.displayRows]
    await flushPromises()

    expect(selectionRegion.find('.server-selection-toolbar').exists()).toBe(true)
    expect(wrapper.find('.xy-search-input').exists()).toBe(true)
    const buttons = wrapper.findAll('button')
    expect(buttons.some((button) => button.text() === 'Create Game Server')).toBe(true)
    const stopButton = buttons.find((button) => button.text().startsWith('Stop'))
    const restartButton = buttons.find((button) => button.text().startsWith('Restart'))
    expect(stopButton?.attributes('color')).toBe('negative')
    expect(restartButton?.attributes('color')).toBe('warning')
  })

  it('keeps an update pending until terminal progress arrives', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ resolvedHasUpdate: true }),
        }),
      ],
    })
    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      isServerActionPending: (server: DisplayRow, action: 'update') => boolean
      runServerAction: (action: 'update', server: DisplayRow) => Promise<void>
    }
    const server = vm.displayRows[0]
    if (!server) throw new Error('expected updateable server')

    await vm.runServerAction('update', server)
    expect(vm.isServerActionPending(server, 'update')).toBe(true)

    mocks.eventBus.emit(
      'gameServerUpdateProgress',
      createProto(UpdateProgressSchema, {
        gameServerId: server.id,
        step: UpdateStep.INSTALLING,
        stepStatus: StepStatus.COMPLETED,
      }),
    )
    await flushPromises()
    expect(vm.isServerActionPending(server, 'update')).toBe(false)
  })

  it('updates player counts and resource usage from live server feeds', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            status: Status.ONLINE,
            maxPlayers: 20n,
            currentPlayerCount: 1n,
          }),
        }),
      ],
    })

    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      formatCpuUsage: (row: DisplayRow) => string
      formatMemoryUsage: (row: DisplayRow) => string
      getPlayerCountLabel: (row: DisplayRow) => string
    }
    const row = vm.displayRows[0]
    if (!row) {
      throw new Error('expected the live server row')
    }
    expect(mocks.websocketClient.send).toHaveBeenCalledTimes(1)

    mocks.eventBus.emit('gameServersQueryInfo', {
      servers: {
        'server-local': {
          type: ServerQuery_Type.Minecraft,
          minecraft: { numberOfPlayers: 4, maxPlayers: 20 },
        },
      },
    })
    mocks.eventBus.emit('gameServerMetrics', {
      servers: {
        'server-local': {
          cpuPercent: 12.5,
          cpuValid: true,
          memoryWorkingSetBytes: 512n * 1024n * 1024n,
          memoryBytes: 0n,
          memoryPercent: 25,
          metricsValid: true,
        },
      },
    })
    await flushPromises()

    expect(vm.getPlayerCountLabel(row)).toBe('4 / 20')
    expect(vm.formatCpuUsage(row)).toBe('12.5%')
    expect(vm.formatMemoryUsage(row)).toBe('512.0 MB · 25.0%')

    mocks.eventBus.emit('websocketDisconnected')
    await flushPromises()

    expect(vm.getPlayerCountLabel(row)).toBe('0 / 20')
    expect(vm.formatCpuUsage(row)).toBe('—')
    expect(vm.formatMemoryUsage(row)).toBe('—')
  })

  it('does not reuse telemetry from a previous server process', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            status: Status.ONLINE,
            setMaxPlayers: 20n,
            maxPlayers: 100n,
          }),
        }),
      ],
    })

    const wrapper = mountList(true)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      formatCpuUsage: (row: DisplayRow) => string
      formatMemoryUsage: (row: DisplayRow) => string
      getPlayerCountLabel: (row: DisplayRow) => string
      setServerStatus: (serverID: string, status: Status) => void
    }

    mocks.eventBus.emit('gameServersQueryInfo', {
      servers: {
        'server-local': {
          type: ServerQuery_Type.Minecraft,
          minecraft: { numberOfPlayers: 4, maxPlayers: 20 },
        },
      },
    })
    mocks.eventBus.emit('gameServerMetrics', {
      servers: {
        'server-local': {
          cpuPercent: 12.5,
          cpuValid: true,
          memoryWorkingSetBytes: 512n * 1024n * 1024n,
          memoryPercent: 25,
          metricsValid: true,
        },
      },
    })
    await flushPromises()

    vm.setServerStatus('server-local', Status.OFFLINE)
    vm.setServerStatus('server-local', Status.ONLINE)
    await flushPromises()

    const restartedRow = vm.displayRows[0]
    if (!restartedRow) {
      throw new Error('expected the restarted server row')
    }
    expect(vm.getPlayerCountLabel(restartedRow)).toBe('0 / 20')
    expect(vm.formatCpuUsage(restartedRow)).toBe('—')
    expect(vm.formatMemoryUsage(restartedRow)).toBe('—')
  })

  it('does not subscribe to resource metrics without the metrics permission', async () => {
    mocks.listAggregatedGameServers.mockResolvedValue({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ status: Status.ONLINE }),
        }),
      ],
    })

    const wrapper = mountList(false)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      displayRows: DisplayRow[]
      formatCpuUsage: (row: DisplayRow) => string
      formatMemoryUsage: (row: DisplayRow) => string
    }
    const row = vm.displayRows[0]
    if (!row) {
      throw new Error('expected the live server row')
    }

    expect(mocks.websocketClient.send).not.toHaveBeenCalled()
    expect(vm.formatCpuUsage(row)).toBe('—')
    expect(vm.formatMemoryUsage(row)).toBe('—')
  })

  it('renders local and remote rows from aggregated data', async () => {
    const aggregatedRequest = createDeferred<{ servers: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(aggregatedRequest.promise)
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode(), buildRemoteNode()],
    })

    const wrapper = mountList(true)

    aggregatedRequest.resolve({
      servers: [buildLocalAggregatedServer(), buildRemoteAggregatedServer()],
    })

    await vi.waitFor(() => {
      expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('2')
    })

    await flushPromises()

    expect(
      (wrapper.vm as unknown as { displayRows: Array<{ displayName: string }> }).displayRows,
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ displayName: 'Local Server' }),
        expect.objectContaining({ displayName: 'Remote Server' }),
      ]),
    )
  })

  it('preserves buffered local state when aggregated data resolves after websocket updates', async () => {
    const aggregatedRequest = createDeferred<{ servers: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(aggregatedRequest.promise)
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    const wrapper = mountList(true)

    ;(
      wrapper.vm as unknown as {
        setServerStatus: (serverID: string, status: Status) => void
        setServerVersion: (serverID: string, version: string) => void
      }
    ).setServerStatus('server-local', Status.ONLINE)
    ;(
      wrapper.vm as unknown as {
        setServerStatus: (serverID: string, status: Status) => void
        setServerVersion: (serverID: string, version: string) => void
      }
    ).setServerVersion('server-local', '1.20.6')

    aggregatedRequest.resolve({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            status: Status.OFFLINE,
            version: '1.20.4',
          }),
        }),
      ],
    })
    await flushPromises()

    expect((wrapper.vm as unknown as { displayRows: DisplayRow[] }).displayRows).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          compositeId: 'local/server-local',
          statusEnum: Status.ONLINE,
          version: '1.20.6',
        }),
      ]),
    )
  })

  it('uses aggregated local state when no websocket updates occurred', async () => {
    const aggregatedRequest = createDeferred<{ servers: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(aggregatedRequest.promise)
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    const wrapper = mountList(true)

    aggregatedRequest.resolve({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            status: Status.ONLINE,
            version: '1.20.6',
          }),
        }),
      ],
    })
    await flushPromises()

    expect((wrapper.vm as unknown as { displayRows: DisplayRow[] }).displayRows).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          compositeId: 'local/server-local',
          statusEnum: Status.ONLINE,
          version: '1.20.6',
        }),
      ]),
    )
  })

  it('applies version events that arrive before aggregated rows finish loading', async () => {
    const aggregatedRequest = createDeferred<{ servers: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(aggregatedRequest.promise)
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    const wrapper = mountList(true)

    mocks.eventBus.emit(
      'gameServerVersion',
      'server-local',
      '1.21.1',
      createProto(VersionInfoSchema, {
        status: VersionStatus.CHECKED,
        installedVersion: '1.21.1',
      }),
    )

    aggregatedRequest.resolve({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            version: '1.20.4',
            versionInfo: createProto(VersionInfoSchema, {
              status: VersionStatus.CHECKING,
            }),
          }),
        }),
      ],
    })
    await flushPromises()

    expect((wrapper.vm as unknown as { displayRows: DisplayRow[] }).displayRows).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'server-local',
          version: '1.21.1',
          versionInfo: expect.objectContaining({
            status: VersionStatus.CHECKED,
            installedVersion: '1.21.1',
          }),
        }),
      ]),
    )
  })

  it('reloads the list after websocket reconnects', async () => {
    mocks.listAggregatedGameServers.mockResolvedValueOnce({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            version: '1.20.4',
            versionInfo: createProto(VersionInfoSchema, {
              status: VersionStatus.CHECKING,
            }),
          }),
        }),
      ],
    })
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    const wrapper = mountList(true)
    await flushPromises()

    mocks.listAggregatedGameServers.mockResolvedValueOnce({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            version: '1.21.1',
            versionInfo: createProto(VersionInfoSchema, {
              status: VersionStatus.CHECKED,
              installedVersion: '1.21.1',
            }),
          }),
        }),
      ],
    })
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    mocks.eventBus.emit('websocketConnected')
    await flushPromises()

    expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(2)
    expect(mocks.listGameServers).not.toHaveBeenCalled()
    expect((wrapper.vm as unknown as { displayRows: DisplayRow[] }).displayRows).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'server-local',
          version: '1.21.1',
          versionInfo: expect.objectContaining({
            status: VersionStatus.CHECKED,
            installedVersion: '1.21.1',
          }),
        }),
      ]),
    )
  })

  it('queues one follow-up reload when reconnect occurs during an active load', async () => {
    const initialAggregatedRequest = createDeferred<{ servers: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(initialAggregatedRequest.promise)
    mocks.listNodes.mockResolvedValue({
      nodes: [buildLocalNode()],
    })

    const wrapper = mountList(true)

    await vi.waitFor(() => {
      expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(1)
    })

    mocks.listAggregatedGameServers.mockResolvedValueOnce({
      servers: [buildLocalAggregatedServer('Reloaded Server')],
    })

    mocks.eventBus.emit('websocketConnected')
    mocks.eventBus.emit('websocketConnected')

    expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(1)

    initialAggregatedRequest.resolve({
      servers: [buildLocalAggregatedServer('Initial Server')],
    })

    await vi.waitFor(() => {
      expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(2)
    })
    await flushPromises()

    expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(2)
    expect(
      (wrapper.vm as unknown as { displayRows: Array<{ displayName: string }> }).displayRows,
    ).toEqual(expect.arrayContaining([expect.objectContaining({ displayName: 'Reloaded Server' })]))
  })

  it('keeps lifecycle actions stale and shows retry when reconnect refresh fails', async () => {
    mocks.listAggregatedGameServers.mockResolvedValueOnce({
      servers: [buildLocalAggregatedServer()],
    })
    const wrapper = mountList(true)
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      lifecycleStateAuthoritative: boolean
      serverListError: string
    }
    expect(viewModel.lifecycleStateAuthoritative).toBe(true)

    setWebsocketConnectionStatus('reconnecting')
    mocks.eventBus.emit('websocketDisconnected')
    expect(viewModel.lifecycleStateAuthoritative).toBe(false)

    mocks.listAggregatedGameServers.mockRejectedValueOnce(new Error('node snapshot timed out'))
    setWebsocketConnectionStatus('connected')
    mocks.eventBus.emit('websocketConnected')
    await flushPromises()

    expect(viewModel.lifecycleStateAuthoritative).toBe(false)
    expect(viewModel.serverListError).toContain('node snapshot timed out')
    expect(wrapper.text()).toContain('Start, stop, restart, update, and delete remain unavailable')
    expect(wrapper.text()).toContain('Retry')
  })

  it('prefers fresher reloaded data over stale buffered websocket state after reconnect', async () => {
    mocks.listAggregatedGameServers.mockResolvedValueOnce({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            version: '1.20.4',
            versionInfo: createProto(VersionInfoSchema, {
              status: VersionStatus.CHECKING,
            }),
          }),
        }),
      ],
    })
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    const wrapper = mountList(true)
    await flushPromises()

    mocks.eventBus.emit(
      'gameServerVersion',
      'server-local',
      '1.20.4',
      createProto(VersionInfoSchema, {
        status: VersionStatus.CHECKING,
      }),
    )

    mocks.listAggregatedGameServers.mockResolvedValueOnce({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({
            version: '1.21.1',
            versionInfo: createProto(VersionInfoSchema, {
              status: VersionStatus.CHECKED,
              installedVersion: '1.21.1',
            }),
          }),
        }),
      ],
    })
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode()],
    })

    mocks.eventBus.emit('websocketConnected')
    await flushPromises()

    expect((wrapper.vm as unknown as { displayRows: DisplayRow[] }).displayRows).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          id: 'server-local',
          version: '1.21.1',
          versionInfo: expect.objectContaining({
            status: VersionStatus.CHECKED,
            installedVersion: '1.21.1',
          }),
        }),
      ]),
    )
  })

  it('ignores stale overlapping game server loads', async () => {
    const firstAggregated = createDeferred<{ servers: unknown[] }>()
    const firstNodes = createDeferred<{ nodes: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(firstAggregated.promise)
    mocks.listNodes.mockReturnValueOnce(firstNodes.promise)

    const wrapper = mountList(true)

    const secondAggregated = createDeferred<{ servers: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(secondAggregated.promise)
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [buildLocalNode({ name: 'Current Node' })],
    })

    const currentLoad = (
      wrapper.vm as unknown as { getGameServers: () => Promise<void> }
    ).getGameServers()

    secondAggregated.resolve({
      servers: [
        createProto(AggregatedGameServerSchema, {
          isLocal: true,
          localServer: buildLocalServer({ id: 'server-current', name: 'Current Local' }),
        }),
        buildRemoteAggregatedServer('Current Remote'),
      ],
    })
    await currentLoad

    await vi.waitFor(() => {
      expect(
        (wrapper.vm as unknown as { displayRows: Array<{ displayName: string }> }).displayRows,
      ).toEqual(expect.arrayContaining([expect.objectContaining({ displayName: 'Current Local' })]))
    })

    firstAggregated.resolve({
      servers: [
        buildLocalAggregatedServer('Stale Local'),
        buildRemoteAggregatedServer('Stale Remote'),
      ],
    })
    firstNodes.resolve({
      nodes: [buildLocalNode({ name: 'Stale Node' })],
    })
    await flushPromises()

    expect(
      (wrapper.vm as unknown as { displayRows: Array<{ displayName: string }> }).displayRows,
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({ displayName: 'Current Local' }),
        expect.objectContaining({ displayName: 'Current Remote' }),
      ]),
    )
    expect(
      (wrapper.vm as unknown as { displayRows: Array<{ displayName: string }> }).displayRows,
    ).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({ displayName: 'Stale Local' }),
        expect.objectContaining({ displayName: 'Stale Remote' }),
      ]),
    )
  })
})
