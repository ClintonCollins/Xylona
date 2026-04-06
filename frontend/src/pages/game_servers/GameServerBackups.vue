<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import type { Timestamp } from '@bufbuild/protobuf/wkt'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { ConnectError } from '@connectrpc/connect'
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { useQuasar } from 'quasar'
import dayjs from 'dayjs'

import BackupRestoreDialog from '@/components/game_servers/BackupRestoreDialog.vue'
import {
  BackupProgressOperation,
  BackupProgressPhase,
  BackupRestoreMode,
  GameServerBackupOverviewSchema,
  GameServerBackupStatus,
  GameServerBackupTriggerSource,
} from '@/proto/shared_pb'
import type { BackupProgress, GameServerBackup, GameServerBackupOverview } from '@/proto/shared_pb'
import {
  CreateGameServerBackupRequestSchema,
  DeleteGameServerBackupRequestSchema,
  GetGameServerBackupOverviewRequestSchema,
  ListGameServerBackupsRequestSchema,
  RestoreGameServerBackupRequestSchema,
} from '@/proto/xylona_pb'
import { bytesToSize, ConnectErrorToString, GetXylonaClient, XylonaEventBus } from '@/utils/shared'

const $q = useQuasar()
const route = useRoute()
const gameServerId = computed(() => String(route.params.id ?? ''))

const loading = ref(true)
const overview = ref<GameServerBackupOverview>(create(GameServerBackupOverviewSchema))
const backups = ref<GameServerBackup[]>([])
const creatingBackup = ref(false)
const deletingBackupId = ref('')
const restoringBackupId = ref('')
const restoreTarget = ref<GameServerBackup | null>(null)
const showRestoreDialog = ref(false)
const latestProgress = ref<BackupProgress | null>(null)

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
  },
  {
    name: 'createdAt',
    label: 'Completed',
    field: (row: GameServerBackup) => formatTimestamp(row.completedAt ?? row.createdAt),
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'actions',
    label: 'Actions',
    field: '',
    align: 'right' as const,
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
const createAllowed = computed(() => overview.value.operationsAllowed)
const restoreAllowed = computed(() => overview.value.enabled && overview.value.localServer)
const deleteAllowed = computed(() => overview.value.localServer)

const showStateAlert = computed(() => {
  if (loading.value) {
    return false
  }

  return !overview.value.enabled || !overview.value.operationsAllowed
})

const stateAlertColor = computed(() => {
  if (!overview.value.enabled) {
    return 'negative'
  }

  return 'warning'
})

const stateAlertIcon = computed(() => {
  if (!overview.value.enabled) {
    return 'block'
  }

  return 'warning'
})

const stateAlertTitle = computed(() => {
  if (!overview.value.enabled) {
    return 'Backups are disabled for this server'
  }

  return 'Backup operations are unavailable'
})

const stateAlertMessage = computed(() => {
  if (overview.value.disabledReason) {
    return overview.value.disabledReason
  }
  if (!overview.value.localServer) {
    return 'Backups can only be managed on local servers.'
  }
  if (!overview.value.backupDirectoryConfigured) {
    return 'Configure a backup directory before creating new backups.'
  }
  if (!overview.value.enabled) {
    return 'A superuser must re-enable backups before new backup or restore operations can run.'
  }

  return 'Backup operations are currently unavailable.'
})

const latestProgressTitle = computed(() => {
  if (latestProgress.value?.operation === BackupProgressOperation.RESTORE) {
    return 'Restore in Progress'
  }

  return 'Backup in Progress'
})

const latestProgressPhaseLabel = computed(() => {
  return formatProgressPhase(latestProgress.value?.phase ?? BackupProgressPhase.UNSPECIFIED)
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

onMounted(async () => {
  XylonaEventBus.on('gameServerBackupProgress', onBackupProgress)
  await loadPage()
})

onUnmounted(() => {
  XylonaEventBus.off('gameServerBackupProgress', onBackupProgress)
})

async function loadPage(): Promise<void> {
  loading.value = true
  try {
    await loadOverview()
    await loadBackups()
  } catch (unknownErr: unknown) {
    notifyError('Failed to load backups', unknownErr)
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
  backups.value = response.backups
}

function onBackupProgress(progress: BackupProgress): void {
  if (progress.gameServerId !== gameServerId.value) {
    return
  }

  latestProgress.value = progress

  if (
    progress.phase === BackupProgressPhase.COMPLETE ||
    progress.phase === BackupProgressPhase.FAILED
  ) {
    void loadOverview()
    void loadBackups()
  }
}

async function createBackup(): Promise<void> {
  creatingBackup.value = true
  try {
    const response = await GetXylonaClient().createGameServerBackup(
      create(CreateGameServerBackupRequestSchema, {
        gameServerId: gameServerId.value,
      }),
    )
    await loadBackups()
    $q.notify({
      type: 'xylona-success',
      caption: response.backup
        ? `Backup created: ${archiveFileName(response.backup.archivePath)}`
        : 'Backup created',
      position: 'top',
      timeout: 3000,
    })
  } catch (unknownErr: unknown) {
    notifyError('Failed to create backup', unknownErr)
  } finally {
    creatingBackup.value = false
  }
}

function openRestoreDialog(backup: GameServerBackup): void {
  restoreTarget.value = backup
  showRestoreDialog.value = true
}

async function restoreBackup(mode: BackupRestoreMode): Promise<void> {
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
      }),
    )
    await loadBackups()
    $q.notify({
      type: 'xylona-success',
      caption: `Restore completed: ${archiveFileName(backup.archivePath)}`,
      position: 'top',
      timeout: 3000,
    })
  } catch (unknownErr: unknown) {
    notifyError('Failed to restore backup', unknownErr)
  } finally {
    restoringBackupId.value = ''
    restoreTarget.value = null
  }
}

function confirmDelete(backup: GameServerBackup): void {
  $q.dialog({
    title: 'Delete Backup',
    message: `Delete ${archiveFileName(backup.archivePath)} from disk and backup history?`,
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
      await loadBackups()
      $q.notify({
        type: 'xylona-success',
        caption: 'Backup deleted',
        position: 'top',
        timeout: 3000,
      })
    } catch (unknownErr: unknown) {
      notifyError('Failed to delete backup', unknownErr)
    } finally {
      deletingBackupId.value = ''
    }
  })
}

function notifyError(prefix: string, unknownErr: unknown): void {
  const err = ConnectError.from(unknownErr)
  $q.notify({
    type: 'xylona-error',
    caption: `${prefix}: ${ConnectErrorToString(err)}`,
    position: 'top',
    timeout: 5000,
  })
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

function formatTimestamp(ts: Timestamp | undefined): string {
  if (!ts) {
    return '-'
  }

  return dayjs(timestampDate(ts)).format('MM/DD/YYYY HH:mm:ss A')
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
    <div class="xy-page-header">
      <div>
        <div class="xy-page-title">Backups</div>
        <div class="backups-page__subtitle">
          Manual backups, restore history, and the shortcut into scheduled backup automation.
        </div>
      </div>
      <div class="xy-page-actions">
        <q-btn
          color="primary"
          icon="archive"
          label="Create Backup"
          no-caps
          :disable="loading || !createAllowed"
          :loading="creatingBackup"
          @click="createBackup" />
      </div>
    </div>

    <q-banner
      v-if="showStateAlert"
      rounded
      dense
      class="backups-page__banner"
      :class="`backups-page__banner--${stateAlertColor}`">
      <template #avatar>
        <q-icon :name="stateAlertIcon" :color="stateAlertColor" />
      </template>
      <div class="backups-page__banner-title">{{ stateAlertTitle }}</div>
      <div class="backups-page__banner-copy">{{ stateAlertMessage }}</div>
    </q-banner>

    <q-card flat bordered class="backups-page__section">
      <q-card-section class="backups-page__section-header">
        <div>
          <div class="backups-page__section-title">Automated Backups</div>
          <div class="backups-page__section-copy">
            {{ overview.scheduledBackupCount }} scheduled backup
            {{ overview.scheduledBackupCount === 1 ? 'task' : 'tasks' }} currently target this
            server.
          </div>
        </div>
        <router-link class="backups-page__schedule-link" :to="scheduledBackupsLink">
          <q-btn no-caps outline color="primary" icon="schedule" :label="scheduleShortcutLabel" />
        </router-link>
      </q-card-section>
    </q-card>

    <q-card v-if="latestProgress" flat bordered class="backups-page__section">
      <q-card-section class="backups-page__progress">
        <div class="backups-page__section-title">{{ latestProgressTitle }}</div>
        <div class="backups-page__progress-meta">
          {{ latestProgressPhaseLabel }} · {{ latestProgress.percent }}%
        </div>
        <q-linear-progress
          rounded
          size="10px"
          color="accent"
          track-color="dark"
          :value="Math.max(0, Math.min(1, latestProgress.percent / 100))" />
        <div class="backups-page__progress-copy">
          {{ latestProgress.message || 'Working on the selected backup operation.' }}
        </div>
        <div v-if="progressIsTerminal" class="backups-page__progress-terminal">
          Latest update received from the server.
        </div>
      </q-card-section>
    </q-card>

    <q-card flat bordered class="backups-page__section">
      <q-card-section class="backups-page__section-header">
        <div>
          <div class="backups-page__section-title">Backup History</div>
          <div class="backups-page__section-copy">
            Manual and scheduled backups appear here with restore and cleanup actions.
          </div>
        </div>
      </q-card-section>
      <q-separator />
      <q-table
        :rows="sortedBackups"
        :columns="columns"
        row-key="id"
        flat
        :loading="loading"
        :pagination="{ rowsPerPage: 0 }"
        hide-pagination
        class="xy-standalone-table"
        no-data-label="No backups yet. Manual and scheduled backups will appear here.">
        <template #body-cell-archive="props">
          <q-td :props="props">
            <div class="backups-page__archive-cell">
              <div class="backups-page__archive-name">
                {{ archiveFileName(props.row.archivePath) }}
              </div>
              <div class="backups-page__archive-path">{{ props.row.archivePath }}</div>
            </div>
          </q-td>
        </template>

        <template #body-cell-source="props">
          <q-td :props="props">
            <div class="row items-center no-wrap">
              <q-badge outline color="secondary" :label="formatSource(props.row.triggerSource)" />
              <span v-if="props.row.retentionExempt" class="backups-page__retention-copy">
                Manual backups are never auto-pruned
              </span>
            </div>
          </q-td>
        </template>

        <template #body-cell-status="props">
          <q-td :props="props">
            <div class="backups-page__status-cell">
              <q-badge
                :color="statusColor(props.row.status)"
                :label="formatStatus(props.row.status)" />
              <div v-if="props.row.errorMessage" class="backups-page__status-copy">
                {{ props.row.errorMessage }}
              </div>
            </div>
          </q-td>
        </template>

        <template #body-cell-size="props">
          <q-td :props="props">{{ formatBackupSize(props.row.sizeBytes) }}</q-td>
        </template>

        <template #body-cell-createdAt="props">
          <q-td :props="props">{{
            formatTimestamp(props.row.completedAt ?? props.row.createdAt)
          }}</q-td>
        </template>

        <template #body-cell-actions="props">
          <q-td :props="props">
            <q-btn
              flat
              dense
              icon="settings_backup_restore"
              size="sm"
              aria-label="Restore backup"
              :disable="!restoreAllowed || props.row.status !== GameServerBackupStatus.COMPLETED"
              :loading="restoringBackupId === props.row.id"
              @click="openRestoreDialog(props.row)">
              <q-tooltip>Restore</q-tooltip>
            </q-btn>
            <q-btn
              flat
              dense
              icon="delete"
              size="sm"
              color="negative"
              aria-label="Delete backup"
              :disable="!deleteAllowed"
              :loading="deletingBackupId === props.row.id"
              @click="confirmDelete(props.row)">
              <q-tooltip>Delete</q-tooltip>
            </q-btn>
          </q-td>
        </template>
      </q-table>
    </q-card>

    <backup-restore-dialog
      v-model="showRestoreDialog"
      :backup="restoreTarget"
      :loading="restoringBackupId !== ''"
      @restore="restoreBackup" />
  </div>
</template>

<style scoped>
.backups-page {
  display: flex;
  flex-direction: column;
  gap: 1rem;
}

.backups-page__subtitle {
  margin-top: 0.25rem;
  color: var(--xy-text-muted);
}

.backups-page__banner {
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.backups-page__banner--negative {
  background: color-mix(in srgb, var(--xy-danger) 10%, var(--xy-surface-1));
}

.backups-page__banner--warning {
  background: color-mix(in srgb, var(--xy-warning) 12%, var(--xy-surface-1));
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
  border-radius: 1rem;
}

.backups-page__section-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
}

.backups-page__section-title {
  font-family: var(--xy-font-display);
  font-size: 1rem;
  color: var(--xy-text-primary);
}

.backups-page__section-copy {
  margin-top: 0.2rem;
  color: var(--xy-text-muted);
  line-height: 1.5;
}

.backups-page__schedule-link {
  text-decoration: none;
}

.backups-page__progress {
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}

.backups-page__progress-meta,
.backups-page__progress-copy,
.backups-page__progress-terminal {
  color: var(--xy-text-muted);
}

.backups-page__archive-cell,
.backups-page__status-cell {
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}

.backups-page__archive-name {
  color: var(--xy-text-primary);
}

.backups-page__archive-path,
.backups-page__status-copy,
.backups-page__retention-copy {
  color: var(--xy-text-muted);
  font-size: 0.85rem;
}

.backups-page__retention-copy {
  margin-left: 0.5rem;
}

@media (max-width: 900px) {
  .backups-page__section-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .backups-page__schedule-link {
    width: 100%;
  }
}
</style>
