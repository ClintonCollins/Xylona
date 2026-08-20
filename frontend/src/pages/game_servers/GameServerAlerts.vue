<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { notifyConnectError, notifyError, notifySuccess } from '@/api/notifications'
import PageHeader from '@/components/shared/PageHeader.vue'
import { useUserAuthStore } from '@/stores/xylona'
import {
  formatDuration,
  isFiniteNonNegativeNumber,
  isNonNegativeInteger,
  readPositiveInteger,
} from '@/utils/alert-conditions'
import { canManageAlerts } from '@/utils/alert-permissions'
import { formatTimestamp } from '@/utils/format-timestamp'
import { GetXylonaClient } from '@/utils/shared'
import {
  CreateAlertRuleRequestSchema,
  DeleteAlertRuleRequestSchema,
  GetAlertHistoryRequestSchema,
  GetGameServerRequestSchema,
  ListAlertRulesRequestSchema,
  ListNotificationChannelsRequestSchema,
  UpdateAlertRuleRequestSchema,
} from '@/proto/xylona_pb'
import type { AlertHistoryEntry, AlertRule, NotificationChannel } from '@/proto/shared_pb'
import { AlertEventType, DeliveryStatus, NotificationChannelType } from '@/proto/shared_pb'

const $q = useQuasar()
const authStore = useUserAuthStore()
const route = useRoute()
const gameServerId = computed(() => route.params.id as string)
const mobileGrid = computed(() => $q.screen?.lt?.md ?? false)
const gameServerNodeId = ref('')
const hasAlertsManage = computed(() => canManageAlerts(authStore.user, authStore.initialResponse))

const activeTab = ref<'rules' | 'history'>('rules')

// Rules state
const rulesLoading = ref(true)
const alertRules = ref<AlertRule[]>([])

// History state
const historyLoading = ref(true)
const alertHistory = ref<AlertHistoryEntry[]>([])
const historyHasMore = ref(false)
const historyPage = ref(1)
const historyRowsPerPage = ref(25)
const historyEventTypeFilter = ref<AlertEventType | null>(null)
const historyPageSize = 100

// Channels for dropdown
const channels = ref<NotificationChannel[]>([])

// Dialog state
const showRuleDialog = ref(false)
const editingRule = ref<AlertRule | null>(null)
const ruleForm = ref({
  eventType: AlertEventType.CRASH,
  notificationChannelId: '',
  condition: '',
  enabled: true,
})

// Condition sub-fields for threshold types
const thresholdOperator = ref('>=')
const thresholdValue = ref(80)
const thresholdForSeconds = ref(0)
const thresholdRecoveryValue = ref<number | null>(null)
const thresholdCooldownSeconds = ref(0)
const thresholdRepeatSeconds = ref(0)
const thresholdNoDataSeconds = ref(0)

// Condition sub-fields for status change
const statusCheckboxes = ref({
  ONLINE: true,
  OFFLINE: true,
})

const thresholdOperators = [
  { label: '>=', value: '>=' },
  { label: '>', value: '>' },
  { label: '<=', value: '<=' },
  { label: '<', value: '<' },
  { label: '=', value: '==' },
]

const eventTypeLabels: Record<number, string> = {
  [AlertEventType.CRASH]: 'Server Crash',
  [AlertEventType.STATUS_CHANGE]: 'Status Change',
  [AlertEventType.CPU_THRESHOLD]: 'CPU Threshold',
  [AlertEventType.MEMORY_THRESHOLD]: 'Memory Threshold',
  [AlertEventType.DISK_THRESHOLD]: 'Disk Threshold',
  [AlertEventType.PLAYER_COUNT_THRESHOLD]: 'Player Count Threshold',
}

const serverEventTypes = [
  { label: 'Server Crash', value: AlertEventType.CRASH },
  { label: 'Status Change', value: AlertEventType.STATUS_CHANGE },
  { label: 'CPU Threshold', value: AlertEventType.CPU_THRESHOLD },
  { label: 'Memory Threshold', value: AlertEventType.MEMORY_THRESHOLD },
  { label: 'Disk Threshold', value: AlertEventType.DISK_THRESHOLD },
  { label: 'Player Count Threshold', value: AlertEventType.PLAYER_COUNT_THRESHOLD },
]

const historyFilterOptions = [{ label: 'All Events', value: null }, ...serverEventTypes]

const channelTypeLabels: Record<number, string> = {
  [NotificationChannelType.WEBHOOK_DISCORD]: 'Discord',
  [NotificationChannelType.WEBHOOK_SLACK]: 'Slack',
  [NotificationChannelType.WEBHOOK_GENERIC]: 'Webhook',
  [NotificationChannelType.EMAIL]: 'Email',
}

const deliveryStatusLabels: Record<number, string> = {
  [DeliveryStatus.PENDING]: 'Pending',
  [DeliveryStatus.SENT]: 'Sent',
  [DeliveryStatus.FAILED]: 'Failed',
}

const deliveryStatusColors: Record<number, string> = {
  [DeliveryStatus.PENDING]: 'warning',
  [DeliveryStatus.SENT]: 'positive',
  [DeliveryStatus.FAILED]: 'negative',
}

const isThresholdType = computed(() => {
  return [
    AlertEventType.CPU_THRESHOLD,
    AlertEventType.MEMORY_THRESHOLD,
    AlertEventType.DISK_THRESHOLD,
    AlertEventType.PLAYER_COUNT_THRESHOLD,
  ].includes(ruleForm.value.eventType)
})

const isStatusChangeType = computed(() => {
  return ruleForm.value.eventType === AlertEventType.STATUS_CHANGE
})

const thresholdUnit = computed(() => {
  switch (ruleForm.value.eventType) {
    case AlertEventType.CPU_THRESHOLD:
    case AlertEventType.MEMORY_THRESHOLD:
    case AlertEventType.DISK_THRESHOLD:
      return '%'
    case AlertEventType.PLAYER_COUNT_THRESHOLD:
      return 'players'
    default:
      return ''
  }
})

const thresholdHelp = computed(() => {
  switch (ruleForm.value.eventType) {
    case AlertEventType.CPU_THRESHOLD:
      return 'Process CPU normalized to the node host. Valid samples are evaluated; unavailable samples do not trigger alerts.'
    case AlertEventType.MEMORY_THRESHOLD:
      return 'Process RSS as a percentage of total node memory. Valid samples are evaluated; unavailable samples do not trigger alerts.'
    case AlertEventType.DISK_THRESHOLD:
      return 'Usage of the volume containing the server working directory. Valid samples are evaluated; unavailable samples do not trigger alerts.'
    case AlertEventType.PLAYER_COUNT_THRESHOLD:
      return 'Player count from the game query source. Unavailable or unsupported query samples do not trigger alerts.'
    default:
      return ''
  }
})

const thresholdConditionValid = computed(() => {
  return (
    isFiniteNonNegativeNumber(thresholdValue.value) &&
    isNonNegativeInteger(thresholdForSeconds.value) &&
    isNonNegativeInteger(thresholdCooldownSeconds.value) &&
    isNonNegativeInteger(thresholdRepeatSeconds.value) &&
    recoveryValidationMessage(thresholdRecoveryValue.value) === ''
  )
})

const canSaveRule = computed(() => {
  return (
    ruleForm.value.notificationChannelId !== '' &&
    (!isThresholdType.value || thresholdConditionValid.value)
  )
})

const durationRules = [
  (value: unknown) => isNonNegativeInteger(value) || 'Use a whole number of seconds, 0 or greater',
]

const recoveryRules = computed(() => [(value: unknown) => recoveryValidationMessage(value) || true])

const dialogTitle = computed(() => {
  return editingRule.value ? 'Edit Alert Rule' : 'Create Alert Rule'
})

const rulesColumns = computed(() => [
  {
    name: 'eventType',
    label: 'Event Type',
    field: (row: AlertRule) => eventTypeLabels[row.eventType] ?? 'Unknown',
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'condition',
    label: 'Condition',
    field: (row: AlertRule) => formatCondition(row.eventType, row.condition),
    align: 'left' as const,
  },
  {
    name: 'channel',
    label: 'Channel',
    field: (row: AlertRule) => {
      const channel = channels.value.find((c) => c.id === row.notificationChannelId)
      return channel?.name ?? 'Unknown'
    },
    align: 'left' as const,
  },
  {
    name: 'enabled',
    label: 'Enabled',
    field: 'enabled',
    align: 'center' as const,
  },
  {
    name: 'actions',
    label: 'Actions',
    field: '',
    align: 'right' as const,
  },
])

const historyColumns = computed(() => [
  {
    name: 'createdAt',
    label: 'Time',
    field: (row: AlertHistoryEntry) => formatTimestamp(row.createdAt),
    align: 'left' as const,
    sortable: true,
  },
  {
    name: 'eventType',
    label: 'Event Type',
    field: (row: AlertHistoryEntry) => eventTypeLabels[row.eventType] ?? 'Unknown',
    align: 'left' as const,
  },
  {
    name: 'eventData',
    label: 'Details',
    field: 'eventData',
    align: 'left' as const,
  },
  {
    name: 'channelType',
    label: 'Channel',
    field: (row: AlertHistoryEntry) => channelTypeLabels[row.channelType] ?? 'Unknown',
    align: 'left' as const,
  },
  {
    name: 'deliveryStatus',
    label: 'Status',
    field: 'deliveryStatus',
    align: 'center' as const,
  },
])

const filteredHistory = computed(() => {
  if (historyEventTypeFilter.value === null) {
    return alertHistory.value
  }
  return alertHistory.value.filter((entry) => entry.eventType === historyEventTypeFilter.value)
})

function formatCondition(eventType: AlertEventType, condition: string): string {
  if (!condition) {
    if (eventType === AlertEventType.CRASH) {
      return 'Any crash'
    }
    return '-'
  }

  try {
    const parsed = JSON.parse(condition) as Record<string, unknown>

    if ('operator' in parsed && 'value' in parsed) {
      const unit = [
        AlertEventType.CPU_THRESHOLD,
        AlertEventType.MEMORY_THRESHOLD,
        AlertEventType.DISK_THRESHOLD,
      ].includes(eventType)
        ? '%'
        : ''
      const parts = [`${String(parsed.operator)} ${String(parsed.value)}${unit}`]
      const forSeconds = readPositiveInteger(parsed.for_seconds)
      const cooldownSeconds = readPositiveInteger(parsed.cooldown_seconds)
      const repeatSeconds = readPositiveInteger(parsed.repeat_seconds)

      if (forSeconds > 0) parts.push(`for ${formatDuration(forSeconds)}`)
      if (typeof parsed.recovery_value === 'number' && Number.isFinite(parsed.recovery_value)) {
        parts.push(`recover at ${parsed.recovery_value}${unit}`)
      }
      if (cooldownSeconds > 0) parts.push(`${formatDuration(cooldownSeconds)} cooldown`)
      if (repeatSeconds > 0) parts.push(`repeat ${formatDuration(repeatSeconds)}`)

      return parts.join(' · ')
    }

    if ('statuses' in parsed && Array.isArray(parsed.statuses)) {
      return (parsed.statuses as string[]).join(', ')
    }
  } catch {
    // Not JSON; return raw
  }

  return condition
}

function recoveryValidationMessage(value: unknown): string {
  if (value === null || value === undefined || value === '') return ''
  if (!isFiniteNonNegativeNumber(value)) return 'Use a number that is 0 or greater'

  const recoveryValue = Number(value)
  if (['>=', '>'].includes(thresholdOperator.value) && recoveryValue >= thresholdValue.value) {
    return 'Recovery must be below the trigger value'
  }
  if (['<=', '<'].includes(thresholdOperator.value) && recoveryValue <= thresholdValue.value) {
    return 'Recovery must be above the trigger value'
  }
  if (['==', '='].includes(thresholdOperator.value)) {
    return 'Equality alerts cannot use a recovery threshold'
  }
  return ''
}

function resetThresholdBehavior(): void {
  thresholdForSeconds.value = 0
  thresholdRecoveryValue.value = null
  thresholdCooldownSeconds.value = 0
  thresholdRepeatSeconds.value = 0
  thresholdNoDataSeconds.value = 0
}

function parseConditionForEdit(condition: string): void {
  if (!condition) return

  try {
    const parsed = JSON.parse(condition) as Record<string, unknown>

    if ('operator' in parsed && 'value' in parsed) {
      const parsedOperator = String(parsed.operator)
      thresholdOperator.value = parsedOperator === '=' ? '==' : parsedOperator
      thresholdValue.value = Number(parsed.value)
      thresholdForSeconds.value = readPositiveInteger(parsed.for_seconds)
      thresholdCooldownSeconds.value = readPositiveInteger(parsed.cooldown_seconds)
      thresholdRepeatSeconds.value = readPositiveInteger(parsed.repeat_seconds)
      thresholdNoDataSeconds.value = readPositiveInteger(parsed.no_data_seconds)

      if (
        typeof parsed.recovery_value === 'number' &&
        Number.isFinite(parsed.recovery_value) &&
        parsed.recovery_value >= 0
      ) {
        thresholdRecoveryValue.value = parsed.recovery_value
      }
    }

    if ('statuses' in parsed && Array.isArray(parsed.statuses)) {
      const statuses = parsed.statuses as string[]
      statusCheckboxes.value = {
        ONLINE: statuses.includes('ONLINE'),
        OFFLINE: statuses.includes('OFFLINE'),
      }
    }
  } catch {
    // Ignore parse errors
  }
}

function buildConditionJson(): string {
  if (isThresholdType.value) {
    const condition: Record<string, number | string> = {
      operator: thresholdOperator.value,
      value: thresholdValue.value,
    }

    if (thresholdForSeconds.value > 0) condition.for_seconds = thresholdForSeconds.value
    if (thresholdRecoveryValue.value !== null) {
      condition.recovery_value = thresholdRecoveryValue.value
    }
    if (thresholdCooldownSeconds.value > 0) {
      condition.cooldown_seconds = thresholdCooldownSeconds.value
    }
    if (thresholdRepeatSeconds.value > 0) condition.repeat_seconds = thresholdRepeatSeconds.value
    if (thresholdNoDataSeconds.value > 0) condition.no_data_seconds = thresholdNoDataSeconds.value

    return JSON.stringify(condition)
  }

  if (isStatusChangeType.value) {
    const statuses: string[] = []
    if (statusCheckboxes.value.ONLINE) statuses.push('ONLINE')
    if (statusCheckboxes.value.OFFLINE) statuses.push('OFFLINE')
    return JSON.stringify({ statuses })
  }

  return ''
}

onMounted(async () => {
  await loadChannels()
  const loadedServerNodeID = await loadServerNodeID()
  if (!loadedServerNodeID) {
    rulesLoading.value = false
    historyLoading.value = false
    return
  }
  await loadRules()
  await loadHistory()
})

async function loadServerNodeID(): Promise<boolean> {
  try {
    const request = create(GetGameServerRequestSchema, {
      id: gameServerId.value,
    })
    const response = await GetXylonaClient().getGameServer(request)
    const nodeID = response.gameServer?.nodeId ?? ''
    if (nodeID === '') {
      notifyError('Failed to determine the game server node for alerts')
      return false
    }
    gameServerNodeId.value = nodeID
    return true
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
    return false
  }
}

async function loadChannels(): Promise<void> {
  try {
    const request = create(ListNotificationChannelsRequestSchema, {})
    const response = await GetXylonaClient().listNotificationChannels(request)
    channels.value = response.channels
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}

async function loadRules(): Promise<void> {
  rulesLoading.value = true
  try {
    const request = create(ListAlertRulesRequestSchema, {
      serverId: gameServerId.value,
      serverNodeId: gameServerNodeId.value,
    })
    const response = await GetXylonaClient().listAlertRules(request)
    alertRules.value = response.rules
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    rulesLoading.value = false
  }
}

async function loadHistory(append: boolean = false): Promise<void> {
  historyLoading.value = true
  try {
    const offset = append ? alertHistory.value.length : 0
    const request = create(GetAlertHistoryRequestSchema, {
      serverId: gameServerId.value,
      serverNodeId: gameServerNodeId.value,
      limit: historyPageSize,
      offset,
    })
    const response = await GetXylonaClient().getAlertHistory(request)
    if (append) {
      alertHistory.value.push(...response.entries)
    } else {
      alertHistory.value = response.entries
    }
    historyHasMore.value = response.entries.length === historyPageSize
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    historyLoading.value = false
  }
}

function loadMoreHistory(): void {
  void loadHistory(true)
}

function openCreateDialog(): void {
  if (!hasAlertsManage.value) return

  editingRule.value = null
  ruleForm.value = {
    eventType: AlertEventType.CRASH,
    notificationChannelId: channels.value.length > 0 ? channels.value[0].id : '',
    condition: '',
    enabled: true,
  }
  thresholdOperator.value = '>='
  thresholdValue.value = 80
  resetThresholdBehavior()
  statusCheckboxes.value = { ONLINE: true, OFFLINE: true }
  showRuleDialog.value = true
}

function openEditDialog(rule: AlertRule): void {
  if (!hasAlertsManage.value) return

  editingRule.value = rule
  ruleForm.value = {
    eventType: rule.eventType,
    notificationChannelId: rule.notificationChannelId,
    condition: rule.condition,
    enabled: rule.enabled,
  }
  // Reset sub-fields to defaults before parsing
  thresholdOperator.value = '>='
  thresholdValue.value = 80
  resetThresholdBehavior()
  statusCheckboxes.value = { ONLINE: true, OFFLINE: true }
  parseConditionForEdit(rule.condition)
  showRuleDialog.value = true
}

async function saveRule(): Promise<void> {
  if (!hasAlertsManage.value) return
  if (!canSaveRule.value) return

  const condition = buildConditionJson()

  if (editingRule.value) {
    // Update
    try {
      const request = create(UpdateAlertRuleRequestSchema, {
        id: editingRule.value.id,
        serverId: gameServerId.value,
        serverNodeId: gameServerNodeId.value,
        eventType: ruleForm.value.eventType,
        notificationChannelId: ruleForm.value.notificationChannelId,
        condition,
        enabled: ruleForm.value.enabled,
      })
      await GetXylonaClient().updateAlertRule(request)
      notifySuccess('Alert rule updated')
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr)
    }
  } else {
    // Create
    try {
      const request = create(CreateAlertRuleRequestSchema, {
        serverId: gameServerId.value,
        serverNodeId: gameServerNodeId.value,
        eventType: ruleForm.value.eventType,
        notificationChannelId: ruleForm.value.notificationChannelId,
        condition,
        enabled: ruleForm.value.enabled,
      })
      await GetXylonaClient().createAlertRule(request)
      notifySuccess('Alert rule created')
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr)
    }
  }

  showRuleDialog.value = false
  await loadRules()
}

function confirmDeleteRule(rule: AlertRule): void {
  if (!hasAlertsManage.value) return

  const label = eventTypeLabels[rule.eventType] ?? 'this rule'
  $q.dialog({
    title: 'Delete Alert Rule',
    message: `Are you sure you want to delete the "${label}" alert rule?`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Delete' },
    persistent: true,
  }).onOk(async () => {
    try {
      const request = create(DeleteAlertRuleRequestSchema, {
        id: rule.id,
      })
      await GetXylonaClient().deleteAlertRule(request)
      notifySuccess('Alert rule deleted')
      await loadRules()
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr)
    }
  })
}

async function toggleRuleEnabled(rule: AlertRule): Promise<void> {
  if (!hasAlertsManage.value) return

  try {
    const request = create(UpdateAlertRuleRequestSchema, {
      id: rule.id,
      serverId: gameServerId.value,
      serverNodeId: gameServerNodeId.value,
      eventType: rule.eventType,
      notificationChannelId: rule.notificationChannelId,
      condition: rule.condition,
      enabled: !rule.enabled,
    })
    await GetXylonaClient().updateAlertRule(request)
    await loadRules()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  }
}
</script>

<template>
  <div class="alerts-page">
    <page-header class="alerts-page-header" title="Alerts" />
    <q-tabs
      v-model="activeTab"
      active-color="primary"
      align="left"
      class="alerts-tabs"
      dense
      indicator-color="primary"
      narrow-indicator>
      <q-tab label="Alert Rules" name="rules" />
      <q-tab label="Alert History" name="history" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" animated class="alerts-panels">
      <!-- Alert Rules Panel -->
      <q-tab-panel name="rules">
        <div class="q-pa-md">
          <div class="row items-center q-mb-md">
            <q-icon class="q-mr-sm" color="primary" name="notifications" size="sm" />
            <div class="text-h6">Alert Rules</div>
            <q-space />
            <q-btn
              v-if="hasAlertsManage"
              :disable="channels.length === 0"
              color="primary"
              icon="add"
              label="Create Rule"
              no-caps
              @click="openCreateDialog">
              <q-tooltip v-if="channels.length === 0">
                Create a notification channel first
              </q-tooltip>
            </q-btn>
          </div>

          <q-banner
            v-if="channels.length === 0 && !rulesLoading"
            class="q-mb-md bg-warning text-dark"
            rounded>
            <template #avatar>
              <q-icon color="dark" name="warning" />
            </template>
            No notification channels configured.
            <template v-if="hasAlertsManage">
              <router-link class="text-dark text-weight-bold" to="/notifications">
                Create a notification channel
              </router-link>
              before adding alert rules.
            </template>
            <template v-else>
              A user with alert management access must create a notification channel before alert
              rules can be added.
            </template>
          </q-banner>

          <q-table
            :columns="rulesColumns"
            :grid="mobileGrid"
            :loading="rulesLoading"
            :pagination="{ rowsPerPage: 0 }"
            :rows="alertRules"
            class="xy-standalone-table"
            flat
            hide-pagination
            no-data-label="No alert rules configured for this server"
            row-key="id">
            <template #item="props">
              <q-card bordered class="alerts-mobile-card" flat>
                <q-card-section class="alerts-mobile-card__header">
                  <div>
                    <div class="alerts-mobile-card__title">
                      {{ eventTypeLabels[props.row.eventType] ?? 'Unknown' }}
                    </div>
                    <div class="text-caption text-xy-muted">
                      {{ formatCondition(props.row.eventType, props.row.condition) }}
                    </div>
                  </div>
                  <q-toggle
                    v-if="hasAlertsManage"
                    :model-value="props.row.enabled"
                    color="positive"
                    dense
                    @update:model-value="toggleRuleEnabled(props.row)" />
                  <q-badge
                    v-else
                    :color="props.row.enabled ? 'positive' : 'negative'"
                    :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
                </q-card-section>

                <q-card-section class="alerts-mobile-card__fields q-pt-none">
                  <div>
                    <span>Channel</span>
                    <strong>{{
                      channels.find((channel) => channel.id === props.row.notificationChannelId)
                        ?.name ?? 'Unknown'
                    }}</strong>
                  </div>
                </q-card-section>

                <q-card-actions v-if="hasAlertsManage" align="right">
                  <q-btn flat icon="edit" label="Edit" no-caps @click="openEditDialog(props.row)" />
                  <q-btn
                    color="negative"
                    flat
                    icon="delete"
                    label="Delete"
                    no-caps
                    @click="confirmDeleteRule(props.row)" />
                </q-card-actions>
              </q-card>
            </template>

            <template #body-cell-enabled="props">
              <q-td :props="props">
                <q-toggle
                  v-if="hasAlertsManage"
                  :model-value="props.row.enabled"
                  color="positive"
                  @update:model-value="toggleRuleEnabled(props.row)" />
                <q-badge
                  v-else
                  :color="props.row.enabled ? 'positive' : 'negative'"
                  :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
              </q-td>
            </template>

            <template #body-cell-actions="props">
              <q-td :props="props">
                <template v-if="hasAlertsManage">
                  <q-btn
                    aria-label="Edit rule"
                    dense
                    flat
                    icon="edit"
                    size="sm"
                    @click="openEditDialog(props.row)">
                    <q-tooltip>Edit</q-tooltip>
                  </q-btn>
                  <q-btn
                    aria-label="Delete rule"
                    color="negative"
                    dense
                    flat
                    icon="delete"
                    size="sm"
                    @click="confirmDeleteRule(props.row)">
                    <q-tooltip>Delete</q-tooltip>
                  </q-btn>
                </template>
              </q-td>
            </template>
          </q-table>
        </div>
      </q-tab-panel>

      <!-- Alert History Panel -->
      <q-tab-panel name="history">
        <div class="q-pa-md">
          <div class="row items-center q-mb-md">
            <q-icon class="q-mr-sm" color="primary" name="history" size="sm" />
            <div class="text-h6">Alert History</div>
            <q-space />
            <q-select
              v-model="historyEventTypeFilter"
              :options="historyFilterOptions"
              class="q-mr-sm"
              dense
              emit-value
              label="Filter by event"
              map-options
              outlined
              style="min-width: 200px" />
            <q-btn aria-label="Refresh history" flat icon="refresh" @click="loadHistory">
              <q-tooltip>Refresh</q-tooltip>
            </q-btn>
          </div>

          <q-table
            :columns="historyColumns"
            :grid="mobileGrid"
            :loading="historyLoading"
            :pagination="{ page: historyPage, rowsPerPage: historyRowsPerPage }"
            :rows="filteredHistory"
            class="xy-standalone-table"
            flat
            no-data-label="No alert history for this server"
            row-key="id">
            <template #item="props">
              <q-card bordered class="alerts-mobile-card" flat>
                <q-card-section class="alerts-mobile-card__header">
                  <div>
                    <div class="alerts-mobile-card__title">
                      {{ eventTypeLabels[props.row.eventType] ?? 'Unknown' }}
                    </div>
                    <div class="text-caption text-xy-muted">
                      {{ formatTimestamp(props.row.createdAt) }}
                    </div>
                  </div>
                  <q-badge
                    :color="deliveryStatusColors[props.row.deliveryStatus] ?? 'grey'"
                    :label="deliveryStatusLabels[props.row.deliveryStatus] ?? 'Unknown'" />
                </q-card-section>

                <q-card-section class="alerts-mobile-card__fields q-pt-none">
                  <div>
                    <span>Channel</span>
                    <strong>{{ channelTypeLabels[props.row.channelType] ?? 'Unknown' }}</strong>
                  </div>
                  <div>
                    <span>Details</span>
                    <strong>{{ props.row.eventData || '-' }}</strong>
                  </div>
                  <div v-if="props.row.deliveryError">
                    <span>Delivery Error</span>
                    <strong class="text-negative">{{ props.row.deliveryError }}</strong>
                  </div>
                </q-card-section>
              </q-card>
            </template>

            <template #body-cell-deliveryStatus="props">
              <q-td :props="props">
                <q-badge
                  :color="deliveryStatusColors[props.row.deliveryStatus] ?? 'grey'"
                  :label="deliveryStatusLabels[props.row.deliveryStatus] ?? 'Unknown'" />
                <q-tooltip v-if="props.row.deliveryError">
                  {{ props.row.deliveryError }}
                </q-tooltip>
              </q-td>
            </template>
          </q-table>
          <div v-if="historyHasMore" class="row justify-center q-mt-md">
            <q-btn
              :loading="historyLoading"
              color="primary"
              flat
              label="Load More"
              no-caps
              @click="loadMoreHistory" />
          </div>
        </div>
      </q-tab-panel>
    </q-tab-panels>

    <!-- Create/Edit Rule Dialog -->
    <q-dialog v-if="hasAlertsManage" v-model="showRuleDialog" persistent>
      <q-card class="alert-rule-dialog">
        <q-card-section>
          <div class="text-h6">{{ dialogTitle }}</div>
        </q-card-section>

        <q-card-section class="q-pt-none">
          <q-select
            v-model="ruleForm.eventType"
            :options="serverEventTypes"
            class="q-mb-md"
            dense
            emit-value
            label="Event Type"
            map-options
            outlined />

          <!-- Threshold condition fields -->
          <div v-if="isThresholdType" class="row q-gutter-sm q-mb-md">
            <q-select
              v-model="thresholdOperator"
              :options="thresholdOperators"
              class="col-4"
              dense
              emit-value
              label="Operator"
              map-options
              outlined />
            <q-input
              v-model.number="thresholdValue"
              :rules="[
                (value: unknown) =>
                  isFiniteNonNegativeNumber(value) || 'Use a number that is 0 or greater',
              ]"
              :suffix="thresholdUnit"
              class="col"
              dense
              label="Value"
              outlined
              type="number" />
          </div>

          <div v-if="isThresholdType" class="threshold-help q-mb-md" role="note">
            {{ thresholdHelp }}
          </div>

          <q-expansion-item
            v-if="isThresholdType"
            class="threshold-advanced q-mb-md"
            dense
            expand-separator
            icon="tune"
            label="Advanced behavior">
            <div class="threshold-advanced__grid q-pt-sm">
              <q-input
                v-model.number="thresholdForSeconds"
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
                v-model.number="thresholdRecoveryValue"
                :rules="recoveryRules"
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
                v-model.number="thresholdCooldownSeconds"
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
                v-model.number="thresholdRepeatSeconds"
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

          <!-- Status change condition fields -->
          <div v-if="isStatusChangeType" class="q-mb-md">
            <div class="text-caption q-mb-xs">Trigger on status:</div>
            <q-checkbox v-model="statusCheckboxes.ONLINE" label="Online" />
            <q-checkbox v-model="statusCheckboxes.OFFLINE" label="Offline" />
          </div>

          <q-select
            v-model="ruleForm.notificationChannelId"
            :options="channels.map((c) => ({ label: c.name, value: c.id }))"
            class="q-mb-md"
            dense
            emit-value
            label="Notification Channel"
            map-options
            outlined />

          <q-toggle v-model="ruleForm.enabled" color="positive" label="Enabled" />
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat label="Cancel" no-caps @click="showRuleDialog = false" />
          <q-btn
            :disable="!canSaveRule"
            :label="editingRule ? 'Save' : 'Create'"
            color="primary"
            no-caps
            @click="saveRule" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<style scoped>
.alerts-page {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.alerts-page-header {
  padding: var(--xy-space-md) var(--xy-space-md) 0;
  margin-bottom: var(--xy-space-sm);
}

.alerts-tabs {
  background-color: var(--xy-surface-1);
  flex-shrink: 0;
}

.alerts-panels {
  flex: 1;
  min-height: 0;
  overflow: auto;
  background-color: transparent;
}

.alert-rule-dialog {
  width: min(560px, calc(100vw - 2rem));
  max-height: calc(100vh - 2rem);
}

.threshold-help {
  max-width: 65ch;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: 1.5;
}

.threshold-advanced {
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-md);
}

.threshold-advanced :deep(.q-expansion-item__container > .q-item) {
  min-height: 2.5rem;
}

.threshold-advanced__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-md);
  padding-inline: var(--xy-space-md);
  padding-bottom: var(--xy-space-md);
}

.alerts-mobile-card {
  width: 100%;
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.alerts-mobile-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.alerts-mobile-card__title {
  color: var(--xy-text-primary);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.alerts-mobile-card__fields {
  display: grid;
  gap: var(--xy-space-sm);
}

.alerts-mobile-card__fields > div {
  display: grid;
  gap: 0.15rem;
}

.alerts-mobile-card__fields span {
  color: var(--xy-text-muted);
  font-size: 0.72rem;
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.alerts-mobile-card__fields strong {
  color: var(--xy-text-primary);
  font-weight: 500;
  overflow-wrap: anywhere;
}

@media (max-width: 520px) {
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

  .threshold-advanced__grid {
    grid-template-columns: 1fr;
  }
}
</style>
