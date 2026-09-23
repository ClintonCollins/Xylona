import { create } from '@bufbuild/protobuf'
import { Code, ConnectError } from '@connectrpc/connect'
import { flushPromises, shallowMount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { GameServerMapShareSettingsSchema } from '@/proto/xylona_pb'
import GameServerMapShareSettings from './GameServerMapShareSettings.vue'

const mocks = vi.hoisted(() => ({
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
  notify: vi.fn(),
  dialog: vi.fn(),
}))

vi.mock('vue-router', () => ({ onBeforeRouteLeave: vi.fn() }))

vi.mock('@/utils/shared', () => ({
  ConnectErrorToString: (error: Error) => error.message,
  GetXylonaClient: () => ({
    getOrCreateGameServerMapShareSettings: mocks.getSettings,
    updateGameServerMapShareSettings: mocks.updateSettings,
  }),
}))

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    copyToClipboard: vi.fn(),
    useQuasar: () => ({ dialog: mocks.dialog, notify: mocks.notify }),
  }
})

// Every toast goes through the shared helpers; record them on one mock.
vi.mock('@/api/notifications', () => ({
  notifySuccess: mocks.notify,
  notifyError: mocks.notify,
  notifyWarning: mocks.notify,
}))

const settings = create(GameServerMapShareSettingsSchema, {
  gameServerId: 'server-1',
  publicIdentifier: 'Live_Map',
  enabled: true,
  publicPath: '/maps/Live_Map',
})

describe('GameServerMapShareSettings', () => {
  beforeEach(() => {
    mocks.getSettings.mockReset()
    mocks.updateSettings.mockReset()
    mocks.notify.mockReset()
    mocks.dialog.mockReset()
    mocks.getSettings.mockResolvedValue({ settings })
  })

  it('closes a clean dialog at once and asks before discarding edits', async () => {
    mocks.dialog.mockReturnValue({
      onOk() {
        return this
      },
      onCancel(handler: () => void) {
        handler()
        return this
      },
      onDismiss() {
        return this
      },
    })
    const wrapper = shallowMount(GameServerMapShareSettings, {
      props: { gameServerId: 'server-1' },
    })
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      publicIdentifier: string
      requestClose: () => Promise<void>
    }

    await vm.requestClose()
    expect(mocks.dialog).not.toHaveBeenCalled()
    expect(wrapper.emitted('close')).toHaveLength(1)

    vm.publicIdentifier = 'Edited_Map'
    await vm.requestClose()
    expect(mocks.dialog).toHaveBeenCalledTimes(1)
    expect(wrapper.emitted('close')).toHaveLength(1)
  })

  it('saves a renamed or disabled canonical link', async () => {
    mocks.updateSettings.mockImplementation(async (request) => ({
      settings: create(GameServerMapShareSettingsSchema, {
        gameServerId: request.gameServerId,
        publicIdentifier: request.publicIdentifier,
        enabled: request.enabled,
        publicPath: `/maps/${request.publicIdentifier}`,
      }),
    }))
    const wrapper = shallowMount(GameServerMapShareSettings, {
      props: { gameServerId: 'server-1' },
    })
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      publicIdentifier: string
      enabled: boolean
      save: () => Promise<void>
    }

    vm.publicIdentifier = 'Renamed_Map'
    vm.enabled = false
    await vm.save()

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      expect.objectContaining({
        gameServerId: 'server-1',
        publicIdentifier: 'Renamed_Map',
        enabled: false,
      }),
    )
    expect(mocks.notify).toHaveBeenCalledWith('Public map link settings saved.')
  })

  it('blocks invalid identifiers and shows identifier conflicts inline', async () => {
    const wrapper = shallowMount(GameServerMapShareSettings, {
      props: { gameServerId: 'server-1' },
    })
    await flushPromises()
    const vm = wrapper.vm as unknown as {
      publicIdentifier: string
      identifierError: string
      save: () => Promise<void>
    }

    vm.publicIdentifier = 'bad slug'
    await vm.save()
    expect(mocks.updateSettings).not.toHaveBeenCalled()

    vm.publicIdentifier = 'Taken_Map'
    mocks.updateSettings.mockRejectedValueOnce(
      new ConnectError('identifier already exists', Code.AlreadyExists),
    )
    await vm.save()
    await flushPromises()

    expect(vm.identifierError).toBe('This public identifier is unavailable. Choose another.')
    expect(mocks.notify).not.toHaveBeenCalled()
  })
})
