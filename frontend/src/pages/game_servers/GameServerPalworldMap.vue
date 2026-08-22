<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { useQuasar } from 'quasar'
import { computed, onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { useRoute } from 'vue-router'

import GameServerMapShareSettings from '@/components/game_servers/GameServerMapShareSettings.vue'
import PalworldLiveMap from '@/components/palworld/PalworldLiveMap.vue'
import {
  GetPalworldMapRequestSchema,
  InstallPalworldMapTilesRequestSchema,
  PalworldMapLayerSchema,
  UpdatePalworldMapConfigRequestSchema,
  type PalworldMapLayer,
  type PalworldMapView,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const pollIntervalMs = 5_000
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

const route = useRoute()
const quasar = useQuasar()
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
let pollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const canManage = computed(() => mapView.value?.canManageShare ?? false)
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
  const configured = mapView.value?.layers[0]
  layerForm.value = configured ? create(PalworldMapLayerSchema, configured) : defaultLayer()
  settingsOpen.value = true
}

async function saveSettings(): Promise<void> {
  savingSettings.value = true
  try {
    const response = await GetXylonaClient().updatePalworldMapConfig(
      create(UpdatePalworldMapConfigRequestSchema, {
        gameServerId: gameServerID.value,
        layers: [layerForm.value],
      }),
    )
    if (mapView.value !== null) {
      mapView.value.layers = response.layers
    }
    settingsOpen.value = false
    quasar.notify({ type: 'positive', message: 'Map imagery settings saved.' })
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
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
    const configured = response.layers[0]
    if (configured !== undefined) {
      layerForm.value = create(PalworldMapLayerSchema, configured)
    }
    settingsOpen.value = false
    quasar.notify({
      type: 'positive',
      message: 'Palpagos and World Tree tiles are installed and served by Xylona.',
    })
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    installingTiles.value = false
  }
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
    quasar.notify({ type: 'positive', message: 'Map imagery removed. Using the coordinate grid.' })
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
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
  <div class="palworld-map-page">
    <header class="palworld-map-page__header">
      <div class="palworld-map-page__heading">
        <span class="palworld-map-page__heading-icon"><q-icon name="public" /></span>
        <div>
          <h1>Live world map</h1>
          <p>{{ mapDescription }}</p>
        </div>
      </div>
      <div v-if="canManage" class="palworld-map-page__actions">
        <q-btn dense flat icon="map" label="Map imagery" no-caps @click="openSettings" />
        <q-btn
          color="primary"
          dense
          icon="ios_share"
          label="Public link"
          no-caps
          @click="shareOpen = true" />
      </div>
    </header>

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
            <q-icon color="accent" name="download_for_offline" size="32px" />
            <div>
              <strong>Palworld 1.0 local tiles</strong>
              <span>
                Install or repair the Palpagos and World Tree layers. Xylona downloads them once,
                stores them beside the controller database, and hosts them for private and public
                maps.
              </span>
            </div>
            <q-btn
              :loading="installingTiles"
              color="primary"
              label="Install / repair"
              no-caps
              @click="installLocalTiles" />
          </section>

          <div class="palworld-map-dialog__custom-heading">
            <q-separator />
            <span>Custom tile source</span>
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
        </q-card-section>

        <q-card-actions align="between">
          <q-btn
            :disable="mapView?.layers.length === 0"
            :loading="savingSettings"
            color="negative"
            flat
            label="Use coordinate grid"
            no-caps
            @click="removeImagery" />
          <div class="row q-gutter-sm">
            <q-btn v-close-popup flat label="Cancel" no-caps />
            <q-btn
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
  padding: var(--xy-space-base) var(--xy-space-md) var(--xy-space-md);
  overflow: hidden;
}

.palworld-map-page__header,
.palworld-map-dialog__heading {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
}

.palworld-map-page__header {
  flex: 0 0 auto;
  min-height: 52px;
}

.palworld-map-page__heading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-base);
  min-width: 0;
}

.palworld-map-page__heading-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  width: 38px;
  height: 38px;
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-lg);
  font-size: 21px;
}

.palworld-map-page__header h1 {
  margin: 0;
  color: var(--xy-text-primary);
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-xl);
  line-height: var(--xy-line-height-tight);
}

.palworld-map-page__header p {
  margin: var(--xy-space-xs) 0 0;
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
}

.palworld-map-page__actions {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-sm);
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

@media (max-width: 700px) {
  .palworld-map-page {
    padding: var(--xy-space-sm);
  }

  .palworld-map-dialog__local-tiles {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .palworld-map-dialog__local-tiles .q-btn {
    grid-column: 1 / -1;
  }

  .palworld-map-page__header {
    align-items: flex-start;
    flex-direction: column;
  }

  .palworld-map-page__actions {
    width: 100%;
  }

  .palworld-map-dialog__grid {
    grid-template-columns: 1fr;
  }

  .palworld-map-dialog__grid > span {
    display: none;
  }
}
</style>
