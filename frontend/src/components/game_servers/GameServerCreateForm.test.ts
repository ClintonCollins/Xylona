import { flushPromises, shallowMount } from '@vue/test-utils'
import { defineComponent, ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import GameServerCreateForm from './GameServerCreateForm.vue'

const mocks = vi.hoisted(() => ({
  back: vi.fn(),
  createGameServer: vi.fn(),
  initialize: vi.fn(),
  notify: vi.fn(),
  push: vi.fn(),
  resetSubmissionState: vi.fn(),
  startSubmitting: vi.fn(),
  ensurePortAvailabilityBeforeSave: vi.fn(),
  validateBeforeSave: vi.fn(),
}))

const portAvailabilityState = vi.hoisted(() => ({
  blocking: { value: false },
  checking: { value: false },
  message: { value: '' },
  state: { value: 'idle' as 'idle' | 'checking' | 'available' | 'conflict' | 'unavailable' },
  visible: { value: false },
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      createGameServer: mocks.createGameServer,
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
    availableGames: ref([]),
    availableIPs: ref([]),
    availableUsers: ref([]),
    deploymentReady: ref(false),
    deploymentReadyText: ref(''),
    deploymentWarningItems: ref([]),
    formRef: ref(null),
    formSubmitting: ref(false),
    gameRules: [],
    gameServer: ref({ name: 'Minecraft Server', gameId: 'minecraft' }),
    initialize: mocks.initialize,
    ipRules: [],
    isMinecraftGame: ref(true),
    loading: ref(false),
    maxMemoryModel: ref(1024),
    maxMemoryRules: [],
    maxPlayersModel: ref(32),
    maxPlayersRules: [],
    nodeRules: [],
    nodes: ref([]),
    onGameSelected: vi.fn(),
    ownerRules: [],
    portModel: ref(25565),
    portRules: [],
    queryPortModel: ref(25565),
    queryPortRules: [],
    resetSubmissionState: mocks.resetSubmissionState,
    selectedGame: ref({ id: 'minecraft', name: 'Minecraft' }),
    serverNameRules: [],
    setPlayersModel: ref(0),
    setPlayersRules: [],
    startSubmitting: mocks.startSubmitting,
    validateBeforeSave: mocks.validateBeforeSave,
  }),
}))

vi.mock('./game-server-port-availability', () => ({
  useGameServerPortAvailability: () => ({
    ensurePortAvailabilityBeforeSave: mocks.ensurePortAvailabilityBeforeSave,
    portAvailabilityBlocking: portAvailabilityState.blocking,
    portAvailabilityChecking: portAvailabilityState.checking,
    portAvailabilityMessage: portAvailabilityState.message,
    portAvailabilityState: portAvailabilityState.state,
    portAvailabilityVisible: portAvailabilityState.visible,
  }),
}))

const GameServerFormShellStub = defineComponent({
  emits: ['cancel', 'save'],
  template: '<div><slot /><button data-testid="save" @click="$emit(\'save\')">Save</button></div>',
})

const QInputStub = defineComponent({
  name: 'QInput',
  props: {
    error: {
      type: Boolean,
      default: false,
    },
    errorMessage: {
      type: String,
      default: '',
    },
    label: {
      type: String,
      default: '',
    },
  },
  template:
    '<div class="q-input-stub" :data-label="label" :data-error="String(error)" :data-error-message="errorMessage"></div>',
})

const QSelectStub = defineComponent({
  name: 'QSelect',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<div class="q-select-stub" :label="label"></div>',
})

describe('GameServerCreateForm submit flow', () => {
  beforeEach(() => {
    mocks.back.mockReset()
    mocks.createGameServer.mockReset()
    mocks.initialize.mockReset()
    mocks.notify.mockReset()
    mocks.push.mockReset()
    mocks.resetSubmissionState.mockReset()
    mocks.startSubmitting.mockReset()
    mocks.ensurePortAvailabilityBeforeSave.mockReset()
    mocks.validateBeforeSave.mockReset()
    portAvailabilityState.blocking.value = false
    portAvailabilityState.checking.value = false
    portAvailabilityState.message.value = ''
    portAvailabilityState.state.value = 'idle'
    portAvailabilityState.visible.value = false
    mocks.validateBeforeSave.mockResolvedValue(true)
    mocks.ensurePortAvailabilityBeforeSave.mockResolvedValue(true)
    mocks.createGameServer.mockResolvedValue({ gameServer: { id: 'server-created-1' } })
  })

  it('redirects to the new server console after create', async () => {
    const wrapper = shallowMount(GameServerCreateForm, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
          QInput: QInputStub,
          'q-input': QInputStub,
          QSelect: QSelectStub,
          'q-select': QSelectStub,
        },
      },
    })

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.createGameServer).toHaveBeenCalledTimes(1)
    expect(mocks.push).toHaveBeenCalledWith('/game-servers/server-created-1/console')
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'positive',
        caption: 'Game server created successfully.',
      }),
    )
  })

  it('notifies the user when create fails', async () => {
    mocks.createGameServer.mockRejectedValue(new Error('boom'))

    const wrapper = shallowMount(GameServerCreateForm, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
          QInput: QInputStub,
          'q-input': QInputStub,
          QSelect: QSelectStub,
          'q-select': QSelectStub,
        },
      },
    })

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.push).not.toHaveBeenCalled()
    expect(mocks.notify).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'xylona-error',
        caption: expect.stringContaining('Failed to create game server:'),
      }),
    )
    expect(mocks.resetSubmissionState).toHaveBeenCalledTimes(1)
  })

  it('blocks submit when the live port check reports a conflict', async () => {
    mocks.ensurePortAvailabilityBeforeSave.mockResolvedValue(false)

    const wrapper = shallowMount(GameServerCreateForm, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
          QInput: QInputStub,
          'q-input': QInputStub,
          QSelect: QSelectStub,
          'q-select': QSelectStub,
        },
      },
    })

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.createGameServer).not.toHaveBeenCalled()
    expect(mocks.notify).not.toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'positive',
      }),
    )
    expect(mocks.resetSubmissionState).not.toHaveBeenCalled()
  })

  it('shows the live port conflict on the port input instead of a separate status row', () => {
    portAvailabilityState.blocking.value = true
    portAvailabilityState.message.value = 'Port 25565 is already in use on 216.177.177.228.'
    portAvailabilityState.state.value = 'conflict'
    portAvailabilityState.visible.value = true

    const wrapper = shallowMount(GameServerCreateForm, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
          QInput: QInputStub,
          'q-input': QInputStub,
          QSelect: QSelectStub,
          'q-select': QSelectStub,
        },
      },
    })

    expect(wrapper.find('[data-testid="port-availability-status"]').exists()).toBe(false)

    const portInput = wrapper.find('.q-input-stub[data-label="Port *"]')

    expect(portInput.exists()).toBe(true)
    expect(portInput.attributes('data-error')).toBe('true')
    expect(portInput.attributes('data-error-message')).toBe(
      'Port 25565 is already in use on 216.177.177.228.',
    )
  })

  it('shows a server executable field during provisioning', () => {
    const wrapper = shallowMount(GameServerCreateForm, {
      global: {
        renderStubDefaultSlot: true,
        stubs: {
          GameServerFormShell: GameServerFormShellStub,
          QInput: QInputStub,
          'q-input': QInputStub,
          QSelect: QSelectStub,
          'q-select': QSelectStub,
        },
      },
    })

    expect(wrapper.find('.q-input-stub[data-label="Server Executable"]').exists()).toBe(true)
  })
})
