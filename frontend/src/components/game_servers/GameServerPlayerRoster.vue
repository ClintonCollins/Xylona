<template>
  <div class="roster">
    <div class="roster__summary">
      <span class="roster__count font-mono">{{ currentPlayerCount }} / {{ maxPlayerCount }}</span>
    </div>

    <template v-if="isOnline && playerListSupported">
      <ul v-if="playerNames.length > 0" aria-label="Online players" class="roster__list">
        <li
          v-for="(name, index) in playerNames"
          :key="`${name}-${index}`"
          :class="{ 'roster__row--new': recentlyJoined.has(name) }"
          class="roster__row">
          <q-icon aria-hidden="true" class="roster__icon" name="person" size="1rem" />
          <span :title="name" class="roster__name">{{ name }}</span>
          <span v-if="rowActions(name).length > 0" class="roster__actions">
            <q-btn
              v-for="definition in rowActions(name)"
              :key="definition.action"
              :aria-label="`${definition.label} ${name}`"
              :color="definition.color"
              dense
              flat
              :icon="definition.icon"
              round
              size="sm"
              @click="openAction(definition, name)">
              <q-tooltip>{{ definition.label }} {{ name }}</q-tooltip>
            </q-btn>
          </span>
        </li>
      </ul>
      <div v-else class="roster__empty">
        {{ currentPlayerCount > 0 ? 'Player names unavailable' : 'No players online' }}
      </div>
      <div v-if="playerNames.length > 0 && unlistedPlayerCount > 0" class="roster__note">
        {{ unlistedPlayerCount }} more {{ unlistedPlayerCount === 1 ? 'player' : 'players' }} not
        reported
      </div>
    </template>
    <div v-else-if="isOnline" class="roster__empty">The roster is not available for this game.</div>
    <div v-else class="roster__empty">The roster appears while the server is online.</div>

    <q-dialog v-model="confirmOpen" persistent>
      <q-card class="roster__dialog">
        <q-card-section class="roster__dialog-heading">
          <q-avatar
            :color="pendingDefinition?.color || 'primary'"
            :icon="pendingDefinition?.icon || 'admin_panel_settings'"
            :text-color="dialogTextColor" />
          <div class="roster__dialog-title">
            {{ pendingDefinition?.label || 'Player action' }} {{ pendingName }}?
          </div>
        </q-card-section>
        <q-card-section class="roster__dialog-copy">
          {{ pendingDefinition?.description }} This action is sent immediately through the game
          server's native management protocol.
        </q-card-section>
        <q-card-section v-if="pendingDefinition?.reasonAllowed">
          <q-input
            v-model="actionReason"
            :disable="performing"
            autogrow
            hint="Optional. Control characters are rejected."
            label="Reason"
            maxlength="256"
            outlined
            type="textarea" />
        </q-card-section>
        <q-card-actions align="right">
          <q-btn :disable="performing" flat label="Cancel" no-caps @click="closeAction" />
          <q-btn
            :color="pendingDefinition?.color || 'primary'"
            :icon="pendingDefinition?.icon"
            :label="pendingDefinition?.label || 'Confirm'"
            :loading="performing"
            no-caps
            :text-color="dialogTextColor"
            @click="performAction" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { notifyConnectError, notifySuccess } from '@/api/notifications'
import { Status } from '@/proto/shared_pb'
import {
  GetGameServerPlayerManagementRequestSchema,
  PerformGameServerPlayerActionRequestSchema,
  type GetGameServerPlayerManagementResponse,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'
import { diffRoster } from '@/pages/game_servers/console-feed'
import {
  getQuickPlayerActionDefinitions,
  type PlayerActionDefinition,
} from '@/pages/game_servers/player-management'

const props = defineProps<{
  gameServerId: string
  isOnline: boolean
  playerNames: string[]
  currentPlayerCount: number
  maxPlayerCount: number
  playerListSupported: boolean
  unlistedPlayerCount: number
  canManagePlayers: boolean
  nativeIdentifiersRequired: boolean
}>()

const management = ref<GetGameServerPlayerManagementResponse | null>(null)
const performing = ref(false)
const confirmOpen = ref(false)
const pendingDefinition = ref<PlayerActionDefinition | null>(null)
const pendingName = ref('')
const pendingId = ref('')
const actionReason = ref('')
const recentlyJoined = ref<Set<string>>(new Set())

let refreshTimer: ReturnType<typeof setTimeout> | undefined
const joinFlashTimers: ReturnType<typeof setTimeout>[] = []

const capabilities = computed(() => management.value?.capabilities)
const quickActionDefinitions = computed(() =>
  getQuickPlayerActionDefinitions(capabilities.value?.supportedActions ?? []),
)
const canPerformActions = computed(
  () =>
    props.canManagePlayers &&
    !props.nativeIdentifiersRequired &&
    Boolean(capabilities.value?.actionsSupported) &&
    management.value?.status === Status.ONLINE &&
    props.isOnline &&
    !performing.value,
)
const dialogTextColor = computed(() =>
  pendingDefinition.value?.color === 'warning' || pendingDefinition.value?.color === 'positive'
    ? 'dark'
    : 'white',
)

function resolvePlayerId(name: string): string {
  const managed = management.value?.players.find((player) => player.name === name)
  return managed?.actionIdentifier ?? ''
}

function rowActions(name: string): PlayerActionDefinition[] {
  if (!canPerformActions.value || resolvePlayerId(name) === '') return []
  return quickActionDefinitions.value
}

async function loadManagement(): Promise<void> {
  if (!props.canManagePlayers || props.nativeIdentifiersRequired || props.gameServerId === '') {
    return
  }
  try {
    management.value = await GetXylonaClient().getGameServerPlayerManagement(
      create(GetGameServerPlayerManagementRequestSchema, { gameServerId: props.gameServerId }),
    )
  } catch {
    // Quick actions are a progressive enhancement on the console page; the
    // roster itself keeps rendering from live query data.
    management.value = null
  }
}

function scheduleManagementRefresh(): void {
  if (refreshTimer !== undefined) clearTimeout(refreshTimer)
  refreshTimer = setTimeout(() => void loadManagement(), 2000)
}

function openAction(definition: PlayerActionDefinition, name: string): void {
  const playerId = resolvePlayerId(name)
  if (!canPerformActions.value || playerId === '') return
  pendingDefinition.value = definition
  pendingName.value = name
  pendingId.value = playerId
  actionReason.value = ''
  confirmOpen.value = true
}

function closeAction(): void {
  if (performing.value) return
  confirmOpen.value = false
  pendingDefinition.value = null
  pendingName.value = ''
  pendingId.value = ''
  actionReason.value = ''
}

async function performAction(): Promise<void> {
  const definition = pendingDefinition.value
  if (definition === null || pendingId.value === '' || performing.value) return

  performing.value = true
  try {
    await GetXylonaClient().performGameServerPlayerAction(
      create(PerformGameServerPlayerActionRequestSchema, {
        gameServerId: props.gameServerId,
        action: definition.action,
        playerId: pendingId.value,
        reason: definition.reasonAllowed ? actionReason.value.trim() || undefined : undefined,
      }),
    )
    notifySuccess(`${definition.label} action sent.`)
    confirmOpen.value = false
    pendingDefinition.value = null
    pendingName.value = ''
    pendingId.value = ''
    actionReason.value = ''
    scheduleManagementRefresh()
  } catch (error) {
    notifyConnectError(error, `Unable to ${definition.label.toLowerCase()} player`)
  } finally {
    performing.value = false
  }
}

watch(
  () => [props.isOnline, props.gameServerId] as const,
  ([online]) => {
    management.value = null
    if (online) void loadManagement()
  },
  { immediate: true },
)

watch(
  () => props.playerNames,
  (next, previous) => {
    const diff = diffRoster(previous ?? [], next)
    if (diff.joined.length === 0 && diff.left.length === 0) return

    for (const name of diff.joined) {
      recentlyJoined.value.add(name)
      joinFlashTimers.push(
        setTimeout(() => {
          recentlyJoined.value.delete(name)
          recentlyJoined.value = new Set(recentlyJoined.value)
        }, 1800),
      )
    }
    recentlyJoined.value = new Set(recentlyJoined.value)

    // New names need ids before quick actions can target them.
    if (diff.joined.length > 0 && props.isOnline) scheduleManagementRefresh()
  },
)

onBeforeUnmount(() => {
  if (refreshTimer !== undefined) clearTimeout(refreshTimer)
  joinFlashTimers.forEach((timer) => clearTimeout(timer))
})
</script>

<style scoped>
.roster {
  display: grid;
  gap: var(--xy-space-xs);
}

.roster__summary {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--xy-space-sm);
}

.roster__count {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  font-weight: 600;
}

.roster__list {
  display: grid;
  gap: var(--xy-space-2xs);
  padding: 0;
  margin: 0;
  list-style: none;
}

.roster__row {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 28px;
  padding: var(--xy-space-2xs) var(--xy-space-xs);
  border-radius: var(--xy-radius-sm);
}

.roster__row:hover {
  background: var(--xy-surface-2);
}

.roster__row--new {
  animation: roster-join-flash calc(1.6s * var(--xy-animation-duration)) ease-out;
}

@keyframes roster-join-flash {
  0% {
    background: var(--xy-success-bg);
  }

  100% {
    background: transparent;
  }
}

.roster__icon {
  flex: 0 0 auto;
  color: var(--xy-accent-hover);
}

.roster__name {
  min-width: 0;
  overflow: hidden;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.roster__actions {
  display: none;
  flex: 0 0 auto;
  margin-left: auto;
}

.roster__row:hover .roster__actions,
.roster__row:focus-within .roster__actions {
  display: inline-flex;
}

.roster__empty,
.roster__note {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.roster__dialog {
  width: min(480px, calc(100vw - 32px));
  border: 1px solid var(--xy-border);
  background: var(--xy-surface-1);
}

.roster__dialog-heading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-md);
}

.roster__dialog-title {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-display);
  font-size: 1.1rem;
}

.roster__dialog-copy {
  max-width: 60ch;
  color: var(--xy-text-muted);
}
</style>
