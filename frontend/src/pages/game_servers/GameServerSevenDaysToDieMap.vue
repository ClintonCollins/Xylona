<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { useQuasar } from 'quasar'
import { computed, defineAsyncComponent, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { useRoute } from 'vue-router'

import { notifyConnectError, notifySuccess } from '@/api/notifications'
import GameServerMapShareSettings from '@/components/game_servers/GameServerMapShareSettings.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import SevenDaysToDieWorldOverview from '@/components/seven_days_to_die/SevenDaysToDieWorldOverview.vue'
import {
  GetGameServerRequestSchema,
  GetSevenDaysToDieMapRequestSchema,
  GetSevenDaysToDieWebAPIStatusRequestSchema,
  InstallSevenDaysToDieLandClaimsModRequestSchema,
  SevenDaysToDieWebAPIConnectionState,
  type SevenDaysToDieMapView,
  type SevenDaysToDieWebAPIStatus,
  SevenDaysToDieWebAPIStatusSchema,
  SevenDaysToDieWebAPIValueState,
} from '@/proto/xylona_pb'
import { GetXylonaClient } from '@/utils/shared'

const SevenDaysToDieLiveMap = defineAsyncComponent(
  () => import('@/components/seven_days_to_die/SevenDaysToDieLiveMap.vue'),
)

const mapPollIntervalMilliseconds = 5_000
const statusPollIntervalMilliseconds = 30_000

type StatusPresentation = {
  icon: string
  label: string
  tone: 'neutral' | 'warning' | 'negative'
}

const $q = useQuasar()
const route = useRoute()
const mapView = ref<SevenDaysToDieMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const canManage = ref(false)
const canConfigure = ref(false)
const permissionsLoaded = ref(false)
const shareOpen = ref(false)
const shareSettings = ref<InstanceType<typeof GameServerMapShareSettings> | null>(null)

async function onShareDialogToggle(open: boolean) {
  if (open || !shareSettings.value) {
    shareOpen.value = open
    return
  }
  await shareSettings.value.requestClose()
}
const currentStatus = shallowRef<SevenDaysToDieWebAPIStatus | null>(null)
const lastAvailableStatus = shallowRef<SevenDaysToDieWebAPIStatus | null>(null)
const statusLoading = ref(true)
const statusRefreshing = ref(false)
const manualRefreshing = ref(false)
const statusTransportError = ref(false)
const installingLandClaims = ref(false)
let mapPollTimer: ReturnType<typeof setInterval> | undefined
let statusPollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const configurationPath = computed(() =>
  canConfigure.value ? `/game-servers/${gameServerID.value}/configuration` : '',
)
const statusPresentation = computed<StatusPresentation | null>(() => {
  if (statusTransportError.value) {
    return {
      icon: 'cloud_off',
      label: 'World data unavailable',
      tone: 'negative',
    }
  }

  switch (currentStatus.value?.connectionState) {
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE:
      return null
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_SERVER_OFFLINE:
      return {
        icon: 'power_settings_new',
        label: 'Server offline',
        tone: 'neutral',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DASHBOARD_DISABLED:
      return {
        icon: 'toggle_off',
        label: 'Web dashboard disabled',
        tone: 'warning',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_MISCONFIGURED:
      return {
        icon: 'settings_alert',
        label: 'WebAPI misconfigured',
        tone: 'warning',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_NODE_UNAVAILABLE:
      return {
        icon: 'dns',
        label: 'Node unavailable',
        tone: 'negative',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_WEB_API_UNREACHABLE:
      return {
        icon: 'link_off',
        label: 'WebAPI unreachable',
        tone: 'negative',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_DISCOVERY_UNSUPPORTED:
      return {
        icon: 'help_outline',
        label: 'API discovery unsupported',
        tone: 'warning',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AUTHENTICATION_DENIED:
      return {
        icon: 'lock',
        label: 'WebAPI access denied',
        tone: 'negative',
      }
    case SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_INVALID_RESPONSE:
      return {
        icon: 'error_outline',
        label: 'Invalid WebAPI response',
        tone: 'negative',
      }
    default:
      return {
        icon: 'help_outline',
        label: 'World data unavailable',
        tone: 'neutral',
      }
  }
})
function withoutTacticalStatus(status: SevenDaysToDieWebAPIStatus): SevenDaysToDieWebAPIStatus {
  return create(SevenDaysToDieWebAPIStatusSchema, {
    ...status,
    capabilities: status.capabilities
      ? {
          ...status.capabilities,
          hostileAndAnimalPositions: false,
          hostilePositions: false,
          animalPositions: false,
        }
      : undefined,
    bloodMoonState:
      SevenDaysToDieWebAPIValueState.SEVEN_DAYS_TO_DIE_WEB_API_VALUE_STATE_UNSPECIFIED,
    bloodMoonActive: undefined,
    nextBloodMoon: undefined,
    nextBloodMoonEnd: undefined,
  })
}

function clearRetainedTacticalStatus(): void {
  if (currentStatus.value !== null) {
    currentStatus.value = withoutTacticalStatus(currentStatus.value)
  }
  if (lastAvailableStatus.value !== null) {
    lastAvailableStatus.value = withoutTacticalStatus(lastAvailableStatus.value)
  }
}

async function loadPermissions(): Promise<void> {
  try {
    const response = await GetXylonaClient().getGameServer(
      create(GetGameServerRequestSchema, { id: gameServerID.value }),
    )
    const permissions = response.gameServer?.effectivePermissions ?? []
    canManage.value = permissions.includes('game_server.settings')
    canConfigure.value = permissions.includes('game_server.config')
  } catch (unknownError: unknown) {
    canManage.value = false
    canConfigure.value = false
    console.error(unknownError)
  } finally {
    permissionsLoaded.value = true
    if (!canManage.value) {
      clearRetainedTacticalStatus()
    }
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
    const status =
      permissionsLoaded.value && !canManage.value
        ? withoutTacticalStatus(response.status)
        : response.status
    currentStatus.value = status
    statusTransportError.value = false
    if (
      status.connectionState ===
      SevenDaysToDieWebAPIConnectionState.SEVEN_DAYS_TO_DIE_WEB_API_CONNECTION_STATE_AVAILABLE
    ) {
      lastAvailableStatus.value = status
    } else {
      clearRetainedTacticalStatus()
    }
  } catch (unknownError: unknown) {
    currentStatus.value = null
    clearRetainedTacticalStatus()
    statusTransportError.value = true
    console.error(unknownError)
  } finally {
    statusLoading.value = false
    statusRefreshing.value = false
  }
}

async function refreshLiveData(): Promise<void> {
  if (manualRefreshing.value) return

  manualRefreshing.value = true
  try {
    await Promise.all([loadMap(), loadStatus()])
  } finally {
    manualRefreshing.value = false
  }
}

function confirmInstallLandClaimHelper(): void {
  $q.dialog({
    title: 'Install land claim support?',
    message:
      "Xylona writes its land claim helper mod to Mods/Xylona_LandClaims in this server's directory, replacing an older copy. The mod loads the next time the server starts.",
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'primary', label: 'Install' },
  }).onOk(() => {
    void installLandClaimHelper()
  })
}

async function installLandClaimHelper(): Promise<void> {
  if (installingLandClaims.value) return

  installingLandClaims.value = true
  try {
    await GetXylonaClient().installSevenDaysToDieLandClaimsMod(
      create(InstallSevenDaysToDieLandClaimsModRequestSchema, {
        gameServerId: gameServerID.value,
      }),
    )
    notifySuccess('Land claim support installed. It will load the next time the server starts.')
  } catch (unknownError: unknown) {
    notifyConnectError(unknownError, 'Failed to install land claim support')
  } finally {
    installingLandClaims.value = false
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
  <div class="seven-days-map-page xy-page-content">
    <page-header
      class="seven-days-map-page__header"
      subtitle="7 Days to Die. Player positions and world activity."
      title="Live Map">
      <template v-if="canManage" #actions>
        <q-btn
          aria-label="Open public map link settings"
          flat
          icon="share"
          no-caps
          @click="shareOpen = true">
          <span class="seven-days-map-page__share-label">Public link</span>
          <q-tooltip>Public map link</q-tooltip>
        </q-btn>
      </template>
    </page-header>

    <seven-days-to-die-live-map
      class="seven-days-map-page__map"
      :configuration-path="configurationPath"
      :load-error="loadError"
      :loading="loading"
      :refreshing="manualRefreshing"
      :view="mapView"
      @refresh="refreshLiveData" />

    <seven-days-to-die-world-overview
      class="seven-days-map-page__overview"
      :configuration-path="configurationPath"
      :show-tactical="canManage"
      :status="lastAvailableStatus"
      :status-loading="statusLoading"
      :status-presentation="statusPresentation"
      :view="mapView">
      <!-- Sits beside the land claims fact it fixes, not in the page header. -->
      <template v-if="canManage" #claims-action>
        <q-btn
          class="seven-days-map-page__claim-helper"
          data-testid="install-land-claim-helper"
          dense
          flat
          icon="extension"
          :loading="installingLandClaims"
          no-caps
          @click="confirmInstallLandClaimHelper">
          <span>Install / repair claims</span>
        </q-btn>
      </template>
    </seven-days-to-die-world-overview>

    <!-- Backdrop clicks and Esc ask the settings first so unsaved edits get the discard prompt.
         Route changes are left to the settings' own leave guard, so it never asks twice. -->
    <q-dialog :model-value="shareOpen" no-route-dismiss @update:model-value="onShareDialogToggle">
      <game-server-map-share-settings
        ref="shareSettings"
        :game-server-id="gameServerID"
        @close="shareOpen = false" />
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
  color: var(--xy-text-primary);
}

.seven-days-map-page__header {
  margin-bottom: 0;
}

.seven-days-map-page__map {
  flex: 0 0 auto;
}

.seven-days-map-page__claim-helper {
  justify-self: start;
}

@media (min-width: 1200px) {
  .seven-days-map-page {
    display: grid;
    flex: 1 0 auto;
    grid-template-areas:
      'header header'
      'map overview';
    grid-template-columns: minmax(0, 1fr) minmax(260px, 320px);
    grid-template-rows: auto minmax(0, 1fr);
    align-items: stretch;
  }

  .seven-days-map-page__header {
    grid-area: header;
  }

  .seven-days-map-page__map {
    grid-area: map;
    align-self: stretch;
    min-width: 0;
  }

  .seven-days-map-page__overview {
    grid-area: overview;
    min-width: 0;
  }
}

@media (max-width: 599px) {
  .seven-days-map-page__share-label {
    display: none;
  }
}
</style>
