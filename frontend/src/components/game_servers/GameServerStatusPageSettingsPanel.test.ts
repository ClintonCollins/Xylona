import { createPinia, setActivePinia } from 'pinia'
import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  GameServerStatusPageSettingsSchema,
  GameServerStatusPageSettingsServerSchema,
  UserSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import GameServerStatusPageSettingsPanel from './GameServerStatusPageSettingsPanel.vue'

const mocks = vi.hoisted(() => ({
  copyToClipboard: vi.fn(),
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  notify: vi.fn(),
}))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getOrCreateGameServerStatusPageSettings: mocks.getSettings,
    updateGameServerStatusPageSettings: mocks.updateSettings,
  }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    copyToClipboard: mocks.copyToClipboard,
    useQuasar: () => ({ notify: mocks.notify, dialog: vi.fn() }),
  }
})

describe('GameServerStatusPageSettingsPanel', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    const authStore = useUserAuthStore()
    authStore.user = create(UserSchema, { id: 'owner-1', userName: 'owner' })
    mocks.copyToClipboard.mockReset()
    mocks.copyToClipboard.mockResolvedValue(undefined)
    mocks.getSettings.mockReset()
    mocks.updateSettings.mockReset()
    mocks.notify.mockReset()
  })

  it('submits the complete owner-level settings in one update', async () => {
    const settings = create(GameServerStatusPageSettingsSchema, {
      ownerId: 'owner-1',
      ownerName: 'owner',
      title: 'Owner fleet',
      publicIdentifier: 'Owner_Page',
      publicPath: '/status/Owner_Page',
      servers: [
        create(GameServerStatusPageSettingsServerSchema, {
          id: 'server-1',
          name: 'Alpha',
          configuredConnectionAddress: '127.0.0.1:25565',
          effectiveConnectionAddress: '127.0.0.1:25565',
        }),
      ],
    })
    mocks.getSettings.mockResolvedValue({ settings })
    mocks.updateSettings.mockImplementation(async (request) => ({
      settings: create(GameServerStatusPageSettingsSchema, {
        ...settings,
        title: request.title,
        enabled: request.enabled,
      }),
    }))

    const wrapper = shallowMount(GameServerStatusPageSettingsPanel)
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      title: string
      enabled: boolean
      addresses: Record<string, string>
      save: () => Promise<void>
    }
    vm.title = 'Public fleet'
    vm.enabled = true
    vm.addresses = { 'server-1': 'play.example.test:25565' }
    await vm.save()

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        ownerId: '',
        title: 'Public fleet',
        publicIdentifier: 'Owner_Page',
        enabled: true,
        connectionAddresses: [
          expect.objectContaining({
            gameServerId: 'server-1',
            publicConnectionAddress: 'play.example.test:25565',
          }),
        ],
      }),
    )
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({ message: 'Status page settings saved' }),
    )

    mocks.updateSettings.mockRejectedValueOnce(
      new ConnectError('Connection address must be a valid host and port.', Code.InvalidArgument),
    )
    vm.addresses = { 'server-1': 'invalid' }
    await vm.save()
    await flushPromises()

    expect(wrapper.text()).toContain('Connection address must be a valid host and port.')
  })

  it('copies the public link with the fallback and reports failures', async () => {
    const settings = create(GameServerStatusPageSettingsSchema, {
      ownerId: 'owner-1',
      publicIdentifier: 'Owner_Page',
      publicPath: '/status/Owner_Page',
    })
    mocks.getSettings.mockResolvedValue({ settings })
    vi.stubGlobal('navigator', {})
    const wrapper = shallowMount(GameServerStatusPageSettingsPanel)
    await flushPromises()
    const vm = wrapper.vm as unknown as { copyPublicLink: () => Promise<void> }

    await vm.copyPublicLink()

    expect(mocks.copyToClipboard).toHaveBeenCalledWith(
      `${window.location.origin}/status/Owner_Page`,
    )
    expect(mocks.notify).toHaveBeenCalledWith({
      type: 'positive',
      message: 'Public link copied',
    })

    mocks.notify.mockReset()
    mocks.copyToClipboard.mockRejectedValueOnce(new Error('copy failed'))
    await vm.copyPublicLink()

    expect(mocks.notify).toHaveBeenCalledWith({
      type: 'negative',
      message: 'Could not copy the public link.',
    })
  })
})
