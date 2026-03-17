import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { NodeSchema } from '@/proto/shared_pb'
import NodeList from './NodeList.vue'

const mocks = vi.hoisted(() => ({
  listNodes: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>(
    '@/utils/shared',
  )
  return {
    ...actual,
    GetXylonaClient: () => ({
      listNodes: mocks.listNodes,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    Notify: {
      create: vi.fn(),
    },
  }
})

const globalStubs = {
  stubs: {
    'q-page': { template: '<div><slot /></div>' },
    'q-card': { template: '<div><slot /></div>' },
    'q-card-section': { template: '<div><slot /></div>' },
    'q-btn': true,
    'q-input': true,
    'q-icon': true,
    'q-td': { template: '<div><slot /></div>' },
    'q-tooltip': true,
    'q-badge': true,
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

describe('NodeList', () => {
  beforeEach(() => {
    mocks.listNodes.mockReset()
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
    expect(
      (wrapper.vm as unknown as { rows: unknown[] }).rows.length,
    ).toBe(2)
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
      expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe(
        'true',
      )
    })

    resolveRequest({ nodes: [] })
    await flushPromises()

    expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('false')
  })
})
