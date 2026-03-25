<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { Timestamp, timestampDate } from '@bufbuild/protobuf/wkt'
import { useQuasar } from 'quasar'
import dayjs from 'dayjs'
import { useUserAuthStore } from '@/stores/xylona'
import { canManageAlerts } from '@/utils/alert-permissions'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import {
  GetGameServerRequestSchema,
  ListAlertRulesRequestSchema,
  CreateAlertRuleRequestSchema,
  UpdateAlertRuleRequestSchema,
  DeleteAlertRuleRequestSchema,
  GetAlertHistoryRequestSchema,
  ListNotificationChannelsRequestSchema,
} from '@/proto/xylona_pb'
import { AlertEventType, NotificationChannelType, DeliveryStatus } from '@/proto/shared_pb'
import type { AlertRule, AlertHistoryEntry, NotificationChannel } from '@/proto/shared_pb'

const $q = useQuasar()
const authStore = useUserAuthStore()
const route = useRoute()
const gameServerId = computed(() => route.params.id as string)
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

function formatTimestamp(ts: Timestamp | undefined): string {
  if (!ts) return ''
  return dayjs(timestampDate(ts)).format('MM/DD/YYYY HH:mm:ss A')
}

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
      return `${String(parsed.operator)} ${String(parsed.value)}${unit}`
    }

    if ('statuses' in parsed && Array.isArray(parsed.statuses)) {
      return (parsed.statuses as string[]).join(', ')
    }
  } catch {
    // Not JSON; return raw
  }

  return condition
}

function parseConditionForEdit(eventType: AlertEventType, condition: string): void {
  if (!condition) return

  try {
    const parsed = JSON.parse(condition) as Record<string, unknown>

    if ('operator' in parsed && 'value' in parsed) {
      thresholdOperator.value = String(parsed.operator)
      thresholdValue.value = Number(parsed.value)
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
    return JSON.stringify({ operator: thresholdOperator.value, value: thresholdValue.value })
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
      $q.notify({
        type: 'xylona-error',
        caption: 'Failed to determine the game server node for alerts',
        position: 'top',
        timeout: 5000,
      })
      return false
    }
    gameServerNodeId.value = nodeID
    return true
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
    return false
  }
}

async function loadChannels(): Promise<void> {
  try {
    const request = create(ListNotificationChannelsRequestSchema, {})
    const response = await GetXylonaClient().listNotificationChannels(request)
    channels.value = response.channels
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
  statusCheckboxes.value = { ONLINE: true, OFFLINE: true }
  parseConditionForEdit(rule.eventType, rule.condition)
  showRuleDialog.value = true
}

async function saveRule(): Promise<void> {
  if (!hasAlertsManage.value) return

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
      $q.notify({
        type: 'xylona-success',
        caption: 'Alert rule updated',
        position: 'top',
        timeout: 3000,
      })
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: ConnectErrorToString(err),
        position: 'top',
        timeout: 5000,
      })
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
      $q.notify({
        type: 'xylona-success',
        caption: 'Alert rule created',
        position: 'top',
        timeout: 3000,
      })
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
      $q.notify({
        type: 'xylona-success',
        caption: 'Alert rule deleted',
        position: 'top',
        timeout: 3000,
      })
      await loadRules()
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  }
}
</script>

<template>
  <div class="alerts-page">
    <q-tabs
      v-model="activeTab"
      dense
      class="alerts-tabs"
      active-color="primary"
      indicator-color="primary"
      align="left"
      narrow-indicator>
      <q-tab name="rules" label="Alert Rules" />
      <q-tab name="history" label="Alert History" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" animated class="alerts-panels">
      <!-- Alert Rules Panel -->
      <q-tab-panel name="rules">
        <div class="q-pa-md">
          <div class="row items-center q-mb-md">
            <q-icon name="notifications" size="sm" color="primary" class="q-mr-sm" />
            <div class="text-h6">Alert Rules</div>
            <q-space />
            <q-btn
              v-if="hasAlertsManage"
              color="primary"
              icon="add"
              label="Create Rule"
              no-caps
              :disable="channels.length === 0"
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
              <q-icon name="warning" color="dark" />
            </template>
            No notification channels configured.
            <template v-if="hasAlertsManage">
              <router-link to="/notifications" class="text-dark text-weight-bold">
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
            :rows="alertRules"
            :columns="rulesColumns"
            row-key="id"
            flat
            :loading="rulesLoading"
            :pagination="{ rowsPerPage: 0 }"
            hide-pagination
            class="xy-standalone-table"
            no-data-label="No alert rules configured for this server">
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
                    flat
                    dense
                    icon="edit"
                    size="sm"
                    aria-label="Edit rule"
                    @click="openEditDialog(props.row)">
                    <q-tooltip>Edit</q-tooltip>
                  </q-btn>
                  <q-btn
                    flat
                    dense
                    icon="delete"
                    size="sm"
                    color="negative"
                    aria-label="Delete rule"
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
            <q-icon name="history" size="sm" color="primary" class="q-mr-sm" />
            <div class="text-h6">Alert History</div>
            <q-space />
            <q-select
              v-model="historyEventTypeFilter"
              :options="historyFilterOptions"
              dense
              outlined
              emit-value
              map-options
              label="Filter by event"
              style="min-width: 200px"
              class="q-mr-sm" />
            <q-btn flat icon="refresh" aria-label="Refresh history" @click="loadHistory">
              <q-tooltip>Refresh</q-tooltip>
            </q-btn>
          </div>

          <q-table
            :rows="filteredHistory"
            :columns="historyColumns"
            row-key="id"
            flat
            :loading="historyLoading"
            :pagination="{ page: historyPage, rowsPerPage: historyRowsPerPage }"
            class="xy-standalone-table"
            no-data-label="No alert history for this server">
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
              flat
              color="primary"
              label="Load More"
              no-caps
              :loading="historyLoading"
              @click="loadMoreHistory" />
          </div>
        </div>
      </q-tab-panel>
    </q-tab-panels>

    <!-- Create/Edit Rule Dialog -->
    <q-dialog v-if="hasAlertsManage" v-model="showRuleDialog" persistent>
      <q-card style="min-width: 450px">
        <q-card-section>
          <div class="text-h6">{{ dialogTitle }}</div>
        </q-card-section>

        <q-card-section class="q-pt-none">
          <q-select
            v-model="ruleForm.eventType"
            :options="serverEventTypes"
            label="Event Type"
            emit-value
            map-options
            outlined
            dense
            class="q-mb-md" />

          <!-- Threshold condition fields -->
          <div v-if="isThresholdType" class="row q-gutter-sm q-mb-md">
            <q-select
              v-model="thresholdOperator"
              :options="thresholdOperators"
              label="Operator"
              emit-value
              map-options
              outlined
              dense
              class="col-4" />
            <q-input
              v-model.number="thresholdValue"
              type="number"
              label="Value"
              outlined
              dense
              :suffix="thresholdUnit"
              class="col" />
          </div>

          <!-- Status change condition fields -->
          <div v-if="isStatusChangeType" class="q-mb-md">
            <div class="text-caption q-mb-xs">Trigger on status:</div>
            <q-checkbox v-model="statusCheckboxes.ONLINE" label="Online" />
            <q-checkbox v-model="statusCheckboxes.OFFLINE" label="Offline" />
          </div>

          <q-select
            v-model="ruleForm.notificationChannelId"
            :options="channels.map((c) => ({ label: c.name, value: c.id }))"
            label="Notification Channel"
            emit-value
            map-options
            outlined
            dense
            class="q-mb-md" />

          <q-toggle v-model="ruleForm.enabled" label="Enabled" color="positive" />
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat label="Cancel" no-caps @click="showRuleDialog = false" />
          <q-btn
            color="primary"
            :label="editingRule ? 'Save' : 'Create'"
            no-caps
            :disable="!ruleForm.notificationChannelId"
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
</style>
