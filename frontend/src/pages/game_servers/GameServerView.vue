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
            </q-btn>
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
                    :style="{ width: isServerOnline ? metricsCpu + '%' : '0%' }"
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
                      width: isServerOnline ? metricsMemoryRatio * 100 + '%' : '0%',
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

      <!-- Console output -->
      <template v-if="showConsolePlaceholder">
        <div class="console-output console-output-offline">
          <div class="offline-placeholder">
            <div class="offline-icon">
              <q-icon name="power_settings_new" />
            </div>
            <div class="offline-text">Server is offline</div>
            <div class="offline-hint">Press Start to launch the server</div>
          </div>
        </div>
      </template>
      <template v-else>
        <q-scroll-area id="consoleContainer" ref="consoleScrollArea" class="console-scroll-area">
          <div
            v-if="isServerOffline && !updateInProgress && !softwareOperationInProgress"
            class="console-status-banner">
            Server offline.
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
      <div :class="{ 'console-input-disabled': isServerOffline }" class="console-input-wrapper">
        <span class="console-prompt">&gt;</span>
        <q-input
          id="consoleInput"
          v-model="serverInput"
          :disable="!hasPermission('game_server.console') || isServerOffline"
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
              :disable="!hasPermission('game_server.console') || isServerOffline"
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
    v-model="updateDialogOpen"
    :complete="updateDialogComplete"
    :output-lines="updateOutputLines"
    :show-output-area="true"
    :steps="updateSteps"
    subtitle="Xylona will apply the update and only restart the server if it was already running."
    title="Updating Server" />

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
import type { UpdateProgress } from '@/proto/xylona_pb'
import {
  GetGameServerRequest,
  GetGameServerRequestSchema,
  GetUpdateTargetsRequestSchema,
  StepStatus,
  UpdateGameServerRequest,
  UpdateGameServerRequestSchema,
  UpdateStep,
} from '@/proto/xylona_pb'
import { ConnectError } from '@connectrpc/connect'
import { parseConsole } from '@/utils/console'
import { canShowUpdateButton } from './game-server-update-capability'
import { type ConsoleLine, splitConsoleChunk, trimConsoleLines } from './console-buffer'
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
import { computed, nextTick, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { useRoute } from 'vue-router'
import { resolveCanonicalVersionDisplay, resolveVariantTrackingLabel } from './version-display'
import { canSelectSteamBranch, chooseSteamBranchForUpdate } from './steam-branch-update'
import { useGameServerMetricsPreview } from './useGameServerMetricsPreview'
import { useGameServerQueryStatusVersion } from './useGameServerQueryStatusVersion'

const $q = useQuasar()
const consoleLines = ref<ConsoleLine[]>([])
let consoleLineIdCounter = 0
let pendingConsoleChunks: string[] = []
let consoleRafId: number | null = null
const route = useRoute()
const gameServer: Ref<GameServer> = ref(create(GameServerSchema)) as Ref<GameServer>
const serverInput = ref('')
const gameServerId: Ref<string> = ref(
  route.params.id instanceof Array ? route.params.id[0] : route.params.id,
)
const consoleScrollArea = ref<QScrollArea | null>(null)
const softwareSelector = ref<InstanceType<typeof ServerSoftwareSelector> | null>(null)
const consoleHistory = ref<string[]>([])
const consoleHistoryCurrentIndex = ref(0)
const consoleExpanded = ref(false)
const maxConsoleCharacters = 100000
const consoleTruncated = ref(false)
const sidebarCollapsed = ref(false)
const consoleAutoScroll = ref(localStorage.getItem('xylona_console_autoscroll') !== 'false')

function scrollConsoleToBottom() {
  const el = consoleScrollArea.value?.$el as HTMLElement | undefined
  const container = el?.querySelector('.q-scrollarea__container') as HTMLElement | null
  if (container) {
    container.scrollTop = container.scrollHeight
  }
}

function appendConsoleOutput(rawOutput: string) {
  const parsed = parseConsole(gameServer.value.gameId, rawOutput)
  if (parsed.length === 0) return
  pendingConsoleChunks.push(parsed)
  if (consoleRafId === null) {
    consoleRafId = requestAnimationFrame(flushConsolePending)
  }
}

function flushConsolePending() {
  consoleRafId = null
  if (pendingConsoleChunks.length === 0) return

  const newLines = pendingConsoleChunks.flatMap((html) =>
    splitConsoleChunk(html).map((lineHtml) => ({
      id: consoleLineIdCounter++,
      html: lineHtml,
    })),
  )
  pendingConsoleChunks = []

  if (newLines.length === 0) {
    return
  }

  consoleLines.value.push(...newLines)

  const trimmedConsole = trimConsoleLines(consoleLines.value, maxConsoleCharacters)
  consoleLines.value = trimmedConsole.lines
  if (trimmedConsole.truncated) {
    consoleTruncated.value = true
  }

  if (consoleAutoScroll.value) {
    void nextTick(scrollConsoleToBottom)
  }
}

const startingServer = ref(false)
const stoppingServer = ref(false)
const updatingServer = ref(false)
const updateInProgress = ref(false)
const updateDialogOpen = ref(false)
const updateDialogComplete = ref(false)
const updateSteps = ref<StepState[]>([])
const updateOutputLines = ref<string[]>([])
const softwareOperationOpen = ref(false)
const softwareOperationComplete = ref(false)
const softwareOperationSteps = ref<StepState[]>([])
const softwareOperationOutputLines = ref<string[]>([])
const softwareOperationContextFacts = ref<OperationContextFact[]>([])
const maxOperationOutputLines = 80

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
const isServerOffline = computed(
  () => gameServer.value.status === Status.OFFLINE || gameServer.value.status === Status.UNKNOWN,
)
const hasConsoleOutput = computed(() => consoleLines.value.length > 0)
const softwareOperationInProgress = computed(() =>
  softwareOperationSteps.value.some((step) => step.status === StepStatus.IN_PROGRESS),
)
const showConsolePlaceholder = computed(
  () =>
    isServerOffline.value &&
    !hasConsoleOutput.value &&
    !updateInProgress.value &&
    !softwareOperationInProgress.value,
)

const connectionAddress = computed(() => {
  const ip = gameServer.value.ip?.address ?? ''
  const port = gameServer.value.port.toString()
  if (!ip) return port
  return `${ip}:${port}`
})

function toggleAutoScroll() {
  consoleAutoScroll.value = !consoleAutoScroll.value
  localStorage.setItem('xylona_console_autoscroll', String(consoleAutoScroll.value))
  if (consoleAutoScroll.value) {
    void nextTick(scrollConsoleToBottom)
  }
}

function onEscapeKey(e: KeyboardEvent) {
  if (e.key === 'Escape' && consoleExpanded.value) {
    consoleExpanded.value = false
  }
}

const disableStartButton = computed(() => {
  return gameServer.value.status !== Status.OFFLINE && gameServer.value.status !== Status.UNKNOWN
})

const disableStopButton = computed(() => {
  return gameServer.value.status !== Status.ONLINE
})

const disableUpdateButton = computed(() => {
  return (
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

  void getGameServerDetails()
    .then(() => {
      void getGameServerOutput()
      streamGameServerOutput()
      startQueryStatusVersionLifecycle()
      startMetricsPreviewLifecycle()
    })
    .then(queryGameServer)
})

onBeforeUnmount(() => {
  if (consoleRafId !== null) {
    cancelAnimationFrame(consoleRafId)
    consoleRafId = null
  }
  document.removeEventListener('keydown', onEscapeKey)
  unsubscribeConsoleOutputStream()

  XylonaEventBus.off('gameServerUpdateProgress', onUpdateProgress)
  XylonaEventBus.off('websocketConnected', onWebsocketReconnect)
  XylonaEventBus.off('gameServerConsoleOutput', onServerConsoleOutput)
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
  const request: GetGameServerRequest = create(GetGameServerRequestSchema, {})
  try {
    request.id = gameServerId.value
    const response = await GetXylonaClient().getGameServer(request)
    if (response.gameServer === undefined) {
      return
    }
    gameServer.value = response.gameServer
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to load game server details: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function startGameServer() {
  const request: StartGameServerRequest = create(StartGameServerRequestSchema, {})
  startingServer.value = true
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().startGameServer(request)
  } catch (e) {
    console.error(e)
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
  updateDialogComplete.value = false
  updateSteps.value = buildUpdateSteps(
    gameServer.value.status,
    buildUpdateStepLabels({
      usesSteamcmd: Boolean(gameServer.value.game?.usesSteamcmd),
    }),
  )
  updateOutputLines.value = []
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

function appendOutputLines(target: typeof updateOutputLines, output: string) {
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
    appendOutputLines(updateOutputLines, output)
    updateDialogOpen.value = true
    return true
  }

  if (route === 'discard') {
    return true
  }

  return false
}

function onUpdateProgress(progress: UpdateProgress) {
  if (progress.gameServerId !== gameServerId.value) return

  updateSteps.value = applyUpdateProgress(updateSteps.value, progress)

  if (isUpdateProgressTerminal(progress, updateSteps.value)) {
    updateInProgress.value = false
    updateDialogComplete.value = true
    updateDialogOpen.value = true
    if (
      progress.step === UpdateStep.RESTARTING ||
      (progress.step === UpdateStep.INSTALLING && progress.stepStatus === StepStatus.COMPLETED)
    ) {
      void getGameServerDetails()
    }
  }
}

async function updateGameServer() {
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
  updateDialogOpen.value = true
  try {
    request.serverId = gameServerId.value
    request.target = steamBranchSelection.steamBranch
    await GetXylonaClient().updateGameServer(request)
    recordLifecycleIntent(gameServerId.value, 'update')
  } catch (e) {
    updateInProgress.value = false
    updateDialogOpen.value = false
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

async function getGameServerOutput() {
  const request: ReadGameServerOutputRequest = create(ReadGameServerOutputRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    const response: ReadGameServerOutputResponse =
      await GetXylonaClient().readGameServerOutput(request)
    if (captureOperationOutput(response.output)) {
      return
    }
    appendConsoleOutput(response.output)
  } catch (e) {
    console.error(e)
  }
}

function onWebsocketReconnect() {
  void requestConsoleOutputStream().catch((error) => {
    console.error('Failed to resubscribe to game server console output', error)
  })
}

function onServerConsoleOutput(serverID: string, output: string) {
  if (serverID !== gameServerId.value) {
    return
  }
  if (captureOperationOutput(output)) {
    return
  }
  appendConsoleOutput(output)
}

function streamGameServerOutput() {
  // Stream game server output.
  XylonaEventBus.on('gameServerConsoleOutput', onServerConsoleOutput)

  // Listen for update progress events before any initial websocket request so
  // an early send failure cannot skip the listener registration.
  XylonaEventBus.on('gameServerUpdateProgress', onUpdateProgress)

  // Handle socket reconnection.
  XylonaEventBus.on('websocketConnected', onWebsocketReconnect)

  // Request the game server to start streaming output.
  void requestConsoleOutputStream().catch((error) => {
    console.error('Failed to subscribe to game server console output', error)
  })
}

async function navigateConsoleInputHistory(direction: string) {
  let historyDirection: number
  switch (direction.toLowerCase()) {
    case 'up':
      historyDirection = -1
      break
    case 'down':
      historyDirection = 1
      break
    default:
      return
  }
  if (consoleHistory.value.length === 0) {
    return
  }

  if (consoleHistoryCurrentIndex.value > consoleHistory.value.length) {
    return
  }

  const newIndex = consoleHistoryCurrentIndex.value + historyDirection
  if (newIndex < 0) {
    return
  }
  if (newIndex > consoleHistory.value.length) {
    return
  }
  consoleHistoryCurrentIndex.value = newIndex
  if (newIndex === consoleHistory.value.length) {
    serverInput.value = ''
  } else {
    serverInput.value = consoleHistory.value[newIndex]
  }
}

async function sendGameServerInput() {
  const request: SendGameServerInputRequest = create(SendGameServerInputRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    request.input = serverInput.value
    await GetXylonaClient().sendGameServerInput(request)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top-right',
      caption: 'Failed to send command: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
  consoleHistory.value.push(serverInput.value)
  consoleHistoryCurrentIndex.value = consoleHistory.value.length
  serverInput.value = ''
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
  width: 0 !important;
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
  height: 100%;
  border-radius: 2px;
  transition: width 0.8s cubic-bezier(0.25, 1, 0.5, 1);
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
  z-index: 9999;
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
