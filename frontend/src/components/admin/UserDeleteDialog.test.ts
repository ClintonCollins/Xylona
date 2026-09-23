import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { GetUserDeletionImpactResponseSchema, UserSchema } from '@/proto/xylona_pb'
import UserDeleteDialog from './UserDeleteDialog.vue'

const mocks = vi.hoisted(() => ({
  notifySuccess: vi.fn(),
  deleteUser: vi.fn(),
  getUserDeletionImpact: vi.fn(),
}))

vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notifySuccess,
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    deleteUser: mocks.deleteUser,
    getUserDeletionImpact: mocks.getUserDeletionImpact,
  }),
}))

function mountDialog(userName: string) {
  return mount(UserDeleteDialog, {
    props: {
      user: create(UserSchema, { id: `id-${userName}`, userName }),
      showDialog: true,
    },
    global: {
      stubs: {
        'q-dialog': { template: '<div><slot /></div>' },
        'q-card': { template: '<div><slot /></div>' },
        'q-card-section': { template: '<div><slot /></div>' },
        'q-card-actions': { template: '<div><slot /></div>' },
        'q-banner': { template: '<div role="alert"><slot /></div>' },
        'q-spinner': true,
        'router-link': { props: ['to'], template: '<a :href="to"><slot /></a>' },
        'q-btn': {
          props: ['label'],
          emits: ['click'],
          template: '<button @click="$emit(\'click\')">{{ label }}</button>',
        },
      },
    },
  })
}

function deleteButton(wrapper: ReturnType<typeof mountDialog>) {
  return wrapper.findAll('button').find((button) => button.text() === 'Delete')
}

describe('UserDeleteDialog', () => {
  afterEach(() => {
    vi.clearAllMocks()
  })

  it('lists the schedules that go with the user, then deletes', async () => {
    mocks.getUserDeletionImpact.mockResolvedValueOnce(
      create(GetUserDeletionImpactResponseSchema, {
        schedules: [{ gameServerId: 'gs-1', gameServerName: 'Valheim', name: 'Nightly backup' }],
      }),
    )
    mocks.deleteUser.mockResolvedValueOnce({})

    const wrapper = mountDialog('user-one')
    await flushPromises()

    expect(mocks.getUserDeletionImpact).toHaveBeenCalledWith(
      expect.objectContaining({ id: 'id-user-one' }),
    )
    expect(wrapper.text()).toContain('user-one')
    expect(wrapper.text()).toContain('This schedule they created will be deleted too')
    expect(wrapper.text()).toContain('Valheim · Nightly backup')

    await deleteButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(mocks.deleteUser).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('submit')).toEqual([[false]])
    expect(mocks.notifySuccess).toHaveBeenCalledWith('user-one deleted successfully', {
      timeout: 5000,
    })
  })

  it('names owned servers and access grants instead of offering Delete', async () => {
    mocks.getUserDeletionImpact.mockResolvedValueOnce(
      create(GetUserDeletionImpactResponseSchema, {
        ownedGameServers: [{ id: 'gs-1', name: 'Valheim' }],
        grantsGiven: [{ gameServerId: 'gs-2', gameServerName: 'Rust', userName: 'friend' }],
      }),
    )

    const wrapper = mountDialog('owner')
    await flushPromises()

    expect(wrapper.text()).toContain("can't be deleted yet")
    expect(wrapper.find('a[href="/game-servers/gs-1/settings"]').text()).toBe('Valheim')
    expect(wrapper.find('a[href="/game-servers/gs-2/access"]').text()).toBe('Rust')
    expect(wrapper.text()).toContain('friend on')
    expect(deleteButton(wrapper)).toBeUndefined()
  })

  it('shows the server message in the dialog when delete fails', async () => {
    mocks.getUserDeletionImpact.mockResolvedValueOnce(
      create(GetUserDeletionImpactResponseSchema, {}),
    )
    mocks.deleteUser.mockRejectedValueOnce(
      new ConnectError('cannot remove the last super user', Code.FailedPrecondition),
    )

    const wrapper = mountDialog('user-two')
    await flushPromises()
    await deleteButton(wrapper)?.trigger('click')
    await flushPromises()

    expect(wrapper.emitted('submit')).toEqual([[true]])
    expect(wrapper.find('[role="alert"]').text()).toBe('cannot remove the last super user')
    expect(mocks.notifySuccess).not.toHaveBeenCalled()
  })
})
