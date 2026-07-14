<template>
  <div class="identity-bar">
    <div class="identity-bar-left">
      <span class="identity-bar-name">{{ gameServer.name }}</span>
      <span class="identity-bar-detail">
        <span>{{ gameServer.gameName }}</span>
        <template v-if="hasSoftwareOptions && !softwareNameRedundant">
          <span class="identity-bar-sep">&middot;</span>
          <span class="identity-bar-running">running</span>
          <span>{{ softwareDisplayName }}</span>
        </template>
        <template v-if="displayVersion && hasSoftwareOptions">
          <span
            :class="{ 'version-outdated': versionDisplay.updateAvailable }"
            class="identity-bar-version">
            {{ displayVersion }}
            <q-tooltip v-if="versionDisplay.updateAvailable">
              Update available: {{ versionDisplay.latestVersion }}
            </q-tooltip>
          </span>
        </template>
      </span>
    </div>
    <div class="identity-bar-spacer"></div>
    <status-badge :status="gameServer.status" />
  </div>

  <div :class="{ 'main-area-expanded': consoleExpanded }" class="main-area">
    <!-- Sidebar -->
    <aside :class="{ collapsed: sidebarCollapsed }" class="sidebar">
      <div class="sidebar-content">
        <!-- Controls -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Controls</div>
          <div class="server-controls">
            <q-btn
              :disable="disableStartButton || !hasPermission('game_server.start')"
              :loading="startingServer"
              color="positive"
              label="Start"
              @click="startGameServer">
              <q-tooltip v-if="!hasPermission('game_server.start')">
                Requires start permission
              </q-tooltip>
              <q-tooltip v-else-if="!serverStateAuthoritative">
                Waiting for authoritative server status
              </q-tooltip>
            </q-btn>
            <q-btn
              :disable="disableStopButton || !hasPermission('game_server.stop')"
              :loading="stoppingServer"
              color="negative"
              label="Stop"
              @click="stopGameServer">
              <q-tooltip v-if="!hasPermission('game_server.stop')">
                Requires stop permission
              </q-tooltip>
              <q-tooltip v-else-if="!serverStateAuthoritative">
                Waiting for authoritative server status
              </q-tooltip>
            </q-btn>
            <q-btn
              v-if="showUpdateButton"
              :disable="disableUpdateButton || !hasPermission('game_server.settings')"
              :loading="updatingServer"
              class="update-server-btn"
              color="primary"
              label="Update"
              @click="updateGameServer">
              <q-tooltip v-if="!hasPermission('game_server.settings')">
                Requires settings permission
              </q-tooltip>
              <q-tooltip v-else-if="!serverStateAuthoritative">
                Waiting for authoritative server status
              </q-tooltip>
            </q-btn>
          </div>
        </div>

        <div v-if="readinessVisible" class="sidebar-section">
          <div class="sidebar-section-label">Readiness</div>
          <div class="readiness-list">
            <div
              v-for="item in visibleReadinessItems"
              :key="item.kind"
              :class="{ 'readiness-item--complete': item.complete }"
              class="readiness-item">
              <div class="readiness-item-icon">
                <q-icon :name="item.complete ? 'verified' : 'report_problem'" />
              </div>
              <div class="readiness-item-body">
                <div class="readiness-item-title">{{ readinessLabel(item.kind) }}</div>
                <div class="readiness-item-message">{{ item.message }}</div>
                <q-btn
                  v-if="item.kind === 'minecraft_eula' && hasPermission('game_server.settings')"
                  :loading="acceptingMinecraftEula"
                  class="readiness-action"
                  color="primary"
                  dense
                  label="Accept EULA"
                  unelevated
                  @click="acceptMinecraftEula" />
                <div
                  v-if="item.kind === 'steam_gslt' && hasPermission('game_server.settings')"
                  class="readiness-secret-form">
                  <q-input
                    v-model="steamGSLT"
                    autocomplete="off"
                    class="readiness-secret-input"
                    dense
                    label="Steam GSLT"
                    outlined
                    type="password" />
                  <div class="readiness-secret-actions">
                    <q-btn
                      :disable="steamGSLT.trim() === ''"
                      :loading="savingSteamGSLT"
                      color="primary"
                      dense
                      label="Save Token"
                      unelevated
                      @click="saveSteamGSLT" />
                    <q-btn
                      v-if="item.complete"
                      :loading="clearingSteamGSLT"
                      color="negative"
                      dense
                      flat
                      label="Clear"
                      @click="clearSteamGSLT" />
                  </div>
                </div>
                <div
                  v-if="item.kind === 'hytale_account' && hasPermission('game_server.settings')"
                  class="readiness-secret-form">
                  <q-btn
                    v-if="!item.complete && hytaleFlowId === '' && hytaleProfiles.length === 0"
                    :loading="startingHytaleAuth"
                    color="primary"
                    dense
                    label="Link Account"
                    unelevated
                    @click="startHytaleDeviceAuth" />
                  <div
                    v-if="hytaleFlowId !== '' && hytaleProfiles.length === 0"
                    class="readiness-device-flow">
                    <div v-if="hytaleUserCode !== ''" class="readiness-device-code">
                      {{ hytaleUserCode }}
                    </div>
                    <a
                      v-if="hytaleVerificationLink !== ''"
                      :href="hytaleVerificationLink"
                      class="readiness-link"
                      rel="noopener noreferrer"
                      target="_blank">
                      Open Hytale authorization
                    </a>
                    <q-btn
                      :loading="pollingHytaleAuth"
                      color="primary"
                      dense
                      label="Check Status"
                      outline
                      @click="pollHytaleDeviceAuth" />
                  </div>
                  <div v-if="hytaleProfiles.length > 0" class="readiness-device-flow">
                    <q-select
                      v-model="selectedHytaleProfile"
                      :options="hytaleProfileOptions"
                      class="readiness-profile-select"
                      dense
                      emit-value
                      label="Profile"
                      map-options
                      outlined />
                    <q-btn
                      :disable="selectedHytaleProfile === ''"
                      :loading="selectingHytaleProfile"
                      color="primary"
                      dense
                      label="Use Profile"
                      unelevated
                      @click="selectHytaleProfile" />
                  </div>
                  <q-btn
                    v-if="item.complete"
                    :loading="clearingHytaleAccount"
                    color="negative"
                    dense
                    flat
                    label="Clear Link"
                    @click="clearHytaleAccount" />
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Variant -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Variant</div>
          <div class="software-card">
            <template v-if="hasSoftwareOptions">
              <div class="software-card-body">
                <div class="software-icon">
                  <q-icon name="settings" />
                </div>
                <div class="software-info">
                  <div class="software-name-row">
                    <span class="software-name">
                      {{ softwareNameRedundant ? gameServer.gameName : softwareDisplayName }}
                    </span>
                    <span v-if="displayVersion" class="software-version">
                      {{ displayVersion }}
                    </span>
                  </div>
                  <div v-if="!softwareNameRedundant" class="software-game">
                    {{ gameServer.gameName }}
                  </div>
                  <div v-if="variantTrackingLabel" class="software-track-state">
                    {{ variantTrackingLabel }}
                  </div>
                </div>
              </div>
              <div v-if="versionDisplay.updateAvailable" class="update-hint">
                <span class="update-dot"></span>
                {{ displayVersion }} &rarr; {{ versionDisplay.latestVersion }}
              </div>
              <div v-if="showChangeButton" class="software-card-footer">
                <button class="change-btn" @click="softwareSelector?.openChangeDialog()">
                  Change
                  <span class="change-arrow">&rsaquo;</span>
                </button>
              </div>
            </template>
            <div v-else class="software-card-body">
              <div class="software-icon">
                <q-icon name="sports_esports" />
              </div>
              <div class="software-info">
                <div class="software-name-row">
                  <span class="software-name">{{ gameServer.gameName }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>

        <!-- Connection -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Connection</div>
          <div class="connection-list">
            <div class="connection-item">
              <span class="cl-label">Address</span>
              <span class="cl-value">
                <clip-board-copy
                  :clip-board-value="connectionAddress"
                  :display-text="connectionAddress" />
              </span>
            </div>
            <div class="connection-item">
              <span class="cl-label">Players</span>
              <span class="cl-value cl-value-plain">
                {{ currentPlayerCount }} / {{ maxPlayerCount }}
              </span>
            </div>
          </div>
        </div>

        <!-- Resource Usage -->
        <div class="sidebar-section">
          <div class="sidebar-section-label">Resource Usage</div>
          <div :class="{ 'metrics-offline': !isServerOnline }" class="metrics-preview">
            <!-- Compute -->
            <div class="metrics-group">
              <div class="metrics-group-label">Compute</div>
              <div>
                <div class="metric-row">
                  <span class="ml"
                    >CPU
                    <span class="metric-detail"
                      >({{ isServerOnline ? metricsCpuCores : '--' }} cores)</span
                    ></span
                  >
                  <span class="mv">{{ isServerOnline ? metricsCpu.toFixed(1) + '%' : '--' }}</span>
                </div>
                <div class="metric-bar">
                  <div
                    :class="cpuBarClass"
                    :style="{
                      transform: `scaleX(${isServerOnline ? Math.min(Math.max(metricsCpu / 100, 0), 1) : 0})`,
                    }"
                    class="metric-bar-fill"></div>
                </div>
              </div>
              <div class="metric-row">
                <span class="ml">Threads</span>
                <span class="mv">{{ isServerOnline ? metricsThreads : '--' }}</span>
              </div>
            </div>

            <!-- Memory -->
            <div class="metrics-group">
              <div class="metrics-group-label">Memory</div>
              <div>
                <div class="metric-row">
                  <span class="ml">Memory</span>
                  <span class="mv">
                    <template v-if="isServerOnline">
                      {{ bytesToSize(metricsMemory) }}
                      <template v-if="metricsMaxMemory > 0">
                        / {{ bytesToSize(metricsMaxMemory) }}
                      </template>
                    </template>
                    <template v-else>--</template>
                  </span>
                </div>
                <div v-if="metricsMaxMemory > 0" class="metric-bar">
                  <div
                    :class="memoryBarClass"
                    :style="{
                      transform: `scaleX(${isServerOnline ? Math.min(Math.max(metricsMemoryRatio, 0), 1) : 0})`,
                    }"
                    class="metric-bar-fill"></div>
                </div>
              </div>
              <div v-if="isServerOnline && metricsMemoryPercent > 0" class="metric-row">
                <span class="ml">System RAM</span>
                <span class="mv">{{ metricsMemoryPercent.toFixed(1) }}%</span>
              </div>
            </div>

            <!-- Storage -->
            <div class="metrics-group">
              <div class="metrics-group-label">Storage</div>
              <div class="metric-row">
                <span class="ml">Disk Usage</span>
                <span class="mv">{{ isServerOnline ? bytesToSize(metricsDisk) : '--' }}</span>
              </div>
              <div class="metric-row">
                <span class="ml">I/O Read</span>
                <span class="mv">{{ isServerOnline ? formatRate(metricsIoReadRate) : '--' }}</span>
              </div>
              <div class="metric-row">
                <span class="ml">I/O Write</span>
                <span class="mv">{{ isServerOnline ? formatRate(metricsIoWriteRate) : '--' }}</span>
              </div>
            </div>

            <!-- Network -->
            <div class="metrics-group">
              <div class="metrics-group-label">Network</div>
              <div class="metric-row">
                <span class="ml">Connections</span>
                <span class="mv">{{ isServerOnline ? metricsConnections : '--' }}</span>
              </div>
              <div class="metric-row">
                <span class="ml">Uptime</span>
                <span class="mv">{{ isServerOnline ? formattedUptime : '--' }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </aside>

    <!-- Console wrapper -->
    <div :class="{ expanded: consoleExpanded }" class="console-wrapper">
      <!-- Console toolbar buttons -->
      <div class="console-toolbar-btns">
        <q-btn
          :aria-label="sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar'"
          :icon="sidebarCollapsed ? 'chevron_right' : 'chevron_left'"
          class="console-toolbar-btn"
          dense
          flat
          padding="xs"
          square
          @click="sidebarCollapsed = !sidebarCollapsed">
          <q-tooltip>{{ sidebarCollapsed ? 'Show sidebar' : 'Hide sidebar' }}</q-tooltip>
        </q-btn>
        <q-btn
          :aria-label="consoleAutoScroll ? 'Auto-scroll enabled' : 'Auto-scroll disabled'"
          :class="{ 'console-toolbar-btn-off': !consoleAutoScroll }"
          :text-color="consoleAutoScroll ? 'info' : undefined"
          class="console-toolbar-btn"
          dense
          flat
          icon="vertical_align_bottom"
          padding="xs"
          square
          @click="toggleAutoScroll">
          <q-tooltip>{{
            consoleAutoScroll ? 'Auto-scroll enabled' : 'Auto-scroll disabled'
          }}</q-tooltip>
        </q-btn>
        <q-btn
          :icon="tabMaximize"
          aria-label="Toggle fullscreen console"
          class="console-toolbar-btn"
          dense
          flat
          padding="xs"
          square
          text-color="info"
          @click="consoleExpanded = !consoleExpanded" />
      </div>

      <div
        v-if="consoleStreamState !== 'ready' || consoleLoadError"
        :class="{
          'console-stream-state--error': consoleStreamState === 'error' || consoleLoadError,
        }"
        class="console-stream-state"
        :role="consoleStreamState === 'error' || consoleLoadError ? 'alert' : 'status'"
        aria-live="assertive">
        <q-spinner
          v-if="consoleStreamState === 'loading' || consoleStreamState === 'reconnecting'"
          color="info"
          size="1rem" />
        <q-icon v-else name="sync_problem" size="sm" />
        <span v-if="consoleStreamState === 'loading'">Loading console output…</span>
        <span v-else-if="consoleStreamState === 'reconnecting'">
          Console connection interrupted. Reconnecting…
        </span>
        <span v-else>{{ consoleLoadError || 'Console output is unavailable.' }}</span>
        <q-btn
          v-if="consoleStreamState === 'error' || consoleLoadError"
          dense
          flat
          icon="refresh"
          label="Retry"
          @click="retryConsoleOutput" />
      </div>

      <!-- Console output -->
      <template v-if="showConsolePlaceholder">
        <div class="console-output console-output-offline">
          <div class="offline-placeholder">
            <div class="offline-icon">
              <q-icon :name="isServerStatusUnknown ? 'sync_problem' : 'power_settings_new'" />
            </div>
            <div class="offline-text">
              {{ isServerStatusUnknown ? 'Server status unavailable' : 'Server is offline' }}
            </div>
            <div class="offline-hint">
              {{
                isServerStatusUnknown
                  ? 'Lifecycle controls are paused until status is confirmed'
                  : 'Press Start to launch the server'
              }}
            </div>
          </div>
        </div>
      </template>
      <template v-else>
        <q-scroll-area id="consoleContainer" ref="consoleScrollArea" class="console-scroll-area">
          <div
            v-if="
              (isServerOffline || isServerStatusUnknown) &&
              !updateInProgress &&
              !softwareOperationInProgress
            "
            class="console-status-banner">
            {{ isServerStatusUnknown ? 'Server status unavailable.' : 'Server offline.' }}
          </div>
          <div v-if="consoleTruncated" class="console-truncated-notice">
            Earlier output truncated
          </div>
          <!-- eslint-disable vue/no-v-html -- accepted per CLAUDE.md: game server console output -->
          <code
            id="consoleCodeEl"
            aria-label="Game server console output"
            aria-live="polite"
            class="q-pb-md"
            role="log">
            <span v-for="line in consoleLines" :key="line.id" v-html="line.html"></span>
          </code>
          <!-- eslint-enable vue/no-v-html -->
        </q-scroll-area>
      </template>

      <!-- Console input -->
      <div
        :class="{ 'console-input-disabled': consoleInputDisabled }"
        class="console-input-wrapper">
        <span class="console-prompt">&gt;</span>
        <q-input
          id="consoleInput"
          v-model="serverInput"
          :disable="consoleInputDisabled"
          autofocus
          borderless
          class="console-input-field"
          dense
          name="consoleInput"
          placeholder="Enter command..."
          square
          @keyup.enter="sendGameServerInput"
          @keyup.up="navigateConsoleInputHistory('up')"
          @keyup.down="navigateConsoleInputHistory('down')">
          <template #append>
            <q-btn
              :disable="consoleInputDisabled"
              :loading="sendingConsoleInput"
              aria-label="Send command"
              color="primary"
              flat
              icon="send"
              name="send"
              type="submit"
              @click="sendGameServerInput">
              <q-tooltip v-if="!hasPermission('game_server.console')">
                Requires console permission
              </q-tooltip>
            </q-btn>
          </template>
        </q-input>
      </div>
    </div>
  </div>

  <!-- ServerSoftwareSelector (renders its own dialog, kept mounted for ref access) -->
  <server-software-selector
    v-if="gameServer.gameId !== ''"
    ref="softwareSelector"
    :current-installed-version="gameServer.versionInfo?.installedVersion || gameServer.version"
    :current-software="gameServer.selectedVariantId"
    :current-target="gameServer.selectedTarget"
    :current-target-pinned="gameServer.selectedTargetPinned"
    :current-version="displayVersion"
    :game-name="gameServer.gameName"
    :game-server-id="gameServerId"
    :variants="gameServer.game?.variants ?? []"
    @software-changed="handleSoftwareChanged"
    @software-operation-state="onSoftwareOperationState" />

  <operation-progress-dialog
    v-model="softwareOperationOpen"
    :complete="softwareOperationComplete"
    :context-facts="softwareOperationContextFacts"
    :output-lines="softwareOperationOutputLines"
    :show-output-area="true"
    :steps="softwareOperationSteps"
    subtitle="Xylona will apply the selected variant and refresh the detected version when it finishes."
    title="Changing Variant" />
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import ClipBoardCopy from '@/components/ClipBoardCopy.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import OperationProgressDialog from '@/components/game_servers/OperationProgressDialog.vue'
import ServerSoftwareSelector from '@/components/game_servers/ServerSoftwareSelector.vue'
import type {
  OperationContextFact,
  StepState,
} from '@/components/game_servers/UpdateProgressPanel.types'
import type { ServerSoftwareOperationEvent } from '@/components/game_servers/ServerSoftwareSelector.types'
import { QScrollArea, useQuasar } from 'quasar'
import { tabMaximize } from 'quasar-extras-svg-icons/tabler-icons-v2'
import {
  GameServer,
  GameServerSchema,
  ReadGameServerOutputRequest,
  ReadGameServerOutputRequestSchema,
  ReadGameServerOutputResponse,
  SendGameServerInputRequest,
  SendGameServerInputRequestSchema,
  StartGameServerRequest,
  StartGameServerRequestSchema,
  Status,
  StopGameServerRequest,
  StopGameServerRequestSchema,
} from '@/proto/shared_pb'
import type { GameServerReadinessItem, HytaleProfile, UpdateProgress } from '@/proto/xylona_pb'
import {
  AcceptMinecraftEulaRequestSchema,
  ClearHytaleAccountRequestSchema,
  ClearSteamGSLTRequestSchema,
  GetGameServerRequest,
  GetGameServerRequestSchema,
  GetGameServerReadinessRequestSchema,
  GetUpdateTargetsRequestSchema,
  PollHytaleDeviceAuthRequestSchema,
  SelectHytaleProfileRequestSchema,
  SetSteamGSLTRequestSchema,
  StartHytaleDeviceAuthRequestSchema,
  StepStatus,
  UpdateGameServerRequest,
  UpdateGameServerRequestSchema,
  UpdateStep,
} from '@/proto/xylona_pb'
import { ConnectError } from '@connectrpc/connect'
import { canShowUpdateButton } from './game-server-update-capability'
import {
  appendOperationOutputLines,
  normalizeOperationOutputChunk,
  resolveOperationOutputRoute,
} from './operation-output'
import {
  applyUpdateProgress,
  buildUpdateStepLabels,
  buildUpdateSteps,
  isUpdateProgressTerminal,
} from './update-progress'
import {
  bytesToSize,
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import { recordLifecycleIntent } from '@/utils/game-server-notifications'
import { computed, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { useRoute } from 'vue-router'
import { resolveCanonicalVersionDisplay, resolveVariantTrackingLabel } from './version-display'
import { canSelectSteamBranch, chooseSteamBranchForUpdate } from './steam-branch-update'
import { useGameServerConsoleState } from './useGameServerConsoleState'
import { useGameServerMetricsPreview } from './useGameServerMetricsPreview'
import { useGameServerQueryStatusVersion } from './useGameServerQueryStatusVersion'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'
import { resolveConsoleStreamChunk } from './console-stream-sequence'

const $q = useQuasar()
const route = useRoute()
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const gameServerId: Ref<string> = ref(
  route.params.id instanceof Array ? route.params.id[0] : route.params.id,
)
const consoleScrollArea = ref<QScrollArea | null>(null)
const softwareSelector = ref<InstanceType<typeof ServerSoftwareSelector> | null>(null)
const consoleExpanded = ref(false)
const sidebarCollapsed = ref(false)

function scrollConsoleToBottom() {
  const el = consoleScrollArea.value?.$el as HTMLElement | undefined
  const container = el?.querySelector('.q-scrollarea__container') as HTMLElement | null
  if (container) {
    container.scrollTop = container.scrollHeight
  }
}

const {
  appendConsoleOutput,
  cancelPendingConsoleFlush,
  consoleAutoScroll,
  consoleLines,
  consoleTruncated,
  navigateConsoleInputHistory,
  recordConsoleInput,
  replaceConsoleOutput,
  serverInput,
  toggleConsoleAutoScroll: toggleAutoScroll,
} = useGameServerConsoleState({
  gameID: computed(() => gameServer.value.gameId),
  scrollToBottom: scrollConsoleToBottom,
})

const startingServer = ref(false)
const stoppingServer = ref(false)
const serverStatusFresh = ref(false)
const sendingConsoleInput = ref(false)
const consoleStreamState = ref<'loading' | 'ready' | 'reconnecting' | 'error'>('loading')
const consoleLoadError = ref('')
const lastConsoleSequence = ref(0n)
const receivedConsoleReset = ref(false)
const updatingServer = ref(false)
const updateInProgress = ref(false)
const updateSteps = ref<StepState[]>([])
const softwareOperationOpen = ref(false)
const softwareOperationComplete = ref(false)
const softwareOperationSteps = ref<StepState[]>([])
const softwareOperationOutputLines = ref<string[]>([])
const softwareOperationContextFacts = ref<OperationContextFact[]>([])
const readinessItems = ref<GameServerReadinessItem[]>([])
const readinessLoading = ref(false)
const acceptingMinecraftEula = ref(false)
const steamGSLT = ref('')
const savingSteamGSLT = ref(false)
const clearingSteamGSLT = ref(false)
const hytaleFlowId = ref('')
const hytaleUserCode = ref('')
const hytaleVerificationUri = ref('')
const hytaleVerificationUriComplete = ref('')
const hytaleProfiles = ref<HytaleProfile[]>([])
const selectedHytaleProfile = ref('')
const startingHytaleAuth = ref(false)
const pollingHytaleAuth = ref(false)
const selectingHytaleProfile = ref(false)
const clearingHytaleAccount = ref(false)
const maxOperationOutputLines = 80
let gameServerDetailsRequestSequence = 0
let liveStatusSequence = 0

const {
  cpuBarClass,
  formatRate,
  formattedUptime,
  memoryBarClass,
  metricsConnections,
  metricsCpu,
  metricsCpuCores,
  metricsDisk,
  metricsIoReadRate,
  metricsIoWriteRate,
  metricsMaxMemory,
  metricsMemory,
  metricsMemoryPercent,
  metricsMemoryRatio,
  metricsThreads,
  startMetricsPreviewLifecycle,
} = useGameServerMetricsPreview({
  gameServer,
  gameServerId,
})
const { currentPlayerCount, maxPlayerCount, queryGameServer, startQueryStatusVersionLifecycle } =
  useGameServerQueryStatusVersion({
    gameServer,
    gameServerId,
  })

const isServerOnline = computed(() => gameServer.value.status === Status.ONLINE)
const isServerOffline = computed(() => gameServer.value.status === Status.OFFLINE)
const isServerStatusUnknown = computed(() => gameServer.value.status === Status.UNKNOWN)
const serverStateAuthoritative = computed(
  () =>
    websocketStateAuthoritative.value && serverStatusFresh.value && !isServerStatusUnknown.value,
)
const consoleInputDisabled = computed(
  () =>
    !hasPermission('game_server.console') ||
    !isServerOnline.value ||
    !serverStateAuthoritative.value ||
    sendingConsoleInput.value,
)
const hasConsoleOutput = computed(() => consoleLines.value.length > 0)
const softwareOperationInProgress = computed(() =>
  softwareOperationSteps.value.some((step) => step.status === StepStatus.IN_PROGRESS),
)
const showConsolePlaceholder = computed(
  () =>
    (isServerOffline.value || isServerStatusUnknown.value) &&
    !hasConsoleOutput.value &&
    !updateInProgress.value &&
    !softwareOperationInProgress.value,
)
const visibleReadinessItems = computed(() =>
  readinessItems.value.filter(
    (item) =>
      item.required &&
      (item.blocking ||
        ((item.kind === 'steam_gslt' || item.kind === 'hytale_account') &&
          item.complete &&
          hasPermission('game_server.settings'))),
  ),
)
const readinessVisible = computed(
  () => readinessLoading.value || visibleReadinessItems.value.length > 0,
)
const hytaleVerificationLink = computed(
  () => hytaleVerificationUriComplete.value || hytaleVerificationUri.value,
)
const hytaleProfileOptions = computed(() =>
  hytaleProfiles.value.map((profile) => ({
    label: profile.username || profile.uuid,
    value: profile.uuid,
  })),
)

const connectionAddress = computed(() => {
  const ip = gameServer.value.ip?.address ?? ''
  const port = gameServer.value.port.toString()
  if (!ip) return port
  return `${ip}:${port}`
})

function onEscapeKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && consoleExpanded.value) {
    consoleExpanded.value = false
  }
}

const disableStartButton = computed(() => {
  return !serverStateAuthoritative.value || gameServer.value.status !== Status.OFFLINE
})

const disableStopButton = computed(() => {
  return !serverStateAuthoritative.value || gameServer.value.status !== Status.ONLINE
})

const disableUpdateButton = computed(() => {
  return (
    !serverStateAuthoritative.value ||
    gameServer.value.status === Status.INSTALLING ||
    gameServer.value.status === Status.UPDATING ||
    updateInProgress.value
  )
})

const showUpdateButton = computed(() => {
  return canShowUpdateButton(gameServer.value)
})

const softwareDisplayName = computed(() => {
  return softwareSelector.value?.currentSoftwareDisplayName ?? ''
})

const hasSoftwareOptions = computed(() => {
  return (gameServer.value.game?.variants?.length ?? 0) > 0
})

const showChangeButton = computed(() => {
  if (!hasPermission('game_server.settings')) return false
  return (gameServer.value.game?.variants?.length ?? 0) > 1
})

const softwareNameRedundant = computed(() => {
  const swName = softwareDisplayName.value.toLowerCase()
  const gameName = gameServer.value.gameName.toLowerCase()
  return swName === gameName || gameName.includes(swName) || swName.includes(gameName)
})

const versionDisplay = computed(() => {
  return resolveCanonicalVersionDisplay(gameServer.value.version, gameServer.value.versionInfo)
})

const displayVersion = computed(() => {
  return versionDisplay.value.installedVersion
})

const variantTrackingLabel = computed(() => {
  return resolveVariantTrackingLabel(
    gameServer.value.resolvedUpdateProvider?.kind,
    gameServer.value.selectedTarget,
    gameServer.value.selectedTargetPinned,
  )
})

function hasPermission(perm: string): boolean {
  const perms = gameServer.value?.effectivePermissions ?? []
  // Empty permissions = unknown (cache fallback) — allow everything, backend enforces.
  return perms.length === 0 || perms.includes(perm)
}

function handleMobileSidebar() {
  if (window.innerWidth < 1024) {
    sidebarCollapsed.value = true
  }
}

onMounted(async () => {
  document.addEventListener('keydown', onEscapeKey)
  handleMobileSidebar()
  streamGameServerOutput()

  void getGameServerDetails()
    .then(() => {
      void loadReadiness()
      void retryConsoleOutput()
      startQueryStatusVersionLifecycle()
      startMetricsPreviewLifecycle()
    })
    .then(queryGameServer)
})

onBeforeUnmount(() => {
  cancelPendingConsoleFlush()
  document.removeEventListener('keydown', onEscapeKey)
  unsubscribeConsoleOutputStream()

  XylonaEventBus.off('gameServerUpdateProgress', onUpdateProgress)
  XylonaEventBus.off('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.off('websocketDisconnected', onWebsocketDisconnect)
  XylonaEventBus.off('gameServerConsoleOutput', onServerConsoleOutput)
  XylonaEventBus.off('gameServerStatus', onServerStatus)
})

function unsubscribeConsoleOutputStream() {
  const ws = GetOrCreateXylonaWebsocketClient()
  if (!ws.isOpen()) {
    return
  }
  XylonaEventBus.emit('gameServerConsoleOutputRemoveRequest', gameServerId.value)
}

async function requestConsoleOutputStream() {
  const ws = GetOrCreateXylonaWebsocketClient()
  await ws.waitForOpen(10_000)
  XylonaEventBus.emit('gameServerConsoleOutputRequest', gameServerId.value)
}

async function getGameServerDetails() {
  const requestSequence = ++gameServerDetailsRequestSequence
  const statusSequenceAtStart = liveStatusSequence
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
  try {
    request.id = gameServerId.value
    const response = await GetXylonaClient().getGameServer(request)
    if (requestSequence !== gameServerDetailsRequestSequence) {
      return false
    }
    if (response.gameServer === undefined) {
      serverStatusFresh.value = false
      gameServer.value = create(GameServerSchema, {
        ...gameServer.value,
        status: Status.UNKNOWN,
      })
      return false
    }
    gameServer.value =
      statusSequenceAtStart === liveStatusSequence
        ? response.gameServer
        : create(GameServerSchema, {
            ...response.gameServer,
            status: gameServer.value.status,
          })
    serverStatusFresh.value = websocketStateAuthoritative.value
    return true
  } catch (e) {
    if (requestSequence !== gameServerDetailsRequestSequence) {
      return false
    }
    serverStatusFresh.value = false
    gameServer.value = create(GameServerSchema, {
      ...gameServer.value,
      status: Status.UNKNOWN,
    })
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to load game server details: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
    return false
  }
}

async function loadReadiness() {
  readinessLoading.value = true
  try {
    const request = create(GetGameServerReadinessRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().getGameServerReadiness(request)
    readinessItems.value = response.items
  } catch (e) {
    console.error(e)
  } finally {
    readinessLoading.value = false
  }
}

async function acceptMinecraftEula() {
  acceptingMinecraftEula.value = true
  try {
    const request = create(AcceptMinecraftEulaRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().acceptMinecraftEula(request)
    readinessItems.value = response.items
    $q.notify({
      type: 'positive',
      position: 'top-right',
      caption: 'Minecraft EULA accepted.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to accept Minecraft EULA: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    acceptingMinecraftEula.value = false
  }
}

async function saveSteamGSLT() {
  savingSteamGSLT.value = true
  try {
    const request = create(SetSteamGSLTRequestSchema, {
      serverId: gameServerId.value,
      token: steamGSLT.value,
    })
    const response = await GetXylonaClient().setSteamGSLT(request)
    readinessItems.value = response.items
    steamGSLT.value = ''
    $q.notify({
      type: 'positive',
      position: 'top-right',
      caption: 'Steam GSLT saved.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to save Steam GSLT: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    savingSteamGSLT.value = false
  }
}

async function clearSteamGSLT() {
  clearingSteamGSLT.value = true
  try {
    const request = create(ClearSteamGSLTRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().clearSteamGSLT(request)
    readinessItems.value = response.items
    steamGSLT.value = ''
    $q.notify({
      type: 'positive',
      position: 'top-right',
      caption: 'Steam GSLT cleared.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to clear Steam GSLT: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    clearingSteamGSLT.value = false
  }
}

function resetHytaleFlow() {
  hytaleFlowId.value = ''
  hytaleUserCode.value = ''
  hytaleVerificationUri.value = ''
  hytaleVerificationUriComplete.value = ''
  hytaleProfiles.value = []
  selectedHytaleProfile.value = ''
}

async function startHytaleDeviceAuth() {
  startingHytaleAuth.value = true
  try {
    const request = create(StartHytaleDeviceAuthRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().startHytaleDeviceAuth(request)
    hytaleFlowId.value = response.flowId
    hytaleUserCode.value = response.userCode
    hytaleVerificationUri.value = response.verificationUri
    hytaleVerificationUriComplete.value = response.verificationUriComplete
    hytaleProfiles.value = []
    selectedHytaleProfile.value = ''
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption:
        'Failed to start Hytale authorization: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    startingHytaleAuth.value = false
  }
}

async function pollHytaleDeviceAuth() {
  if (hytaleFlowId.value === '') {
    return
  }
  pollingHytaleAuth.value = true
  try {
    const request = create(PollHytaleDeviceAuthRequestSchema, {
      flowId: hytaleFlowId.value,
    })
    const response = await GetXylonaClient().pollHytaleDeviceAuth(request)
    if (response.status === 'ready') {
      hytaleProfiles.value = response.profiles
      selectedHytaleProfile.value = response.profiles[0]?.uuid ?? ''
      return
    }
    if (response.status === 'denied' || response.status === 'expired') {
      resetHytaleFlow()
      $q.notify({
        type: 'xylona-error',
        position: 'top-right',
        caption: response.message || 'Hytale authorization was not completed.',
        icon: 'report_problem',
      })
    }
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption:
        'Failed to check Hytale authorization: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    pollingHytaleAuth.value = false
  }
}

async function selectHytaleProfile() {
  selectingHytaleProfile.value = true
  try {
    const request = create(SelectHytaleProfileRequestSchema, {
      serverId: gameServerId.value,
      flowId: hytaleFlowId.value,
      profileUuid: selectedHytaleProfile.value,
    })
    const response = await GetXylonaClient().selectHytaleProfile(request)
    readinessItems.value = response.items
    resetHytaleFlow()
    $q.notify({
      type: 'positive',
      position: 'top-right',
      caption: 'Hytale account linked.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to link Hytale account: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    selectingHytaleProfile.value = false
  }
}

async function clearHytaleAccount() {
  clearingHytaleAccount.value = true
  try {
    const request = create(ClearHytaleAccountRequestSchema, {
      serverId: gameServerId.value,
    })
    const response = await GetXylonaClient().clearHytaleAccount(request)
    readinessItems.value = response.items
    resetHytaleFlow()
    $q.notify({
      type: 'positive',
      position: 'top-right',
      caption: 'Hytale account link cleared.',
      icon: 'task_alt',
    })
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to clear Hytale account: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    clearingHytaleAccount.value = false
  }
}

function readinessLabel(kind: string): string {
  if (kind === 'minecraft_eula') {
    return 'Minecraft EULA'
  }
  if (kind === 'steam_gslt') {
    return 'Steam GSLT'
  }
  if (kind === 'hytale_account') {
    return 'Hytale account'
  }
  if (kind === 'sunkenland_world') {
    return 'Sunkenland world'
  }
  if (kind === 'dragonwilds_config') {
    return 'Dragonwilds configuration'
  }
  return 'Setup'
}

async function startGameServer() {
  if (!serverStateAuthoritative.value) {
    return
  }
  const request: StartGameServerRequest = create(StartGameServerRequestSchema, {})
  startingServer.value = true
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().startGameServer(request)
  } catch (e) {
    console.error(e)
    void loadReadiness()
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to start game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    startingServer.value = false
  }
}

async function stopGameServer() {
  if (!serverStateAuthoritative.value) {
    return
  }
  const request: StopGameServerRequest = create(StopGameServerRequestSchema, {})
  stoppingServer.value = true
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().stopGameServer(request)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to stop game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    stoppingServer.value = false
  }
}

function resetUpdateSteps() {
  updateSteps.value = buildUpdateSteps(
    gameServer.value.status,
    buildUpdateStepLabels({
      usesSteamcmd: Boolean(gameServer.value.game?.usesSteamcmd),
    }),
  )
}

function buildSoftwareOperationLabel(event: ServerSoftwareOperationEvent): string {
  if (event.versionLabel) {
    return `${event.softwareName} ${event.versionLabel}`
  }
  return event.softwareName
}

function currentSoftwareOperationLabel(): string {
  const softwareName = softwareDisplayName.value || gameServer.value.gameName
  if (gameServer.value.version) {
    return `${softwareName} ${gameServer.value.version}`
  }
  return softwareName
}

function buildSoftwareOperationContextFacts(targetLabel: string): OperationContextFact[] {
  return [
    { label: 'Current', value: currentSoftwareOperationLabel() },
    { label: 'Target', value: targetLabel },
    { label: 'Restart policy', value: 'No restart required' },
  ]
}

function baseSoftwareOperationSteps(targetLabel: string): StepState[] {
  return [
    {
      step: 'software-selection',
      label: `Selected ${targetLabel}`,
      status: StepStatus.COMPLETED,
    },
    {
      step: 'software-download',
      label: `Applying ${targetLabel}`,
      status: StepStatus.IN_PROGRESS,
    },
    {
      step: 'software-apply',
      label: 'Refreshing variant state',
      status: StepStatus.PENDING,
    },
  ]
}

function setSoftwareOperationStep(stepId: string, status: StepStatus, message?: string) {
  const stepIndex = softwareOperationSteps.value.findIndex((step) => step.step === stepId)
  if (stepIndex < 0) {
    return
  }
  softwareOperationSteps.value[stepIndex] = {
    ...softwareOperationSteps.value[stepIndex],
    status,
    message,
  }
}

function onSoftwareOperationState(event: ServerSoftwareOperationEvent) {
  const targetLabel = buildSoftwareOperationLabel(event)

  if (event.status === 'installing') {
    softwareOperationSteps.value = baseSoftwareOperationSteps(targetLabel)
    softwareOperationContextFacts.value = buildSoftwareOperationContextFacts(targetLabel)
    softwareOperationOutputLines.value = []
    softwareOperationComplete.value = false
    softwareOperationOpen.value = true
    return
  }

  if (softwareOperationSteps.value.length === 0) {
    softwareOperationSteps.value = baseSoftwareOperationSteps(targetLabel)
  }
  if (softwareOperationContextFacts.value.length === 0) {
    softwareOperationContextFacts.value = buildSoftwareOperationContextFacts(targetLabel)
  }

  if (event.status === 'complete') {
    setSoftwareOperationStep('software-download', StepStatus.COMPLETED)
    setSoftwareOperationStep('software-apply', StepStatus.COMPLETED, 'Variant changed')
    softwareOperationComplete.value = true
    softwareOperationOpen.value = true
    return
  }

  setSoftwareOperationStep('software-download', StepStatus.COMPLETED)
  setSoftwareOperationStep(
    'software-apply',
    StepStatus.FAILED,
    event.error || 'Variant change failed',
  )
  softwareOperationComplete.value = true
  softwareOperationOpen.value = true
}

async function handleSoftwareChanged() {
  await getGameServerDetails()
  if (hasConsoleOutput.value) {
    return
  }
  await getGameServerOutput()
}

function appendOutputLines(target: Ref<string[]>, output: string) {
  target.value = appendOperationOutputLines(target.value, output, maxOperationOutputLines)
}

function captureOperationOutput(output: string): boolean {
  if (normalizeOperationOutputChunk(output).length === 0) {
    return false
  }

  const route = resolveOperationOutputRoute({
    isServerOffline: isServerOffline.value,
    updateRequested: updatingServer.value,
    updateInProgress: updateInProgress.value,
    softwareOperationInProgress: softwareOperationInProgress.value,
  })

  if (route === 'software') {
    appendOutputLines(softwareOperationOutputLines, output)
    softwareOperationOpen.value = true
    return true
  }

  if (route === 'update') {
    appendConsoleOutput(output)
    return true
  }

  return false
}

function onUpdateProgress(progress: UpdateProgress) {
  if (progress.gameServerId !== gameServerId.value) return

  updateSteps.value = applyUpdateProgress(updateSteps.value, progress)

  if (isUpdateProgressTerminal(progress, updateSteps.value)) {
    updateInProgress.value = false
    if (
      progress.step === UpdateStep.RESTARTING ||
      (progress.step === UpdateStep.INSTALLING && progress.stepStatus === StepStatus.COMPLETED)
    ) {
      void getGameServerDetails()
    }
  }
}

async function updateGameServer() {
  if (!serverStateAuthoritative.value) {
    return
  }

  if (gameServer.value.status === Status.ONLINE) {
    const confirmed = await new Promise<boolean>((resolve) => {
      let settled = false
      $q.dialog({
        title: 'Update running server?',
        message: 'Xylona will stop the server, install the update, and start it again.',
        cancel: true,
        persistent: true,
        ok: {
          label: 'Update server',
          color: 'primary',
          unelevated: true,
        },
      })
        .onOk(() => {
          settled = true
          resolve(true)
        })
        .onDismiss(() => {
          if (!settled) {
            resolve(false)
          }
        })
    })
    if (!confirmed) {
      return
    }
  }

  const steamBranchSelection = await chooseSteamBranchForUpdate({
    gameServerId: gameServerId.value,
    gameServer: gameServer.value,
    getBranches: async (serverId: string) => {
      const request = create(GetUpdateTargetsRequestSchema, { gameServerId: serverId })
      return GetXylonaClient().getUpdateTargets(request)
    },
    openDialog: ({ currentBranch, items, onOk, onDismiss }) => {
      let settled = false
      $q.dialog({
        title: 'Choose Update Target',
        message: 'Select which release target this server should update to.',
        cancel: true,
        persistent: true,
        ok: {
          label: 'Update server',
          color: 'primary',
          unelevated: true,
        },
        options: {
          type: 'radio',
          model: currentBranch,
          items,
        },
      })
        .onOk((value) => {
          settled = true
          onOk(typeof value === 'string' && value.trim() !== '' ? value : currentBranch)
        })
        .onDismiss(() => {
          if (!settled) {
            onDismiss()
          }
        })
    },
  })
  if (steamBranchSelection.cancelled) {
    return
  }
  if (canSelectSteamBranch(gameServer.value) && !steamBranchSelection.metadataAvailable) {
    $q.notify({
      type: 'xylona-info',
      position: 'top-right',
      caption: 'Update target metadata is unavailable. Updating with the current target.',
      icon: 'report_problem',
    })
  }

  const request: UpdateGameServerRequest = create(UpdateGameServerRequestSchema, {})
  updatingServer.value = true
  resetUpdateSteps()
  updateInProgress.value = true
  try {
    request.serverId = gameServerId.value
    request.target = steamBranchSelection.steamBranch
    await GetXylonaClient().updateGameServer(request)
    recordLifecycleIntent(gameServerId.value, 'update')
  } catch (e) {
    updateInProgress.value = false
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to update game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    updatingServer.value = false
  }
}

async function getGameServerOutput(fallbackOnly = false) {
  const request: ReadGameServerOutputRequest = create(ReadGameServerOutputRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    const response: ReadGameServerOutputResponse =
      await GetXylonaClient().readGameServerOutput(request)
    if (fallbackOnly && receivedConsoleReset.value) {
      return
    }
    if (captureOperationOutput(response.output)) {
      return
    }
    appendConsoleOutput(response.output)
    consoleLoadError.value = ''
  } catch (e) {
    console.error(e)
    consoleLoadError.value =
      'Earlier console output could not be loaded. Live output may still appear.'
    if (consoleStreamState.value !== 'ready') {
      consoleStreamState.value = 'error'
    }
  }
}

async function onWebsocketReconnect() {
  serverStatusFresh.value = false
  receivedConsoleReset.value = false
  lastConsoleSequence.value = 0n
  consoleStreamState.value = 'reconnecting'
  const detailsLoaded = await getGameServerDetails()
  if (!detailsLoaded) {
    console.error('Failed to refresh game server status after websocket reconnect')
  }
  try {
    await requestConsoleOutputStream()
  } catch (error) {
    console.error('Failed to resubscribe to game server console output', error)
    consoleLoadError.value = 'Console reconnection failed.'
    consoleStreamState.value = 'error'
  }
}

function onWebsocketDisconnect() {
  serverStatusFresh.value = false
  consoleStreamState.value = 'reconnecting'
}

function onServerStatus(serverID: string, _serverName: string, status: Status) {
  if (serverID !== gameServerId.value) {
    return
  }

  liveStatusSequence++
  if (status === Status.OFFLINE) {
    lastConsoleSequence.value = 0n
    receivedConsoleReset.value = false
  }
  gameServer.value = create(GameServerSchema, {
    ...gameServer.value,
    status,
  })
  serverStatusFresh.value = websocketStateAuthoritative.value
}

function onServerConsoleOutput(
  serverID: string,
  output: string,
  sequence: bigint = 0n,
  resetBuffer: boolean = false,
  reconnecting: boolean | undefined = undefined,
) {
  if (serverID !== gameServerId.value) {
    return
  }

  if (reconnecting !== undefined) {
    consoleLoadError.value = ''
    consoleStreamState.value = reconnecting ? 'reconnecting' : 'ready'
    if (reconnecting && output !== '') {
      appendConsoleOutput(output)
    }
    return
  }

  const decision = resolveConsoleStreamChunk(lastConsoleSequence.value, {
    sequence,
    reset: resetBuffer,
  })
  if (decision.action === 'ignore') {
    return
  }
  lastConsoleSequence.value = decision.nextSequence

  if (decision.action === 'replace') {
    receivedConsoleReset.value = true
    consoleLoadError.value = ''
    consoleStreamState.value = 'ready'
    replaceConsoleOutput(output)
    return
  }

  if (captureOperationOutput(output)) {
    consoleStreamState.value = 'ready'
    return
  }
  appendConsoleOutput(output)
  consoleStreamState.value = 'ready'
}

function streamGameServerOutput() {
  // Stream game server output.
  XylonaEventBus.on('gameServerConsoleOutput', onServerConsoleOutput)
  XylonaEventBus.on('gameServerStatus', onServerStatus)

  // Listen for update progress events before any initial websocket request so
  // an early send failure cannot skip the listener registration.
  XylonaEventBus.on('gameServerUpdateProgress', onUpdateProgress)

  // Handle socket reconnection.
  XylonaEventBus.on('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.on('websocketDisconnected', onWebsocketDisconnect)
}

async function retryConsoleOutput() {
  receivedConsoleReset.value = false
  consoleStreamState.value = 'loading'
  consoleLoadError.value = ''
  try {
    await requestConsoleOutputStream()
  } catch (error) {
    console.error('Failed to subscribe to game server console output', error)
    consoleLoadError.value = 'Could not connect to live console output.'
    consoleStreamState.value = 'error'
  }
  await getGameServerOutput(true)
}

async function sendGameServerInput() {
  if (consoleInputDisabled.value || serverInput.value === '') {
    return
  }

  const request: SendGameServerInputRequest = create(SendGameServerInputRequestSchema, {})
  sendingConsoleInput.value = true
  try {
    request.serverId = gameServerId.value
    request.input = serverInput.value
    await GetXylonaClient().sendGameServerInput(request)
    recordConsoleInput()
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to send command: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    sendingConsoleInput.value = false
  }
}
</script>

<style scoped>
/* ===== Identity Bar ===== */
.identity-bar {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
  flex-shrink: 0;
}

.identity-bar-left {
  display: flex;
  flex-direction: column;
  gap: 0.15rem;
  min-width: 0;
}

.identity-bar-name {
  font-family: var(--xy-font-display);
  font-size: 1.15rem;
  font-weight: 700;
  color: var(--xy-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.identity-bar-detail {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.78rem;
  color: var(--xy-text-secondary);
  flex-wrap: wrap;
}

.identity-bar-sep {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

.identity-bar-running {
  font-style: italic;
  color: var(--xy-text-muted);
}

.identity-bar-version {
  font-family: var(--xy-font-mono);
  font-size: 0.72rem;
  color: var(--xy-text-muted);
}

.identity-bar-version.version-outdated {
  color: var(--xy-accent);
  cursor: help;
}

.identity-bar-spacer {
  flex: 1;
}

/* ===== Main Area ===== */
.main-area {
  flex: 1;
  display: flex;
  min-height: 0;
  border: 1px solid var(--xy-border);
  border-top: none;
  border-radius: 0 0 6px 6px;
  overflow: hidden;
}

/* ===== Sidebar ===== */
.sidebar {
  width: 290px;
  display: flex;
  flex-direction: column;
  background: var(--xy-surface-0);
  border-right: 1px solid var(--xy-border);
  flex-shrink: 0;
  transition:
    width 0.25s cubic-bezier(0.25, 1, 0.5, 1),
    opacity 0.2s ease;
  overflow: hidden;
}

.sidebar.collapsed {
  width: 0;
  border-right: none;
  opacity: 0;
}

.sidebar-content {
  padding: var(--xy-space-md);
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-lg);
  overflow-y: auto;
  flex: 1;
  min-width: 290px;
}

.sidebar-section-label {
  font-size: 0.62rem;
  font-weight: 600;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  margin-bottom: var(--xy-space-xs);
}

/* ===== Controls ===== */
.server-controls {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
}

.readiness-list {
  display: grid;
  gap: var(--xy-space-sm);
}

.readiness-item {
  display: grid;
  grid-template-columns: 28px minmax(0, 1fr);
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm);
  border: 1px solid color-mix(in srgb, var(--xy-warning) 35%, var(--xy-border));
  border-radius: 6px;
  background: color-mix(in srgb, var(--xy-warning) 9%, var(--xy-surface-1));
}

.readiness-item--complete {
  border-color: color-mix(in srgb, var(--xy-success) 35%, var(--xy-border));
  background: color-mix(in srgb, var(--xy-success) 8%, var(--xy-surface-1));
}

.readiness-item-icon {
  color: var(--xy-warning);
  display: flex;
  align-items: flex-start;
  justify-content: center;
  padding-top: 2px;
}

.readiness-item--complete .readiness-item-icon {
  color: var(--xy-success);
}

.readiness-item-body {
  min-width: 0;
}

.readiness-item-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: 0.78rem;
}

.readiness-item-message {
  color: var(--xy-text-muted);
  font-size: 0.75rem;
  line-height: 1.35;
  margin-top: 2px;
}

.readiness-action {
  margin-top: var(--xy-space-sm);
}

.readiness-secret-form {
  display: grid;
  gap: var(--xy-space-xs);
  margin-top: var(--xy-space-sm);
}

.readiness-secret-input {
  min-width: 0;
}

.readiness-secret-actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
}

.readiness-device-flow {
  display: grid;
  gap: var(--xy-space-xs);
}

.readiness-device-code {
  display: inline-flex;
  justify-content: center;
  width: fit-content;
  max-width: 100%;
  padding: 0.2rem 0.45rem;
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  background: var(--xy-surface-2);
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-size: 0.82rem;
  overflow-wrap: anywhere;
}

.readiness-link {
  color: var(--xy-accent);
  font-size: 0.75rem;
  text-decoration: none;
}

.readiness-link:hover {
  text-decoration: underline;
}

.readiness-profile-select {
  min-width: 0;
}

/* ===== Software Card ===== */
.software-card {
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: 6px;
  overflow: hidden;
}

.software-card-body {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
}

.software-icon {
  width: 32px;
  height: 32px;
  border-radius: 4px;
  background: var(--xy-surface-3);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.1rem;
  flex-shrink: 0;
  color: var(--xy-accent);
}

.software-info {
  flex: 1;
  min-width: 0;
}

.software-name-row {
  display: flex;
  align-items: baseline;
  gap: 0.4rem;
}

.software-name {
  font-family: var(--xy-font-display);
  font-size: 0.88rem;
  font-weight: 700;
  color: var(--xy-text-primary);
  line-height: 1.2;
}

.software-version {
  font-family: var(--xy-font-mono);
  font-size: 0.72rem;
  color: var(--xy-text-muted);
}

.software-game {
  font-size: 0.7rem;
  color: var(--xy-text-muted);
  margin-top: 1px;
}

.software-track-state {
  margin-top: 0.2rem;
  font-size: 0.68rem;
  color: var(--xy-info);
  letter-spacing: 0.01em;
}

.software-none {
  font-size: 0.78rem;
  color: var(--xy-text-muted);
  font-style: italic;
  padding: var(--xy-space-sm) var(--xy-space-md);
}

/* Update hint */
.update-hint {
  display: flex;
  align-items: center;
  gap: 0.3rem;
  padding: 0 var(--xy-space-md) var(--xy-space-sm);
  font-size: 0.68rem;
  color: var(--xy-warning);
  cursor: help;
}

.update-dot {
  width: 5px;
  height: 5px;
  border-radius: 50%;
  background: var(--xy-warning);
  flex-shrink: 0;
  animation: update-pulse 2.5s ease-in-out infinite;
}

@keyframes update-pulse {
  0%,
  100% {
    opacity: 1;
    box-shadow: 0 0 0 transparent;
  }
  50% {
    opacity: 0.6;
    box-shadow: 0 0 6px var(--xy-warning-border);
  }
}

/* Change button */
.software-card-footer {
  padding: var(--xy-space-xs) var(--xy-space-md) var(--xy-space-sm);
}

.change-btn {
  display: inline-flex;
  align-items: center;
  gap: 0.3rem;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: 4px;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-body);
  font-size: 0.72rem;
  font-weight: 500;
  cursor: pointer;
  padding: 0.25rem 0.6rem;
  transition: all var(--xy-transition-fast);
}

.change-btn:hover {
  border-color: var(--xy-primary);
  color: var(--xy-primary);
  background: var(--xy-primary-muted);
}

.change-arrow {
  font-size: 0.72rem;
  transition: transform var(--xy-transition-fast);
}

.change-btn:hover .change-arrow {
  transform: translateX(2px);
}

/* ===== Connection ===== */
.connection-list {
  display: flex;
  flex-direction: column;
  gap: 1px;
  background: var(--xy-border);
  border-radius: 5px;
  overflow: hidden;
}

.connection-item {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-surface-1);
}

.cl-label {
  font-size: 0.75rem;
  color: var(--xy-text-muted);
}

.cl-value {
  font-family: var(--xy-font-mono);
  font-size: 0.78rem;
  color: var(--xy-text-secondary);
}

.cl-value :deep(.copy-clipboard) {
  cursor: pointer;
  transition: color var(--xy-transition-fast);
}

.cl-value :deep(.copy-clipboard:hover) {
  color: var(--xy-accent);
}

.cl-value-plain {
  font-family: var(--xy-font-mono);
  font-size: 0.78rem;
  color: var(--xy-text-secondary);
}

/* ===== Metrics Preview ===== */
.metrics-preview {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-sm);
}

.metrics-group {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  padding-bottom: var(--xy-space-sm);
  border-bottom: 1px solid var(--xy-border);
}

.metrics-group:last-child {
  border-bottom: none;
  padding-bottom: 0;
}

.metrics-group-label {
  font-size: 0.62rem;
  font-weight: 600;
  letter-spacing: 0.12em;
  text-transform: uppercase;
  color: var(--xy-text-muted);
  margin-bottom: 0.1rem;
}

.metrics-offline {
  opacity: 0.35;
}

.metrics-offline .metric-bar-fill {
  transform: scaleX(0) !important;
}

.metric-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 0.75rem;
}

.metric-row .ml {
  color: var(--xy-text-muted);
  font-weight: 500;
}

.metric-detail {
  font-size: 0.65rem;
  color: var(--xy-text-muted);
  opacity: 0.7;
}

.metric-row .mv {
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-mono);
  font-size: 0.75rem;
}

.metric-bar {
  width: 100%;
  height: 3px;
  background: var(--xy-surface-3);
  border-radius: 2px;
  margin-top: 3px;
  overflow: hidden;
}

.metric-bar-fill {
  width: 100%;
  height: 100%;
  border-radius: 2px;
  transform-origin: left center;
  transition: transform 0.8s var(--xy-ease-standard);
  will-change: transform;
}

.fill-low {
  background: var(--xy-success);
  box-shadow: 0 0 4px var(--xy-success-border);
}

.fill-mid {
  background: var(--xy-warning);
  box-shadow: 0 0 4px var(--xy-warning-border);
}

.fill-high {
  background: var(--xy-danger);
  box-shadow: 0 0 4px var(--xy-danger-border);
}

/* ===== Console Wrapper ===== */
.console-wrapper {
  flex: 1;
  display: flex;
  flex-direction: column;
  min-width: 0;
  position: relative;
}

/* Console toolbar buttons */
.console-toolbar-btns {
  position: absolute;
  top: var(--xy-space-xs);
  right: var(--xy-space-sm);
  z-index: 10;
  display: flex;
  gap: 2px;
}

.console-toolbar-btn {
  opacity: 0.6;
  transition: opacity var(--xy-transition-fast);
}

.console-toolbar-btn:hover {
  opacity: 1;
}

.console-toolbar-btn-off {
  opacity: 0.3;
  color: var(--xy-text-muted);
}

.console-stream-state {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 2.5rem;
  padding: var(--xy-space-xs) 10rem var(--xy-space-xs) var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-warning-bg-faint);
  border-bottom: 1px solid var(--xy-warning-border);
  font-family: var(--xy-font-body);
}

.console-stream-state--error {
  background: var(--xy-danger-bg-faint);
  border-color: var(--xy-danger-border);
}

.console-stream-state .q-btn {
  margin-inline-start: auto;
}

/* Console output scroll area */
.console-scroll-area {
  flex: 1;
  min-height: 0;
  padding-left: var(--xy-space-md);
  padding-right: var(--xy-space-sm);
  font-family: var(--xy-font-mono);
  font-size: 0.85rem;
  font-weight: 400;
  font-style: normal;
  overflow-x: hidden;
  white-space: pre-wrap;
  max-width: 100%;
  background-color: var(--xy-base);
  position: relative;
}

/* Top fade gradient */
.console-scroll-area::before {
  content: '';
  position: sticky;
  top: 0;
  display: block;
  height: 16px;
  margin-bottom: -16px;
  background: linear-gradient(to bottom, var(--xy-base) 0%, transparent 100%);
  pointer-events: none;
  z-index: 1;
}

.console-truncated-notice {
  font-family: var(--xy-font-mono);
  font-size: 0.75rem;
  color: var(--xy-text-muted);
  text-align: center;
  padding: var(--xy-space-xs) 0;
  border-bottom: 1px solid var(--xy-border);
}

.console-status-banner {
  font-family: var(--xy-font-mono);
  font-size: 0.75rem;
  color: var(--xy-accent);
  padding: var(--xy-space-xs) 0;
  border-bottom: 1px solid var(--xy-border);
  margin-bottom: var(--xy-space-xs);
}

.console-scroll-area :deep(#consoleCodeEl) {
  white-space: pre-wrap;
}

.console-scroll-area :deep(.q-scrollarea__content) {
  padding-top: var(--xy-space-md);
}

/* Offline console output */
.console-output-offline {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--xy-base);
  min-height: 0;
}

.offline-placeholder {
  text-align: center;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
}

.offline-icon {
  width: 56px;
  height: 56px;
  margin: 0 auto var(--xy-space-md);
  border-radius: 50%;
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 1.3rem;
  opacity: 0.5;
  position: relative;
}

.offline-icon::after {
  content: '';
  position: absolute;
  inset: -4px;
  border-radius: 50%;
  border: 1px solid var(--xy-border);
  animation: offline-ring 4s ease-in-out infinite;
}

@keyframes offline-ring {
  0%,
  100% {
    opacity: 0;
    transform: scale(0.95);
  }
  50% {
    opacity: 0.4;
    transform: scale(1);
  }
}

.offline-text {
  font-size: 0.88rem;
  font-weight: 500;
  margin-bottom: 0.25rem;
}

.offline-hint {
  font-size: 0.75rem;
  opacity: 0.6;
}

/* ===== Console Input ===== */
.console-input-wrapper {
  display: flex;
  align-items: center;
  background: var(--xy-surface-1);
  border-top: 1px solid var(--xy-border);
  flex-shrink: 0;
}

.console-input-wrapper:focus-within {
  border-top-color: var(--xy-accent-border);
  background: color-mix(in srgb, var(--xy-surface-1) 95%, var(--xy-accent) 5%);
}

.console-input-disabled {
  opacity: 0.3;
  pointer-events: none;
}

.console-prompt {
  padding: 0 0.35rem 0 var(--xy-space-md);
  font-family: var(--xy-font-mono);
  font-size: 0.85rem;
  color: var(--xy-accent);
  user-select: none;
  opacity: 0.8;
}

.console-input-wrapper:focus-within .console-prompt {
  opacity: 1;
}

.console-input-field {
  flex: 1;
}

.console-input-field :deep(.q-field__control) {
  font-family: var(--xy-font-mono);
}

/* ===== Fullscreen Console ===== */
.console-wrapper.expanded {
  position: fixed;
  inset: 0;
  z-index: var(--xy-z-fullscreen);
  width: 100%;
  height: 100dvh;
  background-color: var(--xy-base);
  animation: console-expand 200ms cubic-bezier(0.25, 1, 0.5, 1) both;
}

.console-wrapper.expanded .console-scroll-area {
  padding-left: var(--xy-space-sm);
}

@keyframes console-expand {
  from {
    opacity: 0;
    transform: scale(0.97);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}

/* Expanded state also hides sidebar when fullscreen is active */
.main-area-expanded .sidebar {
  display: none;
}

/* ===== Focus rings ===== */
.change-btn:focus-visible,
.console-toolbar-btn:focus-visible {
  outline: 2px solid var(--xy-primary);
  outline-offset: 2px;
}

/* ===== Mobile ===== */
@media (max-width: 1023px) {
  .identity-bar {
    flex-wrap: wrap;
  }

  .identity-bar-name {
    font-size: 1rem;
  }
}
</style>
