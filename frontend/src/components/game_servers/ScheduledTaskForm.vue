<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import cronstrue from 'cronstrue'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  CreateScheduledTaskRequestSchema,
  UpdateScheduledTaskRequestSchema,
} from '@/proto/xylona_pb'
import type { ScheduledTask } from '@/proto/shared_pb'

const props = defineProps<{
  showDialog: boolean
  gameServerId: string
  existingTask?: ScheduledTask
}>()

const emit = defineEmits<{
  submit: []
  close: []
}>()

const $q = useQuasar()
const submitting = ref(false)

// ── Task type options ────────────────────────────────────────────────

const taskTypeOptions = [
  { label: 'Restart Server', value: 'restart' },
  { label: 'Backup Server', value: 'backup' },
  { label: 'Console Command', value: 'console_command' },
]

// ── Schedule builder types & options ─────────────────────────────────

type Frequency = 'every_minutes' | 'every_hours' | 'daily' | 'weekly' | 'monthly'

interface ScheduleBuilder {
  frequency: Frequency
  minuteInterval: number
  hourInterval: number
  timeHour: number
  timeMinute: number
  weekdays: number[]
  monthDay: number
}

const frequencyOptions: { label: string; value: Frequency; description: string }[] = [
  {
    label: 'Every N minutes',
    value: 'every_minutes',
    description: 'Run at a fixed minute interval',
  },
  { label: 'Every N hours', value: 'every_hours', description: 'Run at a fixed hourly interval' },
  { label: 'Daily', value: 'daily', description: 'Run once a day at a specific time' },
  { label: 'Weekly', value: 'weekly', description: 'Run on selected days at a specific time' },
  { label: 'Monthly', value: 'monthly', description: 'Run on a specific day each month' },
]

const minuteIntervalOptions = [5, 10, 15, 30].map((v) => ({
  label: `Every ${v} minutes`,
  value: v,
}))

const hourIntervalOptions = [1, 2, 3, 4, 6, 8, 12].map((v) => ({
  label: v === 1 ? 'Every hour' : `Every ${v} hours`,
  value: v,
}))

const hourOptions = Array.from({ length: 24 }, (_, i) => ({
  label: String(i).padStart(2, '0'),
  value: i,
}))

const minuteOptions = Array.from({ length: 12 }, (_, i) => ({
  label: String(i * 5).padStart(2, '0'),
  value: i * 5,
}))

const monthDayOptions = Array.from({ length: 28 }, (_, i) => ({
  label: `${i + 1}`,
  value: i + 1,
}))

const weekdayLabels = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat']

const timezoneOptions = Intl.supportedValuesOf('timeZone')

// ── State ────────────────────────────────────────────────────────────

const form = ref({
  name: '',
  taskType: 'restart',
  consoleCommand: '',
  cronExpression: '0 * * * *',
  timezone: 'UTC',
  enabled: true,
})

const builder = ref<ScheduleBuilder>(defaultBuilder())

const useAdvancedCron = ref(false)

function defaultBuilder(): ScheduleBuilder {
  return {
    frequency: 'every_hours',
    minuteInterval: 5,
    hourInterval: 1,
    timeHour: 3,
    timeMinute: 0,
    weekdays: [],
    monthDay: 1,
  }
}

// ── Computed ─────────────────────────────────────────────────────────

const isEditing = computed(() => !!props.existingTask)

const dialogTitle = computed(() =>
  isEditing.value ? 'Edit Scheduled Task' : 'Create Scheduled Task',
)

const showConsoleCommand = computed(() => form.value.taskType === 'console_command')

const cronFromBuilder = computed((): string => {
  const b = builder.value
  switch (b.frequency) {
    case 'every_minutes':
      return `*/${b.minuteInterval} * * * *`
    case 'every_hours':
      return `0 */${b.hourInterval} * * *`
    case 'daily':
      return `${b.timeMinute} ${b.timeHour} * * *`
    case 'weekly': {
      const days = [...b.weekdays].sort().join(',')
      return `${b.timeMinute} ${b.timeHour} * * ${days || '0'}`
    }
    case 'monthly':
      return `${b.timeMinute} ${b.timeHour} ${b.monthDay} * *`
    default:
      return '0 * * * *'
  }
})

const cronPreview = computed(() => {
  const expr = useAdvancedCron.value ? form.value.cronExpression : cronFromBuilder.value
  if (!expr) return ''
  try {
    return cronstrue.toString(expr)
  } catch {
    return 'Invalid cron expression'
  }
})

const isFormValid = computed(() => {
  if (!form.value.name.trim()) return false
  if (!form.value.timezone) return false
  if (showConsoleCommand.value && !form.value.consoleCommand.trim()) return false

  if (useAdvancedCron.value) {
    if (!form.value.cronExpression.trim()) return false
  } else {
    if (builder.value.frequency === 'weekly' && builder.value.weekdays.length === 0) {
      return false
    }
  }
  return true
})

// ── Builder ↔ Cron sync ─────────────────────────────────────────────

watch(cronFromBuilder, (expr) => {
  if (!useAdvancedCron.value) {
    form.value.cronExpression = expr
  }
})

// When switching TO advanced, pre-fill the cron input
// When switching FROM advanced, attempt to parse back into builder
watch(useAdvancedCron, (advanced, wasAdvanced) => {
  if (advanced && !wasAdvanced) {
    // Visual → Advanced: seed the text input with the builder's cron
    form.value.cronExpression = cronFromBuilder.value
  } else if (!advanced && wasAdvanced) {
    // Advanced → Visual: try to parse the user-entered cron
    const parsed = parseCronToBuilder(form.value.cronExpression)
    if (!parsed) {
      // Can't represent it visually — stay in advanced
      useAdvancedCron.value = true
      $q.notify({
        type: 'xylona-error',
        caption: `This cron expression can't be represented in the visual builder`,
        position: 'top',
        timeout: 4000,
      })
    } else {
      builder.value = parsed
    }
  }
})

// ── Cron parser (cron → builder) ─────────────────────────────────────

const validMinuteIntervals = new Set([5, 10, 15, 30])
const validHourIntervals = new Set([1, 2, 3, 4, 6, 8, 12])

function parseCronToBuilder(expr: string): ScheduleBuilder | null {
  const parts = expr.trim().split(/\s+/)
  if (parts.length !== 5) return null

  const [minute, hour, dom, month, dow] = parts

  // month must be wildcard for all our patterns
  if (month !== '*') return null

  // Pattern: */{N} * * * * → every_minutes
  const everyMinMatch = minute.match(/^\*\/(\d+)$/)
  if (everyMinMatch && hour === '*' && dom === '*' && dow === '*') {
    const n = parseInt(everyMinMatch[1], 10)
    if (validMinuteIntervals.has(n)) {
      return { ...defaultBuilder(), frequency: 'every_minutes', minuteInterval: n }
    }
  }

  // Pattern: 0 */{N} * * * → every_hours
  const everyHrMatch = hour.match(/^\*\/(\d+)$/)
  if (minute === '0' && everyHrMatch && dom === '*' && dow === '*') {
    const n = parseInt(everyHrMatch[1], 10)
    if (validHourIntervals.has(n)) {
      return { ...defaultBuilder(), frequency: 'every_hours', hourInterval: n }
    }
  }

  // Remaining patterns need numeric minute and hour
  const m = parseInt(minute, 10)
  const h = parseInt(hour, 10)
  if (isNaN(m) || isNaN(h) || m < 0 || m > 59 || h < 0 || h > 23) return null
  if (m % 5 !== 0) return null // can't represent in 5-minute-step selects

  // Pattern: {M} {H} * * * → daily
  if (dom === '*' && dow === '*') {
    return { ...defaultBuilder(), frequency: 'daily', timeHour: h, timeMinute: m }
  }

  // Pattern: {M} {H} * * {dow_list} → weekly
  if (dom === '*' && dow !== '*') {
    const days = dow.split(',').map((d) => parseInt(d, 10))
    if (days.some((d) => isNaN(d) || d < 0 || d > 6)) return null
    return {
      ...defaultBuilder(),
      frequency: 'weekly',
      timeHour: h,
      timeMinute: m,
      weekdays: days,
    }
  }

  // Pattern: {M} {H} {D} * * → monthly
  if (dow === '*') {
    const d = parseInt(dom, 10)
    if (isNaN(d) || d < 1 || d > 28) return null
    return {
      ...defaultBuilder(),
      frequency: 'monthly',
      timeHour: h,
      timeMinute: m,
      monthDay: d,
    }
  }

  return null
}

// ── Weekday toggle ───────────────────────────────────────────────────

function toggleWeekday(day: number): void {
  const idx = builder.value.weekdays.indexOf(day)
  if (idx >= 0) {
    builder.value.weekdays.splice(idx, 1)
  } else {
    builder.value.weekdays.push(day)
  }
}

function isWeekdayActive(day: number): boolean {
  return builder.value.weekdays.includes(day)
}

// ── Dialog lifecycle ─────────────────────────────────────────────────

watch(
  () => props.showDialog,
  (visible) => {
    if (!visible) return
    if (props.existingTask) {
      form.value = {
        name: props.existingTask.name,
        taskType: props.existingTask.taskType,
        consoleCommand: props.existingTask.consoleCommand ?? '',
        cronExpression: props.existingTask.cronExpression,
        timezone: props.existingTask.timezone,
        enabled: props.existingTask.enabled,
      }
      const parsed = parseCronToBuilder(props.existingTask.cronExpression)
      if (parsed) {
        builder.value = parsed
        useAdvancedCron.value = false
      } else {
        builder.value = defaultBuilder()
        useAdvancedCron.value = true
      }
    } else {
      form.value = {
        name: '',
        taskType: 'restart',
        consoleCommand: '',
        cronExpression: '0 * * * *',
        timezone: 'UTC',
        enabled: true,
      }
      builder.value = defaultBuilder()
      useAdvancedCron.value = false
    }
  },
)

// ── Submit ────────────────────────────────────────────────────────────

async function handleSubmit(): Promise<void> {
  if (!isFormValid.value || submitting.value) return

  // Ensure cron is synced from builder
  if (!useAdvancedCron.value) {
    form.value.cronExpression = cronFromBuilder.value
  }

  submitting.value = true
  try {
    if (isEditing.value && props.existingTask) {
      const request = create(UpdateScheduledTaskRequestSchema, {
        id: props.existingTask.id,
        name: form.value.name.trim(),
        taskType: form.value.taskType,
        cronExpression: form.value.cronExpression.trim(),
        timezone: form.value.timezone,
        enabled: form.value.enabled,
      })
      if (form.value.taskType === 'console_command') {
        request.consoleCommand = form.value.consoleCommand.trim()
      }
      await GetXylonaClient().updateScheduledTask(request)
      $q.notify({
        type: 'xylona-success',
        caption: 'Scheduled task updated',
        position: 'top',
        timeout: 3000,
      })
    } else {
      const request = create(CreateScheduledTaskRequestSchema, {
        gameServerId: props.gameServerId,
        name: form.value.name.trim(),
        taskType: form.value.taskType,
        cronExpression: form.value.cronExpression.trim(),
        timezone: form.value.timezone,
        enabled: form.value.enabled,
      })
      if (form.value.taskType === 'console_command') {
        request.consoleCommand = form.value.consoleCommand.trim()
      }
      await GetXylonaClient().createScheduledTask(request)
      $q.notify({
        type: 'xylona-success',
        caption: 'Scheduled task created',
        position: 'top',
        timeout: 3000,
      })
    }
    emit('submit')
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <q-dialog :model-value="showDialog" persistent @update:model-value="emit('close')">
    <q-card class="scheduled-task-form-card">
      <q-card-section>
        <div class="text-h6 font-display">{{ dialogTitle }}</div>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <q-input
          v-model="form.name"
          :rules="[(v: string) => !!v.trim() || 'Name is required']"
          class="q-mb-md"
          dense
          label="Name"
          maxlength="80"
          outlined />

        <q-select
          v-model="form.taskType"
          :options="taskTypeOptions"
          class="q-mb-md"
          dense
          emit-value
          label="Task Type"
          map-options
          outlined />

        <q-input
          v-if="showConsoleCommand"
          v-model="form.consoleCommand"
          :rules="[(v: string) => !!v.trim() || 'Console command is required']"
          class="q-mb-md"
          dense
          label="Console Command"
          outlined />

        <!-- ── Schedule Builder Section ────────────────────────── -->
        <div class="schedule-builder-section">
          <div class="schedule-section-label">Schedule</div>

          <!-- Visual Builder -->
          <template v-if="!useAdvancedCron">
            <q-select
              v-model="builder.frequency"
              :options="frequencyOptions"
              class="q-mb-sm"
              dense
              emit-value
              label="Frequency"
              map-options
              outlined />

            <!-- Every N minutes -->
            <q-select
              v-if="builder.frequency === 'every_minutes'"
              v-model="builder.minuteInterval"
              :options="minuteIntervalOptions"
              class="q-mb-sm"
              dense
              emit-value
              label="Interval"
              map-options
              outlined />

            <!-- Every N hours -->
            <q-select
              v-if="builder.frequency === 'every_hours'"
              v-model="builder.hourInterval"
              :options="hourIntervalOptions"
              class="q-mb-sm"
              dense
              emit-value
              label="Interval"
              map-options
              outlined />

            <!-- Time picker (daily / weekly / monthly) -->
            <div
              v-if="
                builder.frequency === 'daily' ||
                builder.frequency === 'weekly' ||
                builder.frequency === 'monthly'
              "
              class="time-row q-mb-sm">
              <q-select
                v-model="builder.timeHour"
                :options="hourOptions"
                class="time-select"
                dense
                emit-value
                label="Hour"
                map-options
                outlined />
              <span class="time-separator">:</span>
              <q-select
                v-model="builder.timeMinute"
                :options="minuteOptions"
                class="time-select"
                dense
                emit-value
                label="Minute"
                map-options
                outlined />
            </div>

            <!-- Weekday picker (weekly) -->
            <div v-if="builder.frequency === 'weekly'" class="weekday-row q-mb-sm">
              <q-btn
                v-for="(label, idx) in weekdayLabels"
                :key="idx"
                :class="{ 'weekday-btn--active': isWeekdayActive(idx) }"
                :color="isWeekdayActive(idx) ? 'primary' : undefined"
                :label="label"
                :outline="!isWeekdayActive(idx)"
                :unelevated="isWeekdayActive(idx)"
                class="weekday-btn"
                dense
                no-caps
                @click="toggleWeekday(idx)" />
            </div>
            <div
              v-if="builder.frequency === 'weekly' && builder.weekdays.length === 0"
              class="text-caption text-negative q-mb-sm">
              Select at least one day
            </div>

            <!-- Day of month (monthly) -->
            <q-select
              v-if="builder.frequency === 'monthly'"
              v-model="builder.monthDay"
              :options="monthDayOptions"
              class="q-mb-sm"
              dense
              emit-value
              label="Day of Month"
              map-options
              outlined />
          </template>

          <!-- Advanced cron input -->
          <template v-else>
            <q-input
              v-model="form.cronExpression"
              :rules="[(v: string) => !!v.trim() || 'Cron expression is required']"
              class="q-mb-sm"
              dense
              hint="5-field format: minute hour day month weekday"
              label="Cron Expression"
              outlined />
          </template>

          <!-- Advanced toggle -->
          <div class="advanced-toggle-row">
            <q-btn
              :icon="useAdvancedCron ? 'tune' : 'code'"
              :label="useAdvancedCron ? 'Use visual builder' : 'Advanced: cron expression'"
              class="advanced-toggle-btn"
              dense
              flat
              no-caps
              size="sm"
              @click="useAdvancedCron = !useAdvancedCron" />
          </div>

          <!-- Schedule preview -->
          <div
            :class="{ 'schedule-preview--invalid': cronPreview === 'Invalid cron expression' }"
            class="schedule-preview">
            <q-icon class="q-mr-xs" name="schedule" size="xs" />
            {{ cronPreview || 'Configure a schedule above' }}
          </div>
        </div>

        <q-select
          v-model="form.timezone"
          :options="timezoneOptions"
          class="q-mb-md"
          dense
          input-debounce="100"
          label="Timezone"
          outlined
          use-input
          @filter="
            (val: string, update: (fn: () => void) => void) => {
              update(() => {
                // Filtering handled by Quasar use-input
              })
            }
          " />

        <q-toggle v-model="form.enabled" color="positive" label="Enabled" />
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="Cancel" no-caps @click="emit('close')" />
        <q-btn
          :disable="!isFormValid"
          :label="isEditing ? 'Save' : 'Create'"
          :loading="submitting"
          color="primary"
          no-caps
          @click="handleSubmit" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<style scoped>
.scheduled-task-form-card {
  min-width: 480px;
  max-width: 560px;
  background-color: var(--xy-surface-1);
}

/* ── Schedule Builder Section ─────────────────────────────────────── */

.schedule-builder-section {
  margin-bottom: 16px;
}

.schedule-section-label {
  font-size: 0.7rem;
  font-weight: 600;
  letter-spacing: 0.1em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  margin-bottom: 10px;
}

/* ── Time Row ─────────────────────────────────────────────────────── */

.time-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.time-select {
  flex: 0 0 100px;
}

.time-separator {
  font-size: 1.25rem;
  font-weight: 600;
  color: var(--xy-text-muted);
  line-height: 1;
  padding-bottom: 2px;
}

/* ── Weekday Selector ─────────────────────────────────────────────── */

.weekday-row {
  display: flex;
  gap: 4px;
}

.weekday-btn {
  min-width: 0;
  width: 50px;
  padding: 4px 6px;
  font-size: 0.78rem;
  border-radius: 6px;
  transition:
    background-color 0.15s ease,
    color 0.15s ease;
}

.weekday-btn:not(.weekday-btn--active) {
  background-color: var(--xy-surface-3);
  color: var(--xy-text-secondary);
}

.weekday-btn:not(.weekday-btn--active):hover {
  background-color: var(--xy-surface-4);
  color: var(--xy-text-primary);
}

/* ── Advanced Toggle ──────────────────────────────────────────────── */

.advanced-toggle-row {
  display: flex;
  margin-bottom: 8px;
}

.advanced-toggle-btn {
  color: var(--xy-text-muted);
  font-size: 0.78rem;
}

.advanced-toggle-btn:hover {
  color: var(--xy-text-primary);
}

/* ── Schedule Preview ─────────────────────────────────────────────── */

.schedule-preview {
  display: flex;
  align-items: center;
  padding: 8px 12px;
  border-radius: 6px;
  background-color: var(--xy-surface-2);
  font-size: 0.82rem;
  color: var(--xy-text-secondary);
  margin-bottom: 16px;
}

.schedule-preview--invalid {
  color: var(--xy-danger);
  background-color: var(--xy-danger-bg);
}
</style>
