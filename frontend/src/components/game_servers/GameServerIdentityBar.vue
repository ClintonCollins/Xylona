<template>
  <div class="identity">
    <div class="identity-bar">
      <div class="identity-bar-left">
        <p :title="server.name" class="identity-bar-name">{{ server.name }}</p>
        <span class="identity-bar-detail">
          <span>{{ server.gameName }}</span>
          <template v-if="softwareName !== '' && !softwareNameRedundant">
            <span aria-hidden="true" class="identity-bar-sep">&middot;</span>
            <span class="identity-bar-running">on</span>
            <span>{{ softwareName }}</span>
          </template>
          <span
            v-if="versionDisplay.installedVersion && softwareName !== ''"
            :class="{ 'version-outdated': versionDisplay.updateAvailable }"
            class="identity-bar-version">
            {{ versionDisplay.installedVersion }}
            <span v-if="versionDisplay.updateAvailable" class="xy-visually-hidden">
              — update available: {{ versionDisplay.latestVersion }}
            </span>
            <q-tooltip v-if="versionDisplay.updateAvailable">
              Update available: {{ versionDisplay.latestVersion }}
            </q-tooltip>
          </span>
        </span>
      </div>
      <span aria-live="polite" class="identity-bar-status" role="status">
        <status-badge :phase="statusBadgePhase" :status="server.status" />
      </span>
      <div class="identity-bar-actions">
        <q-btn
          :aria-label="
            playerCountUnknown
              ? 'Player count unknown. Show players'
              : `${playerCountLabel} players online. Show players`
          "
          class="identity-bar-players"
          dense
          flat
          icon="group"
          no-caps
          no-wrap
          @click="playersOpen = true">
          <span class="identity-bar-players__count">{{ playerCountLabel }}</span>
          <q-tooltip>Players</q-tooltip>
        </q-btn>
        <!-- Hints sit on a wrapper: a disabled button gets no hover, so its own tooltip never shows. -->
        <div aria-label="Server controls" class="identity-bar-lifecycle" role="group">
          <span class="identity-bar-control">
            <q-btn
              :aria-label="startAriaLabel"
              :disable="!canStart"
              :loading="startingServer"
              color="positive"
              dense
              icon="play_arrow"
              label="Start"
              no-caps
              no-wrap
              outline
              @click="startGameServer" />
            <q-tooltip v-if="startHint">{{ startHint }}</q-tooltip>
          </span>
          <span class="identity-bar-control">
            <q-btn
              :aria-label="lifecycleAriaLabel('Restart', 'game_server.restart')"
              :disable="!canRestart"
              :loading="restartingServer"
              color="warning"
              dense
              icon="restart_alt"
              label="Restart"
              no-caps
              no-wrap
              outline
              @click="restartGameServer" />
            <q-tooltip v-if="lifecycleHint('game_server.restart')">
              {{ lifecycleHint('game_server.restart') }}
            </q-tooltip>
          </span>
          <span class="identity-bar-control">
            <q-btn
              :aria-label="lifecycleAriaLabel('Stop', 'game_server.stop')"
              :disable="!canStop"
              :loading="stoppingServer"
              color="negative"
              dense
              icon="stop"
              label="Stop"
              no-caps
              no-wrap
              outline
              @click="stopGameServer" />
            <q-tooltip v-if="lifecycleHint('game_server.stop')">
              {{ lifecycleHint('game_server.stop') }}
            </q-tooltip>
          </span>
        </div>
      </div>
    </div>

    <div v-if="!serverStateAuthoritative" class="identity-bar-hint" role="status">
      Waiting for server status — controls are paused until it is confirmed.
    </div>
    <!-- Spelled out, not only a tooltip: touch screens never show the disabled Start's hint. -->
    <div
      v-else-if="startBlockedVisible"
      class="identity-bar-hint identity-bar-blocker"
      role="status">
      <span class="identity-bar-blocker__text">
        <q-icon aria-hidden="true" class="identity-bar-blocker__icon" name="report_problem" />
        {{ startHint }}
      </span>
      <q-btn
        v-if="blockerFixedInConfiguration"
        :to="`/game-servers/${server.id}/configuration`"
        color="primary"
        dense
        flat
        icon="tune"
        label="Open Configuration"
        no-caps />
    </div>

    <div v-if="lastStartFailure" class="start-failure" role="alert">
      <q-icon aria-hidden="true" class="start-failure__icon" name="report_problem" />
      <div class="start-failure__body">
        <span class="start-failure__title">
          Start failed at {{ formatFailureTime(lastStartFailure.at) }}
        </span>
        <span class="start-failure__message">{{ lastStartFailure.message }}</span>
      </div>
      <div class="start-failure__actions">
        <q-btn
          v-if="!onConsole"
          :to="`/game-servers/${server.id}/console`"
          dense
          flat
          label="Show output"
          no-caps />
        <q-btn
          :disable="!canStart"
          :loading="startingServer"
          color="primary"
          dense
          label="Start again"
          no-caps
          unelevated
          @click="startGameServer" />
        <q-btn
          aria-label="Dismiss start failure"
          dense
          flat
          icon="close"
          round
          @click="lastStartFailure = null" />
      </div>
    </div>

    <q-dialog
      v-model="playersOpen"
      aria-labelledby="identity-players-title"
      :position="$q.screen.lt.sm ? 'bottom' : 'standard'">
      <q-card class="identity-players">
        <div class="identity-players__head">
          <span id="identity-players-title" class="identity-players__title">
            Players on {{ server.name }}
          </span>
          <q-btn v-close-popup aria-label="Close players" dense flat icon="close" round />
        </div>
        <div class="identity-players__body">
          <p v-if="playerCountUnknown" class="q-ma-none text-caption text-xy-muted" role="status">
            {{ unknownPlayersMessage }}
          </p>
          <game-server-player-roster
            v-else
            :can-manage-players="
              server.gameId !== 'valheim' && hasPermission('game_server.players.manage')
            "
            :current-player-count="currentPlayerCount"
            :game-server-id="server.id"
            :is-online="isServerOnline"
            :max-player-count="displayedMaxPlayerCount"
            :native-identifiers-required="server.gameId === '7_days_to_die'"
            :player-list-supported="playerListSupported"
            :player-names="onlinePlayers"
            :unlisted-player-count="unlistedPlayerCount" />
        </div>
        <div v-if="hasPermission('game_server.players.manage')" class="identity-players__foot">
          <q-btn
            dense
            flat
            icon="manage_accounts"
            :label="isSevenDays ? 'Player operations' : 'Player management'"
            no-caps
            @click="openPlayerManagement" />
        </div>
      </q-card>
    </q-dialog>

    <game-server-player-management-dialog
      v-if="!isSevenDays"
      v-model="playerManagementOpen"
      :game-server-id="server.id" />
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref, watch, type Ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { connectErrorMessage } from '@/api/connect-errors'
import { notifyConnectError } from '@/api/notifications'
import StatusBadge from '@/components/StatusBadge.vue'
import GameServerPlayerManagementDialog from '@/components/game_servers/GameServerPlayerManagementDialog.vue'
import GameServerPlayerRoster from '@/components/game_servers/GameServerPlayerRoster.vue'
import { playerLimit } from '@/components/game_servers/start-args'
import {
  type GameServer,
  RestartGameServerRequestSchema,
  StartGameServerRequestSchema,
  Status,
  StopGameServerRequestSchema,
} from '@/proto/shared_pb'
import {
  findStartBlocker,
  isConfigReadinessItem,
  readinessLabel,
  useGameServerReadiness,
} from '@/pages/game_servers/game-server-readiness'
import {
  buildLifecycleConfirmation,
  isServerRunning,
  type LifecycleConfirmAction,
} from '@/pages/game_servers/server-list-actions'
import {
  detectStartFailure,
  formatFailureTime,
  type StartFailure,
} from '@/pages/game_servers/start-failure'
import { useGameServerQueryStatusVersion } from '@/pages/game_servers/useGameServerQueryStatusVersion'
import { resolveCanonicalVersionDisplay } from '@/pages/game_servers/version-display'
import { isServerStopping } from '@/utils/game-server-stopping'
import { GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import { websocketStateAuthoritative } from '@/utils/websocket-connection'

const props = defineProps<{
  /** Latest snapshot from GetGameServer; live status and players update on top of it. */
  gameServer: GameServer
}>()

const $q = useQuasar()
const route = useRoute()
const router = useRouter()

// Shallow copy so live status and version events never write into the parent's snapshot.
const server = ref({ ...props.gameServer }) as Ref<GameServer>
const gameServerId = computed(() => server.value.id)
const statusFresh = ref(websocketStateAuthoritative.value)

// ponytail: a status event that lands while the parent is refetching can be
// overwritten by the older snapshot; the next status event corrects it.
watch(
  () => props.gameServer,
  (next) => {
    server.value = { ...next }
    statusFresh.value = websocketStateAuthoritative.value
  },
)

const {
  currentPlayerCount,
  maxPlayerCount,
  onlinePlayers,
  playerCount,
  playerListSupported,
  unknownPlayersMessage,
  queryGameServer,
  startQueryStatusVersionLifecycle,
} = useGameServerQueryStatusVersion({ gameServer: server, gameServerId })
const readiness = useGameServerReadiness(gameServerId)
const startBlocker = computed(() => findStartBlocker(readiness.items.value))

const startingServer = ref(false)
const stoppingServer = ref(false)
const restartingServer = ref(false)
const lastStartFailure = ref<StartFailure | null>(null)
const lifecycleIntents = { startRequestedAt: 0, stopRequestedAt: 0 }
const playersOpen = ref(false)
const playerManagementOpen = ref(false)

const isServerOnline = computed(() => server.value.status === Status.ONLINE)
// Starting servers can be stopped or restarted too; only players wait for Online.
const isRunning = computed(() => isServerRunning(server.value.status))
// A stop from any view, schedule or tab, not only this bar's own button.
const isStopping = computed(
  () => stoppingServer.value || isServerStopping(server.value.id, server.value.status),
)
const isSevenDays = computed(() => server.value.gameId === '7_days_to_die')
const onConsole = computed(() => route.path.endsWith('/console'))
const serverStateAuthoritative = computed(
  () =>
    websocketStateAuthoritative.value &&
    statusFresh.value &&
    server.value.status !== Status.UNKNOWN,
)
const canStart = computed(
  () =>
    serverStateAuthoritative.value &&
    server.value.status === Status.OFFLINE &&
    hasPermission('game_server.start') &&
    startBlocker.value === undefined,
)
const canStop = computed(
  () =>
    serverStateAuthoritative.value &&
    isRunning.value &&
    !isStopping.value &&
    hasPermission('game_server.stop'),
)
const canRestart = computed(
  () =>
    serverStateAuthoritative.value &&
    isRunning.value &&
    !isStopping.value &&
    hasPermission('game_server.restart'),
)
const statusBadgePhase = computed(() => {
  if (restartingServer.value) return 'restarting'
  if (isStopping.value) return 'stopping'
  if (lastStartFailure.value && server.value.status === Status.OFFLINE) return 'failed'
  return undefined
})
// A setup blocker only explains Start while the server is offline; a running one is not startable anyway.
const startBlockedVisible = computed(
  () =>
    startBlocker.value !== undefined &&
    server.value.status === Status.OFFLINE &&
    hasPermission('game_server.start'),
)
const startHint = computed(() => {
  const permissionOrStatusHint = lifecycleHint('game_server.start')
  if (permissionOrStatusHint !== '') return permissionOrStatusHint
  const blocker = startBlocker.value
  if (!startBlockedVisible.value || blocker === undefined) return ''
  return `Finish setup first — ${readinessLabel(blocker.kind)}: ${blocker.message}`
})
const blockerFixedInConfiguration = computed(
  () =>
    startBlocker.value !== undefined &&
    isConfigReadinessItem(startBlocker.value) &&
    hasPermission('game_server.config') &&
    !route.path.endsWith('/configuration'),
)
const startAriaLabel = computed(() => {
  const blocker = startBlocker.value
  if (startBlockedVisible.value && blocker !== undefined) {
    return `Start (blocked until ${readinessLabel(blocker.kind)} is finished)`
  }
  return lifecycleAriaLabel('Start', 'game_server.start')
})

// Without a query reply (e.g. offline Valheim) show the configured limit, as the server list does.
const displayedMaxPlayerCount = computed(
  () => maxPlayerCount.value || Number(playerLimit(server.value)),
)
const playerCountUnknown = computed(() => playerCount.value === null)
const playerCountLabel = computed(() =>
  playerCountUnknown.value
    ? '?'
    : `${currentPlayerCount.value}/${displayedMaxPlayerCount.value || '?'}`,
)
const unlistedPlayerCount = computed(() =>
  Math.max(currentPlayerCount.value - onlinePlayers.value.length, 0),
)

const softwareName = computed(() => {
  const variants = server.value.game?.variants ?? []
  if (variants.length === 0) return ''
  const selected = variants.find((variant) => variant.id === server.value.selectedVariantId)
  return selected?.name || server.value.gameName || 'Default'
})
const softwareNameRedundant = computed(() => {
  const swName = softwareName.value.toLowerCase()
  const gameName = server.value.gameName.toLowerCase()
  return swName === gameName || gameName.includes(swName) || swName.includes(gameName)
})
const versionDisplay = computed(() =>
  resolveCanonicalVersionDisplay(server.value.version, server.value.versionInfo),
)

function hasPermission(perm: string): boolean {
  const perms = server.value.effectivePermissions
  // Empty permissions = unknown (cache fallback) — allow everything, backend enforces.
  return perms.length === 0 || perms.includes(perm)
}

function lifecycleHint(perm: string): string {
  if (!hasPermission(perm)) return `Requires ${perm.split('.').pop()} permission`
  if (!serverStateAuthoritative.value) return 'Waiting for authoritative server status'
  return ''
}

function lifecycleAriaLabel(label: string, perm: string): string | undefined {
  return hasPermission(perm) ? undefined : `${label} (requires ${perm.split('.').pop()} permission)`
}

function openPlayerManagement(): void {
  playersOpen.value = false
  if (isSevenDays.value) {
    void router.push(`/game-servers/${server.value.id}/operations`)
    return
  }
  playerManagementOpen.value = true
}

function onServerStatus(serverID: string, _serverName: string, status: Status): void {
  if (serverID !== server.value.id) return
  const failure = detectStartFailure(status, lifecycleIntents, Date.now())
  if (failure !== undefined) {
    lastStartFailure.value = failure
    lifecycleIntents.startRequestedAt = 0
  }
  statusFresh.value = websocketStateAuthoritative.value
}

function onWebsocketDisconnect(): void {
  statusFresh.value = false
}

onMounted(() => {
  XylonaEventBus.on('gameServerStatus', onServerStatus)
  XylonaEventBus.on('websocketDisconnected', onWebsocketDisconnect)
  startQueryStatusVersionLifecycle()
  void queryGameServer()
})

onBeforeUnmount(() => {
  XylonaEventBus.off('gameServerStatus', onServerStatus)
  XylonaEventBus.off('websocketDisconnected', onWebsocketDisconnect)
})

function confirmLifecycleAction(action: LifecycleConfirmAction): Promise<boolean> {
  const confirmation = buildLifecycleConfirmation(action, [
    { displayName: server.value.name, playerCount: playerCount.value },
  ])
  if (confirmation === null) return Promise.resolve(true)
  return new Promise<boolean>((resolve) => {
    let settled = false
    $q.dialog({
      title: confirmation.title,
      message: confirmation.message,
      cancel: true,
      persistent: true,
      ok: {
        label: confirmation.confirmLabel,
        color: confirmation.confirmColor,
        unelevated: true,
      },
    })
      .onOk(() => {
        settled = true
        resolve(true)
      })
      .onDismiss(() => {
        if (!settled) resolve(false)
      })
  })
}

async function startGameServer(): Promise<void> {
  if (!canStart.value) return
  startingServer.value = true
  lastStartFailure.value = null
  lifecycleIntents.startRequestedAt = Date.now()
  try {
    await GetXylonaClient().startGameServer(
      create(StartGameServerRequestSchema, { serverId: server.value.id }),
    )
  } catch (error) {
    console.error(error)
    lifecycleIntents.startRequestedAt = 0
    lastStartFailure.value = { at: Date.now(), message: connectErrorMessage(error) }
    notifyConnectError(error, 'Failed to start game server')
    // Readiness is re-read so a blocker behind the rejection disables Start.
    XylonaEventBus.emit('gameServerStartRejected', server.value.id)
  } finally {
    startingServer.value = false
  }
}

async function stopGameServer(): Promise<void> {
  if (!canStop.value || !(await confirmLifecycleAction('stop'))) return
  stoppingServer.value = true
  lifecycleIntents.stopRequestedAt = Date.now()
  try {
    await GetXylonaClient().stopGameServer(
      create(StopGameServerRequestSchema, { serverId: server.value.id }),
    )
  } catch (error) {
    console.error(error)
    notifyConnectError(error, 'Failed to stop game server')
  } finally {
    stoppingServer.value = false
  }
}

async function restartGameServer(): Promise<void> {
  if (!canRestart.value || !(await confirmLifecycleAction('restart'))) return
  restartingServer.value = true
  // The restart passes through Offline; that is not a failed start.
  lifecycleIntents.stopRequestedAt = Date.now()
  try {
    await GetXylonaClient().restartGameServer(
      create(RestartGameServerRequestSchema, { serverId: server.value.id }),
    )
  } catch (error) {
    console.error(error)
    notifyConnectError(error, 'Failed to restart game server')
  } finally {
    restartingServer.value = false
  }
}
</script>

<style scoped>
.identity {
  flex-shrink: 0;
  background: var(--xy-surface-1);
  border-bottom: 1px solid var(--xy-border);
}

.identity-bar {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
  padding: var(--xy-space-sm) var(--xy-space-md);
}

.identity-bar-left {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-2xs);
  min-width: 0;
  flex: 0 1 auto;
}

.identity-bar-name {
  margin: 0;
  font-family: var(--xy-font-display);
  font-size: var(--xy-font-size-lg);
  font-weight: 700;
  line-height: 1.2;
  color: var(--xy-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.identity-bar-detail {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: var(--xy-space-xs);
  font-size: var(--xy-font-size-xs);
  color: var(--xy-text-secondary);
}

.identity-bar-sep {
  color: var(--xy-text-muted);
}

.identity-bar-running {
  font-style: italic;
  color: var(--xy-text-muted);
}

.identity-bar-version {
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-2xs);
  color: var(--xy-text-muted);
}

.identity-bar-version.version-outdated {
  color: var(--xy-accent);
  cursor: help;
}

.identity-bar-status {
  flex-shrink: 0;
}

.identity-bar-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  margin-left: auto;
  flex-shrink: 0;
}

.identity-bar-players__count {
  margin-left: var(--xy-space-xs);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-sm);
}

.identity-bar-lifecycle {
  display: flex;
  gap: var(--xy-space-xs);
}

.identity-bar-lifecycle .q-btn {
  padding-inline: var(--xy-space-sm);
}

.identity-bar-control {
  display: inline-flex;
}

/* Browsers send no hover to a disabled button; let it reach the wrapper's hint. */
.identity-bar-control:has(.q-btn.disabled) {
  cursor: not-allowed;
}

.identity-bar-control .q-btn.disabled {
  pointer-events: none;
}

.identity-bar-hint {
  padding: 0 var(--xy-space-md) var(--xy-space-sm);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.identity-bar-blocker {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  column-gap: var(--xy-space-xs);
  color: var(--xy-text-secondary);
}

/* Inline in the text, so it stays beside the first line when a phone wraps the reason. */
.identity-bar-blocker__icon {
  margin-right: var(--xy-space-xs);
  vertical-align: -0.15em;
  font-size: var(--xy-font-size-sm);
  color: var(--xy-warning);
}

.identity-bar-blocker__text {
  flex: 1 1 16rem;
  min-width: 0;
  overflow-wrap: anywhere;
}

.start-failure {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
  padding: var(--xy-space-sm) var(--xy-space-md);
  background: var(--xy-danger-bg);
  border-top: 1px solid var(--xy-danger-border);
}

.start-failure__icon {
  flex-shrink: 0;
  font-size: var(--xy-font-size-lg);
  color: var(--xy-danger);
}

.start-failure__body {
  display: flex;
  flex-wrap: wrap;
  column-gap: var(--xy-space-sm);
  row-gap: var(--xy-space-2xs);
  min-width: 0;
  flex: 1;
  font-size: var(--xy-font-size-sm);
}

.start-failure__title {
  font-weight: 600;
  color: var(--xy-text-primary);
}

.start-failure__message {
  color: var(--xy-text-secondary);
  overflow-wrap: anywhere;
}

.start-failure__actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  flex-shrink: 0;
}

.identity-players {
  width: min(28rem, 100vw);
  max-height: 80dvh;
  display: flex;
  flex-direction: column;
}

.identity-players__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-sm) var(--xy-space-sm) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.identity-players__title {
  min-width: 0;
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.identity-players__body {
  flex: 1;
  min-height: 0;
  padding: var(--xy-space-md);
  overflow-y: auto;
}

.identity-players__foot {
  display: flex;
  justify-content: flex-end;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border-top: 1px solid var(--xy-border);
}

@media (max-width: 1023px) {
  .identity-bar {
    flex-wrap: wrap;
    row-gap: var(--xy-space-sm);
  }

  .identity-bar-name {
    font-size: var(--xy-font-size-base);
  }
}

@media (max-width: 599px) {
  .identity-bar {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm) var(--xy-space-base);
  }

  /* Phones drop the game/version line: name + status on one row, controls on the next. */
  .identity-bar-detail {
    display: none;
  }

  .identity-bar-actions {
    grid-column: 1 / -1;
    justify-content: space-between;
    margin-left: 0;
    container-type: inline-size;
  }

  .identity-bar-lifecycle .q-btn {
    min-height: 44px;
    min-width: 44px;
    padding-inline: var(--xy-space-xs);
  }

  .identity-bar-players {
    min-height: 44px;
  }

  .start-failure {
    flex-wrap: wrap;
  }

  .start-failure__actions {
    width: 100%;
    justify-content: flex-end;
  }
}

/* Narrow phones: labelled controls plus a 3-digit player count overflow the row,
   so the controls go icon-only (their aria-labels still name them). */
@container (max-width: 360px) {
  /* !important: Quasar's .block utility is display: block !important. */
  .identity-bar-lifecycle .q-btn :deep(.block) {
    display: none !important;
  }

  .identity-bar-lifecycle .q-btn :deep(.on-left) {
    margin-right: 0;
  }
}
</style>
