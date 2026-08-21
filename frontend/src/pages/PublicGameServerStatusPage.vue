<script lang="ts" setup>
import { create, fromJsonString } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { Code, ConnectError } from '@connectrpc/connect'
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { useRoute } from 'vue-router'

import { Status } from '@/proto/shared_pb'
import {
  GameServerStatusPageRosterState,
  GetPublicGameServerStatusPageRequestSchema,
  type PublicGameServerStatus,
  type PublicGameServerStatusPage,
  PublicGameServerStatusPageSchema,
} from '@/proto/xylona_pb'
import { formatMetricAge } from '@/pages/game_servers/metrics-format'
import { GetXylonaClient } from '@/utils/shared'

const route = useRoute()
const page = shallowRef<PublicGameServerStatusPage | null>(null)
const loading = ref(true)
const initialError = ref(false)
const unavailable = ref(false)
const reconnecting = ref(false)
const copiedServerID = ref('')
const expandedServerIDs = ref(new Set<string>())
const now = ref(Date.now())
let eventSource: EventSource | undefined
let pollTimer: ReturnType<typeof setInterval> | undefined
let ageTimer: ReturnType<typeof setInterval> | undefined

const identifier = computed(() => String(route.params['identifier'] ?? ''))
const onlineCount = computed(
  () => page.value?.servers.filter((server) => server.status === Status.ONLINE).length ?? 0,
)
const knownPlayerCount = computed(
  () => page.value?.servers.reduce((sum, server) => sum + (server.currentPlayerCount ?? 0), 0) ?? 0,
)
const allPlayerCountsKnown = computed(
  () => page.value?.servers.every((server) => server.currentPlayerCount !== undefined) ?? false,
)
const maximumPlayerCount = computed(
  () => page.value?.servers.reduce((sum, server) => sum + server.maxPlayerCount, 0) ?? 0,
)
const reconnectSnapshotLabel = computed(() => {
  let latest: number | null = null
  for (const server of page.value?.servers ?? []) {
    if (!server.observedAt) continue
    const observedAt = timestampDate(server.observedAt).getTime()
    if (Number.isFinite(observedAt) && (latest === null || observedAt > latest)) latest = observedAt
  }
  return latest === null
    ? 'the last known update'
    : `data observed ${formatMetricAge(latest, now.value)}`
})

async function loadSnapshot(): Promise<boolean> {
  try {
    const response = await GetXylonaClient().getPublicGameServerStatusPage(
      create(GetPublicGameServerStatusPageRequestSchema, {
        publicIdentifier: identifier.value,
      }),
    )
    if (!response.page) throw new Error('The status response was empty.')
    page.value = response.page
    initialError.value = false
    unavailable.value = false
    return true
  } catch (error: unknown) {
    const connectError = ConnectError.from(error)
    if (connectError.code === Code.NotFound) {
      page.value = null
      unavailable.value = true
      initialError.value = false
      stopPolling()
      eventSource?.close()
      eventSource = undefined
    } else if (page.value === null) {
      initialError.value = true
    } else {
      reconnecting.value = true
    }
    return false
  } finally {
    loading.value = false
  }
}

function connectEvents() {
  eventSource?.close()
  eventSource = new EventSource(
    `/api/public/status-pages/${encodeURIComponent(identifier.value)}/events`,
  )
  eventSource.addEventListener('snapshot', (event) => {
    if (!(event instanceof MessageEvent) || typeof event.data !== 'string') return
    try {
      page.value = fromJsonString(PublicGameServerStatusPageSchema, event.data)
      unavailable.value = false
      reconnecting.value = false
      stopPolling()
    } catch (error: unknown) {
      console.error('Invalid status page snapshot', error)
    }
  })
  eventSource.onerror = () => {
    reconnecting.value = page.value !== null
    startPolling()
  }
}

function startPolling() {
  if (pollTimer !== undefined || unavailable.value) return
  pollTimer = setInterval(() => void loadSnapshot(), 15_000)
}

function stopPolling() {
  if (pollTimer === undefined) return
  clearInterval(pollTimer)
  pollTimer = undefined
}

async function retry() {
  loading.value = true
  if (await loadSnapshot()) connectEvents()
}

async function copyAddress(server: PublicGameServerStatus) {
  await navigator.clipboard.writeText(server.connectionAddress)
  copiedServerID.value = server.id
  window.setTimeout(() => {
    if (copiedServerID.value === server.id) copiedServerID.value = ''
  }, 1500)
}

function toggleRoster(serverID: string) {
  const next = new Set(expandedServerIDs.value)
  if (next.has(serverID)) next.delete(serverID)
  else next.add(serverID)
  expandedServerIDs.value = next
}

function statusLabel(status: Status): string {
  switch (status) {
    case Status.ONLINE:
      return 'Online'
    case Status.OFFLINE:
      return 'Offline'
    case Status.INSTALLING:
      return 'Installing'
    case Status.UPDATING:
      return 'Updating'
    case Status.PRE_START:
      return 'Starting'
    default:
      return 'Unavailable'
  }
}

function statusClass(status: Status): string {
  if (status === Status.ONLINE) return 'public-status--online'
  if (status === Status.OFFLINE) return 'public-status--offline'
  return 'public-status--pending'
}

function playerLabel(server: PublicGameServerStatus): string {
  const current = server.currentPlayerCount ?? 'Unavailable'
  return `${current} / ${server.maxPlayerCount}`
}

function rosterLabel(server: PublicGameServerStatus): string {
  if (server.rosterState === GameServerStatusPageRosterState.UNSUPPORTED) {
    return 'Player names are not supported by this game.'
  }
  if (server.rosterState === GameServerStatusPageRosterState.UNAVAILABLE) {
    return 'Player names are temporarily unavailable.'
  }
  if (server.playerNames.length === 0) return 'No players are online.'
  return ''
}

onMounted(async () => {
  ageTimer = setInterval(() => (now.value = Date.now()), 30_000)
  if (await loadSnapshot()) connectEvents()
})

onUnmounted(() => {
  eventSource?.close()
  stopPolling()
  if (ageTimer !== undefined) clearInterval(ageTimer)
})
</script>

<template>
  <main class="public-status-page">
    <div class="public-status-page__inner">
      <header class="public-status-header">
        <div class="public-status-header__brand">XYLONA</div>
        <div>
          <h1>{{ page?.title || 'Game server status' }}</h1>
          <p>Live game server availability and player counts</p>
        </div>
        <span v-if="page" class="public-status public-status--online">Live</span>
      </header>

      <div v-if="reconnecting && page" class="public-status-notice" role="status">
        Live updates were interrupted. Showing {{ reconnectSnapshotLabel }} while the connection
        recovers.
      </div>

      <div v-if="loading && !page" class="public-status-state" role="status">
        <q-spinner color="primary" size="42px" />
        <h2>Loading server status</h2>
      </div>
      <div v-else-if="unavailable" class="public-status-state">
        <q-icon name="link_off" size="48px" />
        <h2>This status page is not available</h2>
        <p>The link may be incomplete, disabled, or no longer current.</p>
      </div>
      <div v-else-if="initialError" class="public-status-state" role="alert">
        <q-icon name="cloud_off" size="48px" />
        <h2>Server status could not be loaded</h2>
        <p>Check your connection and try again.</p>
        <q-btn color="primary" label="Try again" @click="retry" />
      </div>
      <template v-else-if="page">
        <div
          v-if="page.servers.length > 0"
          class="public-status-summary"
          aria-label="Fleet summary">
          <span
            ><strong>{{ page.servers.length }}</strong> servers</span
          >
          <span
            ><strong>{{ onlineCount }}</strong> online</span
          >
          <span v-if="maximumPlayerCount > 0 && allPlayerCountsKnown">
            <strong>{{ knownPlayerCount }} / {{ maximumPlayerCount }}</strong> players
          </span>
        </div>

        <div v-if="page.servers.length === 0" class="public-status-state">
          <q-icon name="dns" size="48px" />
          <h2>No game servers are published yet</h2>
          <p>This page is active. Its owner has not added any game servers.</p>
        </div>
        <section v-else class="public-server-stack" aria-label="Game server status">
          <article v-for="server in page.servers" :key="server.id" class="public-server-row">
            <div class="public-server-row__summary">
              <div class="public-server-row__identity">
                <h2>{{ server.name }}</h2>
                <p>{{ server.gameName }}</p>
                <span class="public-status" :class="statusClass(server.status)">
                  {{ statusLabel(server.status) }}
                </span>
              </div>
              <div>
                <span class="public-server-row__label">Connect</span>
                <div class="public-address-line">
                  <span class="public-server-row__value">{{ server.connectionAddress }}</span>
                  <q-btn
                    :aria-label="`Copy ${server.name} connection address`"
                    dense
                    flat
                    :icon="copiedServerID === server.id ? 'check' : 'content_copy'"
                    round
                    @click="copyAddress(server)" />
                </div>
              </div>
              <div>
                <span class="public-server-row__label">Players</span>
                <span class="public-server-row__value">{{ playerLabel(server) }}</span>
              </div>
              <q-btn
                v-if="server.rosterState !== GameServerStatusPageRosterState.UNSPECIFIED"
                :aria-expanded="expandedServerIDs.has(server.id)"
                :aria-label="`${expandedServerIDs.has(server.id) ? 'Hide' : 'Show'} ${server.name} player roster`"
                dense
                flat
                :icon="expandedServerIDs.has(server.id) ? 'expand_less' : 'expand_more'"
                round
                @click="toggleRoster(server.id)" />
            </div>
            <div v-if="expandedServerIDs.has(server.id)" class="public-server-row__roster">
              <span class="public-server-row__label">Player roster</span>
              <p v-if="rosterLabel(server)">{{ rosterLabel(server) }}</p>
              <ul v-else class="public-roster-list">
                <li v-for="playerName in server.playerNames" :key="playerName">{{ playerName }}</li>
              </ul>
            </div>
          </article>
        </section>
      </template>

      <footer class="public-status-footer">Status provided by Xylona</footer>
    </div>
  </main>
</template>

<style scoped>
.public-status-page {
  min-width: 320px;
  min-height: 100dvh;
  color: var(--xy-text-primary);
  background: var(--xy-base);
  font-family: var(--xy-font-body);
}

.public-status-page__inner {
  display: grid;
  width: min(1180px, 100%);
  min-height: 100dvh;
  gap: var(--xy-space-lg);
  margin: 0 auto;
  padding: var(--xy-space-xl) var(--xy-space-lg);
}

.public-status-header {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-lg);
  padding-bottom: var(--xy-space-lg);
  border-bottom: 1px solid var(--xy-border);
}

.public-status-header__brand {
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-lg);
  letter-spacing: 0.08em;
}

.public-status-header h1,
.public-status-state h2,
.public-server-row h2 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-weight: 600;
}

.public-status-header h1 {
  font-size: clamp(var(--xy-font-size-xl), 4vw, var(--xy-font-size-2xl));
  line-height: var(--xy-line-height-tight);
}

.public-status-header p,
.public-server-row p,
.public-status-state p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
}

.public-status,
.public-status-summary,
.public-address-line {
  display: flex;
  align-items: center;
}

.public-status {
  width: max-content;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-base);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
}

.public-status::before {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentcolor;
  content: '';
}

.public-status--online {
  color: var(--xy-success-text-soft);
  background: var(--xy-success-bg);
}

.public-status--offline {
  color: var(--xy-danger-hover);
  background: var(--xy-danger-bg);
}

.public-status--pending {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg);
}

.public-status-summary {
  flex-wrap: wrap;
  gap: var(--xy-space-base) var(--xy-space-lg);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.public-status-summary strong,
.public-server-row__value {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}

.public-status-notice {
  padding: var(--xy-space-base) var(--xy-space-md);
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg-faint);
  border: 1px solid var(--xy-warning-border);
  border-radius: var(--xy-radius-md);
}

.public-server-stack {
  display: grid;
  gap: var(--xy-space-base);
}

.public-server-row {
  overflow: hidden;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.public-server-row__summary {
  display: grid;
  grid-template-columns: minmax(180px, 1.3fr) minmax(170px, 1fr) minmax(130px, 0.65fr) auto;
  align-items: center;
  gap: var(--xy-space-md);
  min-height: 92px;
  padding: var(--xy-space-md);
}

.public-server-row__identity {
  display: grid;
  gap: var(--xy-space-sm);
}

.public-server-row__identity h2 {
  font-size: var(--xy-font-size-base);
}

.public-server-row__identity p,
.public-server-row__label {
  margin: 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.public-server-row__value {
  display: block;
  margin-top: var(--xy-space-xs);
  overflow-wrap: anywhere;
  font-size: var(--xy-font-size-sm);
}

.public-address-line {
  gap: var(--xy-space-xs);
}

.public-server-row__roster {
  display: grid;
  grid-template-columns: 150px minmax(0, 1fr);
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  background: var(--xy-surface-0);
  border-top: 1px solid var(--xy-border);
  animation: public-roster-reveal 180ms var(--xy-ease-standard);
}

@keyframes public-roster-reveal {
  from {
    opacity: 0.6;
    transform: translateY(-4px);
  }
}

.public-roster-list {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--xy-space-sm) var(--xy-space-md);
  margin: 0;
  padding: 0;
  list-style: none;
}

.public-roster-list li {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.public-status-state {
  display: grid;
  min-height: 360px;
  place-content: center;
  justify-items: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-2xl) var(--xy-space-lg);
  text-align: center;
}

.public-status-state p {
  max-width: 55ch;
}

.public-status-footer {
  align-self: end;
  padding-top: var(--xy-space-sm);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  text-align: center;
}

.public-status-page :deep(:focus-visible) {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: 3px;
}

.public-status-page ::selection {
  color: var(--xy-text-on-dark);
  background: var(--xy-primary);
}

@media (max-width: 1023px) {
  .public-server-row__summary {
    grid-template-columns: minmax(0, 1fr) minmax(170px, 1fr) auto;
  }

  .public-server-row__summary > :nth-child(3) {
    grid-column: 2;
  }
}

@media (max-width: 599px) {
  .public-status-page__inner {
    gap: var(--xy-space-md);
    padding: var(--xy-space-md) var(--xy-space-sm);
  }

  .public-status-header {
    grid-template-columns: 1fr auto;
    gap: var(--xy-space-sm);
  }

  .public-status-header__brand {
    grid-column: 1 / -1;
  }

  .public-server-row__summary {
    grid-template-columns: 1fr auto;
  }

  .public-server-row__summary > :nth-child(2),
  .public-server-row__summary > :nth-child(3) {
    grid-column: 1 / -1;
  }

  .public-server-row__roster {
    grid-template-columns: 1fr;
  }

  .public-roster-list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (prefers-reduced-motion: reduce) {
  .public-server-row__roster {
    animation: none;
  }
}
</style>
