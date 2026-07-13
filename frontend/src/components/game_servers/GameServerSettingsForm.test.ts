import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent } from 'vue'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  BackupSettingsSchema,
  GameSchema,
  GameServerBackupOverviewSchema,
  GameServerSchema,
  IPSchema,
  NodeSchema,
} from '@/proto/shared_pb'
import GameServerSettingsForm from './GameServerSettingsForm.vue'

const mocks = vi.hoisted(() => ({
  getGameServer: vi.fn(),
  getGameServerEnvironment: vi.fn(),
  getGameServerBackupOverview: vi.fn(),
  getBackupSettings: vi.fn(),
  updateGameServerEnvironment: vi.fn(),
  setGameServerSecretEnv: vi.fn(),
  clearGameServerSecretEnv: vi.fn(),
  updateBackupSettings: vi.fn(),
  listGames: vi.fn(),
  listNodes: vi.fn(),
  listUsers: vi.fn(),
  listIPs: vi.fn(),
}))

vi.mock('@/api/game-server-provisioning', () => ({
  getGameServer: mocks.getGameServer,
  listGames: mocks.listGames,
  listNodes: mocks.listNodes,
  listUsers: mocks.listUsers,
  listIPs: mocks.listIPs,
}))

vi.mock('@/utils/shared', async () => {
  const actual = await vi.importActual<typeof import('@/utils/shared')>('@/utils/shared')
  return {
    ...actual,
    GetXylonaClient: () => ({
      getGameServerEnvironment: mocks.getGameServerEnvironment,
      getGameServerBackupOverview: mocks.getGameServerBackupOverview,
      getBackupSettings: mocks.getBackupSettings,
      updateGameServerEnvironment: mocks.updateGameServerEnvironment,
      setGameServerSecretEnv: mocks.setGameServerSecretEnv,
      clearGameServerSecretEnv: mocks.clearGameServerSecretEnv,
      updateBackupSettings: mocks.updateBackupSettings,
      editGameServer: vi.fn(),
    }),
  }
})

vi.mock('quasar', async () => {
  const actual = await vi.importActual<typeof import('quasar')>('quasar')
  return {
    ...actual,
    useQuasar: () => ({
      notify: vi.fn(),
    }),
  }
})

vi.mock('vue-router', async () => {
  const actual = await vi.importActual<typeof import('vue-router')>('vue-router')
  return {
    ...actual,
    useRouter: () => ({
      back: vi.fn(),
      push: vi.fn(),
    }),
  }
})

const QInputStub = defineComponent({
  name: 'QInputStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<div class="q-input-stub" v-bind="$attrs">{{ label }}</div>',
})

const QSelectStub = defineComponent({
  name: 'QSelectStub',
  props: {
    label: {
      type: String,
      default: '',
    },
  },
  template: '<div class="q-select-stub" v-bind="$attrs">{{ label }}</div>',
})

function mountSettingsForm(canEditProvisioning: boolean) {
  return mount(GameServerSettingsForm, {
    props: {
      gameServerId: 'server-local-1',
      canEditProvisioning,
    },
    global: {
      stubs: {
        'q-form': { template: '<form><slot /></form>' },
        'q-banner': { template: '<div v-bind="$attrs"><slot /></div>' },
        'q-input': QInputStub,
        'q-select': QSelectStub,
        'q-btn': {
          template: '<button v-bind="$attrs" :disabled="disable"><slot />{{ label }}</button>',
          props: ['label', 'disable'],
        },
        'q-icon': true,
        'q-spinner-dots': true,
        'q-inner-loading': true,
        'q-toggle': { template: '<input type="checkbox" />' },
        'router-link': { template: '<a><slot /></a>' },
      },
    },
  })
}

describe('GameServerSettingsForm', () => {
  beforeEach(() => {
    vi.stubGlobal(
      'IntersectionObserver',
      class {
        disconnect() {}
        observe() {}
      },
    )

    mocks.getGameServer.mockReset()
    mocks.getGameServerEnvironment.mockReset()
    mocks.getGameServerBackupOverview.mockReset()
    mocks.getBackupSettings.mockReset()
    mocks.updateGameServerEnvironment.mockReset()
    mocks.setGameServerSecretEnv.mockReset()
    mocks.clearGameServerSecretEnv.mockReset()
    mocks.updateBackupSettings.mockReset()
    mocks.listGames.mockReset()
    mocks.listNodes.mockReset()
    mocks.listUsers.mockReset()
    mocks.listIPs.mockReset()

    mocks.listGames.mockResolvedValue({
      games: [
        create(GameSchema, {
          id: 'minecraft',
          name: 'Minecraft',
          defaultPort: 25565n,
          defaultQueryPort: 25565n,
          defaultMaxPlayers: 20n,
        }),
      ],
      options: [{ label: 'Minecraft', value: 'minecraft' }],
    })
    mocks.listNodes.mockResolvedValue([
      create(NodeSchema, {
        id: 'node-local',
        name: 'Local Node',
        local: true,
      }),
    ])
    mocks.listUsers.mockResolvedValue([{ label: 'owner', value: 'user-owner' }])
    mocks.listIPs.mockResolvedValue([
      create(IPSchema, {
        address: '127.0.0.1',
      }),
    ])
    mocks.getGameServerBackupOverview.mockResolvedValue({
      overview: create(GameServerBackupOverviewSchema, {
        enabled: true,
        operationsAllowed: true,
        backupsSupported: true,
        canManageSettings: true,
        localServer: true,
        backupDirectoryConfigured: true,
        scheduledBackupCount: 0,
      }),
    })
    mocks.getBackupSettings.mockResolvedValue({
      settings: create(BackupSettingsSchema, {
        backupsEnabled: true,
        backupsSupported: true,
        backupDirectory: 'C:\\\\backups',
        maxBackups: 10n,
        defaultBackupDirectory: 'C:\\\\default-backups',
      }),
    })
    mocks.getGameServerEnvironment.mockResolvedValue({
      serverEnv: [],
      secretEnv: [],
      validationIssues: [],
    })
    mocks.updateGameServerEnvironment.mockResolvedValue({
      serverEnv: [],
      effectiveEnv: [],
      validationIssues: [],
    })
    mocks.setGameServerSecretEnv.mockResolvedValue({
      secretEnv: [],
      validationIssues: [],
    })
    mocks.clearGameServerSecretEnv.mockResolvedValue({
      secretEnv: [],
      validationIssues: [],
    })
  })

  it('shows full editable provisioning controls for superusers', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GameServerSchema, {
        id: 'server-local-1',
        name: 'Local One',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 25565n,
        queryPort: 25565n,
        setMaxPlayers: 20n,
        maxPlayers: 20n,
        maxMemoryMb: 1024n,
        serverExecutable: 'paper.jar',
      }),
    )

    const wrapper = mountSettingsForm(true)
    await flushPromises()

    expect(wrapper.find('[data-testid="editable-game"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-owner"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-node"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-ip"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-port"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-query-port"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-server-executable"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-max-players"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-max-memory"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="readonly-provisioning"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="backup-settings-enabled"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="save-backup-settings"]').exists()).toBe(true)
  })

  it('requires unsupported enabled backups to be disabled before saving', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GameServerSchema, {
        id: 'server-local-1',
        name: 'Local One',
        userId: 'user-owner',
        gameId: 'minecraft',
        nodeId: 'node-local',
        ip: create(IPSchema, { address: '127.0.0.1' }),
      }),
    )
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: create(GameServerBackupOverviewSchema, {
        enabled: true,
        operationsAllowed: false,
        backupsSupported: false,
        canManageSettings: true,
        disabledReason: 'Backups are not supported on this platform.',
      }),
    })
    mocks.getBackupSettings.mockResolvedValueOnce({
      settings: create(BackupSettingsSchema, {
        backupsEnabled: true,
        backupsSupported: false,
        disabledReason: 'Backups are not supported on this platform.',
        maxBackups: 5n,
      }),
    })

    const wrapper = mountSettingsForm(true)
    await flushPromises()

    expect(wrapper.get('[data-testid="backup-settings-unsupported"]').text()).toContain(
      'Backups are not supported on this platform.',
    )
    expect(
      (wrapper.get('[data-testid="save-backup-settings"]').element as HTMLButtonElement).disabled,
    ).toBe(true)
  })

  it('shows read-only provisioning context and only editable operational fields for non-superusers', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: create(GameServerBackupOverviewSchema, {
        enabled: true,
        operationsAllowed: true,
        backupsSupported: true,
        canManageSettings: false,
        localServer: true,
        backupDirectoryConfigured: true,
        scheduledBackupCount: 0,
      }),
    })

    mocks.getGameServer.mockResolvedValue(
      create(GameServerSchema, {
        id: 'server-local-1',
        name: 'Local One',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 25565n,
        queryPort: 25565n,
        setMaxPlayers: 20n,
        maxPlayers: 20n,
        maxMemoryMb: 1024n,
        serverExecutable: 'paper.jar',
      }),
    )

    const wrapper = mountSettingsForm(false)
    await flushPromises()

    expect(wrapper.find('[data-testid="readonly-provisioning"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="readonly-game"]').text()).toContain('Minecraft')
    expect(wrapper.find('[data-testid="readonly-owner"]').text()).toContain('owner')
    expect(wrapper.find('[data-testid="readonly-node"]').text()).toContain('Local Node')
    expect(wrapper.find('[data-testid="readonly-connection"]').text()).toContain('127.0.0.1:25565')
    expect(wrapper.find('[data-testid="readonly-capacity"]').text()).toContain('20')
    expect(wrapper.find('[data-testid="readonly-max-memory"]').text()).toContain('1024')
    expect(wrapper.find('[data-testid="readonly-server-executable"]').text()).toContain('paper.jar')

    expect(wrapper.find('[data-testid="editable-name"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-set-players"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="editable-game"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-owner"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-node"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-ip"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-port"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-query-port"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-server-executable"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-max-players"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="backup-settings-readonly"]').exists()).toBe(true)
    expect(wrapper.find('[data-testid="save-backup-settings"]').exists()).toBe(false)
  })

  it('marks environment edits as separately unsaved', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GameServerSchema, {
        id: 'server-local-1',
        name: 'Local One',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'minecraft',
        gameName: 'Minecraft',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 25565n,
        queryPort: 25565n,
        setMaxPlayers: 20n,
        maxPlayers: 20n,
        maxMemoryMb: 1024n,
      }),
    )

    const wrapper = mountSettingsForm(true)
    await flushPromises()

    expect(wrapper.find('[data-testid="environment-unsaved-warning"]').exists()).toBe(false)
    expect(wrapper.text()).toContain('Save Environment')

    await wrapper.get('[data-testid="add-environment-row"]').trigger('click')

    expect(wrapper.find('[data-testid="environment-unsaved-warning"]').exists()).toBe(true)
  })

  it('hides minecraft memory context when the server is not minecraft', async () => {
    mocks.getGameServer.mockResolvedValue(
      create(GameServerSchema, {
        id: 'server-local-1',
        name: 'TF2 Server',
        userId: 'user-owner',
        userName: 'owner',
        gameId: 'team-fortress-2',
        gameName: 'Team Fortress 2',
        nodeId: 'node-local',
        nodeName: 'Local Node',
        ip: create(IPSchema, { address: '127.0.0.1' }),
        port: 27015n,
        queryPort: 27016n,
        setMaxPlayers: 24n,
        maxPlayers: 24n,
        maxMemoryMb: 0n,
      }),
    )

    const wrapper = mountSettingsForm(false)
    await flushPromises()

    expect(wrapper.find('[data-testid="readonly-max-memory"]').exists()).toBe(false)
    expect(wrapper.find('[data-testid="editable-max-memory"]').exists()).toBe(false)
  })
})
