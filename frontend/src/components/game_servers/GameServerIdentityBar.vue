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
          <game-server-player-list
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
import { useQuasar } from 'quasar'
import { computed, onMounted, ref, watch, type Ref } from 'vue'
import { useRouter } from 'vue-router'

import StatusBadge from '@/components/StatusBadge.vue'
import GameServerPlayerManagementDialog from '@/components/game_servers/GameServerPlayerManagementDialog.vue'
import GameServerPlayerList from '@/components/game_servers/GameServerPlayerList.vue'
import { playerLimit } from '@/components/game_servers/start-args'
import { type GameServer, Status } from '@/proto/shared_pb'
import { useGameServerQueryStatusVersion } from '@/pages/game_servers/useGameServerQueryStatusVersion'
import { useGameServerLifecycle } from '@/pages/game_servers/start-failure'
import { resolveCanonicalVersionDisplay } from '@/pages/game_servers/version-display'
import { isServerStopping } from '@/utils/game-server-stopping'

const props = defineProps<{
  /** Latest snapshot from GetGameServer; live status and players update on top of it. */
  gameServer: GameServer
}>()

const $q = useQuasar()
const router = useRouter()

// Shallow copy so live status and version events never write into the parent's snapshot.
const server = ref({ ...props.gameServer }) as Ref<GameServer>
const gameServerId = computed(() => server.value.id)

// ponytail: a status event that lands while the parent is refetching can be
// overwritten by the older snapshot; the next status event corrects it.
watch(
  () => props.gameServer,
  (next) => {
    server.value = { ...next }
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

const playersOpen = ref(false)
const playerManagementOpen = ref(false)

const isServerOnline = computed(() => server.value.status === Status.ONLINE)
const isSevenDays = computed(() => server.value.gameId === '7_days_to_die')
// Restart and Start state come from the layout, so the badge says it on every tab.
const { lastStartFailure, restarting } = useGameServerLifecycle(gameServerId)
const statusBadgePhase = computed(() => {
  if (restarting.value) return 'restarting'
  // A stop from any view, schedule or tab.
  if (isServerStopping(server.value.id, server.value.status)) return 'stopping'
  if (lastStartFailure.value && server.value.status === Status.OFFLINE) return 'failed'
  return undefined
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

function openPlayerManagement(): void {
  playersOpen.value = false
  if (isSevenDays.value) {
    void router.push(`/game-servers/${server.value.id}/operations`)
    return
  }
  playerManagementOpen.value = true
}

onMounted(() => {
  startQueryStatusVersionLifecycle()
  void queryGameServer()
})
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
    grid-template-columns: minmax(0, 1fr) auto auto;
    gap: var(--xy-space-sm);
    padding: var(--xy-space-sm) var(--xy-space-base);
  }

  /* Phones drop the game/version line: name, status and players on one row. */
  .identity-bar-detail {
    display: none;
  }

  .identity-bar-players {
    min-height: 44px;
  }
}
</style>
