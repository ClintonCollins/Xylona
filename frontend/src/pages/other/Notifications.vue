<template>
  <q-page class="xy-page-content">
    <div class="xy-page-header">
      <div class="xy-page-title">Notifications</div>
    </div>

    <q-tabs
      v-model="activeTab"
      dense
      class="notifications-tabs"
      active-color="primary"
      indicator-color="primary"
      align="left"
      narrow-indicator>
      <q-tab name="channels" label="Channels" />
      <q-tab name="rules" label="Alert Rules" />
      <q-tab name="history" label="Alert History" />
    </q-tabs>

    <q-separator />

    <q-tab-panels v-model="activeTab" animated class="notifications-panels">
      <!-- Channels Tab -->
      <q-tab-panel name="channels">
        <div class="tab-toolbar">
          <q-btn
            v-if="hasAlertsManage"
            color="primary"
            label="Add Channel"
            icon="add"
            no-caps
            @click="openChannelDialog(null)" />
        </div>
        <q-table
          flat
          class="xy-standalone-table"
          :rows="channels"
          :columns="channelColumns"
          row-key="id"
          :loading="channelsLoading"
          :rows-per-page-options="[10, 25, 50]"
          :grid="$q.screen.lt.md"
          hide-header-in-grid>
          <template #body-cell-channelType="props">
            <q-td :props="props">
              <q-badge
                :color="channelTypeBadgeColor(props.row.channelType)"
                :label="channelTypeLabel(props.row.channelType)" />
            </q-td>
          </template>
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-toggle
                v-if="hasAlertsManage"
                :model-value="props.row.enabled"
                color="positive"
                aria-label="Toggle channel enabled"
                @update:model-value="toggleChannelEnabled(props.row)" />
              <q-badge
                v-else
                :color="props.row.enabled ? 'positive' : 'negative'"
                :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div v-if="hasAlertsManage" class="q-gutter-xs row no-wrap items-center">
                <q-btn
                  v-if="props.row.channelType === NotificationChannelType.EMAIL"
                  flat
                  dense
                  icon="send"
                  aria-label="Test channel"
                  @click="testChannel(props.row)">
                  <q-tooltip>Test</q-tooltip>
                </q-btn>
                <q-btn
                  flat
                  dense
                  icon="edit"
                  aria-label="Edit channel"
                  @click="openChannelDialog(props.row)">
                  <q-tooltip>Edit</q-tooltip>
                </q-btn>
                <q-btn
                  flat
                  dense
                  icon="delete"
                  class="text-error-brighter"
                  aria-label="Delete channel"
                  @click="confirmDeleteChannel(props.row)">
                  <q-tooltip>Delete</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-lg text-xy-secondary">
              <q-icon name="notifications_off" size="3rem" class="q-mb-sm text-xy-muted" />
              <div class="text-subtitle1">No notification channels</div>
              <div class="text-caption text-xy-muted">
                {{
                  hasAlertsManage
                    ? 'Add a channel to start receiving alerts.'
                    : 'Notification channels will appear here once a user with alert management access creates them.'
                }}
              </div>
            </div>
          </template>
        </q-table>
      </q-tab-panel>

      <!-- Alert Rules Tab -->
      <q-tab-panel name="rules">
        <div class="tab-toolbar">
          <q-select
            v-model="rulesEventFilter"
            dense
            outlined
            clearable
            emit-value
            map-options
            label="Filter by event type"
            :options="eventTypeOptions"
            class="filter-select"
            aria-label="Filter alert rules by event type" />
        </div>
        <q-table
          flat
          class="xy-standalone-table"
          :rows="filteredRules"
          :columns="ruleColumns"
          row-key="id"
          :loading="rulesLoading"
          :rows-per-page-options="[10, 25, 50]"
          :grid="$q.screen.lt.md"
          hide-header-in-grid>
          <template #body-cell-eventType="props">
            <q-td :props="props">
              <q-badge color="primary" :label="eventTypeLabel(props.row.eventType)" />
            </q-td>
          </template>
          <template #body-cell-server="props">
            <q-td :props="props">
              {{ resolveServerName(props.row.serverId) }}
            </q-td>
          </template>
          <template #body-cell-channelName="props">
            <q-td :props="props">
              {{ resolveChannelName(props.row.notificationChannelId) }}
            </q-td>
          </template>
          <template #body-cell-enabled="props">
            <q-td :props="props">
              <q-toggle
                v-if="hasAlertsManage"
                :model-value="props.row.enabled"
                color="positive"
                aria-label="Toggle rule enabled"
                @update:model-value="toggleRuleEnabled(props.row)" />
              <q-badge
                v-else
                :color="props.row.enabled ? 'positive' : 'negative'"
                :label="props.row.enabled ? 'Enabled' : 'Disabled'" />
            </q-td>
          </template>
          <template #body-cell-actions="props">
            <q-td :props="props">
              <div v-if="hasAlertsManage" class="q-gutter-xs row no-wrap items-center">
                <q-btn
                  flat
                  dense
                  icon="edit"
                  aria-label="Edit rule"
                  @click="openRuleEditDialog(props.row)">
                  <q-tooltip>Edit</q-tooltip>
                </q-btn>
                <q-btn
                  flat
                  dense
                  icon="delete"
                  class="text-error-brighter"
                  aria-label="Delete alert rule"
                  @click="confirmDeleteRule(props.row)">
                  <q-tooltip>Delete</q-tooltip>
                </q-btn>
              </div>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-lg text-xy-secondary">
              <q-icon name="rule" size="3rem" class="q-mb-sm text-xy-muted" />
              <div class="text-subtitle1">No alert rules</div>
              <div class="text-caption text-xy-muted">
                Create alert rules from individual game server pages.
              </div>
            </div>
          </template>
        </q-table>
      </q-tab-panel>

      <!-- Alert History Tab -->
      <q-tab-panel name="history">
        <div class="tab-toolbar">
          <q-select
            v-model="historyEventFilter"
            dense
            outlined
            clearable
            emit-value
            map-options
            label="Filter by event type"
            :options="eventTypeOptions"
            class="filter-select"
            aria-label="Filter alert history by event type" />
        </div>
        <q-table
          flat
          class="xy-standalone-table"
          :rows="filteredHistory"
          :columns="historyColumns"
          row-key="id"
          :loading="historyLoading"
          :rows-per-page-options="[10, 25, 50]"
          :grid="$q.screen.lt.md"
          hide-header-in-grid>
          <template #body-cell-eventType="props">
            <q-td :props="props">
              <q-badge color="primary" :label="eventTypeLabel(props.row.eventType)" />
            </q-td>
          </template>
          <template #body-cell-server="props">
            <q-td :props="props">
              {{ resolveServerName(props.row.serverId) }}
            </q-td>
          </template>
          <template #body-cell-channelType="props">
            <q-td :props="props">
              <q-badge
                :color="channelTypeBadgeColor(props.row.channelType)"
                :label="channelTypeLabel(props.row.channelType)" />
            </q-td>
          </template>
          <template #body-cell-deliveryStatus="props">
            <q-td :props="props">
              <q-badge
                :color="deliveryStatusColor(props.row.deliveryStatus)"
                :label="deliveryStatusLabel(props.row.deliveryStatus)" />
              <q-tooltip v-if="props.row.deliveryError">
                {{ props.row.deliveryError }}
              </q-tooltip>
            </q-td>
          </template>
          <template #no-data>
            <div class="full-width column items-center q-pa-lg text-xy-secondary">
              <q-icon name="history" size="3rem" class="q-mb-sm text-xy-muted" />
              <div class="text-subtitle1">No alert history</div>
              <div class="text-caption text-xy-muted">
                Alert events will appear here once rules are triggered.
              </div>
            </div>
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
      </q-tab-panel>
    </q-tab-panels>

    <!-- Channel Create/Edit Dialog -->
    <q-dialog v-if="hasAlertsManage" v-model="showChannelDialog" persistent>
      <q-card class="channel-dialog-card">
        <q-card-section>
          <div class="text-h6">{{ editingChannel ? 'Edit Channel' : 'Add Channel' }}</div>
        </q-card-section>

        <q-card-section class="q-pt-none">
          <q-input
            v-model="channelForm.name"
            outlined
            dense
            label="Name"
            aria-label="Channel name"
            class="q-mb-md"
            :rules="[(val: string) => !!val || 'Name is required']" />

          <q-select
            v-model="channelForm.channelType"
            outlined
            dense
            emit-value
            map-options
            label="Channel Type"
            :options="channelTypeOptions"
            :disable="!!editingChannel"
            class="q-mb-md"
            aria-label="Channel type" />

          <!-- Webhook config -->
          <q-input
            v-if="isWebhookType(channelForm.channelType)"
            v-model="channelForm.webhookUrl"
            outlined
            dense
            label="Webhook URL"
            aria-label="Webhook URL"
            class="q-mb-md"
            :rules="[(val: string) => !!val || 'Webhook URL is required']" />

          <!-- Email config -->
          <template v-if="channelForm.channelType === NotificationChannelType.EMAIL">
            <q-input
              v-model="channelForm.emailTo"
              outlined
              dense
              label="Recipient Email"
              aria-label="Recipient email address"
              class="q-mb-md"
              :rules="[(val: string) => !!val || 'Email is required']" />
            <q-select
              v-model="channelForm.smtpSource"
              outlined
              dense
              emit-value
              map-options
              label="SMTP Source"
              :options="smtpSourceOptions"
              class="q-mb-md"
              aria-label="SMTP source" />
            <div v-if="channelForm.smtpSource === 'node'" class="q-mb-md">
              <q-badge
                :color="nodeSMTPConfigured ? 'positive' : 'warning'"
                :label="nodeSMTPConfigured ? 'Node SMTP configured' : 'Node SMTP not configured'" />
              <div class="text-caption text-xy-secondary q-mt-sm">
                Transport and from address come from Admin -> Node Settings.
              </div>
            </div>
            <template v-else>
              <q-input
                v-model="channelForm.smtpHost"
                outlined
                dense
                label="SMTP Host"
                aria-label="SMTP host"
                class="q-mb-md" />
              <q-input
                v-model.number="channelForm.smtpPort"
                outlined
                dense
                type="number"
                label="SMTP Port"
                aria-label="SMTP port"
                class="q-mb-md" />
              <q-input
                v-model="channelForm.smtpUser"
                outlined
                dense
                label="SMTP Username"
                aria-label="SMTP username"
                class="q-mb-md" />
              <q-input
                v-model="channelForm.smtpPassword"
                outlined
                dense
                type="password"
                label="SMTP Password"
                aria-label="SMTP password"
                class="q-mb-md"
                :hint="
                  channelForm.hasExistingSmtpPassword
                    ? 'Leave blank to keep the current password.'
                    : undefined
                " />
              <q-input
                v-model="channelForm.smtpFrom"
                outlined
                dense
                label="SMTP From Address"
                aria-label="SMTP from address"
                class="q-mb-md" />
              <q-toggle
                v-model="channelForm.smtpTLSEnabled"
                label="TLS Enabled"
                aria-label="SMTP TLS enabled"
                class="q-mb-md" />
            </template>
          </template>
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat label="Cancel" no-caps @click="showChannelDialog = false" />
          <q-btn
            color="primary"
            :label="editingChannel ? 'Save' : 'Create'"
            no-caps
            :loading="channelSaving"
            @click="saveChannel" />
        </q-card-actions>
      </q-card>
    </q-dialog>

    <!-- Rule Edit Dialog -->
    <q-dialog v-if="hasAlertsManage" v-model="showRuleDialog" persistent>
      <q-card class="channel-dialog-card">
        <q-card-section>
          <div class="text-h6">Edit Alert Rule</div>
        </q-card-section>

        <q-card-section class="q-pt-none">
          <q-select
            v-model="ruleForm.eventType"
            outlined
            dense
            emit-value
            map-options
            label="Event Type"
            :options="eventTypeOptions"
            class="q-mb-md"
            aria-label="Event type" />

          <!-- Threshold condition fields -->
          <div v-if="isThresholdType" class="row q-gutter-sm q-mb-md">
            <q-select
              v-model="ruleForm.thresholdOperator"
              outlined
              dense
              emit-value
              map-options
              label="Operator"
              :options="thresholdOperators"
              class="col-4"
              aria-label="Threshold operator" />
            <q-input
              v-model.number="ruleForm.thresholdValue"
              outlined
              dense
              type="number"
              label="Value"
              :suffix="thresholdUnit"
              class="col"
              aria-label="Threshold value" />
          </div>

          <!-- Status change condition fields -->
          <div v-if="isStatusChangeType" class="q-mb-md">
            <div class="text-caption q-mb-xs">Trigger on status:</div>
            <q-checkbox v-model="ruleForm.statusOnline" label="Online" />
            <q-checkbox v-model="ruleForm.statusOffline" label="Offline" />
          </div>

          <q-select
            v-model="ruleForm.notificationChannelId"
            outlined
            dense
            emit-value
            map-options
            label="Notification Channel"
            :options="channels.map((c) => ({ label: c.name, value: c.id }))"
            class="q-mb-md"
            aria-label="Notification channel" />

          <q-toggle v-model="ruleForm.enabled" label="Enabled" />
        </q-card-section>

        <q-card-actions align="right">
          <q-btn flat label="Cancel" no-caps @click="showRuleDialog = false" />
          <q-btn
            color="primary"
            label="Save"
            no-caps
            :loading="ruleSaving"
            :disable="!ruleForm.notificationChannelId"
            @click="saveRule" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </q-page>
</template>

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { ConnectError } from '@connectrpc/connect'
import dayjs from 'dayjs'
import { useQuasar } from 'quasar'
import { computed, onMounted, ref, watch } from 'vue'
import { NotificationChannelType, AlertEventType, DeliveryStatus } from '@/proto/shared_pb'
import type {
  NotificationChannel,
  AlertRule,
  AlertHistoryEntry,
  GameServer,
} from '@/proto/shared_pb'
import {
  ListNotificationChannelsRequestSchema,
  CreateNotificationChannelRequestSchema,
  UpdateNotificationChannelRequestSchema,
  DeleteNotificationChannelRequestSchema,
  GetLocalSMTPStatusRequestSchema,
  ListAlertRulesRequestSchema,
  UpdateAlertRuleRequestSchema,
  DeleteAlertRuleRequestSchema,
  GetAlertHistoryRequestSchema,
  ListGameServersRequestSchema,
  TestNotificationChannelRequestSchema,
} from '@/proto/xylona_pb'
import { useUserAuthStore } from '@/stores/xylona'
import { canManageAlerts } from '@/utils/alert-permissions'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const $q = useQuasar()
const authStore = useUserAuthStore()
const hasAlertsManage = computed(() => canManageAlerts(authStore.user, authStore.initialResponse))

// ─── Tab state ───────────────────────────────────────────────────────────────
const activeTab = ref<'channels' | 'rules' | 'history'>('channels')

// ─── Game Servers (for name resolution) ─────────────────────────────────────
const gameServers = ref<GameServer[]>([])

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

function resolveServerName(serverId: string | undefined): string {
  if (!serverId) return 'All Servers'
  const server = gameServers.value.find((gs) => gs.id === serverId)
  return server ? server.name : serverId
}

// ─── Channels ────────────────────────────────────────────────────────────────
const channels = ref<NotificationChannel[]>([])
const channelsLoading = ref(false)

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
    field: (row: NotificationChannel) =>
      row.createdAt ? dayjs(timestampDate(row.createdAt)).format('MM/DD/YYYY HH:mm A') : '',
    sortable: true,
  },
  {
    name: 'actions',
    label: '',
    align: 'center' as const,
    field: () => '',
  },
]

async function loadChannels(): Promise<void> {
  channelsLoading.value = true
  try {
    const response = await GetXylonaClient().listNotificationChannels(
      create(ListNotificationChannelsRequestSchema, {}),
    )
    channels.value = response.channels
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: 'Failed to load channels: ' + ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: 'Failed to update channel: ' + ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
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
      $q.notify({
        type: 'xylona-success',
        caption: `Channel "${channel.name}" deleted`,
        position: 'top',
        timeout: 3000,
      })
      await loadChannels()
    } catch (unknownErr: unknown) {
      const err = ConnectError.from(unknownErr)
      $q.notify({
        type: 'xylona-error',
        caption: 'Failed to delete channel: ' + ConnectErrorToString(err),
        position: 'top',
        timeout: 5000,
      })
    }
  })
}

// ─── Channel Dialog ──────────────────────────────────────────────────────────
const showChannelDialog = ref(false)
const channelSaving = ref(false)
const editingChannel = ref<NotificationChannel | null>(null)
const nodeSMTPConfigured = ref(false)

type SMTPSource = 'node' | 'custom'

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
    smtpSource: 'node',
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
  { label: 'Use Node Settings SMTP', value: 'node' },
  { label: 'Use Custom SMTP', value: 'custom' },
]

async function loadLocalSMTPStatus(): Promise<void> {
  try {
    const response = await GetXylonaClient().getLocalSMTPStatus(
      create(GetLocalSMTPStatusRequestSchema, {}),
    )
    nodeSMTPConfigured.value = response.configured
  } catch {
    nodeSMTPConfigured.value = false
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
        channelForm.value.smtpSource = ((config.smtp_source as SMTPSource) || 'node') as SMTPSource
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
    if (channelForm.value.smtpSource === 'node') {
      return JSON.stringify({
        to: channelForm.value.emailTo,
        smtp_source: 'node',
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
  if (!hasAlertsManage.value) return

  if (!channelForm.value.name) {
    $q.notify({
      type: 'xylona-error',
      caption: 'Channel name is required',
      position: 'top',
      timeout: 3000,
    })
    return
  }

  if (
    channelForm.value.channelType === NotificationChannelType.EMAIL &&
    channelForm.value.smtpSource === 'node' &&
    !nodeSMTPConfigured.value
  ) {
    $q.notify({
      type: 'xylona-error',
      caption: 'Node SMTP is not configured in Admin -> Node Settings',
      position: 'top',
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
      $q.notify({
        type: 'xylona-success',
        caption: 'Channel updated',
        position: 'top',
        timeout: 3000,
      })
    } else {
      await GetXylonaClient().createNotificationChannel(
        create(CreateNotificationChannelRequestSchema, {
          name: channelForm.value.name,
          channelType: channelForm.value.channelType,
          config,
          enabled: channelForm.value.enabled,
        }),
      )
      $q.notify({
        type: 'xylona-success',
        caption: 'Channel created',
        position: 'top',
        timeout: 3000,
      })
    }
    showChannelDialog.value = false
    await loadChannels()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    channelSaving.value = false
  }
}

async function testChannel(channel: NotificationChannel): Promise<void> {
  try {
    const response = await GetXylonaClient().testNotificationChannel(
      create(TestNotificationChannelRequestSchema, { id: channel.id }),
    )
    if (response.success) {
      $q.notify({
        type: 'xylona-success',
        caption: 'Test notification sent',
        position: 'top',
        timeout: 3000,
      })
      return
    }

    $q.notify({
      type: 'xylona-error',
      caption: response.error || 'Test notification failed',
      position: 'top',
      timeout: 5000,
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

// ─── Alert Rules ─────────────────────────────────────────────────────────────
const rules = ref<AlertRule[]>([])
const rulesLoading = ref(false)
const rulesEventFilter = ref<AlertEventType | null>(null)

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
    label: 'Server',
    align: 'left' as const,
    field: (row: AlertRule) => row.serverId || '',
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
  },
]

async function loadRules(): Promise<void> {
  rulesLoading.value = true
  try {
    const response = await GetXylonaClient().listAlertRules(create(ListAlertRulesRequestSchema, {}))
    rules.value = response.rules
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: 'Failed to load alert rules: ' + ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    rulesLoading.value = false
  }
}

async function toggleRuleEnabled(rule: AlertRule): Promise<void> {
  if (!hasAlertsManage.value) return

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
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: 'Failed to update rule: ' + ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  }
}

function confirmDeleteRule(rule: AlertRule): void {
  if (!hasAlertsManage.value) return

  $q.dialog({
    title: 'Delete Alert Rule',
    message: `Are you sure you want to delete this ${eventTypeLabel(rule.eventType)} alert rule?`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Delete' },
    persistent: true,
  }).onOk(async () => {
    try {
      await GetXylonaClient().deleteAlertRule(create(DeleteAlertRuleRequestSchema, { id: rule.id }))
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
        caption: 'Failed to delete alert rule: ' + ConnectErrorToString(err),
        position: 'top',
        timeout: 5000,
      })
    }
  })
}

function resolveChannelName(channelId: string): string {
  const channel = channels.value.find((ch) => ch.id === channelId)
  return channel ? channel.name : channelId
}

// ─── Rule Edit Dialog ────────────────────────────────────────────────────────
const showRuleDialog = ref(false)
const ruleSaving = ref(false)
const editingRule = ref<AlertRule | null>(null)

interface RuleForm {
  eventType: AlertEventType
  notificationChannelId: string
  enabled: boolean
  thresholdOperator: string
  thresholdValue: number
  statusOnline: boolean
  statusOffline: boolean
}

const ruleForm = ref<RuleForm>({
  eventType: AlertEventType.CRASH,
  notificationChannelId: '',
  enabled: true,
  thresholdOperator: '>=',
  thresholdValue: 80,
  statusOnline: true,
  statusOffline: true,
})

const thresholdOperators = [
  { label: '>=', value: '>=' },
  { label: '>', value: '>' },
  { label: '<=', value: '<=' },
  { label: '<', value: '<' },
  { label: '==', value: '==' },
]

const isThresholdType = computed(() => {
  return [
    AlertEventType.CPU_THRESHOLD,
    AlertEventType.MEMORY_THRESHOLD,
    AlertEventType.DISK_THRESHOLD,
    AlertEventType.PLAYER_COUNT_THRESHOLD,
    AlertEventType.NODE_CPU_THRESHOLD,
    AlertEventType.NODE_MEMORY_THRESHOLD,
    AlertEventType.NODE_DISK_THRESHOLD,
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
    case AlertEventType.NODE_CPU_THRESHOLD:
    case AlertEventType.NODE_MEMORY_THRESHOLD:
    case AlertEventType.NODE_DISK_THRESHOLD:
      return '%'
    case AlertEventType.PLAYER_COUNT_THRESHOLD:
      return 'players'
    default:
      return ''
  }
})

function openRuleEditDialog(rule: AlertRule): void {
  if (!hasAlertsManage.value) return

  editingRule.value = rule
  ruleForm.value = {
    eventType: rule.eventType,
    notificationChannelId: rule.notificationChannelId,
    enabled: rule.enabled,
    thresholdOperator: '>=',
    thresholdValue: 80,
    statusOnline: true,
    statusOffline: true,
  }
  parseConditionForEdit(rule.eventType, rule.condition)
  showRuleDialog.value = true
}

function parseConditionForEdit(eventType: AlertEventType, condition: string): void {
  if (!condition) return

  try {
    const parsed = JSON.parse(condition) as Record<string, unknown>

    if ('operator' in parsed && 'value' in parsed) {
      ruleForm.value.thresholdOperator = String(parsed.operator)
      ruleForm.value.thresholdValue = Number(parsed.value)
    }

    if ('statuses' in parsed && Array.isArray(parsed.statuses)) {
      const statuses = parsed.statuses as string[]
      ruleForm.value.statusOnline = statuses.includes('ONLINE')
      ruleForm.value.statusOffline = statuses.includes('OFFLINE')
    }
  } catch {
    // Ignore parse errors -- keep defaults
  }
}

function buildRuleConditionJson(): string {
  if (isThresholdType.value) {
    return JSON.stringify({
      operator: ruleForm.value.thresholdOperator,
      value: ruleForm.value.thresholdValue,
    })
  }

  if (isStatusChangeType.value) {
    const statuses: string[] = []
    if (ruleForm.value.statusOnline) statuses.push('ONLINE')
    if (ruleForm.value.statusOffline) statuses.push('OFFLINE')
    return JSON.stringify({ statuses })
  }

  return ''
}

async function saveRule(): Promise<void> {
  if (!hasAlertsManage.value) return

  if (!editingRule.value) return

  ruleSaving.value = true
  try {
    const condition = buildRuleConditionJson()
    await GetXylonaClient().updateAlertRule(
      create(UpdateAlertRuleRequestSchema, {
        id: editingRule.value.id,
        serverId: editingRule.value.serverId,
        serverNodeId: editingRule.value.serverNodeId,
        nodeId: editingRule.value.nodeId,
        eventType: ruleForm.value.eventType,
        condition,
        notificationChannelId: ruleForm.value.notificationChannelId,
        enabled: ruleForm.value.enabled,
      }),
    )
    $q.notify({
      type: 'xylona-success',
      caption: 'Alert rule updated',
      position: 'top',
      timeout: 3000,
    })
    showRuleDialog.value = false
    await loadRules()
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: ConnectErrorToString(err),
      position: 'top',
      timeout: 5000,
    })
  } finally {
    ruleSaving.value = false
  }
}

// ─── Alert History ───────────────────────────────────────────────────────────
const historyEntries = ref<AlertHistoryEntry[]>([])
const historyLoading = ref(false)
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
    field: (row: AlertHistoryEntry) =>
      row.createdAt ? dayjs(timestampDate(row.createdAt)).format('MM/DD/YYYY HH:mm:ss A') : '',
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
    label: 'Server',
    align: 'left' as const,
    field: (row: AlertHistoryEntry) => row.serverId || '',
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
    field: (row: AlertHistoryEntry) => row.eventData || '-',
    sortable: false,
    style: 'max-width: 300px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;',
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
  } catch (unknownErr: unknown) {
    const err = ConnectError.from(unknownErr)
    $q.notify({
      type: 'xylona-error',
      caption: 'Failed to load alert history: ' + ConnectErrorToString(err),
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

function channelTypeBadgeColor(type: NotificationChannelType): string {
  switch (type) {
    case NotificationChannelType.WEBHOOK_DISCORD:
      return 'purple'
    case NotificationChannelType.WEBHOOK_SLACK:
      return 'orange'
    case NotificationChannelType.WEBHOOK_GENERIC:
      return 'blue-grey'
    case NotificationChannelType.EMAIL:
      return 'teal'
    default:
      return 'grey'
  }
}

const eventTypeOptions = [
  { label: 'Crash', value: AlertEventType.CRASH },
  { label: 'Status Change', value: AlertEventType.STATUS_CHANGE },
  { label: 'CPU Threshold', value: AlertEventType.CPU_THRESHOLD },
  { label: 'Memory Threshold', value: AlertEventType.MEMORY_THRESHOLD },
  { label: 'Disk Threshold', value: AlertEventType.DISK_THRESHOLD },
  { label: 'Player Count', value: AlertEventType.PLAYER_COUNT_THRESHOLD },
  { label: 'Node CPU', value: AlertEventType.NODE_CPU_THRESHOLD },
  { label: 'Node Memory', value: AlertEventType.NODE_MEMORY_THRESHOLD },
  { label: 'Node Disk', value: AlertEventType.NODE_DISK_THRESHOLD },
]

function eventTypeLabel(type: AlertEventType): string {
  switch (type) {
    case AlertEventType.CRASH:
      return 'Crash'
    case AlertEventType.STATUS_CHANGE:
      return 'Status Change'
    case AlertEventType.CPU_THRESHOLD:
      return 'CPU Threshold'
    case AlertEventType.MEMORY_THRESHOLD:
      return 'Memory Threshold'
    case AlertEventType.DISK_THRESHOLD:
      return 'Disk Threshold'
    case AlertEventType.PLAYER_COUNT_THRESHOLD:
      return 'Player Count'
    case AlertEventType.NODE_CPU_THRESHOLD:
      return 'Node CPU'
    case AlertEventType.NODE_MEMORY_THRESHOLD:
      return 'Node Memory'
    case AlertEventType.NODE_DISK_THRESHOLD:
      return 'Node Disk'
    default:
      return 'Unknown'
  }
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
        AlertEventType.NODE_CPU_THRESHOLD,
        AlertEventType.NODE_MEMORY_THRESHOLD,
        AlertEventType.NODE_DISK_THRESHOLD,
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
onMounted(async () => {
  await Promise.all([loadGameServers(), loadChannels(), loadRules(), loadHistory()])
})
</script>

<style scoped>
.notifications-tabs {
  background-color: var(--xy-surface-1);
  border-radius: 8px 8px 0 0;
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

@media (max-width: 480px) {
  .channel-dialog-card {
    min-width: unset;
  }
}
</style>
