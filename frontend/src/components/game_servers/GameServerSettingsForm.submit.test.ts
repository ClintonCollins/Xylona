import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, nextTick, type Ref } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { Status } from '@/proto/shared_pb'
import GameServerSettingsForm from './GameServerSettingsForm.vue'

const mocks = vi.hoisted(() => ({
  back: vi.fn(),
  editGameServer: vi.fn(),
  getGameServerAdminInterface: vi.fn(),
  getGameServerEnvironment: vi.fn(),
  getGameServerBackupOverview: vi.fn(),
  getBackupSettings: vi.fn(),
  updateGameServerEnvironment: vi.fn(),
  setGameServerSecretEnv: vi.fn(),
  clearGameServerSecretEnv: vi.fn(),
  setGameServerAdminInterfacePassword: vi.fn(),
  updateBackupSettings: vi.fn(),
  initialize: vi.fn(),
  notify: vi.fn(),
  notifyError: vi.fn(),
  notifySuccess: vi.fn(),
  push: vi.fn(),
  resetSubmissionState: vi.fn(),
  startSubmitting: vi.fn(),
  validateBeforeSave: vi.fn(),
  gameServer: undefined as undefined | Ref<Record<string, unknown>>,
  serverStatus: undefined as Status | undefined,
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      editGameServer: mocks.editGameServer,
      getGameServerAdminInterface: mocks.getGameServerAdminInterface,
      getGameServerEnvironment: mocks.getGameServerEnvironment,
      getGameServerBackupOverview: mocks.getGameServerBackupOverview,
      getBackupSettings: mocks.getBackupSettings,
      updateGameServerEnvironment: mocks.updateGameServerEnvironment,
      setGameServerSecretEnv: mocks.setGameServerSecretEnv,
      clearGameServerSecretEnv: mocks.clearGameServerSecretEnv,
      setGameServerAdminInterfacePassword: mocks.setGameServerAdminInterfacePassword,
      updateBackupSettings: mocks.updateBackupSettings,
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

vi.mock('@/api/notifications', () => ({
  notifyError: mocks.notifyError,
  notifySuccess: mocks.notifySuccess,
}))

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    onBeforeRouteLeave: vi.fn(),
    onBeforeRouteUpdate: vi.fn(),
    useRouter: () => ({
      back: mocks.back,
      push: mocks.push,
    }),
  }
})

vi.mock('./useGameServerFormState', async () => {
  const { ref } = await import('vue')
  return {
    useGameServerFormState: () => {
      mocks.gameServer = ref({
        id: 'server-local-1',
        name: 'Minecraft Server',
        gameId: 'minecraft',
        serverExecutable: 'paper.jar',
        status: mocks.serverStatus,
      })
      return {
        availableGames: [],
        availableIPs: [],
        availableUsers: [],
        formRef: null,
        formSubmitting: false,
        gameRules: [],
        gameServer: mocks.gameServer,
        initialize: mocks.initialize,
        ipRules: [],
        isMinecraftGame: true,
        loading: false,
        maxMemoryModel: 1024,
        maxMemoryRules: [],
        maxMemoryStateMessage: '',
        maxPlayersHint: '',
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
        serverExecutableSummary: 'paper.jar',
        serverNameRules: [],
        setPlayersHint: '',
        setPlayersModel: 0,
        setPlayersRules: [],
        showMaxMemoryStateError: false,
        startSubmitting: mocks.startSubmitting,
        validateBeforeSave: mocks.validateBeforeSave,
      }
    },
  }
})

const GameServerFormShellStub = defineComponent({
  emits: ['cancel', 'save'],
  template: '<button data-testid="save" @click="$emit(\'save\')">Save</button>',
})

async function mountAndRename() {
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
  await flushPromises()
  if (!mocks.gameServer) {
    throw new Error('expected the form state to be created')
  }
  mocks.gameServer.value.name = 'Renamed Server'
  await nextTick()
  return wrapper
}

describe('GameServerSettingsForm submit flow', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => {
      if (typeof mock === 'function' && 'mockReset' in mock) {
        mock.mockReset()
      }
    })
    mocks.serverStatus = undefined
    mocks.validateBeforeSave.mockResolvedValue(true)
    mocks.editGameServer.mockResolvedValue({ gameServer: { id: 'server-local-1' } })
    mocks.getGameServerBackupOverview.mockResolvedValue({
      overview: {
        enabled: true,
        operationsAllowed: true,
        backupsSupported: true,
        canManageSettings: false,
        localServer: true,
        backupDirectoryConfigured: true,
        scheduledBackupCount: 0,
      },
    })
    mocks.getBackupSettings.mockResolvedValue({
      settings: {
        backupsEnabled: true,
        backupsSupported: true,
        backupDirectory: 'C:\\\\backups',
        maxBackups: 10n,
        defaultBackupDirectory: 'C:\\\\default-backups',
      },
    })
    mocks.getGameServerEnvironment.mockResolvedValue({
      serverEnv: [],
      secretEnv: [],
      validationIssues: [],
    })
    mocks.getGameServerAdminInterface.mockResolvedValue({
      adminInterface: { supported: false },
    })
  })

  it('does nothing when no section changed', async () => {
    const wrapper = mount(GameServerSettingsForm, {
      props: { canEditProvisioning: true, gameServerId: 'server-local-1' },
      global: { stubs: { GameServerFormShell: GameServerFormShellStub } },
    })
    await flushPromises()

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.validateBeforeSave).not.toHaveBeenCalled()
    expect(mocks.editGameServer).not.toHaveBeenCalled()
    expect(mocks.notifySuccess).not.toHaveBeenCalled()
  })

  it.each([
    { status: Status.OFFLINE, message: 'Server settings saved.' },
    { status: Status.ONLINE, message: 'Server settings saved. Restart the server to apply.' },
  ])('saves changed server settings without redirecting ($status)', async ({ status, message }) => {
    mocks.serverStatus = status
    const wrapper = await mountAndRename()

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.editGameServer).toHaveBeenCalledTimes(1)
    expect(mocks.push).not.toHaveBeenCalled()
    expect(mocks.notifySuccess).toHaveBeenCalledWith(message)

    // The saved state is the new baseline, so a second Save sends nothing.
    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()
    expect(mocks.editGameServer).toHaveBeenCalledTimes(1)
  })

  it('notifies the user when save fails', async () => {
    mocks.editGameServer.mockRejectedValue(new Error('boom'))
    const wrapper = await mountAndRename()

    await wrapper.get('[data-testid="save"]').trigger('click')
    await flushPromises()

    expect(mocks.push).not.toHaveBeenCalled()
    expect(mocks.notifyError).toHaveBeenCalledWith(
      expect.stringContaining('Not saved: Server settings:'),
    )
    expect(mocks.notifyError).toHaveBeenCalledWith(expect.stringContaining('boom'))
    expect(mocks.resetSubmissionState).toHaveBeenCalledTimes(1)
  })
})
