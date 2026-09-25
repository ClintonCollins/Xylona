import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UpdateProviderKind } from '@/proto/shared_pb'
import {
  SevenDaysToDieWebAPIConnectionState,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import GameServerMods from './GameServerMods.vue'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  getReportedMods: vi.fn(),
  getUpdateTargets: vi.fn(),
  listInstalledMods: vi.fn(),
  getModVersions: vi.fn(),
  installMod: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    getSevenDaysToDieReportedMods: mocks.getReportedMods,
    getUpdateTargets: mocks.getUpdateTargets,
    listInstalledMods: mocks.listInstalledMods,
    getModVersions: mocks.getModVersions,
    installMod: mocks.installMod,
  }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    Notify: { create: vi.fn() },
    useQuasar: () => ({ dialog: vi.fn() }),
  }
})

const PanelStub = defineComponent({
  template: '<section><slot /></section>',
})

const InstalledModsTableStub = defineComponent({
  props: {
    installedMods: { type: Array, default: () => [] },
    emptyDescription: { type: String, default: '' },
  },
  template:
    '<div data-testid="managed-mods">managed:{{ installedMods.length }} {{ emptyDescription }}</div>',
})

const minecraftSources = {
  sources: [{ id: 'modrinth', searchParamsJson: '{}' }],
}

const reportedModsResponse = {
  connectionState:
    SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
  state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
  mods: [],
}

function mountPage() {
  return mount(GameServerMods, {
    global: {
      stubs: {
        'q-banner': {
          template: '<div role="alert"><slot /><slot name="action" /></div>',
        },
        'q-btn': {
          props: ['label'],
          emits: ['click'],
          template: '<button @click="$emit(\'click\')">{{ label }}</button>',
        },
        'q-icon': true,
        'q-tabs': PanelStub,
        'q-tab': true,
        'q-separator': true,
        'q-tab-panels': PanelStub,
        'q-tab-panel': PanelStub,
        InstalledModsTable: InstalledModsTableStub,
        ModBrowse: true,
        ModDetailDialog: true,
        ModInstallDialog: true,
        PageHeader: true,
      },
    },
  })
}

describe('GameServerMods reported mods', () => {
  beforeEach(() => {
    mocks.getModVersions.mockReset()
    mocks.installMod.mockReset()
    mocks.getGameServer.mockReset()
    mocks.getReportedMods.mockReset()
    mocks.getUpdateTargets.mockReset()
    mocks.listInstalledMods.mockReset()
    mocks.listInstalledMods.mockResolvedValue({ installedMods: [] })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: '7_days_to_die' } })
    mocks.getReportedMods.mockResolvedValue(reportedModsResponse)
  })

  it('lets Valheim browse Thunderstore and install a selected mod', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: {
        gameId: 'valheim',
        resolvedModProfile: {
          sources: [{ id: 'thunderstore', searchParamsJson: '{"community":"valheim"}' }],
        },
      },
    })
    mocks.getModVersions.mockResolvedValue({
      versions: [
        {
          versionId: '1.2.3',
          dependencies: [{ sourceId: 'Example-Library-1.0.0', name: 'Library', required: true }],
        },
      ],
    })
    mocks.installMod.mockResolvedValue({})
    const wrapper = mountPage()
    await flushPromises()

    const browser = wrapper.getComponent({ name: 'ModBrowse' })
    expect(browser.props('sources')).toEqual([
      { id: 'thunderstore', searchParams: { community: 'valheim' } },
    ])
    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('browse')
    expect(wrapper.find('[data-testid="managed-mods"]').exists()).toBe(true)
    browser.vm.$emit('install', 'thunderstore', 'Example-Mod', 'Example Mod')
    await flushPromises()

    expect(mocks.installMod).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-1',
        source: 'thunderstore',
        sourceId: 'Example-Mod',
        versionId: '1.2.3',
      }),
    )
    expect(mocks.listInstalledMods).toHaveBeenCalledTimes(2)
    expect(mocks.installMod).toHaveBeenCalledOnce()
    expect(mocks.getReportedMods).not.toHaveBeenCalled()
  })

  it('keeps managed and reported inventories separate and escapes reported text', async () => {
    mocks.getReportedMods.mockResolvedValue({
      ...reportedModsResponse,
      mods: [
        {
          name: 'safe-name',
          displayName: '<script>alert(1)</script>',
          description: '<img src=x onerror=alert(1)>',
          author: 'Author',
          version: '1.2.3',
        },
      ],
    })

    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Xylona-managed')
    expect(wrapper.text()).toContain('Reported by game server')
    expect(wrapper.text()).toContain('<script>alert(1)</script>')
    expect(wrapper.text()).toContain('<img src=x onerror=alert(1)>')
    expect(wrapper.find('.reported-mods script').exists()).toBe(false)
    expect(wrapper.find('.reported-mods img').exists()).toBe(false)
    expect(wrapper.find('.reported-mods button').exists()).toBe(false)
    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('installed')
  })

  it.each([
    {
      name: 'empty',
      overrides: {},
      text: 'No mods reported by the game server.',
    },
    {
      name: 'offline',
      overrides: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
      },
      text: 'The game server is offline.',
    },
    {
      name: 'unsupported',
      overrides: {
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
      },
      text: 'This game server does not support reporting mods.',
    },
    {
      name: 'permission denied',
      overrides: {
        state:
          SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED,
      },
      text: 'The game server denied access to its reported mods.',
    },
    {
      name: 'discovery authentication denied',
      overrides: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
      },
      text: 'The game server denied access to its reported mods.',
    },
    {
      name: 'unavailable',
      overrides: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
      },
      text: 'Reported mods are currently unavailable.',
    },
  ])('shows the $name state', async ({ overrides, text }) => {
    mocks.getReportedMods.mockResolvedValue({ ...reportedModsResponse, ...overrides })
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain(text)
  })

  it('shows loading while the reported inventory is pending', async () => {
    mocks.getReportedMods.mockReturnValue(new Promise(() => undefined))
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Loading reported mods...')
  })

  it('keeps managed mods visible when the reported query fails', async () => {
    mocks.listInstalledMods.mockResolvedValue({ installedMods: [{ id: 'managed-1' }] })
    mocks.getReportedMods.mockRejectedValue(new Error('native query failed'))
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.get('[data-testid="managed-mods"]').text()).toContain('managed:1')
    expect(wrapper.text()).toContain('Reported mods are currently unavailable.')
  })

  it('starts reported loading without waiting for update targets', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { gameId: '7_days_to_die', selectedVariantId: 'stable' },
    })
    mocks.getUpdateTargets.mockReturnValue(new Promise(() => undefined))
    const wrapper = mountPage()
    await flushPromises()

    expect(mocks.getReportedMods).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('No mods reported by the game server.')
  })

  it('keeps the existing browse default for non-7DTD servers', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { gameId: 'minecraft', resolvedModProfile: minecraftSources },
    })
    const wrapper = mountPage()
    await flushPromises()

    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('browse')
    expect(mocks.getReportedMods).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('Reported by game server')
  })

  it('shows a retryable error instead of an empty list when installed mods fail', async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: { gameId: 'minecraft', resolvedModProfile: minecraftSources },
    })
    mocks.listInstalledMods.mockRejectedValueOnce(new Error('backend down'))
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Installed mods could not be loaded.')
    expect(wrapper.text()).toContain('backend down')
    expect(wrapper.find('[data-testid="managed-mods"]').exists()).toBe(false)
    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('installed')

    await wrapper.get('[aria-label="Retry loading installed mods"]').trigger('click')
    await flushPromises()

    expect(mocks.listInstalledMods).toHaveBeenCalledTimes(2)
    expect(wrapper.find('[data-testid="managed-mods"]').exists()).toBe(true)
  })

  it('does not claim there are no mod sources when the server config fails', async () => {
    mocks.getGameServer.mockRejectedValueOnce(new Error('config down'))
    const wrapper = mountPage()
    await flushPromises()

    expect(wrapper.text()).toContain('Mod sources could not be loaded.')
    expect(wrapper.text()).not.toContain('Xylona has no mod sources')

    mocks.getGameServer.mockResolvedValue({
      gameServer: { gameId: 'minecraft', resolvedModProfile: minecraftSources },
    })
    await wrapper.get('[aria-label="Retry loading mod sources"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).not.toContain('Mod sources could not be loaded.')
    expect(wrapper.findComponent({ name: 'ModBrowse' }).exists()).toBe(true)
  })

  it('stays on Installed and points to Files when the game has no mod sources', async () => {
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'minecraft' } })
    const wrapper = mountPage()
    await flushPromises()

    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('installed')
    expect(wrapper.findComponent({ name: 'ModBrowse' }).exists()).toBe(false)
    expect(wrapper.text()).toContain('Install mods manually through Files.')
  })

  it("installs the newest version that supports the server's game version", async () => {
    mocks.getGameServer.mockResolvedValue({
      gameServer: {
        gameId: 'minecraft',
        name: 'Survival',
        resolvedModProfile: minecraftSources,
        resolvedUpdateProvider: { kind: UpdateProviderKind.PAPERMC },
      },
    })
    mocks.getUpdateTargets.mockResolvedValue({
      targets: [
        { id: '1.21.5', label: '1.21.5', isSelected: false },
        { id: '1.21.4', label: '1.21.4', isSelected: true },
      ],
    })
    mocks.getModVersions.mockResolvedValue({ versions: [{ versionId: 'v9', dependencies: [] }] })
    mocks.installMod.mockResolvedValue({})
    const wrapper = mountPage()
    await flushPromises()

    const browser = wrapper.getComponent({ name: 'ModBrowse' })
    expect(browser.props('defaultGameVersion')).toBe('1.21.4')
    browser.vm.$emit('install', 'modrinth', 'simple-voice-chat', 'Simple Voice Chat')
    await flushPromises()

    expect(mocks.getModVersions).toHaveBeenCalledWith(
      expect.objectContaining({ sourceId: 'simple-voice-chat', gameVersion: '1.21.4' }),
    )
    expect(mocks.installMod).toHaveBeenCalledWith(expect.objectContaining({ versionId: 'v9' }))
  })
})
