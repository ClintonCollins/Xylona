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
  ListScheduledTasksRequestSchema,
  UpdateScheduledTaskRequestSchema,
} from '@/proto/xylona_pb'
import type { ScheduledTask } from '@/proto/shared_pb'
import ScheduledTaskForm from '@/components/game_servers/ScheduledTaskForm.vue'

const $q = useQuasar()
const route = useRoute()
const gameServerId = computed(() => route.params.id as string)
const mobileGrid = computed(() => $q.screen?.lt?.md ?? false)

const loading = ref(true)
const tasks = ref<ScheduledTask[]>([])

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

onMounted(async () => {
  await loadTasks()
})

async function loadTasks(): Promise<void> {
  loading.value = true
  try {
    const request = create(ListScheduledTasksRequestSchema, {
      gameServerId: gameServerId.value,
    })
    const response = await GetXylonaClient().listScheduledTasks(request)
    tasks.value = response.tasks
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
          </q-card-section>

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

      <template #body-cell-taskType="props">
        <q-td :props="props">
          <div class="row items-center no-wrap">
            <q-icon :name="taskTypeIcons[props.row.taskType] ?? 'help'" class="q-mr-sm" size="xs" />
            {{ taskTypeLabels[props.row.taskType] ?? props.row.taskType }}
          </div>
        </q-td>
      </template>

      <template #body-cell-enabled="props">
        <q-td :props="props">
          <q-toggle
            :model-value="props.row.enabled"
            color="positive"
            @update:model-value="toggleEnabled(props.row)" />
        </q-td>
      </template>

      <template #body-cell-actions="props">
        <q-td :props="props">
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
      </template>
    </q-table>

    <scheduled-task-form
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
</style>
