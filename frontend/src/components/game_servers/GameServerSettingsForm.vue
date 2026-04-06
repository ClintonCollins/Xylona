<template>
  <game-server-form-shell
    compact-header
    :form-submitting="formSubmitting"
    header-title="Server Settings"
    :loading="loading"
    :save-disabled="loading"
    loading-text="Loading server settings..."
    test-id="settings-form-shell"
    @cancel="cancel"
    @save="submitGameServer">
    <q-form ref="formRef" greedy class="server-form-layout">
      <section class="form-section" :class="{ 'form-section--last': !canEditProvisioning }">
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
            data-testid="editable-name"
            class="col-12 col-md-6"
            outlined
            type="text"
            label="Server Name *"
            :rules="serverNameRules"
            reactive-rules
            lazy-rules
            maxlength="80" />
          <q-select
            v-if="canEditProvisioning"
            v-model="gameServer.gameId"
            data-testid="editable-game"
            class="col-12 col-md-6"
            outlined
            label="Game *"
            emit-value
            :options="availableGames"
            option-label="label"
            map-options
            :rules="gameRules"
            reactive-rules
            lazy-rules
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
              data-testid="editable-owner"
              class="col-12 col-md-4"
              outlined
              label="Owner *"
              emit-value
              :options="availableUsers"
              option-label="label"
              map-options
              :rules="ownerRules"
              reactive-rules
              lazy-rules />
            <q-select
              v-model="gameServer.nodeId"
              data-testid="editable-node"
              class="col-12 col-md-4"
              outlined
              label="Node *"
              emit-value
              :options="nodes"
              option-label="name"
              map-options
              option-value="id"
              :rules="nodeRules"
              reactive-rules
              lazy-rules />
            <q-select
              v-model="gameServer.ip"
              data-testid="editable-ip"
              class="col-12 col-md-4"
              outlined
              label="IP Address *"
              :options="availableIPs"
              option-label="address"
              :rules="ipRules"
              reactive-rules
              lazy-rules />
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
              data-testid="editable-port"
              class="col-12 col-sm-6"
              outlined
              type="number"
              label="Port *"
              :rules="portRules"
              reactive-rules
              lazy-rules />
            <q-input
              v-model.number="queryPortModel"
              data-testid="editable-query-port"
              class="col-12 col-sm-6"
              outlined
              type="number"
              label="Query Port *"
              :rules="queryPortRules"
              reactive-rules
              lazy-rules />
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
            data-testid="editable-server-executable"
            class="col-12 col-lg-6"
            outlined
            type="text"
            label="Server Executable"
            hint="Optional override for the {{SERVER_EXECUTABLE}} launch placeholder." />
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
            data-testid="editable-set-players"
            class="col-12 col-sm-6 col-lg-4"
            outlined
            type="number"
            label="Set Players *"
            :rules="setPlayersRules"
            reactive-rules
            lazy-rules />
          <q-input
            v-if="canEditProvisioning"
            v-model.number="maxPlayersModel"
            data-testid="editable-max-players"
            class="col-12 col-sm-6 col-lg-4"
            outlined
            type="number"
            label="Max Players *"
            :rules="maxPlayersRules"
            reactive-rules
            lazy-rules />
          <q-input
            v-if="canEditProvisioning && isMinecraftGame"
            v-model.number="maxMemoryModel"
            data-testid="editable-max-memory"
            class="col-12 col-lg-4"
            outlined
            type="number"
            label="Max Memory MB *"
            :rules="maxMemoryRules"
            :error="showMaxMemoryStateError"
            :error-message="maxMemoryStateMessage"
            reactive-rules
            lazy-rules />
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

        <div v-if="backupSettingsLoading" class="text-caption text-muted" data-testid="backup-settings-loading">
          Loading backup settings...
        </div>

        <template v-else>
          <div v-if="backupOverview.canManageSettings" class="row q-col-gutter-md q-gutter-y-md full-width">
            <div class="col-12">
              <q-toggle
                :model-value="backupSettings.backupsEnabled"
                label="Enable Backups"
                color="primary"
                data-testid="backup-settings-enabled"
                @update:model-value="backupSettings.backupsEnabled = $event" />
            </div>

            <q-input
              :model-value="backupSettings.backupDirectory"
              class="col-12 col-lg-8"
              data-testid="backup-settings-directory"
              outlined
              dense
              label="Backup Directory"
              @update:model-value="backupSettings.backupDirectory = $event" />

            <q-input
              :model-value="String(backupSettings.maxBackups)"
              class="col-12 col-sm-6 col-lg-4"
              data-testid="backup-settings-max-backups"
              outlined
              dense
              type="number"
              min="0"
              label="Max Automated Backups"
              @update:model-value="updateBackupMaxBackups($event)" />

            <div
              v-if="backupSettings.defaultBackupDirectory"
              class="col-12 text-caption text-muted"
              data-testid="backup-settings-default-directory">
              Default backup directory: {{ backupSettings.defaultBackupDirectory }}
            </div>

            <div class="col-12 row justify-end">
              <q-btn
                color="primary"
                no-caps
                label="Save Backup Settings"
                data-testid="save-backup-settings"
                :loading="backupSettingsSaving"
                @click="saveBackupSettings" />
            </div>
          </div>

          <div v-else class="row q-col-gutter-md q-gutter-y-sm full-width" data-testid="backup-settings-readonly">
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

<script setup lang="ts">
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import GameServerFormShell from './GameServerFormShell.vue'
import GameServerProvisioningContext from './GameServerProvisioningContext.vue'
import { useGameServerFormState } from './game-server-form-state'
import {
  BackupSettingsSchema,
  EditGameServerRequest,
  EditGameServerRequestSchema,
  GameServerBackupOverviewSchema,
} from '@/proto/shared_pb'
import type { BackupSettings, GameServerBackupOverview } from '@/proto/shared_pb'
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
