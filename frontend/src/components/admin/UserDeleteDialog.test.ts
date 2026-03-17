import { create } from '@bufbuild/protobuf'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { UserSchema } from '@/proto/xylona_pb'
import UserDeleteDialog from './UserDeleteDialog.vue'

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  deleteUser: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    deleteUser: mocks.deleteUser,
  }),
}))

describe('UserDeleteDialog', () => {
  afterEach(() => {
    mocks.notify.mockReset()
    mocks.deleteUser.mockReset()
  })

  it('renders user name and emits submit=false on successful delete', async () => {
    mocks.deleteUser.mockResolvedValueOnce({})

    const user = create(UserSchema, {
      id: 'user-1',
      userName: 'user-one',
    })

    const wrapper = mount(UserDeleteDialog, {
      props: {
        user,
        showDialog: true,
      },
      global: {
        stubs: {
          'q-dialog': { template: '<div><slot /></div>' },
          'q-card': { template: '<div><slot /></div>' },
          'q-card-section': { template: '<div><slot /></div>' },
          'q-card-actions': { template: '<div><slot /></div>' },
          'q-card-title': { template: '<div><slot /></div>' },
          'q-btn': {
            props: ['label'],
            emits: ['click'],
            template: '<button @click="$emit(\'click\')">{{ label }}</button>',
          },
        },
      },
    })

    expect(wrapper.text()).toContain('user-one')

    const buttons = wrapper.findAll('button')
    await buttons[1].trigger('click')
    await Promise.resolve()

    expect(mocks.deleteUser).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('submit')).toEqual([[false]])
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-success',
      }),
    )
  })

  it('emits submit=true when delete fails', async () => {
    mocks.deleteUser.mockRejectedValueOnce(new Error('delete failed'))

    const user = create(UserSchema, {
      id: 'user-2',
      userName: 'user-two',
    })

    const wrapper = mount(UserDeleteDialog, {
      props: {
        user,
        showDialog: true,
      },
      global: {
        stubs: {
          'q-dialog': { template: '<div><slot /></div>' },
          'q-card': { template: '<div><slot /></div>' },
          'q-card-section': { template: '<div><slot /></div>' },
          'q-card-actions': { template: '<div><slot /></div>' },
          'q-card-title': { template: '<div><slot /></div>' },
          'q-btn': {
            props: ['label'],
            emits: ['click'],
            template: '<button @click="$emit(\'click\')">{{ label }}</button>',
          },
        },
      },
    })

    const buttons = wrapper.findAll('button')
    await buttons[1].trigger('click')
    await Promise.resolve()

    expect(wrapper.emitted('submit')).toEqual([[true]])
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-error',
      }),
    )
  })
})
