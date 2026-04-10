<template>
  <game-server-form-shell
    :form-submitting="formSubmitting"
    :loading="loading"
    :save-disabled="loading"
    compact-header
    header-title="Server Settings"
    loading-text="Loading server settings..."
    test-id="settings-form-shell"
    @cancel="cancel"
    @save="submitGameServer">
    <q-form ref="formRef" class="server-form-layout" greedy>
      <section :class="{ 'form-section--last': !canEditProvisioning }" class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--accent">
            <q-icon name="badge" size="14px" />
          </span>
          <span class="section-title font-display">Identity</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model="gameServer.name"
            :rules="serverNameRules"
            class="col-12 col-md-6"
            data-testid="editable-name"
            label="Server Name *"
            lazy-rules
            maxlength="80"
            outlined
            reactive-rules
            type="text" />
          <q-select
            v-if="canEditProvisioning"
            v-model="gameServer.gameId"
            :options="availableGames"
            :rules="gameRules"
            class="col-12 col-md-6"
            data-testid="editable-game"
            emit-value
            label="Game *"
            lazy-rules
            map-options
            option-label="label"
            outlined
            reactive-rules
            @update:model-value="onGameSelected" />
        </div>
      </section>

      <template v-if="canEditProvisioning">
        <section class="form-section">
          <div class="section-header">
            <span class="section-icon section-icon--primary">
              <q-icon name="hub" size="14px" />
            </span>
            <span class="section-title font-display">Placement</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-select
              v-model="gameServer.userId"
              :options="availableUsers"
              :rules="ownerRules"
              class="col-12 col-md-4"
              data-testid="editable-owner"
              emit-value
              label="Owner *"
              lazy-rules
              map-options
              option-label="label"
              outlined
              reactive-rules />
            <q-select
              v-model="gameServer.nodeId"
              :options="nodes"
              :rules="nodeRules"
              class="col-12 col-md-4"
              data-testid="editable-node"
              emit-value
              label="Node *"
              lazy-rules
              map-options
              option-label="name"
              option-value="id"
              outlined
              reactive-rules />
            <q-select
              v-model="gameServer.ip"
              :options="availableIPs"
              :rules="ipRules"
              class="col-12 col-md-4"
              data-testid="editable-ip"
              label="IP Address *"
              lazy-rules
              option-label="address"
              outlined
              reactive-rules />
          </div>
        </section>

        <section class="form-section">
          <div class="section-header">
            <span class="section-icon section-icon--success">
              <q-icon name="lan" size="14px" />
            </span>
            <span class="section-title font-display">Networking</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md q-gutter-y-md full-width">
            <q-input
              v-model.number="portModel"
              :rules="portRules"
              class="col-12 col-sm-6"
              data-testid="editable-port"
              label="Port *"
              lazy-rules
              outlined
              reactive-rules
              type="number" />
            <q-input
              v-model.number="queryPortModel"
              :rules="queryPortRules"
              class="col-12 col-sm-6"
              data-testid="editable-query-port"
              label="Query Port *"
              lazy-rules
              outlined
              reactive-rules
              type="number" />
          </div>
        </section>
      </template>

      <game-server-provisioning-context
        v-else
        :capacity="provisioningCapacity"
        :connection="provisioningConnection"
        :executable="serverExecutableSummary"
        :game="selectedGameName"
        :memory="`${maxMemoryModel || 0} MB`"
        :node="selectedNodeName"
        :owner="selectedOwnerName"
        :show-memory="isMinecraftGame" />

      <section v-if="canEditProvisioning" class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--muted">
            <q-icon name="terminal" size="14px" />
          </span>
          <span class="section-title font-display">Launch</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model="gameServer.serverExecutable"
            class="col-12 col-lg-6"
            data-testid="editable-server-executable"
            hint="Optional override for the {{SERVER_EXECUTABLE}} launch placeholder."
            label="Server Executable"
            outlined
            type="text" />
        </div>
      </section>

      <section class="form-section">
        <div class="section-header">
          <span class="section-icon section-icon--warning">
            <q-icon name="memory" size="14px" />
          </span>
          <span class="section-title font-display">Capacity</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <q-input
            v-model.number="setPlayersModel"
            :rules="setPlayersRules"
            class="col-12 col-sm-6 col-lg-4"
            data-testid="editable-set-players"
            label="Set Players *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-if="canEditProvisioning"
            v-model.number="maxPlayersModel"
            :rules="maxPlayersRules"
            class="col-12 col-sm-6 col-lg-4"
            data-testid="editable-max-players"
            label="Max Players *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-if="canEditProvisioning && isMinecraftGame"
            v-model.number="maxMemoryModel"
            :error="showMaxMemoryStateError"
            :error-message="maxMemoryStateMessage"
            :rules="maxMemoryRules"
            class="col-12 col-lg-4"
            data-testid="editable-max-memory"
            label="Max Memory MB *"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
        </div>
      </section>

      <section class="form-section" data-testid="auto-restart-section">
        <div class="section-header">
          <span class="section-icon section-icon--success">
            <q-icon name="restart_alt" size="14px" />
          </span>
          <span class="section-title font-display">Auto-Restart</span>
          <span class="section-line"></span>
        </div>
        <div class="row q-col-gutter-md q-gutter-y-md full-width">
          <div class="col-12">
            <q-toggle
              v-model="gameServer.autoRestartEnabled"
              color="primary"
              data-testid="auto-restart-enabled"
              label="Restart server automatically on unexpected exit" />
          </div>
          <q-input
            v-if="gameServer.autoRestartEnabled"
            v-model.number="autoRestartMaxRetriesModel"
            :rules="autoRestartMaxRetriesRules"
            class="col-12 col-sm-6"
            data-testid="auto-restart-max-retries"
            hint="Maximum restart attempts before giving up (resets after 5 min of stable uptime)."
            label="Max Retries"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
          <q-input
            v-if="gameServer.autoRestartEnabled"
            v-model.number="autoRestartCooldownModel"
            :rules="autoRestartCooldownRules"
            class="col-12 col-sm-6"
            data-testid="auto-restart-cooldown"
            hint="Initial delay before first retry. Doubles with each subsequent attempt."
            label="Base Cooldown (seconds)"
            lazy-rules
            outlined
            reactive-rules
            type="number" />
        </div>
      </section>

      <section class="form-section form-section--last" data-testid="backup-settings-section">
        <div class="section-header">
          <span class="section-icon section-icon--primary">
            <q-icon name="backup" size="14px" />
          </span>
          <span class="section-title font-display">Backup Settings</span>
          <span class="section-line"></span>
        </div>

        <div
          v-if="backupSettingsLoading"
          class="text-caption text-muted"
          data-testid="backup-settings-loading">
          Loading backup settings...
        </div>

        <template v-else>
          <div
            v-if="backupOverview.canManageSettings"
            class="row q-col-gutter-md q-gutter-y-md full-width">
            <div class="col-12">
              <q-toggle
                :model-value="backupSettings.backupsEnabled"
                color="primary"
                data-testid="backup-settings-enabled"
                label="Enable Backups"
                @update:model-value="backupSettings.backupsEnabled = $event" />
            </div>

            <q-input
              :model-value="backupSettings.backupDirectory"
              class="col-12 col-lg-8"
              data-testid="backup-settings-directory"
              dense
              label="Backup Directory"
              outlined
              @update:model-value="backupSettings.backupDirectory = $event" />

            <q-input
              :model-value="String(backupSettings.maxBackups)"
              class="col-12 col-sm-6 col-lg-4"
              data-testid="backup-settings-max-backups"
              dense
              label="Max Automated Backups"
              min="0"
              outlined
              type="number"
              @update:model-value="updateBackupMaxBackups($event)" />

            <div
              v-if="backupSettings.defaultBackupDirectory"
              class="col-12 text-caption text-muted"
              data-testid="backup-settings-default-directory">
              Default backup directory: {{ backupSettings.defaultBackupDirectory }}
            </div>

            <div class="col-12 row justify-end">
              <q-btn
                :loading="backupSettingsSaving"
                color="primary"
                data-testid="save-backup-settings"
                label="Save Backup Settings"
                no-caps
                @click="saveBackupSettings" />
            </div>
          </div>

          <div
            v-else
            class="row q-col-gutter-md q-gutter-y-sm full-width"
            data-testid="backup-settings-readonly">
            <div class="col-12 col-md-6">
              <div class="text-caption text-muted">Status</div>
              <div class="text-body2 text-primary">
                {{ formatBackupEnabled(backupSettings.backupsEnabled) }}
              </div>
            </div>
            <div class="col-12 col-md-6">
              <div class="text-caption text-muted">Max Automated Backups</div>
              <div class="text-body2 text-primary">{{ String(backupSettings.maxBackups) }}</div>
            </div>
          </div>
        </template>
      </section>
    </q-form>
  </game-server-form-shell>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import GameServerFormShell from './GameServerFormShell.vue'
import GameServerProvisioningContext from './GameServerProvisioningContext.vue'
import { useGameServerFormState } from './game-server-form-state'
import type { BackupSettings, GameServerBackupOverview } from '@/proto/shared_pb'
import {
  BackupSettingsSchema,
  EditGameServerRequest,
  EditGameServerRequestSchema,
  GameServerBackupOverviewSchema,
} from '@/proto/shared_pb'
import {
  GetBackupSettingsRequestSchema,
  GetGameServerBackupOverviewRequestSchema,
  UpdateBackupSettingsRequestSchema,
} from '@/proto/xylona_pb'

const props = defineProps<{
  canEditProvisioning: boolean
  gameServerId: string
}>()

const router = useRouter()
const $q = useQuasar()
const backupSettings = ref<BackupSettings>(create(BackupSettingsSchema))
const backupOverview = ref<GameServerBackupOverview>(create(GameServerBackupOverviewSchema))
const backupSettingsLoading = ref(true)
const backupSettingsSaving = ref(false)

const {
  autoRestartCooldownModel,
  autoRestartCooldownRules,
  autoRestartMaxRetriesModel,
  autoRestartMaxRetriesRules,
  availableGames,
  availableIPs,
  availableUsers,
  formRef,
  formSubmitting,
  gameRules,
  gameServer,
  initialize,
  ipRules,
  isMinecraftGame,
  loading,
  maxMemoryModel,
  maxMemoryRules,
  maxMemoryStateMessage,
  maxPlayersModel,
  maxPlayersRules,
  nodeRules,
  nodes,
  onGameSelected,
  ownerRules,
  portModel,
  portRules,
  provisioningCapacity,
  provisioningConnection,
  queryPortModel,
  queryPortRules,
  resetSubmissionState,
  selectedGameName,
  selectedNodeName,
  selectedOwnerName,
  serverExecutableSummary,
  serverNameRules,
  setPlayersModel,
  setPlayersRules,
  showMaxMemoryStateError,
  startSubmitting,
  validateBeforeSave,
} = useGameServerFormState({
  existingGameServerId: props.gameServerId,
  loadProvisioningOptions: props.canEditProvisioning,
})

onMounted(async () => {
  await initialize()
  await initializeBackupSettings()
})

async function cancel() {
  router.back()
}

async function initializeBackupSettings() {
  backupSettingsLoading.value = true

  try {
    const [overviewResponse, settingsResponse] = await Promise.all([
      GetXylonaClient().getGameServerBackupOverview(
        create(GetGameServerBackupOverviewRequestSchema, {
          gameServerId: props.gameServerId,
        }),
      ),
      GetXylonaClient().getBackupSettings(
        create(GetBackupSettingsRequestSchema, {
          gameServerId: props.gameServerId,
        }),
      ),
    ])

    if (overviewResponse.overview) {
      backupOverview.value = overviewResponse.overview
    } else {
      backupOverview.value = create(GameServerBackupOverviewSchema)
    }

    if (settingsResponse.settings) {
      backupSettings.value = create(BackupSettingsSchema, settingsResponse.settings)
    } else {
      backupSettings.value = create(BackupSettingsSchema)
    }
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load backup settings: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    backupSettingsLoading.value = false
  }
}

function updateBackupMaxBackups(value: string | number | null): void {
  const numericValue = Number(value)
  if (!Number.isFinite(numericValue) || numericValue < 0) {
    backupSettings.value.maxBackups = 0n
    return
  }

  backupSettings.value.maxBackups = BigInt(Math.floor(numericValue))
}

function formatBackupEnabled(enabled: boolean): string {
  return enabled ? 'Enabled' : 'Disabled'
}

async function saveBackupSettings() {
  backupSettingsSaving.value = true
  try {
    const response = await GetXylonaClient().updateBackupSettings(
      create(UpdateBackupSettingsRequestSchema, {
        gameServerId: props.gameServerId,
        backupsEnabled: backupSettings.value.backupsEnabled,
        backupDirectory: backupSettings.value.backupDirectory,
        maxBackups: backupSettings.value.maxBackups,
      }),
    )

    if (response.settings) {
      backupSettings.value = create(BackupSettingsSchema, response.settings)
    }

    await initializeBackupSettings()
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Backup settings saved successfully.',
      icon: 'task_alt',
    })
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to save backup settings: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    backupSettingsSaving.value = false
  }
}

async function submitGameServer() {
  const formValid = await validateBeforeSave(
    'Complete the required fields before saving this server.',
  )
  if (!formValid) {
    return
  }

  startSubmitting()

  try {
    const request: EditGameServerRequest = create(EditGameServerRequestSchema, {})
    request.serverId = props.gameServerId
    request.gameServer = gameServer.value

    await GetXylonaClient().editGameServer(request)
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Server settings saved successfully.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to save game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    resetSubmissionState()
  }
}
</script>
