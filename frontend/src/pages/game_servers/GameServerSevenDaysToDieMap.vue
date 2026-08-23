<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import { computed, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { useRoute } from 'vue-router'

import GameServerMapShareSettings from '@/components/game_servers/GameServerMapShareSettings.vue'
import SevenDaysToDieLiveMap from '@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'
import {
  GetGameServerRequestSchema,
  GetSevenDaysToDieMapRequestSchema,
  GetSevenDaysToDieWebAPIStatusRequestSchema,
  SevenDaysToDieWebAPIConnectionState,
  type SevenDaysToDieGameTime,
  type SevenDaysToDieMapView,
  type SevenDaysToDieWebAPICapabilities,
  type SevenDaysToDieWebAPIStatus,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const mapPollIntervalMilliseconds = 5_000
const statusPollIntervalMilliseconds = 30_000

type StatusPresentation = {
  description: string
  icon: string
  label: string
  tone: 'neutral' | 'positive' | 'warning' | 'negative'
}

type Capability = {
  icon: string
  key: keyof SevenDaysToDieWebAPICapabilities
  label: string
}

const capabilities: Capability[] = [
  { icon: 'person', key: 'playerData', label: 'Player data' },
  { icon: 'tune', key: 'runtimeSettings', label: 'Runtime settings' },
  { icon: 'article', key: 'nativeLog', label: 'Native log' },
  { icon: 'groups', key: 'worldPopulation', label: 'World population' },
  { icon: 'location_searching', key: 'hostilePositions', label: 'Hostile positions' },
  { icon: 'pets', key: 'animalPositions', label: 'Animal positions' },
  { icon: 'admin_panel_settings', key: 'accessControl', label: 'Access control' },
  { icon: 'key', key: 'gamePermissions', label: 'Game permissions' },
  { icon: 'extension', key: 'reportedMods', label: 'Reported mods' },
]

const route = useRoute()
const mapView = ref<SevenDaysToDieMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const canManage = ref(false)
const shareOpen = ref(false)
const currentStatus = shallowRef<SevenDaysToDieWebAPIStatus | null>(null)
const lastAvailableStatus = shallowRef<SevenDaysToDieWebAPIStatus | null>(null)
const statusLoading = ref(true)
const statusRefreshing = ref(false)
const statusTransportError = ref(false)
const now = ref(Date.now())
let mapPollTimer: ReturnType<typeof setInterval> | undefined
let statusPollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})

const statusIsAvailable = computed(
  () =>
    currentStatus.value?.connectionState ===
    SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE,
)
const statusIsStale = computed(
  () =>
    lastAvailableStatus.value !== null && (statusTransportError.value || !statusIsAvailable.value),
)
const statusPresentation = computed<StatusPresentation>(() => {
  if (statusTransportError.value) {
    return {
      description: 'Could not refresh diagnostics. Retry when the connection is restored.',
      icon: 'cloud_off',
      label: 'Diagnostics unavailable',
      tone: 'negative',
    }
  }

  switch (currentStatus.value?.connectionState) {
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE:
      return {
        description: 'The game server WebAPI is responding.',
        icon: 'check_circle',
        label: 'WebAPI available',
        tone: 'positive',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE:
      return {
        description: 'Start the game server to refresh diagnostics.',
        icon: 'power_settings_new',
        label: 'Server offline',
        tone: 'neutral',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED:
      return {
        description: 'Enable WebDashboardEnabled in serverconfig.xml.',
        icon: 'toggle_off',
        label: 'Web dashboard disabled',
        tone: 'warning',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED:
      return {
        description: 'Check WebDashboardPort in serverconfig.xml.',
        icon: 'settings_alert',
        label: 'WebAPI misconfigured',
        tone: 'warning',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE:
      return {
        description: 'The controller cannot reach the assigned node.',
        icon: 'dns',
        label: 'Node unavailable',
        tone: 'negative',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE:
      return {
        description: 'The game server WebAPI did not respond.',
        icon: 'link_off',
        label: 'WebAPI unreachable',
        tone: 'negative',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED:
      return {
        description: 'This WebAPI does not advertise an OpenAPI 3.x document.',
        icon: 'help_outline',
        label: 'API discovery unsupported',
        tone: 'warning',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED:
      return {
        description: 'The game server rejected the controller credentials.',
        icon: 'lock',
        label: 'WebAPI access denied',
        tone: 'negative',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE:
      return {
        description: 'The WebAPI returned data Xylona could not read.',
        icon: 'error_outline',
        label: 'Invalid WebAPI response',
        tone: 'negative',
      }
    default:
      return {
        description: 'Status has not been reported.',
        icon: 'help_outline',
        label: 'Diagnostics unavailable',
        tone: 'neutral',
      }
  }
})
const worldTimeLabel = computed(() =>
  valueLabel(lastAvailableStatus.value?.worldTimeState, lastAvailableStatus.value?.worldTime),
)
const bloodMoonLabel = computed(() => {
  const status = lastAvailableStatus.value
  if (
    status?.bloodMoonState ===
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
  ) {
    if (status.bloodMoonActive === true) return 'Active'
    if (status.bloodMoonActive === false) return 'Inactive'
  }
  return valueStateLabel(status?.bloodMoonState)
})
const nextBloodMoonLabel = computed(() =>
  bloodMoonTimeLabel(lastAvailableStatus.value?.nextBloodMoon),
)
const nextBloodMoonEndLabel = computed(() =>
  bloodMoonTimeLabel(lastAvailableStatus.value?.nextBloodMoonEnd),
)
const observationAgeLabel = computed(() => {
  const observedAt = lastAvailableStatus.value?.observedAt
  if (!observedAt) return 'Observation time unavailable'
  const observedAtMilliseconds = timestampDate(observedAt).getTime()
  if (!Number.isFinite(observedAtMilliseconds)) return 'Observation time unavailable'
  const seconds = Math.max(0, Math.floor((now.value - observedAtMilliseconds) / 1000))
  if (seconds < 60) return `Observed ${seconds} second${seconds === 1 ? '' : 's'} ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `Observed ${minutes} minute${minutes === 1 ? '' : 's'} ago`
  const hours = Math.floor(minutes / 60)
  return `Observed ${hours} hour${hours === 1 ? '' : 's'} ago`
})
const supportedCapabilityCount = computed(
  () =>
    capabilities.filter(
      (capability) => lastAvailableStatus.value?.capabilities?.[capability.key] === true,
    ).length,
)

function formatGameTime(gameTime: SevenDaysToDieGameTime): string {
  return `Day ${gameTime.day}, ${String(gameTime.hour).padStart(2, '0')}:${String(gameTime.minute).padStart(2, '0')}`
}

function valueStateLabel(state: SevenDaysToDieWebAPIValueState | undefined): string {
  switch (state) {
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSUPPORTED:
      return 'Not supported by this WebAPI'
    case SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_PERMISSION_DENIED:
      return 'Access denied by the game server'
    default:
      return 'Unavailable'
  }
}

function valueLabel(
  state: SevenDaysToDieWebAPIValueState | undefined,
  gameTime: SevenDaysToDieGameTime | undefined,
): string {
  return state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE &&
    gameTime
    ? formatGameTime(gameTime)
    : valueStateLabel(state)
}

function bloodMoonTimeLabel(gameTime: SevenDaysToDieGameTime | undefined): string {
  const state = lastAvailableStatus.value?.bloodMoonState
  return state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE &&
    gameTime
    ? formatGameTime(gameTime)
    : state === SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_AVAILABLE
      ? 'Not reported'
      : valueStateLabel(state)
}

async function loadPermissions(): Promise<void> {
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerID.value }),
    )
    canManage.value =
      response.gameServer?.effectivePermissions.includes('game_server.settings') ?? false
  } catch (unknownError: unknown) {
    console.error(unknownError)
  }
}

async function loadMap(): Promise<void> {
  if (loading.value || gameServerID.value === '') {
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getSevenDaysToDieMap(
      create(GetSevenDaysToDieMapRequestSchema, { gameServerId: gameServerID.value }),
    )
    mapView.value = response.map ?? null
    loadError.value = false
  } catch (unknownError: unknown) {
    clearTacticalMapAfterTransportFailure(mapView.value)
    loadError.value = true
    console.error(unknownError)
  } finally {
    loading.value = false
  }
}

function clearTacticalMapAfterTransportFailure(view: SevenDaysToDieMapView | null): void {
  if (view === null) return
  const unspecified =
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED
  const hadTacticalProjection = [
    view.nativeMarkerState,
    view.claimsState,
    view.bloodMoonState,
    view.hostileState,
    view.animalState,
  ].some((state) => state !== unspecified)
  if (!hadTacticalProjection) return

  const unavailable =
    SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNAVAILABLE
  view.nativeMarkers = []
  view.nativeMarkerState = unavailable
  view.claims = []
  view.claimsSupported = false
  view.claimsState = unavailable
  view.bloodMoon = undefined
  view.bloodMoonState = unavailable
  view.hostiles = []
  view.hostileState = unavailable
  view.animals = []
  view.animalState = unavailable
}

async function loadStatus(): Promise<void> {
  if (statusRefreshing.value) {
    return
  }
  if (gameServerID.value === '') {
    statusLoading.value = false
    return
  }
  statusRefreshing.value = true
  try {
    const response = await GetXylonaClient().getSevenDaysToDieWebAPIStatus(
      create(GetSevenDaysToDieWebAPIStatusRequestSchema, { gameServerId: gameServerID.value }),
    )
    if (!response.status) {
      throw new Error('The WebAPI status response was empty.')
    }
    currentStatus.value = response.status
    statusTransportError.value = false
    if (
      response.status.connectionState ===
      SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE
    ) {
      lastAvailableStatus.value = response.status
    }
  } catch (unknownError: unknown) {
    currentStatus.value = null
    statusTransportError.value = true
    console.error(unknownError)
  } finally {
    now.value = Date.now()
    statusLoading.value = false
    statusRefreshing.value = false
  }
}

onMounted(() => {
  void loadPermissions()
  void loadMap()
  void loadStatus()
  mapPollTimer = setInterval(() => void loadMap(), mapPollIntervalMilliseconds)
  statusPollTimer = setInterval(() => void loadStatus(), statusPollIntervalMilliseconds)
})

onBeforeUnmount(() => {
  if (mapPollTimer !== undefined) {
    clearInterval(mapPollTimer)
  }
  if (statusPollTimer !== undefined) {
    clearInterval(statusPollTimer)
  }
})
</script>

<template>
  <div class="seven-days-map-page">
    <header class="seven-days-map-page__header">
      <div class="seven-days-map-page__heading">
        <span class="seven-days-map-page__heading-icon"><q-icon name="map" /></span>
        <div>
          <span>7 Days to Die</span>
          <h1>Live world map</h1>
          <p>Live and last-known player positions across the world.</p>
        </div>
      </div>
      <div v-if="canManage" class="seven-days-map-page__actions">
        <q-btn color="primary" icon="share" label="Public link" no-caps @click="shareOpen = true" />
      </div>
    </header>

    <section class="webapi-diagnostics" aria-labelledby="webapi-diagnostics-title">
      <div class="webapi-diagnostics__header">
        <div>
          <h2 id="webapi-diagnostics-title">WebAPI diagnostics</h2>
          <p>Local game state and feature discovery reported by the server.</p>
        </div>
        <q-btn
          :disable="statusRefreshing"
          :loading="statusRefreshing"
          aria-label="Refresh WebAPI diagnostics"
          data-testid="webapi-retry"
          flat
          icon="refresh"
          label="Refresh"
          no-caps
          @click="loadStatus" />
      </div>

      <div
        class="webapi-diagnostics__content"
        data-testid="webapi-diagnostics"
        aria-atomic="true"
        aria-live="polite">
        <div v-if="statusLoading" class="webapi-diagnostics__loading" role="status">
          <q-spinner size="1.25rem" />
          <span>Checking WebAPI diagnostics…</span>
        </div>
        <template v-else>
          <div class="webapi-diagnostics__state" :class="`is-${statusPresentation.tone}`">
            <q-icon :name="statusPresentation.icon" size="1.5rem" />
            <div class="webapi-diagnostics__state-copy">
              <strong>{{ statusPresentation.label }}</strong>
              <span>{{ statusPresentation.description }}</span>
            </div>
            <span v-if="statusIsStale" class="webapi-diagnostics__freshness is-stale">
              <q-icon name="history" />
              Last successful observation
            </span>
            <span v-else-if="statusIsAvailable" class="webapi-diagnostics__freshness">
              <q-icon name="sensors" />
              Live observation
            </span>
          </div>

          <template v-if="lastAvailableStatus">
            <dl class="webapi-diagnostics__facts">
              <div>
                <dt>API version</dt>
                <dd>API {{ lastAvailableStatus.apiVersion || 'Not reported' }}</dd>
              </div>
              <div>
                <dt>World time</dt>
                <dd>{{ worldTimeLabel }}</dd>
              </div>
              <div data-testid="blood-moon-state">
                <dt>Blood Moon</dt>
                <dd>{{ bloodMoonLabel }}</dd>
              </div>
              <div>
                <dt>Next Blood Moon</dt>
                <dd>{{ nextBloodMoonLabel }}</dd>
              </div>
              <div>
                <dt>Blood Moon end</dt>
                <dd>{{ nextBloodMoonEndLabel }}</dd>
              </div>
              <div>
                <dt>Observation age</dt>
                <dd>{{ observationAgeLabel }}</dd>
              </div>
            </dl>

            <details class="webapi-diagnostics__capabilities" data-testid="webapi-capabilities">
              <summary>
                <span><q-icon name="fact_check" /> Capability details</span>
                <span>{{ supportedCapabilityCount }} of {{ capabilities.length }} supported</span>
              </summary>
              <ul>
                <li v-for="capability in capabilities" :key="capability.key">
                  <span><q-icon :name="capability.icon" /> {{ capability.label }}</span>
                  <span
                    :class="{
                      'is-supported': lastAvailableStatus.capabilities?.[capability.key],
                    }">
                    <q-icon
                      :name="
                        lastAvailableStatus.capabilities?.[capability.key]
                          ? 'check_circle'
                          : 'remove_circle_outline'
                      " />
                    {{
                      lastAvailableStatus.capabilities?.[capability.key]
                        ? 'Supported'
                        : 'Not advertised'
                    }}
                  </span>
                </li>
              </ul>
            </details>
          </template>
        </template>
      </div>
    </section>

    <seven-days-to-die-live-map
      :load-error="loadError"
      :loading="loading"
      :view="mapView"
      @refresh="loadMap" />

    <q-dialog v-model="shareOpen">
      <game-server-map-share-settings :game-server-id="gameServerID" @close="shareOpen = false" />
    </q-dialog>
  </div>
</template>

<style scoped>
.seven-days-map-page {
  display: flex;
  flex: 1;
  flex-direction: column;
  gap: var(--xy-space-md);
  min-height: 0;
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
}

.seven-days-map-page__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-lg);
}

.seven-days-map-page__heading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
}

.seven-days-map-page__heading-icon {
  display: grid;
  width: 42px;
  height: 42px;
  flex: 0 0 auto;
  place-items: center;
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
  border: 1px solid var(--xy-accent-border);
  border-radius: var(--xy-radius-md);
}

.seven-days-map-page__heading > div {
  display: grid;
  gap: var(--xy-space-2xs);
}

.seven-days-map-page__heading span {
  color: var(--xy-accent);
  font-size: var(--xy-font-size-xs);
  letter-spacing: 0.1em;
  text-transform: uppercase;
}

.seven-days-map-page__heading h1 {
  margin: 0;
  font-family: var(--xy-font-heading);
  font-weight: 500;
}

.seven-days-map-page__heading h1 {
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
}

.seven-days-map-page__heading p {
  margin: 0;
  color: var(--xy-text-secondary);
}

.seven-days-map-page__actions {
  display: flex;
  gap: var(--xy-space-sm);
}

.webapi-diagnostics {
  overflow: hidden;
  flex-shrink: 0;
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.webapi-diagnostics__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-base);
  padding: var(--xy-space-base) var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.webapi-diagnostics__header h2,
.webapi-diagnostics__header p {
  margin: 0;
}

.webapi-diagnostics__header h2 {
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  font-weight: 500;
}

.webapi-diagnostics__header p {
  margin-top: var(--xy-space-xs);
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.webapi-diagnostics__content {
  min-width: 0;
}

.webapi-diagnostics__loading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 72px;
  padding: var(--xy-space-md);
  color: var(--xy-text-secondary);
}

.webapi-diagnostics__state {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
  min-width: 0;
  padding: var(--xy-space-base) var(--xy-space-md);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-0);
}

.webapi-diagnostics__state.is-positive > .q-icon {
  color: var(--xy-success-hover);
}

.webapi-diagnostics__state.is-warning > .q-icon {
  color: var(--xy-warning-hover);
}

.webapi-diagnostics__state.is-negative > .q-icon {
  color: var(--xy-danger-hover);
}

.webapi-diagnostics__state-copy {
  display: grid;
  min-width: 0;
  gap: var(--xy-space-2xs);
}

.webapi-diagnostics__state-copy strong {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

.webapi-diagnostics__state-copy span {
  overflow-wrap: anywhere;
  font-size: var(--xy-font-size-xs);
}

.webapi-diagnostics__freshness {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  margin-inline-start: auto;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-success-text-soft);
  background: var(--xy-success-bg-faint);
  border-radius: var(--xy-radius-pill);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  white-space: nowrap;
}

.webapi-diagnostics__freshness.is-stale {
  color: var(--xy-warning-hover);
  background: var(--xy-warning-bg-faint);
}

.webapi-diagnostics__facts {
  display: flex;
  flex-wrap: wrap;
  margin: 0;
  padding: var(--xy-space-base) var(--xy-space-md);
}

.webapi-diagnostics__facts > div {
  min-width: 160px;
  flex: 1 1 160px;
  padding: var(--xy-space-xs) var(--xy-space-base);
  border-inline-start: 1px solid var(--xy-border);
}

.webapi-diagnostics__facts > div:first-child {
  border-inline-start: 0;
}

.webapi-diagnostics__facts dt {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.webapi-diagnostics__facts dd {
  margin: var(--xy-space-xs) 0 0;
  overflow-wrap: anywhere;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  font-variant-numeric: tabular-nums;
}

.webapi-diagnostics__capabilities {
  border-top: 1px solid var(--xy-border);
}

.webapi-diagnostics__capabilities summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-base);
  min-height: 44px;
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-secondary);
  cursor: pointer;
  font-size: var(--xy-font-size-sm);
  list-style: none;
}

.webapi-diagnostics__capabilities summary::-webkit-details-marker {
  display: none;
}

.webapi-diagnostics__capabilities summary:hover {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
}

.webapi-diagnostics__capabilities summary:focus-visible {
  outline: 2px solid var(--xy-focus-ring);
  outline-offset: -2px;
}

.webapi-diagnostics__capabilities summary > span {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.webapi-diagnostics__capabilities summary > span:last-child {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
}

.webapi-diagnostics__capabilities ul {
  display: grid;
  margin: 0;
  padding: 0 var(--xy-space-md) var(--xy-space-base);
  list-style: none;
}

.webapi-diagnostics__capabilities li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-base);
  min-height: 36px;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border-top: 1px solid var(--xy-border);
  font-size: var(--xy-font-size-sm);
}

.webapi-diagnostics__capabilities li > span {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-sm);
}

.webapi-diagnostics__capabilities li > span:last-child {
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  white-space: nowrap;
}

.webapi-diagnostics__capabilities li > span.is-supported {
  color: var(--xy-success-text-soft);
}

@media (max-width: 760px) {
  .seven-days-map-page {
    padding: var(--xy-space-sm) 0 0;
  }

  .seven-days-map-page__header {
    align-items: flex-start;
    padding: 0 var(--xy-space-sm);
  }

  .seven-days-map-page__heading p {
    display: none;
  }

  .seven-days-map-page__actions {
    flex-direction: column;
  }

  .webapi-diagnostics {
    border-inline: 0;
    border-radius: 0;
  }

  .webapi-diagnostics__header,
  .webapi-diagnostics__state {
    align-items: flex-start;
  }

  .webapi-diagnostics__freshness {
    margin-inline-start: 0;
  }

  .webapi-diagnostics__state {
    flex-wrap: wrap;
  }

  .webapi-diagnostics__state-copy {
    flex: 1 1 calc(100% - var(--xy-space-xl));
  }

  .webapi-diagnostics__facts {
    display: grid;
    grid-template-columns: 1fr;
  }

  .webapi-diagnostics__facts > div {
    min-width: 0;
    padding: var(--xy-space-sm) 0;
    border-top: 1px solid var(--xy-border);
    border-inline-start: 0;
  }

  .webapi-diagnostics__facts > div:first-child {
    border-top: 0;
  }

  .webapi-diagnostics__capabilities summary,
  .webapi-diagnostics__capabilities li {
    align-items: flex-start;
  }

  .webapi-diagnostics__capabilities li > span:first-child {
    min-width: 0;
  }
}
</style>
