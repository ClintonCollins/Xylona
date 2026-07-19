<script lang="ts" setup>
import { create } from '@bufbuild/protobuf'
import { ConnectError } from '@connectrpc/connect'
import { copyToClipboard, useQuasar } from 'quasar'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import PalworldLiveMap from '@/components/palworld/PalworldLiveMap.vue'
import {
  GetPalworldMapRequestSchema,
  InstallPalworldMapTilesRequestSchema,
  PalworldMapLayerSchema,
  RegeneratePalworldMapShareRequestSchema,
  RevokePalworldMapShareRequestSchema,
  UpdatePalworldMapConfigRequestSchema,
  type PalworldMapLayer,
  type PalworldMapView,
} from '@/proto/xylona_pb'
import { ConnectErrorToString, GetXylonaClient } from '@/utils/shared'

const pollIntervalMs = 15_000
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
const mapView = ref<PalworldMapView | null>(null)
const loading = ref(false)
const loadError = ref(false)
const settingsOpen = ref(false)
const shareOpen = ref(false)
const savingSettings = ref(false)
const installingTiles = ref(false)
const changingShare = ref(false)
const generatedShareURL = ref('')
const layerForm = ref<PalworldMapLayer>(defaultLayer())
let pollTimer: ReturnType<typeof setInterval> | undefined

const gameServerID = computed(() => {
  const id = route.params.id
  return Array.isArray(id) ? (id[0] ?? '') : String(id ?? '')
})
const canManage = computed(() => mapView.value?.canManageShare ?? false)

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

async function regenerateShare(): Promise<void> {
  changingShare.value = true
  try {
    const response = await GetXylonaClient().regeneratePalworldMapShare(
      create(RegeneratePalworldMapShareRequestSchema, { gameServerId: gameServerID.value }),
    )
    generatedShareURL.value = `${window.location.origin}/shared/palworld-map#${response.shareToken}`
    if (mapView.value !== null) {
      mapView.value.shareEnabled = true
    }
    await copyShareURL()
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    changingShare.value = false
  }
}

async function copyShareURL(): Promise<void> {
  if (generatedShareURL.value === '') {
    return
  }
  try {
    await copyToClipboard(generatedShareURL.value)
    quasar.notify({ type: 'positive', message: 'Public map link copied.' })
  } catch (unknownError: unknown) {
    console.error(unknownError)
    quasar.notify({ type: 'negative', message: 'Could not copy the public map link.' })
  }
}

async function revokeShare(): Promise<void> {
  changingShare.value = true
  try {
    await GetXylonaClient().revokePalworldMapShare(
      create(RevokePalworldMapShareRequestSchema, { gameServerId: gameServerID.value }),
    )
    generatedShareURL.value = ''
    if (mapView.value !== null) {
      mapView.value.shareEnabled = false
    }
    quasar.notify({ type: 'positive', message: 'Public map link revoked.' })
  } catch (unknownError: unknown) {
    quasar.notify({
      type: 'negative',
      message: ConnectErrorToString(ConnectError.from(unknownError)),
    })
  } finally {
    changingShare.value = false
  }
}

onMounted(() => {
  void loadMap()
  pollTimer = setInterval(() => void loadMap(), pollIntervalMs)
})

onBeforeUnmount(() => {
  if (pollTimer !== undefined) {
    clearInterval(pollTimer)
  }
})
</script>

<template>
  <div class="palworld-map-page">
    <header class="palworld-map-page__header">
      <div>
        <div class="palworld-map-page__eyebrow">Palworld</div>
        <h1>Live world map</h1>
        <p>Exact positions for players, bases, Pals, NPCs, and other world actors.</p>
      </div>
      <div v-if="canManage" class="palworld-map-page__actions">
        <q-btn flat icon="map" label="Map imagery" no-caps @click="openSettings" />
        <q-btn
          color="primary"
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
      <q-card class="palworld-map-dialog palworld-share-dialog">
        <q-card-section class="palworld-map-dialog__heading">
          <div>
            <div class="text-h6">Public live map</div>
            <div class="text-caption text-xy-secondary">
              Anyone with the link can see the same exact names and positions shown here.
            </div>
          </div>
          <q-btn v-close-popup aria-label="Close public link settings" flat icon="close" round />
        </q-card-section>

        <q-card-section>
          <div v-if="generatedShareURL" class="palworld-share-dialog__link">
            <q-input :model-value="generatedShareURL" dense label="Public link" outlined readonly />
            <q-btn color="primary" icon="content_copy" label="Copy" no-caps @click="copyShareURL" />
          </div>
          <div v-else-if="mapView?.shareEnabled" class="palworld-share-dialog__state">
            <q-icon color="positive" name="public" size="28px" />
            <div>
              <strong>A public link is active</strong>
              <span>Generate a new link to replace it and copy the new address.</span>
            </div>
          </div>
          <div v-else class="palworld-share-dialog__state">
            <q-icon name="link_off" size="28px" />
            <div>
              <strong>Public sharing is off</strong>
              <span>Generate a link when you are ready to share this live map.</span>
            </div>
          </div>
        </q-card-section>

        <q-card-actions align="between">
          <q-btn
            v-if="mapView?.shareEnabled"
            :loading="changingShare"
            color="negative"
            flat
            label="Revoke link"
            no-caps
            @click="revokeShare" />
          <span v-else />
          <q-btn
            :loading="changingShare"
            color="primary"
            :label="mapView?.shareEnabled ? 'Generate new link' : 'Generate public link'"
            no-caps
            @click="regenerateShare" />
        </q-card-actions>
      </q-card>
    </q-dialog>
  </div>
</template>

<style scoped>
.palworld-map-page {
  display: flex;
  flex: 1;
  min-height: 0;
  flex-direction: column;
  gap: var(--xy-space-md);
  padding: var(--xy-space-lg);
  overflow: hidden;
}

.palworld-map-page__header,
.palworld-map-dialog__heading,
.palworld-share-dialog__link,
.palworld-share-dialog__state {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-md);
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

.palworld-map-page__eyebrow {
  color: var(--xy-accent);
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  letter-spacing: 0.12em;
  text-transform: uppercase;
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

.palworld-share-dialog__link {
  align-items: flex-end;
}

.palworld-share-dialog__link .q-field {
  flex: 1;
  min-width: 0;
}

.palworld-share-dialog__state {
  justify-content: flex-start;
  padding: var(--xy-space-md);
  background: var(--xy-surface-2);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-lg);
}

.palworld-share-dialog__state div {
  display: grid;
  gap: var(--xy-space-xs);
}

.palworld-share-dialog__state span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-sm);
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

  .palworld-map-dialog__grid {
    grid-template-columns: 1fr;
  }

  .palworld-map-dialog__grid > span {
    display: none;
  }

  .palworld-share-dialog__link {
    align-items: stretch;
    flex-direction: column;
  }
}
</style>
