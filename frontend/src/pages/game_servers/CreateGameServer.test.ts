import { createPinia, setActivePinia } from 'pinia'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, beforeEach, vi } from 'vitest'

import { create as createProto } from '@bufbuild/protobuf'
import { CheckUserAuthenticatedResponseSchema, UserSchema } from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import CreateGameServer from './CreateGameServer.vue'

const mocks = vi.hoisted(() => ({
  replace: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      replace: mocks.replace,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

describe('CreateGameServer page', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.replace.mockReset()
    mocks.notify.mockReset()
  })

  it('redirects non-superusers back to the list and notifies them', async () => {
    const store = useUserAuthStore()
    store.user = createProto(UserSchema, {
      id: 'user-owner',
      userName: 'owner',
      superUser: false,
    })
    store.initialFetch = true
    store.initialResponse = createProto(CheckUserAuthenticatedResponseSchema, {
      authenticated: true,
      user: store.user,
    })

    mount(CreateGameServer, {
      global: {
        stubs: {
          'q-page': { template: '<div><slot /></div>' },
          GameServerCreateForm: { template: '<div data-testid="create-form"></div>' },
        },
      },
    })

    await flushPromises()

    expect(mocks.replace).toHaveBeenCalledWith('/game-servers')
    expect(mocks.notify).toHaveBeenCalledTimes(1)
  })

  it('renders the create form for superusers', async () => {
    const store = useUserAuthStore()
    store.user = createProto(UserSchema, {
      id: 'user-admin',
      userName: 'admin',
      superUser: true,
    })
    store.initialFetch = true
    store.initialResponse = createProto(CheckUserAuthenticatedResponseSchema, {
      authenticated: true,
      user: store.user,
    })

    const wrapper = mount(CreateGameServer, {
      global: {
        stubs: {
          'q-page': { template: '<div><slot /></div>' },
          GameServerCreateForm: { template: '<div data-testid="create-form"></div>' },
        },
      },
    })

    await flushPromises()

    expect(mocks.replace).not.toHaveBeenCalled()
    expect(wrapper.find('[data-testid="create-form"]').exists()).toBe(true)
  })
})
