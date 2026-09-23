<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import type { Timestamp } from '@bufbuild/protobuf/wkt'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { UploadError, uploadFormData } from '@/utils/upload'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useQuasar } from 'quasar'

import BackupRestoreDialog from '@/components/game_servers/BackupRestoreDialog.vue'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import type {
  BackupProgress,
  BackupSettings,
  GameServer,
  GameServerBackup,
  GameServerBackupOverview,
} from '@/proto/shared_pb'
import {
  BackupProgressOperation,
  BackupProgressPhase,
  BackupRestoreMode,
  BackupSettingsSchema,
  GameServerBackupOverviewSchema,
  GameServerBackupStatus,
  GameServerBackupTriggerSource,
  Status,
} from '@/proto/shared_pb'
import {
  CreateGameServerBackupRequestSchema,
  DeleteGameServerBackupRequestSchema,
  GetBackupSettingsRequestSchema,
  GetGameServerRequestSchema,
  GetGameServerBackupOverviewRequestSchema,
  ListGameServerBackupsRequestSchema,
  RestoreGameServerBackupRequestSchema,
} from '@/proto/xylona_pb'
import { bytesToSize, GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import { connectErrorMessage } from '@/api/connect-errors'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import { formatTimestamp } from '@/utils/format-timestamp'
import { useGameServerName } from './game-server-context'

const $q = useQuasar()
const serverName = useGameServerName()
const route = useRoute()
const gameServerId = computed(() => String(route.params.id ?? ''))
const mobileGrid = computed(() => $q.screen?.lt?.md ?? false)

const loading = ref(true)
const overview = ref<GameServerBackupOverview>(create(GameServerBackupOverviewSchema))
const backupSettings = ref<BackupSettings>(create(BackupSettingsSchema))
const backups = ref<GameServerBackup[]>([])
const creatingBackup = ref(false)
const deletingBackupId = ref('')
const restoringBackupId = ref('')
const restoreTarget = ref<GameServerBackup | null>(null)
const showRestoreDialog = ref(false)
const latestProgress = ref<BackupProgress | null>(null)
const progressByBackupId = ref<Record<string, BackupProgress>>({})
const showUploadBackupDialog = ref(false)
const uploadingBackup = ref(false)
const uploadProgress = ref(0)
const uploadError = ref('')
const uploadFile = ref<File | null>(null)
const loadError = ref('')
// Only the server's name, status and size matter here; a failed load leaves
// the backend's own offline check as the guard.
const gameServer = ref<GameServer | null>(null)
const materializingBackupIds = new Set<string>()

const columns = [
  {
    name: 'archive',
    label: 'Archive',
    field: (row: GameServerBackup) => archiveFileName(row.archivePath),
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'source',
    label: 'Source',
    field: 'triggerSource',
    align: 'left' as const,
  },
  {
    name: 'status',
    label: 'Status',
    field: 'status',
    align: 'left' as const,
  },
  {
    name: 'size',
    label: 'Size',
    field: (row: GameServerBackup) => formatBackupSize(row.sizeBytes),
    align: 'left' as const,
    sortable: true,
    classes: 'xy-num',
  },
  {
    name: 'createdAt',
    label: 'Completed',
    field: (row: GameServerBackup) => formatCompletedTimestamp(row),
    align: 'left' as const,
    sortable: true,
    classes: 'xy-num',
  },
  {
    name: 'actions',
    label: 'Actions',
    field: '',
    align: 'right' as const,
    classes: 'xy-col-actions',
    headerClasses: 'xy-col-actions',
  },
]

const sortedBackups = computed(() => {
  return [...backups.value].sort((left, right) => {
    const leftValue = timestampValue(left.completedAt ?? left.createdAt)
    const rightValue = timestampValue(right.completedAt ?? right.createdAt)
    return rightValue - leftValue
  })
})

const scheduleShortcutLabel = computed(() => {
  if (overview.value.scheduledBackupCount > 0) {
    return 'Manage Scheduled Backups'
  }

  return 'Create Scheduled Backup'
})

const scheduledBackupsLink = computed(() => `/game-servers/${gameServerId.value}/schedules`)
const storedBackups = computed(() =>
  backups.value.filter((backup) => backup.status === GameServerBackupStatus.COMPLETED),
)
const storedBackupCount = computed(() => storedBackups.value.length)
const automatedBackupCount = computed(
  () =>
    storedBackups.value.filter(
      (backup) =>
        backup.triggerSource === GameServerBackupTriggerSource.SCHEDULED && !backup.retentionExempt,
    ).length,
)
const failedBackupCount = computed(
  () => backups.value.filter((backup) => backup.status === GameServerBackupStatus.FAILED).length,
)
const maxBackups = computed(() => Number(backupSettings.value.maxBackups))
const backupStorageSummary = computed(
  () => `${storedBackupCount.value} ${storedBackupCount.value === 1 ? 'backup' : 'backups'} stored`,
)
const backupRetentionSummary = computed(() => {
  if (Number.isFinite(maxBackups.value) && maxBackups.value > 0) {
    return `${automatedBackupCount.value} automated retained · keeps newest ${maxBackups.value} scheduled`
  }

  return `${automatedBackupCount.value} automated retained`
})
const failedBackupSummary = computed(
  () =>
    `${failedBackupCount.value} failed ${failedBackupCount.value === 1 ? 'attempt' : 'attempts'}`,
)
const totalBackupSize = computed(() =>
  storedBackups.value.reduce((totalSize, backup) => totalSize + backup.sizeBytes, 0n),
)
const totalBackupSizeSummary = computed(() => `${formatBackupSize(totalBackupSize.value)} total`)
const createAllowed = computed(() => overview.value.operationsAllowed)
const deleteAllowed = computed(() => true)
const uploadAllowed = computed(() => overview.value.operationsAllowed)
const uploadReady = computed(() => uploadFile.value !== null)
const serverRunning = computed(
  () => gameServer.value !== null && gameServer.value.status !== Status.OFFLINE,
)
const restoreBlockedReason = computed(() =>
  serverRunning.value ? 'Stop the server to restore' : '',
)
// The pre-restore backup runs the same checks as Create Backup.
const safetyBackupUnavailableReason = computed(() =>
  overview.value.operationsAllowed ? '' : stateAlertMessage.value,
)

const showStateAlert = computed(() => {
  // A failed load leaves the empty default overview, which would read as "disabled".
  if (loading.value || loadError.value) {
    return false
  }

  return !overview.value.enabled || !overview.value.operationsAllowed
})

const stateAlertColor = computed(() => {
  if (!overview.value.backupsSupported) {
    return 'warning'
  }

  if (!overview.value.enabled) {
    return 'negative'
  }

  return 'warning'
})

const stateAlertIcon = computed(() => {
  if (!overview.value.backupsSupported) {
    return 'warning'
  }

  if (!overview.value.enabled) {
    return 'block'
  }

  return 'warning'
})

const stateAlertTitle = computed(() => {
  if (!overview.value.backupsSupported) {
    return 'New backups are unavailable'
  }

  if (!overview.value.enabled) {
    return 'Backups are disabled for this server'
  }

  return 'Backup operations are unavailable'
})

const stateAlertMessage = computed(() => {
  if (overview.value.disabledReason) {
    return overview.value.disabledReason
  }
  if (!overview.value.backupDirectoryConfigured) {
    return 'Configure a backup directory before creating new backups.'
  }
  if (!overview.value.enabled) {
    return 'A superuser must re-enable backups before new backups can be created or uploaded.'
  }

  return 'Backup operations are currently unavailable.'
})

const latestProgressTitle = computed(() => {
  if (latestProgress.value?.operation === BackupProgressOperation.RESTORE) {
    return 'Restore running'
  }

  return 'Backup running'
})

const latestProgressPhaseLabel = computed(() => {
  return formatProgressPhase(latestProgress.value?.phase ?? BackupProgressPhase.UNSPECIFIED)
})

const activeBackup = computed(() => {
  const progress = latestProgress.value
  if (!progress) {
    return null
  }

  return backups.value.find((backup) => backup.id === progress.backupId) ?? null
})

const activeProgressArchiveName = computed(() => {
  const backup = activeBackup.value
  if (!backup) {
    return ''
  }

  return archiveFileName(backup.archivePath)
})

const progressIsTerminal = computed(() => {
  const progress = latestProgress.value
  if (!progress) {
    return true
  }

  return (
    progress.phase === BackupProgressPhase.COMPLETE || progress.phase === BackupProgressPhase.FAILED
  )
})

const showLiveProgressStrip = computed(() => {
  return latestProgress.value !== null && !progressIsTerminal.value
})

onMounted(async () => {
  XylonaEventBus.on('gameServerBackupProgress', onBackupProgress)
  XylonaEventBus.on('gameServerStatus', onServerStatus)
  await loadPage()
})

onUnmounted(() => {
  XylonaEventBus.off('gameServerBackupProgress', onBackupProgress)
  XylonaEventBus.off('gameServerStatus', onServerStatus)
})

function onServerStatus(serverID: string, _serverName: string, status: Status): void {
  if (serverID === gameServerId.value && gameServer.value !== null) {
    gameServer.value = { ...gameServer.value, status }
  }
}

async function loadGameServer(): Promise<void> {
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerId.value }),
    )
    gameServer.value = response.gameServer ?? null
  } catch (unknownErr: unknown) {
    console.error(unknownErr)
  }
}

async function loadPage(): Promise<void> {
  void loadGameServer()
  loading.value = true
  loadError.value = ''
  try {
    await Promise.all([loadOverview(), loadBackups(), loadBackupSettings()])
  } catch (unknownErr: unknown) {
    loadError.value = connectErrorMessage(unknownErr)
  } finally {
    loading.value = false
  }
}

async function loadOverview(): Promise<void> {
  const response = await GetXylonaClient().getGameServerBackupOverview(
    create(GetGameServerBackupOverviewRequestSchema, {
      gameServerId: gameServerId.value,
    }),
  )

  if (response.overview) {
    overview.value = response.overview
  } else {
    overview.value = create(GameServerBackupOverviewSchema)
  }
}

async function loadBackups(): Promise<void> {
  const response = await GetXylonaClient().listGameServerBackups(
    create(ListGameServerBackupsRequestSchema, {
      gameServerId: gameServerId.value,
    }),
  )
  const currentBackupIds = new Set(response.backups.map((backup) => backup.id))
  const nextProgressByBackupId = { ...progressByBackupId.value }
  for (const backupId of Object.keys(nextProgressByBackupId)) {
    if (!currentBackupIds.has(backupId)) {
      delete nextProgressByBackupId[backupId]
    }
  }
  backups.value = response.backups.map((backup) => {
    if (backup.status !== GameServerBackupStatus.PENDING) {
      delete nextProgressByBackupId[backup.id]
    }
    return mergeBackupWithProgress(backup)
  })
  progressByBackupId.value = nextProgressByBackupId
  if (latestProgress.value && !currentBackupIds.has(latestProgress.value.backupId)) {
    latestProgress.value = null
  }
}

async function loadBackupSettings(): Promise<void> {
  const response = await GetXylonaClient().getBackupSettings(
    create(GetBackupSettingsRequestSchema, {
      gameServerId: gameServerId.value,
    }),
  )

  if (response.settings) {
    backupSettings.value = create(BackupSettingsSchema, response.settings)
  } else {
    backupSettings.value = create(BackupSettingsSchema)
  }
}

function onBackupProgress(progress: BackupProgress): void {
  if (progress.gameServerId !== gameServerId.value) {
    return
  }

  latestProgress.value = progress
  progressByBackupId.value = {
    ...progressByBackupId.value,
    [progress.backupId]: progress,
  }

  const existingBackup = backups.value.find((backup) => backup.id === progress.backupId)
  if (existingBackup) {
    upsertBackupRow(mergeBackupWithProgress(existingBackup))
  } else if (!materializingBackupIds.has(progress.backupId)) {
    materializingBackupIds.add(progress.backupId)
    void loadBackups().finally(() => {
      materializingBackupIds.delete(progress.backupId)
    })
  }

  if (
    progress.phase === BackupProgressPhase.COMPLETE ||
    progress.phase === BackupProgressPhase.FAILED
  ) {
    void loadOverview()
    void loadBackups()
  }
}

async function createBackup(): Promise<void> {
  // Nothing asks the game to save first, so a running server can change files
  // while they are being copied.
  const runningWarning = serverRunning.value
    ? ' The server is running, so files can change while they are copied. Stop it first for a consistent backup.'
    : ''
  $q.dialog({
    title: 'Create Backup',
    message: `Name this manual backup. Leave it blank to use a timestamped archive name.${runningWarning}`,
    prompt: {
      model: '',
      type: 'text',
      label: 'Backup name (optional)',
      outlined: true,
    },
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'primary', label: 'Start Backup' },
    persistent: true,
  }).onOk(async (backupName: string) => {
    await submitBackupCreate(backupName)
  })
}

async function submitBackupCreate(backupName: string): Promise<void> {
  creatingBackup.value = true
  try {
    const response = await GetXylonaClient().createGameServerBackup(
      create(CreateGameServerBackupRequestSchema, {
        gameServerId: gameServerId.value,
        backupName: backupName.trim(),
      }),
    )
    if (response.backup) {
      upsertBackupRow(response.backup)
    } else {
      await loadBackups()
    }
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr, 'Failed to create backup')
  } finally {
    creatingBackup.value = false
  }
}

function backupDownloadHref(backup: GameServerBackup): string {
  return `/api/backups/download/${encodeURIComponent(gameServerId.value)}/${encodeURIComponent(backup.id)}`
}

function openRestoreDialog(backup: GameServerBackup): void {
  restoreTarget.value = backup
  showRestoreDialog.value = true
}

async function restoreBackup(
  mode: BackupRestoreMode,
  backupCurrentFilesFirst: boolean,
): Promise<void> {
  if (!restoreTarget.value) {
    return
  }

  const backup = restoreTarget.value
  restoringBackupId.value = backup.id
  showRestoreDialog.value = false
  try {
    await GetXylonaClient().restoreGameServerBackup(
      create(RestoreGameServerBackupRequestSchema, {
        gameServerId: gameServerId.value,
        backupId: backup.id,
        restoreMode: mode,
        backupCurrentFilesFirst,
      }),
    )
    await loadBackups()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr, 'Failed to restore backup')
  } finally {
    restoringBackupId.value = ''
    restoreTarget.value = null
  }
}

function confirmDelete(backup: GameServerBackup): void {
  const history = serverName.value ? `${serverName.value}'s backup history` : 'backup history'
  $q.dialog({
    title: 'Delete Backup',
    message: `Delete ${archiveFileName(backup.archivePath)} from disk and ${history}?`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Delete' },
    persistent: true,
  }).onOk(async () => {
    deletingBackupId.value = backup.id
    try {
      await GetXylonaClient().deleteGameServerBackup(
        create(DeleteGameServerBackupRequestSchema, {
          gameServerId: gameServerId.value,
          backupId: backup.id,
        }),
      )
      clearBackupProgress(backup.id)
      backups.value = backups.value.filter((currentBackup) => currentBackup.id !== backup.id)
      await loadBackups()
      notifySuccess('Backup deleted')
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr, 'Failed to delete backup')
    } finally {
      deletingBackupId.value = ''
    }
  })
}

function openUploadBackupDialog(): void {
  resetUploadDialog()
  showUploadBackupDialog.value = true
}

function closeUploadBackupDialog(): void {
  if (uploadingBackup.value) {
    return
  }

  showUploadBackupDialog.value = false
  resetUploadDialog()
}

function resetUploadDialog(): void {
  uploadFile.value = null
  uploadProgress.value = 0
  uploadError.value = ''
}

function onUploadFileChange(): void {
  uploadError.value = ''
  uploadProgress.value = 0
}

async function uploadBackup(): Promise<void> {
  if (!uploadFile.value) {
    uploadError.value = 'Choose a .zip backup archive to upload.'
    return
  }

  uploadingBackup.value = true
  uploadError.value = ''
  uploadProgress.value = 0

  const formData = new FormData()
  formData.append('gameServerId', gameServerId.value)
  formData.append('file', uploadFile.value)

  try {
    await uploadFormData('/api/backups/upload', formData, {
      onProgress: (progressEvent) => {
        const totalBytes = progressEvent.total || uploadFile.value?.size || 0
        if (totalBytes <= 0) {
          return
        }

        uploadProgress.value = Math.max(
          0,
          Math.min(100, Math.round((progressEvent.loaded / totalBytes) * 100)),
        )
      },
    })

    showUploadBackupDialog.value = false
    resetUploadDialog()
    await loadBackups()
    notifySuccess('Backup uploaded')
  } catch (unknownErr: unknown) {
    uploadError.value = formatUploadError(unknownErr)
  } finally {
    uploadingBackup.value = false
  }
}

function formatUploadError(unknownErr: unknown): string {
  if (unknownErr instanceof UploadError && unknownErr.body.trim() !== '') {
    return unknownErr.body
  }

  if (unknownErr instanceof Error && unknownErr.message.trim() !== '') {
    return unknownErr.message
  }

  return 'Failed to upload backup archive.'
}

function archiveFileName(archivePath: string): string {
  if (!archivePath) {
    return 'Unknown archive'
  }

  const segments = archivePath.split(/[\\/]/)
  const fileName = segments[segments.length - 1]
  if (!fileName) {
    return archivePath
  }

  return fileName
}

function timestampValue(ts: Timestamp | undefined): number {
  if (!ts) {
    return 0
  }

  return timestampDate(ts).getTime()
}

function formatBackupSize(sizeBytes: bigint): string {
  const numericSize = Number(sizeBytes)
  if (!Number.isFinite(numericSize)) {
    return `${sizeBytes.toString()} Bytes`
  }

  return bytesToSize(numericSize)
}

function progressForBackup(backup: GameServerBackup): BackupProgress | null {
  return progressByBackupId.value[backup.id] ?? null
}

function isActiveBackup(backup: GameServerBackup): boolean {
  return (
    latestProgress.value?.backupId === backup.id && backup.status === GameServerBackupStatus.PENDING
  )
}

function mergeBackupWithProgress(backup: GameServerBackup): GameServerBackup {
  const progress = progressForBackup(backup)
  if (!progress) {
    return backup
  }

  const mergedBackup = {
    ...backup,
    sizeBytes: progress.sizeBytes > backup.sizeBytes ? progress.sizeBytes : backup.sizeBytes,
  }

  if (backup.status !== GameServerBackupStatus.PENDING) {
    return mergedBackup
  }

  if (progress.phase === BackupProgressPhase.COMPLETE) {
    return {
      ...mergedBackup,
      status: GameServerBackupStatus.COMPLETED,
    }
  }
  if (progress.phase === BackupProgressPhase.FAILED) {
    return {
      ...mergedBackup,
      status: GameServerBackupStatus.FAILED,
    }
  }

  return mergedBackup
}

function upsertBackupRow(backup: GameServerBackup): void {
  const mergedBackup = mergeBackupWithProgress(backup)
  const existingIndex = backups.value.findIndex(
    (currentBackup) => currentBackup.id === mergedBackup.id,
  )
  if (existingIndex === -1) {
    backups.value = [mergedBackup, ...backups.value]
    return
  }

  backups.value = backups.value.map((currentBackup, index) =>
    index === existingIndex ? mergedBackup : currentBackup,
  )
}

function clearBackupProgress(backupId: string): void {
  const nextProgressByBackupId = { ...progressByBackupId.value }
  delete nextProgressByBackupId[backupId]
  progressByBackupId.value = nextProgressByBackupId

  if (latestProgress.value?.backupId === backupId) {
    latestProgress.value = null
  }
}

function formatCompletedTimestamp(backup: GameServerBackup): string {
  if (backup.status === GameServerBackupStatus.PENDING) {
    return '-'
  }

  return formatTimestamp(backup.completedAt, '-')
}

function formatSource(triggerSource: GameServerBackupTriggerSource): string {
  switch (triggerSource) {
    case GameServerBackupTriggerSource.MANUAL:
      return 'Manual'
    case GameServerBackupTriggerSource.SCHEDULED:
      return 'Scheduled'
    default:
      return 'Unknown'
  }
}

function sourceCopy(backup: GameServerBackup): string {
  if (backup.retentionExempt) {
    return 'Never auto-pruned'
  }

  if (backup.triggerSource === GameServerBackupTriggerSource.SCHEDULED) {
    return 'Managed by schedule retention'
  }

  return ''
}

function formatStatus(status: GameServerBackupStatus): string {
  switch (status) {
    case GameServerBackupStatus.PENDING:
      return 'Pending'
    case GameServerBackupStatus.COMPLETED:
      return 'Completed'
    case GameServerBackupStatus.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function statusColor(status: GameServerBackupStatus): string {
  switch (status) {
    case GameServerBackupStatus.PENDING:
      return 'warning'
    case GameServerBackupStatus.COMPLETED:
      return 'positive'
    case GameServerBackupStatus.FAILED:
      return 'negative'
    default:
      return 'grey'
  }
}

function statusLabel(backup: GameServerBackup): string {
  const progress = progressForBackup(backup)
  if (progress && backup.status === GameServerBackupStatus.PENDING) {
    return formatProgressPhase(progress.phase)
  }

  return formatStatus(backup.status)
}

function statusBadgeColor(backup: GameServerBackup): string {
  const progress = progressForBackup(backup)
  if (progress && backup.status === GameServerBackupStatus.PENDING) {
    if (progress.phase === BackupProgressPhase.FAILED) {
      return 'negative'
    }
    if (progress.phase === BackupProgressPhase.COMPLETE) {
      return 'positive'
    }
    return 'warning'
  }

  return statusColor(backup.status)
}

function statusCopy(backup: GameServerBackup): string {
  const progress = progressForBackup(backup)
  if (progress?.message && backup.status === GameServerBackupStatus.PENDING) {
    return progress.message
  }

  return backup.errorMessage ?? ''
}

function formatProgressPhase(phase: BackupProgressPhase): string {
  switch (phase) {
    case BackupProgressPhase.PREPARING:
      return 'Preparing'
    case BackupProgressPhase.ARCHIVING:
      return 'Archiving'
    case BackupProgressPhase.STAGING:
      return 'Staging'
    case BackupProgressPhase.APPLYING:
      return 'Applying'
    case BackupProgressPhase.PRUNING:
      return 'Pruning'
    case BackupProgressPhase.COMPLETE:
      return 'Complete'
    case BackupProgressPhase.FAILED:
      return 'Failed'
    default:
      return 'Waiting'
  }
}
</script>

<template>
  <div class="backups-page xy-page-content">
    <page-header
      subtitle="Archives this server's directory as a .zip. Does not include Xylona's database or encryption key."
      title="Backups">
      <template #actions>
        <q-btn
          :disable="loading || !uploadAllowed"
          :loading="uploadingBackup"
          data-testid="open-upload-backup-dialog"
          flat
          icon="upload_file"
          label="Upload Backup"
          no-caps
          @click="openUploadBackupDialog" />
        <q-btn
          :disable="loading || !createAllowed"
          :loading="creatingBackup"
          color="primary"
          data-testid="open-create-backup-dialog"
          icon="archive"
          label="Create Backup"
          no-caps
          @click="createBackup" />
      </template>
    </page-header>

    <q-banner v-if="loadError" class="xy-banner-negative" dense inline-actions role="alert">
      <template #avatar>
        <q-icon name="sync_problem" />
      </template>
      <div class="backups-page__banner-title">Backups could not be loaded.</div>
      <div class="backups-page__banner-copy">{{ loadError }}</div>
      <template #action>
        <q-btn
          :loading="loading"
          aria-label="Retry loading backups"
          flat
          icon="refresh"
          label="Retry"
          no-caps
          @click="loadPage" />
      </template>
    </q-banner>

    <q-banner v-if="showStateAlert" :class="`xy-banner-${stateAlertColor}`" dense>
      <template #avatar>
        <q-icon :name="stateAlertIcon" />
      </template>
      <div class="backups-page__banner-title">{{ stateAlertTitle }}</div>
      <div class="backups-page__banner-copy">{{ stateAlertMessage }}</div>
    </q-banner>

    <q-card bordered class="backups-page__section" flat>
      <q-card-section class="backups-page__section-header">
        <div>
          <h2 class="xy-section-title">Automated Backups</h2>
          <div class="backups-page__section-copy">
            {{ overview.scheduledBackupCount }} scheduled backup
            {{ overview.scheduledBackupCount === 1 ? 'task' : 'tasks' }} currently target this
            server.
          </div>
        </div>
        <router-link :to="scheduledBackupsLink" class="backups-page__schedule-link">
          <q-btn :label="scheduleShortcutLabel" color="primary" icon="schedule" no-caps outline />
        </router-link>
      </q-card-section>
    </q-card>

    <div v-if="showLiveProgressStrip" class="backups-page__live-strip">
      <div aria-hidden="true" class="backups-page__live-dot"></div>
      <div class="backups-page__live-copy">
        <div class="backups-page__live-title">{{ latestProgressTitle }}</div>
        <div v-if="activeProgressArchiveName" class="backups-page__live-archive">
          {{ activeProgressArchiveName }}
        </div>
        <div class="backups-page__live-meta">
          <span class="backups-page__live-phase">{{ latestProgressPhaseLabel }}</span>
          <span class="backups-page__live-message">
            {{ latestProgress.message || 'Working on the selected backup operation.' }}
          </span>
        </div>
      </div>
    </div>

    <q-card bordered class="backups-page__section" flat>
      <q-card-section class="backups-page__section-header">
        <div>
          <h2 class="xy-section-title">Backup History</h2>
          <div class="backups-page__section-copy">
            Manual and scheduled backups appear here with restore and cleanup actions.
          </div>
          <div class="backups-page__summary-row" data-testid="backup-history-summary">
            <div class="backups-page__summary-pill">{{ backupStorageSummary }}</div>
            <div class="backups-page__summary-pill">{{ backupRetentionSummary }}</div>
            <div
              v-if="failedBackupCount > 0"
              class="backups-page__summary-pill backups-page__summary-pill--failed">
              {{ failedBackupSummary }}
            </div>
            <div class="backups-page__summary-pill">{{ totalBackupSizeSummary }}</div>
          </div>
        </div>
      </q-card-section>
      <q-separator />
      <q-table
        aria-label="Backups"
        :columns="columns"
        :grid="mobileGrid"
        :loading="loading"
        :pagination="{ rowsPerPage: 0 }"
        :rows="sortedBackups"
        class="xy-standalone-table"
        flat
        hide-pagination
        row-key="id">
        <template #no-data>
          <empty-state
            v-if="!loading && !loadError"
            description="Manual and scheduled backups will appear here."
            icon="backup"
            title="No backups yet" />
        </template>
        <template #item="props">
          <q-card
            bordered
            :class="{ 'backups-page__mobile-card--active': isActiveBackup(props.row) }"
            class="backups-page__mobile-card"
            flat>
            <q-card-section class="backups-page__mobile-card-header">
              <div class="backups-page__archive-cell">
                <div class="backups-page__archive-name">
                  {{ archiveFileName(props.row.archivePath) }}
                </div>
                <div class="backups-page__archive-path">{{ props.row.archivePath }}</div>
              </div>
              <q-badge
                :color="statusBadgeColor(props.row)"
                :label="statusLabel(props.row)"
                class="backups-page__status-badge" />
            </q-card-section>

            <q-separator />

            <q-card-section class="backups-page__mobile-fields">
              <div>
                <span>Source</span>
                <strong>{{ formatSource(props.row.triggerSource) }}</strong>
                <em v-if="sourceCopy(props.row)">{{ sourceCopy(props.row) }}</em>
              </div>
              <div>
                <span>Size</span>
                <strong>{{ formatBackupSize(props.row.sizeBytes) }}</strong>
              </div>
              <div>
                <span>Completed</span>
                <strong>{{ formatCompletedTimestamp(props.row) }}</strong>
              </div>
              <div v-if="statusCopy(props.row)">
                <span>Status Detail</span>
                <strong>{{ statusCopy(props.row) }}</strong>
              </div>
            </q-card-section>

            <q-card-actions align="right">
              <q-btn
                v-if="props.row.status === GameServerBackupStatus.COMPLETED"
                :data-testid="`download-backup-${props.row.id}`"
                :href="backupDownloadHref(props.row)"
                flat
                icon="download"
                label="Download"
                no-caps
                rel="noopener"
                target="_blank" />
              <span class="backups-page__restore-action">
                <q-btn
                  :disable="
                    props.row.status !== GameServerBackupStatus.COMPLETED ||
                    restoreBlockedReason !== ''
                  "
                  :loading="restoringBackupId === props.row.id"
                  flat
                  icon="settings_backup_restore"
                  label="Restore"
                  no-caps
                  @click="openRestoreDialog(props.row)" />
                <q-tooltip v-if="restoreBlockedReason">{{ restoreBlockedReason }}</q-tooltip>
              </span>
              <q-btn
                :disable="!deleteAllowed"
                :loading="deletingBackupId === props.row.id"
                color="negative"
                flat
                icon="delete"
                label="Delete"
                no-caps
                @click="confirmDelete(props.row)" />
            </q-card-actions>
          </q-card>
        </template>

        <template #body-cell-archive="props">
          <q-td :class="{ 'backups-page__cell--active': isActiveBackup(props.row) }" :props="props">
            <div
              :class="{ 'backups-page__archive-cell--active': isActiveBackup(props.row) }"
              class="backups-page__archive-cell">
              <div class="backups-page__archive-name">
                {{ archiveFileName(props.row.archivePath) }}
                <q-tooltip v-if="props.row.archivePath" class="font-mono">
                  {{ props.row.archivePath }}
                </q-tooltip>
              </div>
            </div>
          </q-td>
        </template>

        <template #body-cell-source="props">
          <q-td :class="{ 'backups-page__cell--active': isActiveBackup(props.row) }" :props="props">
            <div class="backups-page__meta-cell">
              <q-badge
                :label="formatSource(props.row.triggerSource)"
                class="backups-page__meta-badge"
                color="secondary"
                outline />
              <span v-if="sourceCopy(props.row)" class="backups-page__source-copy">
                {{ sourceCopy(props.row) }}
              </span>
            </div>
          </q-td>
        </template>

        <template #body-cell-status="props">
          <q-td :class="{ 'backups-page__cell--active': isActiveBackup(props.row) }" :props="props">
            <div class="backups-page__status-cell">
              <q-badge
                :color="statusBadgeColor(props.row)"
                :label="statusLabel(props.row)"
                class="backups-page__status-badge" />
              <div v-if="statusCopy(props.row)" class="backups-page__status-copy">
                {{ statusCopy(props.row) }}
              </div>
            </div>
          </q-td>
        </template>

        <template #body-cell-size="props">
          <q-td :class="{ 'backups-page__cell--active': isActiveBackup(props.row) }" :props="props">
            {{ formatBackupSize(props.row.sizeBytes) }}
          </q-td>
        </template>

        <template #body-cell-createdAt="props">
          <q-td :class="{ 'backups-page__cell--active': isActiveBackup(props.row) }" :props="props">
            {{ formatCompletedTimestamp(props.row) }}
          </q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :class="{ 'backups-page__cell--active': isActiveBackup(props.row) }" :props="props">
            <div class="xy-row-actions">
              <q-btn
                v-if="props.row.status === GameServerBackupStatus.COMPLETED"
                :data-testid="`download-backup-${props.row.id}`"
                :href="backupDownloadHref(props.row)"
                aria-label="Download backup"
                dense
                flat
                icon="download"
                rel="noopener"
                round
                target="_blank">
                <q-tooltip>Download</q-tooltip>
              </q-btn>
              <!-- The wrapper keeps the tooltip reachable while the button is disabled. -->
              <span class="backups-page__restore-action">
                <q-btn
                  :aria-label="
                    restoreBlockedReason
                      ? `Restore backup: ${restoreBlockedReason}`
                      : 'Restore backup'
                  "
                  :disable="
                    props.row.status !== GameServerBackupStatus.COMPLETED ||
                    restoreBlockedReason !== ''
                  "
                  :loading="restoringBackupId === props.row.id"
                  dense
                  flat
                  icon="settings_backup_restore"
                  round
                  @click="openRestoreDialog(props.row)" />
                <q-tooltip>{{ restoreBlockedReason || 'Restore' }}</q-tooltip>
              </span>
              <q-btn
                :disable="!deleteAllowed"
                :loading="deletingBackupId === props.row.id"
                aria-label="Delete backup"
                class="text-error-brighter"
                dense
                flat
                icon="delete"
                round
                @click="confirmDelete(props.row)">
                <q-tooltip>Delete</q-tooltip>
              </q-btn>
            </div>
          </q-td>
        </template>
      </q-table>
    </q-card>

    <backup-restore-dialog
      v-model="showRestoreDialog"
      :backup="restoreTarget"
      :backup-unavailable-reason="safetyBackupUnavailableReason"
      :blocked-reason="restoreBlockedReason"
      :current-size-bytes="gameServer?.diskUsageBytes ?? 0n"
      :loading="restoringBackupId !== ''"
      @restore="restoreBackup" />

    <q-dialog
      v-model="showUploadBackupDialog"
      aria-labelledby="upload-backup-dialog-title"
      persistent>
      <q-card class="backups-page__upload-dialog" data-testid="upload-backup-dialog">
        <q-card-section>
          <h2 id="upload-backup-dialog-title" class="xy-section-title">Upload Backup Archive</h2>
          <div class="backups-page__section-copy">
            Import a .zip backup into this server's managed backup history so it can be restored
            later from this page.
          </div>
        </q-card-section>
        <q-card-section class="backups-page__upload-section">
          <q-file
            v-model="uploadFile"
            accept=".zip,application/zip"
            data-testid="upload-backup-file-input"
            :disable="uploadingBackup"
            label="Backup archive (.zip)"
            outlined
            @update:model-value="onUploadFileChange">
            <template #prepend>
              <q-icon name="upload_file" />
            </template>
          </q-file>
          <div v-if="uploadingBackup || uploadProgress > 0" class="backups-page__upload-progress">
            <div class="backups-page__progress-meta">Uploading · {{ uploadProgress }}%</div>
            <q-linear-progress
              :value="Math.max(0, Math.min(1, uploadProgress / 100))"
              color="accent"
              rounded
              size="10px"
              track-color="dark" />
          </div>
          <div v-if="uploadError" class="backups-page__upload-error">{{ uploadError }}</div>
        </q-card-section>
        <q-card-actions align="right">
          <q-btn
            :disable="uploadingBackup"
            data-testid="cancel-upload-backup"
            flat
            label="Cancel"
            no-caps
            @click="closeUploadBackupDialog" />
          <q-btn
            :disable="!uploadReady"
            :loading="uploadingBackup"
            color="primary"
            data-testid="confirm-upload-backup"
            label="Upload Backup"
            no-caps
            @click="uploadBackup" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<style scoped>
.backups-page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.backups-page__banner-title {
  font-family: var(--xy-font-display);
  color: var(--xy-text-primary);
}

.backups-page__banner-copy {
  margin-top: 0.15rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}

.backups-page__section {
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
  border-radius: var(--xy-radius-xl);
}

.backups-page__section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.backups-page__section-copy {
  margin-top: 0.2rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}

.backups-page__schedule-link {
  text-decoration: none;
}

.backups-page__summary-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  margin-top: 0.75rem;
}

.backups-page__summary-pill {
  padding: 0.35rem 0.65rem;
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-pill);
  background: color-mix(in srgb, var(--xy-surface-2) 78%, transparent);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
  line-height: 1.2;
}

.backups-page__summary-pill--failed {
  border-color: var(--xy-danger-border);
  background: var(--xy-danger-bg-faint);
  color: var(--xy-danger-hover);
}

.backups-page__mobile-card {
  width: 100%;
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.backups-page__mobile-card--active {
  border-color: color-mix(in srgb, var(--xy-accent) 34%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-accent) 5%, var(--xy-surface-1));
}

.backups-page__mobile-card-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.backups-page__mobile-fields {
  display: grid;
  gap: var(--xy-space-sm);
}

.backups-page__mobile-fields > div {
  display: grid;
  gap: 0.15rem;
}

.backups-page__mobile-fields span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.backups-page__mobile-fields strong {
  color: var(--xy-text-primary);
  font-weight: 500;
  overflow-wrap: anywhere;
}

.backups-page__mobile-fields em {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
  font-style: normal;
  overflow-wrap: anywhere;
}

.backups-page__live-strip {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  gap: 0.9rem;
  align-items: center;
  padding: 0.9rem 1rem;
  border: 1px solid color-mix(in srgb, var(--xy-accent) 16%, var(--xy-border));
  border-radius: var(--xy-radius-xl);
  background:
    linear-gradient(90deg, color-mix(in srgb, var(--xy-accent) 7%, transparent), transparent 38%),
    color-mix(in srgb, var(--xy-surface-2) 75%, transparent);
}

.backups-page__live-dot {
  width: 0.7rem;
  height: 0.7rem;
  border-radius: var(--xy-radius-pill);
  background: var(--xy-accent);
  box-shadow: 0 0 0 0 color-mix(in srgb, var(--xy-accent) 34%, transparent);
  animation: backups-page-live-pulse 1.6s ease-out infinite;
}

.backups-page__live-copy {
  display: flex;
  flex-direction: column;
  gap: 0.18rem;
  min-width: 0;
}

.backups-page__live-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-base);
  line-height: 1.3;
}

.backups-page__live-archive {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: color-mix(in srgb, var(--xy-text-secondary) 92%, var(--xy-accent) 8%);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-sm);
}

.backups-page__live-meta {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: 0.45rem 0.85rem;
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
  line-height: 1.35;
}

.backups-page__live-phase {
  display: inline-flex;
  align-items: center;
  min-height: 1.55rem;
  padding: 0.1rem 0.5rem;
  border: 1px solid color-mix(in srgb, var(--xy-warning) 28%, var(--xy-border));
  border-radius: var(--xy-radius-pill);
  background: color-mix(in srgb, var(--xy-warning) 10%, transparent);
  color: color-mix(in srgb, var(--xy-warning) 78%, var(--xy-text-primary));
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  white-space: nowrap;
}

.backups-page__live-message {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.backups-page__archive-cell,
.backups-page__status-cell,
.backups-page__meta-cell {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.backups-page__archive-name {
  color: var(--xy-text-primary);
  line-height: 1.35;
}

.backups-page__archive-cell--active {
  color: var(--xy-accent-hover);
}

.backups-page__archive-path,
.backups-page__status-copy,
.backups-page__source-copy {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-sm);
  line-height: 1.35;
}

.backups-page__archive-path {
  overflow-wrap: anywhere;
}

.backups-page__meta-badge,
.backups-page__status-badge {
  align-self: flex-start;
}

/* Opaque, so the sticky actions cell does not show rows scrolling beneath it. */
.backups-page__cell--active {
  background: color-mix(in srgb, var(--xy-accent) 5%, var(--xy-surface-0));
}

.backups-page__status-cell {
  max-width: 24rem;
}

.backups-page__status-copy {
  max-width: 24rem;
  min-width: 0;
  overflow-wrap: anywhere;
  white-space: normal;
}

:deep(.xy-standalone-table th),
:deep(.xy-standalone-table td) {
  vertical-align: top;
}

@keyframes backups-page-live-pulse {
  0% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--xy-accent) 38%, transparent);
    opacity: 0.95;
  }

  70% {
    box-shadow: 0 0 0 10px color-mix(in srgb, var(--xy-accent) 0%, transparent);
    opacity: 1;
  }

  100% {
    box-shadow: 0 0 0 0 color-mix(in srgb, var(--xy-accent) 0%, transparent);
    opacity: 0.95;
  }
}

.backups-page__upload-dialog {
  width: min(32rem, calc(100vw - 2rem));
}

.backups-page__upload-section {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.backups-page__restore-action {
  display: inline-flex;
}

.backups-page__upload-progress {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}

.backups-page__upload-error {
  color: var(--xy-danger);
  font-size: var(--xy-font-size-sm);
}

@media (max-width: 1023px) {
  .backups-page__section-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .backups-page__schedule-link {
    width: 100%;
  }

  .backups-page__live-strip {
    grid-template-columns: 1fr;
    gap: 0.6rem;
  }

  .backups-page__live-dot {
    display: none;
  }

  .backups-page__live-archive {
    white-space: normal;
    overflow: visible;
    text-overflow: clip;
  }

  .backups-page__live-meta {
    grid-template-columns: 1fr;
    gap: 0.3rem;
  }

  .backups-page__live-message {
    white-space: normal;
    overflow: visible;
    text-overflow: clip;
  }
}
</style>
