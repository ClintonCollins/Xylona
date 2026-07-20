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

const mocks = vi.hoisted(() => ({
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
}))

const storageState = vi.hoisted(() => ({
  values: new Map<string, unknown>(),
}))

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
    useQuasar: () => {
      const dialogResult = {
        onOk(callback: () => void) {
          callback()
          return dialogResult
        },
        onDismiss() {
          return dialogResult
        },
      }
      return {
        dialog: vi.fn(() => dialogResult),
        notify: vi.fn(),
        screen: { lt: { md: false } },
      }
    },
  }
})

vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useStorage: <T>(key: string, initialValue: T) => {
      if (!storageState.values.has(key)) {
        storageState.values.set(key, ref(initialValue))
      }
      return storageState.values.get(key) as { value: T }
    },
  }
})

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
    storageState.values.clear()
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

  function seedStoredDisplayRows(rows: DisplayRow[]) {
    storageState.values.set('game-server-display-rows-cache', ref(rows))
  }

  function getStoredDisplayRows() {
    return storageState.values.get('game-server-display-rows-cache') as
      | { value: DisplayRow[] }
      | undefined
  }

  function mountList(superUser: boolean) {
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
          'q-td': { template: '<div><slot /></div>' },
          'q-tooltip': true,
          'q-spinner': true,
          'q-skeleton': true,
          'q-separator': { template: '<span />' },
          'q-table': defineComponent({
            name: 'QTableStub',
            props: {
              rows: { type: Array, default: () => [] },
              loading: { type: Boolean, default: false },
            },
            template:
              '<div data-test="q-table-row-count">{{ rows.length }}</div>' +
              '<div data-test="q-table-loading">{{ loading }}</div>' +
              '<slot />' +
              '<slot name="bottom-row" />' +
              '<slot name="no-data" />',
          }),
          'router-link': { template: '<a><slot /></a>' },
          'delete-game-server-dialog': true,
          StatusBadge: true,
          VersionBadge: true,
          'q-btn': QBtnStub,
        },
      },
    })
  }

  it('hides the create button for non-superusers', async () => {
    const wrapper = mountList(false)
    await flushPromises()

    expect(wrapper.text()).not.toContain('Create Game Server')
  })

  it('shows the create button for superusers', async () => {
    const wrapper = mountList(true)
    await flushPromises()

    expect(wrapper.text()).toContain('Create Game Server')
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

  it('shows cached rows before aggregated data finishes loading', async () => {
    seedStoredDisplayRows([
      {
        compositeId: 'local/server-cached',
        id: 'server-cached',
        isLocal: true,
        displayName: 'Cached Local Server',
        gameName: 'Minecraft',
        userName: 'owner',
        statusEnum: Status.OFFLINE,
        nodeName: 'Cached Local Node',
        isStale: false,
        sourceNodeId: '',
        version: '1.20.4',
      },
    ])
    const aggregatedRequest = createDeferred<{ servers: unknown[] }>()
    const nodesRequest = createDeferred<{ nodes: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(aggregatedRequest.promise)
    mocks.listNodes.mockReturnValueOnce(nodesRequest.promise)

    const wrapper = mountList(true)

    await vi.waitFor(() => {
      expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('1')
    })
    expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('true')
    expect(
      (wrapper.vm as unknown as { displayRows: Array<{ displayName: string }> }).displayRows,
    ).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          displayName: 'Cached Local Server',
          nodeName: 'Cached Local Node',
        }),
      ]),
    )

    nodesRequest.resolve({ nodes: [buildLocalNode()] })
    aggregatedRequest.resolve({ servers: [] })
    await flushPromises()
  })

  it('does not render cached local online rows before the first live fetch completes', async () => {
    seedStoredDisplayRows([
      {
        compositeId: 'local/server-cached-online',
        id: 'server-cached-online',
        isLocal: true,
        displayName: 'Cached Local Server',
        gameName: 'Minecraft',
        userName: 'owner',
        statusEnum: Status.ONLINE,
        nodeName: 'Local Node',
        isStale: false,
        sourceNodeId: '',
        version: '1.20.4',
      },
    ])

    const pendingRequest = new Promise<never>(() => {})
    mocks.listAggregatedGameServers.mockReturnValueOnce(pendingRequest)
    mocks.listNodes.mockReturnValueOnce(pendingRequest)

    const wrapper = mountList(true)

    await vi.waitFor(() => {
      expect(mocks.listAggregatedGameServers).toHaveBeenCalledTimes(1)
      expect(mocks.listGameServers).not.toHaveBeenCalled()
      expect(mocks.listNodes).toHaveBeenCalledTimes(1)
    })

    const displayRows = (wrapper.vm as unknown as { displayRows: DisplayRow[] }).displayRows

    expect(displayRows).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          compositeId: 'local/server-cached-online',
          isLocal: true,
          statusEnum: Status.ONLINE,
        }),
      ]),
    )
    expect(getStoredDisplayRows()?.value).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          compositeId: 'local/server-cached-online',
          isLocal: true,
          statusEnum: Status.ONLINE,
        }),
      ]),
    )
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

  it('rebuilds cached remote rows when nodes resolve after aggregated data', async () => {
    const aggregatedRequest = createDeferred<{ servers: unknown[] }>()
    const nodesRequest = createDeferred<{ nodes: unknown[] }>()
    mocks.listAggregatedGameServers.mockReturnValueOnce(aggregatedRequest.promise)
    mocks.listNodes.mockReturnValueOnce(nodesRequest.promise)

    const wrapper = mountList(true)

    aggregatedRequest.resolve({
      servers: [buildLocalAggregatedServer(), buildRemoteAggregatedServer()],
    })

    await vi.waitFor(() => {
      expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('2')
    })

    expect(getStoredDisplayRows()?.value).not.toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          compositeId: 'node-remote/server-remote',
          displayName: 'Remote Server',
          isLocal: false,
        }),
      ]),
    )

    nodesRequest.resolve({
      nodes: [buildLocalNode(), buildRemoteNode()],
    })
    await flushPromises()

    expect(getStoredDisplayRows()?.value).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          compositeId: 'node-remote/server-remote',
          displayName: 'Remote Server',
          isLocal: false,
          sourceNodeId: 'node-remote',
        }),
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
    expect(getStoredDisplayRows()?.value).toEqual(
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
    expect(getStoredDisplayRows()?.value).toEqual(
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
    expect(wrapper.text()).toContain('Lifecycle actions remain disabled')
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
