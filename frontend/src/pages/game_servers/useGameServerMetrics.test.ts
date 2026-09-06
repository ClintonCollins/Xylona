import { create } from '@bufbuild/protobuf'
import { TimestampSchema } from '@bufbuild/protobuf/wkt'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { defineComponent, ref, type Ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
  GameServerMetricsCollectionStatus,
  GameServerMetricsHistoryPointSchema,
  GameServerSchema,
} from '@/proto/shared_pb'
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

function mountHarness(gameServerId: Ref<string>, focusAt?: Ref<number | null>) {
  let metrics!: ReturnType<typeof useGameServerMetrics>
  const Harness = defineComponent({
    setup() {
      metrics = useGameServerMetrics({ gameServerId, focusAt })
      return () => null
    },
  })

  const wrapper = mount(Harness)
  mountedWrappers.add(wrapper)
  return { metrics, wrapper }
}

const mountedWrappers = new Set<VueWrapper>()

describe('useGameServerMetrics route changes', () => {
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

  it('loads a fixed window around a failure and resumes current metrics when cleared', async () => {
    mocks.client.getGameServerMetricsHistory.mockResolvedValue(historyResponse('server-a', 25))
    const focusAt = ref<number | null>(1_700_000_000_000)
    mountHarness(ref('server-a'), focusAt)
    await flushPromises()
    const focusedRequest = mocks.client.getGameServerMetricsHistory.mock.calls[0]?.[0]
    expect(Number(focusedRequest.until.seconds) * 1000).toBe(1_700_000_300_000)
    expect(Number(focusedRequest.since.seconds) * 1000).toBe(1_699_996_700_000)
    const now = Date.now()
    focusAt.value = null
    await flushPromises()
    const currentRequest = mocks.client.getGameServerMetricsHistory.mock.calls.at(-1)?.[0]
    expect(Number(currentRequest.until.seconds) * 1000).toBeGreaterThanOrEqual(now - 1000)
  })

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
