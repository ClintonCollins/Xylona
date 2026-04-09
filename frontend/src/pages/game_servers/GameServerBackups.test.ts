import { create } from '@bufbuild/protobuf'
import { flushPromises, mount } from '@vue/test-utils'
import axios from 'axios'
import { defineComponent } from 'vue'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import {
  type GameServerBackup,
  type GameServerBackupOverview,
  GameServerBackupOverviewSchema,
  GameServerBackupSchema,
  GameServerBackupStatus,
  GameServerBackupTriggerSource,
} from '@/proto/shared_pb'
import GameServerBackups from './GameServerBackups.vue'

const mocks = vi.hoisted(() => ({
  notify: vi.fn(),
  dialog: vi.fn(),
  getGameServerBackupOverview: vi.fn(),
  listGameServerBackups: vi.fn(),
  createGameServerBackup: vi.fn(),
  deleteGameServerBackup: vi.fn(),
  restoreGameServerBackup: vi.fn(),
  eventOn: vi.fn(),
  eventOff: vi.fn(),
}))

vi.mock('axios')

const mockedAxios = vi.mocked(axios, true)

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

vi.mock('@/utils/shared', () => ({
  GetXylonaClient: () => ({
    getGameServerBackupOverview: mocks.getGameServerBackupOverview,
    listGameServerBackups: mocks.listGameServerBackups,
    createGameServerBackup: mocks.createGameServerBackup,
    deleteGameServerBackup: mocks.deleteGameServerBackup,
    restoreGameServerBackup: mocks.restoreGameServerBackup,
  }),
  ConnectErrorToString: (err: unknown) => String(err),
  bytesToSize: (bytes: number) => `${bytes} Bytes`,
  XylonaEventBus: {
    on: mocks.eventOn,
    off: mocks.eventOff,
  },
}))

vi.mock('vue-router', () => ({
  useRoute: () => ({
    params: { id: 'test-server-123' },
    path: '/game-servers/test-server-123/backups',
  }),
}))

const QTableStub = defineComponent({
  name: 'QTableStub',
  props: {
    rows: { type: Array, default: () => [] },
    noDataLabel: { type: String, default: '' },
  },
  template: `<div class="q-table-stub">
    <div v-if="rows.length === 0" class="q-table-no-data">{{ noDataLabel }}</div>
    <div v-for="row in rows" :key="row.id" class="q-table-row">
      <slot name="body-cell-archive" :row="row" />
      <slot name="body-cell-source" :row="row" />
      <slot name="body-cell-status" :row="row" />
      <slot name="body-cell-size" :row="row" />
      <slot name="body-cell-createdAt" :row="row" />
      <slot name="body-cell-actions" :row="row" />
    </div>
  </div>`,
})

const QBtnStub = defineComponent({
  name: 'QBtnStub',
  props: {
    label: { type: String, default: '' },
    icon: { type: String, default: '' },
    disable: { type: Boolean, default: false },
    loading: { type: Boolean, default: false },
  },
  emits: ['click'],
  template:
    '<button v-bind="$attrs" :disabled="disable || loading" @click="$emit(\'click\')">{{ label || icon }}<slot /></button>',
})

const QDialogStub = defineComponent({
  name: 'QDialogStub',
  props: {
    modelValue: { type: Boolean, default: false },
  },
  template: '<div v-if="modelValue" class="q-dialog-stub"><slot /></div>',
})

const QInputStub = defineComponent({
  name: 'QInputStub',
  props: {
    modelValue: { type: String, default: '' },
  },
  emits: ['update:modelValue'],
  template:
    '<input v-bind="$attrs" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />',
})

function makeOverview(overrides: Partial<GameServerBackupOverview> = {}) {
  return create(GameServerBackupOverviewSchema, {
    enabled: true,
    operationsAllowed: true,
    canManageSettings: false,
    localServer: true,
    backupDirectoryConfigured: true,
    scheduledBackupCount: 0,
    disabledReason: '',
    ...overrides,
  })
}

function makeBackup(overrides: Partial<GameServerBackup> = {}) {
  return create(GameServerBackupSchema, {
    id: 'backup-1',
    gameServerId: 'test-server-123',
    nodeId: 'node-local',
    triggerSource: GameServerBackupTriggerSource.MANUAL,
    archivePath: 'C:\\\\backups\\\\server-20260406.zip',
    archiveFormat: 'zip',
    status: GameServerBackupStatus.COMPLETED,
    sizeBytes: 1024n,
    retentionExempt: true,
    createdAt: { seconds: 1712361600n, nanos: 0 },
    completedAt: { seconds: 1712361660n, nanos: 0 },
    ...overrides,
  })
}

function mountBackups() {
  return mount(GameServerBackups, {
    global: {
      stubs: {
        'q-banner': { template: '<div class="q-banner-stub"><slot /><slot name="avatar" /></div>' },
        'q-badge': {
          props: ['label'],
          template: '<span class="q-badge-stub">{{ label }}</span>',
        },
        'q-btn': QBtnStub,
        'q-card': { template: '<div class="q-card-stub"><slot /></div>' },
        'q-card-actions': { template: '<div class="q-card-actions-stub"><slot /></div>' },
        'q-card-section': { template: '<div class="q-card-section-stub"><slot /></div>' },
        'q-dialog': QDialogStub,
        'q-icon': { template: '<i class="q-icon-stub" />' },
        'q-input': QInputStub,
        'q-linear-progress': { template: '<div class="q-linear-progress-stub" />' },
        'q-separator': { template: '<hr />' },
        'q-table': QTableStub,
        'q-td': { template: '<div class="q-td-stub"><slot /></div>' },
        'q-tooltip': { template: '<span class="q-tooltip-stub"><slot /></span>' },
        BackupRestoreDialog: { template: '<div class="backup-restore-dialog-stub" />' },
        'router-link': {
          props: ['to'],
          template: '<a class="router-link-stub" :data-to="String(to)"><slot /></a>',
        },
      },
    },
  })
}

describe('GameServerBackups', () => {
  beforeEach(() => {
    mocks.dialog.mockReturnValue({
      onOk: () => undefined,
    })
    mockedAxios.post.mockReset()
  })

  afterEach(() => {
    Object.values(mocks).forEach((mock) => {
      mock.mockReset()
    })
  })

  it('renders the disabled backup alert and create scheduled shortcut', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: makeOverview({
        enabled: false,
        operationsAllowed: false,
        disabledReason: 'Backups are disabled for this server.',
      }),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({ backups: [] })

    const wrapper = mountBackups()
    await flushPromises()

    expect(wrapper.text()).toContain('Backups are disabled for this server')
    expect(wrapper.text()).toContain('Create Scheduled Backup')
    expect(wrapper.text()).not.toContain('Backup Settings')
  })

  it('renders backup history and the manage scheduled shortcut', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: makeOverview({
        scheduledBackupCount: 2,
      }),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({
      backups: [makeBackup()],
    })

    const wrapper = mountBackups()
    await flushPromises()

    expect(wrapper.text()).toContain('Manage Scheduled Backups')
    expect(wrapper.text()).toContain('server-20260406.zip')
    expect(wrapper.text()).toContain('Manual')
    expect(wrapper.text()).toContain('Completed')
  })

  it('creates a manual backup from the page action', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: makeOverview(),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({ backups: [] })
    mocks.createGameServerBackup.mockResolvedValueOnce({
      backup: makeBackup(),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({
      backups: [makeBackup()],
    })

    const wrapper = mountBackups()
    await flushPromises()

    await wrapper.get('[data-testid="open-create-backup-dialog"]').trigger('click')
    await flushPromises()

    expect(mocks.createGameServerBackup).toHaveBeenCalledTimes(1)
    expect(mocks.createGameServerBackup.mock.calls[0][0]).toMatchObject({
      gameServerId: 'test-server-123',
    })
  })

  it('renders a download link for completed backups', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: makeOverview(),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({
      backups: [makeBackup()],
    })

    const wrapper = mountBackups()
    await flushPromises()

    const downloadLink = wrapper.get('[data-testid="download-backup-backup-1"]')
    expect(downloadLink.attributes('href')).toBe('/api/backups/download/test-server-123/backup-1')
  })

  it('uploads a backup archive and refreshes backup history', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: makeOverview(),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({ backups: [] })
    mockedAxios.post.mockResolvedValueOnce({ status: 201, data: '' } as never)
    mocks.listGameServerBackups.mockResolvedValueOnce({
      backups: [makeBackup()],
    })

    const wrapper = mountBackups()
    await flushPromises()

    await wrapper.get('[data-testid="open-upload-backup-dialog"]').trigger('click')
    const file = new File(['backup-bytes'], 'Friday Night Save.zip', { type: 'application/zip' })
    const input = wrapper.get('[data-testid="upload-backup-file-input"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true,
    })
    await input.trigger('change')
    await wrapper.get('[data-testid="confirm-upload-backup"]').trigger('click')
    await flushPromises()

    expect(mockedAxios.post).toHaveBeenCalledTimes(1)
    expect(mocks.listGameServerBackups).toHaveBeenCalledTimes(2)
  })

  it('surfaces upload failures without closing the upload dialog', async () => {
    mocks.getGameServerBackupOverview.mockResolvedValueOnce({
      overview: makeOverview(),
    })
    mocks.listGameServerBackups.mockResolvedValueOnce({ backups: [] })
    mockedAxios.post.mockRejectedValueOnce({
      response: {
        data: 'invalid zip archive',
      },
    })

    const wrapper = mountBackups()
    await flushPromises()

    await wrapper.get('[data-testid="open-upload-backup-dialog"]').trigger('click')
    const file = new File(['backup-bytes'], 'broken.zip', { type: 'application/zip' })
    const input = wrapper.get('[data-testid="upload-backup-file-input"]')
    Object.defineProperty(input.element, 'files', {
      value: [file],
      configurable: true,
    })
    await input.trigger('change')
    await wrapper.get('[data-testid="confirm-upload-backup"]').trigger('click')
    await flushPromises()

    expect(wrapper.text()).toContain('invalid zip archive')
    expect(wrapper.find('[data-testid="upload-backup-dialog"]').exists()).toBe(true)
  })
})
