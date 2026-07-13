<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Timestamp, timestampDate } from '@bufbuild/protobuf/wkt'
import { useQuasar } from 'quasar'
import dayjs from 'dayjs'
import cronstrue from 'cronstrue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  DeleteScheduledTaskRequestSchema,
  GetGameServerBackupOverviewRequestSchema,
  GetScheduledTaskLogsRequestSchema,
  ListScheduledTasksRequestSchema,
  UpdateScheduledTaskRequestSchema,
} from '@/proto/xylona_pb'
import type { GameServerBackupOverview, ScheduledTask, ScheduledTaskLog } from '@/proto/shared_pb'
import { GameServerBackupOverviewSchema } from '@/proto/shared_pb'
import ScheduledTaskForm from '@/components/game_servers/ScheduledTaskForm.vue'

const $q = useQuasar()
const route = useRoute()
const gameServerId = computed(() => route.params.id as string)
const mobileGrid = computed(() => $q.screen?.lt?.md ?? false)

const loading = ref(true)
const tasks = ref<ScheduledTask[]>([])
const backupOverview = ref<GameServerBackupOverview>(create(GameServerBackupOverviewSchema))
const latestTaskLogsByID = ref<Record<string, ScheduledTaskLog>>({})
const taskLogsByID = ref<Record<string, ScheduledTaskLog[]>>({})
const taskLogErrorsByID = ref<Record<string, string>>({})
const taskLogsLoading = ref(new Set<string>())

// Dialog state
const showFormDialog = ref(false)
const editingTask = ref<ScheduledTask | undefined>(undefined)

const taskTypeLabels: Record<string, string> = {
  restart: 'Restart Server',
  backup: 'Backup Server',
  console_command: 'Console Command',
}

const taskTypeIcons: Record<string, string> = {
  restart: 'restart_alt',
  backup: 'backup',
  console_command: 'terminal',
}

const columns = computed(() => [
  {
    name: 'name',
    label: 'Name',
    field: 'name',
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'taskType',
    label: 'Type',
    field: (row: ScheduledTask) => taskTypeLabels[row.taskType] ?? row.taskType,
    align: 'left' as const,
  },
  {
    name: 'schedule',
    label: 'Schedule',
    field: (row: ScheduledTask) => formatCron(row.cronExpression),
    align: 'left' as const,
  },
  {
    name: 'timezone',
    label: 'Timezone',
    field: 'timezone',
    align: 'left' as const,
  },
  {
    name: 'enabled',
    label: 'Enabled',
    field: 'enabled',
    align: 'center' as const,
  },
  {
    name: 'lastResult',
    label: 'Last Result',
    field: (row: ScheduledTask) => latestLog(row.id)?.status ?? '',
    align: 'left' as const,
  },
  {
    name: 'lastRunAt',
    label: 'Last Run',
    field: (row: ScheduledTask) => formatTimestamp(row.lastRunAt),
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'nextRunAt',
    label: 'Next Run',
    field: (row: ScheduledTask) => formatTimestamp(row.nextRunAt),
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'actions',
    label: 'Actions',
    field: '',
    align: 'right' as const,
  },
])

function formatCron(expression: string): string {
  if (!expression) return '-'
  try {
    return cronstrue.toString(expression)
  } catch {
    return expression
  }
}

function formatTimestamp(ts: Timestamp | undefined): string {
  if (!ts) return '-'
  return dayjs(timestampDate(ts)).format('MM/DD/YYYY HH:mm:ss A')
}

function latestLog(taskID: string): ScheduledTaskLog | undefined {
  return latestTaskLogsByID.value[taskID]
}

function historyCaption(taskID: string): string {
  if (taskLogsLoading.value.has(taskID)) {
    return 'Loading recent runs'
  }
  if (taskLogErrorsByID.value[taskID]) {
    return 'History unavailable'
  }
  const logs = taskLogsByID.value[taskID]
  if (!logs) {
    return latestLog(taskID) ? 'Load recent runs' : 'No runs recorded'
  }
  return `${logs.length} recent runs`
}

function statusLabel(status: string): string {
  if (!status) {
    return 'Unknown'
  }
  return status
    .split('_')
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ')
}

function statusColor(status: string): string {
  switch (status) {
    case 'success':
      return 'positive'
    case 'failed':
    case 'timed_out':
      return 'negative'
    case 'skipped':
      return 'warning'
    default:
      return 'info'
  }
}

function logMessage(log: ScheduledTaskLog, task: ScheduledTask): string {
  if (log.message) {
    return log.message
  }
  if (task.taskType === 'console_command') {
    return 'Execution details are hidden or unavailable.'
  }
  return 'No execution details were recorded.'
}

onMounted(async () => {
  await loadTasks()
})

async function loadTasks(): Promise<void> {
  loading.value = true
  try {
    const request = create(ListScheduledTasksRequestSchema, {
      gameServerId: gameServerId.value,
    })
    const [tasksResponse, overviewResponse] = await Promise.all([
      GetXylonaClient().listScheduledTasks(request),
      GetXylonaClient()
        .getGameServerBackupOverview(
          create(GetGameServerBackupOverviewRequestSchema, {
            gameServerId: gameServerId.value,
          }),
        )
        .catch(() => undefined),
    ])
    tasks.value = tasksResponse.tasks
    backupOverview.value = overviewResponse?.overview
      ? create(GameServerBackupOverviewSchema, overviewResponse.overview)
      : create(GameServerBackupOverviewSchema)
    latestTaskLogsByID.value = {}
    for (const log of tasksResponse.latestLogs) {
      latestTaskLogsByID.value[log.scheduledTaskId] = log
    }
    taskLogsByID.value = {}
    taskLogErrorsByID.value = {}
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    loading.value = false
  }
}

async function loadTaskLogs(taskID: string): Promise<void> {
  taskLogsLoading.value.add(taskID)
  delete taskLogErrorsByID.value[taskID]
  try {
    const response = await GetXylonaClient().getScheduledTaskLogs(
      create(GetScheduledTaskLogsRequestSchema, {
        scheduledTaskId: taskID,
        limit: 10,
      }),
    )
    taskLogsByID.value[taskID] = response.logs
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    taskLogErrorsByID.value[taskID] = ConnectErrorToString(err)
  } finally {
    taskLogsLoading.value.delete(taskID)
  }
}

async function loadTaskLogsIfNeeded(taskID: string): Promise<void> {
  if (
    taskLogsByID.value[taskID] ||
    taskLogsLoading.value.has(taskID) ||
    taskLogErrorsByID.value[taskID]
  ) {
    return
  }
  await loadTaskLogs(taskID)
}

function toggleTaskHistory(taskID: string, rowProps: { expand: boolean }): void {
  rowProps.expand = !rowProps.expand
  if (rowProps.expand) {
    void loadTaskLogsIfNeeded(taskID)
  }
}

function openCreateDialog(): void {
  editingTask.value = undefined
  showFormDialog.value = true
}

function openEditDialog(task: ScheduledTask): void {
  editingTask.value = task
  showFormDialog.value = true
}

function closeFormDialog(): void {
  showFormDialog.value = false
  editingTask.value = undefined
}

async function onFormSubmit(): Promise<void> {
  showFormDialog.value = false
  editingTask.value = undefined
  await loadTasks()
}

async function toggleEnabled(task: ScheduledTask): Promise<void> {
  if (task.taskType === 'backup' && !task.enabled && !backupOverview.value.operationsAllowed) {
    $q.notify({
      type: 'xylona-error',
      caption: backupOverview.value.disabledReason || 'New backup schedules are unavailable.',
      position: 'top',
      timeout: 5000,
    })
    return
  }

  try {
    const request = create(UpdateScheduledTaskRequestSchema, {
      id: task.id,
      name: task.name,
      taskType: task.taskType,
      cronExpression: task.cronExpression,
      timezone: task.timezone,
      enabled: !task.enabled,
    })
    if (task.consoleCommand) {
      request.consoleCommand = task.consoleCommand
    }
    await GetXylonaClient().updateScheduledTask(request)
    await loadTasks()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  }
}

function confirmDelete(task: ScheduledTask): void {
  $q.dialog({
    title: 'Delete Scheduled Task',
    message: `Are you sure you want to delete "${task.name}"?`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Delete' },
    persistent: true,
  }).onOk(async () => {
    try {
      const request = create(DeleteScheduledTaskRequestSchema, {
        id: task.id,
      })
      await GetXylonaClient().deleteScheduledTask(request)
      $q.notify({
        type: 'xylona-success',
        caption: 'Scheduled task deleted',
        position: 'top',
        timeout: 3000,
      })
      await loadTasks()
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: ConnectErrorToString(err),
        position: 'top',
        timeout: 5000,
      })
    }
  })
}
</script>

<template>
  <div class="schedules-page xy-page-content">
    <div class="xy-page-header">
      <h1 class="xy-page-title">Scheduled Tasks</h1>
      <div class="xy-page-actions">
        <q-btn color="primary" icon="add" label="Add Schedule" no-caps @click="openCreateDialog" />
      </div>
    </div>

    <q-table
      :columns="columns"
      :grid="mobileGrid"
      :loading="loading"
      :pagination="{ rowsPerPage: 0 }"
      :rows="tasks"
      class="xy-standalone-table"
      flat
      hide-pagination
      no-data-label="No scheduled tasks yet. Create one to automate server actions."
      row-key="id">
      <template #item="props">
        <q-card bordered class="schedule-card" flat>
          <q-card-section class="schedule-card__header">
            <div>
              <div class="schedule-card__title">{{ props.row.name }}</div>
              <div class="schedule-card__type">
                <q-icon
                  :name="taskTypeIcons[props.row.taskType] ?? 'help'"
                  class="q-mr-xs"
                  size="xs" />
                {{ taskTypeLabels[props.row.taskType] ?? props.row.taskType }}
              </div>
            </div>
            <q-toggle
              :disable="
                props.row.taskType === 'backup' &&
                !props.row.enabled &&
                !backupOverview.operationsAllowed
              "
              :model-value="props.row.enabled"
              color="positive"
              dense
              @update:model-value="toggleEnabled(props.row)" />
          </q-card-section>

          <q-separator />

          <q-card-section class="schedule-card__details">
            <div>
              <span class="schedule-card__label">Schedule</span>
              <span>{{ formatCron(props.row.cronExpression) }}</span>
            </div>
            <div>
              <span class="schedule-card__label">Timezone</span>
              <span>{{ props.row.timezone }}</span>
            </div>
            <div>
              <span class="schedule-card__label">Last Run</span>
              <span>{{ formatTimestamp(props.row.lastRunAt) }}</span>
            </div>
            <div>
              <span class="schedule-card__label">Next Run</span>
              <span>{{ formatTimestamp(props.row.nextRunAt) }}</span>
            </div>
            <div>
              <span class="schedule-card__label">Last Result</span>
              <span v-if="latestLog(props.row.id)" class="schedule-latest-result">
                <q-badge
                  :color="statusColor(latestLog(props.row.id)?.status ?? '')"
                  :label="statusLabel(latestLog(props.row.id)?.status ?? '')" />
                <span>{{ logMessage(latestLog(props.row.id)!, props.row) }}</span>
              </span>
              <span v-else class="text-xy-muted">No runs recorded</span>
            </div>
          </q-card-section>

          <q-expansion-item
            :caption="historyCaption(props.row.id)"
            expand-separator
            icon="history"
            label="Execution history"
            @show="loadTaskLogsIfNeeded(props.row.id)">
            <q-card-section>
              <div
                v-if="taskLogErrorsByID[props.row.id]"
                class="schedule-history-error"
                role="alert">
                <span>{{ taskLogErrorsByID[props.row.id] }}</span>
                <q-btn
                  dense
                  flat
                  icon="refresh"
                  label="Retry"
                  @click="loadTaskLogs(props.row.id)" />
              </div>
              <div
                v-else-if="taskLogsLoading.has(props.row.id)"
                class="schedule-history-state"
                role="status">
                <q-spinner color="primary" size="1.25rem" />
                <span>Loading execution history…</span>
              </div>
              <div
                v-else-if="(taskLogsByID[props.row.id]?.length ?? 0) === 0"
                class="schedule-history-state text-xy-muted">
                No executions have been recorded for this task.
              </div>
              <ol v-else class="schedule-history-list">
                <li v-for="log in taskLogsByID[props.row.id]" :key="log.id">
                  <div class="schedule-history-list__heading">
                    <q-badge :color="statusColor(log.status)" :label="statusLabel(log.status)" />
                    <time
                      :datetime="log.startedAt ? timestampDate(log.startedAt).toISOString() : ''">
                      {{ formatTimestamp(log.startedAt) }}
                    </time>
                  </div>
                  <div class="schedule-history-list__message">
                    {{ logMessage(log, props.row) }}
                  </div>
                </li>
              </ol>
            </q-card-section>
          </q-expansion-item>

          <q-card-actions align="right">
            <q-btn flat icon="edit" label="Edit" no-caps @click="openEditDialog(props.row)" />
            <q-btn
              color="negative"
              flat
              icon="delete"
              label="Delete"
              no-caps
              @click="confirmDelete(props.row)" />
          </q-card-actions>
        </q-card>
      </template>

      <template #body="props">
        <q-tr :props="props">
          <q-td key="name" :props="props">{{ props.row.name }}</q-td>
          <q-td key="taskType" :props="props">
            <div class="row items-center no-wrap">
              <q-icon
                :name="taskTypeIcons[props.row.taskType] ?? 'help'"
                class="q-mr-sm"
                size="xs" />
              {{ taskTypeLabels[props.row.taskType] ?? props.row.taskType }}
            </div>
          </q-td>
          <q-td key="schedule" :props="props">{{ formatCron(props.row.cronExpression) }}</q-td>
          <q-td key="timezone" :props="props">{{ props.row.timezone }}</q-td>
          <q-td key="enabled" :props="props">
            <q-toggle
              :disable="
                props.row.taskType === 'backup' &&
                !props.row.enabled &&
                !backupOverview.operationsAllowed
              "
              :model-value="props.row.enabled"
              color="positive"
              @update:model-value="toggleEnabled(props.row)" />
          </q-td>
          <q-td key="lastResult" :props="props">
            <span v-if="latestLog(props.row.id)" class="schedule-latest-result">
              <q-badge
                :color="statusColor(latestLog(props.row.id)?.status ?? '')"
                :label="statusLabel(latestLog(props.row.id)?.status ?? '')" />
              <span>{{ logMessage(latestLog(props.row.id)!, props.row) }}</span>
            </span>
            <span v-else class="text-xy-muted">No runs recorded</span>
          </q-td>
          <q-td key="lastRunAt" :props="props">{{ formatTimestamp(props.row.lastRunAt) }}</q-td>
          <q-td key="nextRunAt" :props="props">{{ formatTimestamp(props.row.nextRunAt) }}</q-td>
          <q-td key="actions" :props="props">
            <q-btn
              :aria-controls="`task-history-${props.row.id}`"
              :aria-expanded="props.expand"
              :icon="props.expand ? 'expand_less' : 'history'"
              aria-label="Toggle execution history"
              dense
              flat
              size="sm"
              @click="toggleTaskHistory(props.row.id, props)">
              <q-tooltip>{{ props.expand ? 'Hide history' : 'Show history' }}</q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Edit task"
              dense
              flat
              icon="edit"
              size="sm"
              @click="openEditDialog(props.row)">
              <q-tooltip>Edit</q-tooltip>
            </q-btn>
            <q-btn
              aria-label="Delete task"
              color="negative"
              dense
              flat
              icon="delete"
              size="sm"
              @click="confirmDelete(props.row)">
              <q-tooltip>Delete</q-tooltip>
            </q-btn>
          </q-td>
        </q-tr>
        <q-tr v-show="props.expand" :id="`task-history-${props.row.id}`" :props="props">
          <q-td colspan="100%">
            <div v-if="taskLogErrorsByID[props.row.id]" class="schedule-history-error" role="alert">
              <span>{{ taskLogErrorsByID[props.row.id] }}</span>
              <q-btn dense flat icon="refresh" label="Retry" @click="loadTaskLogs(props.row.id)" />
            </div>
            <div
              v-else-if="taskLogsLoading.has(props.row.id)"
              class="schedule-history-state"
              role="status">
              <q-spinner color="primary" size="1.25rem" />
              <span>Loading execution history…</span>
            </div>
            <div
              v-else-if="(taskLogsByID[props.row.id]?.length ?? 0) === 0"
              class="schedule-history-state text-xy-muted">
              No executions have been recorded for this task.
            </div>
            <ol v-else class="schedule-history-list">
              <li v-for="log in taskLogsByID[props.row.id]" :key="log.id">
                <div class="schedule-history-list__heading">
                  <q-badge :color="statusColor(log.status)" :label="statusLabel(log.status)" />
                  <time :datetime="log.startedAt ? timestampDate(log.startedAt).toISOString() : ''">
                    {{ formatTimestamp(log.startedAt) }}
                  </time>
                </div>
                <div class="schedule-history-list__message">
                  {{ logMessage(log, props.row) }}
                </div>
              </li>
            </ol>
          </q-td>
        </q-tr>
      </template>
    </q-table>

    <scheduled-task-form
      :backup-disabled-reason="backupOverview.disabledReason"
      :backup-operations-allowed="backupOverview.operationsAllowed"
      :existing-task="editingTask"
      :game-server-id="gameServerId"
      :show-dialog="showFormDialog"
      @close="closeFormDialog"
      @submit="onFormSubmit" />
  </div>
</template>

<style scoped>
.schedules-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: auto;
}

.schedule-card {
  width: 100%;
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.schedule-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.schedule-card__title {
  color: var(--xy-text-primary);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.schedule-card__type {
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-size: 0.85rem;
}

.schedule-card__details {
  display: grid;
  gap: var(--xy-space-sm);
}

.schedule-card__details > div {
  display: grid;
  gap: 0.15rem;
}

.schedule-card__label {
  color: var(--xy-text-muted);
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.schedule-latest-result {
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-sm);
  min-width: 0;
  max-width: 34rem;
}

.schedule-latest-result > span:last-child {
  overflow-wrap: anywhere;
  white-space: normal;
}

.schedule-history-state,
.schedule-history-error {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 3rem;
  padding: var(--xy-space-sm) var(--xy-space-md);
}

.schedule-history-error {
  justify-content: space-between;
  color: var(--xy-text-primary);
  background: var(--xy-danger-bg);
  border: 1px solid var(--xy-danger-border);
  border-radius: 6px;
  overflow-wrap: anywhere;
}

.schedule-history-list {
  display: grid;
  gap: 0;
  max-width: 75ch;
  margin: 0;
  padding: 0;
  list-style: none;
}

.schedule-history-list li {
  display: grid;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm) 0;
  border-bottom: 1px solid var(--xy-border);
}

.schedule-history-list li:last-child {
  border-bottom: 0;
}

.schedule-history-list__heading {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: 0.8rem;
}

.schedule-history-list__message {
  color: var(--xy-text-primary);
  overflow-wrap: anywhere;
}
</style>
