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

const mapElement = ref<HTMLElement | null>(null)
const railOpen = ref(true)
const search = ref('')
const visibleKinds = ref<Record<number, boolean>>(initialPalworldMapVisibility())
const activeLayerID = ref('')
const selectedActorKey = ref('')

let map: LeafletMap | null = null
let actorLayer: L.LayerGroup | null = null
let resizeObserver: ResizeObserver | null = null
let currentCRSLayerID = ''
let fittedOnce = false
const actorMarkers = new Map<string, Marker | CircleMarker>()

const actors = computed(() => props.view?.actors ?? [])
const configuredLayers = computed(() => props.view?.layers ?? [])
const activeLayer = computed(() => {
  const configured = configuredLayers.value.find((layer) => layer.id === activeLayerID.value)
  return configured ?? configuredLayers.value[0] ?? fallbackLayer
})
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

function layerBounds(layer: PalworldMapLayer): L.LatLngBounds {
  return L.latLngBounds(L.latLng(layer.minX, layer.minY), L.latLng(layer.maxX, layer.maxY))
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
  currentCRSLayerID = layer.id
  fittedOnce = false
  map = L.map(mapElement.value, {
    crs: buildCRS(layer),
    minZoom: layer.minZoom,
    maxZoom: layer.maxZoom,
    zoomSnap: 0.5,
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
  map.fitBounds(layerBounds(layer), { animate: false, padding: [24, 24] })
}

function markerTooltip(actor: PalworldMapActor): HTMLElement {
  const element = document.createElement('span')
  element.textContent = `${actor.name} · X ${formatPalworldCoordinate(actor.locationX)}, Y ${formatPalworldCoordinate(actor.locationY)}`
  return element
}

function createActorMarker(actor: PalworldMapActor): Marker | CircleMarker {
  const category = palworldMapCategory(actor.kind)
  const location = L.latLng(actor.locationX, actor.locationY)
  const color = cssColor(category.colorToken)
  let marker: Marker | CircleMarker
  if (category.labeledMarker) {
    marker = L.marker(location, {
      icon: L.divIcon({
        className: '',
        html: `<span class="palworld-map-marker palworld-map-marker--${actor.kind}" style="--actor-color:${escapeHTML(color)}"><span class="material-icons palworld-map-marker__icon" aria-hidden="true">${escapeHTML(category.icon)}</span><span class="palworld-map-marker__label">${escapeHTML(actor.name)}</span></span>`,
        iconAnchor: [15, 15],
      }),
      keyboard: true,
      title: actor.name,
    })
  } else {
    marker = L.circleMarker(location, {
      renderer: L.canvas({ padding: 0.35 }),
      radius: actor.active ? 5 : 4,
      color,
      fillColor: color,
      fillOpacity: actor.active ? 0.82 : 0.4,
      opacity: 0.95,
      weight: 1.5,
    })
    marker.bindTooltip(markerTooltip(actor), { direction: 'top', offset: [0, -5] })
  }
  marker.on('click', () => {
    selectedActorKey.value = actor.key
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
    const location = L.latLng(actor.locationX, actor.locationY)
    if (!activeBounds.contains(location)) {
      continue
    }
    const marker = createActorMarker(actor)
    marker.addTo(actorLayer)
    actorMarkers.set(actor.key, marker)
  }
}

async function refreshMap(): Promise<void> {
  await nextTick()
  const layer = activeLayer.value
  if (map === null || currentCRSLayerID !== layer.id) {
    createMap(layer)
  }
  renderActors()
  map?.invalidateSize({ animate: false })
  if (!fittedOnce) {
    fitVisibleActors()
  }
}

function fitVisibleActors(): void {
  if (map === null) {
    return
  }
  const positioned = filteredActors.value.filter((actor) =>
    layerBounds(activeLayer.value).contains(L.latLng(actor.locationX, actor.locationY)),
  )
  if (positioned.length === 0) {
    map.fitBounds(layerBounds(activeLayer.value), { padding: [24, 24] })
    fittedOnce = true
    return
  }
  map.fitBounds(
    L.latLngBounds(positioned.map((actor) => L.latLng(actor.locationX, actor.locationY))),
    { maxZoom: Math.min(activeLayer.value.maxZoom, 4), padding: [48, 48] },
  )
  fittedOnce = true
}

async function focusActor(actor: PalworldMapActor): Promise<void> {
  visibleKinds.value[actor.kind] = true
  selectedActorKey.value = actor.key
  await nextTick()
  renderActors()
  const location = L.latLng(actor.locationX, actor.locationY)
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

watch(
  () => props.view,
  () => {
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

onMounted(() => {
  void refreshMap()
  if (mapElement.value !== null) {
    resizeObserver = new ResizeObserver(() => map?.invalidateSize({ animate: false }))
    resizeObserver.observe(mapElement.value)
  }
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  destroyMap()
})
</script>

<template>
  <section
    class="palworld-live-map"
    :class="{ 'palworld-live-map--rail-open': railOpen, 'palworld-live-map--public': publicMode }">
    <aside class="palworld-live-map__rail" :aria-hidden="!railOpen">
      <div class="palworld-live-map__rail-header">
        <div>
          <div class="palworld-live-map__rail-title">World actors</div>
          <div class="palworld-live-map__rail-copy">
            {{ actors.length.toLocaleString() }} reported
          </div>
        </div>
        <q-btn
          aria-label="Close actor rail"
          flat
          icon="left_panel_close"
          round
          size="sm"
          @click="railOpen = false" />
      </div>

      <q-input
        v-model="search"
        aria-label="Search world actors"
        class="palworld-live-map__search"
        clearable
        dense
        outlined
        placeholder="Search names, guilds, or classes">
        <template #prepend><q-icon name="search" /></template>
      </q-input>

      <div class="palworld-live-map__layers" aria-label="Actor layers">
        <button
          v-for="category in palworldMapCategories"
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
            size="18px" />
        </button>
      </div>

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
              :style="{ '--actor-color': `var(${palworldMapCategory(actor.kind).colorToken})` }">
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
        <q-icon name="filter_alt_off" size="28px" />
        <strong>No actors match</strong>
        <span>Adjust the search or enable another actor layer.</span>
      </div>
    </aside>

    <div class="palworld-live-map__canvas-wrap">
      <div ref="mapElement" class="palworld-live-map__canvas" aria-label="Palworld live map" />

      <div class="palworld-live-map__toolbar">
        <q-btn
          v-if="!railOpen"
          aria-label="Open actor rail"
          class="palworld-live-map__toolbar-button"
          icon="left_panel_open"
          round
          unelevated
          @click="railOpen = true" />
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
        <q-btn
          aria-label="Refresh live map"
          class="palworld-live-map__toolbar-button palworld-live-map__refresh"
          :loading="loading"
          icon="refresh"
          round
          unelevated
          @click="emit('refresh')" />
      </div>

      <div
        v-if="view?.unavailableReason"
        class="palworld-live-map__notice"
        :class="{ 'palworld-live-map__notice--overlay': view.available }">
        <q-icon :name="view.available ? 'history' : 'map'" />
        <span>{{ view.unavailableReason }}</span>
      </div>

      <div v-if="view?.partial || view?.truncated" class="palworld-live-map__data-flags">
        <span v-if="view.partial"><q-icon name="group" /> Players-only fallback</span>
        <span v-if="view.truncated"><q-icon name="data_usage" /> Actor limit reached</span>
      </div>

      <div v-if="selectedActor !== null" class="palworld-live-map__selection" role="status">
        <span
          class="palworld-live-map__selection-icon"
          :style="{
            '--actor-color': `var(${palworldMapCategory(selectedActor.kind).colorToken})`,
          }">
          <q-icon :name="palworldMapCategory(selectedActor.kind).icon" />
        </span>
        <div class="palworld-live-map__selection-main">
          <div class="palworld-live-map__selection-heading">
            <strong>{{ selectedActor.name }}</strong>
            <span>{{ palworldMapCategory(selectedActor.kind).singular }}</span>
          </div>
          <div class="palworld-live-map__selection-details">
            <span>X {{ formatPalworldCoordinate(selectedActor.locationX) }}</span>
            <span>Y {{ formatPalworldCoordinate(selectedActor.locationY) }}</span>
            <span>Z {{ formatPalworldCoordinate(selectedActor.locationZ) }}</span>
            <span v-if="selectedActor.level > 0">Level {{ selectedActor.level }}</span>
            <span v-if="selectedActor.guildName">Guild {{ selectedActor.guildName }}</span>
            <span v-if="selectedActor.trainerName">Trainer {{ selectedActor.trainerName }}</span>
            <span v-if="selectedActor.action">{{ selectedActor.action }}</span>
          </div>
        </div>
        <q-btn
          aria-label="Close actor details"
          flat
          icon="close"
          round
          @click="closeSelectedActor" />
      </div>

      <div class="palworld-live-map__footer">
        <span v-if="activeLayer.tileUrlTemplate === ''"
          >Coordinate grid · no map imagery configured</span
        >
        <span v-else>{{ activeLayer.attribution }}</span>
        <button type="button" @click="fitVisibleActors">Fit visible actors</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.palworld-live-map {
  position: relative;
  display: grid;
  grid-template-columns: 0 minmax(0, 1fr);
  min-height: 520px;
  height: calc(100dvh - 178px);
  max-height: 920px;
  overflow: hidden;
  background: var(--xy-surface-0);
  border: 1px solid var(--xy-border);
  border-radius: var(--xy-radius-xl);
}

.palworld-live-map--rail-open {
  grid-template-columns: 310px minmax(0, 1fr);
}

.palworld-live-map--public {
  height: calc(100dvh - 112px);
  max-height: none;
}

.palworld-live-map__rail {
  position: relative;
  z-index: var(--xy-z-sticky);
  display: flex;
  flex-direction: column;
  min-width: 0;
  overflow: hidden;
  background: var(--xy-surface-1);
  border-right: 1px solid var(--xy-border);
  opacity: 0;
  pointer-events: none;
  transition:
    opacity var(--xy-transition-fast),
    transform var(--xy-transition-base);
}

.palworld-live-map--rail-open .palworld-live-map__rail {
  opacity: 1;
  pointer-events: auto;
}

.palworld-live-map__rail-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-md);
  border-bottom: 1px solid var(--xy-border);
}

.palworld-live-map__rail-title {
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  color: var(--xy-text-primary);
}

.palworld-live-map__rail-copy,
.palworld-live-map__actor-copy span,
.palworld-live-map__roster-empty span {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__search {
  margin: var(--xy-space-base) var(--xy-space-md) var(--xy-space-sm);
}

.palworld-live-map__layers {
  display: flex;
  flex-direction: column;
  gap: var(--xy-space-2xs);
  padding: 0 var(--xy-space-sm) var(--xy-space-base);
}

.palworld-live-map__layer,
.palworld-live-map__actor {
  display: flex;
  align-items: center;
  width: 100%;
  min-height: 44px;
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
  padding: var(--xy-space-xs) var(--xy-space-sm);
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
  background: var(--xy-primary-bg-subtle);
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
  width: 28px;
  height: 28px;
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
  justify-content: center;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-lg);
  color: var(--xy-text-secondary);
  text-align: center;
}

.palworld-live-map__canvas-wrap {
  position: relative;
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

.palworld-live-map__toolbar {
  position: absolute;
  z-index: var(--xy-z-sticky);
  top: var(--xy-space-md);
  left: var(--xy-space-md);
  right: var(--xy-space-md);
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  pointer-events: none;
}

.palworld-live-map__toolbar > * {
  pointer-events: auto;
}

.palworld-live-map__toolbar-button,
.palworld-live-map__status,
.palworld-live-map__map-layers {
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-strong);
  box-shadow: var(--xy-shadow-md);
}

.palworld-live-map__status {
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 44px;
  padding: var(--xy-space-xs) var(--xy-space-base);
  border-radius: var(--xy-radius-lg);
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
  border-radius: var(--xy-radius-lg);
}

.palworld-live-map__map-layers button,
.palworld-live-map__footer button {
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

.palworld-live-map__refresh {
  margin-left: auto;
}

.palworld-live-map__notice,
.palworld-live-map__data-flags,
.palworld-live-map__footer {
  position: absolute;
  z-index: var(--xy-z-sticky);
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
  top: 76px;
  transform: translateX(-50%);
}

.palworld-live-map__data-flags {
  top: 78px;
  right: var(--xy-space-md);
  display: flex;
  gap: var(--xy-space-sm);
  padding: var(--xy-space-xs) var(--xy-space-sm);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selection {
  position: absolute;
  z-index: var(--xy-z-sticky);
  left: 50%;
  bottom: 42px;
  display: flex;
  align-items: flex-start;
  gap: var(--xy-space-base);
  width: min(680px, calc(100% - 48px));
  padding: var(--xy-space-md);
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid var(--xy-border-strong);
  border-radius: var(--xy-radius-xl);
  box-shadow: var(--xy-shadow-lg);
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

.palworld-live-map__selection-main {
  flex: 1;
  min-width: 0;
}

.palworld-live-map__selection-heading {
  display: flex;
  align-items: baseline;
  gap: var(--xy-space-sm);
}

.palworld-live-map__selection-heading strong {
  overflow: hidden;
  font-family: var(--xy-font-heading);
  font-size: var(--xy-font-size-lg);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.palworld-live-map__selection-heading span,
.palworld-live-map__selection-details {
  color: var(--xy-text-secondary);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__selection-details {
  display: flex;
  flex-wrap: wrap;
  gap: var(--xy-space-xs) var(--xy-space-md);
  margin-top: var(--xy-space-xs);
  font-family: var(--xy-font-mono);
}

.palworld-live-map__footer {
  right: var(--xy-space-sm);
  bottom: var(--xy-space-sm);
  display: flex;
  align-items: center;
  gap: var(--xy-space-sm);
  min-height: 30px;
  padding-left: var(--xy-space-sm);
  border-radius: var(--xy-radius-md);
  font-size: var(--xy-font-size-xs);
}

.palworld-live-map__footer button {
  min-height: 28px;
  color: var(--xy-accent);
}

@media (max-width: 900px) {
  .palworld-live-map,
  .palworld-live-map--rail-open {
    grid-template-columns: minmax(0, 1fr);
    height: calc(100dvh - 150px);
    min-height: 500px;
  }

  .palworld-live-map__rail {
    position: absolute;
    inset: 0 auto 0 0;
    z-index: var(--xy-z-drawer);
    width: min(86vw, 340px);
    transform: translateX(-100%);
    box-shadow: var(--xy-shadow-xl);
  }

  .palworld-live-map--rail-open .palworld-live-map__rail {
    transform: translateX(0);
  }

  .palworld-live-map__map-layers {
    order: 4;
    width: 100%;
    overflow-x: auto;
  }

  .palworld-live-map__toolbar {
    flex-wrap: wrap;
  }

  .palworld-live-map__selection {
    bottom: 48px;
    width: calc(100% - 24px);
  }

  .palworld-live-map__data-flags {
    top: auto;
    right: var(--xy-space-sm);
    bottom: 84px;
    flex-direction: column;
  }
}

@media (max-width: 560px) {
  .palworld-live-map__toolbar {
    top: var(--xy-space-sm);
    left: var(--xy-space-sm);
    right: var(--xy-space-sm);
  }

  .palworld-live-map__status {
    flex: 1;
  }

  .palworld-live-map__selection-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: var(--xy-space-2xs);
  }

  .palworld-live-map__selection-details {
    max-height: 58px;
    overflow-y: auto;
  }

  .palworld-live-map__footer {
    left: var(--xy-space-sm);
    justify-content: space-between;
  }
}
</style>

<style>
.palworld-map-marker {
  display: inline-flex;
  align-items: center;
  gap: var(--xy-space-xs);
  min-height: 30px;
  padding: 3px 8px 3px 4px;
  color: var(--xy-text-primary);
  background: var(--xy-surface-1);
  border: 1px solid color-mix(in srgb, var(--actor-color) 62%, var(--xy-border) 38%);
  border-radius: var(--xy-radius-pill);
  box-shadow: var(--xy-shadow-md);
  white-space: nowrap;
}

.palworld-map-marker__icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 22px;
  height: 22px;
  color: var(--actor-color);
  font-size: 16px;
  background: color-mix(in srgb, var(--actor-color) 13%, transparent);
  border-radius: var(--xy-radius-pill);
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
</style>
