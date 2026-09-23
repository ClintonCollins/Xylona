<script lang="ts" setup>
import { clone, create, equals } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, defineAsyncComponent, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { useRoute } from 'vue-router'

import { notifyError, notifySuccess } from '@/api/notifications'
import GameServerMapShareSettings from '@/components/game_servers/GameServerMapShareSettings.vue'
import PageHeader from '@/components/shared/PageHeader.vue'
import {
  GetPalworldMapRequestSchema,
  InstallPalworldMapTilesRequestSchema,
  PalworldMapLayerSchema,
  UpdatePalworldMapConfigRequestSchema,
  type PalworldMapLayer,
  type PalworldMapView,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const PalworldLiveMap = defineAsyncComponent(
  () => import('@/components/palworld/PalworldLiveMap.vue'),
)

const pollIntervalMs = 5_000
// Tiles installed by "Install / repair" are served from this controller path.
const managedTilePathPrefix = '/palworld-map-tiles/'
const defaultLayer = (): PalworldMapLayer =>
  create(PalworldMapLayerSchema, {
    id: 'world',
    label: 'World map',
    minZoom: 0,
    maxZoom: 6,
    tileSize: 512,
    transformA: 2048 / 1_800_000,
    transformB: 1024,
    transformC: -2048 / 1_950_000,
    transformD: 2048 * (750_000 / 1_950_000),
    minX: -1_200_000,
    minY: -900_000,
    maxX: 750_000,
    maxY: 900_000,
  })

const $q = useQuasar()
const route = useRoute()
// Each poll replaces the view wholesale, so deep reactivity would only re-proxy
// every actor in the snapshot for nothing.
const mapView = shallowRef<PalworldMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const settingsOpen = ref(false)
const shareOpen = ref(false)
const savingSettings = ref(false)
const installingTiles = ref(false)
const layerForm = ref<PalworldMapLayer>(defaultLayer())
const layerFormOriginal = ref<PalworldMapLayer>(defaultLayer())
const customSourceOpen = ref(false)
let pollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const canManage = computed(() => mapView.value?.canManageShare ?? false)
const configuredLayers = computed(() => mapView.value?.layers ?? [])
const managedTilesActive = computed(
  () =>
    configuredLayers.value.length > 0 &&
    configuredLayers.value.every((layer) =>
      layer.tileUrlTemplate.startsWith(managedTilePathPrefix),
    ),
)
const configuredLayerNames = computed(() =>
  configuredLayers.value.map((layer) => layer.label || layer.id).join(' and '),
)
const showCustomForm = computed(() => !managedTilesActive.value || customSourceOpen.value)
const settingsChanged = computed(
  () => !equals(PalworldMapLayerSchema, layerForm.value, layerFormOriginal.value),
)
const mapDescription = computed(() => {
  if (!mapView.value?.available) {
    return 'Palworld · live position tracking'
  }
  if (mapView.value?.partial) {
    return 'Palworld · live player positions'
  }
  return 'Palworld · players, bases, Pals, NPCs, and world actors'
})

async function loadMap(): Promise<void> {
  if (loading.value || gameServerID.value === '') {
    return
  }
  loading.value = true
  try {
    const response = await GetXylonaClient().getPalworldMap(
      create(GetPalworldMapRequestSchema, { gameServerId: gameServerID.value }),
    )
    mapView.value = response.map ?? null
    loadError.value = false
  } catch (unknownError: unknown) {
    loadError.value = true
    console.error(unknownError)
  } finally {
    loading.value = false
  }
}

function openSettings(): void {
  // Managed tiles are shown as the active source; the custom form then starts
  // from the default alignment instead of prefilling one managed layer.
  const configured = managedTilesActive.value ? undefined : configuredLayers.value[0]
  layerForm.value = configured ? clone(PalworldMapLayerSchema, configured) : defaultLayer()
  layerFormOriginal.value = clone(PalworldMapLayerSchema, layerForm.value)
  customSourceOpen.value = false
  settingsOpen.value = true
}

async function saveSettings(): Promise<void> {
  // A custom source replaces managed tiles; otherwise only the edited layer
  // changes and every other configured layer is kept.
  const layers =
    managedTilesActive.value || configuredLayers.value.length === 0
      ? [layerForm.value]
      : configuredLayers.value.map((layer, index) => (index === 0 ? layerForm.value : layer))
  savingSettings.value = true
  try {
    const response = await GetXylonaClient().updatePalworldMapConfig(
      create(UpdatePalworldMapConfigRequestSchema, {
        gameServerId: gameServerID.value,
        layers,
      }),
    )
    if (mapView.value !== null) {
      mapView.value.layers = response.layers
    }
    settingsOpen.value = false
    notifySuccess('Map imagery settings saved.')
  } catch (unknownError: unknown) {
    notifyError(ConnectErrorToString(ConnectError.from(unknownError)))
  } finally {
    savingSettings.value = false
  }
}

async function installLocalTiles(): Promise<void> {
  installingTiles.value = true
  try {
    const response = await GetXylonaClient().installPalworldMapTiles(
      create(InstallPalworldMapTilesRequestSchema, { gameServerId: gameServerID.value }),
    )
    if (mapView.value !== null) {
      mapView.value.layers = response.layers
    }
    settingsOpen.value = false
    notifySuccess('Palpagos and World Tree tiles are installed and served by Xylona.')
  } catch (unknownError: unknown) {
    notifyError(ConnectErrorToString(ConnectError.from(unknownError)))
  } finally {
    installingTiles.value = false
  }
}

function confirmRemoveImagery(): void {
  const layerCount = configuredLayers.value.length
  $q.dialog({
    title: 'Use the coordinate grid?',
    message: `This removes ${layerCount === 1 ? 'the' : `all ${layerCount}`} map imagery ${layerCount === 1 ? 'layer' : 'layers'} (${configuredLayerNames.value}) from this server's map, including its public link. You can install or add imagery again later.`,
    cancel: { flat: true, label: 'Cancel' },
    ok: { color: 'negative', label: 'Remove imagery' },
    persistent: true,
  }).onOk(() => {
    void removeImagery()
  })
}

async function removeImagery(): Promise<void> {
  savingSettings.value = true
  try {
    const response = await GetXylonaClient().updatePalworldMapConfig(
      create(UpdatePalworldMapConfigRequestSchema, {
        gameServerId: gameServerID.value,
        layers: [],
      }),
    )
    if (mapView.value !== null) {
      mapView.value.layers = response.layers
    }
    settingsOpen.value = false
    notifySuccess('Map imagery removed. Using the coordinate grid.')
  } catch (unknownError: unknown) {
    notifyError(ConnectErrorToString(ConnectError.from(unknownError)))
  } finally {
    savingSettings.value = false
  }
}

function stopPolling() {
  if (pollTimer !== undefined) {
    clearInterval(pollTimer)
    pollTimer = undefined
  }
}

function startPolling() {
  stopPolling()
  void loadMap()
  pollTimer = setInterval(() => {
    void loadMap()
  }, pollIntervalMs)
}

function handleVisibilityChange() {
  if (document.visibilityState === 'hidden') {
    stopPolling()
    return
  }
  startPolling()
}

onMounted(() => {
  document.addEventListener('visibilitychange', handleVisibilityChange)
  handleVisibilityChange()
})

onUnmounted(() => {
  document.removeEventListener('visibilitychange', handleVisibilityChange)
  stopPolling()
})
</script>

<template>
  <div class="palworld-map-page xy-page-content">
    <page-header class="palworld-map-page__header" :subtitle="mapDescription" title="Live Map">
      <template v-if="canManage" #actions>
        <q-btn flat icon="map" label="Map imagery" no-caps @click="openSettings" />
        <q-btn flat icon="share" label="Public link" no-caps @click="shareOpen = true" />
      </template>
    </page-header>

    <palworld-live-map
      :load-error="loadError"
      :loading="loading"
      :view="mapView"
      @refresh="loadMap" />

    <q-dialog v-model="settingsOpen">
      <q-card class="palworld-map-dialog">
        <q-card-section class="palworld-map-dialog__heading">
          <div>
            <div class="text-h6">Map imagery</div>
            <div class="text-caption text-xy-secondary">
              Download optional imagery into Xylona's data directory or use your own permitted XYZ
              source. Map art is not bundled with Xylona.
            </div>
          </div>
          <q-btn v-close-popup aria-label="Close map imagery settings" flat icon="close" round />
        </q-card-section>

        <q-card-section class="palworld-map-dialog__fields">
          <section class="palworld-map-dialog__local-tiles">
            <q-icon
              color="accent"
              :name="managedTilesActive ? 'check_circle' : 'download_for_offline'"
              size="32px" />
            <div>
              <strong>Palworld 1.0 local tiles</strong>
              <span v-if="managedTilesActive">
                Active source: {{ configuredLayerNames }}, hosted by Xylona for private and public
                maps.
              </span>
              <span v-else>
                Install or repair the Palpagos and World Tree layers. Xylona downloads them once,
                stores them beside the controller database, and hosts them for private and public
                maps.
              </span>
            </div>
            <q-btn
              :loading="installingTiles"
              label="Install / repair"
              no-caps
              outline
              @click="installLocalTiles" />
          </section>

          <q-btn
            v-if="!showCustomForm"
            class="palworld-map-dialog__custom-toggle"
            flat
            icon="tune"
            label="Use a custom tile source instead"
            no-caps
            @click="customSourceOpen = true" />

          <template v-if="showCustomForm">
            <div class="palworld-map-dialog__custom-heading">
              <q-separator />
              <span>Custom tile source</span>
            </div>
            <div v-if="managedTilesActive" class="palworld-map-dialog__custom-note">
              Saving a custom source replaces the Xylona-hosted layers.
            </div>

            <q-input v-model="layerForm.label" dense label="Map label" outlined />
            <q-input
              v-model="layerForm.tileUrlTemplate"
              dense
              hint="Must contain {z}, {x}, and {y}"
              label="Tile URL template"
              outlined />
            <q-input v-model="layerForm.attribution" dense label="Attribution" outlined />

            <q-expansion-item icon="tune" label="Coordinate alignment">
              <div class="palworld-map-dialog__grid q-pt-md">
                <q-input
                  v-model.number="layerForm.minZoom"
                  dense
                  label="Min zoom"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.maxZoom"
                  dense
                  label="Max zoom"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.tileSize"
                  dense
                  label="Tile size"
                  outlined
                  type="number" />
                <span aria-hidden="true" />
                <q-input
                  v-model.number="layerForm.transformA"
                  dense
                  label="Transform A"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.transformB"
                  dense
                  label="Transform B"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.transformC"
                  dense
                  label="Transform C"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.transformD"
                  dense
                  label="Transform D"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.minX"
                  dense
                  label="Minimum X"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.maxX"
                  dense
                  label="Maximum X"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.minY"
                  dense
                  label="Minimum Y"
                  outlined
                  type="number" />
                <q-input
                  v-model.number="layerForm.maxY"
                  dense
                  label="Maximum Y"
                  outlined
                  type="number" />
              </div>
            </q-expansion-item>
          </template>
        </q-card-section>

        <q-card-actions align="between">
          <q-btn
            :disable="configuredLayers.length === 0"
            :loading="savingSettings"
            color="negative"
            flat
            label="Use coordinate grid"
            no-caps
            @click="confirmRemoveImagery" />
          <div class="row q-gutter-sm">
            <q-btn v-close-popup flat label="Cancel" no-caps />
            <q-btn
              :disable="!showCustomForm || !settingsChanged"
              :loading="savingSettings"
              color="primary"
              label="Save"
              no-caps
              @click="saveSettings" />
          </div>
        </q-card-actions>
      </q-card>
    </q-dialog>

    <q-dialog v-model="shareOpen">
      <game-server-map-share-settings :game-server-id="gameServerID" @close="shareOpen = false" />
    </q-dialog>
  </div>
</template>

<style scoped>
.palworld-map-page {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: var(--xy-space-base);
  overflow: hidden;
}

.palworld-map-page__header {
  flex: 0 0 auto;
  margin-bottom: 0;
}

.palworld-map-dialog__heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.palworld-map-dialog {
  width: min(680px, calc(100vw - 2rem));
  max-height: calc(100dvh - 2rem);
}

.palworld-map-dialog__fields {
  display: grid;
  gap: var(--xy-space-md);
  max-height: 65dvh;
  overflow: auto;
}

.palworld-map-dialog__local-tiles {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-md);
  padding: var(--xy-space-md);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.palworld-map-dialog__local-tiles div {
  display: grid;
  gap: var(--xy-space-xs);
}

.palworld-map-dialog__local-tiles strong {
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
}

.palworld-map-dialog__local-tiles span,
.palworld-map-dialog__custom-heading span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.palworld-map-dialog__custom-toggle {
  justify-self: start;
}

.palworld-map-dialog__custom-note {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.palworld-map-dialog__custom-heading {
  display: grid;
  grid-template-columns: minmax(var(--xy-space-xl), 1fr) auto minmax(var(--xy-space-xl), 1fr);
  align-items: center;
  gap: var(--xy-space-sm);
}

.palworld-map-dialog__grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-md);
}

@media (max-width: 599px) {
  .palworld-map-dialog__local-tiles {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .palworld-map-dialog__local-tiles .q-btn {
    grid-column: 1 / -1;
  }

  .palworld-map-dialog__grid {
    grid-template-columns: 1fr;
  }

  .palworld-map-dialog__grid > span {
    display: none;
  }
}
</style>
