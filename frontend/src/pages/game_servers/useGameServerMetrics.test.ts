import { create, type MessageInitShape } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, ref, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  GameServerMetricsCollectionStatus,
  GameServerMetricsHistoryPointSchema,
  GameServerSchema,
  NodeResourceSnapshotSchema,
} from '@/proto/shared_pb'
import { AllNodeMetricsSchema } from '@/proto/websocket_pb'
import { XylonaEventBus } from '@/utils/shared'
import { useGameServerMetrics } from './useGameServerMetrics'

const mocks = vi.hoisted(() => ({
  client: {
    getGameServer: vi.fn(),
    getGameServerMetricsHistory: vi.fn(),
  },
  websocketClient: {
    isOpen: vi.fn(() => false),
    send: vi.fn(),
  },
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')

  return {
    ...actual,
    GetOrCreateXylonaWebsocketClient: () => mocks.websocketClient,
    GetXylonaClient: () => mocks.client,
  }
})

function historyResponse(serverId: string, cpuPercent: number) {
  const timestamp = create(TimestampSchema, { seconds: 1000n })
  return {
    points: [
      create(GameServerMetricsHistoryPointSchema, {
        timestamp,
        nodeId: `node-${serverId}`,
        cpuPercent,
        cpuValid: true,
        cpuValidSampleCount: 1,
        collectionStatus: GameServerMetricsCollectionStatus.AVAILABLE,
        sampleCount: 1,
        availableSampleCount: 1,
        nodeMemoryUsedBytes: 4096n,
        nodeMemoryTotalBytes: 8192n,
      }),
    ],
    lifecycleEvents: [
      {
        id: `event-${serverId}`,
        gameServerId: serverId,
        nodeId: `node-${serverId}`,
        executionId: '',
        transitionSequence: 1n,
        previousStatus: 'offline',
        status: 'online',
        intentionalStop: false,
        observedAt: timestamp,
      },
    ],
    operationEvents: [],
    resolution: 1,
    hasMixedResolution: false,
    sampleIntervalSeconds: 15,
  }
}

function gameServerResponse(serverId: string) {
  return {
    gameServer: create(GameServerSchema, {
      id: serverId,
      nodeId: `node-${serverId}`,
      name: `Server ${serverId}`,
    }),
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

function mountHarness(gameServerId: Ref<string>) {
  let metrics!: ReturnType<typeof useGameServerMetrics>
  const Harness = defineComponent({
    setup() {
      metrics = useGameServerMetrics({ gameServerId })
      return () => null
    },
  })

  const wrapper = mount(Harness)
  mountedWrappers.add(wrapper)
  return { metrics, wrapper }
}

const mountedWrappers = new Set<VueWrapper>()

beforeEach(() => {
  mocks.client.getGameServer.mockReset()
  mocks.client.getGameServer.mockImplementation(({ id }: { id: string }) =>
    Promise.resolve(gameServerResponse(id)),
  )
  mocks.client.getGameServerMetricsHistory.mockReset()
  mocks.websocketClient.isOpen.mockReset()
  mocks.websocketClient.isOpen.mockReturnValue(false)
  mocks.websocketClient.send.mockReset()
})

afterEach(() => {
  for (const wrapper of mountedWrappers) wrapper.unmount()
  mountedWrappers.clear()
  vi.restoreAllMocks()
})

describe('useGameServerMetrics node memory', () => {
  function emitNodeMemory(
    nodeId: string,
    snapshot: MessageInitShape<typeof NodeResourceSnapshotSchema>,
  ) {
    XylonaEventBus.emit(
      'nodeMetrics',
      create(AllNodeMetricsSchema, {
        nodes: { [nodeId]: create(NodeResourceSnapshotSchema, snapshot) },
      }),
    )
  }

  it('uses history until the node reports, then never shows a failed read as 0 B of 0 B', async () => {
    mocks.client.getGameServerMetricsHistory.mockResolvedValue(historyResponse('server-a', 25))
    const { metrics } = mountHarness(ref('server-a'))
    await flushPromises()

    expect(metrics.currentCapacity.value).toMatchObject({
      nodeUsedBytes: 4096,
      nodeTotalBytes: 8192,
    })

    emitNodeMemory('node-server-a', { memoryUnavailable: true })
    expect(metrics.currentCapacity.value).toMatchObject({
      nodeUsedBytes: null,
      nodeTotalBytes: null,
      nodeAvailableBytes: null,
    })

    emitNodeMemory('node-server-a', { memoryUsedBytes: 1024n, memoryTotalBytes: 2048n })
    expect(metrics.currentCapacity.value).toMatchObject({
      nodeUsedBytes: 1024,
      nodeTotalBytes: 2048,
      nodeAvailableBytes: 1024,
    })
  })
})

describe('useGameServerMetrics route changes', () => {
  it('clears server A state synchronously before loading server B', async () => {
    const serverBResponse = deferred<ReturnType<typeof historyResponse>>()
    mocks.client.getGameServerMetricsHistory.mockImplementation(
      ({ gameServerId }: { gameServerId: string }) =>
        gameServerId === 'server-a'
          ? Promise.resolve(historyResponse('server-a', 25))
          : serverBResponse.promise,
    )
    const gameServerId = ref('server-a')
    const { metrics } = mountHarness(gameServerId)
    await flushPromises()

    expect(metrics.samples.value[0]?.nodeId).toBe('node-server-a')
    expect(metrics.timeline.value[0]?.id).toBe('event-server-a')
    expect(metrics.gameServer.value.id).toBe('server-a')

    gameServerId.value = 'server-b'

    expect(metrics.samples.value).toEqual([])
    expect(metrics.timeline.value).toEqual([])
    expect(metrics.gameServer.value.id).toBe('')
    expect(metrics.latestSample.value).toBeNull()
    expect(metrics.loading.value).toBe(true)

    serverBResponse.resolve(historyResponse('server-b', 50))
    await flushPromises()

    expect(metrics.samples.value[0]?.nodeId).toBe('node-server-b')
    expect(metrics.timeline.value[0]?.id).toBe('event-server-b')
    expect(metrics.gameServer.value.id).toBe('server-b')
  })

  it('never renders server A telemetry when the server B fetch fails', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    mocks.client.getGameServerMetricsHistory.mockImplementation(
      ({ gameServerId }: { gameServerId: string }) =>
        gameServerId === 'server-a'
          ? Promise.resolve(historyResponse('server-a', 25))
          : Promise.reject(new Error('server B unavailable')),
    )
    const gameServerId = ref('server-a')
    const { metrics } = mountHarness(gameServerId)
    await flushPromises()
    expect(metrics.samples.value).toHaveLength(1)

    gameServerId.value = 'server-b'
    await flushPromises()

    expect(metrics.samples.value).toEqual([])
    expect(metrics.timeline.value).toEqual([])
    expect(metrics.gameServer.value.id).toBe('')
    expect(metrics.error.value).not.toBe('')
  })

  it('ignores a late server A response after switching to server B', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const serverAResponse = deferred<ReturnType<typeof historyResponse>>()
    mocks.client.getGameServerMetricsHistory.mockImplementation(
      ({ gameServerId }: { gameServerId: string }) =>
        gameServerId === 'server-a'
          ? serverAResponse.promise
          : Promise.reject(new Error('server B unavailable')),
    )
    const gameServerId = ref('server-a')
    const { metrics } = mountHarness(gameServerId)

    gameServerId.value = 'server-b'
    await flushPromises()
    serverAResponse.resolve(historyResponse('server-a', 25))
    await flushPromises()

    expect(metrics.samples.value).toEqual([])
    expect(metrics.timeline.value).toEqual([])
    expect(metrics.gameServer.value.id).toBe('')
    expect(metrics.error.value).not.toBe('')
  })
})
