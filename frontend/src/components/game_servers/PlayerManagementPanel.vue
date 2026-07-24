<script lang="ts" setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { create } from '@bufbuild/protobuf'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import type { GameServerPlayer } from '@/proto/shared_pb'
import { Status } from '@/proto/shared_pb'
import type { AllServersQueryInfo } from '@/proto/websocket_pb'
import {
  GameServerPlayerAction,
  GetGameServerPlayerManagementRequestSchema,
  PerformGameServerPlayerActionRequestSchema,
  type GetGameServerPlayerManagementResponse,
} from '@/proto/xylona_pb'
import { GetXylonaClient, XylonaEventBus } from '@/utils/shared'
import {
  getPlayerActionDefinition,
  getQuickPlayerActionDefinitions,
  getSupportedPlayerActionDefinitions,
  type PlayerActionDefinition,
} from '@/pages/game_servers/player-management'
import { queryInfoPlayerSnapshot } from '@/pages/game_servers/useGameServerQueryStatusVersion'

interface PendingPlayerAction {
  action: GameServerPlayerAction
  playerId: string
  playerName: string
}

const props = defineProps<{ gameServerId: string }>()

const loading = ref(true)
const loadError = ref(false)
const management = ref<GetGameServerPlayerManagementResponse | null>(null)
const performing = ref(false)

const manualAction = ref<PlayerActionDefinition | null>(null)
const manualPlayerId = ref('')

const confirmDialogOpen = ref(false)
const pendingAction = ref<PendingPlayerAction | null>(null)
const actionReason = ref('')

const capabilities = computed(() => management.value?.capabilities)
const isOnline = computed(() => management.value?.status === Status.ONLINE)
const players = computed(() => management.value?.players ?? [])
const supportedActionDefinitions = computed(() =>
  getSupportedPlayerActionDefinitions(capabilities.value?.supportedActions ?? []),
)
const quickActionDefinitions = computed(() =>
  getQuickPlayerActionDefinitions(capabilities.value?.supportedActions ?? []),
)
const canPerformActions = computed(
  () => Boolean(capabilities.value?.actionsSupported) && isOnline.value && !performing.value,
)
const identifierLabel = computed(() => capabilities.value?.identifierLabel || 'Player identifier')
const manualActionReady = computed(
  () =>
    manualAction.value !== null && manualPlayerId.value.trim() !== '' && canPerformActions.value,
)
const pendingDefinition = computed(() => {
  if (pendingAction.value === null) {
    return null
  }
  return getPlayerActionDefinition(pendingAction.value.action)
})
const pendingTarget = computed(
  () => pendingAction.value?.playerName || pendingAction.value?.playerId || 'this player',
)

async function loadPlayerManagement(options: { quiet?: boolean } = {}): Promise<void> {
  if (props.gameServerId === '') {
    return
  }
  if (!options.quiet) {
    loading.value = true
    loadError.value = false
  }
  try {
    management.value = await GetXylonaClient().getGameServerPlayerManagement(
      create(GetGameServerPlayerManagementRequestSchema, {
        gameServerId: props.gameServerId,
      }),
    )
    loadError.value = false
  } catch (error) {
    // Background refreshes keep the last good data; only interactive loads
    // surface the failure.
    if (!options.quiet) {
      management.value = null
      loadError.value = true
      notifyConnectError(error, 'Unable to load player management')
    }
  } finally {
    if (!options.quiet) {
      loading.value = false
    }
  }
}

function refresh(): void {
  void loadPlayerManagement()
}

// --- Live updates: the websocket already broadcasts roster and status
// changes for every server; when they disagree with what this panel shows,
// re-pull the management view (which carries identifiers and capabilities).
let liveRefreshTimer: ReturnType<typeof setTimeout> | undefined

function scheduleQuietRefresh(): void {
  if (liveRefreshTimer !== undefined) return
  liveRefreshTimer = setTimeout(() => {
    liveRefreshTimer = undefined
    if (!performing.value) void loadPlayerManagement({ quiet: true })
  }, 1500)
}

function onServersQueryInfo(allServersQueryInfo: AllServersQueryInfo): void {
  const queryInfo = allServersQueryInfo.servers[props.gameServerId]
  if (queryInfo === undefined || management.value === null) return
  const snapshot = queryInfoPlayerSnapshot(queryInfo)
  if (snapshot === null || !snapshot.playerListSupported) return

  const knownNames = new Set(players.value.map((player) => player.name))
  const rosterChanged =
    snapshot.players.length !== knownNames.size ||
    snapshot.players.some((name) => !knownNames.has(name))
  if (rosterChanged) scheduleQuietRefresh()
}

function onServerStatusUpdate(serverID: string, _serverName: string, serverStatus: Status): void {
  if (serverID !== props.gameServerId) return
  if (management.value !== null && management.value.status !== serverStatus) {
    scheduleQuietRefresh()
  }
}

onMounted(() => {
  XylonaEventBus.on('gameServersQueryInfo', onServersQueryInfo)
  XylonaEventBus.on('gameServerStatus', onServerStatusUpdate)
})

onBeforeUnmount(() => {
  XylonaEventBus.off('gameServersQueryInfo', onServersQueryInfo)
  XylonaEventBus.off('gameServerStatus', onServerStatusUpdate)
  if (liveRefreshTimer !== undefined) clearTimeout(liveRefreshTimer)
})

function openPlayerAction(
  definition: PlayerActionDefinition,
  playerId: string,
  playerName: string,
): void {
  if (!canPerformActions.value || playerId.trim() === '') {
    return
  }
  pendingAction.value = {
    action: definition.action,
    playerId: playerId.trim(),
    playerName: playerName.trim(),
  }
  actionReason.value = ''
  confirmDialogOpen.value = true
}

function openManualPlayerAction(): void {
  if (manualAction.value === null) {
    return
  }
  const playerId = manualPlayerId.value.trim()
  openPlayerAction(manualAction.value, playerId, playerId)
}

function closePlayerAction(): void {
  if (performing.value) {
    return
  }
  confirmDialogOpen.value = false
  pendingAction.value = null
  actionReason.value = ''
}

async function performPlayerAction(): Promise<void> {
  const action = pendingAction.value
  const definition = pendingDefinition.value
  if (action === null || definition === null || performing.value) {
    return
  }

  performing.value = true
  try {
    await GetXylonaClient().performGameServerPlayerAction(
      create(PerformGameServerPlayerActionRequestSchema, {
        gameServerId: props.gameServerId,
        action: action.action,
        playerId: action.playerId,
        reason: definition.reasonAllowed ? actionReason.value.trim() || undefined : undefined,
      }),
    )
    notifySuccess(`${definition.label} action sent.`)
    confirmDialogOpen.value = false
    pendingAction.value = null
    actionReason.value = ''
    manualPlayerId.value = ''
    await loadPlayerManagement()
  } catch (error) {
    notifyConnectError(error, `Unable to ${definition.label.toLowerCase()} player`)
  } finally {
    performing.value = false
  }
}

function playerSecondaryText(player: GameServerPlayer): string {
  if (player.id === undefined || player.id === '' || player.id === player.name) {
    return ''
  }
  return player.id
}

function actionTextColor(definition: PlayerActionDefinition | null): string {
  return definition?.color === 'warning' || definition?.color === 'positive' ? 'dark' : 'white'
}

watch(
  () => props.gameServerId,
  () => void loadPlayerManagement(),
  { immediate: true },
)

defineExpose({ loadPlayerManagement })
</script>

<template>
  <div class="players-panel">
    <q-banner v-if="loadError" class="players-panel__banner players-panel__banner--danger" rounded>
      <template #avatar><q-icon color="negative" name="cloud_off" /></template>
      Player management could not be loaded from the server's node.
      <template #action>
        <q-btn color="negative" flat label="Retry" no-caps @click="refresh" />
      </template>
    </q-banner>

    <template v-else-if="loading && management === null">
      <q-card class="players-panel__card">
        <q-card-section>
          <q-skeleton class="q-mb-md" height="28px" type="rect" width="180px" />
          <q-skeleton class="q-mb-sm" type="text" />
          <q-skeleton class="q-mb-sm" type="text" />
          <q-skeleton type="text" width="70%" />
        </q-card-section>
      </q-card>
    </template>

    <template v-else-if="management !== null">
      <q-card class="players-panel__card">
        <q-card-section class="players-panel__summary">
          <div>
            <div class="players-panel__card-title">Live roster</div>
            <div class="players-panel__card-copy">
              {{ players.length }} {{ players.length === 1 ? 'player' : 'players' }} reported
            </div>
          </div>
          <div class="players-panel__summary-side">
            <q-chip
              :class="isOnline ? 'players-panel__status--online' : 'players-panel__status--offline'"
              :icon="isOnline ? 'radio_button_checked' : 'power_off'"
              :label="isOnline ? 'Online' : 'Offline'" />
            <q-btn
              :disable="loading || performing"
              :loading="loading"
              aria-label="Refresh player management"
              dense
              flat
              icon="refresh"
              round
              @click="refresh">
              <q-tooltip>Refresh</q-tooltip>
            </q-btn>
          </div>
        </q-card-section>

        <q-separator />

        <q-card-section v-if="!isOnline">
          <q-banner class="players-panel__banner players-panel__banner--warning" dense rounded>
            <template #avatar><q-icon color="warning" name="power_settings_new" /></template>
            Start the game server to query its roster or perform player actions.
          </q-banner>
        </q-card-section>
        <q-card-section v-else-if="!capabilities?.actionsSupported">
          <q-banner class="players-panel__banner players-panel__banner--info" dense rounded>
            <template #avatar><q-icon color="info" name="visibility" /></template>
            {{
              capabilities?.unavailableReason || 'Player actions are not available for this game.'
            }}
          </q-banner>
        </q-card-section>

        <q-card-section v-if="players.length === 0" class="players-panel__empty">
          <q-icon name="group_off" size="42px" />
          <div class="players-panel__empty-title">No players reported</div>
          <div>
            {{
              isOnline
                ? 'The server query returned an empty roster.'
                : 'The roster will appear after the server starts.'
            }}
          </div>
        </q-card-section>

        <q-list v-else class="players-panel__roster" separator>
          <q-item
            v-for="player in players"
            :key="player.id || player.name"
            class="players-panel__player">
            <q-item-section avatar>
              <q-avatar class="players-panel__avatar" icon="person" />
            </q-item-section>
            <q-item-section>
              <q-item-label class="players-panel__player-name">{{ player.name }}</q-item-label>
              <q-item-label
                v-if="playerSecondaryText(player)"
                caption
                class="players-panel__player-id">
                {{ playerSecondaryText(player) }}
              </q-item-label>
            </q-item-section>
            <q-item-section v-if="player.id && quickActionDefinitions.length > 0" side>
              <div class="players-panel__quick-actions">
                <q-btn
                  v-for="definition in quickActionDefinitions"
                  :key="definition.action"
                  :aria-label="`${definition.label} ${player.name}`"
                  :color="definition.color"
                  :disable="!canPerformActions"
                  :icon="definition.icon"
                  :label="definition.label"
                  dense
                  flat
                  no-caps
                  @click="openPlayerAction(definition, player.id || '', player.name)" />
              </div>
            </q-item-section>
          </q-item>
        </q-list>
      </q-card>

      <q-card v-if="supportedActionDefinitions.length > 0" class="players-panel__card">
        <q-card-section>
          <div class="players-panel__card-title">Manage by {{ identifierLabel.toLowerCase() }}</div>
          <div class="players-panel__card-copy">
            Use this for offline players, unbans, and allowlist changes that are not represented in
            the live roster.
          </div>
        </q-card-section>
        <q-card-section class="players-panel__manual-form">
          <q-select
            v-model="manualAction"
            :disable="!canPerformActions"
            :options="supportedActionDefinitions"
            label="Action"
            option-label="label"
            outlined>
            <template #prepend>
              <q-icon :name="manualAction?.icon || 'admin_panel_settings'" />
            </template>
          </q-select>
          <q-input
            v-model="manualPlayerId"
            :disable="!canPerformActions"
            :label="identifierLabel"
            autocomplete="off"
            maxlength="256"
            outlined
            @keyup.enter="openManualPlayerAction">
            <template #prepend><q-icon name="fingerprint" /></template>
          </q-input>
          <q-btn
            :color="manualAction?.color || 'primary'"
            :disable="!manualActionReady"
            :icon="manualAction?.icon || 'play_arrow'"
            label="Review action"
            no-caps
            :text-color="actionTextColor(manualAction)"
            @click="openManualPlayerAction" />
        </q-card-section>
      </q-card>
    </template>

    <q-dialog v-model="confirmDialogOpen" persistent>
      <q-card class="players-panel__dialog">
        <q-card-section class="players-panel__dialog-heading">
          <q-avatar
            :color="pendingDefinition?.color || 'primary'"
            :icon="pendingDefinition?.icon || 'admin_panel_settings'"
            :text-color="actionTextColor(pendingDefinition)" />
          <div>
            <div class="players-panel__dialog-title">
              {{ pendingDefinition?.label || 'Player action' }} {{ pendingTarget }}?
            </div>
          </div>
        </q-card-section>
        <q-card-section class="players-panel__dialog-copy">
          {{ pendingDefinition?.description }} This action is sent immediately through the game
          server's native management protocol.
        </q-card-section>
        <q-card-section v-if="pendingDefinition?.reasonAllowed">
          <q-input
            v-model="actionReason"
            :disable="performing"
            hint="Optional. Control characters are rejected."
            label="Reason"
            maxlength="256"
            outlined
            type="textarea"
            autogrow />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn :disable="performing" flat label="Cancel" no-caps @click="closePlayerAction" />
          <q-btn
            :color="pendingDefinition?.color || 'primary'"
            :icon="pendingDefinition?.icon"
            :label="pendingDefinition?.label || 'Confirm'"
            :loading="performing"
            no-caps
            :text-color="actionTextColor(pendingDefinition)"
            @click="performPlayerAction" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<style scoped>
.players-panel {
  display: grid;
  gap: var(--xy-space-lg);
}

.players-panel__card-copy,
.players-panel__dialog-copy {
  max-width: 70ch;
  color: var(--xy-text-muted);
}

.players-panel__status--online {
  background: var(--xy-success);
  color: var(--xy-text-on-bright);
}

.players-panel__status--offline {
  background: var(--xy-surface-4);
  color: var(--xy-text-primary);
}

.players-panel__card {
  overflow: hidden;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.players-panel__summary,
.players-panel__dialog-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.players-panel__summary-side {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.players-panel__card-title,
.players-panel__dialog-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: 1.1rem;
}

.players-panel__card-copy {
  margin-top: var(--xy-space-xs);
}

.players-panel__banner {
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-2);
}

.players-panel__banner--danger {
  border-color: var(--xy-danger-border);
  background: var(--xy-danger-bg);
}

.players-panel__banner--warning {
  border-color: var(--xy-warning-border);
  background: var(--xy-warning-bg);
}

.players-panel__banner--info {
  border-color: var(--xy-info-border);
  background: var(--xy-info-bg);
}

.players-panel__empty {
  display: grid;
  justify-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-2xl);
  color: var(--xy-text-muted);
  text-align: center;
}

.players-panel__empty-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
}

.players-panel__roster {
  border-top: 1px solid var(--xy-border);
}

.players-panel__player {
  min-height: 72px;
  padding: var(--xy-space-sm) var(--xy-space-md);
}

.players-panel__avatar {
  border: 1px solid var(--xy-accent-border-soft);
  background: var(--xy-accent-muted);
  color: var(--xy-accent-hover);
}

.players-panel__player-name {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
}

.players-panel__player-id {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  overflow-wrap: anywhere;
}

.players-panel__quick-actions {
  display: flex;
  gap: var(--xy-space-xs);
}

.players-panel__manual-form {
  display: grid;
  grid-template-columns: minmax(190px, 0.8fr) minmax(240px, 1.4fr) auto;
  align-items: start;
  gap: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.players-panel__dialog {
  width: min(520px, calc(100vw - 32px));
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.players-panel__dialog-heading {
  justify-content: flex-start;
}

@media (max-width: 800px) {
  .players-panel__manual-form {
    grid-template-columns: 1fr;
  }

  .players-panel__player {
    align-items: flex-start;
  }

  .players-panel__quick-actions {
    flex-direction: column;
    align-items: stretch;
  }
}

@media (max-width: 520px) {
  .players-panel__summary {
    align-items: flex-start;
    flex-direction: column;
  }

  .players-panel__player :deep(.q-item__section--side) {
    padding-left: var(--xy-space-xs);
  }
}
</style>
