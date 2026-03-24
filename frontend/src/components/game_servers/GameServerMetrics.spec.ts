import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { mount } from '@vue/test-utils'
import { create } from '@bufbuild/protobuf'
import { Quasar } from 'quasar'
import { GameServerSchema, Status } from '@/proto/shared_pb'
import {
  AllServersMetrics,
  AllServersMetricsSchema,
  GameServerMetricsSchema,
} from '@/proto/websocket_pb'
import { XylonaEventBus } from '@/utils/shared'
import GameServerMetrics from './GameServerMetrics.vue'

function makeGameServer(overrides: Record<string, unknown> = {}) {
  return create(GameServerSchema, {
    id: 'server-1',
    status: Status.ONLINE,
    maxMemoryMb: 4096n,
    cpuPercent: 0n,
    memoryBytes: 0n,
    numberOfThreads: 0n,
    diskUsageBytes: 0n,
    uptimeSeconds: 0n,
    memoryWorkingSetBytes: 0n,
    memoryPercent: 0,
    cpuCores: 8,
    ioReadRate: 0,
    ioWriteRate: 0,
    connectionCount: 0,
    ...overrides,
  })
}

interface MetricsOverrides {
  cpuPercent?: number
  memoryBytes?: bigint
  numberOfThreads?: number
  diskUsageBytes?: bigint
  uptimeSeconds?: bigint
  memoryWorkingSetBytes?: bigint
  memoryPercent?: number
  cpuCores?: number
  ioReadRate?: number
  ioWriteRate?: number
  connectionCount?: number
}

function makeAllMetrics(serverId: string, overrides: MetricsOverrides = {}): AllServersMetrics {
  return create(AllServersMetricsSchema, {
    servers: {
      [serverId]: create(GameServerMetricsSchema, {
        cpuPercent: 25.0,
        memoryBytes: 1073741824n,
        numberOfThreads: 8,
        diskUsageBytes: 5368709120n,
        uptimeSeconds: 3750n,
        memoryWorkingSetBytes: 2147483648n,
        memoryPercent: 3.2,
        cpuCores: 8,
        ioReadRate: 1048576,
        ioWriteRate: 524288,
        connectionCount: 12,
        ...overrides,
      }),
    },
  })
}

const globalConfig = {
  plugins: [Quasar],
}

describe('GameServerMetrics', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows offline state when server is OFFLINE', () => {
    const gameServer = makeGameServer({ status: Status.OFFLINE })
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })
    expect(wrapper.find('.text-subtitle2').exists()).toBe(true)
    expect(wrapper.find('.metrics-offline').exists()).toBe(true)
    expect(wrapper.text()).toContain('Server Offline')
  })

  it('renders resource usage section when server is ONLINE', () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })
    expect(wrapper.find('.text-subtitle2').text()).toBe('Resource Usage')
  })

  it('updates CPU display on gameServerMetrics event', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { cpuPercent: 42.5 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('42.5%')
  })

  it('shows CPU cores annotation', async () => {
    const gameServer = makeGameServer({ cpuCores: 16 })
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { cpuCores: 16 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('16 cores')
  })

  it('ignores metrics events for other servers', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-99', { cpuPercent: 99.0 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('0.0%')
  })

  it('formats memory bytes as human-readable', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', { memoryBytes: 1073741824n }),
    )
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('GB')
  })

  it('shows working set memory row', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit(
      'gameServerMetrics',
      makeAllMetrics('server-1', { memoryWorkingSetBytes: 2147483648n }),
    )
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Working Set')
    expect(wrapper.text()).toContain('GB')
  })

  it('shows memory percent row when non-zero', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { memoryPercent: 3.2 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('System RAM')
    expect(wrapper.text()).toContain('3.2%')
  })

  it('formats I/O read rate in MB/s', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    // 2097152 bytes/sec = 2.0 MB/s
    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { ioReadRate: 2097152 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('I/O Read')
    expect(wrapper.text()).toContain('MB/s')
  })

  it('formats I/O write rate in KB/s', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    // 512*1024 = 524288 bytes/sec = 512 KB/s
    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { ioWriteRate: 524288 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('I/O Write')
    expect(wrapper.text()).toContain('KB/s')
  })

  it('shows connection count', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { connectionCount: 32 }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('Connections')
    expect(wrapper.text()).toContain('32')
  })

  it('formats uptime 3750 seconds as 1h 2m 30s', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { uptimeSeconds: 3750n }))
    await wrapper.vm.$nextTick()

    const text = wrapper.text()
    expect(text).toContain('1h')
    expect(text).toContain('2m')
    expect(text).toContain('30s')
  })

  it('increments uptime counter via local timer', async () => {
    const gameServer = makeGameServer()
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })

    XylonaEventBus.emit('gameServerMetrics', makeAllMetrics('server-1', { uptimeSeconds: 60n }))
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('1m')

    vi.advanceTimersByTime(60000)
    await wrapper.vm.$nextTick()

    expect(wrapper.text()).toContain('2m')
  })

  it('does not show memory progress bar when maxMemoryMb is 0', () => {
    const gameServer = makeGameServer({ maxMemoryMb: 0n })
    const wrapper = mount(GameServerMetrics, {
      global: globalConfig,
      props: { gameServerId: 'server-1', gameServer },
    })
    const progressBars = wrapper.findAll('.q-linear-progress')
    expect(progressBars.length).toBe(1)
  })

  describe('formatRate helper', () => {
    it('shows 0 B/s for zero', async () => {
      const gameServer = makeGameServer()
      const wrapper = mount(GameServerMetrics, {
        global: globalConfig,
        props: { gameServerId: 'server-1', gameServer },
      })
      XylonaEventBus.emit(
        'gameServerMetrics',
        makeAllMetrics('server-1', { ioReadRate: 0, ioWriteRate: 0 }),
      )
      await wrapper.vm.$nextTick()
      // Both rates are 0, both should show "0 B/s"
      expect(wrapper.text().match(/0 B\/s/g)?.length).toBeGreaterThanOrEqual(2)
    })

    it('shows GB/s for very large rates', async () => {
      const gameServer = makeGameServer()
      const wrapper = mount(GameServerMetrics, {
        global: globalConfig,
        props: { gameServerId: 'server-1', gameServer },
      })
      XylonaEventBus.emit(
        'gameServerMetrics',
        makeAllMetrics('server-1', { ioReadRate: 2 * 1024 * 1024 * 1024 }),
      )
      await wrapper.vm.$nextTick()
      expect(wrapper.text()).toContain('GB/s')
    })
  })
})
