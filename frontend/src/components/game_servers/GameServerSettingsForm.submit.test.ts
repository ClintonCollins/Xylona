import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GameServerSettingsForm from './GameServerSettingsForm.vue'

const mocks = vi.hoisted(() => ({
  back: vi.fn(),
  editGameServer: vi.fn(),
  initialize: vi.fn(),
  notify: vi.fn(),
  push: vi.fn(),
  resetSubmissionState: vi.fn(),
  startSubmitting: vi.fn(),
  validateBeforeSave: vi.fn(),
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      editGameServer: mocks.editGameServer,
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: mocks.notify,
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      back: mocks.back,
      push: mocks.push,
    }),
  }
})

vi.mock('./game-server-form-state', () => ({
  useGameServerFormState: () => ({
    availableGames: [],
    availableIPs: [],
    availableUsers: [],
    formRef: null,
    formSubmitting: false,
    gameRules: [],
    gameServer: { value: { id: 'server-local-1', name: 'Minecraft Server', gameId: 'minecraft' } },
    initialize: mocks.initialize,
    ipRules: [],
    isMinecraftGame: true,
    loading: false,
    maxMemoryModel: 1024,
    maxMemoryRules: [],
    maxMemoryStateMessage: '',
    maxPlayersModel: 32,
    maxPlayersRules: [],
    nodeRules: [],
    nodes: [],
    onGameSelected: vi.fn(),
    ownerRules: [],
    portModel: 25565,
    portRules: [],
    provisioningCapacity: '32 max / start 0',
    provisioningConnection: '127.0.0.1:25565',
    queryPortModel: 25565,
    queryPortRules: [],
    resetSubmissionState: mocks.resetSubmissionState,
    selectedGameName: 'Minecraft',
    selectedNodeName: 'Local Node',
    selectedOwnerName: 'owner',
    serverNameRules: [],
    setPlayersModel: 0,
    setPlayersRules: [],
    showMaxMemoryStateError: false,
    startSubmitting: mocks.startSubmitting,
    validateBeforeSave: mocks.validateBeforeSave,
  }),
}))

const GameServerFormShellStub = defineComponent({
  emits: ['cancel', 'save'],
  template: '<button data-testid="save" @click="$emit(\'save\')">Save</button>',
})

describe('GameServerSettingsForm submit flow', () => {
  beforeEach(() => {
    mocks.back.mockReset()
    mocks.editGameServer.mockReset()
    mocks.initialize.mockReset()
    mocks.notify.mockReset()
    mocks.push.mockReset()
    mocks.resetSubmissionState.mockReset()
    mocks.startSubmitting.mockReset()
    mocks.validateBeforeSave.mockReset()
    mocks.validateBeforeSave.mockResolvedValue(true)
    mocks.editGameServer.mockResolvedValue({ gameServer: { id: 'server-local-1' } })
  })

  it('notifies success without redirecting after save', async () => {
    const wrapper = mount(GameServerSettingsForm, {
      props: {
        canEditProvisioning: true,
        gameServerId: 'server-local-1',
      },
      global: {
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
        },
      },
    })

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.editGameServer).toHaveBeenCalledTimes(1)
    expect(mocks.push).not.toHaveBeenCalled()
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'positive',
        caption: 'Server settings saved successfully.',
      }),
    )
  })

  it('notifies the user when save fails', async () => {
    mocks.editGameServer.mockRejectedValue(new Error('boom'))

    const wrapper = mount(GameServerSettingsForm, {
      props: {
        canEditProvisioning: true,
        gameServerId: 'server-local-1',
      },
      global: {
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
        },
      },
    })

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.push).not.toHaveBeenCalled()
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-error',
        caption: expect.stringContaining('Failed to save game server:'),
      }),
    )
    expect(mocks.resetSubmissionState).toHaveBeenCalledTimes(1)
  })
})
