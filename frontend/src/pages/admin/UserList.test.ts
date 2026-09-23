import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UserSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
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
    setActivePinia(createPinia())
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

  it('sorts Created At by instant and labels roles the same on every layout', async () => {
    mocks.listUsers.mockResolvedValueOnce({ users: [] })
    const wrapper = mount(UserList, { global: globalStubs })
    await flushPromises()

    type Column = {
      name: string
      field: (row: unknown) => unknown
      format?: (value: unknown, row: unknown) => string
    }
    const columns = (wrapper.vm as unknown as { columns: Column[] }).columns
    const column = (name: string): Column => {
      const found = columns.find((candidate) => candidate.name === name)
      if (!found) throw new Error(`missing column ${name}`)
      return found
    }
    const createdAt = column('createdAt')
    const role = column('superUser')

    const april = create(UserSchema, { createdAt: { seconds: 1776686400n, nanos: 0 } })
    const august = create(UserSchema, { createdAt: { seconds: 1786060800n, nanos: 0 } })
    expect(createdAt.field(april)).toBeLessThan(createdAt.field(august) as number)
    expect(createdAt.field(create(UserSchema, {}))).toBe(0)
    expect(createdAt.format?.(createdAt.field(april), april)).toBe('Apr 20, 2026')

    expect(role.field(create(UserSchema, { superUser: true }))).toBe('Super user')
    expect(role.field(create(UserSchema, { superUser: false }))).toBe('User')
  })

  it('explains why the signed-in user cannot delete their own account', async () => {
    mocks.listUsers.mockResolvedValueOnce({ users: [] })
    useUserAuthStore().setUser(create(UserSchema, { id: 'user-me', userName: 'me' }))
    const wrapper = mount(UserList, { global: globalStubs })
    await flushPromises()

    const vm = wrapper.vm as unknown as {
      isSignedInUser: (user: unknown) => boolean
      deleteTooltip: (user: unknown) => string
    }
    const me = create(UserSchema, { id: 'user-me' })
    const other = create(UserSchema, { id: 'user-other' })
    expect(vm.isSignedInUser(me)).toBe(true)
    expect(vm.deleteTooltip(me)).toBe("You can't delete your own account")
    expect(vm.isSignedInUser(other)).toBe(false)
    expect(vm.deleteTooltip(other)).toBe('Delete user')
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
