<template>
  <q-page class="xy-page-content">
    <page-header title="Notifications" />
    <q-banner v-if="loadError" class="xy-banner-negative q-mb-md" dense inline-actions role="alert">
      <template #avatar>
        <q-icon name="sync_problem" />
      </template>
      {{ loadError }}
      <template #action>
        <q-btn
          :loading="channelsLoading || rulesLoading || historyLoading"
          aria-label="Retry loading notifications"
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
      class="notifications-tabs"
      dense
      indicator-color="primary"
      narrow-indicator>
      <q-tab label="Channels" name="channels" />
      <q-tab label="Alert Rules" name="rules" />
      <q-tab label="Alert History" name="history" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" animated class="notifications-panels">
      <!-- Channels Tab -->
      <q-tab-panel name="channels">
        <div class="tab-toolbar">
          <q-btn
            v-if="hasAlertsManage"
            color="primary"
            icon="add"
            label="Add Channel"
            no-caps
            @click="openChannelDialog(null)" />
        </div>
        <q-table
          aria-label="Notification channels"
          :columns="channelColumns"
          :grid="$q.screen.lt.md"
          :loading="channelsLoading"
          :rows="channels"
          :rows-per-page-options="[10, 25, 50]"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="id">
          <template #item="props">
            <q-card
              :aria-label="`${props.row.name} notification channel`"
              bordered
              class="notification-mobile-card"
              flat
              role="article">
              <q-card-section class="notification-mobile-card__header">
                <div>
                  <div class="notification-mobile-card__title">{{ props.row.name }}</div>
                  <q-badge :label="channelTypeLabel(props.row.channelType)" color="grey-8" />
                </div>
                <q-toggle
                  v-if="hasAlertsManage"
                  :aria-label="`Toggle ${props.row.name} channel enabled`"
                  :model-value="props.row.enabled"
                  color="positive"
                  dense
                  @update:model-value="toggleChannelEnabled(props.row)" />
                <q-badge
                  v-else
                  :color="props.row.enabled ? 'positive' : 'grey-8'"
                  :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
              </q-card-section>

              <q-card-section class="notification-mobile-card__fields q-pt-none">
                <div>
                  <span>Created</span>
                  <strong>
                    {{ formatTimestamp(props.row.createdAt, 'Unknown') }}
                  </strong>
                </div>
              </q-card-section>

              <q-card-actions v-if="hasAlertsManage" align="right">
                <q-btn
                  :aria-label="`Test ${props.row.name} channel`"
                  :loading="testingChannelIds.has(props.row.id)"
                  flat
                  icon="send"
                  label="Test"
                  no-caps
                  @click="testChannel(props.row)" />
                <q-btn
                  :aria-label="`Edit ${props.row.name} channel`"
                  flat
                  icon="edit"
                  label="Edit"
                  no-caps
                  @click="openChannelDialog(props.row)" />
                <q-btn
                  :aria-label="`Delete ${props.row.name} channel`"
                  class="text-error-brighter"
                  flat
                  icon="delete"
                  label="Delete"
                  no-caps
                  @click="confirmDeleteChannel(props.row)" />
              </q-card-actions>
            </q-card>
          </template>
          <template #body-cell-channelType="props">
            <q-td :props="props">
              <q-badge :label="channelTypeLabel(props.row.channelType)" color="grey-8" />
            </q-td>
          </template>
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-toggle
                v-if="hasAlertsManage"
                :aria-label="`Toggle ${props.row.name} channel enabled`"
                :model-value="props.row.enabled"
                color="positive"
                @update:model-value="toggleChannelEnabled(props.row)" />
              <q-badge
                v-else
                :color="props.row.enabled ? 'positive' : 'grey-8'"
                :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div v-if="hasAlertsManage" class="xy-row-actions">
                <q-btn
                  :aria-label="`Test ${props.row.name} channel`"
                  :loading="testingChannelIds.has(props.row.id)"
                  dense
                  flat
                  icon="send"
                  round
                  @click="testChannel(props.row)">
                  <q-tooltip>Test</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Edit ${props.row.name} channel`"
                  dense
                  flat
                  icon="edit"
                  round
                  @click="openChannelDialog(props.row)">
                  <q-tooltip>Edit</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Delete ${props.row.name} channel`"
                  class="text-error-brighter"
                  dense
                  flat
                  icon="delete"
                  round
                  @click="confirmDeleteChannel(props.row)">
                  <q-tooltip>Delete</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <empty-state
              v-if="!channelsLoading && !channelsError"
              :description="
                hasAlertsManage
                  ? 'Add a channel to start receiving alerts.'
                  : 'Notification channels will appear here once a user with alert management access creates them.'
              "
              icon="notifications_off"
              title="No notification channels" />
          </template>
        </q-table>
      </q-tab-panel>

      <!-- Alert Rules Tab -->
      <q-tab-panel name="rules">
        <div class="tab-toolbar">
          <q-select
            v-model="rulesEventFilter"
            :options="alertEventTypeOptions"
            aria-label="Filter alert rules by event type"
            class="filter-select"
            clearable
            dense
            emit-value
            label="Filter by event type"
            map-options
            outlined />
        </div>
        <q-table
          aria-label="Alert rules"
          :columns="ruleColumns"
          :grid="$q.screen.lt.md"
          :loading="rulesLoading"
          :rows="filteredRules"
          :rows-per-page-options="[10, 25, 50]"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="id">
          <template #item="props">
            <q-card
              :aria-label="`${alertEventTypeLabel(props.row.eventType)} alert rule`"
              bordered
              class="notification-mobile-card"
              flat
              role="article">
              <q-card-section class="notification-mobile-card__header">
                <div>
                  <div class="notification-mobile-card__title">
                    {{ alertEventTypeLabel(props.row.eventType) }}
                  </div>
                  <div class="text-caption text-xy-muted">
                    {{ formatCondition(props.row.eventType, props.row.condition) }}
                  </div>
                </div>
                <div v-if="ruleNeedsSuperUser(props.row)" class="rule-not-sent">
                  <q-badge color="warning" label="Won't send" />
                  <span>{{ ruleNotSentReason }}</span>
                </div>
                <q-toggle
                  v-else-if="hasAlertsManage"
                  :aria-label="`Toggle ${alertEventTypeLabel(props.row.eventType)} rule enabled`"
                  :model-value="props.row.enabled"
                  color="positive"
                  dense
                  @update:model-value="toggleRuleEnabled(props.row)" />
                <q-badge
                  v-else
                  :color="props.row.enabled ? 'positive' : 'grey-8'"
                  :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
              </q-card-section>

              <q-card-section class="notification-mobile-card__fields q-pt-none">
                <div>
                  <span>Target</span>
                  <strong>{{ resolveTargetName(props.row) }}</strong>
                </div>
                <div>
                  <span>Channel</span>
                  <strong>{{ resolveChannelName(props.row.notificationChannelId) }}</strong>
                </div>
              </q-card-section>

              <q-card-actions v-if="hasAlertsManage" align="right">
                <q-btn
                  v-if="!ruleNeedsSuperUser(props.row)"
                  :aria-label="`Edit ${alertEventTypeLabel(props.row.eventType)} alert rule`"
                  flat
                  icon="edit"
                  label="Edit"
                  no-caps
                  @click="openRuleEditDialog(props.row)" />
                <q-btn
                  :aria-label="`Delete ${alertEventTypeLabel(props.row.eventType)} alert rule`"
                  class="text-error-brighter"
                  flat
                  icon="delete"
                  label="Delete"
                  no-caps
                  @click="confirmDeleteRule(props.row)" />
              </q-card-actions>
            </q-card>
          </template>
          <template #body-cell-eventType="props">
            <q-td :props="props">
              <q-badge :label="alertEventTypeLabel(props.row.eventType)" color="grey-8" />
            </q-td>
          </template>
          <template #body-cell-server="props">
            <q-td :props="props">
              {{ resolveTargetName(props.row) }}
            </q-td>
          </template>
          <template #body-cell-channelName="props">
            <q-td :props="props">
              {{ resolveChannelName(props.row.notificationChannelId) }}
            </q-td>
          </template>
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <div v-if="ruleNeedsSuperUser(props.row)" class="rule-not-sent">
                <q-badge color="warning" label="Won't send" />
                <span>{{ ruleNotSentReason }}</span>
              </div>
              <q-toggle
                v-else-if="hasAlertsManage"
                :aria-label="`Toggle ${alertEventTypeLabel(props.row.eventType)} rule enabled`"
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
              <div v-if="hasAlertsManage" class="xy-row-actions">
                <q-btn
                  v-if="!ruleNeedsSuperUser(props.row)"
                  :aria-label="`Edit ${alertEventTypeLabel(props.row.eventType)} alert rule`"
                  dense
                  flat
                  icon="edit"
                  round
                  @click="openRuleEditDialog(props.row)">
                  <q-tooltip>Edit</q-tooltip>
                </q-btn>
                <q-btn
                  :aria-label="`Delete ${alertEventTypeLabel(props.row.eventType)} alert rule`"
                  class="text-error-brighter"
                  dense
                  flat
                  icon="delete"
                  round
                  @click="confirmDeleteRule(props.row)">
                  <q-tooltip>Delete</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <empty-state
              v-if="!rulesLoading && !rulesError"
              :description="rulesEmptyDescription"
              icon="rule"
              title="No alert rules">
              <template v-if="hasAlertsManage" #actions>
                <q-btn flat icon="dns" label="Game servers" no-caps to="/game-servers" />
                <q-btn
                  v-if="isSuperUser"
                  flat
                  icon="device_hub"
                  label="Nodes"
                  no-caps
                  to="/nodes" />
              </template>
            </empty-state>
          </template>
        </q-table>
      </q-tab-panel>

      <!-- Alert History Tab -->
      <q-tab-panel name="history">
        <div class="tab-toolbar">
          <q-select
            v-model="historyEventFilter"
            :options="alertEventTypeOptions"
            aria-label="Filter alert history by event type"
            class="filter-select"
            clearable
            dense
            emit-value
            label="Filter by event type"
            map-options
            outlined />
        </div>
        <q-table
          aria-label="Alert history"
          :columns="historyColumns"
          :grid="$q.screen.lt.md"
          :loading="historyLoading"
          :rows="filteredHistory"
          :rows-per-page-options="[10, 25, 50]"
          class="xy-standalone-table"
          flat
          hide-header-in-grid
          row-key="id">
          <template #item="props">
            <q-card
              :aria-label="`${alertEventTypeLabel(props.row.eventType)} alert history entry`"
              bordered
              class="notification-mobile-card"
              flat
              role="article">
              <q-card-section class="notification-mobile-card__header">
                <div>
                  <div class="notification-mobile-card__title">
                    {{ alertEventTypeLabel(props.row.eventType) }}
                  </div>
                  <div class="text-caption text-xy-muted">
                    {{ formatTimestamp(props.row.createdAt, 'Unknown time') }}
                  </div>
                </div>
                <q-badge
                  :color="deliveryStatusColor(props.row.deliveryStatus)"
                  :label="deliveryStatusLabel(props.row.deliveryStatus)" />
              </q-card-section>

              <q-card-section class="notification-mobile-card__fields q-pt-none">
                <div>
                  <span>Target</span>
                  <strong>{{ resolveTargetName(props.row) }}</strong>
                </div>
                <div>
                  <span>Channel</span>
                  <strong>{{ channelTypeLabel(props.row.channelType) }}</strong>
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
          <template #body-cell-eventType="props">
            <q-td :props="props">
              <q-badge :label="alertEventTypeLabel(props.row.eventType)" color="grey-8" />
            </q-td>
          </template>
          <template #body-cell-server="props">
            <q-td :props="props">
              {{ resolveTargetName(props.row) }}
            </q-td>
          </template>
          <template #body-cell-channelType="props">
            <q-td :props="props">
              <q-badge :label="channelTypeLabel(props.row.channelType)" color="grey-8" />
            </q-td>
          </template>
          <template #body-cell-deliveryStatus="props">
            <q-td :props="props">
              <q-badge
                :color="deliveryStatusColor(props.row.deliveryStatus)"
                :label="deliveryStatusLabel(props.row.deliveryStatus)" />
              <div v-if="props.row.deliveryError" class="delivery-error text-caption text-negative">
                {{ props.row.deliveryError }}
              </div>
            </q-td>
          </template>
          <template #no-data>
            <empty-state
              v-if="!historyLoading && !historyError"
              description="Alert events will appear here once rules are triggered."
              icon="history"
              title="No alert history" />
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

    <!-- Channel Create/Edit Dialog -->
    <q-dialog
      v-if="hasAlertsManage"
      v-model="showChannelDialog"
      aria-labelledby="notification-channel-dialog-title"
      persistent>
      <q-card class="channel-dialog-card">
        <q-form greedy @submit="saveChannel">
          <q-card-section>
            <div id="notification-channel-dialog-title" class="text-h6">
              {{ editingChannel ? 'Edit Channel' : 'Add Channel' }}
            </div>
          </q-card-section>

          <q-card-section class="q-pt-none">
            <q-input
              v-model="channelForm.name"
              :rules="[(val: string) => !!val || 'Name is required']"
              aria-required="true"
              aria-label="Channel name"
              class="q-mb-md"
              dense
              label="Name *"
              lazy-rules
              outlined />

            <q-select
              v-model="channelForm.channelType"
              :disable="!!editingChannel"
              :options="channelTypeOptions"
              aria-label="Channel type"
              class="q-mb-md"
              dense
              emit-value
              label="Channel Type"
              map-options
              outlined />

            <!-- Webhook config -->
            <!-- The URL carries the webhook's secret token, so it stays masked until revealed. -->
            <q-input
              v-if="isWebhookType(channelForm.channelType)"
              v-model="channelForm.webhookUrl"
              :rules="[(val: string) => !!val || 'Webhook URL is required']"
              :type="showWebhookUrl ? 'text' : 'password'"
              aria-required="true"
              aria-label="Webhook URL"
              autocomplete="new-password"
              class="q-mb-md"
              dense
              label="Webhook URL *"
              lazy-rules
              outlined>
              <template #append>
                <q-btn
                  :aria-label="showWebhookUrl ? 'Hide webhook URL' : 'Show webhook URL'"
                  :icon="showWebhookUrl ? 'visibility_off' : 'visibility'"
                  dense
                  flat
                  round
                  type="button"
                  @click="showWebhookUrl = !showWebhookUrl">
                  <q-tooltip>{{
                    showWebhookUrl ? 'Hide webhook URL' : 'Show webhook URL'
                  }}</q-tooltip>
                </q-btn>
              </template>
            </q-input>

            <!-- Email config -->
            <template v-if="channelForm.channelType === NotificationChannelType.EMAIL">
              <q-input
                v-model="channelForm.emailTo"
                :rules="[(val: string) => !!val || 'Email is required']"
                aria-required="true"
                aria-label="Recipient email address"
                class="q-mb-md"
                dense
                label="Recipient Email *"
                lazy-rules
                outlined />
              <q-select
                v-model="channelForm.smtpSource"
                :options="smtpSourceOptions"
                aria-label="Email delivery source"
                class="q-mb-md"
                dense
                emit-value
                label="Email Delivery"
                map-options
                outlined />
              <div v-if="channelForm.smtpSource === 'controller'" class="q-mb-md">
                <q-badge
                  :color="controllerEmailConfigured ? 'positive' : 'warning'"
                  :label="
                    controllerEmailConfigured
                      ? 'Controller email configured'
                      : 'Controller email not configured'
                  " />
                <div class="text-caption text-xy-secondary q-mt-sm">
                  Delivery provider and sender come from Admin -> Controller Settings.
                </div>
              </div>
              <template v-else>
                <q-input
                  v-model="channelForm.smtpHost"
                  aria-label="SMTP host"
                  class="q-mb-md"
                  dense
                  label="SMTP Host"
                  outlined />
                <q-input
                  v-model.number="channelForm.smtpPort"
                  aria-label="SMTP port"
                  class="q-mb-md"
                  dense
                  label="SMTP Port"
                  outlined
                  type="number" />
                <q-input
                  v-model="channelForm.smtpUser"
                  aria-label="SMTP username"
                  class="q-mb-md"
                  dense
                  label="SMTP Username"
                  outlined />
                <q-input
                  v-model="channelForm.smtpPassword"
                  :hint="
                    channelForm.hasExistingSmtpPassword
                      ? 'Leave blank to keep the current password.'
                      : undefined
                  "
                  aria-label="SMTP password"
                  autocomplete="new-password"
                  class="q-mb-md"
                  dense
                  label="SMTP Password"
                  outlined
                  type="password" />
                <q-input
                  v-model="channelForm.smtpFrom"
                  aria-label="SMTP from address"
                  class="q-mb-md"
                  dense
                  label="SMTP From Address"
                  outlined />
                <q-toggle
                  v-model="channelForm.smtpTLSEnabled"
                  aria-label="SMTP TLS enabled"
                  class="q-mb-md"
                  label="TLS Enabled" />
              </template>
            </template>
          </q-card-section>

          <q-card-actions align="right">
            <q-btn flat label="Cancel" no-caps @click="showChannelDialog = false" />
            <q-btn
              :label="editingChannel ? 'Save' : 'Create'"
              :loading="channelSaving"
              color="primary"
              no-caps
              type="submit" />
          </q-card-actions>
        </q-form>
      </q-card>
    </q-dialog>

    <alert-rule-dialog
      v-if="hasAlertsManage"
      v-model="showRuleDialog"
      :rule="editingRule"
      @saved="loadRules" />
  </q-page>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { notifyConnectError, notifyError, notifySuccess } from '@/api/notifications'
import AlertRuleDialog from '@/components/alerts/AlertRuleDialog.vue'
import EmptyState from '@/components/shared/EmptyState.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import type {
  AlertHistoryEntry,
  AlertRule,
  GameServer,
  Node,
  NotificationChannel,
} from '@/proto/shared_pb'
import { AlertEventType, DeliveryStatus, NotificationChannelType } from '@/proto/shared_pb'
import {
  CreateNotificationChannelRequestSchema,
  DeleteAlertRuleRequestSchema,
  DeleteNotificationChannelRequestSchema,
  GetAlertHistoryRequestSchema,
  GetLocalSMTPStatusRequestSchema,
  ListAlertRulesRequestSchema,
  ListGameServersRequestSchema,
  ListNodesRequestSchema,
  ListNotificationChannelsRequestSchema,
  TestNotificationChannelRequestSchema,
  UpdateAlertRuleRequestSchema,
  UpdateNotificationChannelRequestSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { formatAlertEventData, formatCondition } from '@/utils/alert-conditions'
import { canManageAlerts } from '@/utils/alert-permissions'
import {
  alertEventTypeLabel,
  alertEventTypeOptions,
  alertTargetName,
  isNodeAlertEventType,
} from '@/utils/alert-scope'
import { formatTimestamp } from '@/utils/format-timestamp'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const $q = useQuasar()
const authStore = useUserAuthStore()
const hasAlertsManage = computed(() => canManageAlerts(authStore.user, authStore.initialResponse))
const isSuperUser = computed(() => authStore.user?.superUser === true)

// ─── Tab state ───────────────────────────────────────────────────────────────
// ?tab=rules or ?tab=history opens that tab; plain /notifications opens Channels.
const route = useRoute()
const requestedTab = route.query['tab']
const activeTab = ref<'channels' | 'rules' | 'history'>(
  requestedTab === 'rules' || requestedTab === 'history' ? requestedTab : 'channels',
)

// ─── Game Servers and Nodes (for name resolution) ───────────────────────────
const gameServers = ref<GameServer[]>([])
const nodes = ref<Node[]>([])

async function loadGameServers(): Promise<void> {
  try {
    const response = await GetXylonaClient().listGameServers(
      create(ListGameServersRequestSchema, {}),
    )
    gameServers.value = response.gameServers
  } catch {
    // Non-critical -- name resolution falls back gracefully
  }
}

async function loadNodes(): Promise<void> {
  try {
    const response = await GetXylonaClient().listNodes(create(ListNodesRequestSchema, {}))
    nodes.value = response.nodes
  } catch {
    // Non-critical -- name resolution falls back to the node id
  }
}

function resolveTargetName(row: AlertRule | AlertHistoryEntry): string {
  return alertTargetName(row, gameServers.value, nodes.value)
}

// ─── Channels ────────────────────────────────────────────────────────────────
const channels = ref<NotificationChannel[]>([])
const channelsLoading = ref(false)
// Each list owns its load error and clears it on success, so its empty state never
// stands in for an outage and a later refresh that works removes the banner.
const channelsError = ref('')

const channelColumns = [
  {
    name: 'name',
    label: 'Name',
    align: 'left' as const,
    field: (row: NotificationChannel) => row.name,
    sortable: true,
  },
  {
    name: 'channelType',
    label: 'Type',
    align: 'left' as const,
    field: (row: NotificationChannel) => row.channelType,
    sortable: true,
  },
  {
    name: 'enabled',
    label: 'Enabled',
    align: 'center' as const,
    field: (row: NotificationChannel) => row.enabled,
    sortable: true,
  },
  {
    name: 'createdAt',
    label: 'Created',
    align: 'left' as const,
    field: (row: NotificationChannel) => formatTimestamp(row.createdAt),
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => '',
    classes: 'xy-col-actions',
    headerClasses: 'xy-col-actions',
  },
]

async function loadChannels(): Promise<void> {
  channelsLoading.value = true
  try {
    const response = await GetXylonaClient().listNotificationChannels(
      create(ListNotificationChannelsRequestSchema, {}),
    )
    channels.value = response.channels
    channelsError.value = ''
  } catch (unknownErr: unknown) {
    channelsError.value =
      'Failed to load channels: ' + ConnectErrorToString(ConnectError.from(unknownErr))
  } finally {
    channelsLoading.value = false
  }
}

async function toggleChannelEnabled(channel: NotificationChannel): Promise<void> {
  if (!hasAlertsManage.value) return

  try {
    await GetXylonaClient().updateNotificationChannel(
      create(UpdateNotificationChannelRequestSchema, {
        id: channel.id,
        name: channel.name,
        config: channel.config,
        enabled: !channel.enabled,
      }),
    )
    await loadChannels()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr, 'Failed to update channel')
  }
}

function confirmDeleteChannel(channel: NotificationChannel): void {
  if (!hasAlertsManage.value) return

  $q.dialog({
    title: 'Delete Channel',
    message: `Are you sure you want to delete the channel "${channel.name}"? Any alert rules using this channel will also be removed.`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Delete' },
    persistent: true,
  }).onOk(async () => {
    try {
      await GetXylonaClient().deleteNotificationChannel(
        create(DeleteNotificationChannelRequestSchema, { id: channel.id }),
      )
      notifySuccess(`Channel "${channel.name}" deleted`)
      await loadChannels()
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr, 'Failed to delete channel')
    }
  })
}

// ─── Channel Dialog ──────────────────────────────────────────────────────────
const showChannelDialog = ref(false)
const channelSaving = ref(false)
const editingChannel = ref<NotificationChannel | null>(null)
const controllerEmailConfigured = ref(false)
const showWebhookUrl = ref(false)

type SMTPSource = 'controller' | 'custom'

interface ChannelForm {
  name: string
  channelType: NotificationChannelType
  webhookUrl: string
  emailTo: string
  smtpSource: SMTPSource
  smtpHost: string
  smtpPort: number
  smtpUser: string
  smtpPassword: string
  hasExistingSmtpPassword: boolean
  smtpFrom: string
  smtpTLSEnabled: boolean
  enabled: boolean
}

function defaultChannelForm(): ChannelForm {
  return {
    name: '',
    channelType: NotificationChannelType.WEBHOOK_DISCORD,
    webhookUrl: '',
    emailTo: '',
    smtpSource: 'controller',
    smtpHost: '',
    smtpPort: 587,
    smtpUser: '',
    smtpPassword: '',
    hasExistingSmtpPassword: false,
    smtpFrom: '',
    smtpTLSEnabled: true,
    enabled: true,
  }
}

const channelForm = ref<ChannelForm>(defaultChannelForm())

const smtpSourceOptions = [
  { label: 'Use Controller Settings', value: 'controller' },
  { label: 'Use Custom SMTP', value: 'custom' },
]

async function loadLocalSMTPStatus(): Promise<void> {
  try {
    const response = await GetXylonaClient().getLocalSMTPStatus(
      create(GetLocalSMTPStatusRequestSchema, {}),
    )
    controllerEmailConfigured.value = response.configured
  } catch {
    controllerEmailConfigured.value = false
  }
}

watch(
  () => channelForm.value.channelType,
  async (channelType) => {
    if (channelType === NotificationChannelType.EMAIL) {
      await loadLocalSMTPStatus()
    }
  },
)

function openChannelDialog(channel: NotificationChannel | null): void {
  if (!hasAlertsManage.value) return

  editingChannel.value = channel
  showWebhookUrl.value = false
  if (channel) {
    channelForm.value.name = channel.name
    channelForm.value.channelType = channel.channelType
    channelForm.value.enabled = channel.enabled

    // Parse config JSON
    try {
      const config = JSON.parse(channel.config || '{}') as Record<string, unknown>
      if (isWebhookType(channel.channelType)) {
        channelForm.value.webhookUrl = (config.url as string) || ''
      } else if (channel.channelType === NotificationChannelType.EMAIL) {
        channelForm.value.emailTo = (config.to as string) || ''
        channelForm.value.smtpSource = config.smtp_source === 'custom' ? 'custom' : 'controller'
        channelForm.value.smtpHost = (config.smtp_host as string) || ''
        channelForm.value.smtpPort = (config.smtp_port as number) || 587
        channelForm.value.smtpUser = (config.smtp_user as string) || ''
        channelForm.value.smtpPassword = ''
        channelForm.value.hasExistingSmtpPassword = Boolean(config.smtp_password_configured)
        channelForm.value.smtpFrom = (config.smtp_from as string) || ''
        channelForm.value.smtpTLSEnabled =
          typeof config.smtp_tls_enabled === 'boolean' ? Boolean(config.smtp_tls_enabled) : true
      }
    } catch {
      const defaults = defaultChannelForm()
      channelForm.value.webhookUrl = defaults.webhookUrl
      channelForm.value.emailTo = defaults.emailTo
      channelForm.value.smtpSource = defaults.smtpSource
      channelForm.value.smtpHost = defaults.smtpHost
      channelForm.value.smtpPort = defaults.smtpPort
      channelForm.value.smtpUser = defaults.smtpUser
      channelForm.value.smtpPassword = defaults.smtpPassword
      channelForm.value.hasExistingSmtpPassword = defaults.hasExistingSmtpPassword
      channelForm.value.smtpFrom = defaults.smtpFrom
      channelForm.value.smtpTLSEnabled = defaults.smtpTLSEnabled
    }
  } else {
    channelForm.value = defaultChannelForm()
  }
  showChannelDialog.value = true
}

function buildConfigJson(): string {
  if (isWebhookType(channelForm.value.channelType)) {
    return JSON.stringify({ url: channelForm.value.webhookUrl })
  }
  if (channelForm.value.channelType === NotificationChannelType.EMAIL) {
    if (channelForm.value.smtpSource === 'controller') {
      return JSON.stringify({
        to: channelForm.value.emailTo,
        smtp_source: 'controller',
      })
    }

    return JSON.stringify({
      to: channelForm.value.emailTo,
      smtp_source: 'custom',
      smtp_host: channelForm.value.smtpHost,
      smtp_port: channelForm.value.smtpPort,
      smtp_user: channelForm.value.smtpUser,
      smtp_password: channelForm.value.smtpPassword,
      smtp_from: channelForm.value.smtpFrom,
      smtp_tls_enabled: channelForm.value.smtpTLSEnabled,
    })
  }
  return '{}'
}

async function saveChannel(): Promise<void> {
  // The q-form validates the required fields before it emits submit.
  if (!hasAlertsManage.value) return

  if (
    channelForm.value.channelType === NotificationChannelType.EMAIL &&
    channelForm.value.smtpSource === 'controller' &&
    !controllerEmailConfigured.value
  ) {
    notifyError('Controller email is not configured in Admin -> Controller Settings', {
      timeout: 3000,
    })
    return
  }

  channelSaving.value = true
  try {
    const config = buildConfigJson()
    if (editingChannel.value) {
      await GetXylonaClient().updateNotificationChannel(
        create(UpdateNotificationChannelRequestSchema, {
          id: editingChannel.value.id,
          name: channelForm.value.name,
          config,
          enabled: channelForm.value.enabled,
        }),
      )
      notifySuccess('Channel updated')
    } else {
      await GetXylonaClient().createNotificationChannel(
        create(CreateNotificationChannelRequestSchema, {
          name: channelForm.value.name,
          channelType: channelForm.value.channelType,
          config,
          enabled: channelForm.value.enabled,
        }),
      )
      notifySuccess('Channel created')
    }
    showChannelDialog.value = false
    await loadChannels()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr)
  } finally {
    channelSaving.value = false
  }
}

const testingChannelIds = ref(new Set<string>())

async function testChannel(channel: NotificationChannel): Promise<void> {
  testingChannelIds.value.add(channel.id)
  try {
    const response = await GetXylonaClient().testNotificationChannel(
      create(TestNotificationChannelRequestSchema, { id: channel.id }),
    )
    if (response.success) {
      notifySuccess('Check the channel for the test notification.', {
        message: `Test sent to ${channel.name}`,
        timeout: 4000,
      })
      return
    }

    notifyError(response.error || 'The channel did not accept the test notification.', {
      message: `Test to ${channel.name} failed`,
      timeout: 8000,
    })
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr, undefined, {
      message: `Test to ${channel.name} failed`,
      timeout: 8000,
    })
  } finally {
    testingChannelIds.value.delete(channel.id)
  }
}

// ─── Alert Rules ─────────────────────────────────────────────────────────────
const rules = ref<AlertRule[]>([])
const rulesLoading = ref(false)
const rulesError = ref('')
const rulesEventFilter = ref<AlertEventType | null>(null)
// Node pages are superuser-only, so only superusers are pointed at them.
const rulesEmptyDescription = computed(() =>
  isSuperUser.value
    ? "Add rules from a game server's Alerts tab, or from a node's page for CPU, memory and disk."
    : "Add rules from a game server's Alerts tab.",
)

// Node alerts are only sent to superusers, and only superusers may edit node rules. A node
// rule a non-superuser created before that restriction stays listed with a note, so it can
// still be found and deleted.
const ruleNotSentReason = 'Node alerts only go to superusers.'
function ruleNeedsSuperUser(rule: AlertRule): boolean {
  return !isSuperUser.value && isNodeAlertEventType(rule.eventType)
}

const filteredRules = computed(() => {
  if (rulesEventFilter.value === null) return rules.value
  return rules.value.filter((rule) => rule.eventType === rulesEventFilter.value)
})

const ruleColumns = [
  {
    name: 'eventType',
    label: 'Event Type',
    align: 'left' as const,
    field: (row: AlertRule) => row.eventType,
    sortable: true,
  },
  {
    name: 'server',
    label: 'Target',
    align: 'left' as const,
    field: (row: AlertRule) => resolveTargetName(row),
    sortable: false,
  },
  {
    name: 'condition',
    label: 'Condition',
    align: 'left' as const,
    field: (row: AlertRule) => formatCondition(row.eventType, row.condition),
    sortable: false,
  },
  {
    name: 'channelName',
    label: 'Channel',
    align: 'left' as const,
    field: (row: AlertRule) => row.notificationChannelId,
    sortable: false,
  },
  {
    name: 'enabled',
    label: 'Enabled',
    align: 'center' as const,
    field: (row: AlertRule) => row.enabled,
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => '',
    classes: 'xy-col-actions',
    headerClasses: 'xy-col-actions',
  },
]

async function loadRules(): Promise<void> {
  rulesLoading.value = true
  try {
    const response = await GetXylonaClient().listAlertRules(create(ListAlertRulesRequestSchema, {}))
    rules.value = response.rules
    rulesError.value = ''
  } catch (unknownErr: unknown) {
    rulesError.value =
      'Failed to load alert rules: ' + ConnectErrorToString(ConnectError.from(unknownErr))
  } finally {
    rulesLoading.value = false
  }
}

async function toggleRuleEnabled(rule: AlertRule): Promise<void> {
  if (!hasAlertsManage.value || ruleNeedsSuperUser(rule)) return

  try {
    await GetXylonaClient().updateAlertRule(
      create(UpdateAlertRuleRequestSchema, {
        id: rule.id,
        serverId: rule.serverId,
        serverNodeId: rule.serverNodeId,
        nodeId: rule.nodeId,
        eventType: rule.eventType,
        condition: rule.condition,
        notificationChannelId: rule.notificationChannelId,
        enabled: !rule.enabled,
      }),
    )
    await loadRules()
  } catch (unknownErr: unknown) {
    notifyConnectError(unknownErr, 'Failed to update rule')
  }
}

function confirmDeleteRule(rule: AlertRule): void {
  if (!hasAlertsManage.value) return

  $q.dialog({
    title: 'Delete Alert Rule',
    message: `Are you sure you want to delete this ${alertEventTypeLabel(rule.eventType)} alert rule?`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Delete' },
    persistent: true,
  }).onOk(async () => {
    try {
      await GetXylonaClient().deleteAlertRule(create(DeleteAlertRuleRequestSchema, { id: rule.id }))
      notifySuccess('Alert rule deleted')
      await loadRules()
    } catch (unknownErr: unknown) {
      notifyConnectError(unknownErr, 'Failed to delete alert rule')
    }
  })
}

function resolveChannelName(channelId: string): string {
  const channel = channels.value.find((ch) => ch.id === channelId)
  return channel ? channel.name : channelId
}

// ─── Rule Edit Dialog ────────────────────────────────────────────────────────
const showRuleDialog = ref(false)
const editingRule = ref<AlertRule | null>(null)

function openRuleEditDialog(rule: AlertRule): void {
  if (!hasAlertsManage.value || ruleNeedsSuperUser(rule)) return
  editingRule.value = rule
  showRuleDialog.value = true
}

// ─── Alert History ───────────────────────────────────────────────────────────
const historyEntries = ref<AlertHistoryEntry[]>([])
const historyLoading = ref(false)
const historyError = ref('')
const historyEventFilter = ref<AlertEventType | null>(null)
const historyPageSize = 50
const historyHasMore = ref(false)

const filteredHistory = computed(() => {
  if (historyEventFilter.value === null) return historyEntries.value
  return historyEntries.value.filter((entry) => entry.eventType === historyEventFilter.value)
})

const historyColumns = [
  {
    name: 'createdAt',
    label: 'Time',
    align: 'left' as const,
    field: (row: AlertHistoryEntry) => formatTimestamp(row.createdAt),
    sortable: true,
  },
  {
    name: 'eventType',
    label: 'Event',
    align: 'left' as const,
    field: (row: AlertHistoryEntry) => row.eventType,
    sortable: true,
  },
  {
    name: 'server',
    label: 'Target',
    align: 'left' as const,
    field: (row: AlertHistoryEntry) => resolveTargetName(row),
    sortable: false,
  },
  {
    name: 'channelType',
    label: 'Channel Type',
    align: 'left' as const,
    field: (row: AlertHistoryEntry) => row.channelType,
    sortable: true,
  },
  {
    name: 'deliveryStatus',
    label: 'Status',
    align: 'center' as const,
    field: (row: AlertHistoryEntry) => row.deliveryStatus,
    sortable: true,
  },
  {
    name: 'eventData',
    label: 'Details',
    align: 'left' as const,
    field: (row: AlertHistoryEntry) => formatAlertEventData(row.eventType, row.eventData),
    sortable: false,
    style: 'min-width: 16rem; white-space: normal;',
  },
]

async function loadHistory(append: boolean = false): Promise<void> {
  historyLoading.value = true
  try {
    const offset = append ? historyEntries.value.length : 0
    const response = await GetXylonaClient().getAlertHistory(
      create(GetAlertHistoryRequestSchema, {
        limit: historyPageSize,
        offset,
      }),
    )
    if (append) {
      historyEntries.value.push(...response.entries)
    } else {
      historyEntries.value = response.entries
    }
    historyHasMore.value = response.entries.length === historyPageSize
    historyError.value = ''
  } catch (unknownErr: unknown) {
    historyError.value =
      'Failed to load alert history: ' + ConnectErrorToString(ConnectError.from(unknownErr))
  } finally {
    historyLoading.value = false
  }
}

function loadMoreHistory(): void {
  void loadHistory(true)
}

// ─── Shared helpers ──────────────────────────────────────────────────────────

function isWebhookType(type: NotificationChannelType): boolean {
  return (
    type === NotificationChannelType.WEBHOOK_DISCORD ||
    type === NotificationChannelType.WEBHOOK_SLACK ||
    type === NotificationChannelType.WEBHOOK_GENERIC
  )
}

const channelTypeOptions = [
  { label: 'Discord Webhook', value: NotificationChannelType.WEBHOOK_DISCORD },
  { label: 'Slack Webhook', value: NotificationChannelType.WEBHOOK_SLACK },
  { label: 'Generic Webhook', value: NotificationChannelType.WEBHOOK_GENERIC },
  { label: 'Email', value: NotificationChannelType.EMAIL },
]

function channelTypeLabel(type: NotificationChannelType): string {
  switch (type) {
    case NotificationChannelType.WEBHOOK_DISCORD:
      return 'Discord'
    case NotificationChannelType.WEBHOOK_SLACK:
      return 'Slack'
    case NotificationChannelType.WEBHOOK_GENERIC:
      return 'Webhook'
    case NotificationChannelType.EMAIL:
      return 'Email'
    default:
      return 'Unknown'
  }
}

function deliveryStatusLabel(status: DeliveryStatus): string {
  switch (status) {
    case DeliveryStatus.PENDING:
      return 'Pending'
    case DeliveryStatus.SENT:
      return 'Sent'
    case DeliveryStatus.FAILED:
      return 'Failed'
    default:
      return 'Unknown'
  }
}

function deliveryStatusColor(status: DeliveryStatus): string {
  switch (status) {
    case DeliveryStatus.PENDING:
      return 'warning'
    case DeliveryStatus.SENT:
      return 'positive'
    case DeliveryStatus.FAILED:
      return 'negative'
    default:
      return 'grey'
  }
}

// ─── Lifecycle ───────────────────────────────────────────────────────────────
const loadError = computed(() => channelsError.value || rulesError.value || historyError.value)

async function loadPage(): Promise<void> {
  await Promise.all([loadGameServers(), loadNodes(), loadChannels(), loadRules(), loadHistory()])
}

onMounted(loadPage)
</script>

<style scoped>
.notifications-tabs {
  background-color: var(--xy-surface-1);
  border-radius: var(--xy-radius-lg) var(--xy-radius-lg) 0 0;
}

.notifications-panels {
  background-color: transparent;
}

.notifications-panels :deep(.q-tab-panel) {
  padding: var(--xy-space-md) 0;
}

.tab-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-bottom: var(--xy-space-md);
}

.filter-select {
  min-width: 220px;
}

.channel-dialog-card {
  min-width: 420px;
  max-width: 560px;
  width: 100%;
}

.rule-not-sent {
  display: grid;
  justify-items: center;
  gap: var(--xy-space-2xs);
  max-width: 14rem;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
  text-align: center;
}

.notification-mobile-card {
  width: 100%;
  background: var(--xy-surface-1);
  border-color: var(--xy-border);
}

.notification-mobile-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.notification-mobile-card__title {
  margin-bottom: var(--xy-space-xs);
  color: var(--xy-text-primary);
  font-weight: 600;
  overflow-wrap: anywhere;
}

.notification-mobile-card__fields {
  display: grid;
  gap: var(--xy-space-sm);
}

.delivery-error {
  max-width: 16rem;
  margin: var(--xy-space-2xs) auto 0;
  white-space: normal;
}

.notification-mobile-card__fields > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.notification-mobile-card__fields span {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.notification-mobile-card__fields strong {
  color: var(--xy-text-primary);
  font-weight: 500;
  overflow-wrap: anywhere;
}

@media (max-width: 480px) {
  .notifications-tabs :deep(.q-tab) {
    flex: 1 1 0;
    min-width: 0;
    padding-inline: var(--xy-space-xs);
  }

  .notifications-tabs :deep(.q-tab__label) {
    font-size: var(--xy-font-size-xs);
    letter-spacing: 0;
  }

  .notifications-tabs :deep(.q-tabs__arrow) {
    display: none;
  }

  .channel-dialog-card {
    min-width: unset;
  }
}
</style>
