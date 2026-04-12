import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { NodeSchema } from '@/proto/shared_pb'
import NodeList from './NodeList.vue'

const mocks = vi.hoisted(() => ({
  listNodes: vi.fn(),
  getDashboardOverview: vi.fn().mockResolvedValue({ nodes: [] }),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      listNodes: mocks.listNodes,
      getDashboardOverview: mocks.getDashboardOverview,
    }),
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
    mocks.listNodes.mockReset()
    mocks.getDashboardOverview.mockReset()
    mocks.getDashboardOverview.mockResolvedValue({ nodes: [] })
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
})
