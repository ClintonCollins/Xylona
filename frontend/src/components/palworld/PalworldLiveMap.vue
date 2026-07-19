<script lang="ts" setup>
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { timestampDate } from '@bufbuild/protobuf/wkt'
import L, { type CircleMarker, type Map as LeafletMap, type Marker } from 'leaflet'
import 'leaflet/dist/leaflet.css'

import {
  PalworldMapActorKind,
  type PalworldMapActor,
  type PalworldMapLayer,
  type PalworldMapView,
} from '@/proto/xylona_pb'
import {
  filterPalworldMapActors,
  formatPalworldCoordinate,
  initialPalworldMapVisibility,
  palworldMapCategories,
  palworldMapCategory,
} from '@/pages/game_servers/palworld-map'

const props = withDefaults(
  defineProps<{
    view: PalworldMapView | null
    loading?: boolean
    loadError?: boolean
    publicMode?: boolean
  }>(),
  {
    loading: false,
    loadError: false,
    publicMode: false,
  },
)

const emit = defineEmits<{
  refresh: []
}>()

const fallbackLayer: PalworldMapLayer = {
  $typeName: 'xylona.PalworldMapLayer',
  id: 'coordinate-grid',
  label: 'Coordinate grid',
  tileUrlTemplate: '',
  attribution: '',
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
}

const primaryActorCategories = palworldMapCategories.slice(0, 4)
const secondaryActorCategories = palworldMapCategories.slice(4)

const mapRoot = ref<HTMLElement | null>(null)
const mapElement = ref<HTMLElement | null>(null)
const railOpen = ref(false)
const moreKindsOpen = ref(false)
const fullscreen = ref(false)
const search = ref('')
const visibleKinds = ref<Record<number, boolean>>(initialPalworldMapVisibility())
const activeLayerID = ref('')
const selectedActorKey = ref('')

let map: LeafletMap | null = null
let actorLayer: L.LayerGroup | null = null
let resizeObserver: ResizeObserver | null = null
let currentLayerKey = ''
let fittedOnce = false
let railInitialized = false
const actorMarkers = new Map<string, Marker | CircleMarker>()

const actors = computed(() => props.view?.actors ?? [])
const configuredLayers = computed(() => props.view?.layers ?? [])
const activeLayer = computed(() => {
  const configured = configuredLayers.value.find((layer) => layer.id === activeLayerID.value)
  return configured ?? configuredLayers.value[0] ?? fallbackLayer
})
const hasImagery = computed(() => activeLayer.value.tileUrlTemplate !== '')
const filteredActors = computed(() =>
  filterPalworldMapActors(actors.value, visibleKinds.value, search.value),
)
const selectedActor = computed(
  () => actors.value.find((actor) => actor.key === selectedActorKey.value) ?? null,
)
const actorCounts = computed(() => {
  const counts = new Map<PalworldMapActorKind, number>()
  for (const actor of actors.value) {
    counts.set(actor.kind, (counts.get(actor.kind) ?? 0) + 1)
  }
  return counts
})
const secondaryActorCount = computed(() =>
  secondaryActorCategories.reduce(
    (count, category) => count + (actorCounts.value.get(category.kind) ?? 0),
    0,
  ),
)
const mapStatus = computed(() => {
  if (props.loadError) {
    return { label: 'Connection lost', icon: 'cloud_off', tone: 'danger' }
  }
  if (props.loading && props.view === null) {
    return { label: 'Loading world', icon: 'sync', tone: 'info' }
  }
  if (!props.view?.available) {
    return {
      label: props.view?.serverOnline ? 'Waiting for data' : 'Server offline',
      icon: 'schedule',
      tone: 'warning',
    }
  }
  if (props.view.stale) {
    return { label: 'Last known positions', icon: 'history', tone: 'warning' }
  }
  if (props.view.partial) {
    return { label: 'Players only', icon: 'group', tone: 'info' }
  }
  return { label: 'Live world', icon: 'sensors', tone: 'success' }
})
const collectedLabel = computed(() => {
  const collectedAt = props.view?.collectedAt
  if (collectedAt === undefined) {
    return 'No snapshot received yet'
  }
  return `Updated ${timestampDate(collectedAt).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })}`
})

function cssColor(token: string): string {
  const value = getComputedStyle(document.documentElement).getPropertyValue(token).trim()
  return value || getComputedStyle(document.documentElement).color
}

function escapeHTML(value: string): string {
  return value.replace(
    /[&<>'"]/g,
    (character) =>
      ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', "'": '&#39;', '"': '&quot;' })[character] ??
      character,
  )
}

function buildCRS(layer: PalworldMapLayer): L.CRS {
  return Object.assign({}, L.CRS.Simple, {
    transformation: new L.Transformation(
      layer.transformA,
      layer.transformB,
      layer.transformC,
      layer.transformD,
    ),
  }) as L.CRS
}

function worldLocation(locationX: number, locationY: number): L.LatLng {
  return L.latLng(locationX, locationY)
}

function layerBounds(layer: PalworldMapLayer): L.LatLngBounds {
  return L.latLngBounds(
    worldLocation(layer.minX, layer.minY),
    worldLocation(layer.maxX, layer.maxY),
  )
}

function layerRenderKey(layer: PalworldMapLayer): string {
  return [
    layer.id,
    layer.tileUrlTemplate,
    layer.minZoom,
    layer.maxZoom,
    layer.tileSize,
    layer.transformA,
    layer.transformB,
    layer.transformC,
    layer.transformD,
    layer.minX,
    layer.minY,
    layer.maxX,
    layer.maxY,
  ].join('|')
}

function destroyMap(): void {
  actorMarkers.clear()
  actorLayer = null
  if (map !== null) {
    map.remove()
    map = null
  }
}

function createMap(layer: PalworldMapLayer): void {
  if (mapElement.value === null) {
    return
  }
  destroyMap()
  currentLayerKey = layerRenderKey(layer)
  fittedOnce = false
  map = L.map(mapElement.value, {
    crs: buildCRS(layer),
    minZoom: layer.minZoom,
    maxZoom: layer.maxZoom,
    zoomSnap: 0.25,
    zoomDelta: 0.5,
    attributionControl: false,
    preferCanvas: true,
    maxBounds: layerBounds(layer).pad(0.2),
    maxBoundsViscosity: 0.75,
  })
  actorLayer = L.layerGroup().addTo(map)
  if (layer.tileUrlTemplate !== '') {
    L.tileLayer(layer.tileUrlTemplate, {
      tileSize: layer.tileSize,
      minZoom: layer.minZoom,
      maxZoom: layer.maxZoom,
      noWrap: true,
      bounds: layerBounds(layer),
    }).addTo(map)
  }
  fitWorldBounds()
}

function markerTooltip(actor: PalworldMapActor): HTMLElement {
  const element = document.createElement('span')
  element.textContent = `${actor.name} · X ${formatPalworldCoordinate(actor.locationX)}, Y ${formatPalworldCoordinate(actor.locationY)}`
  return element
}

function createActorMarker(actor: PalworldMapActor): Marker | CircleMarker {
  const category = palworldMapCategory(actor.kind)
  const location = worldLocation(actor.locationX, actor.locationY)
  const color = cssColor(category.colorToken)
  const selected = actor.key === selectedActorKey.value
  let marker: Marker | CircleMarker
  if (category.labeledMarker) {
    marker = L.marker(location, {
      icon: L.divIcon({
        className: '',
        html: `<span class="palworld-map-marker palworld-map-marker--${actor.kind}${selected ? ' palworld-map-marker--selected' : ''}${actor.active ? ' palworld-map-marker--active' : ''}" style="--actor-color:${escapeHTML(color)}"><span class="palworld-map-marker__icon"><span class="material-icons" aria-hidden="true">${escapeHTML(category.icon)}</span></span><span class="palworld-map-marker__label">${escapeHTML(actor.name)}</span></span>`,
        iconAnchor: [17, 17],
      }),
      keyboard: true,
      title: actor.name,
    })
  } else {
    marker = L.circleMarker(location, {
      renderer: L.canvas({ padding: 0.35 }),
      radius: selected ? 7 : actor.active ? 5 : 4,
      color,
      fillColor: color,
      fillOpacity: selected ? 1 : actor.active ? 0.82 : 0.4,
      opacity: 0.95,
      weight: selected ? 3 : 1.5,
    })
    marker.bindTooltip(markerTooltip(actor), { direction: 'top', offset: [0, -5] })
  }
  marker.on('click', () => {
    selectedActorKey.value = actor.key
    railOpen.value = true
  })
  return marker
}

function renderActors(): void {
  if (map === null || actorLayer === null) {
    return
  }
  actorLayer.clearLayers()
  actorMarkers.clear()
  const activeBounds = layerBounds(activeLayer.value)
  for (const actor of filteredActors.value) {
    const location = worldLocation(actor.locationX, actor.locationY)
    if (!activeBounds.contains(location)) {
      continue
    }
    const marker = createActorMarker(actor)
    marker.addTo(actorLayer)
    actorMarkers.set(actor.key, marker)
  }
}

function updateActorMarkerSelection(actorKey: string, selected: boolean): void {
  if (actorKey === '') {
    return
  }
  const actor = actors.value.find((candidate) => candidate.key === actorKey)
  const marker = actorMarkers.get(actorKey)
  if (actor === undefined || marker === undefined) {
    return
  }
  const category = palworldMapCategory(actor.kind)
  if (category.labeledMarker) {
    const markerElement = marker.getElement()?.querySelector('.palworld-map-marker')
    markerElement?.classList.toggle('palworld-map-marker--selected', selected)
    return
  }
  if (!(marker instanceof L.CircleMarker)) {
    return
  }
  marker.setRadius(selected ? 7 : actor.active ? 5 : 4)
  marker.setStyle({
    fillOpacity: selected ? 1 : actor.active ? 0.82 : 0.4,
    weight: selected ? 3 : 1.5,
  })
}

async function refreshMap(): Promise<void> {
  await nextTick()
  const layer = activeLayer.value
  if (map === null || currentLayerKey !== layerRenderKey(layer)) {
    createMap(layer)
  }
  renderActors()
  map?.invalidateSize({ animate: false })
  if (!fittedOnce) {
    fitVisibleActors()
  }
}

function fitWorldBounds(): void {
  if (map === null) {
    return
  }
  map.invalidateSize({ animate: false })
  const wideCanvas = (mapElement.value?.clientWidth ?? 0) >= 900
  map.fitBounds(layerBounds(activeLayer.value), {
    animate: false,
    paddingTopLeft: [railOpen.value && wideCanvas ? 320 : 32, 72],
    paddingBottomRight: [32, 48],
  })
  if (hasImagery.value && wideCanvas && map.getZoom() < 0.5 && activeLayer.value.maxZoom >= 1) {
    map.setZoom(0.5, { animate: false })
  }
}

function fitWorld(): void {
  fitWorldBounds()
  fittedOnce = true
}

function fitVisibleActors(): void {
  if (map === null) {
    return
  }
  const positioned = filteredActors.value.filter((actor) =>
    layerBounds(activeLayer.value).contains(worldLocation(actor.locationX, actor.locationY)),
  )
  if (positioned.length === 0) {
    fitWorld()
    return
  }
  map.fitBounds(
    L.latLngBounds(positioned.map((actor) => worldLocation(actor.locationX, actor.locationY))),
    {
      maxZoom: Math.min(activeLayer.value.maxZoom, 4),
      paddingTopLeft: [railOpen.value ? 320 : 48, 80],
      paddingBottomRight: [48, 80],
    },
  )
  fittedOnce = true
}

async function focusActor(actor: PalworldMapActor): Promise<void> {
  visibleKinds.value[actor.kind] = true
  selectedActorKey.value = actor.key
  railOpen.value = true
  await nextTick()
  const location = worldLocation(actor.locationX, actor.locationY)
  map?.setView(location, Math.min(activeLayer.value.maxZoom, 4), { animate: true })
  actorMarkers.get(actor.key)?.openTooltip()
}

function selectLayer(layerID: string): void {
  if (activeLayerID.value === layerID) {
    return
  }
  activeLayerID.value = layerID
}

function closeSelectedActor(): void {
  selectedActorKey.value = ''
}

function toggleRail(): void {
  railOpen.value = !railOpen.value
}

async function toggleFullscreen(): Promise<void> {
  if (mapRoot.value === null) {
    return
  }
  try {
    if (document.fullscreenElement === mapRoot.value) {
      await document.exitFullscreen()
    } else {
      await mapRoot.value.requestFullscreen()
    }
  } catch (unknownError: unknown) {
    console.error('Could not change Palworld map fullscreen state', unknownError)
  }
}

function handleFullscreenChange(): void {
  fullscreen.value = document.fullscreenElement === mapRoot.value
  void nextTick(() => map?.invalidateSize({ animate: false }))
}

watch(
  () => props.view,
  () => {
    if (!railInitialized && props.view !== null) {
      railOpen.value = actors.value.length > 0
      railInitialized = true
    }
    if (
      activeLayerID.value === '' ||
      !configuredLayers.value.some((layer) => layer.id === activeLayerID.value)
    ) {
      activeLayerID.value = configuredLayers.value[0]?.id ?? fallbackLayer.id
    }
    void refreshMap()
  },
  { immediate: true },
)
watch(
  () => activeLayerID.value,
  () => void refreshMap(),
)
watch(
  () => JSON.stringify(visibleKinds.value),
  () => renderActors(),
)
watch(search, () => renderActors())
watch(selectedActorKey, (actorKey, previousActorKey) => {
  updateActorMarkerSelection(previousActorKey, false)
  updateActorMarkerSelection(actorKey, true)
})

onMounted(() => {
  void refreshMap()
  document.addEventListener('fullscreenchange', handleFullscreenChange)
  if (mapElement.value !== null) {
    resizeObserver = new ResizeObserver(() => map?.invalidateSize({ animate: false }))
    resizeObserver.observe(mapElement.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  document.removeEventListener('fullscreenchange', handleFullscreenChange)
  destroyMap()
})
</script>

<template>
  <section
    ref="mapRoot"
    class="palworld-live-map"
    :class="{
      'palworld-live-map--rail-open': railOpen,
      'palworld-live-map--public': publicMode,
      'palworld-live-map--imagery': hasImagery,
      'palworld-live-map--fullscreen': fullscreen,
    }">
    <div class="palworld-live-map__canvas-wrap">
      <div ref="mapElement" class="palworld-live-map__canvas" aria-label="Palworld live map" />

      <div class="palworld-live-map__toolbar" role="toolbar" aria-label="Map controls">
        <q-btn
          :aria-label="railOpen ? 'Close world actors' : 'Open world actors'"
          class="palworld-live-map__toolbar-action palworld-live-map__actors-toggle"
          :class="{ 'palworld-live-map__toolbar-action--active': railOpen }"
          flat
          icon="radar"
          round
          @click="toggleRail">
          <q-badge v-if="actors.length > 0" color="primary" floating rounded>
            {{ actors.length > 999 ? '999+' : actors.length }}
          </q-badge>
          <q-tooltip>{{ railOpen ? 'Close world actors' : 'Open world actors' }}</q-tooltip>
        </q-btn>

        <div
          class="palworld-live-map__status"
          :class="`palworld-live-map__status--${mapStatus.tone}`">
          <q-icon :name="mapStatus.icon" />
          <div>
            <strong>{{ mapStatus.label }}</strong>
            <span>{{ collectedLabel }}</span>
          </div>
        </div>

        <div v-if="configuredLayers.length > 1" class="palworld-live-map__map-layers">
          <button
            v-for="layer in configuredLayers"
            :key="layer.id"
            :class="{ 'palworld-live-map__map-layer--active': activeLayer.id === layer.id }"
            type="button"
            @click="selectLayer(layer.id)">
            {{ layer.label }}
          </button>
        </div>

        <div class="palworld-live-map__toolbar-actions">
          <span v-if="view?.truncated" class="palworld-live-map__toolbar-flag">
            <q-icon name="data_usage" /> Actor limit
          </span>
          <q-btn
            aria-label="Fit world"
            class="palworld-live-map__toolbar-action"
            flat
            icon="center_focus_strong"
            round
            @click="fitWorld">
            <q-tooltip>Fit entire world</q-tooltip>
          </q-btn>
          <q-btn
            :aria-label="fullscreen ? 'Exit fullscreen map' : 'Open fullscreen map'"
            class="palworld-live-map__toolbar-action"
            flat
            :icon="fullscreen ? 'fullscreen_exit' : 'fullscreen'"
            round
            @click="toggleFullscreen">
            <q-tooltip>{{ fullscreen ? 'Exit fullscreen' : 'Fullscreen map' }}</q-tooltip>
          </q-btn>
          <q-btn
            aria-label="Refresh live map"
            class="palworld-live-map__toolbar-action"
            flat
            :loading="loading"
            icon="refresh"
            round
            @click="emit('refresh')">
            <q-tooltip>Refresh live data</q-tooltip>
          </q-btn>
        </div>
      </div>

      <aside class="palworld-live-map__rail" :aria-hidden="!railOpen" :inert="!railOpen">
        <div class="palworld-live-map__rail-header">
          <div class="palworld-live-map__rail-heading">
            <span class="palworld-live-map__rail-heading-icon"><q-icon name="radar" /></span>
            <div>
              <div class="palworld-live-map__rail-title">World actors</div>
              <div class="palworld-live-map__rail-copy">
                {{ actors.length.toLocaleString() }} reported
              </div>
            </div>
          </div>
          <q-btn
            aria-label="Close world actors"
            flat
            icon="close"
            round
            size="sm"
            @click="railOpen = false" />
        </div>

        <div v-if="selectedActor !== null" class="palworld-live-map__selected" role="status">
          <div class="palworld-live-map__selected-heading">
            <span
              class="palworld-live-map__selection-icon"
              :style="{
                '--actor-color': `var(${palworldMapCategory(selectedActor.kind).colorToken})`,
              }">
              <q-icon :name="palworldMapCategory(selectedActor.kind).icon" />
            </span>
            <div>
              <strong>{{ selectedActor.name }}</strong>
              <span>{{ palworldMapCategory(selectedActor.kind).singular }}</span>
            </div>
            <q-btn
              aria-label="Close actor details"
              flat
              icon="close"
              round
              size="sm"
              @click="closeSelectedActor" />
          </div>
          <div class="palworld-live-map__selected-details">
            <span><small>X</small>{{ formatPalworldCoordinate(selectedActor.locationX) }}</span>
            <span><small>Y</small>{{ formatPalworldCoordinate(selectedActor.locationY) }}</span>
            <span><small>Z</small>{{ formatPalworldCoordinate(selectedActor.locationZ) }}</span>
            <span v-if="selectedActor.level > 0"
              ><small>Level</small>{{ selectedActor.level }}</span
            >
          </div>
          <div
            v-if="selectedActor.guildName || selectedActor.trainerName || selectedActor.action"
            class="palworld-live-map__selected-meta">
            <span v-if="selectedActor.guildName">Guild · {{ selectedActor.guildName }}</span>
            <span v-if="selectedActor.trainerName">Trainer · {{ selectedActor.trainerName }}</span>
            <span v-if="selectedActor.action">{{ selectedActor.action }}</span>
          </div>
        </div>

        <q-input
          v-if="actors.length > 0"
          v-model="search"
          aria-label="Search world actors"
          class="palworld-live-map__search"
          clearable
          dense
          outlined
          placeholder="Search actors">
          <template #prepend><q-icon name="search" /></template>
        </q-input>

        <div class="palworld-live-map__layers" aria-label="Actor layers">
          <button
            v-for="category in moreKindsOpen ? palworldMapCategories : primaryActorCategories"
            :key="category.kind"
            class="palworld-live-map__layer"
            :class="{ 'palworld-live-map__layer--active': visibleKinds[category.kind] }"
            type="button"
            @click="visibleKinds[category.kind] = !visibleKinds[category.kind]">
            <span
              class="palworld-live-map__layer-icon"
              :style="{ '--actor-color': `var(${category.colorToken})` }">
              <q-icon :name="category.icon" />
            </span>
            <span class="palworld-live-map__layer-label">{{ category.label }}</span>
            <span class="palworld-live-map__layer-count">{{
              actorCounts.get(category.kind) ?? 0
            }}</span>
            <q-icon
              :aria-label="visibleKinds[category.kind] ? 'Visible' : 'Hidden'"
              :name="visibleKinds[category.kind] ? 'visibility' : 'visibility_off'"
              size="17px" />
          </button>
          <button
            class="palworld-live-map__more-layers"
            type="button"
            @click="moreKindsOpen = !moreKindsOpen">
            <span>More actors</span>
            <span>{{ secondaryActorCount.toLocaleString() }}</span>
            <q-icon :name="moreKindsOpen ? 'expand_less' : 'expand_more'" size="18px" />
          </button>
        </div>

        <template v-if="actors.length > 0">
          <div class="palworld-live-map__roster-heading">
            <span>Visible actors</span>
            <span>{{ filteredActors.length.toLocaleString() }}</span>
          </div>
          <q-virtual-scroll
            v-if="filteredActors.length > 0"
            class="palworld-live-map__roster"
            :items="filteredActors"
            :virtual-scroll-item-size="54">
            <template #default="{ item: actor }">
              <button
                :key="actor.key"
                class="palworld-live-map__actor"
                :class="{ 'palworld-live-map__actor--selected': selectedActorKey === actor.key }"
                type="button"
                @click="focusActor(actor)">
                <span
                  class="palworld-live-map__actor-symbol"
                  :style="{
                    '--actor-color': `var(${palworldMapCategory(actor.kind).colorToken})`,
                  }">
                  <q-icon :name="palworldMapCategory(actor.kind).icon" />
                </span>
                <span class="palworld-live-map__actor-copy">
                  <strong>{{ actor.name }}</strong>
                  <span>
                    {{ palworldMapCategory(actor.kind).singular }} · X
                    {{ formatPalworldCoordinate(actor.locationX) }} · Y
                    {{ formatPalworldCoordinate(actor.locationY) }}
                  </span>
                </span>
              </button>
            </template>
          </q-virtual-scroll>
          <div v-else class="palworld-live-map__roster-empty">
            <q-icon name="filter_alt_off" size="24px" />
            <strong>No matching actors</strong>
            <span>Change the search or enable another actor type.</span>
          </div>
        </template>
        <div v-else class="palworld-live-map__roster-empty palworld-live-map__roster-empty--world">
          <q-icon name="sensors_off" size="28px" />
          <strong>No world actors reported</strong>
          <span>
            Map imagery remains available. Actors will appear when the server provides a snapshot.
          </span>
        </div>
      </aside>

      <div
        v-if="view?.unavailableReason"
        class="palworld-live-map__notice"
        :class="{ 'palworld-live-map__notice--overlay': view.available }">
        <q-icon :name="view.available ? 'history' : 'map'" />
        <span>{{ view.unavailableReason }}</span>
      </div>

      <div class="palworld-live-map__footer">
        <span v-if="!hasImagery">Coordinate grid · no map imagery configured</span>
        <span v-else>{{ activeLayer.attribution }}</span>
      </div>
    </div>
  </section>
</template>

<style scoped>
.palworld-live-map {
  position: relative;
  display: block;
  flex: 1 0 auto;
  min-height: 520px;
  height: calc(100dvh - 240px);
  overflow: hidden;
  background: var(--xy-base);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-xl);
}

.palworld-live-map--public {
  height: calc(100dvh - 112px);
}

.palworld-live-map--fullscreen {
  width: 100vw;
  height: 100dvh;
  border: 0;
  border-radius: 0;
}

.palworld-live-map__rail {
  position: absolute;
  z-index: var(--xy-z-drawer);
  top: 76px;
  bottom: 42px;
  left: var(--xy-space-base);
  display: flex;
  flex-direction: column;
  width: 288px;
  min-height: 0;
  overflow: hidden;
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-xl);
  box-shadow: var(--xy-shadow-md);
  opacity: 0;
  pointer-events: none;
  transform: translateX(calc(-100% - var(--xy-space-lg)));
  backdrop-filter: blur(10px);
  transition:
    opacity var(--xy-transition-fast),
    transform var(--xy-transition-base);
}

.palworld-live-map--rail-open .palworld-live-map__rail {
  opacity: 1;
  pointer-events: auto;
  transform: translateX(0);
}

.palworld-live-map__rail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  min-height: 58px;
  padding: var(--xy-space-sm) var(--xy-space-base);
  border-bottom: 1px solid var(--xy-border);
}

.palworld-live-map__rail-heading {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 0;
}

.palworld-live-map__rail-heading-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 32px;
  height: 32px;
  color: var(--xy-accent);
  background: var(--xy-accent-muted);
  border-radius: var(--xy-radius-md);
}

.palworld-live-map__rail-title {
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-base);
  color: var(--xy-text-primary);
}

.palworld-live-map__rail-copy,
.palworld-live-map__actor-copy span,
.palworld-live-map__roster-empty span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__search {
  margin: var(--xy-space-base) var(--xy-space-base) var(--xy-space-sm);
}

.palworld-live-map__layers {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-xs);
  padding: var(--xy-space-sm) var(--xy-space-sm) var(--xy-space-base);
}

.palworld-live-map__layer,
.palworld-live-map__actor {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 40px;
  color: var(--xy-text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  text-align: left;
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.palworld-live-map__layer {
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-base) var(--xy-space-xs) var(--xy-space-sm);
}

.palworld-live-map__layer:hover,
.palworld-live-map__layer:focus-visible,
.palworld-live-map__actor:hover,
.palworld-live-map__actor:focus-visible {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  outline: none;
}

.palworld-live-map__layer--active {
  color: var(--xy-text-primary);
  background: var(--xy-surface-overlay-soft);
}

.palworld-live-map__layer-icon,
.palworld-live-map__actor-symbol,
.palworld-live-map__selection-icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: 0 0 auto;
  color: var(--actor-color);
}

.palworld-live-map__layer-icon {
  width: 26px;
  height: 26px;
  background: color-mix(in srgb, var(--actor-color) 12%, transparent);
  border-radius: var(--xy-radius-md);
}

.palworld-live-map__layer-label {
  flex: 1;
  font-weight: 600;
}

.palworld-live-map__layer-count {
  min-width: 28px;
  color: var(--xy-text-muted);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
  text-align: right;
}

.palworld-live-map__more-layers {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 32px;
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  font: inherit;
  font-size: var(--xy-font-size-xs);
  text-align: left;
}

.palworld-live-map__more-layers:hover,
.palworld-live-map__more-layers:focus-visible {
  color: var(--xy-text-primary);
  background: var(--xy-surface-2);
  outline: none;
}

.palworld-live-map__more-layers span:nth-child(2) {
  font-family: var(--xy-font-mono);
}

.palworld-live-map__roster-heading {
  display: flex;
  justify-content: space-between;
  padding: var(--xy-space-sm) var(--xy-space-md);
  color: var(--xy-text-muted);
  font-size: var(--xy-font-size-xs);
  font-weight: 600;
  border-top: 1px solid var(--xy-border);
  border-bottom: 1px solid var(--xy-border);
}

.palworld-live-map__selected {
  display: grid;
  gap: var(--xy-space-base);
  margin: var(--xy-space-base) var(--xy-space-base) 0;
  padding: var(--xy-space-base);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-lg);
}

.palworld-live-map__selected-heading {
  display: grid;
  grid-template-columns: auto minmax(0, 1fr) auto;
  align-items: center;
  gap: var(--xy-space-sm);
}

.palworld-live-map__selected-heading > div {
  display: grid;
  min-width: 0;
}

.palworld-live-map__selected-heading > div strong,
.palworld-live-map__selected-heading > div span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palworld-live-map__selected-heading > div strong {
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
}

.palworld-live-map__selected-heading > div span,
.palworld-live-map__selected-meta {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selected-details {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: var(--xy-space-xs);
}

.palworld-live-map__selected-details > span {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: var(--xy-space-xs);
  min-width: 0;
  padding: var(--xy-space-xs) var(--xy-space-sm);
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border-radius: var(--xy-radius-sm);
  font-family: var(--xy-font-mono);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selected-details small {
  color: var(--xy-text-muted);
  font-family: var(--xy-font-body);
}

.palworld-live-map__selected-meta {
  display: grid;
  gap: var(--xy-space-2xs);
}

.palworld-live-map__roster {
  flex: 1;
  min-height: 0;
}

.palworld-live-map__actor {
  gap: var(--xy-space-sm);
  padding: var(--xy-space-sm) var(--xy-space-md);
  border-radius: 0;
}

.palworld-live-map__actor--selected {
  background: var(--xy-primary-bg-subtle);
}

.palworld-live-map__actor-symbol {
  width: 28px;
  height: 28px;
  border: 1px solid color-mix(in srgb, var(--actor-color) 52%, transparent);
  border-radius: var(--xy-radius-pill);
}

.palworld-live-map__actor-copy {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.palworld-live-map__actor-copy strong {
  overflow: hidden;
  color: var(--xy-text-primary);
  font-size: var(--xy-font-size-sm);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palworld-live-map__roster-empty {
  display: flex;
  flex: 1;
  flex-direction: column;
  align-items: center;
  justify-content: flex-start;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xl) var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
}

.palworld-live-map__roster-empty--world {
  margin: auto 0;
  padding-block: var(--xy-space-lg);
}

.palworld-live-map__roster-empty--world .q-icon {
  color: var(--xy-accent);
}

.palworld-live-map__roster-empty strong {
  color: var(--xy-text-primary);
}

.palworld-live-map__canvas-wrap {
  position: absolute;
  inset: 0;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}

.palworld-live-map__canvas {
  width: 100%;
  height: 100%;
  background-color: var(--xy-base);
  background-image:
    linear-gradient(var(--xy-chart-grid) 1px, transparent 1px),
    linear-gradient(90deg, var(--xy-chart-grid) 1px, transparent 1px);
  background-size: 32px 32px;
}

.palworld-live-map--imagery .palworld-live-map__canvas {
  background-color: color-mix(in srgb, var(--xy-base) 84%, var(--xy-info) 16%);
  background-image: radial-gradient(
    circle at 50% 46%,
    color-mix(in srgb, var(--xy-base) 76%, var(--xy-info) 24%) 0,
    color-mix(in srgb, var(--xy-base) 88%, var(--xy-info) 12%) 62%,
    var(--xy-base) 100%
  );
  background-size: auto;
}

.palworld-live-map__toolbar {
  position: absolute;
  z-index: var(--xy-z-drawer);
  top: var(--xy-space-base);
  left: var(--xy-space-base);
  right: var(--xy-space-base);
  display: flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 52px;
  padding: var(--xy-space-xs);
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid var(--xy-border-hover);
  border-radius: var(--xy-radius-xl);
  box-shadow: var(--xy-shadow-md);
  backdrop-filter: blur(10px);
}

.palworld-live-map__toolbar-action {
  color: var(--xy-text-secondary);
  transition:
    color var(--xy-transition-fast),
    background-color var(--xy-transition-fast);
}

.palworld-live-map__toolbar-action:hover,
.palworld-live-map__toolbar-action:focus-visible,
.palworld-live-map__toolbar-action--active {
  color: var(--xy-text-primary);
  background: var(--xy-surface-3);
}

.palworld-live-map__status {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-width: 154px;
  padding: 0 var(--xy-space-sm);
}

.palworld-live-map__status > .q-icon {
  font-size: 22px;
}

.palworld-live-map__status div {
  display: flex;
  flex-direction: column;
}

.palworld-live-map__status strong {
  font-size: var(--xy-font-size-sm);
}

.palworld-live-map__status span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__status--success > .q-icon {
  color: var(--xy-success);
}

.palworld-live-map__status--warning > .q-icon {
  color: var(--xy-warning);
}

.palworld-live-map__status--info > .q-icon {
  color: var(--xy-info);
}

.palworld-live-map__status--danger > .q-icon {
  color: var(--xy-danger);
}

.palworld-live-map__map-layers {
  display: flex;
  gap: var(--xy-space-2xs);
  padding: var(--xy-space-xs);
  background: var(--xy-surface-2);
  border-radius: var(--xy-radius-lg);
}

.palworld-live-map__map-layers button {
  min-height: 34px;
  padding: 0 var(--xy-space-base);
  color: var(--xy-text-secondary);
  background: transparent;
  border: 0;
  border-radius: var(--xy-radius-md);
  cursor: pointer;
  font: inherit;
}

.palworld-live-map__map-layers button:hover,
.palworld-live-map__map-layers button:focus-visible,
.palworld-live-map__map-layer--active {
  color: var(--xy-text-primary) !important;
  background: var(--xy-surface-3) !important;
  outline: none;
}

.palworld-live-map__toolbar-actions {
  display: flex;
  align-items: center;
  gap: var(--xy-space-2xs);
  margin-left: auto;
}

.palworld-live-map__toolbar-flag {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 30px;
  padding: 0 var(--xy-space-sm);
  color: var(--xy-warning);
  background: var(--xy-warning-bg-faint);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__notice,
.palworld-live-map__footer {
  position: absolute;
  z-index: var(--xy-z-drawer);
  color: var(--xy-text-secondary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-strong);
}

.palworld-live-map__notice {
  top: 50%;
  left: 50%;
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  max-width: min(520px, calc(100% - 48px));
  padding: var(--xy-space-md);
  border-radius: var(--xy-radius-lg);
  transform: translate(-50%, -50%);
  text-align: center;
}

.palworld-live-map__notice--overlay {
  top: 84px;
  transform: translateX(-50%);
}

.palworld-live-map__selection-icon {
  width: 40px;
  height: 40px;
  background: color-mix(in srgb, var(--actor-color) 12%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 45%, transparent);
  border-radius: var(--xy-radius-lg);
  font-size: 22px;
}

.palworld-live-map__footer {
  right: var(--xy-space-sm);
  bottom: var(--xy-space-sm);
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 26px;
  max-width: calc(100% - var(--xy-space-md));
  padding: 0 var(--xy-space-sm);
  overflow: hidden;
  background: color-mix(in srgb, var(--xy-surface-1) 88%, transparent);
  border-color: var(--xy-border);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 900px) {
  .palworld-live-map {
    min-height: 500px;
  }

  .palworld-live-map__rail {
    z-index: var(--xy-z-drawer);
    top: 72px;
    bottom: 40px;
    width: min(86vw, 300px);
  }

  .palworld-live-map__status {
    min-width: 0;
  }
}

@media (max-width: 700px) {
  .palworld-live-map {
    min-height: 440px;
    height: calc(100dvh - 300px);
  }

  .palworld-live-map__toolbar {
    top: var(--xy-space-sm);
    left: var(--xy-space-sm);
    right: var(--xy-space-sm);
    flex-wrap: wrap;
  }

  .palworld-live-map__status {
    flex: 1;
  }

  .palworld-live-map__map-layers {
    order: 4;
    width: 100%;
    overflow-x: auto;
  }

  .palworld-live-map__map-layers button {
    flex: 1 0 auto;
  }

  .palworld-live-map__rail {
    top: 114px;
  }

  .palworld-live-map__roster-empty--world {
    padding-block: var(--xy-space-base);
  }

  .palworld-live-map__roster-empty--world span {
    display: none;
  }
}
</style>

<style>
.palworld-map-marker {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 34px;
  padding: 4px 9px 4px 4px;
  color: var(--xy-text-primary);
  background: color-mix(in srgb, var(--xy-surface-1) 94%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 58%, var(--xy-border) 42%);
  border-radius: var(--xy-radius-pill);
  box-shadow: var(--xy-shadow-md);
  white-space: nowrap;
  transform-origin: 17px 17px;
  transition:
    transform var(--xy-transition-fast),
    border-color var(--xy-transition-fast);
  backdrop-filter: blur(6px);
}

.palworld-map-marker__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  color: var(--actor-color);
  font-size: 17px;
  background: color-mix(in srgb, var(--actor-color) 13%, transparent);
  border: 1px solid color-mix(in srgb, var(--actor-color) 42%, transparent);
  border-radius: var(--xy-radius-pill);
}

.palworld-map-marker--selected {
  border-color: var(--actor-color);
  transform: scale(1.08);
}

.palworld-map-marker--active::after {
  position: absolute;
  top: 4px;
  left: 4px;
  width: 26px;
  height: 26px;
  border: 1px solid var(--actor-color);
  border-radius: var(--xy-radius-pill);
  content: '';
  opacity: 0;
  pointer-events: none;
  animation: palworld-marker-signal calc(1.8s * var(--xy-animation-duration)) ease-out infinite;
}

@keyframes palworld-marker-signal {
  0% {
    opacity: 0.7;
    transform: scale(0.82);
  }

  75%,
  100% {
    opacity: 0;
    transform: scale(1.65);
  }
}

.palworld-map-marker__label {
  max-width: 180px;
  overflow: hidden;
  font-family: var(--xy-font-body);
  font-size: var(--xy-font-size-xs);
  font-weight: 700;
  text-overflow: ellipsis;
}

.palworld-live-map .leaflet-control-zoom a {
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border-color: var(--xy-border-strong);
}

.palworld-live-map .leaflet-top.leaflet-left {
  top: 72px;
  right: var(--xy-space-base);
  left: auto;
}

.palworld-live-map .leaflet-control-zoom {
  border: 1px solid var(--xy-border-hover);
  box-shadow: var(--xy-shadow-md);
}

.palworld-live-map .leaflet-tooltip {
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-strong);
  border-radius: var(--xy-radius-md);
  box-shadow: var(--xy-shadow-md);
  font-family: var(--xy-font-body);
}

.palworld-live-map .leaflet-tooltip::before {
  border-top-color: var(--xy-border-strong);
}

@media (max-width: 700px) {
  .palworld-live-map .leaflet-top.leaflet-left {
    top: 116px;
    right: var(--xy-space-sm);
  }
}
</style>
