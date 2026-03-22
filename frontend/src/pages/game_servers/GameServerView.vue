<template>
  <div class="identity-header">
    <div class="identity-name">{{ gameServer.name }}</div>
    <div class="identity-subtitle">
      <span>{{ gameServer.gameName }}</span>
      <template v-if="hasSoftwareOptions && !softwareNameRedundant">
        <span class="identity-separator">·</span>
        <span class="identity-running">running</span>
        <span>{{ softwareDisplayName }}</span>
      </template>
      <template v-if="gameServer.version && hasSoftwareOptions">
        <span class="identity-version" :class="{ 'version-outdated': softwareHasUpdate }">
          {{ gameServer.version }}
          <q-tooltip v-if="softwareHasUpdate">
            Update available: {{ softwareLatestVersion }}
          </q-tooltip>
        </span>
      </template>
      <q-btn
        v-if="showChangeButton"
        flat
        dense
        no-caps
        size="sm"
        label="Change"
        class="identity-change-btn"
        @click="softwareSelector?.openChangeDialog()" />
    </div>
  </div>
  <div class="row q-gutter-lg-lg q-col-gutter-lg q-px-md">
    <div class="col-lg-4 col-xs-12 q-gutter-md info-panel">
      <q-list separator>
        <q-item>
          <q-item-section>Status</q-item-section>
          <q-item-section side>
            <status-badge :status="gameServer.status" />
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>IP</q-item-section>
          <q-item-section side>
            <clip-board-copy
              :clip-board-value="gameServer.ip !== undefined ? gameServer.ip.address : ''"
              :display-text="gameServer.ip?.address"></clip-board-copy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Port</q-item-section>
          <q-item-section side>
            <clip-board-copy
              :clip-board-value="gameServer.port.toString()"
              :display-text="gameServer.port.toString()"></clip-board-copy>
          </q-item-section>
        </q-item>
        <q-item>
          <q-item-section>Current player count</q-item-section>
          <q-item-section side> {{ currentPlayerCount }} / {{ maxPlayerCount }} </q-item-section>
        </q-item>
      </q-list>
      <server-software-selector
        v-if="gameServer.gameId !== ''"
        ref="softwareSelector"
        :game-server-id="gameServerId"
        :game-id="gameServer.gameId"
        :game-name="gameServer.gameName"
        :current-software="gameServer.serverSoftware"
        :current-version="gameServer.version"
        @software-changed="getGameServerDetails" />
      <game-server-metrics :game-server-id="gameServerId" :game-server="gameServer" />
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
          :disable="disableUpdateButton || !hasPermission('game_server.settings')"
          :loading="updatingServer"
          color="primary"
          label="Update"
          @click="updateGameServer">
          <q-tooltip v-if="!hasPermission('game_server.settings')">
            Requires settings permission
          </q-tooltip>
        </q-btn>
      </div>
    </div>
    <div class="col col-lg-8 col-xs-12 console-panel" :class="{ expanded: consoleExpanded }">
      <q-btn
        flat
        square
        dense
        padding="xs"
        class="console-expand-btn"
        :icon="tabMaximize"
        aria-label="Toggle fullscreen console"
        text-color="info"
        @click="consoleExpanded = !consoleExpanded" />
      <q-scroll-area id="consoleContainer" ref="consoleScrollArea">
        <div v-if="consoleTruncated" class="console-truncated-notice">Earlier output truncated</div>
        <!-- eslint-disable vue/no-v-html -- accepted per CLAUDE.md: game server console output -->
        <code
          id="consoleCodeEl"
          role="log"
          aria-live="polite"
          aria-label="Game server console output"
          class="q-pb-md"
          v-html="gameServerOutput"></code>
        <!-- eslint-enable vue/no-v-html -->
      </q-scroll-area>
      <q-input
        id="consoleInput"
        v-model="serverInput"
        autofocus
        placeholder="Enter command..."
        dense
        square
        outlined
        name="consoleInput"
        :disable="!hasPermission('game_server.console')"
        @keyup.enter="sendGameServerInput"
        @keyup.up="navigateConsoleInputHistory('up')"
        @keyup.down="navigateConsoleInputHistory('down')">
        <template #append>
          <q-btn
            flat
            color="primary"
            icon="send"
            name="send"
            type="submit"
            aria-label="Send command"
            :disable="!hasPermission('game_server.console')"
            @click="sendGameServerInput">
            <q-tooltip v-if="!hasPermission('game_server.console')">
              Requires console permission
            </q-tooltip>
          </q-btn>
        </template>
      </q-input>
    </div>
  </div>
</template>

<script setup lang="ts">
import { create, toJsonString } from '@bufbuild/protobuf'
import ClipBoardCopy from '@/components/ClipBoardCopy.vue'
import StatusBadge from '@/components/StatusBadge.vue'
import GameServerMetrics from '@/components/game_servers/GameServerMetrics.vue'
import ServerSoftwareSelector from '@/components/game_servers/ServerSoftwareSelector.vue'
import { QScrollArea, useQuasar } from 'quasar'
import { tabMaximize } from 'quasar-extras-svg-icons/tabler-icons-v2'
import {
  AllServersQueryInfo,
  GameServer,
  GameServerSchema,
  ReadGameServerOutputRequest,
  ReadGameServerOutputRequestSchema,
  ReadGameServerOutputResponse,
  SendGameServerInputRequest,
  SendGameServerInputRequestSchema,
  ServerQuery_Type,
  StartGameServerRequest,
  StartGameServerRequestSchema,
  Status,
  StopGameServerRequest,
  StopGameServerRequestSchema,
} from 'src/proto/shared_pb'
import {
  GetGameServerRequest,
  GetGameServerRequestSchema,
  QueryGameServerRequest,
  QueryGameServerRequestSchema,
  QueryGameServerResponse,
  UpdateGameServerRequest,
  UpdateGameServerRequestSchema,
} from 'src/proto/xylona_pb'
import { Request, Request_Type, RequestSchema } from 'src/proto/websocket_pb'
import { ConnectError } from '@connectrpc/connect'
import { parseConsole } from '@/utils/console'
import {
  ConnectErrorToString,
  GetOrCreateXylonaWebsocketClient,
  GetXylonaClient,
  XylonaEventBus,
} from '@/utils/shared'
import { computed, nextTick, onBeforeUnmount, onMounted, Ref, ref } from 'vue'
import { useRoute } from 'vue-router'

const $q = useQuasar()
const gameServerOutput = ref('')
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

const startingServer = ref(false)
const stoppingServer = ref(false)
const updatingServer = ref(false)

const currentPlayerCount: Ref<number> = ref(0)
const maxPlayerCount: Ref<number> = ref(0)

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
    gameServer.value.status === Status.ONLINE
  )
})

const softwareDisplayName = computed(() => {
  return softwareSelector.value?.currentSoftwareDisplayName ?? ''
})

const hasSoftwareOptions = computed(() => {
  return (softwareSelector.value?.softwareOptions?.length ?? 0) > 0
})

const showChangeButton = computed(() => {
  if (!hasPermission('game_server.settings')) return false
  const optionCount = softwareSelector.value?.softwareOptions?.length ?? 0
  const versionCount = softwareSelector.value?.versions?.length ?? 0
  return optionCount > 1 || versionCount > 0
})

const softwareNameRedundant = computed(() => {
  const swName = softwareDisplayName.value.toLowerCase()
  const gameName = gameServer.value.gameName.toLowerCase()
  return swName === gameName || gameName.includes(swName) || swName.includes(gameName)
})

const softwareHasUpdate = computed(() => {
  return softwareSelector.value?.hasUpdate ?? false
})

const softwareLatestVersion = computed(() => {
  return softwareSelector.value?.latestVersion ?? ''
})

function hasPermission(perm: string): boolean {
  const perms = gameServer.value?.effectivePermissions ?? []
  // Empty permissions = unknown (cache fallback) — allow everything, backend enforces.
  return perms.length === 0 || perms.includes(perm)
}

onMounted(async () => {
  document.addEventListener('keydown', onEscapeKey)
  void getGameServerDetails()
    .then(() => {
      void getGameServerOutput()
      streamGameServerOutput()
      listenForServerQueryInfo()
      subscribeServerMetrics()
    })
    .then(queryGameServer)
})

onBeforeUnmount(() => {
  document.removeEventListener('keydown', onEscapeKey)
  unsubscribeServerMetrics()
})

function subscribeServerMetrics() {
  const ws = GetOrCreateXylonaWebsocketClient()
  const request: Request = create(RequestSchema, {})
  request.type = Request_Type.SubscribeServerMetrics
  request.gameServerId = gameServerId.value
  ws.send(toJsonString(RequestSchema, request))
}

function unsubscribeServerMetrics() {
  const ws = GetOrCreateXylonaWebsocketClient()
  const request: Request = create(RequestSchema, {})
  request.type = Request_Type.UnsubscribeServerMetrics
  request.gameServerId = gameServerId.value
  ws.send(toJsonString(RequestSchema, request))
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
      position: 'top',
      caption: 'Failed to load game server details: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  }
}

async function queryGameServer() {
  const request: QueryGameServerRequest = create(QueryGameServerRequestSchema, {})
  try {
    request.serverId = gameServerId.value
    const resp: QueryGameServerResponse = await GetXylonaClient().queryGameServer(request)
    switch (resp.queryInfo?.type) {
      case ServerQuery_Type.Minecraft:
        currentPlayerCount.value = resp.queryInfo.minecraft.numberOfPlayers
        maxPlayerCount.value = resp.queryInfo.minecraft.maxPlayers
        break
      case ServerQuery_Type.Source:
        currentPlayerCount.value = resp.queryInfo.source.players
        maxPlayerCount.value = resp.queryInfo.source.maxPlayers
        break
    }
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
      caption: 'Failed to query game server: ' + ConnectErrorToString(ConnectError.from(e)),
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
      position: 'top',
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
      position: 'top',
      caption: 'Failed to stop game server: ' + ConnectErrorToString(ConnectError.from(e)),
      icon: 'report_problem',
    })
  } finally {
    stoppingServer.value = false
  }
}

async function updateGameServer() {
  const request: UpdateGameServerRequest = create(UpdateGameServerRequestSchema, {})
  updatingServer.value = true
  try {
    request.serverId = gameServerId.value
    await GetXylonaClient().updateGameServer(request)
  } catch (e) {
    console.error(e)
    $q.notify({
      type: 'xylona-error',
      position: 'top',
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
    const combined = gameServerOutput.value + parseConsole(gameServer.value.gameId, response.output)
    if (combined.length > maxConsoleCharacters) {
      consoleTruncated.value = true
    }
    gameServerOutput.value = combined.slice(-maxConsoleCharacters)
    if (consoleScrollArea.value === null) {
      return
    }
    void nextTick(() => {
      consoleScrollArea.value?.setScrollPercentage('vertical', 100, 0)
    })
  } catch (e) {
    console.error(e)
  }
}

function listenForServerQueryInfo() {
  XylonaEventBus.on('gameServersQueryInfo', (allServersQueryInfo: AllServersQueryInfo) => {
    const queryInfo = allServersQueryInfo.servers[gameServerId.value]
    if (queryInfo === undefined) {
      return
    }
    switch (queryInfo.type) {
      case ServerQuery_Type.Minecraft:
        currentPlayerCount.value = queryInfo.minecraft.numberOfPlayers
        maxPlayerCount.value = queryInfo.minecraft.maxPlayers
        break
      case ServerQuery_Type.Source:
        currentPlayerCount.value = queryInfo.source.players
        maxPlayerCount.value = queryInfo.source.maxPlayers
        break
    }
  })
}

function streamGameServerOutput() {
  // Listen for game server status changes.
  XylonaEventBus.on('gameServerStatus', (serverID: string, serverStatus: Status) => {
    if (serverID !== gameServerId.value) {
      return
    }
    gameServer.value.status = serverStatus
  })

  // Stream game server output.
  XylonaEventBus.on('gameServerConsoleOutput', (serverID: string, output: string) => {
    if (serverID !== gameServerId.value) {
      return
    }
    const combined = gameServerOutput.value + parseConsole(gameServer.value.gameId, output)
    if (combined.length > maxConsoleCharacters) {
      consoleTruncated.value = true
    }
    gameServerOutput.value = combined.slice(-maxConsoleCharacters)
    if (consoleScrollArea.value === null) {
      return
    }
    void nextTick(() => {
      consoleScrollArea.value?.setScrollPercentage('vertical', 100, 0)
    })
  })
  // Request the game server to start streaming output.
  XylonaEventBus.emit('gameServerConsoleOutputRequest', gameServerId.value)

  // Handle socket reconnection.
  XylonaEventBus.on('websocketConnected', () => {
    XylonaEventBus.emit('gameServerConsoleOutputRequest', gameServerId.value)
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
      position: 'top',
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
.identity-header {
  padding: 0 var(--xy-space-md) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
  margin-bottom: var(--xy-space-md);
}

.identity-name {
  font-family: var(--xy-font-display);
  font-size: 1.35rem;
  font-weight: 700;
  color: var(--xy-text-primary);
  letter-spacing: 0.02em;
}

.identity-subtitle {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-top: 0.25rem;
  font-size: 0.82rem;
  color: var(--xy-text-secondary);
}

.identity-separator {
  color: var(--xy-text-muted);
  opacity: 0.5;
}

.identity-running {
  color: var(--xy-text-muted);
  font-style: italic;
}

.identity-version {
  font-family: var(--xy-font-mono);
  font-size: 0.75rem;
  color: var(--xy-text-muted);
}

.identity-version.version-outdated {
  color: var(--xy-accent);
  cursor: help;
}

.identity-change-btn {
  border: 1px solid var(--xy-border);
  border-radius: 5px;
  color: var(--xy-text-secondary);
  font-family: var(--xy-font-display);
  font-size: 0.72rem;
  padding: 0.15rem 0.5rem;
  margin-left: 0.25rem;
  transition: all var(--xy-transition-fast);
}

.identity-change-btn:hover {
  border-color: var(--xy-primary);
  color: var(--xy-primary);
  background: var(--xy-primary-muted);
}

.server-controls {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
  margin-top: var(--xy-space-sm);
}

.console-panel {
  display: flex;
  flex-direction: column;
  position: relative;
}

.console-expand-btn {
  position: absolute;
  top: var(--xy-space-xs);
  right: var(--xy-space-sm);
  z-index: 1;
  opacity: 0.6;
  transition: opacity var(--xy-transition-fast);
}

.console-expand-btn:hover {
  opacity: 1;
}

.console-panel :deep(.q-scrollarea) {
  flex: 1;
  min-height: 50dvh;
  padding-left: var(--xy-space-sm);
  padding-right: var(--xy-space-sm);
  font-family: var(--xy-font-mono);
  font-size: 0.85rem;
  font-weight: 400;
  font-style: normal;
  overflow-x: hidden;
  white-space: pre-wrap;
  max-width: 100%;
  background-color: var(--xy-base);
  border-top: 1px solid var(--xy-border);
}

.console-truncated-notice {
  font-family: var(--xy-font-mono);
  font-size: 0.75rem;
  color: var(--xy-text-muted);
  text-align: center;
  padding: var(--xy-space-xs) 0;
  border-bottom: 1px solid var(--xy-border);
}

.console-panel :deep(#consoleCodeEl) {
  white-space: pre-wrap;
}

.console-panel :deep(#consoleInput) {
  background-color: var(--xy-surface-1);
  border: none;
  border-top: 1px solid var(--xy-border);
}

.console-panel :deep(#consoleInput .q-field__control) {
  font-family: var(--xy-font-mono);
}

@media (max-width: 1023px) {
  .console-panel {
    order: -1;
  }

  .info-panel {
    order: 1;
  }
}

.expanded {
  position: fixed !important;
  inset: 0 !important;
  z-index: 9999;
  width: 100%;
  height: 100dvh;
  margin: 0;
  padding: 0;
  background-color: var(--xy-base);
  animation: console-expand 200ms cubic-bezier(0.25, 1, 0.5, 1) both;
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

.expanded :deep(.q-scrollarea) {
  min-height: 0;
  flex: 1;
}
</style>
