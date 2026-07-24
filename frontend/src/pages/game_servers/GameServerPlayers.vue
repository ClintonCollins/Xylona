<script lang="ts" setup>
import { computed, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { create } from '@bufbuild/protobuf'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import PageHeader from '@/components/shared/PageHeader.vue'
import type { GameServerPlayer } from '@/proto/shared_pb'
import { Status } from '@/proto/shared_pb'
import {
  GameServerPlayerAction,
  GetGameServerPlayerManagementRequestSchema,
  PerformGameServerPlayerActionRequestSchema,
  type GetGameServerPlayerManagementResponse,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'
import {
  getPlayerActionDefinition,
  getQuickPlayerActionDefinitions,
  getSupportedPlayerActionDefinitions,
  type PlayerActionDefinition,
} from './player-management'

interface PendingPlayerAction {
  action: GameServerPlayerAction
  playerId: string
  playerName: string
}

const route = useRoute()
const gameServerId = computed(() => String(route.params.id ?? ''))
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

async function loadPlayerManagement(): Promise<void> {
  if (gameServerId.value === '') {
    return
  }
  loading.value = true
  loadError.value = false
  try {
    management.value = await GetXylonaClient().getGameServerPlayerManagement(
      create(GetGameServerPlayerManagementRequestSchema, {
        gameServerId: gameServerId.value,
      }),
    )
  } catch (error) {
    management.value = null
    loadError.value = true
    notifyConnectError(error, 'Unable to load player management')
  } finally {
    loading.value = false
  }
}

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
        gameServerId: gameServerId.value,
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

watch(gameServerId, loadPlayerManagement, { immediate: true })
</script>

<template>
  <div class="players-page xy-page-content">
    <page-header
      subtitle="Inspect the live roster and use the game server's native administration controls."
      title="Players">
      <template #actions>
        <q-btn
          :disable="loading || performing"
          :loading="loading"
          color="primary"
          icon="refresh"
          label="Refresh"
          no-caps
          outline
          @click="loadPlayerManagement" />
      </template>
    </page-header>

    <q-banner v-if="loadError" class="players-page__banner players-page__banner--danger" rounded>
      <template #avatar><q-icon color="negative" name="cloud_off" /></template>
      Player management could not be loaded from the server's node.
      <template #action>
        <q-btn color="negative" flat label="Retry" no-caps @click="loadPlayerManagement" />
      </template>
    </q-banner>

    <template v-else-if="loading">
      <q-card class="players-page__card">
        <q-card-section>
          <q-skeleton class="q-mb-md" height="28px" type="rect" width="180px" />
          <q-skeleton class="q-mb-sm" type="text" />
          <q-skeleton class="q-mb-sm" type="text" />
          <q-skeleton type="text" width="70%" />
        </q-card-section>
      </q-card>
    </template>

    <template v-else-if="management !== null">
      <q-card class="players-page__card">
        <q-card-section class="players-page__summary">
          <div>
            <div class="players-page__card-title">Live roster</div>
            <div class="players-page__card-copy">
              {{ players.length }} {{ players.length === 1 ? 'player' : 'players' }} reported
            </div>
          </div>
          <q-chip
            :class="isOnline ? 'players-page__status--online' : 'players-page__status--offline'"
            :icon="isOnline ? 'radio_button_checked' : 'power_off'"
            :label="isOnline ? 'Online' : 'Offline'" />
        </q-card-section>

        <q-separator />

        <q-card-section v-if="!isOnline">
          <q-banner class="players-page__banner players-page__banner--warning" dense rounded>
            <template #avatar><q-icon color="warning" name="power_settings_new" /></template>
            Start the game server to query its roster or perform player actions.
          </q-banner>
        </q-card-section>
        <q-card-section v-else-if="!capabilities?.actionsSupported">
          <q-banner class="players-page__banner players-page__banner--info" dense rounded>
            <template #avatar><q-icon color="info" name="visibility" /></template>
            {{
              capabilities?.unavailableReason || 'Player actions are not available for this game.'
            }}
          </q-banner>
        </q-card-section>

        <q-card-section v-if="players.length === 0" class="players-page__empty">
          <q-icon name="group_off" size="42px" />
          <div class="players-page__empty-title">No players reported</div>
          <div>
            {{
              isOnline
                ? 'The server query returned an empty roster.'
                : 'The roster will appear after the server starts.'
            }}
          </div>
        </q-card-section>

        <q-list v-else class="players-page__roster" separator>
          <q-item
            v-for="player in players"
            :key="player.id || player.name"
            class="players-page__player">
            <q-item-section avatar>
              <q-avatar class="players-page__avatar" icon="person" />
            </q-item-section>
            <q-item-section>
              <q-item-label class="players-page__player-name">{{ player.name }}</q-item-label>
              <q-item-label
                v-if="playerSecondaryText(player)"
                caption
                class="players-page__player-id">
                {{ playerSecondaryText(player) }}
              </q-item-label>
            </q-item-section>
            <q-item-section v-if="player.id && quickActionDefinitions.length > 0" side>
              <div class="players-page__quick-actions">
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

      <q-card v-if="supportedActionDefinitions.length > 0" class="players-page__card">
        <q-card-section>
          <div class="players-page__card-title">Manage by {{ identifierLabel.toLowerCase() }}</div>
          <div class="players-page__card-copy">
            Use this for offline players, unbans, and allowlist changes that are not represented in
            the live roster.
          </div>
        </q-card-section>
        <q-card-section class="players-page__manual-form">
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
      <q-card class="players-page__dialog">
        <q-card-section class="players-page__dialog-heading">
          <q-avatar
            :color="pendingDefinition?.color || 'primary'"
            :icon="pendingDefinition?.icon || 'admin_panel_settings'"
            :text-color="actionTextColor(pendingDefinition)" />
          <div>
            <div class="players-page__dialog-title">
              {{ pendingDefinition?.label || 'Player action' }} {{ pendingTarget }}?
            </div>
          </div>
        </q-card-section>
        <q-card-section class="players-page__dialog-copy">
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
.players-page {
  display: grid;
  gap: var(--xy-space-lg);
}

.players-page__card-copy,
.players-page__dialog-copy {
  max-width: 70ch;
  color: var(--xy-text-muted);
}

.players-page__status--online {
  background: var(--xy-success);
  color: var(--xy-text-on-bright);
}

.players-page__status--offline {
  background: var(--xy-surface-4);
  color: var(--xy-text-primary);
}

.players-page__card {
  overflow: hidden;
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.players-page__summary,
.players-page__dialog-heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.players-page__card-title,
.players-page__dialog-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: 1.1rem;
}

.players-page__card-copy {
  margin-top: var(--xy-space-xs);
}

.players-page__banner {
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-2);
}

.players-page__banner--danger {
  border-color: var(--xy-danger-border);
  background: var(--xy-danger-bg);
}

.players-page__banner--warning {
  border-color: var(--xy-warning-border);
  background: var(--xy-warning-bg);
}

.players-page__banner--info {
  border-color: var(--xy-info-border);
  background: var(--xy-info-bg);
}

.players-page__empty {
  display: grid;
  justify-items: center;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-2xl);
  color: var(--xy-text-muted);
  text-align: center;
}

.players-page__empty-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
}

.players-page__roster {
  border-top: 1px solid var(--xy-border);
}

.players-page__player {
  min-height: 72px;
  padding: var(--xy-space-sm) var(--xy-space-md);
}

.players-page__avatar {
  border: 1px solid var(--xy-accent-border-soft);
  background: var(--xy-accent-muted);
  color: var(--xy-accent-hover);
}

.players-page__player-name {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
}

.players-page__player-id {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  overflow-wrap: anywhere;
}

.players-page__quick-actions {
  display: flex;
  gap: var(--xy-space-xs);
}

.players-page__manual-form {
  display: grid;
  grid-template-columns: minmax(190px, 0.8fr) minmax(240px, 1.4fr) auto;
  align-items: start;
  gap: var(--xy-space-md);
  border-top: 1px solid var(--xy-border);
}

.players-page__dialog {
  width: min(520px, calc(100vw - 32px));
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.players-page__dialog-heading {
  justify-content: flex-start;
}

@media (max-width: 800px) {
  .players-page__manual-form {
    grid-template-columns: 1fr;
  }

  .players-page__player {
    align-items: flex-start;
  }

  .players-page__quick-actions {
    flex-direction: column;
    align-items: stretch;
  }
}

@media (max-width: 520px) {
  .players-page__summary {
    align-items: flex-start;
    flex-direction: column;
  }

  .players-page__player :deep(.q-item__section--side) {
    padding-left: var(--xy-space-xs);
  }
}
</style>
