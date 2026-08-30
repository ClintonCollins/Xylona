<template>
  <game-server-form-shell
    :form-submitting="formSubmitting"
    :loading="loading"
    :save-disabled="loading"
    :subtitle="settingsSubtitle"
    class="settings-form-shell"
    header-title="Server Settings"
    loading-text="Loading server settings..."
    save-label="Save server"
    test-id="settings-form-shell"
    @cancel="cancel"
    @save="submitGameServer">
    <q-form ref="formRef" class="server-form-layout settings-workspace" greedy>
      <nav aria-label="Settings categories" class="settings-category-rail">
        <div class="settings-category-group">Server</div>
        <button
          :aria-current="activeCategory === 'general' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'general' }"
          class="settings-category-button"
          data-testid="settings-category-general"
          type="button"
          @click="selectSettingsCategory('general')">
          <q-icon name="badge" size="18px" />
          <span>
            General
            <small>Identity & placement</small>
          </span>
        </button>
        <button
          v-if="canEditProvisioning"
          :aria-current="activeCategory === 'network' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'network' }"
          class="settings-category-button"
          data-testid="settings-category-network"
          type="button"
          @click="selectSettingsCategory('network')">
          <q-icon name="lan" size="18px" />
          <span>
            Network & Launch
            <small>Ports & executable</small>
          </span>
        </button>
        <button
          v-if="canEditProvisioning"
          :aria-current="activeCategory === 'dns' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'dns' }"
          class="settings-category-button"
          data-testid="settings-category-dns"
          type="button"
          @click="selectSettingsCategory('dns')">
          <q-icon name="dns" size="18px" />
          <span>
            DNS
            <small>Manual record sync</small>
          </span>
        </button>
        <button
          :aria-current="activeCategory === 'capacity' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'capacity' }"
          class="settings-category-button"
          data-testid="settings-category-capacity"
          type="button"
          @click="selectSettingsCategory('capacity')">
          <q-icon name="memory" size="18px" />
          <span>
            Capacity
            <small>Players & recovery</small>
          </span>
        </button>

        <div class="settings-category-group">Operations</div>
        <button
          v-if="adminInterface.supported"
          :aria-current="activeCategory === 'admin' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'admin' }"
          class="settings-category-button"
          data-testid="settings-category-admin"
          type="button"
          @click="selectSettingsCategory('admin')">
          <q-icon name="admin_panel_settings" size="18px" />
          <span>
            Remote Admin
            <small>Credentials & access</small>
          </span>
        </button>
        <button
          :aria-current="activeCategory === 'environment' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'environment' }"
          class="settings-category-button"
          data-testid="settings-category-environment"
          type="button"
          @click="selectSettingsCategory('environment')">
          <q-icon name="key" size="18px" />
          <span>
            Environment
            <small>Variables & secrets</small>
          </span>
        </button>
        <button
          :aria-current="activeCategory === 'backups' ? 'page' : undefined"
          :class="{ 'is-active': activeCategory === 'backups' }"
          class="settings-category-button"
          data-testid="settings-category-backups"
          type="button"
          @click="selectSettingsCategory('backups')">
          <q-icon name="backup" size="18px" />
          <span>
            Backups
            <small>Storage & retention</small>
          </span>
        </button>
      </nav>

      <div class="settings-panel">
        <header class="settings-panel-heading">
          <div>
            <h2 class="settings-panel-title font-display">
              {{ settingsCategoryDetails[activeCategory].title }}
            </h2>
            <p>{{ settingsCategoryDetails[activeCategory].description }}</p>
          </div>
          <span
            :class="{
              'settings-scope--separate': settingsCategoryDetails[activeCategory].separate,
            }"
            class="settings-scope">
            {{ settingsCategoryDetails[activeCategory].scope }}
          </span>
        </header>

        <section
          v-show="activeCategory === 'general'"
          class="form-section"
          data-settings-category="general">
          <div class="section-header">
            <span class="section-icon section-icon--accent">
              <q-icon name="badge" size="14px" />
            </span>
            <span class="section-title">Identity</span>
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

        <section
          v-if="canEditProvisioning"
          v-show="activeCategory === 'general'"
          class="form-section form-section--last"
          data-settings-category="general">
          <div class="section-header">
            <span class="section-icon section-icon--primary">
              <q-icon name="hub" size="14px" />
            </span>
            <span class="section-title">Placement</span>
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

        <section
          v-if="canEditProvisioning"
          v-show="activeCategory === 'network'"
          class="form-section"
          data-settings-category="network">
          <div class="section-header">
            <span class="section-icon section-icon--success">
              <q-icon name="lan" size="14px" />
            </span>
            <span class="section-title">Networking</span>
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

        <section
          v-if="canEditProvisioning"
          v-show="activeCategory === 'dns'"
          class="form-section form-section--last"
          data-settings-category="dns"
          data-testid="dns-binding-settings-section">
          <game-server-dns-binding-settings :game-server-id="gameServerId" />
        </section>

        <game-server-provisioning-context
          v-if="!canEditProvisioning"
          v-show="activeCategory === 'general'"
          class="settings-panel-content"
          :capacity="provisioningCapacity"
          :connection="provisioningConnection"
          :executable="serverExecutableSummary"
          :game="selectedGameName"
          :memory="`${maxMemoryModel || 0} MB`"
          :node="selectedNodeName"
          :owner="selectedOwnerName"
          :show-memory="isMinecraftGame" />

        <section
          v-if="canEditProvisioning"
          v-show="activeCategory === 'network'"
          class="form-section form-section--last"
          data-settings-category="network">
          <div class="section-header">
            <span class="section-icon section-icon--muted">
              <q-icon name="terminal" size="14px" />
            </span>
            <span class="section-title">Launch</span>
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

        <section
          v-if="adminInterfaceLoading || adminInterface.supported"
          v-show="activeCategory === 'admin'"
          class="form-section form-section--last"
          data-settings-category="admin"
          data-testid="admin-interface-settings-section">
          <div class="section-header">
            <span class="section-icon section-icon--warning">
              <q-icon name="admin_panel_settings" size="14px" />
            </span>
            <span class="section-title">Remote Administration</span>
            <span class="section-line"></span>
          </div>

          <div
            v-if="adminInterfaceLoading"
            class="text-caption text-muted"
            data-testid="admin-interface-settings-loading">
            Loading admin interface...
          </div>

          <template v-else>
            <div class="admin-interface-summary">
              <div>
                <div class="text-caption text-muted">Interface</div>
                <div class="text-body2 text-primary">{{ adminInterface.transport }}</div>
              </div>
              <div>
                <div class="text-caption text-muted">Endpoint</div>
                <div
                  class="text-body2 text-primary font-mono"
                  data-testid="admin-interface-endpoint">
                  {{ adminInterfaceEndpoint }}
                </div>
              </div>
              <div v-if="adminInterface.username">
                <div class="text-caption text-muted">Username</div>
                <div class="text-body2 text-primary font-mono">{{ adminInterface.username }}</div>
              </div>
              <div>
                <div class="text-caption text-muted">Password</div>
                <div class="text-body2 text-primary" data-testid="admin-interface-password-status">
                  {{
                    adminInterface.passwordConfigured ? 'Configured' : 'Generated on first start'
                  }}
                </div>
              </div>
            </div>

            <q-banner class="q-mt-md" data-testid="admin-interface-access-note" dense rounded>
              {{ adminInterface.remoteAccessNote }}
              Changes take effect the next time the game server starts.
            </q-banner>
            <q-banner
              v-if="adminInterface.transportSecurityNote"
              class="q-mt-sm"
              data-testid="admin-interface-security-note"
              dense
              rounded>
              {{ adminInterface.transportSecurityNote }}
            </q-banner>

            <div class="admin-interface-password-editor q-mt-md">
              <q-input
                v-model="adminInterfacePassword"
                autocomplete="new-password"
                data-testid="admin-interface-password"
                hint="8–128 printable characters; spaces, double quotes, and backslashes are not supported."
                label="New Admin Interface Password"
                outlined
                type="password" />
              <q-btn
                :disable="adminInterfacePassword.length === 0"
                :loading="adminInterfaceSaving"
                color="primary"
                data-testid="save-admin-interface-password"
                label="Update Password"
                no-caps
                @click="saveAdminInterfacePassword" />
            </div>
          </template>
        </section>

        <section
          v-show="activeCategory === 'environment'"
          class="form-section form-section--last"
          data-settings-category="environment"
          data-testid="environment-settings-section">
          <div class="section-header">
            <span class="section-icon section-icon--accent">
              <q-icon name="key" size="14px" />
            </span>
            <span class="section-title">Environment</span>
            <span class="section-line"></span>
          </div>

          <div
            v-if="environmentLoading"
            class="text-caption text-muted"
            data-testid="environment-settings-loading">
            Loading environment...
          </div>

          <template v-else>
            <q-banner
              v-if="environmentIssues.length > 0"
              class="q-mb-md"
              data-testid="environment-validation-issues"
              dense
              rounded>
              <div v-for="issue in environmentIssues" :key="issue.name + issue.message">
                {{ issue.message }}
              </div>
            </q-banner>
            <q-banner
              v-if="environmentDirty"
              class="q-mb-md"
              data-testid="environment-unsaved-warning"
              dense
              rounded>
              Unsaved environment changes. Save Environment applies them separately from server
              settings.
            </q-banner>

            <div class="environment-grid">
              <div class="environment-panel">
                <div class="environment-panel-header">
                  <div class="environment-panel-title">Variables</div>
                  <q-btn
                    color="primary"
                    data-testid="add-environment-row"
                    dense
                    flat
                    icon="add"
                    round
                    @click="addEnvironmentRow" />
                </div>

                <div
                  v-if="environmentRows.length === 0"
                  class="environment-empty text-caption text-muted"
                  data-testid="environment-empty">
                  No variables configured.
                </div>

                <div
                  v-for="(row, index) in environmentRows"
                  :key="index"
                  class="environment-row"
                  data-testid="environment-row">
                  <q-input
                    v-model="row.name"
                    class="environment-name-input"
                    data-testid="environment-name"
                    dense
                    label="Name"
                    outlined />
                  <q-input
                    v-model="row.value"
                    class="environment-value-input"
                    data-testid="environment-value"
                    dense
                    label="Value"
                    outlined />
                  <q-btn
                    color="negative"
                    data-testid="remove-environment-row"
                    dense
                    flat
                    icon="delete"
                    round
                    @click="removeEnvironmentRow(index)" />
                </div>

                <div class="environment-actions">
                  <q-btn
                    :loading="environmentSaving"
                    color="primary"
                    data-testid="save-environment-settings"
                    label="Save Environment"
                    no-caps
                    @click="saveEnvironmentSettings" />
                </div>
              </div>

              <div class="environment-panel">
                <div class="environment-panel-header">
                  <div class="environment-panel-title">Secrets</div>
                </div>

                <div
                  v-if="secretEnvironmentStates.length === 0"
                  class="environment-empty text-caption text-muted"
                  data-testid="secret-environment-empty">
                  No secrets configured.
                </div>

                <div
                  v-for="secret in secretEnvironmentStates"
                  :key="secret.name"
                  class="secret-environment-row"
                  data-testid="secret-environment-row">
                  <div class="secret-environment-summary">
                    <div class="secret-environment-name">{{ secret.name }}</div>
                    <div class="secret-environment-updated text-caption text-muted">
                      {{ formatSecretUpdatedAt(secret) }}
                    </div>
                  </div>
                  <q-btn
                    color="negative"
                    data-testid="clear-secret-environment"
                    dense
                    flat
                    icon="delete"
                    round
                    @click="clearSecretEnvironment(secret.name)" />
                </div>

                <div class="secret-environment-editor">
                  <q-input
                    v-model="secretEnvironmentName"
                    data-testid="secret-environment-name"
                    dense
                    label="Name"
                    outlined />
                  <q-input
                    v-model="secretEnvironmentValue"
                    data-testid="secret-environment-value"
                    dense
                    label="Value"
                    outlined
                    type="password" />
                  <q-btn
                    :loading="secretEnvironmentSaving"
                    color="primary"
                    data-testid="set-secret-environment"
                    label="Set Secret"
                    no-caps
                    @click="setSecretEnvironment" />
                </div>
              </div>
            </div>
          </template>
        </section>

        <section
          v-show="activeCategory === 'capacity'"
          class="form-section"
          data-settings-category="capacity">
          <div class="section-header">
            <span class="section-icon section-icon--warning">
              <q-icon name="memory" size="14px" />
            </span>
            <span class="section-title">Capacity</span>
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

        <section
          v-show="activeCategory === 'capacity'"
          class="form-section form-section--last"
          data-settings-category="capacity"
          data-testid="auto-restart-section">
          <div class="section-header">
            <span class="section-icon section-icon--success">
              <q-icon name="restart_alt" size="14px" />
            </span>
            <span class="section-title">Auto-Restart</span>
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

        <section
          v-show="activeCategory === 'backups'"
          class="form-section form-section--last"
          data-settings-category="backups"
          data-testid="backup-settings-section">
          <div class="section-header">
            <span class="section-icon section-icon--primary">
              <q-icon name="backup" size="14px" />
            </span>
            <span class="section-title">Backup Settings</span>
            <span class="section-line"></span>
          </div>

          <div
            v-if="backupSettingsLoading"
            class="text-caption text-muted"
            data-testid="backup-settings-loading">
            Loading backup settings...
          </div>

          <template v-else>
            <q-banner
              v-if="!backupSettings.backupsSupported"
              class="col-12 bg-warning text-dark q-mb-md"
              data-testid="backup-settings-unsupported"
              dense
              rounded>
              {{ backupSettings.disabledReason || 'New backups are unavailable for this server.' }}
              Existing backups remain available from the Backups page.
            </q-banner>

            <div
              v-if="backupOverview.canManageSettings"
              class="row q-col-gutter-md q-gutter-y-md full-width">
              <div class="col-12">
                <q-toggle
                  :model-value="backupSettings.backupsEnabled"
                  color="primary"
                  data-testid="backup-settings-enabled"
                  label="Enable Backups"
                  :disable="backupEnableBlocked && !backupSettings.backupsEnabled"
                  @update:model-value="updateBackupsEnabled" />
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
                  :disable="backupSettingsSaveBlocked"
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
      </div>
    </q-form>
  </game-server-form-shell>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, nextTick, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'
import GameServerFormShell from './GameServerFormShell.vue'
import GameServerDnsBindingSettings from './GameServerDNSBindingSettings.vue'
import GameServerProvisioningContext from './GameServerProvisioningContext.vue'
import { formatProtoTimestamp } from './game-server-access-utils'
import { useGameServerFormState } from './useGameServerFormState'
import type {
  BackupSettings,
  EnvironmentValidationIssue,
  EnvironmentVariable,
  GameServerBackupOverview,
  SecretEnvironmentVariableState,
} from '@/proto/shared_pb'
import type { GameServerAdminInterface } from '@/proto/xylona_pb'
import {
  BackupSettingsSchema,
  EditGameServerRequest,
  EditGameServerRequestSchema,
  EnvironmentVariableSchema,
  GameServerBackupOverviewSchema,
} from '@/proto/shared_pb'
import {
  ClearGameServerSecretEnvRequestSchema,
  GameServerAdminInterfaceSchema,
  GetGameServerAdminInterfaceRequestSchema,
  GetBackupSettingsRequestSchema,
  GetGameServerEnvironmentRequestSchema,
  GetGameServerBackupOverviewRequestSchema,
  SetGameServerSecretEnvRequestSchema,
  SetGameServerAdminInterfacePasswordRequestSchema,
  UpdateGameServerEnvironmentRequestSchema,
  UpdateBackupSettingsRequestSchema,
} from '@/proto/xylona_pb'

const props = defineProps<{
  canEditProvisioning: boolean
  gameServerId: string
}>()

type SettingsCategory =
  'general' | 'network' | 'dns' | 'capacity' | 'admin' | 'environment' | 'backups'

const settingsCategoryDetails: Record<
  SettingsCategory,
  { description: string; scope: string; separate: boolean; title: string }
> = {
  general: {
    title: 'General',
    description: "The server's identity and where it runs.",
    scope: 'Saved with server',
    separate: false,
  },
  network: {
    title: 'Network & Launch',
    description: 'Connection endpoints and process override.',
    scope: 'Saved with server',
    separate: false,
  },
  dns: {
    title: 'DNS Binding',
    description: 'Manually synchronize one DNS name to this game server.',
    scope: 'Separate actions',
    separate: true,
  },
  capacity: {
    title: 'Capacity & Recovery',
    description: 'Player limits and automatic restart behavior.',
    scope: 'Saved with server',
    separate: false,
  },
  admin: {
    title: 'Remote Administration',
    description: 'Remote access credentials for this game server.',
    scope: 'Separate action',
    separate: true,
  },
  environment: {
    title: 'Environment',
    description: 'Runtime variables and encrypted secrets.',
    scope: 'Separate save',
    separate: true,
  },
  backups: {
    title: 'Backup Policy',
    description: 'Storage location and automated retention.',
    scope: 'Separate save',
    separate: true,
  },
}

const router = useRouter()
const $q = useQuasar()
const activeCategory = ref<SettingsCategory>('general')
const backupSettings = ref<BackupSettings>(create(BackupSettingsSchema))
const backupOverview = ref<GameServerBackupOverview>(create(GameServerBackupOverviewSchema))
const backupSettingsLoading = ref(true)
const backupSettingsSaving = ref(false)
const environmentRows = ref<EnvironmentVariable[]>([])
const environmentSnapshot = ref('')
const environmentIssues = ref<EnvironmentValidationIssue[]>([])
const environmentLoading = ref(true)
const environmentSaving = ref(false)
const adminInterface = ref<GameServerAdminInterface>(create(GameServerAdminInterfaceSchema))
const adminInterfaceLoading = ref(true)
const adminInterfacePassword = ref('')
const adminInterfaceSaving = ref(false)
const secretEnvironmentStates = ref<SecretEnvironmentVariableState[]>([])
const secretEnvironmentName = ref('')
const secretEnvironmentValue = ref('')
const secretEnvironmentSaving = ref(false)
const environmentDirty = computed(() => {
  if (!environmentSnapshot.value) {
    return false
  }

  return serializeEnvironmentRows(environmentRows.value) !== environmentSnapshot.value
})
const backupEnableBlocked = computed(() => !backupSettings.value.backupsSupported)
const backupSettingsSaveBlocked = computed(
  () => backupEnableBlocked.value && backupSettings.value.backupsEnabled,
)
const adminInterfaceEndpoint = computed(() => {
  const address = adminInterface.value.bindAddress
  const displayAddress =
    address.includes(':') && !address.startsWith('[') ? `[${address}]` : address
  return `${displayAddress}:${adminInterface.value.port.toString()}`
})

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

const settingsSubtitle = computed(
  () => `${gameServer.value.name || 'Game server'} · ${selectedGameName.value}`,
)

onMounted(async () => {
  await initialize()
  await Promise.all([
    initializeAdminInterface(),
    initializeBackupSettings(),
    initializeEnvironmentSettings(),
  ])
})

function selectSettingsCategory(category: SettingsCategory) {
  activeCategory.value = category
}

async function revealFirstInvalidCategory() {
  await nextTick()
  const formElement = formRef.value?.$el as HTMLElement | undefined
  const category = formElement
    ?.querySelector('.q-field--error')
    ?.closest<HTMLElement>('[data-settings-category]')?.dataset.settingsCategory as
    SettingsCategory | undefined

  if (category && category !== activeCategory.value) {
    selectSettingsCategory(category)
  }
}

async function cancel() {
  router.back()
}

async function initializeAdminInterface() {
  adminInterfaceLoading.value = true

  try {
    const response = await GetXylonaClient().getGameServerAdminInterface(
      create(GetGameServerAdminInterfaceRequestSchema, {
        serverId: props.gameServerId,
      }),
    )
    adminInterface.value = response.adminInterface
      ? create(GameServerAdminInterfaceSchema, response.adminInterface)
      : create(GameServerAdminInterfaceSchema)
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load admin interface: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    adminInterfaceLoading.value = false
  }
}

async function saveAdminInterfacePassword() {
  adminInterfaceSaving.value = true

  try {
    const response = await GetXylonaClient().setGameServerAdminInterfacePassword(
      create(SetGameServerAdminInterfacePasswordRequestSchema, {
        serverId: props.gameServerId,
        password: adminInterfacePassword.value,
      }),
    )
    if (response.adminInterface) {
      adminInterface.value = create(GameServerAdminInterfaceSchema, response.adminInterface)
    }
    adminInterfacePassword.value = ''
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Admin interface password updated. Restart the game server to apply it.',
      icon: 'task_alt',
    })
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption:
        'Failed to save admin interface password: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    adminInterfaceSaving.value = false
  }
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

async function initializeEnvironmentSettings() {
  environmentLoading.value = true

  try {
    const response = await GetXylonaClient().getGameServerEnvironment(
      create(GetGameServerEnvironmentRequestSchema, {
        serverId: props.gameServerId,
      }),
    )

    environmentRows.value = cloneEnvironmentVariables(response.serverEnv)
    commitEnvironmentSnapshot()
    environmentIssues.value = response.validationIssues
    secretEnvironmentStates.value = response.secretEnv
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to load environment: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    environmentLoading.value = false
  }
}

function cloneEnvironmentVariables(variables: EnvironmentVariable[]): EnvironmentVariable[] {
  return variables.map((variable) =>
    create(EnvironmentVariableSchema, {
      name: variable.name,
      value: variable.value,
    }),
  )
}

function serializeEnvironmentRows(rows: EnvironmentVariable[]): string {
  return JSON.stringify(
    rows.map((row) => ({
      name: row.name,
      value: row.value,
    })),
  )
}

function commitEnvironmentSnapshot(): void {
  environmentSnapshot.value = serializeEnvironmentRows(environmentRows.value)
}

function addEnvironmentRow(): void {
  environmentRows.value.push(create(EnvironmentVariableSchema))
}

function removeEnvironmentRow(index: number): void {
  environmentRows.value.splice(index, 1)
}

async function saveEnvironmentSettings() {
  environmentSaving.value = true
  try {
    const envVars = environmentRows.value.map((row) =>
      create(EnvironmentVariableSchema, {
        name: row.name.trim(),
        value: row.value,
      }),
    )

    const response = await GetXylonaClient().updateGameServerEnvironment(
      create(UpdateGameServerEnvironmentRequestSchema, {
        serverId: props.gameServerId,
        envVars,
      }),
    )

    environmentRows.value = cloneEnvironmentVariables(response.serverEnv)
    commitEnvironmentSnapshot()
    environmentIssues.value = response.validationIssues
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Environment variables saved successfully.',
      icon: 'task_alt',
    })
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to save environment: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    environmentSaving.value = false
  }
}

async function setSecretEnvironment() {
  secretEnvironmentSaving.value = true
  try {
    const response = await GetXylonaClient().setGameServerSecretEnv(
      create(SetGameServerSecretEnvRequestSchema, {
        serverId: props.gameServerId,
        name: secretEnvironmentName.value.trim(),
        value: secretEnvironmentValue.value,
      }),
    )

    secretEnvironmentStates.value = response.secretEnv
    environmentIssues.value = response.validationIssues
    secretEnvironmentValue.value = ''
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Secret saved successfully.',
      icon: 'task_alt',
    })
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to save secret: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    secretEnvironmentSaving.value = false
  }
}

async function clearSecretEnvironment(name: string) {
  secretEnvironmentSaving.value = true
  try {
    const response = await GetXylonaClient().clearGameServerSecretEnv(
      create(ClearGameServerSecretEnvRequestSchema, {
        serverId: props.gameServerId,
        name,
      }),
    )

    secretEnvironmentStates.value = response.secretEnv
    environmentIssues.value = response.validationIssues
    $q.notify({
      type: 'positive',
      position: 'top',
      caption: 'Secret cleared successfully.',
      icon: 'task_alt',
    })
  } catch (e) {
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to clear secret: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    secretEnvironmentSaving.value = false
  }
}

function formatSecretUpdatedAt(secret: SecretEnvironmentVariableState): string {
  if (!secret.configured) {
    return 'Not configured'
  }
  return formatProtoTimestamp(secret.updatedAt)
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

function updateBackupsEnabled(enabled: boolean): void {
  if (enabled && backupEnableBlocked.value) {
    return
  }

  backupSettings.value.backupsEnabled = enabled
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
    await revealFirstInvalidCategory()
    return
  }

  startSubmitting()

  try {
    const request: EditGameServerRequest = create(EditGameServerRequestSchema, {})
    request.serverId = props.gameServerId
    request.gameServer = gameServer.value

    await GetXylonaClient().editGameServer(request)
    await initializeAdminInterface()
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

<style scoped>
.settings-form-shell {
  --xy-header-stack-height: 0px;
}

.settings-workspace {
  display: grid;
  grid-template-columns: minmax(180px, 210px) minmax(0, 1fr);
  overflow: hidden;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-xl);
}

.settings-category-rail {
  grid-column: 1;
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm);
  background: var(--xy-surface-0);
  border-right: 1px solid var(--xy-border);
}

.settings-category-group {
  padding: var(--xy-space-sm) var(--xy-space-sm) var(--xy-space-xs);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.06em;
  line-height: var(--xy-line-height-tight);
  text-transform: uppercase;
}

.settings-category-group:not(:first-child) {
  margin-top: var(--xy-space-sm);
}

.settings-category-button {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  width: 100%;
  padding: 10px var(--xy-space-sm);
  color: var(--xy-text-secondary);
  text-align: left;
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--xy-radius-md);
  font: inherit;
  cursor: pointer;
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
}

.settings-category-button:hover {
  color: var(--xy-text-primary);
  background: var(--xy-surface-overlay-soft);
  border-color: var(--xy-border);
}

.settings-category-button:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 2px;
}

.settings-category-button.is-active {
  color: var(--xy-text-primary);
  background: var(--xy-primary-muted);
  border-color: var(--xy-primary-border-soft);
}

.settings-category-button :deep(.q-icon) {
  flex: 0 0 auto;
  color: var(--xy-text-muted);
}

.settings-category-button.is-active :deep(.q-icon) {
  color: var(--xy-accent);
}

.settings-category-button > span {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-2xs);
  min-width: 0;
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
  line-height: var(--xy-line-height-tight);
}

.settings-category-button small {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-2xs);
  font-weight: 400;
}

.settings-panel-heading {
  min-width: 0;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--xy-space-md);
  padding-bottom: var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.settings-panel-title {
  margin: 0;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
  letter-spacing: 0.015em;
  line-height: var(--xy-line-height-tight);
}

.settings-panel-heading p {
  max-width: 65ch;
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
  line-height: var(--xy-line-height-base);
}

.settings-scope {
  flex: 0 0 auto;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-2xs);
  line-height: var(--xy-line-height-tight);
}

.settings-scope--separate {
  color: var(--xy-accent-hover);
  background: var(--xy-accent-muted);
  border-color: var(--xy-accent-border-soft);
}

.settings-panel {
  grid-column: 2;
  min-width: 0;
  padding: var(--xy-space-lg);
}

.settings-panel > .form-section,
.settings-panel-content {
  min-width: 0;
}

.settings-panel > .form-section:first-of-type {
  padding-top: var(--xy-space-md);
}

.settings-workspace .section-title {
  font-family: var(--xy-font-body);
}

.settings-panel :deep(.q-field--outlined .q-field__control) {
  background: var(--xy-surface-0);
}

.settings-panel :deep(.q-field--outlined .q-field__control::before) {
  border-color: var(--xy-border-hover);
}

.admin-interface-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--xy-space-md);
}

.admin-interface-password-editor {
  display: grid;
  grid-template-columns: minmax(220px, 1fr) auto;
  gap: var(--xy-space-sm);
  align-items: start;
}

.environment-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 340px), 1fr));
  gap: var(--xy-space-md);
}

.environment-panel {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.environment-panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.environment-panel-title {
  color: var(--xy-text-emphasis-soft);
  font-size: 0.82rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.environment-empty {
  padding: var(--xy-space-sm) 0;
}

.environment-row {
  display: grid;
  grid-template-columns: minmax(120px, 0.7fr) minmax(160px, 1fr) auto;
  gap: var(--xy-space-sm);
  align-items: start;
}

.environment-actions {
  display: flex;
  justify-content: flex-end;
}

.secret-environment-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  min-height: 40px;
  border-bottom: 1px solid var(--xy-border);
}

.secret-environment-summary {
  min-width: 0;
}

.secret-environment-name {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: 0.88rem;
  overflow-wrap: anywhere;
}

.secret-environment-updated {
  line-height: 1.3;
}

.secret-environment-editor {
  display: grid;
  grid-template-columns: minmax(120px, 0.8fr) minmax(160px, 1fr) auto;
  gap: var(--xy-space-sm);
  align-items: start;
}

@media (max-width: 720px) {
  .admin-interface-password-editor,
  .environment-row,
  .secret-environment-editor {
    grid-template-columns: 1fr;
  }

  .admin-interface-password-editor :deep(.q-btn),
  .environment-row :deep(.q-btn),
  .secret-environment-editor :deep(.q-btn) {
    justify-self: flex-start;
  }
}

@media (max-width: 1023px) {
  .settings-workspace {
    grid-template-columns: 1fr;
  }

  .settings-category-rail {
    grid-column: 1;
    position: static;
    flex-direction: row;
    overflow-x: auto;
    padding: var(--xy-space-xs);
    border-right: 0;
    border-bottom: 1px solid var(--xy-border);
    scrollbar-width: none;
  }

  .settings-category-rail::-webkit-scrollbar {
    display: none;
  }

  .settings-category-group {
    display: none;
  }

  .settings-category-button {
    flex: 0 0 auto;
    width: auto;
  }

  .settings-category-button small {
    display: none;
  }

  .settings-panel {
    grid-column: 1;
  }
}

@media (max-width: 599px) {
  :deep(.server-form-header) {
    flex-direction: column;
    align-items: stretch;
  }

  :deep(.server-form-header-actions) {
    width: 100%;
    justify-content: flex-end;
  }

  .settings-panel-heading {
    flex-direction: column;
    gap: var(--xy-space-sm);
  }

  .settings-panel {
    padding: var(--xy-space-md);
  }
}
</style>
