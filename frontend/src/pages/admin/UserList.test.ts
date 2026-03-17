import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { describe, expect, it, vi } from 'vitest'
import { UserSchema } from '@/proto/xylona_pb'
import UserList from './UserList.vue'

const mocks = vi.hoisted(() => ({
  listUsers: vi.fn(),
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

describe('UserList', () => {
  it('loads users on mount', async () => {
    const users = [
      create(UserSchema, {
        id: 'user-1',
        userName: 'admin',
        email: 'admin@example.com',
        firstName: 'Admin',
        lastName: 'User',
      }),
    ]

    mocks.listUsers.mockResolvedValueOnce({ users })

    const wrapper = mount(UserList, {
      global: {
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
              rows: {
                type: Array,
                default: () => [],
              },
            },
            template: '<div data-test="q-table-row-count">{{ rows.length }}</div>',
          }),
        },
      },
    })

    await flushPromises()

    expect(mocks.listUsers).toHaveBeenCalledTimes(1)
    expect((wrapper.vm as unknown as { rows: unknown[] }).rows.length).toBe(1)
    expect(wrapper.find('[data-test="q-table-row-count"]').text()).toBe('1')
  })
})
