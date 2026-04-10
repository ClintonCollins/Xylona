import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UserSchema } from '@/proto/xylona_pb'
import UserList from './UserList.vue'

const mocks = vi.hoisted(() => ({
  listUsers: vi.fn(),
  notifyCreate: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      listUsers: mocks.listUsers,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notifyCreate,
      screen: { lt: { md: false } },
    }),
    Notify: {
      create: mocks.notifyCreate,
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
    'router-link': { template: '<a><slot /></a>' },
    UserDeleteDialog: true,
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

describe('UserList', () => {
  beforeEach(() => {
    mocks.listUsers.mockReset()
    mocks.notifyCreate.mockReset()
  })

  it('renders table with users from API', async () => {
    const users = [
      create(UserSchema, {
        id: 'user-1',
        userName: 'admin',
        email: 'admin@example.com',
        firstName: 'Admin',
        lastName: 'User',
      }),
      create(UserSchema, {
        id: 'user-2',
        userName: 'operator',
        email: 'op@example.com',
        firstName: 'Op',
        lastName: 'Erator',
      }),
    ]

    mocks.listUsers.mockResolvedValueOnce({ users })

    const wrapper = mount(UserList, { global: globalStubs })

    await flushPromises()

    expect(mocks.listUsers).toHaveBeenCalledTimes(1)
    expect((wrapper.vm as unknown as { rows: unknown[] }).rows.length).toBe(2)
    expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('2')
  })

  it('shows loading state while fetching', async () => {
    let resolveRequest!: (value: unknown) => void
    mocks.listUsers.mockReturnValueOnce(
      new Promise((resolve) => {
        resolveRequest = resolve
      }),
    )

    const wrapper = mount(UserList, { global: globalStubs })

    await vi.waitFor(() => {
      expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('true')
    })

    resolveRequest({ users: [] })
    await flushPromises()

    expect(wrapper.find('[data-test="q-table-loading"]').text()).toBe('false')
  })

  it('shows error notification on API failure', async () => {
    const error = new ConnectError('failed to list users')
    mocks.listUsers.mockRejectedValueOnce(error)

    const wrapper = mount(UserList, { global: globalStubs })

    await flushPromises()

    expect(mocks.notifyCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-error',
        position: 'top',
      }),
    )
    expect((wrapper.vm as unknown as { rows: unknown[] }).rows.length).toBe(0)
  })
})
