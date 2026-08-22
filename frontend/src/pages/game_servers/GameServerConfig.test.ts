import { flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

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

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRoute: () => mocks.routeState.current,
    onBeforeRouteLeave: vi.fn(),
  }
})

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerConfigFiles: mocks.getGameServerConfigFiles,
    getGameServerConfigFile: mocks.getGameServerConfigFile,
    updateGameServerConfigFile: mocks.updateGameServerConfigFile,
    generateGameServerConfigFile: mocks.generateGameServerConfigFile,
    getSevenDaysToDieSandboxSettings: mocks.getSevenDaysToDieSandboxSettings,
  }),
  ConnectErrorToString: (err: unknown) => String(err),
}))

function mountConfig() {
  return mount(GameServerConfig, {
    global: {
      stubs: {
        'q-icon': { template: '<i />' },
        'q-spinner-dots': { template: '<div class="spinner-stub" />' },
        ConfigFileSidebar: { template: '<div class="sidebar-stub" />' },
        ConfigFileEditor: { template: '<div class="editor-stub" />' },
      },
    },
  })
}

describe('GameServerConfig', () => {
  beforeEach(() => {
    mocks.routeState.current = undefined
  })

  afterEach(() => {
    mocks.notify.mockReset()
    mocks.dialog.mockReset()
    mocks.getGameServerConfigFiles.mockReset()
    mocks.getGameServerConfigFile.mockReset()
    mocks.updateGameServerConfigFile.mockReset()
    mocks.generateGameServerConfigFile.mockReset()
    mocks.getSevenDaysToDieSandboxSettings.mockReset()
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
      visible: true,
    },
    {
      name: 'leaves other config workflows unchanged',
      fields: [{ key: 'ServerName', value: 'Example' }],
      visible: false,
    },
  ])('$name', async ({ fields, visible }) => {
    mocks.routeState.current = { params: { id: 'server-12' } }
    mocks.getGameServerConfigFiles.mockResolvedValue({
      configFiles: [
        { path: 'serverconfig.xml', category: 'General', format: 'xml', existsOnDisk: true },
      ],
    })
    mocks.getGameServerConfigFile.mockResolvedValue({ fields, advancedFields: [] })

    const wrapper = mountConfig()
    await flushPromises()

    expect(wrapper.findComponent({ name: 'SevenDaysToDieSandboxInspector' }).exists()).toBe(visible)
    expect(wrapper.find('.editor-stub').exists()).toBe(true)
  })
})
