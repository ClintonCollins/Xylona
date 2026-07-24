import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { NodeResourceSnapshotSchema, NodeSchema } from '@/proto/shared_pb'
import { AllNodeMetricsSchema } from '@/proto/websocket_pb'
import { DashboardNodeSummarySchema } from '@/proto/xylona_pb'
import { setWebsocketConnectionStatus } from '@/utils/websocket-connection'
import NodeList from './NodeList.vue'

const mocks = vi.hoisted(() => {
  const listeners = new Map<string, Set<(...args: unknown[]) => void>>()

  return {
    listNodes: vi.fn(),
    getDashboardOverview: vi.fn().mockResolvedValue({ nodes: [] }),
    getOrCreateWebsocketClient: vi.fn(),
    eventBus: {
      on: vi.fn((eventName: string, handler: (...args: unknown[]) => void) => {
        const handlers = listeners.get(eventName) ?? new Set<(...args: unknown[]) => void>()
        handlers.add(handler)
        listeners.set(eventName, handlers)
      }),
      off: vi.fn((eventName: string, handler: (...args: unknown[]) => void) => {
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
    },
  }
})

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      listNodes: mocks.listNodes,
      getDashboardOverview: mocks.getDashboardOverview,
    }),
    GetOrCreateXylonaWebsocketClient: mocks.getOrCreateWebsocketClient,
    XylonaEventBus: mocks.eventBus,
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
      screen: { lt: { md: false } },
    }),
    Notify: {
      create: vi.fn(),
    },
  }
})

vi.mock('@vueuse/core', async () => {
  const actual = await vi.importActual<typeof import('@vueuse/core')>('@vueuse/core')
  return {
    ...actual,
    useStorage: <T>(_: string, initialValue: T) => ref(initialValue),
  }
})

const globalStubs = {
  stubs: {
    'q-page': { template: '<div><slot /></div>' },
    'q-card': { template: '<div><slot /></div>' },
    'q-card-section': { template: '<div><slot /></div>' },
    'q-btn': true,
    'q-input': { template: '<div><slot /></div>' },
    'q-icon': true,
    'q-td': { template: '<div><slot /></div>' },
    'q-tooltip': true,
    'q-badge': true,
    'q-spinner': true,
    'q-skeleton': true,
    'q-dialog': true,
    'q-card-actions': true,
    'router-link': { template: '<a><slot /></a>' },
    'q-table': defineComponent({
      name: 'QTableStub',
      props: {
        rows: { type: Array, default: () => [] },
        loading: { type: Boolean, default: false },
      },
      template:
        '<div data-test="q-table-row-count">{{ rows.length }}</div>' +
        '<div data-test="q-table-loading">{{ loading }}</div>',
    }),
  },
}

function createDeferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((res, rej) => {
    resolve = res
    reject = rej
  })

  return { promise, resolve, reject }
}

describe('NodeList', () => {
  beforeEach(() => {
    setWebsocketConnectionStatus('connected')
    mocks.listNodes.mockReset()
    mocks.getDashboardOverview.mockReset()
    mocks.getDashboardOverview.mockResolvedValue({ nodes: [] })
    mocks.getOrCreateWebsocketClient.mockReset()
    mocks.eventBus.reset()
  })

  it('renders table with nodes from API', async () => {
    const nodes = [
      create(NodeSchema, {
        id: 'node-1',
        name: 'Local Node',
        local: true,
      }),
      create(NodeSchema, {
        id: 'node-2',
        name: 'Remote Node',
        local: false,
        baseUrl: 'https://remote.example.com',
        healthStatus: 'healthy',
      }),
    ]

    mocks.listNodes.mockResolvedValueOnce({ nodes })

    const wrapper = mount(NodeList, { global: globalStubs })

    await flushPromises()

    expect(mocks.listNodes).toHaveBeenCalledTimes(1)
    expect((wrapper.vm as unknown as { rows: unknown[] }).rows.length).toBe(2)
    expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('2')
  })

  it('shows loading state while fetching', async () => {
    let resolveRequest!: (value: unknown) => void
    mocks.listNodes.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )

    const wrapper = mount(NodeList, { global: globalStubs })

    await vi.waitFor(() => {
      expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('true')
    })

    resolveRequest({ nodes: [] })
    await flushPromises()

    expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('false')
  })

  it('renders nodes before dashboard metrics finish loading', async () => {
    let resolveDashboard!: (value: unknown) => void
    mocks.listNodes.mockResolvedValueOnce({
      nodes: [
        create(NodeSchema, {
          id: 'node-1',
          name: 'Local Node',
          local: true,
        }),
      ],
    })
    mocks.getDashboardOverview.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveDashboard = resolve
      }),
    )

    const wrapper = mount(NodeList, { global: globalStubs })

    await vi.waitFor(() => {
      expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('1')
    })
    expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('false')
    expect(
      (
        wrapper.vm as unknown as { shouldShowMetricSkeleton: (nodeId: string) => boolean }
      ).shouldShowMetricSkeleton('node-1'),
    ).toBe(true)
    expect(
      (
        wrapper.vm as unknown as { shouldShowVersionSkeleton: (nodeId: string) => boolean }
      ).shouldShowVersionSkeleton('node-1'),
    ).toBe(true)

    resolveDashboard({ nodes: [] })
    await flushPromises()

    expect(
      (
        wrapper.vm as unknown as { shouldShowMetricSkeleton: (nodeId: string) => boolean }
      ).shouldShowMetricSkeleton('node-1'),
    ).toBe(false)
    expect(
      (
        wrapper.vm as unknown as { shouldShowVersionSkeleton: (nodeId: string) => boolean }
      ).shouldShowVersionSkeleton('node-1'),
    ).toBe(false)
  })

  it('ignores stale overlapping node fetches', async () => {
    const firstNodes = createDeferred<{ nodes: unknown[] }>()
    const firstDashboard = createDeferred<{ nodes: unknown[] }>()
    mocks.listNodes.mockReturnValueOnce(firstNodes.promise)
    mocks.getDashboardOverview.mockReturnValueOnce(firstDashboard.promise)

    const wrapper = mount(NodeList, { global: globalStubs })

    mocks.listNodes.mockResolvedValueOnce({
      nodes: [
        create(NodeSchema, {
          id: 'node-current',
          name: 'Current Node',
          local: true,
        }),
      ],
    })
    mocks.getDashboardOverview.mockResolvedValueOnce({
      nodes: [
        {
          node: { id: 'node-current' },
          systemInfo: { xylonaVersion: '2.0.0' },
        },
      ],
    })

    await (wrapper.vm as unknown as { fetchAll: () => Promise<void> }).fetchAll()
    await flushPromises()

    expect((wrapper.vm as unknown as { rows: Array<{ name: string }> }).rows).toEqual(
      expect.arrayContaining([expect.objectContaining({ name: 'Current Node' })]),
    )
    expect(
      (
        wrapper.vm as unknown as { getNodeVersion: (nodeId: string) => string | undefined }
      ).getNodeVersion('node-current'),
    ).toBe('2.0.0')

    firstNodes.resolve({
      nodes: [
        create(NodeSchema, {
          id: 'node-stale',
          name: 'Stale Node',
          local: true,
        }),
      ],
    })
    firstDashboard.resolve({
      nodes: [
        {
          node: { id: 'node-stale' },
          systemInfo: { xylonaVersion: '1.0.0' },
        },
      ],
    })
    await flushPromises()

    expect((wrapper.vm as unknown as { rows: Array<{ name: string }> }).rows).toEqual(
      expect.arrayContaining([expect.objectContaining({ name: 'Current Node' })]),
    )
    expect((wrapper.vm as unknown as { rows: Array<{ name: string }> }).rows).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ name: 'Stale Node' })]),
    )
    expect(
      (
        wrapper.vm as unknown as { getNodeVersion: (nodeId: string) => string | undefined }
      ).getNodeVersion('node-current'),
    ).toBe('2.0.0')
    expect(
      (
        wrapper.vm as unknown as { getNodeVersion: (nodeId: string) => string | undefined }
      ).getNodeVersion('node-stale'),
    ).toBeUndefined()
  })

  it('invalidates live snapshots on disconnect and reloads them once after reconnect', async () => {
    const node = create(NodeSchema, {
      id: 'node-1',
      name: 'Local Node',
      local: true,
    })
    const initialSnapshot = create(NodeResourceSnapshotSchema, {
      cpuPercent: 10,
      gameServerCount: 2,
    })
    const reloadedSnapshot = create(NodeResourceSnapshotSchema, {
      cpuPercent: 55,
      gameServerCount: 3,
    })

    mocks.listNodes.mockResolvedValue({ nodes: [node] })
    mocks.getDashboardOverview
      .mockResolvedValueOnce({
        nodes: [
          create(DashboardNodeSummarySchema, {
            node,
            snapshot: initialSnapshot,
          }),
        ],
      })
      .mockResolvedValueOnce({
        nodes: [
          create(DashboardNodeSummarySchema, {
            node,
            snapshot: reloadedSnapshot,
          }),
        ],
      })

    const wrapper = mount(NodeList, { global: globalStubs })
    await flushPromises()
    const viewModel = wrapper.vm as unknown as {
      dashboardSnapshotsFresh: boolean
      getSnapshot: (nodeId: string) => { cpuPercent: number } | undefined
      liveSnapshots: Map<string, unknown>
    }

    expect(viewModel.getSnapshot('node-1')?.cpuPercent).toBe(10)

    mocks.eventBus.emit(
      'nodeMetrics',
      create(AllNodeMetricsSchema, {
        nodes: {
          'node-1': create(NodeResourceSnapshotSchema, {
            cpuPercent: 80,
            gameServerCount: 4,
          }),
        },
      }),
    )

    expect(viewModel.getSnapshot('node-1')?.cpuPercent).toBe(80)

    setWebsocketConnectionStatus('reconnecting')
    mocks.eventBus.emit('websocketDisconnected')

    expect(viewModel.dashboardSnapshotsFresh).toBe(false)
    expect(viewModel.liveSnapshots.size).toBe(0)
    expect(viewModel.getSnapshot('node-1')).toBeUndefined()

    mocks.eventBus.emit(
      'nodeMetrics',
      create(AllNodeMetricsSchema, {
        nodes: {
          'node-1': create(NodeResourceSnapshotSchema, {
            cpuPercent: 99,
          }),
        },
      }),
    )
    expect(viewModel.getSnapshot('node-1')).toBeUndefined()

    setWebsocketConnectionStatus('connected')
    mocks.eventBus.emit('websocketConnected')
    await flushPromises()

    expect(mocks.listNodes).toHaveBeenCalledTimes(2)
    expect(mocks.getDashboardOverview).toHaveBeenCalledTimes(2)
    expect(viewModel.dashboardSnapshotsFresh).toBe(true)
    expect(viewModel.getSnapshot('node-1')?.cpuPercent).toBe(55)

    wrapper.unmount()
  })

  it('supersedes a pending pre-disconnect dashboard request on reconnect', async () => {
    const node = create(NodeSchema, {
      id: 'node-1',
      name: 'Local Node',
      local: true,
    })
    const staleDashboard = createDeferred<{ nodes: unknown[] }>()
    const recoveredSnapshot = create(NodeResourceSnapshotSchema, {
      cpuPercent: 42,
    })

    mocks.listNodes.mockResolvedValue({ nodes: [node] })
    mocks.getDashboardOverview.mockReturnValueOnce(staleDashboard.promise).mockResolvedValueOnce({
      nodes: [
        create(DashboardNodeSummarySchema, {
          node,
          snapshot: recoveredSnapshot,
        }),
      ],
    })

    const wrapper = mount(NodeList, { global: globalStubs })

    await vi.waitFor(() => {
      expect(mocks.listNodes).toHaveBeenCalledTimes(1)
      expect(mocks.getDashboardOverview).toHaveBeenCalledTimes(1)
    })

    setWebsocketConnectionStatus('reconnecting')
    mocks.eventBus.emit('websocketDisconnected')
    setWebsocketConnectionStatus('connected')
    mocks.eventBus.emit('websocketConnected')

    await vi.waitFor(() => {
      expect(mocks.listNodes).toHaveBeenCalledTimes(2)
      expect(mocks.getDashboardOverview).toHaveBeenCalledTimes(2)
    })

    const viewModel = wrapper.vm as unknown as {
      dashboardSnapshotsFresh: boolean
      getSnapshot: (nodeId: string) => { cpuPercent: number } | undefined
    }
    await vi.waitFor(() => {
      expect(viewModel.dashboardSnapshotsFresh).toBe(true)
      expect(viewModel.getSnapshot('node-1')?.cpuPercent).toBe(42)
    })

    staleDashboard.resolve({ nodes: [] })
    await flushPromises()

    expect(viewModel.getSnapshot('node-1')?.cpuPercent).toBe(42)
    wrapper.unmount()
  })

  it('maps node health status to badge labels and colors', async () => {
    mocks.listNodes.mockResolvedValueOnce({ nodes: [] })

    const wrapper = mount(NodeList, { global: globalStubs })
    await flushPromises()

    const healthBadgeFor = (
      wrapper.vm as unknown as {
        healthBadgeFor: (node: { healthStatus: string }) => { color: string; label: string }
      }
    ).healthBadgeFor

    expect(healthBadgeFor({ healthStatus: 'healthy' }).label).toBe('Healthy')
    expect(healthBadgeFor({ healthStatus: 'healthy' }).color).toBe('positive')
    expect(healthBadgeFor({ healthStatus: 'offline' }).label).toBe('Offline')
    expect(healthBadgeFor({ healthStatus: 'offline' }).color).toBe('negative')
    expect(healthBadgeFor({ healthStatus: 'disabled' }).label).toBe('Disabled')
    expect(healthBadgeFor({ healthStatus: 'disabled' }).color).toBe('warning')
  })
})
