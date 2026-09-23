<template>
  <game-server-form-shell
    :form-submitting="formSubmitting"
    :loading="loading"
    :save-disabled="loading || !hasSaveableChanges"
    :subtitle="settingsSubtitle"
    class="settings-form-shell"
    header-title="Server Settings"
    loading-text="Loading server settings..."
    save-label="Save changes"
    submitting-label="Saving changes..."
    test-id="settings-form-shell"
    @cancel="cancel"
    @save="saveAllChanges">
    <q-form ref="formRef" class="server-form-layout settings-workspace" greedy>
      <nav aria-label="Settings categories" class="settings-category-rail">
        <template v-for="{ group, categories } in categoryGroups" :key="group">
          <div class="settings-category-group">{{ group }}</div>
          <button
            v-for="category in categories"
            :key="category"
            :aria-current="activeCategory === category ? 'page' : undefined"
            :class="{ 'is-active': activeCategory === category }"
            :data-testid="`settings-category-${category}`"
            class="settings-category-button"
            type="button"
            @click="selectSettingsCategory(category)">
            <q-icon :name="settingsCategories[category].icon" size="18px" />
            <span>
              {{ settingsCategories[category].label }}
              <small>{{ settingsCategories[category].caption }}</small>
            </span>
            <span
              v-if="dirtyCategories.has(category)"
              :data-testid="`settings-category-${category}-dirty`"
              class="settings-category-dirty">
              <span class="xy-visually-hidden">Unsaved changes</span>
            </span>
          </button>
        </template>
      </nav>

      <div class="settings-panel">
        <header class="settings-panel-heading">
          <div>
            <h2 class="xy-section-title">{{ settingsCategories[activeCategory].title }}</h2>
            <p>{{ settingsCategories[activeCategory].description }}</p>
          </div>
          <span
            :class="{ 'settings-scope--dirty': dirtyCategories.has(activeCategory) }"
            class="settings-scope"
            data-testid="settings-save-state">
            {{ dirtyCategories.has(activeCategory) ? 'Unsaved changes' : 'All changes saved' }}
          </span>
        </header>

        <section
          v-show="activeCategory === 'general'"
          class="form-section"
          data-settings-category="general">
          <div class="section-header">
            <span class="section-icon">
              <q-icon name="badge" size="14px" />
            </span>
            <span class="section-title">Identity</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md">
            <q-input
              v-model="gameServer.name"
              :rules="serverNameRules"
              aria-required="true"
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
              aria-required="true"
              class="col-12 col-md-6"
              data-testid="editable-game"
              emit-value
              hint="Keeps installed files, ports and capacity. Stop the server before changing it."
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
          :class="{ 'form-section--last': !showJoinPassword }"
          class="form-section"
          data-settings-category="general">
          <div class="section-header">
            <span class="section-icon">
              <q-icon name="hub" size="14px" />
            </span>
            <span class="section-title">Placement</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md">
            <q-select
              v-model="gameServer.userId"
              :options="availableUsers"
              :rules="ownerRules"
              aria-required="true"
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
              aria-required="true"
              class="col-12 col-md-4"
              data-testid="editable-node"
              emit-value
              hint="Installed files don't move to the new node. Stop the server first."
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
              aria-required="true"
              class="col-12 col-md-4"
              data-testid="editable-ip"
              label="IP Address *"
              lazy-rules
              option-label="address"
              outlined
              reactive-rules />
          </div>
        </section>

        <game-server-provisioning-context
          v-if="!canEditProvisioning"
          v-show="activeCategory === 'general'"
          :capacity="provisioningCapacity"
          :class="{ 'form-section--last': !showJoinPassword }"
          :connection="provisioningConnection"
          :executable="serverExecutableSummary"
          :game="selectedGameName"
          :memory="`${maxMemoryModel || 0} MB`"
          :node="selectedNodeName"
          :owner="selectedOwnerName"
          :show-memory="isMinecraftGame"
          class="settings-panel-content" />

        <join-password-settings
          v-if="showJoinPassword"
          v-show="activeCategory === 'general'"
          ref="joinPasswordSettings"
          :can-edit="gameServer.effectivePermissions.includes('game_server.settings')"
          :server-id="gameServerId"
          class="form-section--last"
          data-settings-category="general" />

        <section
          v-if="canEditProvisioning"
          v-show="activeCategory === 'network'"
          class="form-section"
          data-settings-category="network">
          <div class="section-header">
            <span class="section-icon">
              <q-icon name="lan" size="14px" />
            </span>
            <span class="section-title">Networking</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md">
            <q-input
              v-model.number="portModel"
              :hint="adminPortHint('port')"
              :rules="portRules"
              aria-required="true"
              class="col-12 col-sm-6"
              data-testid="editable-port"
              label="Port *"
              lazy-rules
              outlined
              reactive-rules
              type="number" />
            <q-input
              v-model.number="queryPortModel"
              :hint="adminPortHint('query_port')"
              :rules="queryPortRules"
              aria-required="true"
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
          v-show="activeCategory === 'network'"
          class="form-section form-section--last"
          data-settings-category="network">
          <div class="section-header">
            <span class="section-icon">
              <q-icon name="terminal" size="14px" />
            </span>
            <span class="section-title">Launch</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md">
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
          v-if="canEditProvisioning"
          v-show="activeCategory === 'dns'"
          class="form-section form-section--last"
          data-settings-category="dns"
          data-testid="dns-binding-settings-section">
          <game-server-dns-binding-settings
            ref="dnsBindingSettings"
            :game-server-id="gameServerId" />
        </section>

        <section
          v-if="adminInterfaceLoading || adminInterface.supported"
          v-show="activeCategory === 'admin'"
          class="form-section form-section--last"
          data-settings-category="admin"
          data-testid="admin-interface-settings-section">
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
                <div class="text-body2 text-xy-primary">{{ adminInterface.transport }}</div>
              </div>
              <div>
                <div class="text-caption text-muted">Endpoint</div>
                <div
                  class="text-body2 text-xy-primary font-mono"
                  data-testid="admin-interface-endpoint">
                  {{ adminInterfaceEndpoint }}
                </div>
                <div
                  v-if="adminPortSource"
                  class="text-caption text-muted"
                  data-testid="admin-interface-port-source">
                  Port set by {{ adminPortSource }}
                  <template v-if="canEditProvisioning">
                    in
                    <button
                      class="settings-inline-link"
                      type="button"
                      @click="selectSettingsCategory('network')">
                      Network & Launch
                    </button>
                  </template>
                </div>
              </div>
              <div v-if="adminInterface.username">
                <div class="text-caption text-muted">Username</div>
                <div class="text-body2 text-xy-primary font-mono">
                  {{ adminInterface.username }}
                </div>
              </div>
              <div>
                <div class="text-caption text-muted">Password</div>
                <div
                  class="text-body2 text-xy-primary"
                  data-testid="admin-interface-password-status">
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
              class="q-mt-sm xy-banner-warning"
              data-testid="admin-interface-security-note"
              dense
              rounded>
              <template #avatar>
                <q-icon name="warning_amber" size="sm" />
              </template>
              {{ adminInterface.transportSecurityNote }}
            </q-banner>

            <q-input
              v-model="adminInterfacePassword"
              autocomplete="new-password"
              class="admin-interface-password q-mt-md"
              data-testid="admin-interface-password"
              hint="8–128 printable characters; spaces, double quotes, and backslashes are not supported. Save changes applies it."
              label="New Admin Interface Password"
              outlined
              type="password" />
          </template>
        </section>

        <section
          v-show="activeCategory === 'environment'"
          class="form-section form-section--last"
          data-settings-category="environment"
          data-testid="environment-settings-section">
          <div
            v-if="environmentLoading"
            class="text-caption text-muted"
            data-testid="environment-settings-loading">
            Loading environment...
          </div>

          <!-- Without a loaded baseline, edits can't be diffed or saved, so offer Retry instead of the grid. -->
          <q-banner
            v-else-if="environmentLoadError"
            class="xy-banner-negative"
            data-testid="environment-load-error"
            dense
            inline-actions
            role="alert">
            <template #avatar>
              <q-icon name="sync_problem" />
            </template>
            <strong>Environment could not be loaded.</strong> {{ environmentLoadError }}
            <template #action>
              <q-btn
                aria-label="Retry loading environment"
                data-testid="environment-load-retry"
                flat
                icon="refresh"
                label="Retry"
                no-caps
                @click="initializeEnvironmentSettings" />
            </template>
          </q-banner>

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

            <div class="environment-grid">
              <div class="environment-panel">
                <div class="environment-panel-header">
                  <div class="environment-panel-title">Variables</div>
                  <q-btn
                    aria-label="Add environment variable"
                    color="primary"
                    data-testid="add-environment-row"
                    dense
                    flat
                    icon="add"
                    round
                    @click="addEnvironmentRow">
                    <q-tooltip>Add variable</q-tooltip>
                  </q-btn>
                </div>
                <div class="environment-panel-note text-caption text-muted">
                  Saved with Save changes.
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
                    label="Name"
                    outlined />
                  <q-input
                    v-model="row.value"
                    class="environment-value-input"
                    data-testid="environment-value"
                    label="Value"
                    outlined />
                  <q-btn
                    :aria-label="`Remove environment variable ${index + 1}`"
                    color="negative"
                    data-testid="remove-environment-row"
                    dense
                    flat
                    icon="delete"
                    round
                    @click="removeEnvironmentRow(index)">
                    <q-tooltip>Remove variable</q-tooltip>
                  </q-btn>
                </div>
              </div>

              <div class="environment-panel">
                <div class="environment-panel-header">
                  <div class="environment-panel-title">Secrets</div>
                </div>
                <div class="environment-panel-note text-caption text-muted">
                  Encrypted and write-only. Set Secret and Clear apply immediately.
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
                    :aria-label="`Clear secret ${secret.name}`"
                    :disable="secretEnvironmentSaving"
                    color="negative"
                    data-testid="clear-secret-environment"
                    dense
                    flat
                    icon="delete"
                    round
                    @click="confirmClearSecretEnvironment(secret.name)">
                    <q-tooltip>Clear secret</q-tooltip>
                  </q-btn>
                </div>

                <div class="secret-environment-editor">
                  <q-input
                    v-model="secretEnvironmentName"
                    data-testid="secret-environment-name"
                    label="Name"
                    outlined />
                  <q-input
                    v-model="secretEnvironmentValue"
                    autocomplete="new-password"
                    data-testid="secret-environment-value"
                    label="Value"
                    outlined
                    type="password" />
                  <q-btn
                    :disable="!secretEnvironmentName.trim() || !secretEnvironmentValue"
                    :loading="secretEnvironmentSaving"
                    color="primary"
                    data-testid="set-secret-environment"
                    label="Set Secret"
                    no-caps
                    no-wrap
                    outline
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
            <span class="section-icon">
              <q-icon name="memory" size="14px" />
            </span>
            <span class="section-title">Capacity</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md">
            <q-input
              v-model.number="setPlayersModel"
              :hint="setPlayersHint"
              :rules="setPlayersRules"
              aria-required="true"
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
              :hint="maxPlayersHint"
              :rules="maxPlayersRules"
              aria-required="true"
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
              aria-required="true"
              class="col-12 col-lg-4"
              data-testid="editable-max-memory"
              hint="Passed to -Xms and -Xmx. The JVM always uses more memory than its heap."
              label="Java heap limit (MB) *"
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
            <span class="section-icon">
              <q-icon name="restart_alt" size="14px" />
            </span>
            <span class="section-title">Auto-Restart</span>
            <span class="section-line"></span>
          </div>
          <div class="row q-col-gutter-md">
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
              aria-required="true"
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
              aria-required="true"
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
          <div
            v-if="backupSettingsLoading"
            class="text-caption text-muted"
            data-testid="backup-settings-loading">
            Loading backup settings...
          </div>

          <q-banner
            v-else-if="backupSettingsLoadError"
            class="xy-banner-negative"
            data-testid="backup-settings-load-error"
            dense
            inline-actions
            role="alert">
            <template #avatar>
              <q-icon name="sync_problem" />
            </template>
            <strong>Backup settings could not be loaded.</strong> {{ backupSettingsLoadError }}
            <template #action>
              <q-btn
                aria-label="Retry loading backup settings"
                flat
                icon="refresh"
                label="Retry"
                no-caps
                @click="initializeBackupSettings" />
            </template>
          </q-banner>

          <template v-else>
            <q-banner
              v-if="!backupSettings.backupsSupported"
              class="col-12 xy-banner-warning q-mb-md"
              data-testid="backup-settings-unsupported"
              dense>
              {{ backupSettings.disabledReason || 'New backups are unavailable for this server.' }}
              Existing backups remain available from the Backups page.
            </q-banner>

            <div v-if="backupOverview.canManageSettings" class="row q-col-gutter-md">
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
                label="Backup Directory"
                outlined
                @update:model-value="backupSettings.backupDirectory = $event" />

              <q-input
                :error="backupSettings.maxBackups < 1n"
                :model-value="String(backupSettings.maxBackups)"
                class="col-12 col-sm-6 col-lg-4"
                data-testid="backup-settings-max-backups"
                error-message="Keep at least 1 automated backup."
                hint="Older automated backups beyond this count are deleted. Manual backups are kept."
                label="Max Automated Backups"
                min="1"
                outlined
                type="number"
                @update:model-value="updateBackupMaxBackups($event)" />

              <div
                v-if="backupSettings.defaultBackupDirectory"
                class="col-12 text-caption text-muted"
                data-testid="backup-settings-default-directory">
                Default backup directory: {{ backupSettings.defaultBackupDirectory }}
              </div>
            </div>

            <div v-else class="row q-col-gutter-md" data-testid="backup-settings-readonly">
              <div class="col-12 col-md-6">
                <div class="text-caption text-muted">Status</div>
                <div class="text-body2 text-xy-primary">
                  {{ formatBackupEnabled(backupSettings.backupsEnabled) }}
                </div>
              </div>
              <div class="col-12 col-md-6">
                <div class="text-caption text-muted">Max Automated Backups</div>
                <div class="text-body2 text-xy-primary">
                  {{ String(backupSettings.maxBackups) }}
                </div>
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
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { connectErrorMessage } from '@/api/connect-errors'
import { ConnectErrorToString, GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import { notifyError, notifySuccess } from '@/api/notifications'
import { useUnsavedChangesGuard } from '@/utils/unsaved-changes-guard'
import GameServerFormShell from './GameServerFormShell.vue'
import GameServerDnsBindingSettings from './GameServerDNSBindingSettings.vue'
import GameServerProvisioningContext from './GameServerProvisioningContext.vue'
import JoinPasswordSettings from './JoinPasswordSettings.vue'
import { formatProtoTimestamp } from './game-server-access-utils'
import { useGameServerFormState } from './useGameServerFormState'
import type {
  BackupSettings,
  EnvironmentValidationIssue,
  EnvironmentVariable,
  GameServer,
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
  Status,
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
type CoreCategory = 'general' | 'network' | 'capacity'

interface SettingsSection {
  category: SettingsCategory
  label: string
  dirty: boolean
  appliesOnRestart: boolean
  save: () => Promise<void>
}

const settingsCategories: Record<
  SettingsCategory,
  {
    caption: string
    description: string
    group: 'Server' | 'Operations'
    icon: string
    label: string
    title: string
  }
> = {
  general: {
    label: 'General',
    caption: 'Identity & placement',
    icon: 'badge',
    group: 'Server',
    title: 'General',
    description: "The server's identity and where it runs.",
  },
  network: {
    label: 'Network & Launch',
    caption: 'Ports & executable',
    icon: 'lan',
    group: 'Server',
    title: 'Network & Launch',
    description: 'Connection endpoints and process override.',
  },
  dns: {
    label: 'DNS',
    caption: 'Manual record sync',
    icon: 'dns',
    group: 'Server',
    title: 'DNS Binding',
    description: 'Manually synchronize one DNS name to this game server.',
  },
  capacity: {
    label: 'Capacity',
    caption: 'Players & recovery',
    icon: 'memory',
    group: 'Server',
    title: 'Capacity & Recovery',
    description: 'Player limits and automatic restart behavior.',
  },
  admin: {
    label: 'Remote Admin',
    caption: 'Credentials & access',
    icon: 'admin_panel_settings',
    group: 'Operations',
    title: 'Remote Administration',
    description: 'Remote access credentials for this game server.',
  },
  environment: {
    label: 'Environment',
    caption: 'Variables & secrets',
    icon: 'key',
    group: 'Operations',
    title: 'Environment',
    description: 'Runtime variables and encrypted secrets.',
  },
  backups: {
    label: 'Backups',
    caption: 'Storage & retention',
    icon: 'backup',
    group: 'Operations',
    title: 'Backup Policy',
    description: 'Storage location and automated retention.',
  },
}

const router = useRouter()
const $q = useQuasar()
const activeCategory = ref<SettingsCategory>('general')
const joinPasswordSettings = ref<InstanceType<typeof JoinPasswordSettings> | null>(null)
const dnsBindingSettings = ref<InstanceType<typeof GameServerDnsBindingSettings> | null>(null)
const backupSettings = ref<BackupSettings>(create(BackupSettingsSchema))
const backupOverview = ref<GameServerBackupOverview>(create(GameServerBackupOverviewSchema))
const backupSettingsLoading = ref(true)
const backupSettingsLoadError = ref('')
const environmentRows = ref<EnvironmentVariable[]>([])
const environmentSnapshot = ref('')
const environmentIssues = ref<EnvironmentValidationIssue[]>([])
const environmentLoading = ref(true)
const environmentLoadError = ref('')
const adminInterface = ref<GameServerAdminInterface>(create(GameServerAdminInterfaceSchema))
const adminInterfaceLoading = ref(true)
const adminInterfacePassword = ref('')
const secretEnvironmentStates = ref<SecretEnvironmentVariableState[]>([])
const secretEnvironmentName = ref('')
const secretEnvironmentValue = ref('')
const secretEnvironmentSaving = ref(false)
const savedCoreSnapshot = ref<Record<CoreCategory, string> | null>(null)
const liveStatus = ref<Status>()
const environmentDirty = computed(() => {
  if (!environmentSnapshot.value) {
    return false
  }

  return serializeEnvironmentRows(environmentRows.value) !== environmentSnapshot.value
})
const backupEnableBlocked = computed(() => !backupSettings.value.backupsSupported)
const backupSettingsSnapshot = ref('')
const backupSettingsDirty = computed(
  () =>
    backupSettingsSnapshot.value !== '' &&
    serializeBackupSettings(backupSettings.value) !== backupSettingsSnapshot.value,
)
const adminInterfaceEndpoint = computed(() => {
  const address = adminInterface.value.bindAddress
  const displayAddress =
    address.includes(':') && !address.startsWith('[') ? `[${address}]` : address
  return `${displayAddress}:${adminInterface.value.port.toString()}`
})
const adminPortSource = computed(() => {
  const { supported, portField, portOffset } = adminInterface.value
  if (!supported || !portField) {
    return ''
  }
  const field = portField === 'query_port' ? 'Query Port' : 'Port'
  return portOffset > 0n ? `${field} + ${portOffset}` : field
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
  maxPlayersHint,
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
  setPlayersHint,
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
const showJoinPassword = computed(() => gameServer.value.gameId === 'valheim')
const categoryGroups = computed(() => {
  const visible = (Object.keys(settingsCategories) as SettingsCategory[]).filter((category) => {
    if (category === 'network' || category === 'dns') {
      return props.canEditProvisioning
    }
    if (category === 'admin') {
      return adminInterface.value.supported
    }
    return true
  })

  return (['Server', 'Operations'] as const).map((group) => ({
    group,
    categories: visible.filter((category) => settingsCategories[category].group === group),
  }))
})

const coreDirtyCategories = computed<CoreCategory[]>(() => {
  const saved = savedCoreSnapshot.value
  if (!saved) {
    return []
  }

  const current = coreSnapshot(gameServer.value)
  return (Object.keys(current) as CoreCategory[]).filter(
    (category) => current[category] !== saved[category],
  )
})

// Sections that keep their own RPC; the header Save commits each one that changed.
const separateSections = computed<SettingsSection[]>(() => [
  {
    category: 'general',
    label: 'Join password',
    dirty: joinPasswordSettings.value?.dirty === true,
    appliesOnRestart: true,
    save: async () => joinPasswordSettings.value?.save(),
  },
  {
    category: 'dns',
    label: 'DNS binding',
    dirty: dnsBindingSettings.value?.dirty === true,
    appliesOnRestart: false,
    save: async () => dnsBindingSettings.value?.save(),
  },
  {
    category: 'admin',
    label: 'Admin password',
    dirty: adminInterfacePassword.value.length > 0,
    appliesOnRestart: true,
    save: saveAdminInterfacePassword,
  },
  {
    category: 'environment',
    label: 'Environment',
    dirty: environmentDirty.value,
    appliesOnRestart: true,
    save: saveEnvironmentSettings,
  },
  {
    category: 'backups',
    label: 'Backup policy',
    dirty: backupSettingsDirty.value,
    appliesOnRestart: false,
    save: saveBackupSettings,
  },
])

const dirtyCategories = computed(
  () =>
    new Set<SettingsCategory>([
      ...coreDirtyCategories.value,
      ...separateSections.value
        .filter((section) => section.dirty)
        .map((section) => section.category),
    ]),
)
const hasSaveableChanges = computed(() => dirtyCategories.value.size > 0)
const serverRunning = computed(() => {
  const status = liveStatus.value ?? gameServer.value.status
  return status === Status.ONLINE || status === Status.PRE_START
})

// A typed secret value is not part of Save changes, but leaving would still lose it.
useUnsavedChangesGuard(() => hasSaveableChanges.value || secretEnvironmentValue.value !== '')

onMounted(async () => {
  XylonaEventBus.on('gameServerStatus', onServerStatus)
  await initialize()
  savedCoreSnapshot.value = coreSnapshot(gameServer.value)
  await Promise.all([
    initializeAdminInterface(),
    initializeBackupSettings(),
    initializeEnvironmentSettings(),
  ])
})

onBeforeUnmount(() => {
  XylonaEventBus.off('gameServerStatus', onServerStatus)
})

function onServerStatus(serverID: string, _serverName: string, status: Status) {
  if (serverID === props.gameServerId) {
    liveStatus.value = status
  }
}

function coreSnapshot(server: GameServer): Record<CoreCategory, string> {
  const serialize = (values: unknown[]) => JSON.stringify(values.map((value) => String(value)))
  return {
    general: serialize([
      server.name,
      server.gameId,
      server.userId,
      server.nodeId,
      server.ip?.address,
    ]),
    network: serialize([server.port, server.queryPort, server.serverExecutable]),
    capacity: serialize([
      server.setMaxPlayers,
      server.maxPlayers,
      server.maxMemoryMb,
      server.autoRestartEnabled,
      server.autoRestartMaxRetries,
      server.autoRestartCooldownSeconds,
    ]),
  }
}

function adminPortHint(field: 'port' | 'query_port'): string | undefined {
  const { supported, portField, portOffset, transport } = adminInterface.value
  if (!supported || portField !== field) {
    return undefined
  }

  return portOffset > 0n
    ? `Remote admin (${transport}) uses this port + ${portOffset}.`
    : `Also the remote admin (${transport}) port.`
}

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
  await router.push(`/game-servers/${props.gameServerId}/console`)
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
}

async function initializeBackupSettings() {
  backupSettingsLoading.value = true
  backupSettingsLoadError.value = ''

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
    backupSettingsSnapshot.value = serializeBackupSettings(backupSettings.value)
  } catch (e) {
    backupSettingsSnapshot.value = ''
    backupSettingsLoadError.value = ConnectErrorToString(ConnectError.from(e))
  } finally {
    backupSettingsLoading.value = false
  }
}

async function initializeEnvironmentSettings() {
  environmentLoading.value = true
  environmentLoadError.value = ''

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
    environmentLoadError.value = ConnectErrorToString(ConnectError.from(e))
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
    notifySuccess('Secret saved successfully.')
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

function confirmClearSecretEnvironment(name: string) {
  $q.dialog({
    title: `Clear secret ${name}?`,
    message: "The value can't be recovered. The server keeps using it until it restarts.",
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Clear secret' },
    persistent: true,
  }).onOk(() => void clearSecretEnvironment(name))
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
    notifySuccess('Secret cleared successfully.')
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

function serializeBackupSettings(settings: BackupSettings): string {
  return JSON.stringify([
    settings.backupsEnabled,
    settings.backupDirectory,
    settings.maxBackups.toString(),
  ])
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
  if (backupEnableBlocked.value && backupSettings.value.backupsEnabled) {
    throw new Error('Turn off Enable Backups; new backups are unavailable for this server.')
  }
  if (backupSettings.value.maxBackups < 1n) {
    throw new Error('Keep at least 1 automated backup.')
  }

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
}

async function saveCoreSettings() {
  const request: EditGameServerRequest = create(EditGameServerRequestSchema, {})
  request.serverId = props.gameServerId
  request.gameServer = gameServer.value

  await GetXylonaClient().editGameServer(request)
  savedCoreSnapshot.value = coreSnapshot(gameServer.value)
  await initializeAdminInterface()
}

async function saveAllChanges() {
  const coreDirty = coreDirtyCategories.value.length > 0
  if (
    coreDirty &&
    !(await validateBeforeSave('Complete the required fields before saving this server.'))
  ) {
    await revealFirstInvalidCategory()
    return
  }

  const pending: Array<Omit<SettingsSection, 'category' | 'dirty'>> = [
    ...(coreDirty
      ? [{ label: 'Server settings', appliesOnRestart: true, save: saveCoreSettings }]
      : []),
    ...separateSections.value.filter((section) => section.dirty),
  ]
  if (pending.length === 0) {
    return
  }
  const saved: string[] = []
  const failed: string[] = []
  let restartNeeded = false

  startSubmitting()
  try {
    // One section at a time: later sections (the join password) validate against saved ones.
    for (const section of pending) {
      try {
        await section.save()
        saved.push(section.label)
        restartNeeded ||= section.appliesOnRestart
      } catch (e) {
        console.error(e)
        failed.push(`${section.label}: ${connectErrorMessage(e)}`)
      }
    }
  } finally {
    resetSubmissionState()
  }

  const savedText = saved.length > 0 ? `${saved.join(', ')} saved.` : ''
  if (failed.length > 0) {
    notifyError(`${savedText} Not saved: ${failed.join('; ')}`.trim())
    return
  }
  notifySuccess(
    restartNeeded && serverRunning.value ? `${savedText} Restart the server to apply.` : savedText,
  )
}
</script>

<style scoped>
.settings-form-shell {
  --xy-header-stack-height: 0px;
}

/* The workspace is the one frame under the header; the shell body adds none. */
.settings-form-shell :deep(.server-form-body) {
  padding: 0;
  background: none;
  border: none;
}

.settings-workspace {
  display: grid;
  grid-template-columns: minmax(180px, 210px) minmax(0, 1fr);
  overflow: hidden;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 var(--xy-radius-lg) var(--xy-radius-lg);
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

.settings-category-button > span:not(.settings-category-dirty) {
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

.settings-category-dirty {
  flex: 0 0 auto;
  width: 8px;
  height: 8px;
  margin-left: auto;
  background: var(--xy-warning);
  border-radius: var(--xy-radius-pill);
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

.settings-scope--dirty {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg-faint);
  border-color: var(--xy-warning-border);
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

.settings-inline-link {
  padding: 0;
  color: var(--xy-primary);
  background: none;
  border: none;
  font: inherit;
  text-decoration: underline;
  cursor: pointer;
}

.settings-inline-link:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 2px;
}

.admin-interface-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: var(--xy-space-md);
}

.admin-interface-password {
  max-width: 480px;
}

.environment-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 380px), 1fr));
  gap: var(--xy-space-lg);
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
  /* Same height with or without the add button, so both column headings line up. */
  min-height: 40px;
}

.environment-panel-title {
  color: var(--xy-text-emphasis-soft);
  font-size: var(--xy-font-size-sm);
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
  align-items: center;
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
  font-size: var(--xy-font-size-sm);
  overflow-wrap: anywhere;
}

.secret-environment-updated {
  line-height: 1.3;
}

.secret-environment-editor {
  display: grid;
  grid-template-columns: minmax(120px, 0.8fr) minmax(160px, 1fr) auto;
  gap: var(--xy-space-sm);
  align-items: center;
}

@media (max-width: 599px) {
  .environment-row,
  .secret-environment-editor {
    grid-template-columns: 1fr;
  }

  .environment-row :deep(.q-btn),
  .secret-environment-editor :deep(.q-btn) {
    justify-self: flex-start;
  }
}

@media (max-width: 1023px) {
  .settings-workspace {
    grid-template-columns: 1fr;
  }

  /* Wrap instead of scrolling sideways so every category stays visible. */
  .settings-category-rail {
    grid-column: 1;
    position: static;
    flex-direction: row;
    flex-wrap: wrap;
    padding: var(--xy-space-xs);
    border-right: 0;
    border-bottom: 1px solid var(--xy-border);
  }

  .settings-category-group {
    display: none;
  }

  .settings-category-button {
    flex: 0 0 auto;
    width: auto;
    padding: var(--xy-space-sm);
  }

  .settings-category-button small {
    display: none;
  }

  .settings-category-dirty {
    margin-left: 0;
  }

  .settings-panel {
    grid-column: 1;
  }
}

@media (max-width: 599px) {
  .settings-panel-heading {
    flex-direction: column;
    gap: var(--xy-space-sm);
  }

  .settings-panel {
    padding: var(--xy-space-md);
  }
}
</style>
