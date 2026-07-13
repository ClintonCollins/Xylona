import { create } from '@bufbuild/protobuf'
import { defineComponent, h, nextTick, ref } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema, GameServerSchema } from '@/proto/shared_pb'
import { GetGameServerResponseSchema } from '@/proto/xylona_pb'

import GameServerLayout from './GameServerLayout.vue'

const RouterViewStub = defineComponent({
  name: 'RouterViewStub',
  setup: () => () => h('div'),
})

const mocks = vi.hoisted(() => ({
  changeTabs: vi.fn(),
  checkUserAuthenticated: vi.fn(),
  getGameServer: vi.fn(),
  replace: vi.fn(),
  route: null as unknown as { path: string; params: { id: string } },
}))

vi.mock('vue-router', async () => {
  const { reactive } = await vi.importActual<typeof import('vue')>('vue')
  mocks.route = reactive({ path: '/game-servers/server-a/console', params: { id: 'server-a' } })
  return {
    useRoute: () => mocks.route,
    useRouter: () => ({ replace: mocks.replace }),
  }
})

vi.mock('@/stores/xylona', () => ({
  useToolbarNavQTabsStore: () => ({ tabs: [], changeTabs: mocks.changeTabs }),
  useUserAuthStore: () => ({
    checkUserAuthenticated: mocks.checkUserAuthenticated,
    user: { id: 'user-1', superUser: false },
  }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getGameServer: mocks.getGameServer }),
  WindowWidth: () => ref(1280),
}))

vi.mock('@/composables/useServerSoftwareInstall', () => ({
  useServerSoftwareInstall: vi.fn(),
}))

describe('GameServerLayout', () => {
  beforeEach(() => {
    mocks.route.params.id = 'server-a'
    mocks.route.path = '/game-servers/server-a/console'
    mocks.changeTabs.mockReset()
    mocks.getGameServer.mockReset()
    mocks.replace.mockReset()
    mocks.checkUserAuthenticated.mockResolvedValue({
      user: { id: 'user-1', superUser: false },
      permissionIds: [],
    })
    mocks.getGameServer.mockImplementation((request: { id: string }) =>
      Promise.resolve(buildGameServerResponse(request.id)),
    )
  })

  it('changes the nested router-view key when navigating directly between servers', async () => {
    const wrapper = shallowMount(GameServerLayout, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          'router-view': RouterViewStub,
        },
      },
    })
    const viewModel = wrapper.vm as unknown as { gameServerRouteKey: string }

    expect(viewModel.gameServerRouteKey).toBe('server-a')

    mocks.route.params.id = 'server-b'
    mocks.route.path = '/game-servers/server-b/console'
    await nextTick()

    expect(viewModel.gameServerRouteKey).toBe('server-b')
    const routerView = wrapper.findComponent(RouterViewStub)
    expect(routerView.vm.$.vnode.key).toBe('server-b')
  })

  it('does not let a slower prior server request overwrite the active server tabs', async () => {
    const wrapper = shallowMount(GameServerLayout)
    await flushPromises()
    mocks.changeTabs.mockClear()

    const serverARequest = createDeferred<ReturnType<typeof buildGameServerResponse>>()
    const serverBRequest = createDeferred<ReturnType<typeof buildGameServerResponse>>()
    mocks.getGameServer.mockImplementation((request: { id: string }) =>
      request.id === 'server-a' ? serverARequest.promise : serverBRequest.promise,
    )

    const viewModel = wrapper.vm as unknown as { configureTabs: () => Promise<boolean> }
    const configureA = viewModel.configureTabs()
    mocks.route.params.id = 'server-b'
    mocks.route.path = '/game-servers/server-b/console'
    const configureB = viewModel.configureTabs()

    serverBRequest.resolve(buildGameServerResponse('server-b'))
    await configureB
    serverARequest.resolve(buildGameServerResponse('server-a'))
    await configureA
    await flushPromises()

    const lastTabs = mocks.changeTabs.mock.calls.at(-1)?.[0] as Array<{ to: string }>
    expect(lastTabs.length).toBeGreaterThan(0)
    expect(lastTabs.every((tab) => tab.to.includes('/server-b/'))).toBe(true)
  })
})

function buildGameServerResponse(serverID: string) {
  return create(GetGameServerResponseSchema, {
    gameServer: create(GameServerSchema, {
      id: serverID,
      userId: 'user-1',
      effectivePermissions: ['game_server.console'],
      game: create(GameSchema, { allowStartArgEditing: true }),
    }),
  })
}

function createDeferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}
