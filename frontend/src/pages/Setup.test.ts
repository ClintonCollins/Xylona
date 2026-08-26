import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { CompleteSetupRequestSchema, UserSchema } from '@/proto/xylona_pb'
import Setup from './Setup.vue'

const mocks = vi.hoisted(() => ({
  getSetupStatus: vi.fn(),
  completeSetup: vi.fn(),
  notifyCreate: vi.fn(),
  push: vi.fn(),
  replace: vi.fn(),
  routeQuery: { token: '' } as Record<string, string>,
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      getSetupStatus: mocks.getSetupStatus,
      completeSetup: mocks.completeSetup,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notifyCreate,
    }),
  }
})

vi.mock('vue-router', () => ({
  useRoute: () => ({
    query: mocks.routeQuery,
  }),
  useRouter: () => ({
    push: mocks.push,
    replace: mocks.replace,
  }),
}))

const pageStubs = {
  'q-layout': { template: '<div><slot /></div>' },
  'q-page-container': { template: '<div><slot /></div>' },
  'q-page': { template: '<div><slot /></div>' },
  'q-form': {
    template: '<form @submit.prevent="$emit(\'submit\')"><slot /></form>',
  },
  'q-input': true,
  'q-btn': {
    props: ['label'],
    emits: ['click'],
    template: '<button type="button" @click="$emit(\'click\')">{{ label }}<slot /></button>',
  },
  'q-icon': true,
  'q-tooltip': true,
}

describe('Setup page', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.getSetupStatus.mockReset()
    mocks.completeSetup.mockReset()
    mocks.notifyCreate.mockReset()
    mocks.push.mockReset()
    mocks.replace.mockReset()
    mocks.routeQuery = { token: '' }
  })

  it('shows the blocked copy when setup is needed without a usable token', async () => {
    mocks.getSetupStatus.mockResolvedValue({
      needed: true,
    })

    const wrapper = mount(Setup, { global: { stubs: pageStubs } })
    await flushPromises()

    expect(wrapper.text()).toContain('Xylona is not set up yet')
    expect(wrapper.text()).toContain('xylona setup')
    expect(wrapper.text()).not.toContain('Create the first admin')
  })

  it('shows setup status errors and retries', async () => {
    mocks.getSetupStatus
      .mockRejectedValueOnce(new Error('status unavailable'))
      .mockResolvedValueOnce({
        needed: true,
      })

    const wrapper = mount(Setup, { global: { stubs: pageStubs } })
    await flushPromises()

    expect(wrapper.get('[role="alert"]').text()).toContain('status unavailable')
    expect(wrapper.text()).toContain('Retry')

    await wrapper.get('button').trigger('click')
    await flushPromises()

    expect(mocks.getSetupStatus).toHaveBeenCalledTimes(2)
    expect(wrapper.text()).toContain('Xylona is not set up yet')
  })

  it('submits completeSetup with the token and stores the user', async () => {
    mocks.routeQuery = { token: 'setup-token' }
    mocks.getSetupStatus.mockResolvedValue({
      needed: true,
    })
    mocks.completeSetup.mockResolvedValue({
      user: create(UserSchema, { id: 'user-1', userName: 'admin', superUser: true }),
    })

    const wrapper = mount(Setup, { global: { stubs: pageStubs } })
    await flushPromises()
    expect(wrapper.text()).toContain('Create the first admin')

    const vm = wrapper.vm as unknown as {
      username: string
      password: string
      confirmPassword: string
      submitSetup: () => Promise<void>
    }
    vm.username = 'admin'
    vm.password = 'secret'
    vm.confirmPassword = 'secret'
    await vm.submitSetup()
    await flushPromises()

    expect(mocks.completeSetup).toHaveBeenCalledWith(
      create(CompleteSetupRequestSchema, {
        userName: 'admin',
        email: '',
        password: 'secret',
        token: 'setup-token',
      }),
    )
    expect(mocks.push).toHaveBeenCalledWith({ path: '/game-servers' })
  })
})
