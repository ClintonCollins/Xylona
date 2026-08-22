import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

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
}))

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServer: mocks.getGameServer,
    getSevenDaysToDieReportedMods: mocks.getReportedMods,
    getUpdateTargets: mocks.getUpdateTargets,
    listInstalledMods: mocks.listInstalledMods,
  }),
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({ params: { id: 'server-1' } }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({ dialog: vi.fn() }),
  }
})

const PanelStub = defineComponent({
  template: '<section><slot /></section>',
})

const InstalledModsTableStub = defineComponent({
  props: {
    installedMods: { type: Array, default: () => [] },
  },
  template: '<div data-testid="managed-mods">managed:{{ installedMods.length }}</div>',
})

function mountPage() {
  return mount(GameServerMods, {
    global: {
      stubs: {
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
    mocks.getGameServer.mockReset()
    mocks.getReportedMods.mockReset()
    mocks.getUpdateTargets.mockReset()
    mocks.listInstalledMods.mockReset()
    mocks.listInstalledMods.mockResolvedValue({ installedMods: [] })
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: '7_days_to_die' } })
    mocks.getReportedMods.mockResolvedValue({
      connectionState:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
      mods: [],
    })
  })

  it('keeps managed and reported inventories separate and escapes reported text', async () => {
    mocks.getReportedMods.mockResolvedValue({
      connectionState:
        SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
      state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
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
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE,
        mods: [],
      },
      text: 'No mods reported by the game server.',
    },
    {
      name: 'offline',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
        mods: [],
      },
      text: 'The game server is offline.',
    },
    {
      name: 'unsupported',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED,
        mods: [],
      },
      text: 'This game server does not support reporting mods.',
    },
    {
      name: 'permission denied',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
        state:
          SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED,
        mods: [],
      },
      text: 'The game server denied access to its reported mods.',
    },
    {
      name: 'unavailable',
      response: {
        connectionState:
          SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE,
        state: SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE,
        mods: [],
      },
      text: 'Reported mods are currently unavailable.',
    },
  ])('shows the $name state', async ({ response, text }) => {
    mocks.getReportedMods.mockResolvedValue(response)
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

    expect(wrapper.get('[data-testid="managed-mods"]').text()).toBe('managed:1')
    expect(wrapper.text()).toContain('Reported mods are currently unavailable.')
  })

  it('keeps the existing browse default for non-7DTD servers', async () => {
    mocks.getGameServer.mockResolvedValue({ gameServer: { gameId: 'minecraft' } })
    const wrapper = mountPage()
    await flushPromises()

    expect((wrapper.vm as unknown as { activeTab: string }).activeTab).toBe('browse')
    expect(mocks.getReportedMods).not.toHaveBeenCalled()
    expect(wrapper.text()).not.toContain('Reported by game server')
  })
})
