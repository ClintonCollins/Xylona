import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  SevenDaysToDieSandboxComparisonState,
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'

import { useUserAuthStore } from '@/stores/xylona'

import GameServerConfig from './GameServerConfig.vue'

const mocks = vi.hoisted(() => ({
  routeState: {
    current: undefined as
      | {
          params?: {
            id?: string | string[]
          }
        }
      | undefined,
  },
  notify: vi.fn(),
  dialog: vi.fn(),
  getGameServerConfigFiles: vi.fn(),
  getGameServerConfigFile: vi.fn(),
  updateGameServerConfigFile: vi.fn(),
  generateGameServerConfigFile: vi.fn(),
  getSevenDaysToDieSandboxSettings: vi.fn(),
  getGameServer: vi.fn(),
  confirmSaved: vi.fn(),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
      dialog: mocks.dialog,
    }),
  }
})

// Every toast goes through the shared helpers; record them on one mock.
vi.mock('@/api/notifications', () => ({
  notifyConnectError: mocks.notify,
  notifySuccess: mocks.notify,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => mocks.routeState.current,
    onBeforeRouteLeave: vi.fn(),
    onBeforeRouteUpdate: vi.fn(),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerConfigFiles: mocks.getGameServerConfigFiles,
    getGameServerConfigFile: mocks.getGameServerConfigFile,
    updateGameServerConfigFile: mocks.updateGameServerConfigFile,
    generateGameServerConfigFile: mocks.generateGameServerConfigFile,
    getSevenDaysToDieSandboxSettings: mocks.getSevenDaysToDieSandboxSettings,
    getGameServer: mocks.getGameServer,
  }),
  ConnectErrorToString: (err: unknown) => String(err),
}))

function mountConfig() {
  return mount(GameServerConfig, {
    global: {
      stubs: {
        'q-icon': { template: '<i />' },
        'q-spinner-dots': { template: '<div class="spinner-stub" />' },
        'q-banner': { template: '<div><slot /><slot name="action" /></div>' },
        'q-btn': {
          props: ['label'],
          emits: ['click'],
          template: `<button @click="$emit('click')">{{ label }}</button>`,
        },
        ConfigFileSidebar: { template: '<div class="sidebar-stub" />' },
        ConfigFileEditor: {
          name: 'ConfigFileEditor',
          props: ['fields', 'saving', 'validationErrors'],
          emits: ['save', 'discard'],
          methods: { confirmSaved: mocks.confirmSaved },
          template: '<div class="editor-stub" />',
        },
      },
    },
  })
}

const oneConfigFile = [
  { path: 'server.properties', category: 'Server', format: 'properties', existsOnDisk: true },
]

async function mountWithFile() {
  mocks.routeState.current = { params: { id: 'server-12' } }
  mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: oneConfigFile })
  mocks.getGameServerConfigFile.mockResolvedValue({
    fields: [{ key: 'motd', value: 'Hello' }],
    advancedFields: [],
  })
  mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'minecraft' } })
  const wrapper = mountConfig()
  await flushPromises()
  return wrapper
}

function createDeferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void
  const promise = new Promise<T>((resolvePromise) => {
    resolve = resolvePromise
  })
  return { promise, resolve }
}

describe('GameServerConfig', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    mocks.routeState.current = undefined
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: '7_days_to_die' } })
  })

  afterEach(() => {
    mocks.notify.mockReset()
    mocks.dialog.mockReset()
    mocks.getGameServerConfigFiles.mockReset()
    mocks.getGameServerConfigFile.mockReset()
    mocks.updateGameServerConfigFile.mockReset()
    mocks.generateGameServerConfigFile.mockReset()
    mocks.getSevenDaysToDieSandboxSettings.mockReset()
    mocks.getGameServer.mockReset()
    mocks.confirmSaved.mockReset()
  })

  it('renders the Configuration page title', async () => {
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.find('h1.xy-page-title').text()).toBe('Configuration')
  })

  it('does not crash or fetch config files when the route context is unavailable', async () => {
    const mountComponent = () => mountConfig()

    expect(mountComponent).not.toThrow()

    const wrapper = mountComponent()
    await flushPromises()

    expect(mocks.getGameServerConfigFiles).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('No config files for this game yet')
  })

  it.each([
    {
      name: 'shows the inspector for SandboxCode metadata',
      fields: [{ key: 'SandboxCode', value: 'ABC' }],
      gameId: '7_days_to_die',
      visible: true,
    },
    {
      name: 'leaves other config workflows unchanged',
      fields: [{ key: 'ServerName', value: 'Example' }],
      gameId: '7_days_to_die',
      visible: false,
    },
    {
      name: 'does not show for another game with custom SandboxCode metadata',
      fields: [{ key: 'SandboxCode', value: 'CUSTOM' }],
      gameId: 'custom_game',
      visible: false,
    },
  ])('$name', async ({ fields, gameId, visible }) => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({
      configFiles: [
        { path: 'serverconfig.xml', category: 'General', format: 'xml', existsOnDisk: true },
      ],
    })
    mocks.getGameServerConfigFile.mockResolvedValue({ fields, advancedFields: [] })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId } })

    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.findComponent({ name: 'SevenDaysToDieSandboxInspector' }).exists()).toBe(visible)
    expect(wrapper.find('.editor-stub').exists()).toBe(true)
  })

  it('marks sandbox observations stale before post-save reloads finish', async () => {
    const configFiles = [
      { path: 'serverconfig.xml', category: 'General', format: 'xml', existsOnDisk: true },
    ]
    const reloadedFields = [{ key: 'SandboxCode', value: 'NEW-CODE' }]
    const fileReload = createDeferred<{ fields: typeof reloadedFields; advancedFields: [] }>()
    const fileListReload = createDeferred<{ configFiles: typeof configFiles }>()

    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles })
    mocks.getGameServerConfigFile.mockResolvedValue({
      fields: [{ key: 'SandboxCode', value: 'OLD-CODE' }],
      advancedFields: [],
    })
    mocks.getSevenDaysToDieSandboxSettings.mockResolvedValue({
      connectionState:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      comparisonState: SevenDaysToDieSandboxComparisonState.MATCH,
      configuredCode: 'OLD-CODE',
      effectiveCode: 'OLD-CODE',
      settings: [],
    })
    mocks.updateGameServerConfigFile.mockResolvedValue({ success: true, errors: [] })

    const wrapper = mountConfig()
    await flushPromises()

    const inspector = wrapper.findComponent({ name: 'SevenDaysToDieSandboxInspector' })
    await inspector.find('.sandbox-summary').trigger('click')
    await flushPromises()
    expect(inspector.text()).toContain('Match')

    mocks.getGameServerConfigFile.mockReturnValueOnce(fileReload.promise)
    mocks.getGameServerConfigFiles.mockReturnValueOnce(fileListReload.promise)

    wrapper
      .findComponent({ name: 'ConfigFileEditor' })
      .vm.$emit('save', new Map([['SandboxCode', 'NEW-CODE']]))
    await flushPromises()

    expect(inspector.props('refreshKey')).toBe(1)
    expect(inspector.text()).toContain('Stale')
    expect(mocks.getGameServerConfigFile).toHaveBeenCalledTimes(2)
    expect(mocks.getGameServerConfigFiles).toHaveBeenCalledTimes(1)

    fileReload.resolve({ fields: reloadedFields, advancedFields: [] })
    await flushPromises()

    expect(mocks.getGameServerConfigFiles).toHaveBeenCalledTimes(2)
    expect(inspector.text()).toContain('Stale')

    fileListReload.resolve({ configFiles })
    await flushPromises()

    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('fields')).toEqual(
      reloadedFields,
    )
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('saving')).toBe(false)
  })

  it.each([
    {
      name: 'a rejected save keeps the edits and shows the errors',
      update: () =>
        mocks.updateGameServerConfigFile.mockResolvedValue({
          success: false,
          errors: [{ field: 'motd', message: 'Too long' }],
        }),
      errors: [{ field: 'motd', message: 'Too long' }],
    },
    {
      name: 'a failed request keeps the edits',
      update: () => mocks.updateGameServerConfigFile.mockRejectedValue(new Error('offline')),
      errors: [],
    },
  ])('$name', async ({ update, errors }) => {
    const wrapper = await mountWithFile()
    update()

    wrapper.findComponent({ name: 'ConfigFileEditor' }).vm.$emit('save', new Map([['motd', 'x']]))
    await flushPromises()

    expect(mocks.confirmSaved).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'ConfigFileEditor' }).props('validationErrors')).toEqual(
      errors,
    )
  })

  it('confirms the save to the editor once the file has reloaded', async () => {
    const wrapper = await mountWithFile()
    mocks.updateGameServerConfigFile.mockResolvedValue({ success: true, errors: [] })

    wrapper.findComponent({ name: 'ConfigFileEditor' }).vm.$emit('save', new Map([['motd', 'x']]))
    await flushPromises()

    expect(mocks.confirmSaved).toHaveBeenCalledTimes(1)
    expect(mocks.getGameServerConfigFile).toHaveBeenCalledTimes(2)
  })

  it('offers Retry when the config files fail to load', async () => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockRejectedValueOnce(new Error('unavailable'))
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.find('[data-test="config-load-error"]').exists()).toBe(true)
    expect(wrapper.text()).not.toContain('No config files for this game yet')

    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: oneConfigFile })
    mocks.getGameServerConfigFile.mockResolvedValue({ fields: [], advancedFields: [] })
    await wrapper.find('[data-test="config-load-error"] button').trigger('click')
    await flushPromises()

    expect(wrapper.find('[data-test="config-load-error"]').exists()).toBe(false)
    expect(wrapper.find('.editor-stub').exists()).toBe(true)
  })

  it('shows a file load failure with Retry instead of an empty editor', async () => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: oneConfigFile })
    mocks.getGameServerConfigFile.mockRejectedValueOnce(new Error('node offline'))
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.find('[data-test="config-file-load-error"]').exists()).toBe(true)
    expect(wrapper.find('.editor-stub').exists()).toBe(false)
  })

  it.each([
    { name: 'links superusers to the game editor', superUser: true, linked: true },
    { name: 'leaves other users without the link', superUser: false, linked: false },
  ])('$name when the game has no config files', async ({ superUser, linked }) => {
    useUserAuthStore().user = { superUser } as never
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({ configFiles: [] })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'valheim' } })
    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.text()).toContain('No config files for this game yet')
    expect(wrapper.find('[data-test="config-edit-game"]').exists()).toBe(linked)
  })
})
