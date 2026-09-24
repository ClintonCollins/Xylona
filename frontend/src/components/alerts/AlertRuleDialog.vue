<template>
  <q-dialog v-model="open" aria-labelledby="alert-rule-dialog-title" persistent>
    <q-card class="alert-rule-dialog">
      <q-card-section>
        <h2 id="alert-rule-dialog-title" class="text-h6 q-my-none">
          {{ rule ? 'Edit Alert Rule' : 'Create Alert Rule' }}
        </h2>
        <p v-if="!rule && nodeId" class="alert-rule-dialog__help q-mb-none">
          Alerts for <strong>{{ nodeName }}</strong
          >. Rules are listed and edited under
          <router-link :to="{ path: '/notifications', query: { tab: 'rules' } }"
            >Notifications → Alert Rules</router-link
          >.
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
          aria-label="Event type"
          class="q-mb-md"
          dense
          emit-value
          label="Event Type"
          map-options
          outlined
          @update:model-value="onEventTypeChange" />

        <template v-if="isThresholdType">
          <div class="row q-gutter-sm q-mb-md">
            <q-select
              v-model="form.operator"
              :options="thresholdOperators"
              aria-label="Threshold operator"
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
              :suffix="thresholdUnit"
              aria-label="Threshold value"
              class="col"
              dense
              label="Value"
              outlined
              type="number" />
          </div>

          <div class="alert-rule-dialog__help q-mb-md" role="note">{{ thresholdHelp }}</div>

          <q-expansion-item
            class="alert-rule-dialog__advanced q-mb-md"
            dense
            expand-separator
            icon="tune"
            label="Advanced behavior">
            <div class="alert-rule-dialog__grid q-pt-sm">
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
                :suffix="thresholdUnit"
                aria-label="Recovery threshold"
                clearable
                dense
                hint="Optional hysteresis threshold"
                label="Recovery threshold"
                min="0"
                outlined
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
        </template>

        <div
          v-if="isStatusChangeType"
          aria-labelledby="alert-rule-status-label"
          class="q-mb-md"
          role="group">
          <div id="alert-rule-status-label" class="text-caption q-mb-xs">Trigger on status:</div>
          <q-checkbox v-model="form.statusOnline" label="Online" />
          <q-checkbox v-model="form.statusOffline" label="Offline" />
          <div v-if="statusSelectionMissing" class="text-caption text-negative" role="alert">
            Select at least one status
          </div>
        </div>

        <q-select
          v-model="form.notificationChannelId"
          :display-value="channelDisplay"
          :loading="!channelsLoaded"
          :options="channels.map((channel) => ({ label: channel.name, value: channel.id }))"
          aria-label="Notification channel"
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
          :label="rule ? 'Save' : 'Create'"
          :loading="saving"
          color="primary"
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
import { nodeDiskVolume, nodeResourceThresholds } from '@/components/nodes/node-display'
import type { AlertRule, NotificationChannel } from '@/proto/shared_pb'
import { AlertEventType } from '@/proto/shared_pb'
import {
  CreateAlertRuleRequestSchema,
  ListNotificationChannelsRequestSchema,
  UpdateAlertRuleRequestSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import {
  isFiniteNonNegativeNumber,
  isNonNegativeInteger,
  readPositiveInteger,
} from '@/utils/alert-conditions'
import { alertEventTypeOptions, isNodeAlertEventType } from '@/utils/alert-scope'
import { GetXylonaClient } from '@/utils/shared'

// The one alert rule form. Editing keeps the rule's own server or node; creating targets
// the given game server (serverId with serverNodeId) or node (nodeId).
const props = defineProps<{
  rule?: AlertRule | null
  serverId?: string
  serverNodeId?: string
  nodeId?: string
  nodeName?: string
}>()

const open = defineModel<boolean>({ required: true })
const emit = defineEmits<{ saved: [] }>()

const authStore = useUserAuthStore()

// A rule stays on its node or server (the backend rejects a switch), and node alerts are
// only sent to superusers, so nobody else is offered a node event type.
const nodeScoped = computed(() =>
  props.rule ? isNodeAlertEventType(props.rule.eventType) : props.nodeId !== undefined,
)
const eventTypeOptions = computed(() => {
  if (nodeScoped.value && authStore.user?.superUser !== true) return []
  return alertEventTypeOptions.filter(
    (option) => isNodeAlertEventType(option.value) === nodeScoped.value,
  )
})

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

const nodeCadence =
  'Checked every 5 seconds while the node answers; use Sustain for to ignore short spikes.'
const thresholdHelps: Partial<Record<AlertEventType, string>> = {
  [AlertEventType.CPU_THRESHOLD]:
    'Process CPU normalized to the node host. Valid samples are evaluated; unavailable samples do not trigger alerts.',
  [AlertEventType.MEMORY_THRESHOLD]:
    'Process RSS as a percentage of total node memory. Valid samples are evaluated; unavailable samples do not trigger alerts.',
  [AlertEventType.DISK_THRESHOLD]:
    'Usage of the volume containing the server working directory. Valid samples are evaluated; unavailable samples do not trigger alerts.',
  [AlertEventType.PLAYER_COUNT_THRESHOLD]:
    'Player count from the game query source. Unavailable or unsupported query samples do not trigger alerts.',
  [AlertEventType.NODE_CPU_THRESHOLD]: `Total CPU in use on the node, the reading in the CPU chart. ${nodeCadence}`,
  [AlertEventType.NODE_MEMORY_THRESHOLD]: `Memory in use as a share of the node's total, the reading in the Memory chart. ${nodeCadence}`,
  [AlertEventType.NODE_DISK_THRESHOLD]: `Used space on ${nodeDiskVolume}, the reading in the Disk chart. ${nodeCadence}`,
}

// New node rules start at the node page's "High" band, so an alert fires where the charts turn amber.
function nodeHighBand(eventType: AlertEventType): number {
  switch (eventType) {
    case AlertEventType.NODE_MEMORY_THRESHOLD:
      return nodeResourceThresholds.memory.warn
    case AlertEventType.NODE_DISK_THRESHOLD:
      return nodeResourceThresholds.disk.warn
    default:
      return nodeResourceThresholds.cpu.warn
  }
}

interface RuleForm {
  eventType: AlertEventType
  notificationChannelId: string
  enabled: boolean
  operator: string
  threshold: number
  forSeconds: number
  recoveryValue: number | null
  cooldownSeconds: number
  repeatSeconds: number
  // Not editable here; kept so an edit doesn't drop it.
  noDataSeconds: number
  statusOnline: boolean
  statusOffline: boolean
}

function newRuleForm(): RuleForm {
  const eventType =
    props.nodeId !== undefined ? AlertEventType.NODE_CPU_THRESHOLD : AlertEventType.CRASH
  return {
    eventType,
    notificationChannelId: '',
    enabled: true,
    operator: '>=',
    threshold: isNodeAlertEventType(eventType) ? nodeHighBand(eventType) : 80,
    forSeconds: 0,
    recoveryValue: null,
    cooldownSeconds: 0,
    repeatSeconds: 0,
    noDataSeconds: 0,
    statusOnline: true,
    statusOffline: true,
  }
}

function parseCondition(condition: string): Record<string, unknown> {
  try {
    const parsed: unknown = JSON.parse(condition)
    return typeof parsed === 'object' && parsed !== null ? (parsed as Record<string, unknown>) : {}
  } catch {
    // Empty or unreadable conditions keep the defaults.
    return {}
  }
}

function formFromRule(rule: AlertRule): RuleForm {
  const ruleForm: RuleForm = {
    ...newRuleForm(),
    eventType: rule.eventType,
    notificationChannelId: rule.notificationChannelId,
    enabled: rule.enabled,
    threshold: 80,
  }
  const condition = parseCondition(rule.condition)
  if ('operator' in condition && 'value' in condition) {
    const operator = String(condition.operator)
    ruleForm.operator = operator === '=' ? '==' : operator
    ruleForm.threshold = Number(condition.value)
    ruleForm.forSeconds = readPositiveInteger(condition.for_seconds)
    ruleForm.cooldownSeconds = readPositiveInteger(condition.cooldown_seconds)
    ruleForm.repeatSeconds = readPositiveInteger(condition.repeat_seconds)
    ruleForm.noDataSeconds = readPositiveInteger(condition.no_data_seconds)
    const recovery = condition.recovery_value
    if (typeof recovery === 'number' && Number.isFinite(recovery) && recovery >= 0) {
      ruleForm.recoveryValue = recovery
    }
  }
  if (Array.isArray(condition.statuses)) {
    ruleForm.statusOnline = condition.statuses.includes('ONLINE')
    ruleForm.statusOffline = condition.statuses.includes('OFFLINE')
  }
  return ruleForm
}

const form = ref<RuleForm>(newRuleForm())
const channels = ref<NotificationChannel[]>([])
const channelsLoaded = ref(false)
const channelsError = ref('')
const saving = ref(false)

const isStatusChangeType = computed(() => form.value.eventType === AlertEventType.STATUS_CHANGE)
const isThresholdType = computed(
  () => form.value.eventType !== AlertEventType.CRASH && !isStatusChangeType.value,
)
const thresholdUnit = computed(() =>
  form.value.eventType === AlertEventType.PLAYER_COUNT_THRESHOLD ? 'players' : '%',
)
const thresholdHelp = computed(() => thresholdHelps[form.value.eventType] ?? '')

// Without this the select shows the rule's raw channel ID until its channel is listed.
const channelDisplay = computed(() => {
  if (!channelsLoaded.value) return 'Loading channels…'
  return (
    channels.value.find((channel) => channel.id === form.value.notificationChannelId)?.name ?? ''
  )
})

// An empty status list matches every transition, so a status rule needs at least one.
const statusSelectionMissing = computed(
  () => isStatusChangeType.value && !form.value.statusOnline && !form.value.statusOffline,
)

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

const canSave = computed(() => {
  const ruleForm = form.value
  if (ruleForm.notificationChannelId === '') return false
  if (!eventTypeOptions.value.some((option) => option.value === ruleForm.eventType)) return false
  if (statusSelectionMissing.value) return false
  if (!isThresholdType.value) return true
  return (
    isFiniteNonNegativeNumber(ruleForm.threshold) &&
    isNonNegativeInteger(ruleForm.forSeconds) &&
    isNonNegativeInteger(ruleForm.cooldownSeconds) &&
    isNonNegativeInteger(ruleForm.repeatSeconds) &&
    recoveryMessage(ruleForm.recoveryValue) === ''
  )
})

function onEventTypeChange(eventType: AlertEventType): void {
  if (!props.rule && isNodeAlertEventType(eventType)) {
    form.value.threshold = nodeHighBand(eventType)
  }
}

function buildCondition(): string {
  const ruleForm = form.value
  if (isStatusChangeType.value) {
    const statuses: string[] = []
    if (ruleForm.statusOnline) statuses.push('ONLINE')
    if (ruleForm.statusOffline) statuses.push('OFFLINE')
    return JSON.stringify({ statuses })
  }
  if (!isThresholdType.value) return ''

  const condition: Record<string, number | string> = {
    operator: ruleForm.operator,
    value: ruleForm.threshold,
  }
  if (ruleForm.forSeconds > 0) condition.for_seconds = ruleForm.forSeconds
  // A cleared q-input yields null or '', both of which mean "no hysteresis".
  if (ruleForm.recoveryValue !== null && String(ruleForm.recoveryValue) !== '') {
    condition.recovery_value = ruleForm.recoveryValue
  }
  if (ruleForm.cooldownSeconds > 0) condition.cooldown_seconds = ruleForm.cooldownSeconds
  if (ruleForm.repeatSeconds > 0) condition.repeat_seconds = ruleForm.repeatSeconds
  if (ruleForm.noDataSeconds > 0) condition.no_data_seconds = ruleForm.noDataSeconds
  return JSON.stringify(condition)
}

async function loadChannels(): Promise<void> {
  try {
    const response = await GetXylonaClient().listNotificationChannels(
      create(ListNotificationChannelsRequestSchema, {}),
    )
    channels.value = response.channels
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
  const rule = props.rule
  const request = {
    serverId: rule ? rule.serverId : props.serverId,
    serverNodeId: rule ? rule.serverNodeId : props.serverNodeId,
    nodeId: rule ? rule.nodeId : props.nodeId,
    eventType: form.value.eventType,
    notificationChannelId: form.value.notificationChannelId,
    condition: buildCondition(),
    enabled: form.value.enabled,
  }

  saving.value = true
  try {
    if (rule) {
      await GetXylonaClient().updateAlertRule(
        create(UpdateAlertRuleRequestSchema, { id: rule.id, ...request }),
      )
    } else {
      await GetXylonaClient().createAlertRule(create(CreateAlertRuleRequestSchema, request))
    }
  } catch (unknownErr: unknown) {
    // Keep the dialog open so the input survives a failed save.
    notifyConnectError(unknownErr)
    return
  } finally {
    saving.value = false
  }
  notifySuccess(rule ? 'Alert rule updated' : 'Alert rule created')
  open.value = false
  emit('saved')
}

// Every opening starts from a fresh form and an empty, reloading channel list, so neither
// the last visit's channels nor its "no channels" banner shows, and a channel created in
// another tab since then is offered.
watch(
  open,
  (isOpen) => {
    if (!isOpen) return
    form.value = props.rule ? formFromRule(props.rule) : newRuleForm()
    channels.value = []
    channelsLoaded.value = false
    channelsError.value = ''
    void loadChannels()
  },
  { immediate: true },
)
</script>

<style scoped>
.alert-rule-dialog {
  width: min(560px, calc(100vw - 2rem));
  max-height: calc(100vh - 2rem);
}

.alert-rule-dialog__help {
  max-width: 65ch;
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: 1.5;
}

.alert-rule-dialog__advanced {
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.alert-rule-dialog__advanced :deep(.q-expansion-item__container > .q-item) {
  min-height: 2.5rem;
}

.alert-rule-dialog__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 220px), 1fr));
  gap: var(--xy-space-md);
  padding-inline: var(--xy-space-md);
  padding-bottom: var(--xy-space-md);
}

@media (max-width: 599px) {
  .alert-rule-dialog {
    width: calc(100vw - 1rem);
    max-height: calc(100vh - 1rem);
  }

  .alert-rule-dialog :deep(.q-card__actions) {
    flex-wrap: wrap;
  }

  .alert-rule-dialog :deep(.q-card__actions .q-btn) {
    flex: 1 1 8rem;
  }

  .alert-rule-dialog__grid {
    grid-template-columns: 1fr;
  }
}
</style>
