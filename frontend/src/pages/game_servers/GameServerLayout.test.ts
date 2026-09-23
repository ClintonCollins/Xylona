import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { defineComponent, h, nextTick } from 'vue'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameSchema, GameServerSchema } from '@/proto/shared_pb'
import EmptyState from '@/components/shared/EmptyState.vue'
import { GetGameServerResponseSchema } from '@/proto/xylona_pb'
import { XylonaEventBus } from '@/utils/shared'

import GameServerLayout from './GameServerLayout.vue'

const RouterViewStub = defineComponent({
  name: 'RouterViewStub',
  setup: () => () => h('div'),
})

const mocks = vi.hoisted(() => ({
  checkUserAuthenticated: vi.fn(),
  getGameServer: vi.fn(),
  replace: vi.fn(),
  route: null as unknown as { path: string; params: { id: string }; meta: { title?: string } },
}))

vi.mock('vue-router', async () => {
  const { reactive } = await vi.importActual<typeof import('vue')>('vue')
  mocks.route = reactive({
    path: '/game-servers/server-a/console',
    params: { id: 'server-a' },
    meta: { title: 'Console' },
  })
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
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    getGameServerReadiness: () => Promise.resolve({ items: [] }),
  }),
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
    // The section page waits for the new server to load before it mounts.
    await flushPromises()
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

  it.each([
    { label: 'deleted', code: Code.NotFound },
    { label: 'forbidden', code: Code.PermissionDenied },
  ])('shows not found and never mounts the section page for a $label server', async ({ code }) => {
    mocks.getGameServer.mockRejectedValue(new ConnectError('missing', code))

    const wrapper = shallowMount(GameServerLayout, {
      global: { stubs: { 'router-view': RouterViewStub } },
    })
    await flushPromises()

    expect(wrapper.findComponent(RouterViewStub).exists()).toBe(false)
    expect(wrapper.find('empty-state-stub').attributes('title')).toBe('Game server not found')
    expect(wrapper.findComponent(EmptyState).props('titleTag')).toBe('h1')
    expect(wrapper.find('game-server-identity-bar-stub').exists()).toBe(false)
    expect(document.title).toBe('Game server not found · Xylona')
  })

  it('keeps the tabs and the section when a reconnect re-read fails', async () => {
    mocks.route.path = '/game-servers/server-a/files'
    mocks.getGameServer.mockResolvedValue(
      buildGameServerResponse('server-a', ['game_server.console', 'game_server.files.view']),
    )
    const wrapper = shallowMount(GameServerLayout, {
      global: { stubs: { 'router-view': RouterViewStub } },
    })
    await flushPromises()
    const viewModel = wrapper.vm as unknown as { layoutTabs: Array<{ name: string }> }
    const tabNames = viewModel.layoutTabs.map((tab) => tab.name)
    expect(tabNames).toContain('Files')
    // Layouts mounted by earlier tests also react to the route change above.
    mocks.replace.mockClear()

    mocks.getGameServer.mockRejectedValue(new ConnectError('blip', Code.Unavailable))
    busHandler('websocketConnected')()
    await flushPromises()

    expect(viewModel.layoutTabs.map((tab) => tab.name)).toEqual(tabNames)
    expect(mocks.replace).not.toHaveBeenCalled()
    expect(wrapper.findComponent(RouterViewStub).exists()).toBe(true)
  })

  it('re-reads the server after a section page edits it', async () => {
    shallowMount(GameServerLayout, {
      global: { stubs: { 'router-view': RouterViewStub } },
    })
    await flushPromises()
    mocks.getGameServer.mockResolvedValue(
      buildGameServerResponse('server-a', ['game_server.console'], 'Renamed'),
    )

    expect(document.title).toBe('Test Server · Console · Xylona')

    busHandler('gameServerEdited')('server-b')
    await flushPromises()
    expect(document.title).toBe('Test Server · Console · Xylona')

    busHandler('gameServerEdited')('server-a')
    await flushPromises()
    expect(document.title).toBe('Renamed · Console · Xylona')
  })

  it('still mounts the section page when the server cannot be read for another reason', async () => {
    mocks.getGameServer.mockRejectedValue(new ConnectError('down', Code.Unavailable))

    const wrapper = shallowMount(GameServerLayout, {
      global: { stubs: { 'router-view': RouterViewStub } },
    })
    await flushPromises()

    expect(wrapper.find('empty-state-stub').exists()).toBe(false)
    expect(wrapper.findComponent(RouterViewStub).exists()).toBe(true)
  })
})

function buildGameServerResponse(
  serverID: string,
  permissions = ['game_server.console'],
  name = 'Test Server',
) {
  return create(GetGameServerResponseSchema, {
    gameServer: create(GameServerSchema, {
      id: serverID,
      name,
      userId: 'user-1',
      gameId: 'palworld',
      effectivePermissions: permissions,
      game: create(GameSchema, { allowStartArgEditing: true }),
    }),
  })
}

/** The last handler the layout registered on the (mocked) event bus. */
function busHandler(event: string): (...args: unknown[]) => void {
  const call = vi.mocked(XylonaEventBus.on).mock.calls.findLast(([name]) => name === event)
  if (call === undefined) throw new Error(`no ${event} handler`)
  return call[1] as (...args: unknown[]) => void
}

function createDeferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}
