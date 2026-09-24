<template>
  <q-dialog v-model="open" aria-labelledby="node-alert-rule-dialog-title" persistent>
    <q-card class="node-alert-dialog">
      <q-card-section>
        <h2 id="node-alert-rule-dialog-title" class="text-h6 q-my-none">Create Alert Rule</h2>
        <p class="node-alert-dialog__help q-mb-none">
          Alerts for <strong>{{ nodeName }}</strong
          >. Rules are listed and edited under
          <router-link to="/notifications">Notifications</router-link> → Alert Rules.
        </p>
      </q-card-section>

      <q-card-section class="q-pt-none">
        <q-banner v-if="channelsError" class="xy-banner-negative q-mb-md" dense role="alert">
          Notification channels could not be loaded. {{ channelsError }}
        </q-banner>
        <q-banner
          v-else-if="channelsLoaded && channels.length === 0"
          class="xy-banner-warning q-mb-md"
          dense>
          No notification channels configured.
          <router-link class="text-weight-bold" to="/notifications">
            Create a notification channel
          </router-link>
          before adding alert rules.
        </q-banner>

        <q-select
          v-model="form.eventType"
          :options="eventTypeOptions"
          class="q-mb-md"
          dense
          emit-value
          label="Event Type"
          map-options
          outlined
          @update:model-value="form.threshold = defaultThreshold($event)" />

        <div class="row q-gutter-sm q-mb-md">
          <q-select
            v-model="form.operator"
            :options="thresholdOperators"
            class="col-4"
            dense
            emit-value
            label="Operator"
            map-options
            outlined />
          <q-input
            v-model.number="form.threshold"
            :rules="[
              (value: unknown) =>
                isFiniteNonNegativeNumber(value) || 'Use a number that is 0 or greater',
            ]"
            class="col"
            dense
            label="Value"
            outlined
            suffix="%"
            type="number" />
        </div>

        <div class="node-alert-dialog__help q-mb-md" role="note">{{ thresholdHelp }}</div>

        <q-expansion-item
          class="node-alert-dialog__advanced q-mb-md"
          dense
          expand-separator
          icon="tune"
          label="Advanced behavior">
          <div class="node-alert-dialog__grid q-pt-sm">
            <q-input
              v-model.number="form.forSeconds"
              :rules="durationRules"
              aria-label="Sustained duration in seconds"
              dense
              hint="0 triggers immediately"
              label="Sustain for"
              min="0"
              outlined
              step="1"
              suffix="seconds"
              type="number" />
            <q-input
              v-model.number="form.recoveryValue"
              :rules="[(value: unknown) => recoveryMessage(value) || true]"
              aria-label="Recovery threshold"
              clearable
              dense
              hint="Optional hysteresis threshold"
              label="Recovery threshold"
              min="0"
              outlined
              suffix="%"
              type="number" />
            <q-input
              v-model.number="form.cooldownSeconds"
              :rules="durationRules"
              aria-label="Cooldown duration in seconds"
              dense
              hint="0 allows the next alert immediately"
              label="Cooldown"
              min="0"
              outlined
              step="1"
              suffix="seconds"
              type="number" />
            <q-input
              v-model.number="form.repeatSeconds"
              :rules="durationRules"
              aria-label="Repeat interval in seconds"
              dense
              hint="0 disables repeat notifications"
              label="Repeat every"
              min="0"
              outlined
              step="1"
              suffix="seconds"
              type="number" />
          </div>
        </q-expansion-item>

        <q-select
          v-model="form.notificationChannelId"
          :options="channels.map((channel) => ({ label: channel.name, value: channel.id }))"
          class="q-mb-md"
          dense
          emit-value
          label="Notification Channel"
          map-options
          outlined />

        <q-toggle v-model="form.enabled" color="positive" label="Enabled" />
      </q-card-section>

      <q-card-actions align="right">
        <q-btn flat label="Cancel" no-caps @click="open = false" />
        <q-btn
          :disable="!canSave"
          :loading="saving"
          color="primary"
          label="Create"
          no-caps
          @click="save" />
      </q-card-actions>
    </q-card>
  </q-dialog>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { computed, ref, watch } from 'vue'

import { connectErrorMessage } from '@/api/connect-errors'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import type { NotificationChannel } from '@/proto/shared_pb'
import { AlertEventType } from '@/proto/shared_pb'
import {
  CreateAlertRuleRequestSchema,
  ListNotificationChannelsRequestSchema,
} from '@/proto/xylona_pb'
import { isFiniteNonNegativeNumber, isNonNegativeInteger } from '@/utils/alert-conditions'
import { GetXylonaClient } from '@/utils/shared'
import { nodeResourceThresholds } from './node-display'

const props = defineProps<{
  nodeId: string
  nodeName: string
}>()

const open = defineModel<boolean>({ required: true })

// ponytail: node-only copy of the threshold form in GameServerAlerts.vue and Notifications.vue;
// move all three onto one shared rule dialog the next time either page's dialog changes.
const eventTypeOptions = [
  { label: 'Node CPU', value: AlertEventType.NODE_CPU_THRESHOLD },
  { label: 'Node Memory', value: AlertEventType.NODE_MEMORY_THRESHOLD },
  { label: 'Node Disk', value: AlertEventType.NODE_DISK_THRESHOLD },
]

const thresholdOperators = [
  { label: '>=', value: '>=' },
  { label: '>', value: '>' },
  { label: '<=', value: '<=' },
  { label: '<', value: '<' },
  { label: '=', value: '==' },
]

const durationRules = [
  (value: unknown) => isNonNegativeInteger(value) || 'Use a whole number of seconds, 0 or greater',
]

// New rules start at the node page's "High" band, so an alert fires where the charts turn amber.
function defaultThreshold(eventType: AlertEventType): number {
  switch (eventType) {
    case AlertEventType.NODE_MEMORY_THRESHOLD:
      return nodeResourceThresholds.memory.warn
    case AlertEventType.NODE_DISK_THRESHOLD:
      return nodeResourceThresholds.disk.warn
    default:
      return nodeResourceThresholds.cpu.warn
  }
}

function defaultForm() {
  return {
    eventType: AlertEventType.NODE_CPU_THRESHOLD,
    operator: '>=',
    threshold: defaultThreshold(AlertEventType.NODE_CPU_THRESHOLD),
    forSeconds: 0,
    recoveryValue: null as number | null,
    cooldownSeconds: 0,
    repeatSeconds: 0,
    notificationChannelId: '',
    enabled: true,
  }
}

const form = ref(defaultForm())
const channels = ref<NotificationChannel[]>([])
const channelsLoaded = ref(false)
const channelsError = ref('')
const saving = ref(false)

const thresholdHelp = computed(() => {
  const cadence = 'Checked every 5 seconds; use Sustain for to ignore short spikes.'
  switch (form.value.eventType) {
    case AlertEventType.NODE_MEMORY_THRESHOLD:
      return `Memory in use as a share of the node's total, the reading in the Memory chart. ${cadence}`
    case AlertEventType.NODE_DISK_THRESHOLD:
      return `Used space as a share of the volume in the Disk chart. ${cadence}`
    default:
      return `Total CPU in use on the node, the reading in the CPU chart. ${cadence}`
  }
})

function recoveryMessage(value: unknown): string {
  if (value === null || value === undefined || value === '') return ''
  if (!isFiniteNonNegativeNumber(value)) return 'Use a number that is 0 or greater'
  const recovery = Number(value)
  const { operator, threshold } = form.value
  if (['>=', '>'].includes(operator) && recovery >= threshold) {
    return 'Recovery must be below the trigger value'
  }
  if (['<=', '<'].includes(operator) && recovery <= threshold) {
    return 'Recovery must be above the trigger value'
  }
  if (operator === '==') return 'Equality alerts cannot use a recovery threshold'
  return ''
}

const canSave = computed(
  () =>
    form.value.notificationChannelId !== '' &&
    isFiniteNonNegativeNumber(form.value.threshold) &&
    isNonNegativeInteger(form.value.forSeconds) &&
    isNonNegativeInteger(form.value.cooldownSeconds) &&
    isNonNegativeInteger(form.value.repeatSeconds) &&
    recoveryMessage(form.value.recoveryValue) === '',
)

function buildCondition(): string {
  const { operator, threshold, forSeconds, recoveryValue, cooldownSeconds, repeatSeconds } =
    form.value
  const condition: Record<string, number | string> = { operator, value: threshold }
  if (forSeconds > 0) condition.for_seconds = forSeconds
  // A cleared q-input yields null or '', both of which mean "no hysteresis".
  if (recoveryValue !== null && String(recoveryValue) !== '') {
    condition.recovery_value = recoveryValue
  }
  if (cooldownSeconds > 0) condition.cooldown_seconds = cooldownSeconds
  if (repeatSeconds > 0) condition.repeat_seconds = repeatSeconds
  return JSON.stringify(condition)
}

async function loadChannels(): Promise<void> {
  try {
    const response = await GetXylonaClient().listNotificationChannels(
      create(ListNotificationChannelsRequestSchema, {}),
    )
    channels.value = response.channels
    channelsError.value = ''
    if (form.value.notificationChannelId === '') {
      form.value.notificationChannelId = response.channels[0]?.id ?? ''
    }
  } catch (unknownErr: unknown) {
    channelsError.value = connectErrorMessage(unknownErr)
  } finally {
    channelsLoaded.value = true
  }
}

async function save(): Promise<void> {
  if (!canSave.value || saving.value) return
  saving.value = true
  try {
    await GetXylonaClient().createAlertRule(
      create(CreateAlertRuleRequestSchema, {
        nodeId: props.nodeId,
        eventType: form.value.eventType,
        notificationChannelId: form.value.notificationChannelId,
        condition: buildCondition(),
        enabled: form.value.enabled,
      }),
    )
  } catch (unknownErr: unknown) {
    // Keep the dialog open so the input survives a failed save.
    notifyConnectError(unknownErr)
    return
  } finally {
    saving.value = false
  }
  notifySuccess('Alert rule created')
  open.value = false
}

// Every opening starts from a fresh form and a fresh channel list, so a channel created
// in another tab since the last visit is offered.
watch(
  open,
  (isOpen) => {
    if (!isOpen) return
    form.value = defaultForm()
    void loadChannels()
  },
  { immediate: true },
)
</script>

<style scoped>
.node-alert-dialog {
  width: min(560px, calc(100vw - 2rem));
  max-height: calc(100vh - 2rem);
}

.node-alert-dialog__help {
  max-width: 65ch;
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: 1.5;
}

.node-alert-dialog__advanced {
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.node-alert-dialog__advanced :deep(.q-expansion-item__container > .q-item) {
  min-height: 2.5rem;
}

.node-alert-dialog__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  gap: var(--xy-space-md);
  padding-inline: var(--xy-space-md);
  padding-bottom: var(--xy-space-md);
}

@media (max-width: 599px) {
  .node-alert-dialog {
    width: calc(100vw - 1rem);
    max-height: calc(100vh - 1rem);
  }

  .node-alert-dialog__grid {
    grid-template-columns: 1fr;
  }
}
</style>
