<script lang="ts" setup>
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { connectErrorMessage } from '@/api/connect-errors'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import AlertRuleDialog from '@/components/alerts/AlertRuleDialog.vue'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import { useUserAuthStore } from '@/stores/xylona'
import { formatAlertEventData, formatCondition } from '@/utils/alert-conditions'
import { canManageAlerts } from '@/utils/alert-permissions'
import {
  alertEventTypeLabel,
  alertEventTypeOptions,
  isNodeAlertEventType,
} from '@/utils/alert-scope'
import { formatTimestamp } from '@/utils/format-timestamp'
import { GetXylonaClient } from '@/utils/shared'
import {
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
// Each loader owns its error and clears it on success, so a later refresh that works
// removes the banner and an empty state never stands in for an outage.
const nodeError = ref('')
const channelsError = ref('')
const rulesError = ref('')
const historyError = ref('')
const loadError = computed(
  () => nodeError.value || rulesError.value || historyError.value || channelsError.value,
)
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
const channelsLoaded = ref(false)

// Dialog state
const showRuleDialog = ref(false)
const editingRule = ref<AlertRule | null>(null)

const historyFilterOptions = [
  { label: 'All Events', value: null },
  ...alertEventTypeOptions.filter((option) => !isNodeAlertEventType(option.value)),
]

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

const rulesColumns = computed(() => [
  {
    name: 'eventType',
    label: 'Event Type',
    field: (row: AlertRule) => alertEventTypeLabel(row.eventType),
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
    field: (row: AlertHistoryEntry) => alertEventTypeLabel(row.eventType),
    align: 'left' as const,
  },
  {
    name: 'eventData',
    label: 'Details',
    field: (row: AlertHistoryEntry) => formatAlertEventData(row.eventType, row.eventData),
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

function ruleToggleLabel(rule: AlertRule): string {
  const label = `Enable ${alertEventTypeLabel(rule.eventType)} alert`
  const condition = formatCondition(rule.eventType, rule.condition)
  return condition === '-' ? label : `${label}: ${condition}`
}

onMounted(loadPage)

async function loadPage(): Promise<void> {
  rulesLoading.value = true
  historyLoading.value = true
  await loadChannels()
  const loadedServerNodeID = await loadServerNodeID()
  if (!loadedServerNodeID) {
    rulesLoading.value = false
    historyLoading.value = false
    return
  }
  await loadRules()
  await loadHistory()
}

async function loadServerNodeID(): Promise<boolean> {
  try {
    const request = create(GetGameServerRequestSchema, {
      id: gameServerId.value,
    })
    const response = await GetXylonaClient().getGameServer(request)
    const nodeID = response.gameServer?.nodeId ?? ''
    if (nodeID === '') {
      nodeError.value = 'The game server node for alerts could not be determined.'
      return false
    }
    gameServerNodeId.value = nodeID
    nodeError.value = ''
    return true
  } catch (unknownErr: unknown) {
    nodeError.value = connectErrorMessage(unknownErr)
    return false
  }
}

async function loadChannels(): Promise<void> {
  try {
    const request = create(ListNotificationChannelsRequestSchema, {})
    const response = await GetXylonaClient().listNotificationChannels(request)
    channels.value = response.channels
    channelsLoaded.value = true
    channelsError.value = ''
  } catch (unknownErr: unknown) {
    channelsError.value = connectErrorMessage(unknownErr)
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
    rulesError.value = ''
  } catch (unknownErr: unknown) {
    rulesError.value = connectErrorMessage(unknownErr)
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
    historyError.value = ''
  } catch (unknownErr: unknown) {
    historyError.value = connectErrorMessage(unknownErr)
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
  showRuleDialog.value = true
}

function openEditDialog(rule: AlertRule): void {
  if (!hasAlertsManage.value) return
  editingRule.value = rule
  showRuleDialog.value = true
}

function confirmDeleteRule(rule: AlertRule): void {
  if (!hasAlertsManage.value) return

  const label = alertEventTypeLabel(rule.eventType)
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
  <div class="alerts-page xy-page-content">
    <page-header
      class="alerts-page-header"
      subtitle="Get notified when this server crashes, changes status, or crosses a threshold."
      title="Alerts">
      <template v-if="hasAlertsManage && activeTab === 'rules'" #actions>
        <q-btn
          :disable="channels.length === 0"
          color="primary"
          icon="add"
          label="Create Rule"
          no-caps
          @click="openCreateDialog">
          <q-tooltip v-if="channels.length === 0">Create a notification channel first</q-tooltip>
        </q-btn>
      </template>
    </page-header>
    <q-banner v-if="loadError" class="xy-banner-negative q-mb-md" dense inline-actions role="alert">
      <template #avatar>
        <q-icon name="sync_problem" />
      </template>
      <strong>Alerts could not be loaded.</strong> {{ loadError }}
      <template #action>
        <q-btn
          :loading="rulesLoading || historyLoading"
          aria-label="Retry loading alerts"
          flat
          icon="refresh"
          label="Retry"
          no-caps
          @click="loadPage" />
      </template>
    </q-banner>
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
        <q-banner
          v-if="channelsLoaded && channels.length === 0 && !rulesLoading"
          class="q-mb-md xy-banner-warning"
          dense>
          <template #avatar>
            <q-icon name="warning_amber" size="sm" />
          </template>
          No notification channels configured.
          <template v-if="hasAlertsManage">
            <router-link class="text-weight-bold" to="/notifications">
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
          aria-label="Alert rules"
          :columns="rulesColumns"
          :grid="mobileGrid"
          :loading="rulesLoading"
          :pagination="{ rowsPerPage: 0 }"
          :rows="alertRules"
          class="xy-standalone-table"
          flat
          hide-pagination
          row-key="id">
          <template #no-data>
            <empty-state
              v-if="!rulesLoading && !rulesError && !nodeError"
              icon="notifications_off"
              title="No alert rules configured for this server" />
          </template>
          <template #item="props">
            <q-card bordered class="alerts-mobile-card" flat>
              <q-card-section class="alerts-mobile-card__header">
                <div>
                  <div class="alerts-mobile-card__title">
                    {{ alertEventTypeLabel(props.row.eventType) }}
                  </div>
                  <div class="text-caption text-xy-muted">
                    {{ formatCondition(props.row.eventType, props.row.condition) }}
                  </div>
                </div>
                <q-toggle
                  v-if="hasAlertsManage"
                  :aria-label="ruleToggleLabel(props.row)"
                  :model-value="props.row.enabled"
                  color="positive"
                  dense
                  @update:model-value="toggleRuleEnabled(props.row)" />
                <q-badge
                  v-else
                  :color="props.row.enabled ? 'positive' : 'grey-8'"
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
                :aria-label="ruleToggleLabel(props.row)"
                :model-value="props.row.enabled"
                color="positive"
                @update:model-value="toggleRuleEnabled(props.row)" />
              <q-badge
                v-else
                :color="props.row.enabled ? 'positive' : 'grey-8'"
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
      </q-tab-panel>

      <!-- Alert History Panel -->
      <q-tab-panel name="history">
        <div class="alerts-toolbar">
          <q-select
            v-model="historyEventTypeFilter"
            :options="historyFilterOptions"
            class="alerts-filter-select"
            dense
            emit-value
            label="Filter by event"
            map-options
            outlined />
          <q-btn aria-label="Refresh history" flat icon="refresh" @click="loadHistory()">
            <q-tooltip>Refresh</q-tooltip>
          </q-btn>
        </div>

        <q-table
          aria-label="Alert history"
          :columns="historyColumns"
          :grid="mobileGrid"
          :loading="historyLoading"
          :pagination="{ page: historyPage, rowsPerPage: historyRowsPerPage }"
          :rows="filteredHistory"
          class="xy-standalone-table"
          flat
          row-key="id">
          <template #no-data>
            <empty-state
              v-if="!historyLoading && !historyError && !nodeError"
              icon="history"
              title="No alert history for this server" />
          </template>
          <template #item="props">
            <q-card bordered class="alerts-mobile-card" flat>
              <q-card-section class="alerts-mobile-card__header">
                <div>
                  <div class="alerts-mobile-card__title">
                    {{ alertEventTypeLabel(props.row.eventType) }}
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
                  <strong>{{
                    formatAlertEventData(props.row.eventType, props.row.eventData)
                  }}</strong>
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
      </q-tab-panel>
    </q-tab-panels>

    <alert-rule-dialog
      v-if="hasAlertsManage"
      v-model="showRuleDialog"
      :rule="editingRule"
      :server-id="gameServerId"
      :server-node-id="gameServerNodeId"
      @saved="loadRules" />
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

/* The page well already pads the sides; the panels only add vertical rhythm. */
.alerts-panels :deep(.q-tab-panel) {
  padding: var(--xy-space-md) 0;
}

.alerts-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
}

.alerts-filter-select {
  min-width: 220px;
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
</style>
