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
})
