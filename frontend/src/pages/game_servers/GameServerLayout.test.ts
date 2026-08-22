import { create } from '@bufbuild/protobuf'
import { defineComponent, h, nextTick } from 'vue'
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
  useUserAuthStore: () => ({
    checkUserAuthenticated: mocks.checkUserAuthenticated,
    user: { id: 'user-1', superUser: false },
  }),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({ getGameServer: mocks.getGameServer }),
  XylonaEventBus: {
    on: vi.fn(),
    off: vi.fn(),
  },
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ screen: { width: 1280 } }),
  }
})

describe('GameServerLayout', () => {
  beforeEach(() => {
    mocks.route.params.id = 'server-a'
    mocks.route.path = '/game-servers/server-a/console'
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

    const lastTabs = (wrapper.vm as unknown as { layoutTabs: Array<{ to: string }> }).layoutTabs
    expect(lastTabs.length).toBeGreaterThan(0)
    expect(lastTabs.every((tab) => tab.to.includes('/server-b/'))).toBe(true)
  })

  it('marks the first tab of each subsequent group as a group start', async () => {
    const basePath = '/game-servers/server-a'
    const tabs = [
      {
        name: 'Console',
        to: `${basePath}/console`,
        icon: 'terminal',
        exact: true,
        group: 'Operate',
      },
      { name: 'Map', to: `${basePath}/map`, icon: 'public', exact: true, group: 'Operate' },
      { name: 'Files', to: `${basePath}/files`, icon: 'folder', exact: true, group: 'Configure' },
      {
        name: 'Settings',
        to: `${basePath}/settings`,
        icon: 'settings',
        exact: true,
        group: 'Configure',
      },
      {
        name: 'Backups',
        to: `${basePath}/backups`,
        icon: 'archive',
        exact: true,
        group: 'Automate',
      },
      {
        name: 'Access',
        to: `${basePath}/access`,
        icon: 'manage_accounts',
        exact: true,
        group: 'Access',
      },
    ]

    const wrapper = shallowMount(GameServerLayout, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          'router-view': RouterViewStub,
        },
      },
    })
    ;(wrapper.vm as unknown as { layoutTabs: typeof tabs }).layoutTabs = tabs
    await nextTick()

    const groupStartTabs = wrapper
      .findAll('q-route-tab-stub')
      .filter((tab) => tab.classes('game-server-tab--group-start'))
      .map((tab) => tab.attributes('label'))
    expect(groupStartTabs).toEqual(['Files', 'Backups', 'Access'])
  })

  it('keeps the Mods route available for native 7DTD reports without a managed mod profile', async () => {
    mocks.route.path = '/game-servers/server-a/mods'
    mocks.getGameServer.mockResolvedValue(
      create(GetGameServerResponseSchema, {
        gameServer: create(GameServerSchema, {
          id: 'server-a',
          userId: 'user-1',
          gameId: '7_days_to_die',
          effectivePermissions: ['game_server.mods'],
          resolvedHasModSupport: false,
          game: create(GameSchema, { allowStartArgEditing: true }),
        }),
      }),
    )

    const wrapper = shallowMount(GameServerLayout, {
      global: { stubs: { 'router-view': RouterViewStub } },
    })
    await flushPromises()

    const viewModel = wrapper.vm as unknown as {
      enforceRouteAccess: () => Promise<void>
      layoutTabs: Array<{ name: string }>
    }
    const tabs = viewModel.layoutTabs
    expect(tabs.map((tab) => tab.name)).toContain('Mods')
    mocks.replace.mockClear()
    await viewModel.enforceRouteAccess()
    expect(mocks.replace).not.toHaveBeenCalled()
  })
})

function buildGameServerResponse(serverID: string) {
  return create(GetGameServerResponseSchema, {
    gameServer: create(GameServerSchema, {
      id: serverID,
      userId: 'user-1',
      gameId: 'palworld',
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
