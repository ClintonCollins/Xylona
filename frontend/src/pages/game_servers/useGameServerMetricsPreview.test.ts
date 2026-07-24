import { create, fromJsonString } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { defineComponent, nextTick, ref } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { GameServerSchema, Status } from '@/proto/shared_pb'
import {
  AllServersMetricsSchema,
  GameServerMetricsSchema,
  RequestSchema,
  Request_Type,
} from '@/proto/websocket_pb'
import { XylonaEventBus } from '@/utils/shared'
import {
  formatMetricsRate,
  formatMetricsUptime,
  useGameServerMetricsPreview,
} from './useGameServerMetricsPreview'
import {
  setWebsocketConnectionStatus,
  type WebsocketConnectionStatus,
} from '@/utils/websocket-connection'

const mocks = vi.hoisted(() => {
  const websocketClient = {
    isOpen: vi.fn(),
    send: vi.fn(),
  }

  return {
    getOrCreateWebsocketClient: vi.fn(() => websocketClient),
    websocketClient,
  }
})

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')

  return {
    ...actual,
    GetOrCreateXylonaWebsocketClient: mocks.getOrCreateWebsocketClient,
  }
})

function makeGameServer(status: Status = Status.ONLINE) {
  return create(GameServerSchema, {
    id: 'server-1',
    status,
    maxMemoryMb: 4096n,
  })
}

interface MetricsOverrides {
  connectionCount?: number
  cpuCores?: number
  cpuPercent?: number
  diskUsageBytes?: bigint
  ioReadRate?: number
  ioWriteRate?: number
  memoryBytes?: bigint
  memoryWorkingSetBytes?: bigint
  memoryPercent?: number
  numberOfThreads?: number
  uptimeSeconds?: bigint
}

function makeAllMetrics(serverId: string, overrides: MetricsOverrides = {}) {
  return create(AllServersMetricsSchema, {
    servers: {
      [serverId]: create(GameServerMetricsSchema, {
        connectionCount: 12,
        cpuCores: 8,
        cpuPercent: 25,
        diskUsageBytes: 5368709120n,
        ioReadRate: 1048576,
        ioWriteRate: 524288,
        memoryBytes: 1073741824n,
        memoryWorkingSetBytes: 1073741824n,
        memoryPercent: 3.2,
        numberOfThreads: 8,
        uptimeSeconds: 3750n,
        ...overrides,
      }),
    },
  })
}

function mountHarness(status: Status = Status.ONLINE) {
  const Harness = defineComponent({
    setup() {
      const gameServer = ref(makeGameServer(status))
      const gameServerId = ref('server-1')
      return {
        gameServer,
        ...useGameServerMetricsPreview({
          gameServer,
          gameServerId,
        }),
      }
    },
    template: '<div />',
  })

  const wrapper = mount(Harness)
  mountedWrappers.add(wrapper)
  return wrapper
}

const cleanupCallbacks = new Set<() => void>()
const mountedWrappers = new Set<{ unmount: () => void }>()

function trackCleanup(cleanup: () => void) {
  cleanupCallbacks.add(cleanup)
}

function unmountHarness(wrapper: { unmount: () => void }) {
  if (!mountedWrappers.delete(wrapper)) {
    return
  }

  wrapper.unmount()
}

function expectLatestRequest(type: Request_Type) {
  const payload = mocks.websocketClient.send.mock.calls.at(-1)?.[0]
  expect(payload).toEqual(expect.any(String))

  const request = fromJsonString(RequestSchema, payload as string)
  expect(request.type).toBe(type)
  expect(request.gameServerId).toBe('server-1')
}

describe('useGameServerMetricsPreview', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    setWebsocketConnectionStatus('connected')
    mocks.websocketClient.isOpen.mockReset()
    mocks.websocketClient.isOpen.mockReturnValue(true)
    mocks.websocketClient.send.mockReset()
    mocks.getOrCreateWebsocketClient.mockReset()
    mocks.getOrCreateWebsocketClient.mockReturnValue(mocks.websocketClient)
  })

  afterEach(() => {
    for (const wrapper of [...mountedWrappers]) {
      wrapper.unmount()
    }
    mountedWrappers.clear()

    for (const cleanup of [...cleanupCallbacks].reverse()) {
      cleanup()
    }
    cleanupCallbacks.clear()

    vi.useRealTimers()
  })

  it('subscribes when the metrics preview lifecycle starts and the websocket is open', () => {
    const wrapper = mountHarness()

    wrapper.vm.startMetricsPreviewLifecycle()

    expect(mocks.getOrCreateWebsocketClient).toHaveBeenCalledTimes(1)
    expect(mocks.websocketClient.send).toHaveBeenCalledTimes(1)
    expectLatestRequest(Request_Type.SubscribeServerMetrics)
  })

  it('resubscribes on websocket reconnect while the lifecycle is active', () => {
    const wrapper = mountHarness()

    wrapper.vm.startMetricsPreviewLifecycle()
    mocks.websocketClient.send.mockClear()

    XylonaEventBus.emit('websocketConnected')

    expect(mocks.websocketClient.send).toHaveBeenCalledTimes(1)
    expectLatestRequest(Request_Type.SubscribeServerMetrics)
  })

  it('stops the lifecycle on unmount and unsubscribes when the websocket is open', () => {
    const wrapper = mountHarness()

    wrapper.vm.startMetricsPreviewLifecycle()
    mocks.websocketClient.send.mockClear()

    unmountHarness(wrapper)

    expect(mocks.websocketClient.send).toHaveBeenCalledTimes(1)
    expectLatestRequest(Request_Type.UnsubscribeServerMetrics)

    mocks.websocketClient.send.mockClear()

    XylonaEventBus.emit('websocketConnected')

    expect(mocks.websocketClient.send).not.toHaveBeenCalled()
  })

  it('ignores late lifecycle starts after unmount so no subscription or reconnect listener leaks', () => {
    const wrapper = mountHarness()
    const startMetricsPreviewLifecycle = wrapper.vm.startMetricsPreviewLifecycle

    unmountHarness(wrapper)

    startMetricsPreviewLifecycle()
    XylonaEventBus.emit('websocketConnected')

    expect(mocks.websocketClient.send).not.toHaveBeenCalled()
  })

  it('removes only harness-created listeners during cleanup and leaves shared listeners intact', async () => {
    const wrapper = mountHarness()
    const sharedMetricsListener = vi.fn()
    const initialMetrics = makeAllMetrics('server-1', {
      cpuPercent: 33,
      uptimeSeconds: 60n,
    })
    const nextMetrics = makeAllMetrics('server-1', {
      cpuPercent: 99,
      uptimeSeconds: 600n,
    })

    XylonaEventBus.on('gameServerMetrics', sharedMetricsListener)
    trackCleanup(() => XylonaEventBus.off('gameServerMetrics', sharedMetricsListener))

    XylonaEventBus.emit('gameServerMetrics', initialMetrics)
    await nextTick()

    expect(wrapper.vm.metricsCpu).toBe(33)

    unmountHarness(wrapper)

    XylonaEventBus.emit('gameServerMetrics', nextMetrics)
    await nextTick()

    expect(sharedMetricsListener).toHaveBeenCalledTimes(2)
    expect(sharedMetricsListener).toHaveBeenNthCalledWith(1, initialMetrics)
    expect(sharedMetricsListener).toHaveBeenNthCalledWith(2, nextMetrics)
    expect(wrapper.vm.metricsCpu).toBe(33)
  })

  it('applies metrics events for the active server', async () => {
    const wrapper = mountHarness()

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', {
        cpuCores: 16,
        cpuPercent: 42.5,
        ioReadRate: 2097152,
        uptimeSeconds: 3750n,
      }),
    )
    await nextTick()

    expect(wrapper.vm.metricsCpu).toBe(42.5)
    expect(wrapper.vm.metricsCpuCores).toBe(16)
    expect(wrapper.vm.formattedUptime).toBe('1h 2m 30s')
    expect(wrapper.vm.formatRate(wrapper.vm.metricsIoReadRate)).toBe('2.0 MB/s')
  })

  it('uses working set memory as the displayed memory value when private committed memory is larger', async () => {
    const wrapper = mountHarness()

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', {
        memoryBytes: 40n * 1024n * 1024n * 1024n,
        memoryWorkingSetBytes: 1024n * 1024n * 1024n,
      }),
    )
    await nextTick()

    expect(wrapper.vm.metricsMemory).toBe(1024 * 1024 * 1024)
    expect(wrapper.vm.metricsMemoryRatio).toBe(0.25)
  })

  it('falls back to private committed memory when working set memory is unavailable', async () => {
    const wrapper = mountHarness()

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', {
        memoryBytes: 2n * 1024n * 1024n * 1024n,
        memoryWorkingSetBytes: 0n,
      }),
    )
    await nextTick()

    expect(wrapper.vm.metricsMemory).toBe(2 * 1024 * 1024 * 1024)
    expect(wrapper.vm.metricsMemoryRatio).toBe(0.5)
  })

  it('ignores metrics events for other servers', async () => {
    const wrapper = mountHarness()

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-99', {
        cpuPercent: 99,
      }),
    )
    await nextTick()

    expect(wrapper.vm.metricsCpu).toBe(0)
    expect(wrapper.vm.formattedUptime).toBe('0s')
  })

  it('increments uptime locally while the server is online', async () => {
    const wrapper = mountHarness()

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', {
        uptimeSeconds: 60n,
      }),
    )
    await nextTick()

    vi.advanceTimersByTime(60000)
    await nextTick()

    expect(wrapper.vm.metricsUptimeSeconds).toBe(120)
    expect(wrapper.vm.formattedUptime).toBe('2m')
  })

  it.each<WebsocketConnectionStatus>(['reconnecting', 'disconnected'])(
    'freezes synthetic uptime while the live connection is %s',
    async (connectionStatus) => {
      const wrapper = mountHarness()

      XylonaEventBus.emit(
        'gameServerMetrics',
        makeAllMetrics('server-1', {
          uptimeSeconds: 60n,
        }),
      )
      await nextTick()

      setWebsocketConnectionStatus(connectionStatus)
      vi.advanceTimersByTime(60000)
      await nextTick()

      expect(wrapper.vm.metricsUptimeSeconds).toBe(60)
      expect(wrapper.vm.formattedUptime).toBe('1m')

      setWebsocketConnectionStatus('connected')
      vi.advanceTimersByTime(1000)
      await nextTick()

      expect(wrapper.vm.metricsUptimeSeconds).toBe(61)
    },
  )

  it('stops reacting to metrics events and timer ticks after unmount', async () => {
    const wrapper = mountHarness()

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', {
        cpuPercent: 33,
        uptimeSeconds: 60n,
      }),
    )
    await nextTick()

    expect(wrapper.vm.metricsCpu).toBe(33)
    expect(wrapper.vm.metricsUptimeSeconds).toBe(60)

    unmountHarness(wrapper)

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', {
        cpuPercent: 99,
        uptimeSeconds: 600n,
      }),
    )
    vi.advanceTimersByTime(60000)
    await nextTick()

    expect(wrapper.vm.metricsCpu).toBe(33)
    expect(wrapper.vm.metricsUptimeSeconds).toBe(60)
  })
})

describe('metrics preview formatters', () => {
  it('formats throughput values with stable units', () => {
    expect(formatMetricsRate(0)).toBe('0 B/s')
    expect(formatMetricsRate(1024)).toBe('1.0 KB/s')
    expect(formatMetricsRate(2097152)).toBe('2.0 MB/s')
    expect(formatMetricsRate(2 * 1024 * 1024 * 1024)).toBe('2.0 GB/s')
  })

  it('formats uptime values with compact segments', () => {
    expect(formatMetricsUptime(0)).toBe('0s')
    expect(formatMetricsUptime(59)).toBe('59s')
    expect(formatMetricsUptime(3750)).toBe('1h 2m 30s')
    expect(formatMetricsUptime(90061)).toBe('1d 1h 1m 1s')
  })
})
