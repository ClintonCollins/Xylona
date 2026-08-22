<script lang="ts" setup>
import { create, fromJsonString } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { Code, ConnectError } from '@connectrpc/connect'
import { copyToClipboard, useQuasar } from 'quasar'
import { computed, onMounted, onUnmounted, ref, shallowRef, watch } from 'vue'
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
const $q = useQuasar()
const page = shallowRef<PublicGameServerStatusPage | null>(null)
const loading = ref(true)
const initialError = ref(false)
const unavailable = ref(false)
const reconnecting = ref(false)
const copiedServerID = ref('')
const copyAnnouncement = ref('')
const expandedServerIDs = ref(new Set<string>())
const now = ref(Date.now())
let eventSource: EventSource | undefined
let pollTimer: ReturnType<typeof setInterval> | undefined
let ageTimer: ReturnType<typeof setInterval> | undefined
let copyResetTimer: ReturnType<typeof setTimeout> | undefined
const initialDocumentTitle = document.title

const identifier = computed(() => String(route.params['identifier'] ?? ''))
const onlineCount = computed(
  () => page.value?.servers.filter((server) => server.status === Status.ONLINE).length ?? 0,
)
const latestObservedAt = computed(() => {
  let latest: number | null = null
  for (const server of page.value?.servers ?? []) {
    if (!server.observedAt) continue
    const observedAt = timestampDate(server.observedAt).getTime()
    if (Number.isFinite(observedAt) && (latest === null || observedAt > latest)) latest = observedAt
  }
  return latest
})
const fleetFreshnessLabel = computed(() => {
  const state = reconnecting.value ? 'Last known' : 'Live'
  return latestObservedAt.value === null
    ? `${state} · awaiting player data`
    : `${state} · updated ${relativeAge(latestObservedAt.value)}`
})
const reconnectSnapshotLabel = computed(() =>
  latestObservedAt.value === null
    ? 'the last known update'
    : `data observed ${relativeAge(latestObservedAt.value)}`,
)

watch(page, (currentPage) => {
  document.title = currentPage
    ? `${currentPage.title || 'Game server status'} · Xylona`
    : initialDocumentTitle
})

function relativeAge(timestampMs: number): string {
  const age = formatMetricAge(timestampMs, now.value)
  return age === 'Just now' ? 'just now' : age
}

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
  try {
    await copyToClipboard(server.connectionAddress)
    copiedServerID.value = server.id
    copyAnnouncement.value = `${server.name} connection address copied.`
    if (copyResetTimer !== undefined) clearTimeout(copyResetTimer)
    copyResetTimer = window.setTimeout(() => {
      copiedServerID.value = ''
      copyAnnouncement.value = ''
      copyResetTimer = undefined
    }, 1500)
  } catch {
    copiedServerID.value = ''
    copyAnnouncement.value = ''
    $q.notify({ type: 'negative', message: 'Could not copy the connection address.' })
  }
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
  if (server.currentPlayerCount !== undefined) {
    return `${server.currentPlayerCount} / ${server.maxPlayerCount}`
  }
  return server.status === Status.OFFLINE ? 'Unavailable while offline' : 'Player count unavailable'
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
  if (copyResetTimer !== undefined) clearTimeout(copyResetTimer)
  document.title = initialDocumentTitle
})
</script>

<template>
  <main class="public-status-page">
    <div class="public-status-page__inner">
      <header class="public-status-header">
        <div class="public-status-header__brand">
          Xylona
          <span>Public status</span>
        </div>
        <div>
          <h1>{{ page?.title || 'Game server status' }}</h1>
          <p>Live game server availability and player counts</p>
        </div>
      </header>
      <div class="xy-visually-hidden" role="status" aria-live="polite" aria-atomic="true">
        {{ copyAnnouncement }}
      </div>

      <div v-if="reconnecting && page" class="public-status-notice" role="status">
        Live updates were interrupted. Showing {{ reconnectSnapshotLabel }} while the connection
        recovers.
      </div>

      <div v-if="loading && !page" class="public-status-state" role="status">
        <q-spinner color="primary" size="42px" />
        <h2>Loading server status</h2>
        <p>Connecting to the live status feed.</p>
      </div>
      <div v-else-if="unavailable" class="public-status-state public-status-state--fault">
        <q-icon name="link_off" size="48px" />
        <h2>This status page is not available</h2>
        <p>The link may be incomplete, disabled, or no longer current.</p>
      </div>
      <div
        v-else-if="initialError"
        class="public-status-state public-status-state--fault"
        role="alert">
        <q-icon name="cloud_off" size="48px" />
        <h2>Server status could not be loaded</h2>
        <p>Check your connection and try again.</p>
        <q-btn color="primary" label="Try again" @click="retry" />
      </div>
      <template v-else-if="page">
        <div
          v-if="page.servers.length > 0"
          class="public-status-summary"
          aria-label="Server status summary">
          <div class="public-status-summary__availability">
            <q-icon name="dns" size="24px" aria-hidden="true" />
            <div>
              <span class="public-status-summary__title">
                {{ onlineCount }} of {{ page.servers.length }} servers online
              </span>
              <span class="public-status-summary__caption">
                Availability across published game servers
              </span>
            </div>
          </div>
          <span class="public-status-summary__freshness" :class="{ 'is-stale': reconnecting }">
            {{ fleetFreshnessLabel }}
          </span>
        </div>

        <div
          v-if="page.servers.length === 0"
          class="public-status-state public-status-state--empty">
          <q-icon name="dns" size="48px" />
          <h2>No game servers are published yet</h2>
          <p>This page is active. Its owner has not added any game servers.</p>
        </div>
        <section v-else class="public-server-stack" aria-label="Game server status">
          <article
            v-for="server in page.servers"
            :key="server.id"
            class="public-server-row"
            :class="{
              'is-online': server.status === Status.ONLINE,
              'is-offline': server.status === Status.OFFLINE,
              'is-pending': server.status !== Status.ONLINE && server.status !== Status.OFFLINE,
            }">
            <div class="public-server-row__summary">
              <div class="public-server-row__identity">
                <h2>{{ server.name }}</h2>
                <p>{{ server.gameName }}</p>
                <span
                  class="public-status"
                  :class="statusClass(server.status)"
                  role="status"
                  :aria-label="`${server.name} status: ${statusLabel(server.status)}`">
                  {{ statusLabel(server.status) }}
                </span>
              </div>
              <div>
                <span class="public-server-row__label">Connect</span>
                <div class="public-address-line">
                  <span class="public-server-row__value">{{ server.connectionAddress }}</span>
                  <q-btn
                    class="public-server-row__action public-server-row__copy-action"
                    :class="{ 'is-copied': copiedServerID === server.id }"
                    :aria-label="
                      copiedServerID === server.id
                        ? `${server.name} connection address copied`
                        : `Copy ${server.name} connection address`
                    "
                    dense
                    flat
                    :icon="copiedServerID === server.id ? 'check' : 'content_copy'"
                    :label="copiedServerID === server.id ? 'Copied' : 'Copy'"
                    no-caps
                    @click="copyAddress(server)" />
                </div>
              </div>
              <div class="public-server-row__metrics">
                <div>
                  <span class="public-server-row__label">Players</span>
                  <span
                    class="public-server-row__value"
                    role="status"
                    :aria-label="`${server.name} players: ${playerLabel(server)}`">
                    {{ playerLabel(server) }}
                  </span>
                </div>
                <q-btn
                  v-if="server.rosterState !== GameServerStatusPageRosterState.UNSPECIFIED"
                  class="public-server-row__action public-server-row__roster-action"
                  :aria-expanded="expandedServerIDs.has(server.id)"
                  :aria-label="`${expandedServerIDs.has(server.id) ? 'Hide' : 'Show'} ${server.name} player roster`"
                  flat
                  :icon-right="expandedServerIDs.has(server.id) ? 'expand_less' : 'expand_more'"
                  label="Players"
                  no-caps
                  @click="toggleRoster(server.id)" />
              </div>
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
  min-height: 100dvh;
  color: var(--xy-text-primary);
  background-color: var(--xy-base);
  background-image: var(--xy-surface-gradient-subtle);
  font-family: var(--xy-font-body);
}

.public-status-page__inner {
  display: flex;
  flex-direction: column;
  width: min(100%, clamp(1180px, 50vw, 1920px));
  min-height: 100dvh;
  gap: var(--xy-space-lg);
  margin: 0 auto;
  padding: var(--xy-space-xl) var(--xy-space-lg);
}

.public-status-header {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr);
  align-items: center;
  gap: var(--xy-space-lg);
  padding-bottom: var(--xy-space-lg);
  border-bottom: 1px solid var(--xy-border);
}

.public-status-header__brand {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--xy-space-sm);
  color: var(--xy-accent);
  font-family: var(--xy-font-brand);
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
}

.public-status-header__brand span {
  padding: var(--xy-space-2xs) var(--xy-space-sm);
  color: var(--xy-accent-hover);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-pill);
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-2xs);
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
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
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-md) var(--xy-space-xl);
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
  background: linear-gradient(
    115deg,
    var(--xy-accent-bg),
    var(--xy-primary-bg-subtle) 52%,
    var(--xy-surface-0)
  );
  border: 1px solid var(--xy-accent-border-soft);
  border-radius: var(--xy-radius-lg);
}

.public-server-row__value {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-mono);
  font-weight: 400;
  font-variant-numeric: tabular-nums;
}

.public-status-summary__availability {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
}

.public-status-summary__availability > .q-icon {
  flex: 0 0 auto;
  padding: var(--xy-space-sm);
  color: var(--xy-accent-hover);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-md);
}

.public-status-summary__availability > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.public-status-summary__title {
  display: block;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  font-weight: 600;
}

.public-status-summary__caption {
  display: block;
  color: color-mix(in srgb, var(--xy-text-primary) 72%, var(--xy-accent) 28%);
  font-size: var(--xy-font-size-xs);
}

.public-status-summary__freshness {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-sm);
  width: max-content;
  padding: var(--xy-space-xs) var(--xy-space-base);
  color: var(--xy-success-text-soft);
  background: var(--xy-success-bg-faint);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  animation: public-live-signal 2.8s ease-in-out infinite;
}

.public-status-summary__freshness::before {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: currentcolor;
  content: '';
}

.public-status-summary__freshness.is-stale {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg-faint);
  animation: none;
}

@keyframes public-live-signal {
  50% {
    background: var(--xy-success-bg);
  }
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

.public-server-row.is-online {
  background: color-mix(in srgb, var(--xy-success) 6%, var(--xy-surface-1));
  border-color: var(--xy-success-border-soft);
}

.public-server-row.is-offline {
  background: color-mix(in srgb, var(--xy-danger) 3%, var(--xy-surface-1));
  border-color: color-mix(in srgb, var(--xy-danger) 18%, var(--xy-border));
}

.public-server-row.is-pending {
  background: color-mix(in srgb, var(--xy-warning) 4%, var(--xy-surface-1));
  border-color: var(--xy-warning-border-soft);
}

.public-server-row__summary {
  display: grid;
  grid-template-columns: minmax(180px, 1.3fr) minmax(170px, 1fr) minmax(230px, 0.8fr);
  align-items: center;
  gap: var(--xy-space-base) var(--xy-space-md);
  min-height: 92px;
  padding: var(--xy-space-base) var(--xy-space-md);
}

.public-server-row__identity {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-xs) var(--xy-space-base);
}

.public-server-row__identity h2 {
  grid-column: 1;
  grid-row: 1;
  font-size: var(--xy-font-size-base);
  line-height: var(--xy-line-height-tight);
}

.public-server-row__identity p {
  grid-column: 1;
  grid-row: 2;
}

.public-server-row__identity .public-status {
  grid-column: 2;
  grid-row: 1 / span 2;
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

.public-server-row__metrics {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-base);
}

.public-server-row__action {
  min-width: 44px;
  min-height: 44px;
}

.public-server-row__copy-action {
  min-width: 86px;
  color: var(--xy-text-secondary);
  border-radius: var(--xy-radius-md);
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.public-server-row__copy-action:hover {
  color: var(--xy-primary-hover);
  background: var(--xy-primary-muted);
}

.public-server-row__copy-action.is-copied {
  color: var(--xy-success-text-soft);
  background: var(--xy-success-bg);
}

.public-server-row__roster-action {
  color: var(--xy-text-secondary);
  border-radius: var(--xy-radius-md);
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.public-server-row__roster-action:hover,
.public-server-row__roster-action[aria-expanded='true'] {
  color: var(--xy-accent-hover);
  background: var(--xy-accent-muted);
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
  padding: var(--xy-space-xs) var(--xy-space-base);
  color: var(--xy-text-primary);
  background: var(--xy-success-bg-faint);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.public-roster-list li::before {
  display: inline-block;
  width: 6px;
  height: 6px;
  margin-right: var(--xy-space-sm);
  background: var(--xy-success);
  border-radius: 50%;
  content: '';
  vertical-align: middle;
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

.public-status-state--fault > .q-icon {
  color: var(--xy-danger-hover);
}

.public-status-state--empty > .q-icon {
  color: var(--xy-accent-hover);
}

.public-status-footer {
  margin-top: auto;
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
    grid-template-columns: minmax(0, 1fr) minmax(170px, 1fr);
  }

  .public-server-row__metrics {
    grid-column: 1 / -1;
  }
}

@media (max-width: 599px) {
  .public-status-page__inner {
    gap: var(--xy-space-md);
    padding: var(--xy-space-md) var(--xy-space-sm);
  }

  .public-status-header {
    grid-template-columns: 1fr;
    gap: var(--xy-space-sm);
  }

  .public-status-header__brand {
    grid-column: 1 / -1;
  }

  .public-status-summary {
    grid-template-columns: 1fr;
    gap: var(--xy-space-base);
  }

  .public-server-row__summary {
    grid-template-columns: 1fr;
    gap: var(--xy-space-sm);
  }

  .public-server-row__roster {
    grid-template-columns: 1fr;
  }

  .public-roster-list {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (prefers-reduced-motion: reduce) {
  .public-status-summary__freshness,
  .public-server-row__roster {
    animation: none;
  }
}
</style>
